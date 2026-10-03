// Package contentsource resolves the directory the runbooks are read from: a
// local directory, or a git repository cloned into a cache.
package contentsource

import (
	"context"
	"fmt"
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

// Open returns the directory to read runbooks from and whether it was served
// from an existing git cache. It does not fetch: a local source is the
// configured directory; a git source reuses the cache when it already holds a
// checkout (reused) and otherwise clones synchronously. A caller that gets
// reused serves the cache immediately and refreshes in the background.
func Open(ctx context.Context, opts Options) (string, bool, error) {
	if err := opts.applyDefaults(); err != nil {
		return "", false, err
	}
	if opts.Source == "local" {
		return opts.Dir, false, nil
	}
	exists, err := gitrepo.Exists(opts.Cache)
	if err != nil {
		return "", false, err
	}
	if exists {
		return filepath.Join(opts.Cache, opts.Path), true, nil
	}
	if _, err := gitrepo.Ensure(ctx, opts.Cache, opts.Repo, opts.Branch, opts.Creds, nil); err != nil {
		return "", false, err
	}
	return filepath.Join(opts.Cache, opts.Path), false, nil
}

// Refresh fetches the source and resets the checkout to the remote branch,
// returning the directory to read. A local source is re-read in place. A failed
// git fetch keeps the existing checkout (last-good) and returns the error, so
// the caller keeps its snapshot.
func Refresh(ctx context.Context, opts Options) (string, error) {
	if err := opts.applyDefaults(); err != nil {
		return "", err
	}
	if opts.Source == "local" {
		return opts.Dir, nil
	}
	var fetchErr error
	_, err := gitrepo.Ensure(ctx, opts.Cache, opts.Repo, opts.Branch, opts.Creds, func(err error) {
		fetchErr = err
	})
	if err != nil {
		return "", err
	}
	if fetchErr != nil {
		return "", fmt.Errorf("content: git fetch failed, using the last-good checkout: %w", fetchErr)
	}
	return filepath.Join(opts.Cache, opts.Path), nil
}

// applyDefaults validates the source and fills in the git defaults.
func (opts *Options) applyDefaults() error {
	if opts.Source == "" {
		opts.Source = "local"
	}
	if opts.Source == "local" {
		if opts.Dir == "" {
			return fmt.Errorf("CONTENT_DIR is empty")
		}
		return nil
	}
	if opts.Source != "git" {
		return fmt.Errorf("unknown CONTENT_SOURCE %q (want local or git)", opts.Source)
	}
	if opts.Repo == "" {
		return fmt.Errorf("CONTENT_SOURCE=git requires GITSYNC_REPO")
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
	return nil
}
