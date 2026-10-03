// Package gitrepo wraps go-git for the two places Runbooks touches a remote:
// reading runbook content and writing notes records. It is pure Go — no system
// git binary.
package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// Credentials are the remote auth options shared by content reads and notes
// sync.
type Credentials struct {
	Username string
	Token    string
	SSHKey   string
}

// AuthMethod returns the go-git auth for the credentials, or nil for anonymous
// access. SSH verifies the host against the user's known_hosts.
func (c Credentials) AuthMethod() (transport.AuthMethod, error) {
	if c.SSHKey != "" {
		key, err := gitssh.NewPublicKeysFromFile("git", c.SSHKey, "")
		if err != nil {
			return nil, fmt.Errorf("ssh key: %w", err)
		}
		return key, nil
	}
	if c.Token != "" {
		user := c.Username
		if user == "" {
			user = "oauth2"
		}
		return &githttp.BasicAuth{Username: user, Password: c.Token}, nil
	}
	return nil, nil
}

// Author is the commit identity.
type Author struct {
	Name  string
	Email string
}

// Fresh clones repo into dir and checks out branch (creating it when the remote
// does not have it yet), for the notes-sync write path.
func Fresh(ctx context.Context, dir, repo, branch string, creds Credentials) (*git.Repository, error) {
	auth, err := creds.AuthMethod()
	if err != nil {
		return nil, err
	}
	r, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{URL: repo, Auth: auth})
	// An empty remote reports either of these, depending on whether it has an
	// unborn HEAD; both mean "init and commit to the branch ourselves".
	if errors.Is(err, transport.ErrEmptyRemoteRepository) || errors.Is(err, plumbing.ErrReferenceNotFound) {
		return initEmpty(dir, repo, branch)
	}
	if err != nil {
		return nil, fmt.Errorf("clone %s: %w", repo, err)
	}
	return r, checkoutOrCreate(r, branch)
}

// Ensure updates dir to origin/branch, cloning when dir is not yet a
// repository. A failed fetch keeps an existing checkout (last-good) and reports
// the error through onFetchError, when non-nil.
func Ensure(ctx context.Context, dir, repo, branch string, creds Credentials, onFetchError func(error)) (*git.Repository, error) {
	auth, err := creds.AuthMethod()
	if err != nil {
		return nil, err
	}
	branchRef := plumbing.NewBranchReferenceName(branch)

	r, err := git.PlainOpen(dir)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		r, err = git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
			URL:           repo,
			Auth:          auth,
			ReferenceName: branchRef,
			SingleBranch:  true,
		})
		if err != nil {
			return nil, fmt.Errorf("clone %s: %w", repo, err)
		}
		return r, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dir, err)
	}

	if err := pointOriginAt(r, repo); err != nil {
		return nil, err
	}
	err = r.FetchContext(ctx, &git.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec("+refs/heads/" + branch + ":refs/remotes/origin/" + branch)},
		Auth:       auth,
		Force:      true,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		if onFetchError != nil {
			onFetchError(err)
		}
		return r, nil
	}
	return r, hardResetTo(r, branch)
}

// Commit stages everything in the worktree and commits it if anything changed,
// returning the new hash and whether a commit was made.
func Commit(r *git.Repository, msg string, author Author) (plumbing.Hash, bool, error) {
	wt, err := r.Worktree()
	if err != nil {
		return plumbing.ZeroHash, false, err
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return plumbing.ZeroHash, false, fmt.Errorf("git add: %w", err)
	}
	status, err := wt.Status()
	if err != nil {
		return plumbing.ZeroHash, false, fmt.Errorf("git status: %w", err)
	}
	if status.IsClean() {
		return plumbing.ZeroHash, false, nil
	}
	hash, err := wt.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{Name: author.Name, Email: author.Email, When: time.Now()},
	})
	if err != nil {
		return plumbing.ZeroHash, false, fmt.Errorf("git commit: %w", err)
	}
	return hash, true, nil
}

// Push pushes branch to origin.
func Push(ctx context.Context, r *git.Repository, branch string, creds Credentials) error {
	auth, err := creds.AuthMethod()
	if err != nil {
		return err
	}
	ref := "refs/heads/" + branch
	err = r.PushContext(ctx, &git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec(ref + ":" + ref)},
		Auth:       auth,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("git push: %w", err)
	}
	return nil
}

func checkoutOrCreate(r *git.Repository, branch string) error {
	wt, err := r.Worktree()
	if err != nil {
		return err
	}
	branchRef := plumbing.NewBranchReferenceName(branch)
	if err := wt.Checkout(&git.CheckoutOptions{Branch: branchRef}); err != nil {
		if err := wt.Checkout(&git.CheckoutOptions{Branch: branchRef, Create: true}); err != nil {
			return fmt.Errorf("checkout %s: %w", branch, err)
		}
	}
	return nil
}

func hardResetTo(r *git.Repository, branch string) error {
	remoteRef, err := r.Reference(plumbing.NewRemoteReferenceName("origin", branch), true)
	if err != nil {
		return fmt.Errorf("origin/%s: %w", branch, err)
	}
	wt, err := r.Worktree()
	if err != nil {
		return err
	}
	if err := wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(branch), Force: true}); err != nil {
		return fmt.Errorf("checkout %s: %w", branch, err)
	}
	if err := wt.Reset(&git.ResetOptions{Commit: remoteRef.Hash(), Mode: git.HardReset}); err != nil {
		return fmt.Errorf("reset %s: %w", branch, err)
	}
	return nil
}

func pointOriginAt(r *git.Repository, repo string) error {
	if err := r.DeleteRemote("origin"); err != nil && !errors.Is(err, git.ErrRemoteNotFound) {
		return fmt.Errorf("remove origin: %w", err)
	}
	if _, err := r.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{repo}}); err != nil {
		return fmt.Errorf("set origin: %w", err)
	}
	return nil
}

func initEmpty(dir, repo, branch string) (*git.Repository, error) {
	r, err := git.PlainInit(dir, false)
	if err != nil {
		return nil, fmt.Errorf("init %s: %w", dir, err)
	}
	if _, err := r.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{repo}}); err != nil {
		return nil, fmt.Errorf("set origin: %w", err)
	}
	// An empty repo has an unborn HEAD, so the branch cannot be checked out —
	// point HEAD at it and let the first commit create it.
	if err := r.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(branch))); err != nil {
		return nil, fmt.Errorf("set head: %w", err)
	}
	return r, nil
}
