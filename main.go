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
	"sync"
	"time"

	"runbooks/contentsource"
	"runbooks/identity"
	"runbooks/internal/gitrepo"
	"runbooks/parser"
	"runbooks/stores"
	"runbooks/stores/mysql"
	"runbooks/stores/postgres"
	"runbooks/stores/sqlite"
	"runbooks/views"
)

//go:embed public
var static embed.FS

// version is the build stamp, set with -X main.version=… at build time.
var version = "dev"

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

	ContentDir        string
	ContentSource     string
	ContentGitPath    string
	ContentGitCache   string
	StyleGuideEnabled bool
}

func loadConfig() config {
	cfg := config{
		ContentDir:         os.Getenv("CONTENT_DIR"),
		ContentSource:      os.Getenv("CONTENT_SOURCE"),
		ContentGitPath:     os.Getenv("CONTENT_GIT_PATH"),
		ContentGitCache:    os.Getenv("CONTENT_GIT_CACHE"),
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
	// those, sync stays off.
	hasCredential := cfg.GitSyncToken != "" || cfg.GitSyncSSHKey != ""
	hasEndpointAuth := cfg.GitSyncAPIToken != "" || cfg.IdentityEnabled
	cfg.GitSyncEnabled = cfg.GitSyncRepo != "" && hasCredential && hasEndpointAuth

	// The styleguide is a dev/self-host surface, off in the default container.
	cfg.StyleGuideEnabled = envBool("STYLEGUIDE_ENABLED", false)

	return cfg
}

// gitSyncNeedsBrowserToken reports whether the Sync UI must prompt for the shared
// bearer token. With identity on, a signed-in session (or proxy assertion)
// authorises and attributes the sync — and the page is already behind the read
// gate — so the token is only the identity-off deployment's gate. It stays
// configured server-side either way, as the CI/automation fallback.
func gitSyncNeedsBrowserToken(identityEnabled bool, apiToken string) bool {
	return !identityEnabled && apiToken != ""
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println("runbooks " + version)
		return
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	cfg := loadConfig()
	if cfg.IdentityEnabled && cfg.IdentityPublicURL == "" {
		log.Fatal("IDENTITY_PUBLIC_URL is required when identity is enabled")
	}

	var store stores.Store
	var authn *auth
	if cfg.IdentityEnabled {
		var err error
		store, err = openIdentityStore(cfg)
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
		log.Printf("identity enabled (%s)", cfg.IdentityDriver)
	}

	state, err := newContentState(cfg, store, authn)
	if err != nil {
		log.Fatalf("content: %v", err)
	}
	if cfg.StyleGuideEnabled {
		log.Printf("styleguide enabled at /styleguide")
	}

	log.Printf("runbooks listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, securityHeaders(state)))
}

// contentSnapshot is the parsed content and the indexes derived from it. It is
// immutable: a refresh builds a new one and swaps it in.
type contentSnapshot struct {
	defs   []parser.RunbookDef
	groups []parser.SystemGroup
	search *parser.SearchIndex
}

// contentState holds the live content and the routes built from it. A refresh
// builds a complete snapshot and route set and swaps them under the lock, so no
// request observes a partially loaded set and in-flight requests keep the set
// they started on.
type contentState struct {
	mu    sync.RWMutex
	mux   http.Handler
	cfg   config
	store stores.Store
	authn *auth
}

func newContentState(cfg config, store stores.Store, authn *auth) (*contentState, error) {
	cs := &contentState{cfg: cfg, store: store, authn: authn}
	if err := cs.reload(); err != nil {
		return nil, err
	}
	return cs, nil
}

// ServeHTTP serves from the current route set.
func (cs *contentState) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cs.mu.RLock()
	mux := cs.mux
	cs.mu.RUnlock()
	mux.ServeHTTP(w, r)
}

// reload rebuilds the snapshot and routes and swaps them in. A failed load
// leaves the current set serving.
func (cs *contentState) reload() error {
	snap, err := loadContent(cs.cfg)
	if err != nil {
		return err
	}
	mux := buildMux(cs.cfg, snap, cs.store, cs.authn)
	cs.mu.Lock()
	cs.mux = mux
	cs.mu.Unlock()
	return nil
}

// loadContent resolves the content source and parses it. An empty tree is not an
// error: the index renders a welcome that says how to add runbooks. The git-sync
// records path lives inside the repo when content is at its root, so it is
// excluded from the walk (records have no frontmatter).
func loadContent(cfg config) (*contentSnapshot, error) {
	dir, err := contentsource.Resolve(context.Background(), contentsource.Options{
		Source: cfg.ContentSource,
		Dir:    cfg.ContentDir,
		Repo:   cfg.GitSyncRepo,
		Branch: cfg.GitSyncBranch,
		Creds: gitrepo.Credentials{
			Username: cfg.GitSyncUsername,
			Token:    cfg.GitSyncToken,
			SSHKey:   cfg.GitSyncSSHKey,
		},
		Path:  cfg.ContentGitPath,
		Cache: cfg.ContentGitCache,
	})
	if err != nil {
		return nil, err
	}
	defs, err := parser.LoadDir(dir, cfg.GitSyncBasePath)
	if err != nil {
		return nil, err
	}
	return &contentSnapshot{
		defs:   defs,
		groups: parser.GroupBySystem(defs),
		search: parser.BuildIndex(defs),
	}, nil
}

// buildMux registers every route for a snapshot. It is rebuilt on a refresh and
// swapped in, so slug additions and removals take effect.
func buildMux(cfg config, snap *contentSnapshot, store stores.Store, authn *auth) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/public/", http.FileServer(http.FS(static)))
	// Liveness only: no auth, no information, so probes work on an identity-gated
	// instance.
	mux.HandleFunc("/healthz", healthzHandler)

	index := func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		views.IndexPage(snap.groups, u.IsAdmin(), cfg.IdentityEnabled).Render(r.Context(), w)
	}

	if authn != nil {
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
		mux.HandleFunc("/admin", authn.requireAdmin(func(w http.ResponseWriter, r *http.Request) {
			users, err := store.ListUsers(r.Context())
			if err != nil {
				http.Error(w, "could not load users", http.StatusInternalServerError)
				return
			}
			u := userFrom(r.Context())
			views.AdminPage(snap.groups, users, u.IsAdmin(), cfg.IdentityEnabled).Render(r.Context(), w)
		}))
		mux.HandleFunc("/admin/audit", authn.requireAdmin(authn.auditPage(snap.groups)))
		mux.HandleFunc("/api/runbooks/v1/ack", authn.requireAPI(authn.ackRunbook))
		mux.HandleFunc("/api/auth/v1/invites", authn.requireAdminAPI(authn.createInvite))
		mux.HandleFunc("/api/auth/v1/users/disable", authn.requireAdminAPI(authn.disableUser))
		mux.HandleFunc("/api/auth/v1/users/enable", authn.requireAdminAPI(authn.enableUser))
		mux.HandleFunc("/account", authn.requirePage(authn.accountPage(snap.groups)))
		mux.HandleFunc("/api/auth/v1/profile", authn.requireAPI(authn.updateProfile))
		mux.HandleFunc("/api/auth/v1/sessions", authn.requireAPI(authn.listSessions))
		mux.HandleFunc("/api/auth/v1/sessions/revoke", authn.requireAPI(authn.revokeSessions))
		mux.HandleFunc("/api/auth/v1/passkeys/begin", authn.requireAPI(authn.passkeyBegin))
		mux.HandleFunc("/api/auth/v1/passkeys/finish", authn.requireAPI(authn.passkeyFinish))
		mux.HandleFunc("/api/auth/v1/passkeys/rename", authn.requireAPI(authn.passkeyRename))
		mux.HandleFunc("/api/auth/v1/passkeys/remove", authn.requireAPI(authn.passkeyRemove))
		mux.HandleFunc("/api/auth/v1/apikeys", authn.requireAPI(authn.createAPIKey))
		mux.HandleFunc("/api/auth/v1/apikeys/revoke", authn.requireAPI(authn.revokeAPIKey))
		// Break-glass recovery is only reachable when a token is configured.
		if cfg.IdentityRecoveryToken != "" {
			mux.HandleFunc("/recovery", authn.recoveryPage)
			mux.HandleFunc("/api/auth/v1/recovery/begin", authn.recoveryBegin)
			mux.HandleFunc("/api/auth/v1/recovery/finish", authn.recoveryFinish)
		}

		index = authn.requirePage(index)
	}

	mux.HandleFunc("/{$}", index)

	if cfg.StyleGuideEnabled {
		registerStyleGuide(mux, authn, cfg)
	}

	for _, rb := range snap.defs {
		rb := rb
		page := func(w http.ResponseWriter, r *http.Request) {
			u := userFrom(r.Context())
			views.RunbookPage(rb, snap.groups, views.PageConfig{
				GitSyncEnabled:       cfg.GitSyncEnabled,
				GitSyncRequiresToken: gitSyncNeedsBrowserToken(cfg.IdentityEnabled, cfg.GitSyncAPIToken),
				RecordsBasePath:      cfg.GitSyncBasePath,
				IsAdmin:              u.IsAdmin(),
				IdentityEnabled:      cfg.IdentityEnabled,
			}).Render(r.Context(), w)
		}
		if authn != nil {
			page = authn.requirePage(page)
		}
		mux.HandleFunc("/"+rb.Slug, page)

		// The raw markdown is a read-only machine endpoint: session or API key.
		raw := func(w http.ResponseWriter, r *http.Request) { serveMarkdown(w, r, rb) }
		if authn != nil {
			raw = authn.requireRead(raw)
		}
		mux.HandleFunc("/"+rb.Slug+".md", raw)
	}

	// The llms.txt index is generated from the same groups as the sidebar, so it
	// matches site order. Read-only machine endpoint: session or API key.
	llmsIndex := http.HandlerFunc(handleLLMSIndex(snap.groups))
	if authn != nil {
		llmsIndex = authn.requireRead(llmsIndex)
	}
	mux.Handle("/llms.txt", llmsIndex)

	gitSync := http.HandlerFunc(handleGitSync(cfg))
	if authn != nil {
		gitSync = authn.gateGitSync(gitSync)
	}
	mux.Handle("/api/git-sync/v1", gitSync)

	// Body search reads the same content as the pages, so with identity on it sits
	// behind the same read gate as the raw markdown and the llms index — a 401
	// JSON, not a login redirect. A read-scoped API key satisfies it.
	search := http.HandlerFunc(handleSearch(snap.search))
	if authn != nil {
		search = authn.requireRead(search)
	}
	mux.Handle("/api/runbooks/v1/search", search)

	return mux
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
