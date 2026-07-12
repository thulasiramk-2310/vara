package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/hub"
	"github.com/thulasiramk-2310/vara/internal/protocol"
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// This file is the RFC-0021 Hub read API: GET endpoints that project repository
// content (summary/branches/commits/commit) as JSON. Each authenticates, checks
// the RFC-0018 `read` capability BEFORE reading any content (H1), delegates all
// reading to the internal/hub projection layer (H2/H8 — the server never walks
// the graph or parses objects itself), and returns a strong ETag with
// X-VARA-API/X-Request-ID headers. Errors reuse the transport schema.

const (
	defaultCommitLimit = 50
	maxCommitLimit     = 100
)

// handleRepoSummary: GET /_vara/repositories/{repo}/summary.
func (s *Server) handleRepoSummary(w http.ResponseWriter, r *http.Request) {
	h, repo, ok := s.beginHubRead(w, r)
	if !ok {
		return
	}
	sum, err := h.Summary()
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, err.Error())
		return
	}
	resp := protocol.RepositorySummary{
		Name:          repo,
		DefaultBranch: sum.DefaultBranch,
		Head:          hexOrEmpty(sum.Head),
		CommitCount:   sum.CommitCount,
		BranchCount:   sum.BranchCount,
	}
	// The immutable repository id lives in RFC-0019 metadata, not the engine the
	// hub projects; the server composes it in when a manager is configured.
	if s.manager != nil {
		if md, err := s.manager.Get(repo); err == nil {
			resp.ID = md.ID
		}
	}
	if sum.Last != nil {
		cs := commitSummaryOf(*sum.Last)
		resp.LastCommit = &cs
	}
	s.writeHub(w, r, resp, etag(repo, "summary", resp.Head, strconv.Itoa(resp.CommitCount), strconv.Itoa(resp.BranchCount)))
}

// handleRepoBranches: GET /_vara/repositories/{repo}/branches.
func (s *Server) handleRepoBranches(w http.ResponseWriter, r *http.Request) {
	h, repo, ok := s.beginHubRead(w, r)
	if !ok {
		return
	}
	branches, err := h.Branches()
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, err.Error())
		return
	}
	resp := protocol.BranchesResponse{Branches: make([]protocol.BranchInfo, 0, len(branches))}
	tag := []string{repo, "branches"}
	for _, b := range branches {
		resp.Branches = append(resp.Branches, protocol.BranchInfo{
			Name: b.Name, Target: b.Target.String(), IsHead: b.IsHead,
		})
		tag = append(tag, b.Name, b.Target.String())
	}
	s.writeHub(w, r, resp, etag(tag...))
}

// handleRepoCommits: GET /_vara/repositories/{repo}/commits?ref=&limit=&before=.
func (s *Server) handleRepoCommits(w http.ResponseWriter, r *http.Request) {
	h, repo, ok := s.beginHubRead(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit := defaultCommitLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxCommitLimit {
			writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "limit must be 1..100")
			return
		}
		limit = n
	}
	var before types.CommitID
	if v := q.Get("before"); v != "" {
		c, err := protocol.ParseCommit(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "bad cursor")
			return
		}
		before = c
	}

	commits, next, err := h.Commits(q.Get("ref"), limit, before)
	switch err {
	case nil:
	case hub.ErrNotFound:
		writeError(w, http.StatusNotFound, protocol.CodeUnknownRepo, "no such ref")
		return
	case hub.ErrBadCursor:
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "cursor not reachable")
		return
	default:
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, err.Error())
		return
	}

	resp := protocol.CommitsResponse{Commits: make([]protocol.CommitSummary, 0, len(commits)), Next: hexOrEmpty(next)}
	tag := []string{repo, "commits", q.Get("ref"), resp.Next}
	for _, c := range commits {
		cs := commitSummaryOf(c)
		resp.Commits = append(resp.Commits, cs)
		tag = append(tag, cs.ID)
	}
	s.writeHub(w, r, resp, etag(tag...))
}

// handleRepoCommit: GET /_vara/repositories/{repo}/commits/{id}.
func (s *Server) handleRepoCommit(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.beginHubRead(w, r)
	if !ok {
		return
	}
	id, err := protocol.ParseCommit(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "bad commit id")
		return
	}
	detail, err := h.Commit(id)
	if err != nil {
		writeError(w, http.StatusNotFound, protocol.CodeNotFound, "no such commit")
		return
	}
	resp := protocol.CommitDetail{CommitSummary: commitSummaryOf(detail.Commit), Tree: detail.Tree.String()}
	// A commit id content-addresses its whole content, so the id IS its ETag.
	s.writeHub(w, r, resp, etag(resp.ID))
}

// beginHubRead runs the shared preamble for every read endpoint: authenticate,
// authorize `read` BEFORE touching content (H1), confirm the repository is served,
// and open the projection view. It returns the view and repo name, or writes an
// error and returns ok=false.
func (s *Server) beginHubRead(w http.ResponseWriter, r *http.Request) (*hub.Repo, string, bool) {
	id, ok := s.authn(w, r)
	if !ok {
		return nil, "", false
	}
	repo := r.PathValue("repo")
	if !validRepo(repo) {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "invalid repository name")
		return nil, "", false
	}
	if !s.authorize(w, id, authz.CapRead, repo) {
		return nil, "", false
	}
	// Metadata-gate when a manager is configured (only Active repos), else check
	// the .vara directory exists.
	if s.manager != nil && !s.manager.Servable(repo) {
		writeError(w, http.StatusNotFound, protocol.CodeUnknownRepo, "no repository "+repo)
		return nil, "", false
	}
	vd := filepath.Join(s.root, repo, repository.VaraDir)
	if fi, err := os.Stat(vd); err != nil || !fi.IsDir() {
		writeError(w, http.StatusNotFound, protocol.CodeUnknownRepo, "no repository "+repo)
		return nil, "", false
	}
	return hub.Open(vd), repo, true
}

// writeHub writes a 200 JSON body with the Hub API headers and ETag, honoring a
// conditional If-None-Match (304). Read responses are cacheable; a matching ETag
// re-validates for free (RFC-0021 §5.6).
func (s *Server) writeHub(w http.ResponseWriter, r *http.Request, v any, tag string) {
	w.Header().Set(protocol.HeaderAPI, protocol.APIVersion)
	reqID := r.Header.Get(protocol.HeaderRequestID)
	if reqID == "" {
		reqID = protocol.NewTxnID()
	}
	w.Header().Set(protocol.HeaderRequestID, reqID)
	w.Header().Set("ETag", tag)
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// etag builds a strong ETag from the parts that determine the response content.
func etag(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return `"` + hex.EncodeToString(h.Sum(nil))[:16] + `"`
}

func commitSummaryOf(c hub.Commit) protocol.CommitSummary {
	parents := make([]string, len(c.Parents))
	for i, p := range c.Parents {
		parents[i] = p.String()
	}
	return protocol.CommitSummary{
		ID:        c.ID.String(),
		Message:   c.Message,
		Author:    c.Author,
		Timestamp: time.Unix(c.Timestamp, 0).UTC().Format(time.RFC3339),
		Parents:   parents,
	}
}

func hexOrEmpty(id types.CommitID) string {
	if (id == types.CommitID{}) {
		return ""
	}
	return id.String()
}
