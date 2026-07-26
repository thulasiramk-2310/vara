package server

// This file is the RFC-0024 search API: GET search/commits, search/paths, and
// search/content. Each reuses the RFC-0021 read preamble (beginHubRead:
// authenticate → authorize `read` before any object is read → open the
// projection), delegates all scanning to internal/hub, and returns a
// content-addressed ETag. Search results are inert JSON data (§7/S4): matched
// messages, paths, and code lines are string fields the UI renders, never a
// document, and a binary blob is never scanned or emitted.

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/hub"
	"github.com/thulasiramk-2310/vara/internal/protocol"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

const (
	defaultSearchLimit = 50
	maxSearchLimit     = 200
)

// parseCase reads the ?case= parameter (RFC-0024 §6): "" or "insensitive" →
// case-insensitive (default), "sensitive" → exact. Any other value is invalid.
func parseCase(v string) (caseSensitive, ok bool) {
	switch v {
	case "", "insensitive":
		return false, true
	case "sensitive":
		return true, true
	default:
		return false, false
	}
}

// parseSearchLimit reads a path/content ?limit= (1..maxSearchLimit, default
// defaultSearchLimit).
func parseSearchLimit(v string) (int, bool) {
	if v == "" {
		return defaultSearchLimit, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > maxSearchLimit {
		return 0, false
	}
	return n, true
}

// handleRepoSearchCommits: GET .../search/commits?q=&ref=&in=&limit=&before=&case=.
func (s *Server) handleRepoSearchCommits(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.beginHubRead(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	query := q.Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "q is required")
		return
	}
	caseSensitive, ok := parseCase(q.Get("case"))
	if !ok {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "case must be sensitive or insensitive")
		return
	}
	inMsg, inAuthor, ok := parseIn(q.Get("in"))
	if !ok {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "in must be a comma list of message,author")
		return
	}
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

	res, err := h.SearchCommits(q.Get("ref"), query, caseSensitive, inMsg, inAuthor, limit, before)
	if err != nil {
		writeSearchErr(w, err)
		return
	}
	resp := protocol.SearchCommitsResponse{
		Query:     query,
		Ref:       q.Get("ref"),
		RefCommit: hexOrEmpty(res.RefCommit),
		Matches:   make([]protocol.CommitSummary, 0, len(res.Matches)),
		Next:      hexOrEmpty(res.Next),
		Truncated: res.Truncated,
	}
	for _, c := range res.Matches {
		resp.Matches = append(resp.Matches, commitSummaryOf(c))
	}
	// A search is a pure function of the scanned commit + the query params (§9).
	s.writeHub(w, r, resp, etag("search-commits", res.RefCommit.String(),
		q.Get("in"), boolStr(caseSensitive), query, strconv.Itoa(limit), resp.Next))
}

// parseIn reads the ?in= field list for commit search (default both). Empty →
// both; a comma list of "message"/"author"; any other token is invalid.
func parseIn(v string) (inMsg, inAuthor, ok bool) {
	if v == "" {
		return true, true, true
	}
	for _, f := range strings.Split(v, ",") {
		switch strings.TrimSpace(f) {
		case "message":
			inMsg = true
		case "author":
			inAuthor = true
		default:
			return false, false, false
		}
	}
	return inMsg, inAuthor, inMsg || inAuthor
}

// handleRepoSearchPaths: GET .../search/paths?q=&ref=&limit=&case=.
func (s *Server) handleRepoSearchPaths(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.beginHubRead(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	query := q.Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "q is required")
		return
	}
	caseSensitive, ok := parseCase(q.Get("case"))
	if !ok {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "case must be sensitive or insensitive")
		return
	}
	limit, ok := parseSearchLimit(q.Get("limit"))
	if !ok {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "limit must be 1..200")
		return
	}

	res, err := h.SearchPaths(q.Get("ref"), query, caseSensitive, limit)
	if err != nil {
		writeSearchErr(w, err)
		return
	}
	resp := protocol.SearchPathsResponse{
		Query:     query,
		Ref:       q.Get("ref"),
		RefCommit: hexOrEmpty(res.RefCommit),
		Matches:   make([]protocol.PathMatch, 0, len(res.Matches)),
		Truncated: res.Truncated,
	}
	for _, m := range res.Matches {
		resp.Matches = append(resp.Matches, protocol.PathMatch{
			Path: m.Path, Blob: m.ID.String(), Mode: octalMode(m.Mode), IsDir: m.IsDir,
		})
	}
	// Path search is a pure function of the tree + query params (§9).
	s.writeHub(w, r, resp, etag("search-paths", res.Tree.String(),
		boolStr(caseSensitive), query, strconv.Itoa(limit)))
}

// handleRepoSearchContent: GET .../search/content?q=&ref=&limit=&case=.
func (s *Server) handleRepoSearchContent(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.beginHubRead(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	query := q.Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "q is required")
		return
	}
	caseSensitive, ok := parseCase(q.Get("case"))
	if !ok {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "case must be sensitive or insensitive")
		return
	}
	limit, ok := parseSearchLimit(q.Get("limit"))
	if !ok {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "limit must be 1..200")
		return
	}

	res, err := h.SearchContent(q.Get("ref"), query, caseSensitive, limit)
	if err != nil {
		writeSearchErr(w, err)
		return
	}
	resp := protocol.SearchContentResponse{
		Query:     query,
		Ref:       q.Get("ref"),
		RefCommit: hexOrEmpty(res.RefCommit),
		Matches:   make([]protocol.ContentMatch, 0, len(res.Matches)),
		Truncated: res.Truncated,
	}
	for _, m := range res.Matches {
		lines := make([]protocol.ContentLine, 0, len(m.Lines))
		for _, ln := range m.Lines {
			lines = append(lines, protocol.ContentLine{Line: ln.Line, Content: ln.Content})
		}
		resp.Matches = append(resp.Matches, protocol.ContentMatch{
			Path: m.Path, Blob: m.Blob.String(), Lines: lines,
		})
	}
	// Content search is a pure function of the tree's blobs + query params (§9).
	s.writeHub(w, r, resp, etag("search-content", res.Tree.String(),
		boolStr(caseSensitive), query, strconv.Itoa(limit)))
}

// writeSearchErr maps a projection error to a status code (RFC-0024 §10):
// ErrNotFound→404, ErrBadCursor→400, else 500. Search never 413s — it truncates.
func writeSearchErr(w http.ResponseWriter, err error) {
	switch err {
	case hub.ErrNotFound:
		writeError(w, http.StatusNotFound, protocol.CodeUnknownRepo, "no such ref")
	case hub.ErrBadCursor:
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "cursor not reachable")
	default:
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, err.Error())
	}
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
