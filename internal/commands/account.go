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
// --repo — the capabilities it holds there.
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
	path := protocol.PathWhoami
	if repo != "" {
		path += "?repo=" + url.QueryEscape(repo)
	}
	var resp protocol.WhoamiResponse
	if err := cpDo(base, http.MethodGet, path, authHeader(cfg), nil, &resp); err != nil {
		return err
	}
	fmt.Printf("user:   %s\n", resp.ID)
	fmt.Printf("method: %s\n", resp.Method)
	fmt.Printf("server: %s\n", strings.TrimRight(base, "/"))
	if resp.Repository != "" {
		fmt.Printf("repository: %s\n", resp.Repository)
		fmt.Println("capabilities:")
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
			fmt.Printf("  %s %s\n", mark, c)
		}
	}
	return nil
}

// RunLogin: vara login <server-url> <username> --password <pw>. Prints the
// session secret (shown once) to use as a bearer token.
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
	fmt.Printf("logged in as %s\nsession token (use as --bearer): %s\nexpires: %s\n", username, resp.Secret, resp.ExpiresAt)
	return nil
}

// RunLogout: vara logout <server-url> --bearer <session-token>.
func RunLogout(args []string, cfg AuthConfig) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vara logout <server-url> --bearer <session-token>")
	}
	if cfg.Bearer == "" {
		return fmt.Errorf("logout: --bearer <session-token> is required")
	}
	if err := cpDo(args[0], http.MethodDelete, protocol.PathSessions+"/current", authHeader(cfg), nil, nil); err != nil {
		return err
	}
	fmt.Println("logged out")
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
