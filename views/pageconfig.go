package views

// PageConfig is the runtime configuration the client reads from #page-config:
// where records land on disk and whether the sync endpoint wants a bearer token.
type PageConfig struct {
	GitSyncEnabled       bool
	GitSyncRequiresToken bool
	RecordsBasePath      string
	IsAdmin              bool
	IdentityEnabled      bool
}
