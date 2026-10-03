// Package content resolves the directory the runbooks are read from: a local
// directory, or a git repository cloned into a cache.
package content

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"runbooks/internal/gitcmd"
)

// Options selects and configures the content source. Source is "local" (the
// default) or "git": local reads Dir, git reads Path inside a clone of Repo in
// Cache.
type Options struct {
	Source string
	Dir    string
	Repo   string
	Branch string
	Creds  gitcmd.Credentials
	Path   string
	Cache  string
}

// Resolve returns the directory to read runbooks from. A git source clones the
// repo into Cache when absent and fetches when present; a failed fetch falls
// back to the existing checkout (last-good) rather than failing to start.
func Resolve(ctx context.Context, opts Options) (string, error) {
	switch opts.Source {
	case "", "local":
		if opts.Dir == "" {
			return "", fmt.Errorf("CONTENT_DIR is empty")
		}
		return opts.Dir, nil
	case "git":
		return resolveGit(ctx, opts)
	default:
		return "", fmt.Errorf("unknown CONTENT_SOURCE %q (want local or git)", opts.Source)
	}
}

func resolveGit(ctx context.Context, opts Options) (string, error) {
	if opts.Repo == "" {
		return "", fmt.Errorf("CONTENT_SOURCE=git requires GITSYNC_REPO")
	}
	if opts.Branch == "" {
		opts.Branch = "main"
	}
	if opts.Cache == "" {
		opts.Cache = filepath.Join("data", "content")
	}
	if opts.Path == "" {
		opts.Path = "."
	}

	env, cleanup, err := gitcmd.AuthEnv(opts.Creds)
	if err != nil {
		return "", err
	}
	defer cleanup()

	if _, err := os.Stat(filepath.Join(opts.Cache, ".git")); err == nil {
		// Existing checkout: point origin at the configured repo (it may have
		// changed), then refresh in place, keeping the checkout if the remote is
		// unreachable.
		if err := gitcmd.Run(ctx, opts.Cache, env, "remote", "set-url", "origin", opts.Repo); err != nil {
			return "", fmt.Errorf("git remote set-url failed: %w", err)
		}
		if err := gitcmd.Run(ctx, opts.Cache, env, "fetch", "origin", opts.Branch); err != nil {
			log.Printf("content: git fetch failed, using the last-good checkout: %v", err)
		} else if err := gitcmd.Run(ctx, opts.Cache, env, "reset", "--hard", "origin/"+opts.Branch); err != nil {
			return "", fmt.Errorf("git reset failed: %w", err)
		}
	} else {
		parent := filepath.Dir(opts.Cache)
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return "", fmt.Errorf("content cache: %w", err)
		}
		if err := gitcmd.Run(ctx, parent, env,
			"clone", "--branch", opts.Branch, opts.Repo, filepath.Base(opts.Cache)); err != nil {
			return "", fmt.Errorf("git clone failed for %s", opts.Repo)
		}
	}

	return filepath.Join(opts.Cache, opts.Path), nil
}
