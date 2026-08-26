// Package commands — client-side authentication glue for remote operations
// (RFC-0017 §9 credential presentation).
//
// This is the CLI/client boundary the task cares about: credential resolution
// lives HERE, above the transport, never inside the engine. openRemote opens a
// transport and, for an http(s) peer, automatically attaches the stored bearer
// credential for that server's origin — so once you `vara login`, clone/fetch/
// pull/push authenticate with no per-command flags. Local transports carry no
// credential. Anonymous access stays valid (no credential → anonymous request),
// but a credential we know locally to be expired is NOT silently downgraded to
// anonymous: RFC-0017 keeps "invalid credential" distinct from "no credential".
package commands

import (
	"fmt"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/credstore"
	"github.com/thulasiramk-2310/vara/internal/transport"
)

// remoteCred records what openRemote did with credentials, for later messaging.
type remoteCred struct {
	origin   string // server origin, e.g. https://vara.example.com ("" for local)
	http     bool   // the peer is an http(s) server
	attached bool   // a credential was attached to the transport
	username string // the account attached (when attached)
}

// openRemote opens a transport for rawurl and, when it is an http(s) peer,
// attaches the stored credential for that origin. The caller owns Close.
//
// Credential resolution:
//   - no stored credential            → anonymous request (valid; RFC-0017 §13)
//   - stored credential, not expired  → presented as Bearer
//   - stored credential, expired       → hard error with re-login guidance
//     (never sent, never silently downgraded to anonymous; RFC-0017 §11)
func openRemote(rawurl string) (transport.Transport, *remoteCred, error) {
	tr, err := transport.Open(rawurl)
	if err != nil {
		return nil, nil, err
	}
	ht, ok := tr.(*transport.HTTPTransport)
	if !ok {
		return tr, &remoteCred{}, nil // local filesystem transport: no auth
	}

	origin, _ := credstore.Origin(rawurl)
	info := &remoteCred{origin: origin, http: true}

	store, err := credstore.Load()
	if err != nil {
		tr.Close()
		return nil, nil, err
	}
	cred, ok := store.Get(rawurl)
	if !ok {
		return tr, info, nil // anonymous
	}
	if cred.Expired() {
		tr.Close()
		return nil, nil, fmt.Errorf(
			"stored session for %s has expired.\n\n"+
				"Run:\n\n  vara login %s\n\n"+
				"(or 'vara logout %s' to clear it and use anonymous access)",
			origin, origin, origin)
	}
	ht.SetBearerToken(cred.Secret)
	info.attached = true
	info.username = cred.Username
	return tr, info, nil
}

// classifyRemoteErr rewrites a raw transport error into actionable guidance,
// keeping the RFC-0017/0018 distinction between authentication (401) and
// authorization (403) sharp: a valid credential that lacks permission must
// never tell the user to log in again.
func classifyRemoteErr(op, rawurl string, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	origin, oerr := credstore.Origin(rawurl)
	if oerr != nil {
		origin = rawurl
	}

	switch {
	case isAuthnErr(msg):
		return fmt.Errorf("%s: remote authentication failed.\n\n"+
			"No valid credentials were presented — your VARA session may have expired.\n\n"+
			"Run:\n\n  vara login %s", op, origin)
	case isAuthzErr(msg):
		switch op {
		case "push":
			return fmt.Errorf("push rejected.\n\n" +
				"Authentication succeeded, but this account is not authorized to push here.\n" +
				"This is an authorization problem, not a login problem — do not re-login;\n" +
				"ask an admin to grant push access.")
		default:
			return fmt.Errorf("%s: access denied.\n\n"+
				"Authentication succeeded, but this account is not authorized to read here.\n"+
				"This is an authorization problem, not a login problem — do not re-login;\n"+
				"ask an admin to grant 'read' on this repository.", op)
		}
	default:
		return err
	}
}

func isAuthnErr(msg string) bool {
	return strings.Contains(msg, "UNAUTHENTICATED") || strings.Contains(msg, " 401")
}

func isAuthzErr(msg string) bool {
	return strings.Contains(msg, "UNAUTHORIZED") || strings.Contains(msg, " 403")
}
