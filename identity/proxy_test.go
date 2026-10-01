package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"runbooks/stores"
)

// conflictOnceStore reports a duplicate-email conflict after the insert
// succeeds, simulating a second request that raced the first on the same email.
type conflictOnceStore struct {
	stores.UserStore
}

func (c *conflictOnceStore) InsertUser(ctx context.Context, u stores.User) error {
	if err := c.UserStore.InsertUser(ctx, u); err != nil {
		return err
	}
	return stores.NewConflictError(errors.New("duplicate email"))
}

func TestProvisionProxyUserCreatesMember(t *testing.T) {
	svc, st := newTestService(t)
	ctx := context.Background()

	u, err := svc.ProvisionProxyUser(ctx, "alice@example.com", "Alice")
	if err != nil {
		t.Fatalf("ProvisionProxyUser: %v", err)
	}
	if u.Role != stores.RoleMember {
		t.Errorf("role = %q, want member", u.Role)
	}
	if u.Email != "alice@example.com" || u.DisplayName != "Alice" {
		t.Errorf("user = %+v", u)
	}
	if u.CreatedAt.IsZero() {
		t.Error("CreatedAt not set")
	}

	got, err := st.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("persisted id = %q, want %q", got.ID, u.ID)
	}
}

func TestProvisionProxyUserNameFallsBackToEmail(t *testing.T) {
	svc, _ := newTestService(t)

	u, err := svc.ProvisionProxyUser(context.Background(), "bob@example.com", "   ")
	if err != nil {
		t.Fatalf("ProvisionProxyUser: %v", err)
	}
	if u.DisplayName != "bob@example.com" {
		t.Errorf("DisplayName = %q, want the email", u.DisplayName)
	}
}

func TestProvisionProxyUserReturnsExistingWithoutRenaming(t *testing.T) {
	svc, st := newTestService(t)
	seedUser(t, st, "existing")

	u, err := svc.ProvisionProxyUser(context.Background(), "existing@example.com", "Other Name")
	if err != nil {
		t.Fatalf("ProvisionProxyUser: %v", err)
	}
	if u.ID != "existing" {
		t.Errorf("id = %q, want existing", u.ID)
	}
	if u.DisplayName != "Test existing" {
		t.Errorf("display name was overwritten: %q", u.DisplayName)
	}
}

func TestProvisionProxyUserRefusesDisabled(t *testing.T) {
	svc, st := newTestService(t)
	ctx := context.Background()
	disabled := stores.User{
		ID: "gone", Email: "gone@example.com", DisplayName: "Gone",
		Role: stores.RoleMember, CreatedAt: time.Now(), DisabledAt: time.Now(),
	}
	if err := st.InsertUser(ctx, disabled); err != nil {
		t.Fatalf("insert disabled user: %v", err)
	}

	if _, err := svc.ProvisionProxyUser(ctx, "gone@example.com", "Gone"); !errors.Is(err, ErrUser) {
		t.Errorf("err = %v, want ErrUser", err)
	}
}

func TestProvisionProxyUserRefusesEmptyEmail(t *testing.T) {
	svc, _ := newTestService(t)

	if _, err := svc.ProvisionProxyUser(context.Background(), "   ", "No Email"); !errors.Is(err, ErrUser) {
		t.Errorf("err = %v, want ErrUser", err)
	}
}

func TestProvisionProxyUserHandlesConcurrentInsert(t *testing.T) {
	svc, st := newTestService(t)
	svc.users = &conflictOnceStore{UserStore: st}
	ctx := context.Background()

	u, err := svc.ProvisionProxyUser(ctx, "race@example.com", "Race")
	if err != nil {
		t.Fatalf("ProvisionProxyUser: %v", err)
	}
	if u.ID == "" {
		t.Fatal("expected the concurrently inserted row")
	}

	// A later request resolves the same row by email rather than inserting again.
	again, err := svc.ProvisionProxyUser(ctx, "race@example.com", "Race")
	if err != nil {
		t.Fatalf("second ProvisionProxyUser: %v", err)
	}
	if again.ID != u.ID {
		t.Errorf("id = %q, want the same row %q", again.ID, u.ID)
	}
}
