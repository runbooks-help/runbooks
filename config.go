// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// config is the full runtime configuration, read once from the environment at
// startup. See docs/configuration.md for the operator-facing reference.
type config struct {
	GitSyncRepo        string
	GitSyncBranch      string
	GitSyncBasePath    string
	GitSyncAuthorName  string
	GitSyncAuthorEmail string
	GitSyncUsername    string
	GitSyncToken       string
	GitSyncSSHKey      string
	GitSyncAPIToken    string
	GitSyncEnabled     bool

	IdentityEnabled         bool
	IdentityDriver          string
	IdentityDSN             string
	IdentityPublicURL       string
	IdentityBootstrapToken  string
	IdentityRecoveryToken   string
	IdentityTrustProxyAuth  bool
	IdentityProxyUserHeader string
	IdentityProxyNameHeader string
	IdentitySecureCookies   bool
	IdentitySessionTTL      time.Duration
	IdentitySessionIdleTTL  time.Duration

	ContentDir             string
	ContentSource          string
	ContentGitPath         string
	ContentGitCache        string
	ContentRefreshToken    string
	ContentRefreshInterval time.Duration
	StyleGuideEnabled      bool

	// PublicURL is the site's absolute base (no trailing slash); it enables the
	// canonical/OpenGraph URLs and the sitemap. SiteDescription is the default
	// meta description for pages without their own.
	PublicURL       string
	SiteDescription string
}

