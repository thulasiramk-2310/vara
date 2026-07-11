// Package server is the HTTP binding of the VARA remote transport protocol
// (VARA-RFC-0016 §8/§9). It exposes an http.Handler that dispatches each
// request to a Local transport (internal/transport) bound to the requested
// repository.
//
// The server contains NO version-control logic. It is a codec: decode a
// request, call the matching Local method (ListRefs / FetchPack / ReceivePack),
// encode the result. In particular the concurrency contract (RFC-0016 §7) is
// satisfied by calling Local.ReceivePack — which holds the Refs lock across the
// compare-and-swap — never by reimplementing ref updates here (Single
// Implementation Principle, RFC-0016 §9.1). The server never touches a working
// tree (RFC-0016 §9.2).
//
// Layer: above internal/transport, below internal/commands.
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

	"github.com/thulasiramk-2310/vara/internal/protocol"
	"github.com/thulasiramk-2310/vara/internal/transport"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Server serves repositories rooted at a single directory. Each request's
// {repo} path segment names a subdirectory that must be a VARA repository.
type Server struct {
	root string
	caps []string
}

// Handler returns an http.Handler serving the repositories under root
// (RFC-0016 §8.1 routes).
func Handler(root string) http.Handler {
	s := &Server{
		root: root,
		caps: []string{protocol.CapReportStatus},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+"/{repo}"+protocol.PathInfoRefs, s.handleListRefs)
	mux.HandleFunc("POST "+"/{repo}"+protocol.PathFetch, s.handleFetch)
	mux.HandleFunc("POST "+"/{repo}"+protocol.PathReceive, s.handleReceive)
	return mux
}

// handleListRefs: GET /:repo/info/refs (RFC-0016 §5.1).
func (s *Server) handleListRefs(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.open(w, r)
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

// handleFetch: POST /:repo/fetch (RFC-0016 §5.2). Read-only; streams a VPCK body.
func (s *Server) handleFetch(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.open(w, r)
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

// handleReceive: POST /:repo/receive (RFC-0016 §5.3). Ingests the pack and
// applies ref updates by delegating to Local.ReceivePack (§7, §9.1).
func (s *Server) handleReceive(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.open(w, r)
	if !ok {
		return
	}
	defer tr.Close()

	mr, err := multipartReader(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, err.Error())
		return
	}

	// Part 1: the JSON update list.
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

	// Part 2: the raw VPCK stream, handed straight to the transport.
	p2, err := mr.NextPart()
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "missing pack part")
		return
	}

	results, err := tr.ReceivePack(p2, updates)
	if err != nil {
		// A pack that fails integrity is a request-level error (§8.5); anything
		// else (lock acquisition, I/O) is a server fault. Neither advances a ref.
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

// open validates the protocol version and repository name, echoes the reserved
// headers, and opens the Local transport. On any failure it writes the
// structured error and returns ok=false.
func (s *Server) open(w http.ResponseWriter, r *http.Request) (transport.Transport, bool) {
	echoHeaders(w, r)

	if v := r.Header.Get(protocol.HeaderProto); v != "" && protocol.Major(v) != protocol.Major(protocol.Version) {
		writeError(w, http.StatusUpgradeRequired, protocol.CodeUpgrade, "unsupported protocol version "+v)
		return nil, false
	}

	repo := r.PathValue("repo")
	if !validRepo(repo) {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "invalid repository name")
		return nil, false
	}

	tr, err := transport.OpenLocal(filepath.Join(s.root, repo))
	if err != nil {
		writeError(w, http.StatusNotFound, protocol.CodeUnknownRepo, "no repository "+repo)
		return nil, false
	}
	return tr, true
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

// validRepo rejects empty names and path traversal (RFC-0016 §8.1). The route
// pattern already restricts {repo} to a single segment (no separators), so this
// guards the remaining dangerous names.
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
