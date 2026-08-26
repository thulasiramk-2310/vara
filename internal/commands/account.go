// Package commands — vara login/logout/token/account (RFC-0020 control plane).
//
// These are the client side of the account/session/token control plane a
// `vara serve --accounts` host exposes. Like the repo client they are pure
// codecs: they translate a CLI invocation into a JSON request and print the
// result. Secrets returned by the server (session/token) are shown once here too.
package commands

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/thulasiramk-2310/vara/pkg/color"

	"github.com/thulasiramk-2310/vara/internal/credstore"
	"github.com/thulasiramk-2310/vara/internal/devidentity"
	"github.com/thulasiramk-2310/vara/internal/protocol"
)

// AuthConfig carries the client credential and (for login/create) a password.
type AuthConfig struct {
	Basic    string // "user:secret"
	Bearer   string // opaque bearer secret
	Password string // for login / account create / passwd
}

func authHeader(cfg AuthConfig) string {
	switch {
	case cfg.Basic != "":
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(cfg.Basic))
	case cfg.Bearer != "":
		return "Bearer " + cfg.Bearer
	}
	return ""
}

// cpDo issues a control-plane request to base+path and decodes into out (nil for
// 204). A non-2xx becomes the server's structured error.
func cpDo(base, method, path, auth string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, strings.TrimRight(base, "/")+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set(protocol.HeaderProto, protocol.Version)
	req.Header.Set(protocol.HeaderWire, protocol.WireVersion)
	if body != nil {
		req.Header.Set("Content-Type", protocol.CTJSON)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		var eb protocol.Error
		if json.Unmarshal(data, &eb) == nil && eb.Code != "" {
			return fmt.Errorf("%s [%s]: %s", eb.Code, resp.Status, eb.Message)
		}
		return fmt.Errorf("http %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// RunWhoami: vara whoami <server-url> [--repo <name>] [--basic|--bearer]. Prints
// the identity the server resolves for the presented credential, and — with
// --repo — the capabilities it holds there. When no explicit --basic/--bearer
// is given, the stored login for the server's origin is used automatically, so
// `vara whoami <server>` "just works" after `vara login`.
func RunWhoami(args []string, cfg AuthConfig) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vara whoami <server-url> [--repo <name>] [--basic u:s | --bearer t]")
	}
	base := args[0]
	var repo string
	for i := 1; i < len(args); i++ {
		if args[i] == "--repo" && i+1 < len(args) {
			repo = args[i+1]
			i++
		}
	}

	// Resolve the credential: an explicit flag wins; otherwise the stored login.
	auth := authHeader(cfg)
	usedStored := false
	if auth == "" {
		if store, err := credstore.Load(); err == nil {
			if c, ok := store.Get(base); ok {
				if c.Expired() {
					return fmt.Errorf("your session for %s has expired.\n\nRun:\n\n  vara login %s",
						originOrRaw(base), originOrRaw(base))
				}
				auth = "Bearer " + c.Secret
				usedStored = true
			}
		}
	}
	if auth == "" {
		// No credential at all: report unauthenticated rather than probing.
		fmt.Println(color.Bold("VARA Account"))
		fmt.Println()
		fmt.Printf("  %-10s %s\n", "Status", "Not authenticated")
		fmt.Println()
		fmt.Printf("Run:\n\n  vara login %s\n", strings.TrimRight(base, "/"))
		return nil
	}

	path := protocol.PathWhoami
	if repo != "" {
		path += "?repo=" + url.QueryEscape(repo)
	}
	var resp protocol.WhoamiResponse
	if err := cpDo(base, http.MethodGet, path, auth, nil, &resp); err != nil {
		if usedStored && isAuthnErr(err.Error()) {
			return fmt.Errorf("authentication expired.\n\nRun:\n\n  vara login %s", strings.TrimRight(base, "/"))
		}
		return err
	}

	fmt.Println(color.Bold("VARA Account"))
	fmt.Println()
	fmt.Printf("  %-11s %s\n", "Username", resp.ID)
	// Enrich with the local commit identity when one is configured — handy, and
	// still entirely offline for these two lines.
	if id, ok, _ := devidentity.Default(); ok {
		fmt.Printf("  %-11s %s\n", "Name", id.Name)
		fmt.Printf("  %-11s %s\n", "Email", id.Email)
	}
	fmt.Printf("  %-11s %s\n", "Server", strings.TrimRight(base, "/"))
	fmt.Printf("  %-11s %s\n", "Method", resp.Method)
	fmt.Printf("  %-11s %s\n", "Status", "Authenticated "+color.Green("✓"))

	if resp.Repository != "" {
		fmt.Printf("\n  repository: %s\n", resp.Repository)
		fmt.Println("  capabilities:")
		names := make([]string, 0, len(resp.Capabilities))
		for c := range resp.Capabilities {
			names = append(names, c)
		}
		sort.Strings(names)
		for _, c := range names {
			mark := color.Red("✗")
			if resp.Capabilities[c] {
				mark = color.Green("✓")
			}
			fmt.Printf("    %s %s\n", mark, c)
		}
	}
	return nil
}

// originOrRaw returns the origin of base, or base itself if it cannot be parsed.
func originOrRaw(base string) string {
	if o, err := credstore.Origin(base); err == nil {
		return o
	}
	return strings.TrimRight(base, "/")
}

// RunLogin: vara login <server-url> <username> --password <pw>. On success the
// session secret is stored in the credential store (keyed by the server origin),
// NOT printed — subsequent clone/fetch/pull/push authenticate automatically.
func RunLogin(args []string, cfg AuthConfig) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: vara login <server-url> <username> --password <pw>")
	}
	base, username := args[0], args[1]
	if cfg.Password == "" {
		return fmt.Errorf("login: --password is required")
	}
	var resp protocol.LoginResponse
	if err := cpDo(base, http.MethodPost, protocol.PathSessions, "",
		protocol.LoginRequest{Username: username, Password: cfg.Password}, &resp); err != nil {
		return err
	}

	store, err := credstore.Load()
	if err != nil {
		return fmt.Errorf("login: open credential store: %w", err)
	}
	if err := store.Set(base, credstore.Credential{
		Username:  username,
		Secret:    resp.Secret,
		ExpiresAt: resp.ExpiresAt,
	}); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if err := store.Save(); err != nil {
		return fmt.Errorf("login: store credential: %w", err)
	}

	fmt.Printf("%s Authentication successful\n\n", color.Green("✓"))
	fmt.Printf("Logged in to %s as %s.\n", originOrRaw(base), username)
	fmt.Println("Session stored securely.")
	return nil
}

