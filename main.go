package main

import (
	"embed"
	"log"
	"net/http"
	"os"
	"strings"

	"runbooks/parser"
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
	trustProxy := strings.ToLower(os.Getenv("GITSYNC_TRUST_PROXY_AUTH"))
	cfg.GitSyncTrustProxyAuth = trustProxy == "true" || trustProxy == "1"

	// A repo and a credential are required to do anything, and the write route
	// must never be reachable unauthenticated: without either a shared token or
	// an explicit assertion that upstream auth exists, sync stays disabled.
	hasCredential := cfg.GitSyncToken != "" || cfg.GitSyncSSHKey != ""
	hasEndpointAuth := cfg.GitSyncAPIToken != "" || cfg.GitSyncTrustProxyAuth
	cfg.GitSyncEnabled = cfg.GitSyncRepo != "" && hasCredential && hasEndpointAuth
	return cfg
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	cfg := loadConfig()

	// An empty content/ is not an error: the index renders a welcome that says
	// how to add runbooks, so a fresh checkout still boots.
	runbooks, err := parser.LoadDir("content")
	if err != nil {
		log.Fatalf("load runbooks: %v", err)
	}

	groups := parser.GroupBySystem(runbooks)

	http.Handle("/public/", http.FileServer(http.FS(static)))
	http.HandleFunc("/api/git-sync/v1", handleGitSync(cfg))

	http.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		views.IndexPage(groups).Render(r.Context(), w)
	})

	for _, rb := range runbooks {
		rb := rb
		http.HandleFunc("/"+rb.Slug, func(w http.ResponseWriter, r *http.Request) {
			views.RunbookPage(rb, groups, views.PageConfig{
				GitSyncEnabled:       cfg.GitSyncEnabled,
				GitSyncRequiresToken: cfg.GitSyncAPIToken != "",
				RecordsBasePath:      cfg.GitSyncBasePath,
			}).Render(r.Context(), w)
		})
	}

	log.Printf("runbooks listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
