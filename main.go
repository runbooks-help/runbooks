package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"runbooks/identity"
	"runbooks/parser"
	"runbooks/stores"
	"runbooks/stores/mysql"
	"runbooks/stores/postgres"
	"runbooks/stores/sqlite"
	"runbooks/views"
)

//go:embed public
var static embed.FS

type config struct {
	GitSyncRepo           string
	GitSyncBranch         string
	GitSyncBasePath       string
	GitSyncAuthorName     string
	GitSyncAuthorEmail    string
	GitSyncUsername       string
	GitSyncToken          string
	GitSyncSSHKey         string
	GitSyncAPIToken       string
	GitSyncTrustProxyAuth bool
	GitSyncEnabled        bool

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
}

func loadConfig() config {
	cfg := config{
		GitSyncRepo:        os.Getenv("GITSYNC_REPO"),
		GitSyncBranch:      os.Getenv("GITSYNC_BRANCH"),
		GitSyncBasePath:    os.Getenv("GITSYNC_BASE_PATH"),
		GitSyncAuthorName:  os.Getenv("GITSYNC_AUTHOR_NAME"),
		GitSyncAuthorEmail: os.Getenv("GITSYNC_AUTHOR_EMAIL"),
		GitSyncUsername:    os.Getenv("GITSYNC_USERNAME"),
		GitSyncToken:       os.Getenv("GITSYNC_TOKEN"),
		GitSyncSSHKey:      os.Getenv("GITSYNC_SSH_KEY"),
		GitSyncAPIToken:    os.Getenv("GITSYNC_API_TOKEN"),
	}
	if cfg.GitSyncBranch == "" {
		cfg.GitSyncBranch = "main"
	}
	if cfg.GitSyncBasePath == "" {
		cfg.GitSyncBasePath = "runs"
	}
	if cfg.GitSyncUsername == "" {
		cfg.GitSyncUsername = "oauth2"
	}
	cfg.GitSyncTrustProxyAuth = envBool("GITSYNC_TRUST_PROXY_AUTH", false)

	// A repo and a credential are required to do anything, and the write route
	// must never be reachable unauthenticated: without either a shared token or
	// an explicit assertion that upstream auth exists, sync stays disabled.
	hasCredential := cfg.GitSyncToken != "" || cfg.GitSyncSSHKey != ""
	hasEndpointAuth := cfg.GitSyncAPIToken != "" || cfg.GitSyncTrustProxyAuth
	cfg.GitSyncEnabled = cfg.GitSyncRepo != "" && hasCredential && hasEndpointAuth

	// Identity is off unless a database driver is configured.
	cfg.IdentityDriver = strings.TrimSpace(os.Getenv("IDENTITY_DB_DRIVER"))
	cfg.IdentityDSN = strings.TrimSpace(os.Getenv("IDENTITY_DB_DSN"))
	cfg.IdentityPublicURL = strings.TrimRight(strings.TrimSpace(os.Getenv("IDENTITY_PUBLIC_URL")), "/")
	cfg.IdentityBootstrapToken = os.Getenv("IDENTITY_BOOTSTRAP_TOKEN")
	cfg.IdentityRecoveryToken = os.Getenv("IDENTITY_RECOVERY_TOKEN")
	cfg.IdentityTrustProxyAuth = envBool("IDENTITY_TRUST_PROXY_AUTH", false)
	cfg.IdentityProxyUserHeader = envOr("IDENTITY_PROXY_USER_HEADER", "X-Auth-Request-Email")
	cfg.IdentityProxyNameHeader = os.Getenv("IDENTITY_PROXY_NAME_HEADER")
	cfg.IdentitySecureCookies = envBool("IDENTITY_SECURE_COOKIES", true)
	cfg.IdentitySessionTTL = envDuration("IDENTITY_SESSION_TTL", 720*time.Hour)
	cfg.IdentitySessionIdleTTL = envDuration("IDENTITY_SESSION_IDLE", 168*time.Hour)
	if cfg.IdentityDriver == "sqlite" && cfg.IdentityDSN == "" {
		cfg.IdentityDSN = "file:./data/runbooks.db"
	}
	cfg.IdentityEnabled = cfg.IdentityDriver != ""

	return cfg
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	cfg := loadConfig()
	if cfg.IdentityEnabled && cfg.IdentityPublicURL == "" {
		log.Fatal("IDENTITY_PUBLIC_URL is required when identity is enabled")
	}

	// An empty content/ is not an error: the index renders a welcome that says
	// how to add runbooks, so a fresh checkout still boots.
	runbooks, err := parser.LoadDir("content")
	if err != nil {
		log.Fatalf("load runbooks: %v", err)
	}
	groups := parser.GroupBySystem(runbooks)

	mux := http.NewServeMux()
	mux.Handle("/public/", http.FileServer(http.FS(static)))

	index := func(w http.ResponseWriter, r *http.Request) {
		views.IndexPage(groups).Render(r.Context(), w)
	}

	var authn *auth
	if cfg.IdentityEnabled {
		store, err := openIdentityStore(cfg)
		if err != nil {
			log.Fatalf("identity: %v", err)
		}
		svc, err := identity.New(identity.Config{
			RPID:           rpID(cfg.IdentityPublicURL),
			RPDisplayName:  "Runbooks",
			RPOrigins:      []string{cfg.IdentityPublicURL},
			SessionTTL:     cfg.IdentitySessionTTL,
			SessionIdleTTL: cfg.IdentitySessionIdleTTL,
		}, store)
		if err != nil {
			log.Fatalf("identity: %v", err)
		}
		authn = newAuth(svc, store, cfg)

		mux.HandleFunc("/login", authn.loginPage)
		mux.HandleFunc("/setup", authn.setupPage)
		mux.HandleFunc("/invite/{token}", authn.invitePage)
		mux.HandleFunc("/api/auth/v1/login/begin", authn.loginBegin)
		mux.HandleFunc("/api/auth/v1/login/finish", authn.loginFinish)
		mux.HandleFunc("/api/auth/v1/setup/begin", authn.setupBegin)
		mux.HandleFunc("/api/auth/v1/setup/finish", authn.setupFinish)
		mux.HandleFunc("/api/auth/v1/invite/begin", authn.inviteBegin)
		mux.HandleFunc("/api/auth/v1/invite/finish", authn.inviteFinish)
		mux.HandleFunc("/api/auth/v1/logout", authn.logout)

		index = authn.requirePage(index)
		log.Printf("identity enabled (%s)", cfg.IdentityDriver)
	}

	mux.HandleFunc("/{$}", index)

	for _, rb := range runbooks {
		rb := rb
		page := func(w http.ResponseWriter, r *http.Request) {
			views.RunbookPage(rb, groups, views.PageConfig{
				GitSyncEnabled:       cfg.GitSyncEnabled,
				GitSyncRequiresToken: cfg.GitSyncAPIToken != "",
				RecordsBasePath:      cfg.GitSyncBasePath,
			}).Render(r.Context(), w)
		}
		if authn != nil {
			page = authn.requirePage(page)
		}
		mux.HandleFunc("/"+rb.Slug, page)
	}

	gitSync := http.HandlerFunc(handleGitSync(cfg))
	if authn != nil {
		gitSync = authn.gateGitSync(gitSync)
	}
	mux.Handle("/api/git-sync/v1", gitSync)

	log.Printf("runbooks listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// openIdentityStore opens the configured identity database.
func openIdentityStore(cfg config) (stores.Store, error) {
	ctx := context.Background()
	switch cfg.IdentityDriver {
	case "sqlite":
		return sqlite.Open(ctx, cfg.IdentityDSN)
	case "mysql":
		return mysql.Open(ctx, cfg.IdentityDSN)
	case "postgres":
		return postgres.Open(ctx, cfg.IdentityDSN)
	default:
		return nil, fmt.Errorf("unknown IDENTITY_DB_DRIVER %q", cfg.IdentityDriver)
	}
}

// rpID is the WebAuthn relying-party id: the public URL's host without scheme or
// port.
func rpID(publicURL string) string {
	u, err := url.Parse(publicURL)
	if err != nil || u.Hostname() == "" {
		return publicURL
	}
	return u.Hostname()
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
