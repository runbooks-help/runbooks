// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"runbooks/internal/gitrepo"
	"runbooks/internal/pagination"
)

// recordsSource serves the execution records the app writes: a local folder by
// default, or the configured repo cloned into a cache. Read-only.
type recordsSource struct {
	cfg config
	mu  sync.Mutex
}

func newRecordsSource(cfg config) *recordsSource { return &recordsSource{cfg: cfg} }

var errRecordsDisabled = errors.New("records not configured")

// base resolves the directory that holds the snapshot folders, or
// errRecordsDisabled when the operator configured no destination.
func (s *recordsSource) base(ctx context.Context) (string, error) {
	if s.cfg.GitSyncRepo == "" {
		if s.cfg.GitSyncDir == "" {
			return "", errRecordsDisabled
		}
		return s.cfg.GitSyncDir, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	creds := gitrepo.Credentials{
		Username: s.cfg.GitSyncUsername,
		Token:    s.cfg.GitSyncToken,
		SSHKey:   s.cfg.GitSyncSSHKey,
	}
	if _, err := gitrepo.Ensure(ctx, s.cfg.GitSyncCache, s.cfg.GitSyncRepo, s.cfg.GitSyncBranch, creds, func(err error) {
		log.Printf("records: fetch failed: %v", err)
	}); err != nil {
		return "", err
	}
	return filepath.Join(s.cfg.GitSyncCache, s.cfg.GitSyncBasePath), nil
}

// openRoot confines reads beneath the records base. Every name used against the
// root resolves inside it (os.Root), so a hostile path cannot escape.
func (s *recordsSource) openRoot(ctx context.Context) (*os.Root, error) {
	base, err := s.base(ctx)
	if err != nil {
		return nil, err
	}
	return os.OpenRoot(base)
}

type recordSnapshot struct {
	TakenAt string   `json:"taken_at"`
	Runbook string   `json:"runbook"`
	Notes   string   `json:"notes"`
	Files   []string `json:"files"`
}

type recordsResponse struct {
	Snapshots []recordSnapshot `json:"snapshots"`
	pagination.Response
}

var (
	snapshotNameRe = regexp.MustCompile(`^(\d{8}T\d{6}Z)-`)
	takenAtRe      = regexp.MustCompile(`^\d{8}T\d{6}Z$`)
	safeFileRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// index serves GET /api/runbooks/v1/notes?slug=<slug>&page=&limit= with the
// snapshots for one runbook, newest first.
func (s *recordsSource) index(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	slug := r.URL.Query().Get("slug")
	if !runbookSlugRe.MatchString(slug) {
		writeJSONError(w, http.StatusBadRequest, "invalid slug")
		return
	}
	page := pagination.Parse(r)

	root, err := s.openRoot(r.Context())
	switch {
	case errors.Is(err, errRecordsDisabled):
		writeJSONError(w, http.StatusNotFound, "records not configured")
		return
	case errors.Is(err, fs.ErrNotExist):
		writeJSON(w, http.StatusOK, recordsResponse{Snapshots: []recordSnapshot{}, Response: pagination.BuildResponse(page, 0)})
		return
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, "could not read records")
		return
	}
	defer root.Close()

	names := recordSnapshotNames(root, slug)
	start, end := page.Bounds(len(names))

	out := recordsResponse{Snapshots: []recordSnapshot{}, Response: pagination.BuildResponse(page, len(names))}
	for _, name := range names[start:end] {
		out.Snapshots = append(out.Snapshots, readSnapshot(root, name))
	}
	writeJSON(w, http.StatusOK, out)
}

// file serves GET /api/runbooks/v1/notes/<slug>/<taken_at>/<file> for a record's
// non-markdown files (images), verbatim with their content type.
func (s *recordsSource) file(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/runbooks/v1/notes/"), "/")
	if len(parts) != 3 || !runbookSlugRe.MatchString(parts[0]) || !takenAtRe.MatchString(parts[1]) || !safeFile(parts[2]) {
		writeJSONError(w, http.StatusBadRequest, "invalid record path")
		return
	}
	root, err := s.openRoot(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	defer root.Close()

	data, err := root.ReadFile(path.Join(parts[1]+"-"+parts[0], parts[2]))
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", recordContentType(parts[2]))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// recordSnapshotNames lists a runbook's snapshot directories, newest first. The
// timestamp prefix sorts lexically, so a descending sort is chronological.
func recordSnapshotNames(root *os.Root, slug string) []string {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && snapshotNameRe.MatchString(e.Name()) && strings.HasSuffix(e.Name(), "-"+slug) {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names
}

// readSnapshot reads one snapshot's notes and runbook, and lists its other files.
func readSnapshot(root *os.Root, name string) recordSnapshot {
	snap := recordSnapshot{TakenAt: snapshotNameRe.FindStringSubmatch(name)[1]}
	entries, err := fs.ReadDir(root.FS(), name)
	if err != nil {
		return snap
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch e.Name() {
		case "notes.md":
			if b, err := root.ReadFile(path.Join(name, e.Name())); err == nil {
				snap.Notes = string(b)
			}
		case "runbook.md":
			if b, err := root.ReadFile(path.Join(name, e.Name())); err == nil {
				snap.Runbook = string(b)
			}
		default:
			snap.Files = append(snap.Files, e.Name())
		}
	}
	sort.Strings(snap.Files)
	return snap
}

func recordContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md":
		return "text/markdown; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

func safeFile(name string) bool { return safeFileRe.MatchString(name) }
