package main

import (
	_ "embed"
	"net/http"

	"runbooks/parser"
	"runbooks/views"
)

// styleGuideLLM is the machine-readable mirror of the design system, served at
// /styleguide/llms so agents can build conformant UI without reading the CSS.
//
//go:embed styleguide.llms.txt
var styleGuideLLM string

// registerStyleGuide wires the dev-gated design-system pages. It lives inside
// the app shell, so when identity is on it is gated like the rest of the app and
// the sidebar reflects the real session.
func registerStyleGuide(mux *http.ServeMux, authn *auth, cfg config, groups []parser.SystemGroup) {
	page := func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		views.StyleGuidePage(groups, u.IsAdmin(), cfg.IdentityEnabled).Render(r.Context(), w)
	}
	if authn != nil {
		mux.HandleFunc("/styleguide", authn.requirePage(page))
	} else {
		mux.HandleFunc("/styleguide", page)
	}
	mux.HandleFunc("/styleguide/llms", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(styleGuideLLM))
	})
}
