// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"runbooks/internal/gitrepo"
)

// recordsSource serves the execution records the app writes: a local folder by
// default, or the configured repo cloned into a cache. Read-only.
type recordsSource struct {
	cfg config
	mu  sync.Mutex
}

func newRecordsSource(cfg config) *recordsSource { return &recordsSource{cfg: cfg} }

// root returns the directory that holds the snapshot folders, or "" when the
// operator configured no records destination.
func (s *recordsSource) root(ctx context.Context) (string, error) {
	if s.cfg.GitSyncRepo == "" {
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

type recordSnapshot struct {
	TakenAt string   `json:"taken_at"`
	Runbook string   `json:"runbook"`
	Notes   string   `json:"notes"`
	Files   []string `json:"files"`
}

type recordsResponse struct {
	Snapshots []recordSnapshot `json:"snapshots"`
	Page      int              `json:"page"`
	Limit     int              `json:"limit"`
	Total     int              `json:"total"`
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
	page := queryInt(r, "page", 1, 1, 1<<30)
	limit := queryInt(r, "limit", 20, 1, 100)

	root, err := s.root(r.Context())
	if err != nil || root == "" {
		writeJSONError(w, http.StatusNotFound, "records not configured")
		return
	}

	names, err := recordSnapshotDirs(root, slug)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not read records")
		return
	}
	total := len(names)
	start := clamp((page-1)*limit, 0, total)
	end := clamp(start+limit, 0, total)

	out := recordsResponse{Snapshots: []recordSnapshot{}, Page: page, Limit: limit, Total: total}
	for _, name := range names[start:end] {
		out.Snapshots = append(out.Snapshots, readSnapshot(filepath.Join(root, name), name))
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
	root, err := s.root(r.Context())
	if err != nil || root == "" {
		writeJSONError(w, http.StatusNotFound, "records not configured")
		return
	}
	path := filepath.Join(root, parts[1]+"-"+parts[0], parts[2])
	if !strings.HasPrefix(path, filepath.Clean(root)+string(os.PathSeparator)) {
		writeJSONError(w, http.StatusBadRequest, "invalid record path")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", recordContentType(parts[2]))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// snapshotDirs lists a runbook's snapshot directories, newest first. The
// timestamp prefix sorts lexically, so a descending string sort is chronological.
func recordSnapshotDirs(root, slug string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), "-"+slug) && snapshotNameRe.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names, nil
}

// readSnapshot reads one snapshot's notes and runbook, and lists its other files.
func readSnapshot(dir, name string) recordSnapshot {
	snap := recordSnapshot{TakenAt: snapshotNameRe.FindStringSubmatch(name)[1]}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return snap
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch e.Name() {
		case "notes.md":
			if b, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
				snap.Notes = string(b)
			}
		case "runbook.md":
			if b, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
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

func queryInt(r *http.Request, key string, def, min, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil || v < min {
		return def
	}
	if v > max {
		return max
	}
	return v
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