func loadConfig() config {
	cfg := config{
		ContentDir:          os.Getenv("CONTENT_DIR"),
		ContentSource:       os.Getenv("CONTENT_SOURCE"),
		ContentGitPath:      os.Getenv("CONTENT_GIT_PATH"),
		ContentGitCache:     os.Getenv("CONTENT_GIT_CACHE"),
		ContentRefreshToken: os.Getenv("CONTENT_REFRESH_TOKEN"),
		GitSyncRepo:         os.Getenv("GITSYNC_REPO"),
		GitSyncBranch:       os.Getenv("GITSYNC_BRANCH"),
		GitSyncBasePath:     os.Getenv("GITSYNC_BASE_PATH"),
		GitSyncAuthorName:   os.Getenv("GITSYNC_AUTHOR_NAME"),
		GitSyncAuthorEmail:  os.Getenv("GITSYNC_AUTHOR_EMAIL"),
		GitSyncUsername:     os.Getenv("GITSYNC_USERNAME"),
		GitSyncToken:        os.Getenv("GITSYNC_TOKEN"),
		GitSyncSSHKey:       expandHome(os.Getenv("GITSYNC_SSH_KEY")),
		GitSyncAPIToken:     os.Getenv("GITSYNC_API_TOKEN"),
	}
	if cfg.ContentDir == "" {
		cfg.ContentDir = "content"
	}
	if cfg.ContentSource == "" {
		cfg.ContentSource = "local"
	}
	if cfg.ContentGitPath == "" {
		cfg.ContentGitPath = "."
	}
	if cfg.ContentGitCache == "" {
		cfg.ContentGitCache = "data/content"
	}
	cfg.PublicURL = strings.TrimRight(os.Getenv("PUBLIC_URL"), "/")
	cfg.SiteDescription = os.Getenv("SITE_DESCRIPTION")
	// Automatic refresh is opt-in; a sub-minute interval is a footgun against the
	// remote, so a positive value below the floor is a startup error.
	cfg.ContentRefreshInterval = envDuration("CONTENT_REFRESH_INTERVAL", 0)
	if cfg.ContentRefreshInterval > 0 && cfg.ContentRefreshInterval < time.Minute {
		log.Fatalf("CONTENT_REFRESH_INTERVAL must be at least 1m, got %s", cfg.ContentRefreshInterval)
	}
	if cfg.GitSyncBranch == "" {
		cfg.GitSyncBranch = "main"
	}
	if cfg.GitSyncBasePath == "" {
		cfg.GitSyncBasePath = "runbook_runs"
	}
	if cfg.GitSyncUsername == "" {
		cfg.GitSyncUsername = "oauth2"
	}
	// Identity is off unless a database driver is configured.
	cfg.IdentityDriver = strings.TrimSpace(os.Getenv("IDENTITY_DB_DRIVER"))
	cfg.IdentityDSN = strings.TrimSpace(os.Getenv("IDENTITY_DB_DSN"))
	cfg.IdentityPublicURL = strings.TrimRight(strings.TrimSpace(os.Getenv("IDENTITY_PUBLIC_URL")), "/")
	cfg.IdentityBootstrapToken = os.Getenv("IDENTITY_BOOTSTRAP_TOKEN")
	cfg.IdentityRecoveryToken = os.Getenv("IDENTITY_RECOVERY_TOKEN")
	cfg.IdentityTrustProxyAuth = envBool("IDENTITY_TRUST_PROXY_AUTH", false)
	// A custom identity header keeps no deprecated X- prefix (RFC 6648). The
	// operator points this at whatever their proxy emits — oauth2-proxy uses
	// X-Auth-Request-Email, Authelia Remote-Email.
	cfg.IdentityProxyUserHeader = envOr("IDENTITY_PROXY_USER_HEADER", "Auth-Request-Email")
	cfg.IdentityProxyNameHeader = os.Getenv("IDENTITY_PROXY_NAME_HEADER")
	cfg.IdentitySecureCookies = envBool("IDENTITY_SECURE_COOKIES", true)
	cfg.IdentitySessionTTL = envDuration("IDENTITY_SESSION_TTL", 720*time.Hour)
	cfg.IdentitySessionIdleTTL = envDuration("IDENTITY_SESSION_IDLE", 168*time.Hour)
	if cfg.IdentityDriver == "sqlite" && cfg.IdentityDSN == "" {
		cfg.IdentityDSN = "file:./data/runbooks.db"
	}
	cfg.IdentityEnabled = cfg.IdentityDriver != ""

	if cfg.IdentityTrustProxyAuth {
		log.Printf("IDENTITY_TRUST_PROXY_AUTH is on: the instance must be reachable only through the trusted proxy that sets and strips %s, or the header is an impersonation hole", cfg.IdentityProxyUserHeader)
	}

	// A repo and a credential are required to do anything, and the write route
	// must never be reachable unauthenticated: with identity on a user session (or
	// proxy assertion) authorises the endpoint and an unauthenticated request is
	// refused; with identity off, the shared API token is the gate. Without one of
	// those, sync stays off. An SSH remote counts as a credential: go-git falls
	// back to the ambient SSH agent (ssh-agent, 1Password, …) when no key or token
	// is given, so a key is only needed where there is no agent (deployment/CI).
	hasCredential := cfg.GitSyncToken != "" || cfg.GitSyncSSHKey != "" || sshRemote(cfg.GitSyncRepo)
	hasEndpointAuth := cfg.GitSyncAPIToken != "" || cfg.IdentityEnabled
	cfg.GitSyncEnabled = cfg.GitSyncRepo != "" && hasCredential && hasEndpointAuth

	// The styleguide is a dev/self-host surface, off in the default container.
	cfg.StyleGuideEnabled = envBool("STYLEGUIDE_ENABLED", false)

	return cfg
}

func envBool(name string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "":
		return def
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

func envDuration(name string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Fatalf("%s: %v", name, err)
	}
	return d
}

func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// sshRemote reports whether a remote URL uses SSH, where the ambient SSH agent
// is the credential and an explicit key is optional. It accepts ssh:// URLs and
// scp-style remotes (user@host:path), but not an https:// URL that happens to
// carry a user.
func sshRemote(repo string) bool {
	if strings.HasPrefix(repo, "ssh://") {
		return true
	}
	return !strings.Contains(repo, "://") && strings.Contains(repo, "@")
}

// expandHome expands a leading ~ to the user's home directory, since neither
// mise nor go-git does it for us: GITSYNC_SSH_KEY=~/.ssh/id_ed25519 must resolve
// before the file is opened. A bare ~ becomes the home directory; ~user is left
// alone.
func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}
