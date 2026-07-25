package server

// This file is the RFC-0022 browse API: GET tree/blob/raw plus the path filter on
// commits. Each reuses the RFC-0021 read preamble (beginHubRead: authenticate →
// authorize `read` before any object is read → open the projection), delegates
// all reading to internal/hub, and returns a content-addressed ETag. The security
// crux is handleRepoRaw (§7/B5): repository bytes are served inert — a neutral
// content type, nosniff, and an attachment disposition for non-text — so
// attacker-controlled content can never execute on the Hub's origin.

import (
	"bytes"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/thulasiramk-2310/vara/internal/hub"
	"github.com/thulasiramk-2310/vara/internal/protocol"
)

// Blob size limits (RFC-0022 §7). Two distinct thresholds on purpose: the blob
// JSON endpoint caps INLINE content (over it → 200 with truncated:true), while
// raw enforces a larger HARD streaming ceiling (over it → 413). They are package
// vars, not consts, only so tests can shrink them without multi-MiB fixtures.
var (
	maxInlineBytes = 1 << 20  // 1 MiB — blob JSON inline cap
	maxRawBytes    = 50 << 20 // 50 MiB — raw hard ceiling
)

// inlineText reports whether data may be returned inline as a JSON text string:
// it must be NUL-free AND valid UTF-8. Non-UTF-8 bytes would corrupt or break the
// JSON string, so such a file is treated as binary (§7). This is a presentation
// decision in the server layer; it never alters the blob id or ETag (B2).
func inlineText(data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	return utf8.Valid(data)
}

// handleRepoTree: GET /_vara/repositories/{repo}/tree/{path...}?ref=<name>.
func (s *Server) handleRepoTree(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.beginHubRead(w, r)
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	path := r.PathValue("path")
	listing, err := h.Tree(ref, path)
	if err != nil {
		writeBrowseErr(w, err)
		return
	}
	resp := protocol.TreeResponse{
		Ref:     ref,
		Commit:  listing.Commit.String(),
		Path:    path,
		Entries: make([]protocol.TreeEntryInfo, 0, len(listing.Entries)),
	}
	for _, e := range listing.Entries {
		typ := "file"
		if e.IsDir {
			typ = "dir"
		}
		resp.Entries = append(resp.Entries, protocol.TreeEntryInfo{
			Name: e.Name, Type: typ, Mode: octalMode(e.Mode), ID: e.ID.String(),
		})
	}
	// The listing is fully determined by the tree id (a content address), so an
	// unchanged directory re-validates for free across commits (§9).
	s.writeHub(w, r, resp, etag(listing.Tree.String()))
}

// handleRepoBlob: GET /_vara/repositories/{repo}/blob/{path...}?ref=<name>.
func (s *Server) handleRepoBlob(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.beginHubRead(w, r)
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	path := r.PathValue("path")
	b, err := h.Blob(ref, path)
	if err != nil {
		writeBrowseErr(w, err)
		return
	}
	resp := protocol.BlobResponse{
		Ref:    ref,
		Commit: b.Commit.String(),
		Path:   path,
		ID:     b.ID.String(),
		Size:   len(b.Data),
	}
	switch {
	case len(b.Data) > maxInlineBytes:
		resp.Truncated = true
		resp.Encoding = "binary"
	case !inlineText(b.Data):
		resp.Binary = true
		resp.Encoding = "binary"
	default:
		resp.Encoding = "utf-8"
		resp.Content = string(b.Data)
	}
	// The blob id is the content address of the bytes, so it is the strong ETag.
	s.writeHub(w, r, resp, etag(b.ID.String()))
}

// handleRepoRaw: GET /_vara/repositories/{repo}/raw/{path...}?ref=<name>. Streams
// a file's exact bytes, always inert (§7/B5): neutral content type, nosniff, and
// an attachment disposition for non-text.
func (s *Server) handleRepoRaw(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.beginHubRead(w, r)
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	path := r.PathValue("path")
	b, err := h.Blob(ref, path)
	if err != nil {
		writeBrowseErr(w, err)
		return
	}
	if len(b.Data) > maxRawBytes {
		writeError(w, http.StatusRequestEntityTooLarge, protocol.CodeTooLarge, "file exceeds raw size ceiling")
		return
	}
	tag := etag(b.ID.String())
	w.Header().Set(protocol.HeaderAPI, protocol.APIVersion)
	w.Header().Set("ETag", tag)
	// Inert content, always. Text renders as plain text (cannot execute); anything
	// else downloads as an opaque attachment. nosniff stops the browser from
	// second-guessing the type. This is the security crux of RFC-0022 (B5).
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if inlineText(b.Data) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment")
	}
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(b.Data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b.Data)
}

// writeBrowseErr maps a projection error to a status code (RFC-0022 §10):
// ErrNotFound→404, ErrBadCursor→400, else 500.
func writeBrowseErr(w http.ResponseWriter, err error) {
	switch err {
	case hub.ErrNotFound:
		writeError(w, http.StatusNotFound, protocol.CodeNotFound, "no such path")
	case hub.ErrBadCursor:
		writeError(w, http.StatusBadRequest, protocol.CodeMalformed, "cursor not reachable")
	default:
		writeError(w, http.StatusInternalServerError, protocol.CodeInternal, err.Error())
	}
}

// octalMode renders a tree entry mode git-style in octal, e.g. "40000" (dir),
// "100644" (regular), "100755" (executable).
func octalMode(mode uint32) string {
	return strconv.FormatUint(uint64(mode), 8)
}
