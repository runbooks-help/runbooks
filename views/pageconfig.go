// SPDX-License-Identifier: FSL-1.1-MIT

package views

// PageConfig is the page-rendering configuration shared by the public pages. A
// subset (GitSync*, RecordsBasePath) is embedded for the client as #page-config;
// the rest is used server-side to build the document metadata.
type PageConfig struct {
	GitSyncEnabled       bool
	GitSyncRequiresToken bool
	RecordsBasePath      string
	IsAdmin              bool
	IdentityEnabled      bool

	// PublicURL is the site's absolute base (no trailing slash); empty omits
	// canonical/OpenGraph URLs. SiteDescription is the default meta description
	// for pages without their own.
	PublicURL       string
	SiteDescription string
}
