// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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
		// sit behind upstream auth — only a session or the token gets in.
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
		json.NewEncoder(w).Encode(map[string]string{"commit_sha": sha})
	}
}

func doGitSync(ctx context.Context, cfg config, job gitSyncJob) (string, bool, error) {
	req := job.req

	gitSyncMu.Lock()
	defer gitSyncMu.Unlock()

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

	date := time.Now().UTC().Format("2006-01-02")
	dir := filepath.Join(ws, cfg.GitSyncBasePath, date, req.RunbookSlug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, fmt.Errorf("mkdir failed")
	}

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
		if err := os.WriteFile(filepath.Join(dir, fname), data, 0o644); err != nil {
			return "", false, fmt.Errorf("write image failed")
		}
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

	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte(notes), 0o644); err != nil {
		return "", false, fmt.Errorf("write notes failed")
	}
	if err := os.WriteFile(filepath.Join(dir, "runbook.md"), []byte(req.RunbookSource), 0o644); err != nil {
		return "", false, fmt.Errorf("write runbook failed")
	}

	msg := fmt.Sprintf("sync: %s %s", req.RunbookTitle, date)
	hash, changed, err := gitrepo.Commit(repo, msg, gitrepo.Author{Name: job.author.name, Email: job.author.email})
	if err != nil {
		log.Printf("gitsync: commit failed: %v", err)
		return "", false, fmt.Errorf("git commit failed")
	}
	// Nothing changed since the last sync — report up-to-date rather than
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
