// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"runbooks/internal/gitrepo"
	"runbooks/stores"
)

type gitSyncRequest struct {
	RunbookSlug   string      `json:"runbook_slug"`
	RunbookTitle  string      `json:"runbook_title"`
	Notes         string      `json:"notes"`
	RunbookSource string      `json:"runbook_source"`
	Images        []syncImage `json:"images"`
}

// gitSyncJob is one sync: the parsed request plus the commit identity resolved
// from the caller's session (or the machine fallback).
type gitSyncJob struct {
	req    gitSyncRequest
	author commitAuthor
}

type commitAuthor struct {
	name  string
	email string
}

// resolveAuthor picks the commit identity: the user record when it carries an
// email, otherwise the configured machine identity (the automation fallback).
func resolveAuthor(cfg config, u stores.User) commitAuthor {
	if u.ID != "" && u.Email != "" {
		name := u.DisplayName
		if name == "" {
			name = cfg.GitSyncAuthorName
		}
		return commitAuthor{name: name, email: u.Email}
	}
	return commitAuthor{name: cfg.GitSyncAuthorName, email: cfg.GitSyncAuthorEmail}
}

type syncImage struct {
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	DataB64  string `json:"data_base64"`
}

var (
	gitSyncMu sync.Mutex
	imgRefRe  = regexp.MustCompile(`!\[([^\]]*)\]\[img-(\d+)\]`)
	mimeToExt = map[string]string{
		"jpeg": "jpg",
		"jpg":  "jpg",
		"png":  "png",
		"gif":  "gif",
		"webp": "webp",
	}
)

const bearerPrefix = "Bearer "

func handleGitSync(cfg config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if !cfg.GitSyncEnabled {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "git sync not configured"})
			return
		}

		// A user session (or a proxy assertion) satisfies the endpoint;
		// otherwise the shared instance token does, as the documented
		// automation fallback for CI and scripts. With identity on and no
		// token, an unauthenticated caller is refused rather than assumed to
		// sit behind upstream auth; only a session or the token gets in.
		if !sessionAuthorized(r.Context()) {
			switch {
			case cfg.GitSyncAPIToken != "":
				auth := r.Header.Get("Authorization")
				token := strings.TrimPrefix(auth, bearerPrefix)
				if !strings.HasPrefix(auth, bearerPrefix) ||
					subtle.ConstantTimeCompare([]byte(token), []byte(cfg.GitSyncAPIToken)) != 1 {
					w.WriteHeader(http.StatusUnauthorized)
					json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
					return
				}
			case cfg.IdentityEnabled:
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
		}

		var req gitSyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		sha, changed, err := doGitSync(ctx, cfg, gitSyncJob{
			req:    req,
			author: resolveAuthor(cfg, userFrom(r.Context())),
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if !changed {
			json.NewEncoder(w).Encode(map[string]string{"status": "up_to_date"})
			return
		}
		if sha == "" {
			json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"commit_sha": sha})
	}
}

var runbookSlugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func doGitSync(ctx context.Context, cfg config, job gitSyncJob) (string, bool, error) {
	req := job.req
	// The slug is the record directory name; reject anything that could climb
	// out of the records root.
	if !runbookSlugRe.MatchString(req.RunbookSlug) {
		return "", false, fmt.Errorf("invalid runbook slug")
	}

	gitSyncMu.Lock()
	defer gitSyncMu.Unlock()

	files := snapshotFiles(req)
	now := time.Now().UTC()

	// A configured repo is the destination; otherwise write to the local folder.
	if cfg.GitSyncRepo == "" {
		return writeRecordsFolder(cfg.GitSyncDir, req.RunbookSlug, files, now)
	}
	return writeRecordsRepo(ctx, cfg, job, files, now)
}

// openRecordsRoot returns a root confined to dir, creating dir if needed. Every
// name used against the root resolves beneath it, so a hostile snapshot name
// cannot escape (os.Root, Go 1.24+).
func openRecordsRoot(dir string) (*os.Root, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return os.OpenRoot(dir)
}

// writeRecordsFolder writes one immutable snapshot into the local records
// folder. There is no repository and no commit; the read surface serves it.
func writeRecordsFolder(base, slug string, files map[string][]byte, now time.Time) (string, bool, error) {
	root, err := openRecordsRoot(base)
	if err != nil {
		return "", false, fmt.Errorf("open records dir failed")
	}
	defer root.Close()

	_, changed, err := writeSnapshot(root, slug, files, now)
	return "", changed, err
}

// writeSnapshot writes one snapshot under root, skipping a re-sync that changes
// nothing. It returns the snapshot name ("" when unchanged).
func writeSnapshot(root *os.Root, slug string, files map[string][]byte, now time.Time) (string, bool, error) {
	prev, err := latestSnapshot(root, now.Format("20060102"), slug)
	if err != nil {
		return "", false, fmt.Errorf("read records failed")
	}
	if prev != "" && sameSnapshot(root, prev, files) {
		return "", false, nil
	}
	name, err := newSnapshotDir(root, now, slug)
	if err != nil {
		return "", false, fmt.Errorf("snapshot path failed")
	}
	if err := root.MkdirAll(name, 0o755); err != nil {
		return "", false, fmt.Errorf("mkdir failed")
	}
	for fname, data := range files {
		if err := root.WriteFile(path.Join(name, fname), data, 0o644); err != nil {
			return "", false, fmt.Errorf("write %s failed", fname)
		}
	}
	return name, true, nil
}

