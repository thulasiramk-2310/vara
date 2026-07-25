// Package server is the HTTP binding of the VARA remote transport protocol
// (VARA-RFC-0016 §8/§9), with identity (RFC-0017) and authorization (RFC-0018)
// layered in the request preamble.
//
// The per-request pipeline is fixed (RFC-0018 A10):
//
//	Request → Authenticate → Authorize → Transport → Engine
//
// The server contains NO version-control logic and makes NO identity or
// authorization decision inside the engine. Authentication resolves an identity
// (or 401) before any Transport method; authorization decides allow/deny (or
// 403) before the transport is opened; only then does the handler delegate the
// operation to a Local transport, which alone touches the repository. Identity
// and policy are read only here in the binding — never below internal/transport
// (RFC-0017 C1, RFC-0018 A1).
//
// Layer: above internal/transport (and internal/identity, internal/authz),
// below internal/commands.
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/identity"
	"github.com/thulasiramk-2310/vara/internal/protocol"
	"github.com/thulasiramk-2310/vara/internal/repomanager"
	"github.com/thulasiramk-2310/vara/internal/transport"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

var zeroCommit types.CommitID

// Options configures a server's identity and authorization behavior. The zero
// Options yields an anonymous, allow-all server (RFC-0016 behavior).
type Options struct {
	// Identity resolves credentials. nil means anonymous (auth disabled).
	Identity identity.IdentitySource
	// Authz decides allow/deny. nil means authorization disabled (allow all).
	Authz *authz.Enforcer
	// Methods are the auth-* capability tokens advertised on info/refs and used
	// to build WWW-Authenticate (e.g. "auth-basic", "auth-bearer").
	Methods []string
	// Manager, when set, enables the RFC-0019 control plane (/_vara/repositories)
	// and metadata-gates data-plane serving (only Active repositories are served,
	// M10). nil leaves a bare RFC-0016 transport server that serves by directory
	// existence exactly as before.
	Manager *repomanager.Manager
	// Accounts, when set, enables the RFC-0020 control plane (/_vara/sessions,
	// /_vara/tokens, /_vara/accounts). The same manager should back the identity
	// sources in Identity so authentication and administration share one store.
	Accounts *identity.AccountManager
	// HubDir, when set, serves a same-origin static Hub UI from that directory at
	// any path not claimed by the API or data plane (RFC-0021 §8).
	HubDir string
}

// sessionCookie carries a browser session secret (RFC-0021 §7). It is httpOnly so
// page JavaScript can never read it.
const sessionCookie = "vara_session"

// credentialFrom extracts the caller's credential from the request: the
// Authorization header if present, else the session cookie (RFC-0021 §7 — a
// cookie is just another carrier for a bearer credential), else anonymous.
func credentialFrom(r *http.Request) (*identity.Credential, error) {
	if h := r.Header.Get("Authorization"); h != "" {
		return identity.ParseHeader(h)
	}
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		return &identity.Credential{Scheme: identity.SchemeBearer, Token: c.Value}, nil
	}
	return nil, nil // anonymous
}

// requestIsHTTPS reports whether the request reached us over TLS, directly or via
// a terminating proxy — used to mark the session cookie Secure without breaking
// plain-http local development.
func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// Server serves repositories rooted at a single directory.
type Server struct {
	root     string
	idsrc    identity.IdentitySource
	authz    *authz.Enforcer
	manager  *repomanager.Manager
	accounts *identity.AccountManager
	caps     []string
	methods  []string
}

// Handler returns an anonymous, allow-all handler (RFC-0016 behavior).
func Handler(root string) http.Handler {
	return HandlerWithOptions(root, Options{})
}

