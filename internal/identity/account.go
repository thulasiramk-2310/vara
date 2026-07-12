package identity

import "time"

// AccountState is a durable account's lifecycle position (RFC-0020 §5.1). Only an
// Active account authenticates; Disabled/Creating/Deleting authenticate to
// nothing (fail closed).
type AccountState string

const (
	AccountActive   AccountState = "active"
	AccountDisabled AccountState = "disabled"
	// Creating/Deleting are tombstones; the v1 file store writes an account in a
	// single atomic step so they are rarely observed, but the states exist so a
	// future multi-artifact account (e.g. + profile) can tombstone like a repo.
	AccountCreating AccountState = "creating"
	AccountDeleting AccountState = "deleting"
)

// Account is a durable subject: a username (the RFC-0018 policy subject key), a
// password hash (argon2id PHC, §6.1), and a lifecycle state. It carries NO
// capabilities — authorization is entirely RFC-0018 policy keyed on the username
// (S7).
type Account struct {
	Username     string       `json:"username"`
	PasswordHash string       `json:"password_hash"`
	State        AccountState `json:"state"`
	CreatedAt    time.Time    `json:"created_at"`
}

// Password policy bounds (RFC-0020 §6.3): a length floor, and a high ceiling so
// long passphrases are never silently truncated. No composition/complexity rules.
const (
	MinPasswordLen = 8
	MaxPasswordLen = 1024
)

// ValidPassword enforces the minimal policy. UTF-8 bytes are accepted as-is.
func ValidPassword(pw string) bool {
	return len(pw) >= MinPasswordLen && len(pw) <= MaxPasswordLen
}

// ValidUsername reports whether a username is legal (RFC-0020 §5.1), under the
// same family as repository names: a portable, filesystem-safe charset with the
// control-plane-reserving prefixes forbidden, so a username is a safe filename and
// can never traverse the account store's directory.
func ValidUsername(name string) bool {
	if len(name) == 0 || len(name) > 39 || name == "." || name == ".." {
		return false
	}
	if name[0] == '_' || name[0] == '.' || name[0] == '-' {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z',
			r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}