// writeRecordsRepo commits one immutable snapshot into the configured repo.
func writeRecordsRepo(ctx context.Context, cfg config, job gitSyncJob, files map[string][]byte, now time.Time) (string, bool, error) {
	req := job.req
	ws, err := os.MkdirTemp("", "runbooks-gitsync-*")
	if err != nil {
		return "", false, fmt.Errorf("workspace creation failed")
	}
	defer os.RemoveAll(ws)

	creds := gitrepo.Credentials{
		Username: cfg.GitSyncUsername,
		Token:    cfg.GitSyncToken,
		SSHKey:   cfg.GitSyncSSHKey,
	}
	repo, err := gitrepo.Fresh(ctx, ws, cfg.GitSyncRepo, cfg.GitSyncBranch, creds)
	if err != nil {
		// Generic to the caller: the error can carry the remote URL.
		log.Printf("gitsync: clone failed: %v", err)
		return "", false, fmt.Errorf("clone failed")
	}

	root, err := openRecordsRoot(filepath.Join(ws, cfg.GitSyncBasePath))
	if err != nil {
		return "", false, fmt.Errorf("open records dir failed")
	}
	defer root.Close()
	if _, changed, err := writeSnapshot(root, req.RunbookSlug, files, now); err != nil {
		return "", false, err
	} else if !changed {
		return "", false, nil
	}

	msg := fmt.Sprintf("notes: %s (%s)", req.RunbookTitle, req.RunbookSlug)
	hash, changed, err := gitrepo.Commit(repo, msg, gitrepo.Author{Name: job.author.name, Email: job.author.email})
	if err != nil {
		log.Printf("gitsync: commit failed: %v", err)
		return "", false, fmt.Errorf("git commit failed")
	}
	// Nothing changed since the last sync; report up-to-date rather than
	// creating an empty commit.
	if !changed {
		return "", false, nil
	}
	if err := gitrepo.Push(ctx, repo, cfg.GitSyncBranch, creds); err != nil {
		log.Printf("gitsync: push failed: %v", err)
		return "", false, fmt.Errorf("git push failed")
	}
	return hash.String(), true, nil
}

// snapshotFiles renders the record's files in memory: the notes body with image
// refs rewritten to local filenames, the runbook source, and each pasted image.
func snapshotFiles(req gitSyncRequest) map[string][]byte {
	files := make(map[string][]byte, len(req.Images)+2)

	imgNames := make(map[string]string, len(req.Images))
	for _, img := range req.Images {
		subtype := strings.TrimPrefix(img.MimeType, "image/")
		ext, ok := mimeToExt[subtype]
		if !ok {
			ext = subtype
		}
		fname := img.Name + "." + ext
		imgNames[img.Name] = fname

		data, err := base64.StdEncoding.DecodeString(img.DataB64)
		if err != nil {
			continue
		}
		files[fname] = data
	}

	notes := imgRefRe.ReplaceAllStringFunc(req.Notes, func(match string) string {
		sub := imgRefRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		alt, name := sub[1], "img-"+sub[2]
		if fname, ok := imgNames[name]; ok {
			return fmt.Sprintf("![%s](%s)", alt, fname)
		}
		return match
	})

	files["notes.md"] = []byte(notes)
	files["runbook.md"] = []byte(req.RunbookSource)
	return files
}

// latestSnapshot returns the newest record directory name for a UTC date and
// slug, or "" when none exists. The timestamp prefixes sort lexically, so the
// greatest name is the most recent.
func latestSnapshot(root *os.Root, date, slug string) (string, error) {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return "", err
	}
	prefix, suffix := date+"T", "-"+slug
	latest := ""
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}
		if name > latest {
			latest = name
		}
	}
	return latest, nil
}

// sameSnapshot reports whether a record directory holds exactly the given files
// and nothing else.
func sameSnapshot(root *os.Root, name string, files map[string][]byte) bool {
	entries, err := fs.ReadDir(root.FS(), name)
	if err != nil {
		return false
	}
	seen := 0
	for _, e := range entries {
		if e.IsDir() {
			return false
		}
		want, ok := files[e.Name()]
		if !ok {
			return false
		}
		got, err := root.ReadFile(path.Join(name, e.Name()))
		if err != nil || !bytes.Equal(got, want) {
			return false
		}
		seen++
	}
	return seen == len(files)
}

// newSnapshotDir names a fresh record directory. The timestamp has second
// precision, so two syncs in the same second bump forward rather than collide.
func newSnapshotDir(root *os.Root, now time.Time, slug string) (string, error) {
	ts := now.Truncate(time.Second)
	for {
		name := ts.Format("20060102T150405Z") + "-" + slug
		switch _, err := root.Lstat(name); {
		case err == nil:
			ts = ts.Add(time.Second)
		case errors.Is(err, fs.ErrNotExist):
			return name, nil
		default:
			return "", err
		}
	}
}
