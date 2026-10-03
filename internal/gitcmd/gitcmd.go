// Package gitcmd runs git for the notes-sync write path and the git content
// source. It owns credential handling so secrets never reach argv or a remote
// URL.
package gitcmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Credentials are the remote auth options shared by both callers.
type Credentials struct {
	Username string
	Token    string
	SSHKey   string
}

// AuthEnv builds the per-command environment for git and returns a cleanup for
// any temporary files. An HTTPS token goes through GIT_ASKPASS so the secret
// never appears in argv or the remote URL; SSH uses a key file.
func AuthEnv(c Credentials) ([]string, func(), error) {
	env := []string{"GIT_TERMINAL_PROMPT=0"}
	cleanup := func() {}

	if c.SSHKey != "" {
		env = append(env, "GIT_SSH_COMMAND=ssh -i "+c.SSHKey+
			" -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new")
	}

	if c.Token != "" {
		f, err := os.CreateTemp("", "runbooks-askpass-*")
		if err != nil {
			return nil, nil, fmt.Errorf("auth setup failed")
		}
		script := "#!/bin/sh\ncase \"$1\" in\n" +
			"  *sername*) printf '%s\\n' \"$GIT_ASKPASS_USER\" ;;\n" +
			"  *) printf '%s\\n' \"$GIT_ASKPASS_TOKEN\" ;;\n" +
			"esac\n"
		if _, err := f.WriteString(script); err != nil {
			f.Close()
			os.Remove(f.Name())
			return nil, nil, fmt.Errorf("auth setup failed")
		}
		if err := f.Chmod(0o700); err != nil {
			f.Close()
			os.Remove(f.Name())
			return nil, nil, fmt.Errorf("auth setup failed")
		}
		f.Close()
		env = append(env,
			"GIT_ASKPASS="+f.Name(),
			"GIT_ASKPASS_USER="+c.Username,
			"GIT_ASKPASS_TOKEN="+c.Token,
		)
		cleanup = func() { os.Remove(f.Name()) }
	}

	return env, cleanup, nil
}

// Run runs git in dir.
func Run(ctx context.Context, dir string, env []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	return cmd.Run()
}

// Output runs git in dir and returns its trimmed stdout.
func Output(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