// RunLogout: vara logout <server-url>. Revokes the stored session on the server
// (best-effort) and removes the credential from the local store. An explicit
// --bearer still works for revoking a token you hold but did not store.
func RunLogout(args []string, cfg AuthConfig) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vara logout <server-url>")
	}
	base := args[0]

	// Determine the bearer to revoke: an explicit flag wins; else the stored one.
	revoke := cfg.Bearer
	store, err := credstore.Load()
	if err != nil {
		return fmt.Errorf("logout: open credential store: %w", err)
	}
	stored, hadStored := store.Get(base)
	if revoke == "" && hadStored {
		revoke = stored.Secret
	}
	if revoke == "" && !hadStored {
		fmt.Printf("Not logged in to %s.\n", originOrRaw(base))
		return nil
	}

	// Best-effort server-side revocation; a network failure must still let us
	// clear the local credential so `logout` always leaves you logged out.
	var serverErr error
	if revoke != "" {
		serverErr = cpDo(base, http.MethodDelete, protocol.PathSessions+"/current",
			"Bearer "+revoke, nil, nil)
	}

	if hadStored {
		if _, err := store.Delete(base); err != nil {
			return fmt.Errorf("logout: %w", err)
		}
		if err := store.Save(); err != nil {
			return fmt.Errorf("logout: update credential store: %w", err)
		}
	}

	if serverErr != nil {
		fmt.Printf("Logged out locally (server revocation failed: %v).\n", serverErr)
		return nil
	}
	fmt.Printf("Logged out of %s.\n", originOrRaw(base))
	return nil
}

