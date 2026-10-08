// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func folderJob(slug, notes string) gitSyncJob {
	return gitSyncJob{
		req: gitSyncRequest{
			RunbookSlug:   slug,
			RunbookTitle:  "Replication Lag",
			Notes:         notes,
			RunbookSource: "---\ntitle: Replication Lag\n---\n\n## Step\n",
			Images: []syncImage{{
				Name:     "img-1",
				MimeType: "image/png",
				DataB64:  base64.StdEncoding.EncodeToString([]byte("png-bytes")),
			}},
		},
		author: commitAuthor{name: "Tester", email: "t@example.com"},
	}
}

// TestRecordsFolderRoundTrip writes a record to a local folder and reads it back
// through the index and the static file route.
func TestRecordsFolderRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := config{GitSyncDir: dir}

	if _, changed, err := doGitSync(context.Background(), cfg, folderJob("mts-deadlock", "The worker was the SQL thread.")); err != nil || !changed {
		t.Fatalf("doGitSync changed=%v err=%v, want a new snapshot", changed, err)
	}
	// A re-sync with the same content is a no-op.
	if _, changed, err := doGitSync(context.Background(), cfg, folderJob("mts-deadlock", "The worker was the SQL thread.")); err != nil || changed {
		t.Fatalf("re-sync changed=%v err=%v, want up-to-date", changed, err)
	}

	rs := newRecordsSource(cfg)
	rec := httptest.NewRecorder()
	rs.index(rec, httptest.NewRequest(http.MethodGet, "/api/runbooks/v1/notes?slug=mts-deadlock", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("index status = %d, want 200", rec.Code)
	}
	var out recordsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Snapshots) != 1 || out.Total != 1 {
		t.Fatalf("got %d snapshots (total %d), want 1", len(out.Snapshots), out.Total)
	}
	snap := out.Snapshots[0]
	if snap.Notes != "The worker was the SQL thread." {
		t.Errorf("notes = %q", snap.Notes)
	}
	if !strings.Contains(snap.Runbook, "## Step") {
		t.Errorf("runbook = %q, want the as-executed source", snap.Runbook)
	}
	if len(snap.Files) != 1 || snap.Files[0] != "img-1.png" {
		t.Errorf("files = %v, want [img-1.png]", snap.Files)
	}

	// The image is served statically, with its content type.
	rec = httptest.NewRecorder()
	path := "/api/runbooks/v1/notes/mts-deadlock/" + snap.TakenAt + "/img-1.png"
	rs.file(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("file status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q", ct)
	}
	if rec.Body.String() != "png-bytes" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

// TestRecordsDisabled pins that no destination means a 404, not an empty list.
func TestRecordsDisabled(t *testing.T) {
	rs := newRecordsSource(config{GitSyncDir: ""})
	rec := httptest.NewRecorder()
	rs.index(rec, httptest.NewRequest(http.MethodGet, "/api/runbooks/v1/notes?slug=x", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// TestRecordsRejectsBadPaths pins slug and path validation (traversal).
func TestRecordsRejectsBadPaths(t *testing.T) {
	rs := newRecordsSource(config{GitSyncDir: t.TempDir()})
	for _, target := range []string{
		"/api/runbooks/v1/notes?slug=../../etc",
		"/api/runbooks/v1/notes/MTS/20260101T000000Z/img-1.png",
		"/api/runbooks/v1/notes/ok/2026/../../img-1.png",
		"/api/runbooks/v1/notes/ok/20260101T000000Z/../notes.md",
	} {
		rec := httptest.NewRecorder()
		if strings.Contains(target, "slug=") {
			rs.index(rec, httptest.NewRequest(http.MethodGet, target, nil))
		} else {
			rs.file(rec, httptest.NewRequest(http.MethodGet, target, nil))
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, rec.Code)
		}
	}
}

// TestRecordsPagination pins newest-first ordering and paging.
func TestRecordsPagination(t *testing.T) {
	dir := t.TempDir()
	cfg := config{GitSyncDir: dir}
	for _, notes := range []string{"one", "two", "three"} {
		if _, _, err := doGitSync(context.Background(), cfg, folderJob("x", notes)); err != nil {
			t.Fatalf("doGitSync: %v", err)
		}
	}
	rs := newRecordsSource(cfg)
	rec := httptest.NewRecorder()
	rs.index(rec, httptest.NewRequest(http.MethodGet, "/api/runbooks/v1/notes?slug=x&page=1&limit=2", nil))
	var out recordsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Total != 3 || len(out.Snapshots) != 2 || out.Limit != 2 {
		t.Fatalf("total=%d len=%d limit=%d, want 3/2/2", out.Total, len(out.Snapshots), out.Limit)
	}
	if out.Snapshots[0].Notes != "three" {
		t.Errorf("first snapshot = %q, want the newest", out.Snapshots[0].Notes)
	}
}
