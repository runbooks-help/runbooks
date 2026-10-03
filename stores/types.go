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

// APIKey is a read-scoped credential an automated client (an agent) presents.
// ID is the hash of the opaque key — the raw value only ever lives in the
// caller's one-time copy, never here. A key belongs to a user so reads attribute
// to them.
type APIKey struct {
	ID         string
	UserID     string
	Label      string // user-facing name, e.g. "on-call bot"
	CreatedBy  string
	CreatedAt  time.Time
	LastUsedAt time.Time // zero until first use
	RevokedAt  time.Time // zero while active
}

// Revoked reports whether the key has been revoked. A revoked key never
// authenticates, and revocation takes effect on the next request.
func (k APIKey) Revoked() bool { return !k.RevokedAt.IsZero() }

// AuthAction names an audited identity action. The set is deliberately small;
// the hosted layer augments it.
type AuthAction string

const (
	// ActionLogin is a completed passkey sign-in.
	ActionLogin AuthAction = "login"
	// ActionLogout is a session ending.
	ActionLogout AuthAction = "logout"
	// ActionEnrol is a passkey added to a user (bootstrap, invite or recovery).
	ActionEnrol AuthAction = "enrol"
	// ActionInvite is an enrolment invite minted by an admin.
	ActionInvite AuthAction = "invite"
	// ActionRevoke is a user's sessions ended by an admin.
	ActionRevoke AuthAction = "revoke"
	// ActionAck is a destructive runbook acknowledged by a reader. The runbook
	// slug is carried in AuthEvent.Detail.
	ActionAck AuthAction = "ack"
	// ActionDisable is a user disabled by an admin.
	ActionDisable AuthAction = "disable"
	// ActionEnable is a disabled user re-enabled by an admin.
	ActionEnable AuthAction = "enable"
)

// AuthEvent is one append-only audit row: who did what, to whom, from where. The
// store assigns ID on insert; the optional fields are stored NULL when empty.
type AuthEvent struct {
	ID           int64
	At           time.Time
	ActorUserID  string
	Action       AuthAction
	TargetUserID string
	Detail       string // free-text subject for events not about another user (e.g. a runbook slug)
	IP           string
	UserAgent    string
}
