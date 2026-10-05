// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"runbooks/stores"
)

// initTestRepo creates an empty bare git repo, using go-git (no system git).
// Returns the file:// URL of the bare repo.
func initTestRepo(t *testing.T) string {
	t.Helper()
	bare := t.TempDir()
	r, err := git.PlainInit(bare, true)
	if err != nil {
		t.Fatal(err)
	}
	// Point the bare repo's HEAD at main, the branch sync pushes to, so a clone
	// checks it out.
	if err := r.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatal(err)
	}
	return "file://" + bare
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func testCfg(repoURL string) config {
	return config{
		GitSyncRepo:        repoURL,
		GitSyncBranch:      "main",
		GitSyncBasePath:    "runs",
		GitSyncAuthorName:  "Test Bot",
		GitSyncAuthorEmail: "bot@test.com",
		GitSyncUsername:    "oauth2",
		GitSyncToken:       "test-token",
		GitSyncEnabled:     repoURL != "",
	}
}

// snapshotDirs lists the record directories under base in a clone.
func snapshotDirs(t *testing.T, clone, base string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(clone, base))
	if err != nil {
		t.Fatalf("read %s: %v", base, err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs
}

// assertSnapshotDir checks a record directory name is <YYYYMMDD>T<HHMMSSZ>-<slug>.
func assertSnapshotDir(t *testing.T, name, slug string) {
	t.Helper()
	re := regexp.MustCompile(`^\d{8}T\d{6}Z-` + regexp.QuoteMeta(slug) + `$`)
	if !re.MatchString(name) {
		t.Errorf("snapshot dir %q does not match %s", name, re)
	}
}

func doRequest(t *testing.T, handler http.HandlerFunc, req gitSyncRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/git-sync/v1", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}

func TestGitSync_Success(t *testing.T) {
	repoURL := initTestRepo(t)
	handler := handleGitSync(testCfg(repoURL))

	w := doRequest(t, handler, gitSyncRequest{
		RunbookSlug:   "my-runbook",
		RunbookTitle:  "My Runbook",
		Notes:         "## Step 1\nDo the thing.",
		RunbookSource: "# My Runbook\nSource here.",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp["commit_sha"] == "" {
		t.Error("expected non-empty commit_sha")
	}

	// Verify files exist in the remote by cloning it.
	clone := t.TempDir()
	mustGit(t, clone, "clone", repoURL, ".")

	dirs := snapshotDirs(t, clone, "runs")
	if len(dirs) != 1 {
		t.Fatalf("want 1 snapshot dir, got %v", dirs)
	}
	assertSnapshotDir(t, dirs[0], "my-runbook")
	dir := filepath.Join(clone, "runs", dirs[0])
	for _, name := range []string{"notes.md", "runbook.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s not found: %v", name, err)
		}
	}
}

func TestGitSync_Resync_Changed_CreatesSnapshot(t *testing.T) {
	repoURL := initTestRepo(t)
	handler := handleGitSync(testCfg(repoURL))

	req := gitSyncRequest{
		RunbookSlug:  "resync-test",
		RunbookTitle: "Resync Test",
		Notes:        "First sync",
	}

	w1 := doRequest(t, handler, req)
	if w1.Code != http.StatusOK {
		t.Fatalf("first sync: want 200, got %d: %s", w1.Code, w1.Body.String())
	}

	req.Notes = "Second sync, same day"
	w2 := doRequest(t, handler, req)
	if w2.Code != http.StatusOK {
		t.Fatalf("second sync: want 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var r1, r2 map[string]string
	json.Unmarshal(w1.Body.Bytes(), &r1)
	json.Unmarshal(w2.Body.Bytes(), &r2)

	if r1["commit_sha"] == r2["commit_sha"] {
		t.Error("re-sync should produce a new commit")
	}

	// A changed re-sync is a new immutable snapshot, not an overwrite.
	clone := t.TempDir()
	mustGit(t, clone, "clone", repoURL, ".")
	dirs := snapshotDirs(t, clone, "runs")
	if len(dirs) != 2 {
		t.Fatalf("want 2 snapshot dirs, got %v", dirs)
	}
	var bodies strings.Builder
	for _, d := range dirs {
		assertSnapshotDir(t, d, "resync-test")
		b, err := os.ReadFile(filepath.Join(clone, "runs", d, "notes.md"))
		if err != nil {
			t.Fatalf("read notes in %s: %v", d, err)
		}
		bodies.Write(b)
		bodies.WriteByte('\n')
	}
	for _, want := range []string{"First sync", "Second sync, same day"} {
		if !strings.Contains(bodies.String(), want) {
			t.Errorf("snapshots must preserve %q", want)
		}
	}
}

func TestGitSync_DisabledFeature_Returns403(t *testing.T) {
	cfg := config{GitSyncEnabled: false}
	handler := handleGitSync(cfg)

	w := doRequest(t, handler, gitSyncRequest{RunbookSlug: "test"})
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
}

func TestGitSync_GitFailure_Returns500(t *testing.T) {
	cfg := testCfg("file:///nonexistent/path/that/does/not/exist")
	handler := handleGitSync(cfg)

	w := doRequest(t, handler, gitSyncRequest{
		RunbookSlug: "test",
		Notes:       "some notes",
	})
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] == "" {
		t.Error("expected non-empty error message")
	}
	// Confirm no credential-sensitive content in error (repo URL not exposed)
	if strings.Contains(resp["error"], "nonexistent") {
		t.Error("error response must not contain repository URL details")
	}
}

func TestGitSync_EmptyNotes_Succeeds(t *testing.T) {
	repoURL := initTestRepo(t)
	handler := handleGitSync(testCfg(repoURL))

	w := doRequest(t, handler, gitSyncRequest{
		RunbookSlug:  "empty-notes",
		RunbookTitle: "Empty Notes Test",
		Notes:        "",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGitSync_Images_WrittenWithCorrectExtension(t *testing.T) {
	repoURL := initTestRepo(t)
	handler := handleGitSync(testCfg(repoURL))

	// 1x1 red PNG (minimal valid PNG)
	pngData := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwADhQGAWjR9awAAAABJRU5ErkJggg=="

	w := doRequest(t, handler, gitSyncRequest{
		RunbookSlug:  "img-test",
		RunbookTitle: "Image Test",
		Notes:        "![shot][img-1]",
		Images: []syncImage{
			{Name: "img-1", MimeType: "image/png", DataB64: pngData},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}

	clone := t.TempDir()
	mustGit(t, clone, "clone", repoURL, ".")

	dirs := snapshotDirs(t, clone, "runs")
	if len(dirs) != 1 {
		t.Fatalf("want 1 snapshot dir, got %v", dirs)
	}
	dir := filepath.Join(clone, "runs", dirs[0])
	imgPath := filepath.Join(dir, "img-1.png")
	if _, err := os.Stat(imgPath); err != nil {
		t.Errorf("img-1.png not found: %v", err)
	}

	// Verify image data round-trips correctly
	written, err := os.ReadFile(imgPath)
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := base64.StdEncoding.DecodeString(pngData)
	if !bytes.Equal(written, expected) {
		t.Error("image bytes do not match")
	}

	// Verify notes.md has local filename reference, not bracket token
	notesBytes, _ := os.ReadFile(filepath.Join(dir, "notes.md"))
	notes := string(notesBytes)
	if strings.Contains(notes, "[img-1]") {
		t.Error("notes.md should rewrite [img-1] token to local filename")
	}
	if !strings.Contains(notes, "(img-1.png)") {
		t.Errorf("notes.md should contain local filename reference, got: %s", notes)
	}
}

func TestGitSync_WrongMethod_Returns405(t *testing.T) {
	handler := handleGitSync(testCfg("file:///dummy"))
	r := httptest.NewRequest(http.MethodGet, "/api/git-sync/v1", nil)
	w := httptest.NewRecorder()
	handler(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", w.Code)
	}
}

func doRequestBearer(t *testing.T, handler http.HandlerFunc, req gitSyncRequest, token string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/git-sync/v1", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}

func TestGitSync_BearerAuth(t *testing.T) {
	repoURL := initTestRepo(t)
	cfg := testCfg(repoURL)
	cfg.GitSyncAPIToken = "s3cret"
	handler := handleGitSync(cfg)
	req := gitSyncRequest{RunbookSlug: "auth", RunbookTitle: "Auth", Notes: "hi"}

	if w := doRequestBearer(t, handler, req, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("missing bearer: want 401, got %d", w.Code)
	}
	if w := doRequestBearer(t, handler, req, "wrong"); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong bearer: want 401, got %d", w.Code)
	}
	if w := doRequestBearer(t, handler, req, "s3cret"); w.Code != http.StatusOK {
		t.Errorf("correct bearer: want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGitSync_NoChange_UpToDate(t *testing.T) {
	repoURL := initTestRepo(t)
	handler := handleGitSync(testCfg(repoURL))
	req := gitSyncRequest{
		RunbookSlug:   "nochange",
		RunbookTitle:  "No Change",
		Notes:         "same notes",
		RunbookSource: "same source",
	}

	w1 := doRequest(t, handler, req)
	if w1.Code != http.StatusOK {
		t.Fatalf("first sync: want 200, got %d: %s", w1.Code, w1.Body.String())
	}
	var r1 map[string]string
	json.Unmarshal(w1.Body.Bytes(), &r1)
	if r1["commit_sha"] == "" {
		t.Fatal("first sync should create a commit")
	}

	w2 := doRequest(t, handler, req)
	if w2.Code != http.StatusOK {
		t.Fatalf("second sync: want 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var r2 map[string]string
	json.Unmarshal(w2.Body.Bytes(), &r2)
	if r2["status"] != "up_to_date" {
		t.Errorf("want up_to_date, got %v", r2)
	}
	if r2["commit_sha"] != "" {
		t.Errorf("no-op sync must not create a commit, got %s", r2["commit_sha"])
	}

	// The no-op must not mint a second snapshot directory either.
	clone := t.TempDir()
	mustGit(t, clone, "clone", repoURL, ".")
	if dirs := snapshotDirs(t, clone, "runs"); len(dirs) != 1 {
		t.Errorf("no-op sync must not add a snapshot, got %v", dirs)
	}
}

func TestLoadConfig_GitSyncEnabled(t *testing.T) {
	clear := func(t *testing.T) {
		for _, k := range []string{
			"GITSYNC_REPO", "GITSYNC_TOKEN", "GITSYNC_SSH_KEY",
			"GITSYNC_API_TOKEN", "GITSYNC_TRUST_PROXY_AUTH",
			"IDENTITY_DB_DRIVER", "IDENTITY_TRUST_PROXY_AUTH",
		} {
			t.Setenv(k, "")
		}
	}

	cases := []struct {
		name      string
		repo      string
		token     string
		ssh       string
		api       string
		driver    string
		proxyAuth bool
		want      bool
	}{
		{"nothing", "", "", "", "", "", false, false},
		{"repo only", "https://h/r.git", "", "", "", "", false, false},
		{"repo+token, no endpoint auth", "https://h/r.git", "t", "", "", "", false, false},
		{"repo+ssh, no endpoint auth", "git@h:r.git", "", "/k", "", "", false, false},
		{"ssh remote, agent, endpoint auth", "git@h:r.git", "", "", "a", "", false, true},
		{"ssh:// remote, agent, endpoint auth", "ssh://git@h/r.git", "", "", "a", "", false, true},
		{"https remote, no credential", "https://h/r.git", "", "", "a", "", false, false},
		{"https remote with user, no credential", "https://user@h/r.git", "", "", "a", "", false, false},
		{"repo+token+api token", "https://h/r.git", "t", "", "a", "", false, true},
		{"repo+token+identity proxy auth", "https://h/r.git", "t", "", "", "sqlite", true, true},
		{"proxy auth without identity", "https://h/r.git", "t", "", "", "", true, false},
		{"identity on alone authorises the session", "https://h/r.git", "t", "", "", "sqlite", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clear(t)
			t.Setenv("GITSYNC_REPO", tc.repo)
			t.Setenv("GITSYNC_TOKEN", tc.token)
			t.Setenv("GITSYNC_SSH_KEY", tc.ssh)
			t.Setenv("GITSYNC_API_TOKEN", tc.api)
			t.Setenv("IDENTITY_DB_DRIVER", tc.driver)
			if tc.proxyAuth {
				t.Setenv("IDENTITY_TRUST_PROXY_AUTH", "true")
			}
			if got := loadConfig().GitSyncEnabled; got != tc.want {
				t.Errorf("GitSyncEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

// mustGitOutput runs git and returns its trimmed stdout.
func mustGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}

// doRequestAs submits a sync carrying an authenticated user, the shape
// gateGitSync produces for a session (or proxy) request.
func doRequestAs(t *testing.T, handler http.HandlerFunc, req gitSyncRequest, u stores.User) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/git-sync/v1", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(r.Context(), sessionAuthKey{}, true)
	ctx = context.WithValue(ctx, userKey{}, u)
	w := httptest.NewRecorder()
	handler(w, r.WithContext(ctx))
	return w
}

func TestGitSync_PerUserAuthor(t *testing.T) {
	repoURL := initTestRepo(t)
	cfg := testCfg(repoURL)
	cfg.IdentityEnabled = true
	handler := handleGitSync(cfg)

	u := stores.User{ID: "u1", DisplayName: "Ada Lovelace", Email: "ada@example.com"}
	w := doRequestAs(t, handler, gitSyncRequest{RunbookSlug: "author", RunbookTitle: "Author", Notes: "hi"}, u)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}

	clone := t.TempDir()
	mustGit(t, clone, "clone", repoURL, ".")
	if got, want := mustGitOutput(t, clone, "log", "-1", "--no-show-signature", "--format=%an <%ae>"), "Ada Lovelace <ada@example.com>"; got != want {
		t.Errorf("commit author = %q, want %q", got, want)
	}
}

func TestGitSync_AuthorFallsBackWithoutEmail(t *testing.T) {
	repoURL := initTestRepo(t)
	handler := handleGitSync(testCfg(repoURL))

	// A user without an email cannot author a commit: the machine identity does.
	u := stores.User{ID: "u2", DisplayName: "No Email"}
	w := doRequestAs(t, handler, gitSyncRequest{RunbookSlug: "fallback", RunbookTitle: "Fallback", Notes: "hi"}, u)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}

	clone := t.TempDir()
	mustGit(t, clone, "clone", repoURL, ".")
	if got, want := mustGitOutput(t, clone, "log", "-1", "--no-show-signature", "--format=%an <%ae>"), "Test Bot <bot@test.com>"; got != want {
		t.Errorf("commit author = %q, want the machine fallback %q", got, want)
	}
}

func TestGitSync_UnauthenticatedIdentityOn_401(t *testing.T) {
	// Identity on and no automation token: there is no anonymous way in.
	cfg := testCfg(initTestRepo(t))
	cfg.IdentityEnabled = true
	handler := handleGitSync(cfg)

	w := doRequest(t, handler, gitSyncRequest{RunbookSlug: "nobody", Notes: "x"})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGitSyncNeedsBrowserToken(t *testing.T) {
	cases := []struct {
		name            string
		identityEnabled bool
		apiToken        string
		want            bool
	}{
		{"identity on, token set: session authorises", true, "secret", false},
		{"identity off, token set: prompt", false, "secret", true},
		{"identity off, no token: sync disabled", false, "", false},
		{"identity on, no token: session authorises", true, "", false},
	}
	for _, tc := range cases {
		if got := gitSyncNeedsBrowserToken(tc.identityEnabled, tc.apiToken); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
