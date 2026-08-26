// Package varahome resolves the location of VARA's per-user home directory —
// the place client-side configuration that is NOT part of any repository lives:
// the user's default developer identity and the remote credential store.
//
// RFC:
//
//	VARA-RFC-0017 Identity      (default developer identity)
//	VARA-RFC-0020 Accounts      (client credential storage)
//
// Layer: a leaf. It imports only the standard library and is imported by the
// client-config packages (devidentity, credstore) and, through them, the
// command layer. It never imports a higher layer, and it holds no repository
// state — the point of a home *outside* any `.vara/` is that secrets and global
// identity never live inside a repository (RFC-0020 §11 credential storage).
package varahome

import (
	"os"
	"path/filepath"
)

// EnvHome is the environment variable that, when set, overrides the computed
// home directory. It mirrors GIT_CONFIG-style overrides and makes the home
// trivially relocatable in tests and CI.
const EnvHome = "VARA_HOME"

// Dir returns the VARA home directory, creating nothing. Resolution order:
//
//  1. $VARA_HOME, if set (verbatim).
//  2. <os.UserConfigDir()>/vara — e.g. %AppData%\vara on Windows,
//     ~/.config/vara on Linux, ~/Library/Application Support/vara on macOS.
//
// It returns an error only if neither source can be determined, which on a
// sane account should never happen.
func Dir() (string, error) {
	if h := os.Getenv(EnvHome); h != "" {
		return h, nil
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "vara"), nil
}

// Ensure returns the home directory, creating it with 0700 permissions if it
// does not yet exist. 0700 keeps the directory — and anything a caller writes
// into it — private to the owning user, which matters because the credential
// store lives here (RFC-0020 §11).
func Ensure() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}
