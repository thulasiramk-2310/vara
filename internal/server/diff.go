package server

// This file is the RFC-0023 diff API: GET diff summary, per-file diff, and the
// commit-diff convenience. Each reuses the RFC-0021 read preamble (beginHubRead:
// authenticate → authorize `read` before any object is read → open the
// projection), delegates all reading to internal/hub, and returns a
// content-addressed ETag. Diff content is inert JSON data (§7/D4): there is no
// raw-diff endpoint, and a binary side never enters a line diff.

import (
	"net/http"

	"github.com/thulasiramk-2310/vara/internal/hub"
	"github.com/thulasiramk-2310/vara/internal/protocol"
)

// handleRepoDiffSummary: GET /_vara/repositories/{repo}/diff?head=&base=.
func (s *Server) handleRepoDiffSummary(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.beginHubRead(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	head := q.Get("head")
	if head == "" {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "head is required")
		return
	}
	s.writeDiffSummary(w, r, q.Get("base"), head, h)
}

// handleRepoCommitDiff: GET /_vara/repositories/{repo}/commits/{id}/diff — the
// convenience form, exactly /diff?head={id} (base omitted → first parent).
func (s *Server) handleRepoCommitDiff(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.beginHubRead(w, r)
	if !ok {
		return
	}
	s.writeDiffSummary(w, r, "", r.PathValue("id"), h)
}

// writeDiffSummary is the shared body of the two summary endpoints.
func (s *Server) writeDiffSummary(w http.ResponseWriter, r *http.Request, base, head string, h *hub.Repo) {
	sum, err := h.DiffSummary(base, head)
	if err != nil {
		writeDiffErr(w, err)
		return
	}
	resp := protocol.DiffSummaryResponse{
		Base:       base,
		Head:       head,
		BaseCommit: hexOrEmpty(sum.BaseCommit),
		HeadCommit: hexOrEmpty(sum.HeadCommit),
		Files:      make([]protocol.DiffFileInfo, 0, len(sum.Files)),
		Truncated:  sum.Truncated,
	}
	for _, f := range sum.Files {
		resp.Files = append(resp.Files, protocol.DiffFileInfo{Path: f.Path, OldPath: f.OldPath, Status: f.Status})
	}
	// The change set is a pure function of the two trees, so the tree-id pair is a
	// strong ETag (§9): an unchanged pair re-validates for free.
	s.writeHub(w, r, resp, etag(sum.BaseCommit.String(), sum.HeadCommit.String(), "diff"))
}

// handleRepoFileDiff: GET /_vara/repositories/{repo}/diff/{path...}?head=&base=.
func (s *Server) handleRepoFileDiff(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.beginHubRead(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	head := q.Get("head")
	if head == "" {
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "head is required")
		return
	}
	base := q.Get("base")
	path := r.PathValue("path")
	fd, err := h.FileDiff(base, head, path)
	if err != nil {
		writeDiffErr(w, err)
		return
	}
	resp := protocol.FileDiffResponse{
		Base:      base,
		Head:      head,
		Path:      path,
		Status:    fd.Status,
		Binary:    fd.Binary,
		Additions: fd.Additions,
		Deletions: fd.Deletions,
		Hunks:     make([]protocol.DiffHunk, 0, len(fd.Hunks)),
		Truncated: fd.Truncated,
	}
	for _, hk := range fd.Hunks {
		lines := make([]protocol.DiffLineInfo, 0, len(hk.Lines))
		for _, ln := range hk.Lines {
			lines = append(lines, protocol.DiffLineInfo{Type: ln.Type, Content: ln.Content})
		}
		resp.Hunks = append(resp.Hunks, protocol.DiffHunk{
			OldStart: hk.OldStart, OldLines: hk.OldLines,
			NewStart: hk.NewStart, NewLines: hk.NewLines,
			Header: hk.Header, Lines: lines,
		})
	}
	// The hunks are a pure function of the two blob versions, so the resolved
	// commit pair + path is a stable strong ETag (§9).
	s.writeHub(w, r, resp, etag(fd.BaseCommit.String(), fd.HeadCommit.String(), "filediff", path))
}

// writeDiffErr maps a projection error to a status code (RFC-0023 §10):
// ErrNotFound→404, ErrTooLarge→413, else 500.
func writeDiffErr(w http.ResponseWriter, err error) {
	switch err {
	case hub.ErrNotFound:
		writeError(w, http.StatusNotFound, protocol.CodeNotFound, "no such diff")
	case hub.ErrTooLarge:
		writeError(w, http.StatusRequestEntityTooLarge, protocol.CodeTooLarge, "file too large to diff")
	default:
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, err.Error())
	}
}