// HandlerWithOptions returns a handler with the given identity/authorization
// configuration (RFC-0016 §8.1 routes; RFC-0017/0018 preamble).
func HandlerWithOptions(root string, opts Options) http.Handler {
	idsrc := opts.Identity
	if idsrc == nil {
		idsrc = identity.AnonymousSource{}
	}
	caps := []string{protocol.CapReportStatus}
	caps = append(caps, opts.Methods...)
	if opts.Authz != nil {
		caps = append(caps, "authz-v1")
	}
	if opts.Manager != nil {
		caps = append(caps, "repo-management-v1")
	}
	if opts.Accounts != nil {
		caps = append(caps, "accounts-v1")
	}
	s := &Server{
		root:     root,
		idsrc:    idsrc,
		authz:    opts.Authz,
		manager:  opts.Manager,
		accounts: opts.Accounts,
		caps:     caps,
		methods:  opts.Methods,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{repo}"+protocol.PathInfoRefs, s.handleListRefs)
	mux.HandleFunc("POST /{repo}"+protocol.PathFetch, s.handleFetch)
	mux.HandleFunc("POST /{repo}"+protocol.PathReceive, s.handleReceive)

	// whoami is always available (read-only identity introspection, RFC-0020
	// §8.5): it reports the resolved identity and, for a ?repo, its capabilities.
	mux.HandleFunc("GET "+protocol.PathWhoami, s.handleWhoami)

	// RFC-0021 Hub read API — always available (read-only content projection).
	// Each requires the `read` capability, checked before any content is read.
	mux.HandleFunc("GET "+protocol.PathRepos+"/{repo}/summary", s.handleRepoSummary)
	mux.HandleFunc("GET "+protocol.PathRepos+"/{repo}/branches", s.handleRepoBranches)
	mux.HandleFunc("GET "+protocol.PathRepos+"/{repo}/commits", s.handleRepoCommits)
	mux.HandleFunc("GET "+protocol.PathRepos+"/{repo}/commits/{id}", s.handleRepoCommit)

	// RFC-0022 browse API — tree/blob/raw content reads + a path filter on the
	// commits endpoint above. The {path...} wildcards are scoped under the literal
	// tree/blob/raw prefixes, so the fixed segments above (summary/branches/commits)
	// win by ServeMux most-specific-wins routing; the catch-alls never shadow them.
	mux.HandleFunc("GET "+protocol.PathRepos+"/{repo}/tree/{path...}", s.handleRepoTree)
	mux.HandleFunc("GET "+protocol.PathRepos+"/{repo}/blob/{path...}", s.handleRepoBlob)
	mux.HandleFunc("GET "+protocol.PathRepos+"/{repo}/raw/{path...}", s.handleRepoRaw)

	// RFC-0023 diff API — a changed-file summary, a per-file unified diff, and a
	// commit-diff convenience (base = first parent). Additive, same `read` cap.
	mux.HandleFunc("GET "+protocol.PathRepos+"/{repo}/diff", s.handleRepoDiffSummary)
	mux.HandleFunc("GET "+protocol.PathRepos+"/{repo}/diff/{path...}", s.handleRepoFileDiff)
	mux.HandleFunc("GET "+protocol.PathRepos+"/{repo}/commits/{id}/diff", s.handleRepoCommitDiff)

	// RFC-0019 control plane, only when a manager is configured. Routes live
	// under the reserved /_vara/ prefix so they never collide with the
	// data-plane /{repo}/... routes above.
	if s.manager != nil {
		mux.HandleFunc("GET "+protocol.PathRepos, s.handleListRepos)
		mux.HandleFunc("POST "+protocol.PathRepos, s.handleCreateRepo)
		mux.HandleFunc("GET "+protocol.PathRepos+"/{repo}", s.handleGetRepo)
		mux.HandleFunc("DELETE "+protocol.PathRepos+"/{repo}", s.handleDeleteRepo)
		mux.HandleFunc("POST "+protocol.PathRepos+"/{repo}/rename", s.handleRenameRepo)
		// Reserved v1 routes (RFC-0019 §7.4, §5.4): present but not implemented.
		mux.HandleFunc("PUT "+protocol.PathRepos+"/{repo}/policy", s.handleReserved)
		mux.HandleFunc("POST "+protocol.PathRepos+"/{repo}/archive", s.handleReserved)
	}

	// RFC-0020 account/session/token control plane, only when accounts are
	// configured. Under the same reserved /_vara/ prefix.
	if s.accounts != nil {
		mux.HandleFunc("POST "+protocol.PathSessions, s.handleLogin)
		mux.HandleFunc("DELETE "+protocol.PathSessions+"/current", s.handleLogout)
		mux.HandleFunc("POST "+protocol.PathTokens, s.handleCreateToken)
		mux.HandleFunc("GET "+protocol.PathTokens, s.handleListTokens)
		mux.HandleFunc("DELETE "+protocol.PathTokens+"/{id}", s.handleRevokeToken)
		mux.HandleFunc("POST "+protocol.PathAccounts, s.handleCreateAccount)
		mux.HandleFunc("DELETE "+protocol.PathAccounts+"/{username}", s.handleDeleteAccount)
		mux.HandleFunc("POST "+protocol.PathAccounts+"/{username}/disable", s.handleDisableAccount)
		mux.HandleFunc("PUT "+protocol.PathAccounts+"/{username}/password", s.handleSetPassword)
	}

	// RFC-0021 §8: serve the same-origin static Hub UI as a STRICT fallback — the
	// "/" pattern is the least specific, so every API and data-plane route above
	// wins; only otherwise-unmatched paths reach the static handler (H3).
	if opts.HubDir != "" {
		mux.Handle("GET /", staticHandler(opts.HubDir))
	}
	return mux
}

// handleListRefs: GET /:repo/info/refs (RFC-0016 §5.1). Requires `read`.
func (s *Server) handleListRefs(w http.ResponseWriter, r *http.Request) {
	id, repo, ok := s.begin(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, id, authz.CapRead, repo) {
		return
	}
	tr, ok := s.openRepo(w, repo)
	if !ok {
		return
	}
	defer tr.Close()

	advs, err := tr.ListRefs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, err.Error())
		return
	}
	head, _ := tr.HeadTarget()
	resp := protocol.ListRefsResponse{Head: head, Caps: s.caps}
	for _, a := range advs {
		resp.Refs = append(resp.Refs, protocol.Ref{Name: a.Name, Target: a.Target.String()})
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleFetch: POST /:repo/fetch (RFC-0016 §5.2). Requires `read`.
func (s *Server) handleFetch(w http.ResponseWriter, r *http.Request) {
	id, repo, ok := s.begin(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, id, authz.CapRead, repo) {
		return
	}
	tr, ok := s.openRepo(w, repo)
	if !ok {
		return
	}
	defer tr.Close()

	var req protocol.FetchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "decode fetch request: "+err.Error())
		return
	}
	wants, err := parseCommits(req.Wants)
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "bad want: "+err.Error())
		return
	}
	haves, err := parseCommits(req.Haves)
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "bad have: "+err.Error())
		return
	}

	stream, err := tr.FetchPack(wants, haves)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, "fetch pack: "+err.Error())
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", protocol.CTPack)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, stream)
}

