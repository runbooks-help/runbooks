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