// RunToken: vara token <create|list|revoke> <server-url> [args].
func RunToken(args []string, cfg AuthConfig) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: vara token <create|list|revoke> <server-url> [args]")
	}
	sub, base, rest := args[0], args[1], args[2:]
	auth := authHeader(cfg)
	switch sub {
	case "create":
		if len(rest) < 1 {
			return fmt.Errorf("usage: vara token create <server-url> <name>")
		}
		var resp protocol.CreateTokenResponse
		if err := cpDo(base, http.MethodPost, protocol.PathTokens, auth, protocol.CreateTokenRequest{Name: rest[0]}, &resp); err != nil {
			return err
		}
		fmt.Printf("created token %q (id=%s)\ntoken (use as --bearer, shown once): %s\n", resp.Name, resp.ID, resp.Secret)
		return nil
	case "list":
		var resp protocol.ListTokensResponse
		if err := cpDo(base, http.MethodGet, protocol.PathTokens, auth, nil, &resp); err != nil {
			return err
		}
		if len(resp.Tokens) == 0 {
			fmt.Println("(no tokens)")
			return nil
		}
		for _, t := range resp.Tokens {
			fmt.Printf("%-28s %s  created=%s\n", t.ID, t.Name, t.CreatedAt)
		}
		return nil
	case "revoke":
		if len(rest) < 1 {
			return fmt.Errorf("usage: vara token revoke <server-url> <token-id>")
		}
		if err := cpDo(base, http.MethodDelete, protocol.PathTokens+"/"+rest[0], auth, nil, nil); err != nil {
			return err
		}
		fmt.Printf("revoked %s\n", rest[0])
		return nil
	default:
		return fmt.Errorf("token: unknown subcommand %q (want create|list|revoke)", sub)
	}
}

// RunAccount: vara account <create|disable|delete|passwd> <server-url> <username>.
func RunAccount(args []string, cfg AuthConfig) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: vara account <create|disable|delete|passwd> <server-url> <username>")
	}
	sub, base, username := args[0], args[1], args[2]
	auth := authHeader(cfg)
	switch sub {
	case "create":
		if cfg.Password == "" {
			return fmt.Errorf("account create: --password is required")
		}
		var d protocol.AccountDescriptor
		if err := cpDo(base, http.MethodPost, protocol.PathAccounts, auth,
			protocol.CreateAccountRequest{Username: username, Password: cfg.Password}, &d); err != nil {
			return err
		}
		fmt.Printf("created account %s (state=%s)\n", d.Username, d.State)
		return nil
	case "disable":
		if err := cpDo(base, http.MethodPost, protocol.PathAccounts+"/"+username+"/disable", auth, nil, nil); err != nil {
			return err
		}
		fmt.Printf("disabled %s\n", username)
		return nil
	case "delete":
		if err := cpDo(base, http.MethodDelete, protocol.PathAccounts+"/"+username, auth, nil, nil); err != nil {
			return err
		}
		fmt.Printf("deleted %s\n", username)
		return nil
	case "passwd":
		if cfg.Password == "" {
			return fmt.Errorf("account passwd: --password is required")
		}
		if err := cpDo(base, http.MethodPut, protocol.PathAccounts+"/"+username+"/password", auth,
			protocol.SetPasswordRequest{Password: cfg.Password}, nil); err != nil {
			return err
		}
		fmt.Printf("password changed for %s\n", username)
		return nil
	default:
		return fmt.Errorf("account: unknown subcommand %q (want create|disable|delete|passwd)", sub)
	}
}
