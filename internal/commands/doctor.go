// Package commands — vara doctor.
//
// RunDoctor is a fast, friendly health check across everything a user might need
// working: the local repository (format, HEAD, object store, refs, index, commit
// graph), configuration, and remote connectivity/authentication/authorization.
// It writes no state and calls only existing subsystems — it is a read-only
// diagnostic, the first thing to run when something looks wrong.
package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/protocol"
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/internal/transport"
	"github.com/thulasiramk-2310/vara/pkg/color"
	"github.com/thulasiramk-2310/vara/pkg/config"
	"github.com/thulasiramk-2310/vara/pkg/graphindex"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// errDoctorIssues signals that at least one check FAILED, so the process exits
// non-zero. The report is already printed; the caller should exit quietly.
var errDoctorIssues = errors.New("doctor: checks failed")

type dxStatus int

const (
	dxOK dxStatus = iota
	dxWarn
	dxFail
	dxInfo
)

type doctor struct {
	fails int
	warns int
}

func (d *doctor) section(title string) {
	fmt.Printf("\n%s\n", color.Bold(title))
}

func (d *doctor) line(status dxStatus, label, detail, hint string) {
	var mark string
	switch status {
	case dxOK:
		mark = color.Green("✓")
	case dxWarn:
		mark = color.Yellow("!")
		d.warns++
	case dxFail:
		mark = color.Red("✗")
		d.fails++
	default:
		mark = color.Cyan("·")
	}
	fmt.Printf("  %s %-20s %s\n", mark, label, detail)
	if hint != "" {
		fmt.Printf("      %s\n", color.Dim("→ "+hint))
	}
}

// RunDoctor runs the diagnostic. An optional server URL positional focuses the
// remote checks on that URL (with --basic/--bearer); otherwise configured
// remotes are probed. It returns errDoctorIssues if any check failed.
func RunDoctor(args []string, version string, cfg AuthConfig) error {
	var url string
	if len(args) > 0 {
		url = args[0]
	}
	d := &doctor{}

	fmt.Println(color.Bold("vara doctor"))
	d.section("Environment")
	d.line(dxInfo, "vara version", version, "")
	d.line(dxInfo, "protocol", protocol.Version+" (wire "+protocol.WireVersion+")", "")

	repo, err := repository.Discover(".")
	if err != nil {
		d.line(dxInfo, "repository", "not inside a VARA repository", "run 'vara init' to create one, or pass a server URL")
	} else {
		d.localChecks(repo)
		d.configAndRemotes(repo, url, cfg)
	}

	if url != "" {
		d.checkRemote(url, cfg)
	}

	return d.summary()
}

func (d *doctor) localChecks(repo *repository.Repository) {
	d.section("Repository")
	d.line(dxOK, "location", repo.RootDir, "")

	// Format version.
	if vb, err := os.ReadFile(filepath.Join(repo.VaraDir, repository.VersionFile)); err != nil {
		d.line(dxFail, "format version", "unreadable: "+err.Error(), "the repository may be corrupt")
	} else if v := strings.TrimSpace(string(vb)); v == "1" {
		d.line(dxOK, "format version", v, "")
	} else {
		d.line(dxWarn, "format version", v+" (this vara expects 1)", "vara and repo versions differ")
	}

	// HEAD.
	rr := refs.NewFSResolver(repo.VaraDir)
	var head types.CommitID
	haveHead := false
	if branch, err := rr.ResolveSymbolic("HEAD"); err != nil {
		d.line(dxWarn, "HEAD", "unresolved: "+err.Error(), "")
	} else if head, err = rr.Resolve(branch); err != nil {
		d.line(dxWarn, "HEAD", branch+" (unborn — no commits yet)", "make your first commit with 'vara commit'")
	} else {
		haveHead = true
		d.line(dxOK, "HEAD", branch+" @ "+head.String()[:12], "")
	}

	// Object store.
	store := object.NewStore(repo.VaraDir)
	n := countLooseObjects(repo.VaraDir)
	if haveHead {
		if _, err := store.Read(types.ObjectID(head)); err != nil {
			d.line(dxFail, "object store", "HEAD commit unreadable: "+err.Error(), "run 'vara verify' for a deep check")
		} else {
			d.line(dxOK, "object store", fmt.Sprintf("%d loose objects; HEAD readable", n), "")
		}
	} else {
		d.line(dxOK, "object store", fmt.Sprintf("%d loose objects", n), "")
	}

	// Refs.
	if list, err := rr.List(); err != nil {
		d.line(dxFail, "refs", err.Error(), "")
	} else {
		refCount, broken := 0, 0
		for _, r := range list {
			if r.IsSymbolic() {
				continue
			}
			refCount++
			if _, e := rr.Resolve(r.Name); e != nil {
				broken++
			}
		}
		if broken > 0 {
			d.line(dxFail, "refs", fmt.Sprintf("%d refs, %d unresolvable", refCount, broken), "run 'vara verify'")
		} else {
			d.line(dxOK, "refs", fmt.Sprintf("%d ref(s)", refCount), "")
		}
	}

	// Index.
	idxData, err := os.ReadFile(filepath.Join(repo.VaraDir, "index"))
	switch {
	case os.IsNotExist(err):
		d.line(dxInfo, "index", "none (nothing staged)", "")
	case err != nil:
		d.line(dxFail, "index", err.Error(), "")
	case len(idxData) == 0:
		d.line(dxInfo, "index", "empty", "")
	default:
		if idx, e := index.Deserialize(idxData); e != nil {
			d.line(dxFail, "index", "corrupt: "+e.Error(), "re-stage with 'vara add'")
		} else {
			d.line(dxOK, "index", fmt.Sprintf("%d staged entrie(s)", len(idx.Entries)), "")
		}
	}

	// Commit graph index (do not build during a diagnostic).
	if _, err := graphindex.Load(repo.VaraDir); err != nil {
		d.line(dxInfo, "commit graph index", "not built", "built automatically on next 'vara history'")
	} else {
		d.line(dxOK, "commit graph index", "present", "")
	}

	d.line(dxInfo, "deep integrity", "not checked here", "run 'vara verify' for a full object/DAG audit")
}

