package views

import (
	"context"
	"strings"
	"testing"
	"time"

	"runbooks/stores"
)

func TestAccountPageRendersAPIKeys(t *testing.T) {
	keys := []stores.APIKey{
		{ID: "hash1", UserID: "u1", Label: "on-call bot", CreatedAt: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)},
		{
			ID:        "hash2",
			UserID:    "u1",
			Label:     "old bot",
			CreatedAt: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC),
			RevokedAt: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC),
		},
	}
	var b strings.Builder
	err := AccountPage(nil, stores.User{ID: "u1", DisplayName: "Ada"}, nil, nil, keys, "").Render(context.Background(), &b)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	html := b.String()
	for _, want := range []string{
		"API keys",
		"Create key",
		"data-apikey-reveal",
		"on-call bot",
		"old bot",
		"revoked", // the revoked key is marked
	} {
		if !strings.Contains(html, want) {
			t.Errorf("account page missing %q", want)
		}
	}
	// Only the active key gets a Revoke button.
	if got := strings.Count(html, `data-account="revoke-apikey"`); got != 1 {
		t.Errorf("revoke buttons = %d, want 1", got)
	}
}
