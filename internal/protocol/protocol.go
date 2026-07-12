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

	// Hub read API headers (RFC-0021 §10.1, §10.3). HeaderAPI advertises the Hub
	// API version, independent of the transport protocol version above.
	HeaderAPI       = "X-VARA-API"
	HeaderRequestID = "X-Request-ID"
	APIVersion      = "1"
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

// Control-plane base path (RFC-0019 §8.1). The "/_vara/" prefix is reserved so
// control-plane routes never collide with a data-plane repository route, and no
// repository may be named with a leading "_" (RFC-0019 §10).
const PathRepos = "/_vara/repositories"

// Account/session/token control-plane paths (RFC-0020 §8).
const (
	PathSessions = "/_vara/sessions"
	PathTokens   = "/_vara/tokens"
	PathAccounts = "/_vara/accounts"
	PathWhoami   = "/_vara/whoami" // RFC-0020 §8.5 (additive) — identity introspection
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

// --- control-plane DTOs (RFC-0019 §6.1, §8) ---

// RepositoryDescriptor is the metadata a control-plane request returns for a
// repository (RFC-0019 §6.1). It is descriptive only — it carries no capability
// grants (those live in policy, RFC-0018). Timestamps are RFC 3339 UTC.
type RepositoryDescriptor struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Owner       string `json:"owner"`
	Visibility  string `json:"visibility"`
	State       string `json:"state"`
	Description string `json:"description"`
	Archived    bool   `json:"archived"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// CreateRepoRequest is the body of POST /_vara/repositories (RFC-0019 §8.1).
type CreateRepoRequest struct {
	Name        string `json:"name"`
	Visibility  string `json:"visibility,omitempty"`
	Description string `json:"description,omitempty"`
}

// RenameRepoRequest is the body of POST /_vara/repositories/{repo}/rename.
type RenameRepoRequest struct {
	NewName string `json:"new_name"`
}

// ListReposResponse is the body of GET /_vara/repositories, in stable order
// (RFC-0019 §5.8).
type ListReposResponse struct {
	Repositories []RepositoryDescriptor `json:"repositories"`
}

// --- account/session/token DTOs (RFC-0020 §8) ---

// LoginRequest is the body of POST /_vara/sessions.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse returns the session secret ONCE (RFC-0020 §6.2). Secret is
// omitted in cookie mode (RFC-0021 §7), where it lives only in the httpOnly
// cookie and never in the body.
type LoginResponse struct {
	Secret    string `json:"secret,omitempty"`
	ExpiresAt string `json:"expires_at"`
}

// CreateTokenRequest is the body of POST /_vara/tokens.
type CreateTokenRequest struct {
	Name string `json:"name"`
}

// CreateTokenResponse returns the token secret ONCE (RFC-0020 §6.2).
type CreateTokenResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Secret    string `json:"secret"`
	CreatedAt string `json:"created_at"`
}

// TokenInfo is a token's metadata — never its secret (RFC-0020 §5.3).
type TokenInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
}

// ListTokensResponse is the body of GET /_vara/tokens.
type ListTokensResponse struct {
	Tokens []TokenInfo `json:"tokens"`
}

// CreateAccountRequest is the body of POST /_vara/accounts.
type CreateAccountRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// SetPasswordRequest is the body of PUT /_vara/accounts/{username}/password.
type SetPasswordRequest struct {
	Password string `json:"password"`
}

// AccountDescriptor is the metadata view of an account — never a secret.
type AccountDescriptor struct {
	Username  string `json:"username"`
	State     string `json:"state"`
	CreatedAt string `json:"created_at"`
}

// --- Hub read API DTOs (RFC-0021 §10) ---

// CommitSummary is a projected commit (RFC-0021 §5.3). Timestamps are RFC 3339
// UTC; ids and parents are 64-char hex.
type CommitSummary struct {
	ID        string   `json:"id"`
	Message   string   `json:"message"`
	Author    string   `json:"author"`
	Timestamp string   `json:"timestamp"`
	Parents   []string `json:"parents"`
}

// CommitDetail adds the tree id to a commit summary (RFC-0021 §5.4).
type CommitDetail struct {
	CommitSummary
	Tree string `json:"tree"`
}

// CommitsResponse is the body of GET .../commits, with a cursor for the next page
// (absent at the end).
type CommitsResponse struct {
	Commits []CommitSummary `json:"commits"`
	Next    string          `json:"next,omitempty"`
}

// BranchInfo is a projected branch (RFC-0021 §5.2).
type BranchInfo struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	IsHead bool   `json:"is_head"`
}

// BranchesResponse is the body of GET .../branches.
type BranchesResponse struct {
	Branches []BranchInfo `json:"branches"`
}

// RepositorySummary is the body of GET .../summary (RFC-0021 §5.5).
type RepositorySummary struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	DefaultBranch string         `json:"default_branch"`
	Head          string         `json:"head"`
	CommitCount   int            `json:"commit_count"`
	BranchCount   int            `json:"branch_count"`
	LastCommit    *CommitSummary `json:"last_commit,omitempty"`
}

// WhoamiResponse reports the caller's resolved identity (RFC-0020 §8.5). With a
// ?repo=<name> query it also reports which capabilities the identity holds on
// that resource — a read-only view of the RFC-0018 decision, for debugging
// permissions. It never returns a secret.
type WhoamiResponse struct {
	ID           string          `json:"id"`
	Method       string          `json:"method"`
	Anonymous    bool            `json:"anonymous"`
	Repository   string          `json:"repository,omitempty"`
	Capabilities map[string]bool `json:"capabilities,omitempty"`
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
	CodeUnauthenticated = "UNAUTHENTICATED"   // RFC-0017 §8.2 — 401
	CodeUnauthorized    = "UNAUTHORIZED"      // RFC-0018 §8.2 — 403
	CodeRepoExists      = "REPOSITORY_EXISTS" // RFC-0019 §8.2 — 409
	CodeNotImplemented  = "NOT_IMPLEMENTED"   // RFC-0019 §8.1 — reserved routes, 501
	CodeAccountExists   = "ACCOUNT_EXISTS"    // RFC-0020 §8.4 — 409
	CodeNotFound        = "NOT_FOUND"         // RFC-0020 §8.4 — 404
	CodeRateLimited     = "RATE_LIMITED"      // RFC-0021 §10.3 — 429
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
