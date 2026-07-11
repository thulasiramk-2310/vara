// Package protocol defines the on-the-wire contract for VARA-RFC-0016 (Remote
// Transport Protocol). It holds the message DTOs, header names, version and
// capability tokens, and the hex/code helpers shared by BOTH the client binding
// (internal/transport HTTPTransport) and the server (internal/server).
//
// Keeping the contract in one leaf package is the concrete expression of the
// RFC's Single Implementation Principle (§9.1): client and server encode/decode
// through the same types, so a field can never mean one thing on one side and
// something else on the other.
//
// RFC: VARA-RFC-0016 Remote Transport Protocol (HTTP Binding v1)
//
// Layer: leaf. It imports only pkg/types. transport and server import it.
package protocol

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Protocol and wire versions (RFC-0016 §8.3). Version is "<rfc>.<binding>".
const (
	Version     = "16.1"
	WireVersion = "1"
)

// Reserved headers (RFC-0016 §8.3).
const (
	HeaderProto = "X-VARA-Protocol"
	HeaderWire  = "X-VARA-Wire"
	HeaderRepo  = "X-VARA-Repository"
	HeaderTxn   = "X-VARA-Transaction"
)

// Content types (RFC-0016 §8.2).
const (
	CTJSON = "application/json"
	CTPack = "application/x-vara-pack"
)

// Capability tokens (RFC-0016 §5.4).
const (
	CapReportStatus      = "report-status-v1"
	CapReceiveIdempotent = "receive-idempotent"
)

// Endpoint path suffixes appended to a repository base URL (RFC-0016 §8.1).
const (
	PathInfoRefs = "/info/refs"
	PathFetch    = "/fetch"
	PathReceive  = "/receive"
)

// Major returns the RFC-major of a protocol version string ("16.1" -> "16").
func Major(v string) string {
	major, _, _ := strings.Cut(v, ".")
	return major
}

// --- message DTOs (RFC-0016 §8.4) ---

// Ref is one advertised reference.
type Ref struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

// ListRefsResponse is the body of GET /:repo/info/refs.
type ListRefsResponse struct {
	Head string   `json:"head"`
	Refs []Ref    `json:"refs"`
	Caps []string `json:"caps"`
}

// FetchRequest is the body of POST /:repo/fetch.
type FetchRequest struct {
	Wants []string `json:"wants"`
	Haves []string `json:"haves"`
}

// Update is one requested reference update.
type Update struct {
	Name  string `json:"name"`
	Old   string `json:"old"`
	New   string `json:"new"`
	Force bool   `json:"force"`
}

// ReceiveRequest is the JSON part of the multipart POST /:repo/receive body.
type ReceiveRequest struct {
	Updates []Update `json:"updates"`
}

// Result is one per-ref outcome.
type Result struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

// ReceiveResponse is the body of POST /:repo/receive. OK is the AND of every
// per-ref OK, letting a client branch on one boolean (RFC-0016 §8.6).
type ReceiveResponse struct {
	OK      bool     `json:"ok"`
	Results []Result `json:"results"`
}

// Error is the structured body of every request-level failure (RFC-0016 §8.6).
type Error struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Per-ref result codes (RFC-0016 §8.6).
const (
	CodeOK             = "OK"
	CodeNonFastForward = "NON_FAST_FORWARD"
	CodeStale          = "STALE"
	CodeMissingObject  = "MISSING_OBJECT"
	CodeCreateFailed   = "CREATE_FAILED"
	CodeUpdateFailed   = "UPDATE_FAILED"
	CodeRejected       = "REJECTED"
)

// Request-level error codes (RFC-0016 §8.6, extended by RFC-0017/0018).
const (
	CodeMalformed       = "MALFORMED_REQUEST"
	CodeUnknownRepo     = "UNKNOWN_REPOSITORY"
	CodeInvalidPack     = "INVALID_PACK"
	CodeUpgrade         = "UPGRADE_REQUIRED"
	CodeLockTimeout     = "LOCK_TIMEOUT"
	CodeInternal        = "INTERNAL"
	CodeUnauthenticated = "UNAUTHENTICATED" // RFC-0017 §8.2 — 401
	CodeUnauthorized    = "UNAUTHORIZED"    // RFC-0018 §8.2 — 403
)

// CommitHex renders a commit ID as lowercase hex (64 chars; zero -> 64 zeros).
func CommitHex(c types.CommitID) string { return c.String() }

// ParseCommit parses a 64-char hex string into a CommitID. The all-zero string
// parses to the zero CommitID (the "ref does not exist" sentinel).
func ParseCommit(s string) (types.CommitID, error) {
	b, err := types.ParseHex(s)
	if err != nil {
		return types.CommitID{}, err
	}
	return types.CommitID(b), nil
}

// CodeForReason maps a transport RefUpdateResult reason to a stable per-ref
// code. ok always yields CodeOK; otherwise the reason prefix/substring selects
// the code (mirrors the reason strings Local.applyUpdate produces).
func CodeForReason(ok bool, reason string) string {
	switch {
	case ok:
		return CodeOK
	case strings.HasPrefix(reason, "stale"):
		return CodeStale
	case strings.Contains(reason, "non-fast-forward"):
		return CodeNonFastForward
	case strings.Contains(reason, "missing from pack"):
		return CodeMissingObject
	case strings.Contains(reason, "create ref"):
		return CodeCreateFailed
	case strings.Contains(reason, "update ref"):
		return CodeUpdateFailed
	default:
		return CodeRejected
	}
}

// NewTxnID mints an opaque client transaction ID (RFC-0016 §8.3.1).
func NewTxnID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
