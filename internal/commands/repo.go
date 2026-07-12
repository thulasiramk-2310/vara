// Package commands — vara repo (RFC-0019 control plane client).
//
// RunRepo is the client side of the repository control plane: it drives
// create/delete/rename/list/show over the JSON API a `vara serve --meta` host
// exposes under /_vara/repositories. Like the data-plane transport client, it is
// a pure codec — it performs no repository logic; the server's Repository Manager
// owns all lifecycle.
package commands

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/protocol"
)

// RepoConfig carries the optional client credential for control-plane calls.
type RepoConfig struct {
	Basic  string // "user:secret"
	Bearer string // opaque token
}

// repoClient is a minimal control-plane HTTP client.
type repoClient struct {
	base       string // server base URL, e.g. http://host:8080 (no trailing /)
	authHeader string
	http       *http.Client
}

func newRepoClient(serverURL string, cfg RepoConfig) *repoClient {
	c := &repoClient{base: strings.TrimRight(serverURL, "/"), http: &http.Client{}}
	switch {
	case cfg.Basic != "":
		c.authHeader = "Basic " + base64.StdEncoding.EncodeToString([]byte(cfg.Basic))
	case cfg.Bearer != "":
		c.authHeader = "Bearer " + cfg.Bearer
	}
	return c
}

// do issues a control-plane request and decodes a JSON response into out (which
// may be nil for 204). A non-2xx response is turned into the server's structured
// error (RFC-0016 §8.6 schema, reused by RFC-0019).
func (c *repoClient) do(method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+protocol.PathRepos+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set(protocol.HeaderProto, protocol.Version)
	req.Header.Set(protocol.HeaderWire, protocol.WireVersion)
	if body != nil {
		req.Header.Set("Content-Type", protocol.CTJSON)
	}
	if c.authHeader != "" {
		req.Header.Set("Authorization", c.authHeader)
	}
	resp, err := c.http.Do(req)
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

// RunRepo dispatches a `vara repo <sub> ...` invocation. The first positional
// after the subcommand is always the server base URL (e.g. http://host:8080).
func RunRepo(args []string, cfg RepoConfig) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vara repo <create|delete|rename|list|show> <server-url> [args]")
	}
	sub, rest := args[0], args[1:]
	if len(rest) == 0 {
		return fmt.Errorf("repo %s: missing server URL", sub)
	}
	serverURL, rest := rest[0], rest[1:]
	c := newRepoClient(serverURL, cfg)

	switch sub {
	case "list":
		var resp protocol.ListReposResponse
		if err := c.do(http.MethodGet, "", nil, &resp); err != nil {
			return err
		}
		if len(resp.Repositories) == 0 {
			fmt.Println("(no repositories)")
			return nil
		}
		for _, d := range resp.Repositories {
			fmt.Printf("%-24s %-9s owner=%s\n", d.Name, d.Visibility, d.Owner)
		}
		return nil

	case "create":
		if len(rest) < 1 {
			return fmt.Errorf("usage: vara repo create <server-url> <name> [--visibility private|public] [--description text]")
		}
		req := protocol.CreateRepoRequest{Name: rest[0]}
		for i := 1; i < len(rest); i++ {
			switch rest[i] {
			case "--visibility":
				if i+1 < len(rest) {
					req.Visibility = rest[i+1]
					i++
				}
			case "--description":
				if i+1 < len(rest) {
					req.Description = rest[i+1]
					i++
				}
			}
		}
		var d protocol.RepositoryDescriptor
		if err := c.do(http.MethodPost, "", req, &d); err != nil {
			return err
		}
		fmt.Printf("created %s (id=%s, owner=%s)\n", d.Name, d.ID, d.Owner)
		return nil

	case "show":
		if len(rest) < 1 {
			return fmt.Errorf("usage: vara repo show <server-url> <name>")
		}
		var d protocol.RepositoryDescriptor
		if err := c.do(http.MethodGet, "/"+rest[0], nil, &d); err != nil {
			return err
		}
		fmt.Printf("id:          %s\nname:        %s\nowner:       %s\nvisibility:  %s\nstate:       %s\ncreated_at:  %s\n",
			d.ID, d.Name, d.Owner, d.Visibility, d.State, d.CreatedAt)
		return nil

	case "delete":
		if len(rest) < 1 {
			return fmt.Errorf("usage: vara repo delete <server-url> <name>")
		}
		if err := c.do(http.MethodDelete, "/"+rest[0], nil, nil); err != nil {
			return err
		}
		fmt.Printf("deleted %s\n", rest[0])
		return nil

	case "rename":
		if len(rest) < 2 {
			return fmt.Errorf("usage: vara repo rename <server-url> <name> <new-name>")
		}
		var d protocol.RepositoryDescriptor
		if err := c.do(http.MethodPost, "/"+rest[0]+"/rename", protocol.RenameRepoRequest{NewName: rest[1]}, &d); err != nil {
			return err
		}
		fmt.Printf("renamed to %s (id=%s)\n", d.Name, d.ID)
		return nil

	default:
		return fmt.Errorf("repo: unknown subcommand %q (want create|delete|rename|list|show)", sub)
	}
}
