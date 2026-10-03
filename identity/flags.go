// SPDX-License-Identifier: FSL-1.1-MIT

package identity

import (
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// storeCredentialFlags packs the WebAuthn authenticator flags into the byte a
// stores.Credential keeps. Login compares the stored backup-eligibility state
// against the assertion, so it must round-trip.
func storeCredentialFlags(f webauthn.CredentialFlags) uint8 {
	var flags protocol.AuthenticatorFlags
	if f.UserPresent {
		flags |= protocol.FlagUserPresent
	}
	if f.UserVerified {
		flags |= protocol.FlagUserVerified
	}
	if f.BackupEligible {
		flags |= protocol.FlagBackupEligible
	}
	if f.BackupState {
		flags |= protocol.FlagBackupState
	}
	return uint8(flags)
}

// credentialFlags expands a stored flags byte back into the library's struct.
func credentialFlags(b uint8) webauthn.CredentialFlags {
	return webauthn.NewCredentialFlags(protocol.AuthenticatorFlags(b))
}