func (d *doctor) configAndRemotes(repo *repository.Repository, url string, cfg AuthConfig) {
	d.section("Configuration")
	conf, err := config.Load(filepath.Join(repo.VaraDir, "config"))
	if err != nil {
		d.line(dxWarn, "config", "unreadable: "+err.Error(), "")
		return
	}
	d.line(dxOK, "config", "readable", "")
	remotes := conf.Remotes()
	if len(remotes) == 0 {
		d.line(dxInfo, "remotes", "none configured", "add one with 'vara remote add <name> <url>'")
		return
	}
	for _, r := range remotes {
		d.line(dxInfo, "remote "+r.Name, r.URL, "")
	}
	// With no explicit URL, probe each configured remote's reachability.
	if url == "" {
		for _, r := range remotes {
			d.checkRemote(r.URL, AuthConfig{})
		}
	}
}

func (d *doctor) checkRemote(url string, cfg AuthConfig) {
	d.section("Remote: " + url)
	tr, err := openForDoctor(url, cfg)
	if err != nil {
		d.line(dxFail, "open", err.Error(), "check the URL")
		return
	}
	defer tr.Close()

	list, err := tr.ListRefs()
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "UPGRADE_REQUIRED"):
			d.line(dxFail, "protocol", "incompatible version", "upgrade vara or the server")
		case strings.Contains(msg, "UNAUTHENTICATED"):
			d.line(dxWarn, "authentication", "reachable; credentials required", "pass --basic user:secret or --bearer <token>")
		case strings.Contains(msg, "UNAUTHORIZED"):
			d.line(dxWarn, "authorization", "authenticated; read denied", "ask an admin to grant 'read' on this repository")
		default:
			d.line(dxFail, "reachability", "unreachable: "+msg, "check the URL and that 'vara serve' is running")
		}
		return
	}
	d.line(dxOK, "reachability", fmt.Sprintf("reachable; %d ref(s); read OK", len(list)), "")
}

func (d *doctor) summary() error {
	d.section("Summary")
	switch {
	case d.fails == 0 && d.warns == 0:
		fmt.Println("  " + color.Green("all checks passed"))
	case d.fails == 0:
		fmt.Printf("  %s\n", color.Yellow(fmt.Sprintf("%d warning(s), no failures", d.warns)))
	default:
		fmt.Printf("  %s\n", color.Red(fmt.Sprintf("%d failure(s), %d warning(s)", d.fails, d.warns)))
	}
	if d.fails > 0 {
		return errDoctorIssues
	}
	return nil
}

func openForDoctor(url string, cfg AuthConfig) (transport.Transport, error) {
	if isRemoteURL(url) {
		ht, err := transport.OpenHTTP(url)
		if err != nil {
			return nil, err
		}
		if user, secret, ok := strings.Cut(cfg.Basic, ":"); ok && cfg.Basic != "" {
			ht.SetBasicAuth(user, secret)
		} else if cfg.Bearer != "" {
			ht.SetBearerToken(cfg.Bearer)
		}
		return ht, nil
	}
	return transport.Open(url)
}

func isRemoteURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "vara://")
}

func countLooseObjects(varaDir string) int {
	entries, err := os.ReadDir(varaDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) != 2 || !isHex2(e.Name()) {
			continue
		}
		if sub, err := os.ReadDir(filepath.Join(varaDir, e.Name())); err == nil {
			n += len(sub)
		}
	}
	return n
}

func isHex2(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