// handleReceive: POST /:repo/receive (RFC-0016 §5.3). Authorizes EVERY ref
// update's required capability before the transport is opened, so a denied push
// never reaches Local (RFC-0018 A2, §6.1: request-level, all-or-nothing).
func (s *Server) handleReceive(w http.ResponseWriter, r *http.Request) {
	id, repo, ok := s.begin(w, r)
	if !ok {
		return
	}

	mr, err := multipartReader(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, err.Error())
		return
	}

	// Part 1: the JSON update list — needed to know which capabilities the
	// request requires, before touching the transport.
	p1, err := mr.NextPart()
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "missing updates part")
		return
	}
	var rr protocol.ReceiveRequest
	if err := json.NewDecoder(p1).Decode(&rr); err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "decode updates: "+err.Error())
		return
	}
	updates, err := toRefUpdates(rr.Updates)
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, err.Error())
		return
	}

	// Authorize each update from its shape alone (RFC-0018 §8.1). A single
	// missing capability denies the whole request before the transport opens.
	for _, u := range updates {
		cap := authz.RequiredForUpdate(u.Old == zeroCommit, u.New == zeroCommit, u.Force)
		if !s.authorize(w, id, cap, repo) {
			return
		}
	}

	tr, ok := s.openRepo(w, repo)
	if !ok {
		return
	}
	defer tr.Close()

	// Part 2: the raw VPCK stream, handed straight to the transport.
	p2, err := mr.NextPart()
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "missing pack part")
		return
	}

	results, err := tr.ReceivePack(p2, updates)
	if err != nil {
		if strings.Contains(err.Error(), "ingest pack") {
			writeError(w, http.StatusUnprocessableEntity, protocol.CodeInvalidPack, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, protocol.CodeLockTimeout, err.Error())
		}
		return
	}

	resp := protocol.ReceiveResponse{OK: true}
	for _, res := range results {
		if !res.OK {
			resp.OK = false
		}
		resp.Results = append(resp.Results, protocol.Result{
			Name:   res.Name,
			OK:     res.OK,
			Code:   protocol.CodeForReason(res.OK, res.Reason),
			Reason: res.Reason,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// authn runs the identity portion of the request preamble: echo headers, check
// the protocol version, and AUTHENTICATE. It returns the resolved identity, or
// writes an error (426/401) and returns ok=false. It consults no repository, so
// both the data plane (per-{repo} routes) and the control plane (which may have
// no {repo}, e.g. list/create) share one authentication path (RFC-0018 A10).
func (s *Server) authn(w http.ResponseWriter, r *http.Request) (identity.Identity, bool) {
	echoHeaders(w, r)

	if v := r.Header.Get(protocol.HeaderProto); v != "" && protocol.Major(v) != protocol.Major(protocol.Version) {
		writeError(w, http.StatusUpgradeRequired, protocol.CodeUpgrade, "unsupported protocol version "+v)
		return identity.Identity{}, false
	}

	// Parse precedes authenticate (RFC-0017 §6.2): a malformed header is a 401
	// that never reaches the identity source. The credential may arrive as an
	// Authorization header (CLI) or a session cookie (browser, RFC-0021 §7).
	cred, err := credentialFrom(r)
	if err != nil {
		s.writeUnauthenticated(w, err)
		return identity.Identity{}, false
	}
	id, err := s.idsrc.Authenticate(cred)
	if err != nil {
		s.writeUnauthenticated(w, err)
		return identity.Identity{}, false
	}
	return id, true
}

// begin is authn plus data-plane repository-name validation. It returns the
// resolved identity and repo name, or writes an error (426/400/401) and returns
// ok=false. Authentication happens before any transport is opened (RFC-0018 A10).
func (s *Server) begin(w http.ResponseWriter, r *http.Request) (identity.Identity, string, bool) {
	id, ok := s.authn(w, r)
	if !ok {
		return identity.Identity{}, "", false
	}
	repo := r.PathValue("repo")
	if !validRepo(repo) {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "invalid repository name")
		return identity.Identity{}, "", false
	}
	return id, repo, true
}

// authorize enforces that id holds cap on repo (RFC-0018). With no enforcer
// configured, authorization is disabled and everything is allowed. On denial it
// writes 403 and returns false. Runs before the transport is opened (A2).
func (s *Server) authorize(w http.ResponseWriter, id identity.Identity, cap authz.Capability, repo string) bool {
	if s.authz == nil {
		return true
	}
	if err := s.authz.Authorize(id.ID, cap, repo); err != nil {
		writeError(w, http.StatusForbidden, protocol.CodeUnauthorized, err.Error())
		return false
	}
	return true
}

// openRepo opens the Local transport for repo, or writes 404. When a manager is
// configured, serving is metadata-gated (RFC-0019 M10): only an Active repository
// is served, so a Creating/Deleting/Archived repository is never mistaken for a
// live one. With no manager, behavior is the bare RFC-0016 directory check.
func (s *Server) openRepo(w http.ResponseWriter, repo string) (transport.Transport, bool) {
	if s.manager != nil && !s.manager.Servable(repo) {
		writeError(w, http.StatusNotFound, protocol.CodeUnknownRepo, "no repository "+repo)
		return nil, false
	}
	tr, err := transport.OpenLocal(filepath.Join(s.root, repo))
	if err != nil {
		writeError(w, http.StatusNotFound, protocol.CodeUnknownRepo, "no repository "+repo)
		return nil, false
	}
	return tr, true
}

// writeUnauthenticated writes 401 with a WWW-Authenticate header (RFC-0017 §8).
func (s *Server) writeUnauthenticated(w http.ResponseWriter, err error) {
	if wa := s.wwwAuthenticate(); wa != "" {
		w.Header().Set("WWW-Authenticate", wa)
	}
	writeError(w, http.StatusUnauthorized, protocol.CodeUnauthenticated, err.Error())
}

// wwwAuthenticate builds the WWW-Authenticate value from the advertised methods.
func (s *Server) wwwAuthenticate() string {
	var schemes []string
	for _, m := range s.methods {
		switch m {
		case "auth-basic":
			schemes = append(schemes, `Basic realm="VARA"`)
		case "auth-bearer":
			schemes = append(schemes, "Bearer")
		}
	}
	return strings.Join(schemes, ", ")
}

// echoHeaders sets the protocol/wire version headers and echoes the client's
// transaction ID verbatim (RFC-0016 §8.3.1).
func echoHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(protocol.HeaderProto, protocol.Version)
	w.Header().Set(protocol.HeaderWire, protocol.WireVersion)
	if txn := r.Header.Get(protocol.HeaderTxn); txn != "" {
		w.Header().Set(protocol.HeaderTxn, txn)
	}
}

// validRepo rejects empty names and path traversal (RFC-0016 §8.1).
func validRepo(repo string) bool {
	if repo == "" || repo == "." || repo == ".." {
		return false
	}
	if strings.ContainsAny(repo, `/\`) || strings.Contains(repo, "..") {
		return false
	}
	return true
}

func multipartReader(r *http.Request) (*multipart.Reader, error) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("parse content-type: %w", err)
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		return nil, fmt.Errorf("expected multipart body, got %q", mediaType)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, fmt.Errorf("missing multipart boundary")
	}
	return multipart.NewReader(r.Body, boundary), nil
}

func parseCommits(hexes []string) ([]types.CommitID, error) {
	out := make([]types.CommitID, 0, len(hexes))
	for _, h := range hexes {
		c, err := protocol.ParseCommit(h)
		if err != nil {
			return nil, fmt.Errorf("bad commit id %q: %w", h, err)
		}
		out = append(out, c)
	}
	return out, nil
}

func toRefUpdates(us []protocol.Update) ([]transport.RefUpdate, error) {
	out := make([]transport.RefUpdate, 0, len(us))
	for _, u := range us {
		oldID, err := protocol.ParseCommit(u.Old)
		if err != nil {
			return nil, fmt.Errorf("update %s: bad old id: %w", u.Name, err)
		}
		newID, err := protocol.ParseCommit(u.New)
		if err != nil {
			return nil, fmt.Errorf("update %s: bad new id: %w", u.Name, err)
		}
		out = append(out, transport.RefUpdate{Name: u.Name, Old: oldID, New: newID, Force: u.Force})
	}
	return out, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", protocol.CTJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, protocol.Error{OK: false, Code: code, Message: msg})
}
