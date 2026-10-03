// Package contentsource resolves the directory the runbooks are read from: a
// local directory, or a git repository cloned into a cache.
package contentsource

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"runbooks/internal/gitrepo"
)

// Options selects and configures the content source. Source is "local" (the
// default) or "git": local reads Dir, git reads Path inside a clone of Repo in
// Cache.
type Options struct {
	Source string
	Dir    string
	Repo   string
	Branch string
	Creds  gitrepo.Credentials
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
	if err := os.MkdirAll(opts.Cache, 0o755); err != nil {
		return "", fmt.Errorf("content cache: %w", err)
	}

	_, err := gitrepo.Ensure(ctx, opts.Cache, opts.Repo, opts.Branch, opts.Creds, func(err error) {
		log.Printf("content: git fetch failed, using the last-good checkout: %v", err)
	})
	if err != nil {
		return "", err
	}
	return filepath.Join(opts.Cache, opts.Path), nil
}
