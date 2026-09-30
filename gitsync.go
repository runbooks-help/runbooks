package main

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type gitSyncRequest struct {
	RunbookSlug   string      `json:"runbook_slug"`
	RunbookTitle  string      `json:"runbook_title"`
	Notes         string      `json:"notes"`
	RunbookSource string      `json:"runbook_source"`
	Images        []syncImage `json:"images"`
}

type syncImage struct {
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	DataB64  string `json:"data_base64"`
}

var (
	gitSyncMu sync.Mutex
	imgRefRe  = regexp.MustCompile(`!\[([^\]]*)\]\[img-(\d+)\]`)
	mimeToExt = map[string]string{
		"jpeg": "jpg",
		"jpg":  "jpg",
		"png":  "png",
		"gif":  "gif",
		"webp": "webp",
	}
)

const bearerPrefix = "Bearer "

func handleGitSync(cfg config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if !cfg.GitSyncEnabled {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "git sync not configured"})
			return
		}

		// A user session satisfies the endpoint; otherwise the shared instance
		// token does. When the token is empty the operator has declared the route
		// sits behind upstream (proxy/SSO) auth.
		if !sessionAuthorized(r.Context()) && cfg.GitSyncAPIToken != "" {
			auth := r.Header.Get("Authorization")
			token := strings.TrimPrefix(auth, bearerPrefix)
			if !strings.HasPrefix(auth, bearerPrefix) ||
				subtle.ConstantTimeCompare([]byte(token), []byte(cfg.GitSyncAPIToken)) != 1 {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
		}

		var req gitSyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		sha, changed, err := doGitSync(ctx, cfg, req)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if !changed {
			json.NewEncoder(w).Encode(map[string]string{"status": "up_to_date"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"commit_sha": sha})
	}
}

func doGitSync(ctx context.Context, cfg config, req gitSyncRequest) (string, bool, error) {
	gitSyncMu.Lock()
	defer gitSyncMu.Unlock()

	ws, err := os.MkdirTemp("", "runbooks-gitsync-*")
	if err != nil {
		return "", false, fmt.Errorf("workspace creation failed")
	}
	defer os.RemoveAll(ws)

	env, cleanup, err := gitAuthEnv(cfg)
	if err != nil {
		return "", false, err
	}
	defer cleanup()

	if err := gitRun(ctx, ws, env, "clone", cfg.GitSyncRepo, "."); err != nil {
		return "", false, fmt.Errorf("clone failed")
	}

	if cfg.GitSyncAuthorName != "" {
		gitRun(ctx, ws, env, "config", "user.name", cfg.GitSyncAuthorName)
	}
	if cfg.GitSyncAuthorEmail != "" {
		gitRun(ctx, ws, env, "config", "user.email", cfg.GitSyncAuthorEmail)
	}

	if err := gitRun(ctx, ws, env, "checkout", cfg.GitSyncBranch); err != nil {
		if err := gitRun(ctx, ws, env, "checkout", "-b", cfg.GitSyncBranch); err != nil {
			return "", false, fmt.Errorf("branch setup failed")
		}
	}

	date := time.Now().UTC().Format("2006-01-02")
	dir := filepath.Join(ws, cfg.GitSyncBasePath, date, req.RunbookSlug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, fmt.Errorf("mkdir failed")
	}

	imgNames := make(map[string]string, len(req.Images))
	for _, img := range req.Images {
		subtype := strings.TrimPrefix(img.MimeType, "image/")
		ext, ok := mimeToExt[subtype]
		if !ok {
			ext = subtype
		}
		fname := img.Name + "." + ext
		imgNames[img.Name] = fname

		data, err := base64.StdEncoding.DecodeString(img.DataB64)
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, fname), data, 0o644); err != nil {
			return "", false, fmt.Errorf("write image failed")
		}
	}

	notes := imgRefRe.ReplaceAllStringFunc(req.Notes, func(match string) string {
		sub := imgRefRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		alt, name := sub[1], "img-"+sub[2]
		if fname, ok := imgNames[name]; ok {
			return fmt.Sprintf("![%s](%s)", alt, fname)
		}
		return match
	})

	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte(notes), 0o644); err != nil {
		return "", false, fmt.Errorf("write notes failed")
	}
	if err := os.WriteFile(filepath.Join(dir, "runbook.md"), []byte(req.RunbookSource), 0o644); err != nil {
		return "", false, fmt.Errorf("write runbook failed")
	}

	if err := gitRun(ctx, ws, env, "add", "."); err != nil {
		return "", false, fmt.Errorf("git add failed")
	}

	// Nothing changed since the last sync — report up-to-date rather than
	// creating an empty commit.
	status, err := gitOutput(ctx, ws, env, "status", "--porcelain")
	if err != nil {
		return "", false, fmt.Errorf("git status failed")
	}
	if strings.TrimSpace(status) == "" {
		return "", false, nil
	}

	msg := fmt.Sprintf("sync: %s %s", req.RunbookTitle, date)
	if err := gitRun(ctx, ws, env, "commit", "-m", msg); err != nil {
		return "", false, fmt.Errorf("git commit failed")
	}

	if err := gitRun(ctx, ws, env, "push", "origin", cfg.GitSyncBranch); err != nil {
		return "", false, fmt.Errorf("git push failed")
	}

	sha, err := gitOutput(ctx, ws, env, "rev-parse", "HEAD")
	if err != nil {
		return "", false, fmt.Errorf("rev-parse failed")
	}
	return sha, true, nil
}

// gitAuthEnv builds the per-command environment for git and returns a cleanup
// for any temporary files. HTTPS tokens go through GIT_ASKPASS so the secret
// never appears in argv or the remote URL; SSH uses a key file.
func gitAuthEnv(cfg config) ([]string, func(), error) {
	env := []string{"GIT_TERMINAL_PROMPT=0"}
	cleanup := func() {}

	if cfg.GitSyncSSHKey != "" {
		env = append(env, "GIT_SSH_COMMAND=ssh -i "+cfg.GitSyncSSHKey+
			" -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new")
	}

	if cfg.GitSyncToken != "" {
		f, err := os.CreateTemp("", "runbooks-askpass-*")
		if err != nil {
			return nil, nil, fmt.Errorf("auth setup failed")
		}
		script := "#!/bin/sh\ncase \"$1\" in\n" +
			"  *sername*) printf '%s\\n' \"$GITSYNC_ASKPASS_USER\" ;;\n" +
			"  *) printf '%s\\n' \"$GITSYNC_ASKPASS_TOKEN\" ;;\n" +
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
			"GITSYNC_ASKPASS_USER="+cfg.GitSyncUsername,
			"GITSYNC_ASKPASS_TOKEN="+cfg.GitSyncToken,
		)
		cleanup = func() { os.Remove(f.Name()) }
	}

	return env, cleanup, nil
}

func gitRun(ctx context.Context, dir string, env []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	return cmd.Run()
}

func gitOutput(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
