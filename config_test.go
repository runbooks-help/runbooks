// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"path/filepath"
	"testing"
)

func TestLoadConfigExpandsSSHKeyHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITSYNC_SSH_KEY", "~/.ssh/id_ed25519")

	want := filepath.Join(home, ".ssh/id_ed25519")
	if got := loadConfig().GitSyncSSHKey; got != want {
		t.Errorf("GitSyncSSHKey = %q, want %q", got, want)
	}
}

// TestLoadConfigContentGit pins that the git content source is configured by
// CONTENT_GIT_*, not by GITSYNC_*.
func TestLoadConfigContentGit(t *testing.T) {
	t.Setenv("CONTENT_SOURCE", "git")
	t.Setenv("CONTENT_GIT_REPO", "https://example.com/docs.git")
	t.Setenv("CONTENT_GIT_BRANCH", "release")
	t.Setenv("CONTENT_GIT_TOKEN", "tok")
	t.Setenv("CONTENT_GIT_PATH", "docs")
	t.Setenv("GITSYNC_REPO", "git@example.com:notes.git")

	cfg := loadConfig()
	if cfg.ContentGitRepo != "https://example.com/docs.git" || cfg.ContentGitBranch != "release" || cfg.ContentGitToken != "tok" {
		t.Fatalf("content git = %q/%q/token=%q", cfg.ContentGitRepo, cfg.ContentGitBranch, cfg.ContentGitToken)
	}
	opts := (&contentState{cfg: cfg}).sourceOptions()
	if opts.Repo != cfg.ContentGitRepo || opts.Branch != "release" || opts.Creds.Token != "tok" || opts.Path != "docs" {
		t.Fatalf("sourceOptions = %+v, want the CONTENT_GIT_* values", opts)
	}
}

// TestLoadConfigContentGitDefaults pins the branch and username fallbacks.
func TestLoadConfigContentGitDefaults(t *testing.T) {
	t.Setenv("CONTENT_SOURCE", "git")
	t.Setenv("CONTENT_GIT_REPO", "https://example.com/docs.git")

	cfg := loadConfig()
	if cfg.ContentGitBranch != "main" || cfg.ContentGitUsername != "oauth2" {
		t.Errorf("defaults = %q/%q, want main/oauth2", cfg.ContentGitBranch, cfg.ContentGitUsername)
	}
}

// TestContentSyncConflict pins the guard that keeps the content repo out of the
// notes sync write path.
func TestContentSyncConflict(t *testing.T) {
	cases := []struct {
		source, sync, content string
		want                  bool
	}{
		{"git", "https://x/notes.git", "https://x/docs.git", false},
		{"git", "https://x/notes.git", "https://x/notes", true},
		{"git", "https://x/notes", "https://x/notes.git", true},
		{"local", "https://x/notes.git", "https://x/notes.git", false},
		{"git", "https://x/notes.git", "", false},
		{"git", "", "https://x/docs.git", false},
	}
	for _, tc := range cases {
		got := contentSyncConflict(config{ContentSource: tc.source, GitSyncRepo: tc.sync, ContentGitRepo: tc.content})
		if got != tc.want {
			t.Errorf("contentSyncConflict(%q, %q, %q) = %v, want %v", tc.source, tc.sync, tc.content, got, tc.want)
		}
	}
}

func TestSSHRemote(t *testing.T) {
	cases := []struct {
		repo string
		want bool
	}{
		{"git@github.com:org/repo.git", true},
		{"ssh://git@host/org/repo.git", true},
		{"https://host/org/repo.git", false},
		{"https://user@host/org/repo.git", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := sshRemote(tc.repo); got != tc.want {
			t.Errorf("sshRemote(%q) = %v, want %v", tc.repo, got, tc.want)
		}
	}
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cases := []struct{ in, want string }{
		{"", ""},
		{"/absolute/key", "/absolute/key"},
		{"./relative/key", "./relative/key"},
		{"~user/key", "~user/key"},
		{"~", home},
		{"~/key", filepath.Join(home, "key")},
		{"~/.ssh/id_ed25519", filepath.Join(home, ".ssh/id_ed25519")},
	}
	for _, tc := range cases {
		if got := expandHome(tc.in); got != tc.want {
			t.Errorf("expandHome(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
