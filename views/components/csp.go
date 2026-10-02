package components

import (
	"crypto/sha256"
	"encoding/base64"
)

// ThemeScript returns the inline pre-paint theme script. It is inlined into the
// document head; its hash is what the Content-Security-Policy permits.
func ThemeScript() string { return themeScript }

// ThemeScriptCSPHash is the base64 SHA-256 of ThemeScript, for the CSP
// script-src hash that authorises exactly this inline script.
func ThemeScriptCSPHash() string {
	sum := sha256.Sum256([]byte(themeScript))
	return base64.StdEncoding.EncodeToString(sum[:])
}
