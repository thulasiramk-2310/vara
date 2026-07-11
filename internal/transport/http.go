package transport

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/protocol"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Open resolves a URL to a Transport, dispatching by scheme (RFC-0016 §9):
// http(s):// and vara:// yield an HTTPTransport; everything else (a path or a
// file:// URL) yields a Local transport. Command-layer callers use this and are
// otherwise oblivious to which binding they got — the whole point of the
// Transport interface.
func Open(rawurl string) (Transport, error) {
	if isRemoteURL(rawurl) {
		return OpenHTTP(rawurl)
	}
	return OpenLocal(rawurl)
}

func isRemoteURL(u string) bool {
	return strings.HasPrefix(u, "http://") ||
		strings.HasPrefix(u, "https://") ||
		strings.HasPrefix(u, "vara://")
}

// SuggestDir infers a clone destination directory name from any URL (local
// path, file://, or http(s)://): the last path segment. Used by clone when the
// user does not name a directory.
func SuggestDir(rawurl string) string {
	u := rawurl
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:] // drop scheme; for http this leaves host/.../name
	}
	u = strings.TrimRight(u, `/\`)
	if i := strings.LastIndexAny(u, `/\`); i >= 0 {
		u = u[i+1:]
	}
	return u
}

// HTTPTransport is the HTTP binding of the Transport protocol (RFC-0016 §8). It
// is a pure client: it encodes each Transport call as an HTTP request and
// decodes the response. It performs no repository logic — all mutation happens
// on the server, which routes through Local (Single Implementation Principle,
// RFC-0016 §9.1).
type HTTPTransport struct {
	base   string // repository base URL, e.g. http://host:8080/myrepo (no trailing /)
	client *http.Client

	// authHeader, when non-empty, is sent as the Authorization header on every
	// request (RFC-0017 §9 — the client MAY present a credential).
	authHeader string

	// Cached advertisement from the most recent ListRefs, so HeadTarget (called
	// right after ListRefs by clone) does not need a second round-trip.
	head   string
	gotAdv bool
}

// SetBasicAuth makes the transport present HTTP Basic credentials (RFC-0017 §9).
func (t *HTTPTransport) SetBasicAuth(user, secret string) {
	t.authHeader = "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+secret))
}

// SetBearerToken makes the transport present a Bearer token (RFC-0017 §9).
func (t *HTTPTransport) SetBearerToken(token string) {
	t.authHeader = "Bearer " + token
}

// OpenHTTP constructs an HTTPTransport for a remote repository URL. It does not
// contact the server; the first ListRefs/FetchPack does. A vara:// URL is
// treated as http:// in v1.
func OpenHTTP(rawurl string) (*HTTPTransport, error) {
	u := rawurl
	if rest, ok := strings.CutPrefix(u, "vara://"); ok {
		u = "http://" + rest
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return nil, fmt.Errorf("http transport: bad url %q: %w", rawurl, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("http transport: unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" || strings.Trim(parsed.Path, "/") == "" {
		return nil, fmt.Errorf("http transport: url must be scheme://host/repository, got %q", rawurl)
	}
	return &HTTPTransport{
		base:   strings.TrimRight(u, "/"),
		client: &http.Client{},
	}, nil
}

// ListRefs performs GET /:repo/info/refs.
func (t *HTTPTransport) ListRefs() ([]RefAdvertisement, error) {
	lr, err := t.advertise()
	if err != nil {
		return nil, err
	}
	out := make([]RefAdvertisement, 0, len(lr.Refs))
	for _, r := range lr.Refs {
		c, err := protocol.ParseCommit(r.Target)
		if err != nil {
			return nil, fmt.Errorf("http: bad target for %s: %w", r.Name, err)
		}
		out = append(out, RefAdvertisement{Name: r.Name, Target: c})
	}
	return out, nil
}

// HeadTarget returns the peer's HEAD target, reusing the cached advertisement
// from a preceding ListRefs when available.
func (t *HTTPTransport) HeadTarget() (string, error) {
	if t.gotAdv {
		return t.head, nil
	}
	lr, err := t.advertise()
	if err != nil {
		return "", err
	}
	return lr.Head, nil
}

func (t *HTTPTransport) advertise() (*protocol.ListRefsResponse, error) {
	req, err := t.newRequest(http.MethodGet, protocol.PathInfoRefs, nil, "")
	if err != nil {
		return nil, err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}
	var lr protocol.ListRefsResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, fmt.Errorf("http: decode info/refs: %w", err)
	}
	t.head, t.gotAdv = lr.Head, true
	return &lr, nil
}

// FetchPack performs POST /:repo/fetch and returns the raw VPCK response body
// as a stream. The caller owns closing it.
func (t *HTTPTransport) FetchPack(wants, haves []types.CommitID) (io.ReadCloser, error) {
	body, err := json.Marshal(protocol.FetchRequest{
		Wants: hexCommits(wants),
		Haves: hexCommits(haves),
	})
	if err != nil {
		return nil, err
	}
	req, err := t.newRequest(http.MethodPost, protocol.PathFetch, bytes.NewReader(body), protocol.CTJSON)
	if err != nil {
		return nil, err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, decodeError(resp)
	}
	return resp.Body, nil
}

// ReceivePack performs POST /:repo/receive as multipart/mixed: part 1 is the
// JSON update list, part 2 is the raw VPCK stream (RFC-0016 §8.2). A
// non-fast-forward or stale-CAS rejection comes back as HTTP 200 with per-ref
// results (ok:false), NOT as a transport error (RFC-0016 §8.5).
func (t *HTTPTransport) ReceivePack(pack io.Reader, updates []RefUpdate) ([]RefUpdateResult, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	// Part 1: updates as JSON.
	jsonHdr := textproto.MIMEHeader{"Content-Type": {protocol.CTJSON}}
	p1, err := mw.CreatePart(jsonHdr)
	if err != nil {
		return nil, err
	}
	if err := json.NewEncoder(p1).Encode(protocol.ReceiveRequest{Updates: toWireUpdates(updates)}); err != nil {
		return nil, err
	}

	// Part 2: the raw pack stream.
	packHdr := textproto.MIMEHeader{"Content-Type": {protocol.CTPack}}
	p2, err := mw.CreatePart(packHdr)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(p2, pack); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	req, err := t.newRequest(http.MethodPost, protocol.PathReceive, &body,
		"multipart/mixed; boundary="+mw.Boundary())
	if err != nil {
		return nil, err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// 200 = processed (inspect results); 409 = optional all-stale fast path,
	// which also carries a results body.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return nil, decodeError(resp)
	}
	var rr protocol.ReceiveResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return nil, fmt.Errorf("http: decode receive response: %w", err)
	}
	return fromWireResults(rr.Results), nil
}

// Close releases the HTTP client's idle connections.
func (t *HTTPTransport) Close() error {
	t.client.CloseIdleConnections()
	return nil
}

// newRequest builds a request with the protocol headers set (RFC-0016 §8.3).
func (t *HTTPTransport) newRequest(method, suffix string, body io.Reader, contentType string) (*http.Request, error) {
	req, err := http.NewRequest(method, t.base+suffix, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set(protocol.HeaderProto, protocol.Version)
	req.Header.Set(protocol.HeaderWire, protocol.WireVersion)
	req.Header.Set(protocol.HeaderTxn, protocol.NewTxnID())
	if t.authHeader != "" {
		req.Header.Set("Authorization", t.authHeader)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req, nil
}

// decodeError turns a non-2xx response into an error, preferring the structured
// {code,message} body (RFC-0016 §8.6) and falling back to the status line.
func decodeError(resp *http.Response) error {
	data, _ := io.ReadAll(resp.Body)
	var eb protocol.Error
	if json.Unmarshal(data, &eb) == nil && eb.Code != "" {
		return fmt.Errorf("%s [%s]: %s", eb.Code, resp.Status, eb.Message)
	}
	return fmt.Errorf("http %s: %s", resp.Status, strings.TrimSpace(string(data)))
}

func hexCommits(ids []types.CommitID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}

func toWireUpdates(us []RefUpdate) []protocol.Update {
	out := make([]protocol.Update, len(us))
	for i, u := range us {
		out[i] = protocol.Update{Name: u.Name, Old: u.Old.String(), New: u.New.String(), Force: u.Force}
	}
	return out
}

func fromWireResults(rs []protocol.Result) []RefUpdateResult {
	out := make([]RefUpdateResult, len(rs))
	for i, r := range rs {
		reason := r.Reason
		if reason == "" && !r.OK {
			reason = r.Code
		}
		out[i] = RefUpdateResult{Name: r.Name, OK: r.OK, Reason: reason}
	}
	return out
}
