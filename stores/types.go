package stores

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
)

// Role is a user's permission level. Deliberately two: the rich RBAC layer is
// hosted-only and must not be pre-built here.
type Role string

const (
	// RoleAdmin may invite users, manage them and run recovery.
	RoleAdmin Role = "admin"
	// RoleMember may use the app and sync.
	RoleMember Role = "member"
)

// User is a named person on the instance.
type User struct {
	ID          string
	Email       string // optional: only needed to attribute commits
	DisplayName string
	Role        Role
	CreatedAt   time.Time
	DisabledAt  time.Time // zero when the user is enabled
}

// Enabled reports whether the user may authenticate.
func (u User) Enabled() bool { return u.DisabledAt.IsZero() }

// IsAdmin reports whether the user may invite, manage users and run recovery.
func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

// Transports are the WebAuthn transports an authenticator reported (usb, nfc,
// ble, internal, hybrid). Stored comma-separated, NULL when empty.
type Transports []string

// Value stores the transports as a comma-separated string.
func (t Transports) Value() (driver.Value, error) {
	if len(t) == 0 {
		return nil, nil
	}
	return strings.Join(t, ","), nil
}

// Scan reads the comma-separated column.
func (t *Transports) Scan(src any) error {
	var joined string
	switch v := src.(type) {
	case nil:
		*t = nil
		return nil
	case string:
		joined = v
	case []byte:
		joined = string(v)
	default:
		return fmt.Errorf("stores: cannot scan %T into Transports", src)
	}
	if joined == "" {
		*t = nil
		return nil
	}
	*t = strings.Split(joined, ",")
	return nil
}

// Credential is a stored WebAuthn passkey.
type Credential struct {
	ID           string // internal row id
	UserID       string
	CredentialID []byte // WebAuthn credential id, as sent by the authenticator
	PublicKey    []byte
	SignCount    uint32
	Transports   Transports
	AAGUID       []byte
	Flags        uint8  // WebAuthn authenticator flags (UP/UV/BE/BS); login compares these
	Label        string // user-facing name, e.g. "YubiKey 5"
	CreatedAt    time.Time
	LastUsedAt   time.Time // zero until first use
}

// Session is an authenticated browser session. ID is the hash of the opaque
// cookie value, never the value itself.
type Session struct {
	ID         string
	UserID     string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
	UserAgent  string
	IP         string
}

// Challenge is short-lived WebAuthn ceremony state, keyed by an opaque cookie.
type Challenge struct {
	ID        string
	Kind      string // "registration" | "login"
	Data      []byte
	ExpiresAt time.Time
}

// Invite is a single-use enrolment link. ID is the hash of the token; the raw
// token only ever lives in the link.
type Invite struct {
	ID        string
	UserID    string // empty for a new-user invite; set for re-enrolment
	Role      Role
	CreatedBy string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    time.Time // zero until consumed
}

// Used reports whether the invite has been consumed.
func (i Invite) Used() bool { return !i.UsedAt.IsZero() }

// Expired reports whether the invite is past its expiry at now.
func (i Invite) Expired(now time.Time) bool { return !now.Before(i.ExpiresAt) }
