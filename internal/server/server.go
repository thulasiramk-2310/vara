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
}

// Server serves repositories rooted at a single directory.
type Server struct {
	root    string
	idsrc   identity.IdentitySource
	authz   *authz.Enforcer
	caps    []string
	methods []string
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
	s := &Server{
		root:    root,
		idsrc:   idsrc,
		authz:   opts.Authz,
		caps:    caps,
		methods: opts.Methods,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{repo}"+protocol.PathInfoRefs, s.handleListRefs)
	mux.HandleFunc("POST /{repo}"+protocol.PathFetch, s.handleFetch)
	mux.HandleFunc("POST /{repo}"+protocol.PathReceive, s.handleReceive)
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

// begin runs the request preamble: echo headers, check the protocol version,
// validate the repository name, and AUTHENTICATE. It returns the resolved
// identity and repo name, or writes an error (426/400/401) and returns ok=false.
// Authentication happens here, before any transport is opened (RFC-0018 A10).
func (s *Server) begin(w http.ResponseWriter, r *http.Request) (identity.Identity, string, bool) {
	echoHeaders(w, r)

	if v := r.Header.Get(protocol.HeaderProto); v != "" && protocol.Major(v) != protocol.Major(protocol.Version) {
		writeError(w, http.StatusUpgradeRequired, protocol.CodeUpgrade, "unsupported protocol version "+v)
		return identity.Identity{}, "", false
	}

	repo := r.PathValue("repo")
	if !validRepo(repo) {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "invalid repository name")
		return identity.Identity{}, "", false
	}

	// Parse precedes authenticate (RFC-0017 §6.2): a malformed header is a 401
	// that never reaches the identity source.
	cred, err := identity.ParseHeader(r.Header.Get("Authorization"))
	if err != nil {
		s.writeUnauthenticated(w, err)
		return identity.Identity{}, "", false
	}
	id, err := s.idsrc.Authenticate(cred)
	if err != nil {
		s.writeUnauthenticated(w, err)
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

// openRepo opens the Local transport for repo, or writes 404.
func (s *Server) openRepo(w http.ResponseWriter, repo string) (transport.Transport, bool) {
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
