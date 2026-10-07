// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
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

// gitSyncNeedsBrowserToken reports whether the Sync UI must prompt for the shared
// bearer token. With identity on, a signed-in session (or proxy assertion)
// authorises and attributes the sync, and the page is already behind the read
// gate, so the token is only the identity-off deployment's gate. It stays
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
	if cfg.ContentRefreshInterval > 0 {
		log.Printf("content: refreshing every %s", cfg.ContentRefreshInterval)
		go state.poll(cfg.ContentRefreshInterval)
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
	defs     []parser.RunbookDef
	groups   []parser.SystemGroup
	search   *parser.SearchIndex
	glossary []parser.GlossaryEntry
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

	// refreshMu serializes refreshes so the poller and the manual endpoint cannot
	// fetch and reset the same git worktree concurrently.
	refreshMu sync.Mutex

	// bg tracks the startup background refresh (a reused git cache), so a test
	// can wait for it to settle before its temp dirs are removed.
	bg sync.WaitGroup
}

func newContentState(cfg config, store stores.Store, authn *auth) (*contentState, error) {
	cs := &contentState{cfg: cfg, store: store, authn: authn}
	dir, reused, err := contentsource.Open(context.Background(), cs.sourceOptions())
	if err != nil {
		return nil, err
	}
	snap, err := parseContent(dir, cfg.GitSyncBasePath)
	if err != nil {
		return nil, err
	}
	cs.mu.Lock()
	cs.mux = cs.buildMux(snap)
	cs.mu.Unlock()
	// A reused git cache is last-good: serve it now and fetch in the background.
	// A failed refresh logs and leaves the startup snapshot serving.
	if reused {
		cs.bg.Add(1)
		go func() {
			defer cs.bg.Done()
			if err := cs.reload(); err != nil {
				log.Printf("content: background refresh failed: %v", err)
			}
		}()
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

// reload fetches the source, rebuilds the snapshot and routes, and swaps them
// in. A failed fetch or parse leaves the current set serving (last-good).
// Refreshes are serialized; a caller that must not wait uses reloadIfIdle.
func (cs *contentState) reload() error {
	cs.refreshMu.Lock()
	defer cs.refreshMu.Unlock()
	return cs.reloadLocked()
}

// reloadIfIdle refreshes unless a refresh is already running, in which case the
// tick is skipped rather than queued.
func (cs *contentState) reloadIfIdle() error {
	if !cs.refreshMu.TryLock() {
		return nil
	}
	defer cs.refreshMu.Unlock()
	return cs.reloadLocked()
}

func (cs *contentState) reloadLocked() error {
	dir, err := contentsource.Refresh(context.Background(), cs.sourceOptions())
	if err != nil {
		return err
	}
	snap, err := parseContent(dir, cs.cfg.GitSyncBasePath)
	if err != nil {
		return err
	}
	mux := cs.buildMux(snap)
	cs.mu.Lock()
	cs.mux = mux
	cs.mu.Unlock()
	return nil
}

// poll refreshes the content source every interval until the process ends. Each
// wait is jittered ±10% so instances polling one repo do not beat in lockstep.
func (cs *contentState) poll(interval time.Duration) {
	for {
		time.Sleep(jitter(interval))
		if err := cs.reloadIfIdle(); err != nil {
			log.Printf("content: scheduled refresh failed: %v", err)
		}
	}
}

// jitter spreads a wait by ±10%.
func jitter(interval time.Duration) time.Duration {
	ten := interval / 10
	if ten <= 0 {
		return interval
	}
	return interval + time.Duration(rand.Int64N(int64(2*ten)+1)) - ten
}

// sourceOptions builds the content-source options from the config. Content and
// notes sync share one remote and credential (GITSYNC_*).
func (cs *contentState) sourceOptions() contentsource.Options {
	cfg := cs.cfg
	return contentsource.Options{
		Source: cfg.ContentSource,
		Dir:    cfg.ContentDir,
		Repo:   cfg.ContentGitRepo,
		Branch: cfg.ContentGitBranch,
		Creds: gitrepo.Credentials{
			Username: cfg.ContentGitUsername,
			Token:    cfg.ContentGitToken,
			SSHKey:   cfg.ContentGitSSHKey,
		},
		Path:  cfg.ContentGitPath,
		Cache: cfg.ContentGitCache,
	}
}

// parseContent parses a content directory into a snapshot. An empty tree is not
// an error: the index renders a welcome that says how to add runbooks. The
// git-sync records path lives inside the repo when content is at its root, so it
// is excluded from the walk (records have no frontmatter).
func parseContent(dir, recordsPath string) (*contentSnapshot, error) {
	defs, err := parser.LoadDir(dir, recordsPath)
	if err != nil {
		return nil, err
	}
	glossary, err := parser.LoadGlossary(dir)
	if err != nil {
		return nil, err
	}
	return &contentSnapshot{
		defs:     defs,
		groups:   parser.GroupBySystem(defs),
		search:   parser.BuildIndex(defs),
		glossary: glossary,
	}, nil
}

// buildMux registers every route for a snapshot. It is rebuilt on a refresh and
// swapped in, so slug additions and removals take effect.
func (cs *contentState) buildMux(snap *contentSnapshot) *http.ServeMux {
	cfg, authn, store := cs.cfg, cs.authn, cs.store
	mux := http.NewServeMux()
	mux.Handle("/public/", http.FileServer(http.FS(static)))
	// Liveness only: no auth, no information, so probes work on an identity-gated
	// instance.
	mux.HandleFunc("/healthz", healthzHandler)

	index := func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		views.IndexPage(snap.groups, views.PageConfig{
			IsAdmin:         u.IsAdmin(),
			IdentityEnabled: cfg.IdentityEnabled,
			PublicURL:       cfg.PublicURL,
			SiteDescription: cfg.SiteDescription,
		}).Render(r.Context(), w)
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
		mux.HandleFunc("/admin/audit.csv", authn.requireAdmin(authn.auditCSV))
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
				PublicURL:            cfg.PublicURL,
				SiteDescription:      cfg.SiteDescription,
				Glossary:             snap.glossary,
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

	// The runbook-review agent skill is a read-only machine endpoint like
	// llms.txt: session or read-scoped API key when identity is on.
	reviewSkill := http.HandlerFunc(serveReviewSkill)
	if authn != nil {
		reviewSkill = authn.requireRead(reviewSkill)
	}
	mux.Handle("/skill/runbook-review.md", reviewSkill)

	// robots.txt is public; the sitemap lists the same slugs as the pages, so
	// with identity on it sits behind the read gate like llms.txt. The sitemap
	// needs absolute URLs, so it exists only when PUBLIC_URL is set.
	mux.Handle("/robots.txt", http.HandlerFunc(handleRobots(cfg.IdentityEnabled, cfg.PublicURL)))
	if cfg.PublicURL != "" {
		sitemap := http.HandlerFunc(handleSitemap(snap.groups, cfg.PublicURL))
		if authn != nil {
			sitemap = authn.requireRead(sitemap)
		}
		mux.Handle("/sitemap.xml", sitemap)
	}

	gitSync := http.HandlerFunc(handleGitSync(cfg))
	if authn != nil {
		gitSync = authn.gateGitSync(gitSync)
	}
	mux.Handle("/api/git-sync/v1", gitSync)

	// Body search reads the same content as the pages, so with identity on it sits
	// behind the same read gate as the raw markdown and the llms index: a 401
	// JSON, not a login redirect. A read-scoped API key satisfies it.
	search := http.HandlerFunc(handleSearch(snap.search))
	if authn != nil {
		search = authn.requireRead(search)
	}
	mux.Handle("/api/runbooks/v1/search", search)

	mux.HandleFunc("/api/content/v1/refresh", cs.refreshHandler())

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
