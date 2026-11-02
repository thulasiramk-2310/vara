package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
}

func do(t *testing.T, h http.Handler, method, target string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRequestIDIsAlwaysSet(t *testing.T) {
	h := withMiddleware(okHandler())
	if id := do(t, h, "GET", "/", nil).Header().Get("X-Request-Id"); len(id) != 16 {
		t.Fatalf("generated id %q, want 16 hex chars", id)
	}
	if id := do(t, h, "GET", "/", map[string]string{"X-Request-Id": "abc-123_X"}).Header().Get("X-Request-Id"); id != "abc-123_X" {
		t.Fatalf("a safe caller id should be kept, got %q", id)
	}
	for _, bad := range []string{"has space", "new\nline", strings.Repeat("a", 65), "<script>"} {
		if id := do(t, h, "GET", "/", map[string]string{"X-Request-Id": bad}).Header().Get("X-Request-Id"); id == bad || id == "" {
			t.Fatalf("unsafe id %q must be replaced, got %q", bad, id)
		}
	}
}

func TestCrossSiteWritesAreRefused(t *testing.T) {
	h := withMiddleware(okHandler())
	cases := []struct {
		name   string
		method string
		hdr    map[string]string
		want   int
	}{
		{"no origin (the vara command)", "POST", nil, 204},
		{"same origin", "POST", map[string]string{"Origin": "http://hub.example"}, 204},
		{"same origin via proxy host", "POST", map[string]string{"Origin": "https://public.example", "X-Forwarded-Host": "public.example"}, 204},
		{"other origin", "POST", map[string]string{"Origin": "https://evil.example"}, 403},
		{"other origin, delete", "DELETE", map[string]string{"Origin": "https://evil.example"}, 403},
		{"cross-site fetch metadata", "PUT", map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"same-site fetch metadata", "POST", map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://hub.example"}, 204},
		{"unparseable origin", "POST", map[string]string{"Origin": "::::"}, 403},
		{"reads are never refused", "GET", map[string]string{"Origin": "https://evil.example"}, 204},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, "http://hub.example/x", nil)
		for k, v := range c.hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: status %d, want %d", c.name, rec.Code, c.want)
		}
	}
}

func TestPanicBecomesOneFailedRequest(t *testing.T) {
	boom := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/boom" {
			panic("handler bug")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	ts := httptest.NewServer(withMiddleware(boom))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/boom")
	if err != nil {
		t.Fatalf("the connection must survive a handler panic: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var e struct{ Code, Message string }
	if resp.StatusCode != 500 || json.Unmarshal(body, &e) != nil || e.Code == "" || !strings.Contains(e.Message, resp.Header.Get("X-Request-Id")) {
		t.Fatalf("panic response: %d %s (id %q)", resp.StatusCode, body, resp.Header.Get("X-Request-Id"))
	}
	// And the server keeps serving afterwards.
	if resp, err := http.Get(ts.URL + "/fine"); err != nil || resp.StatusCode != 204 {
		t.Fatalf("server stopped serving after a panic: %v", err)
	}
}

func TestWholeServerRefusesCrossSiteLogin(t *testing.T) {
	ts := httptest.NewServer(HandlerWithOptions(t.TempDir(), Options{}))
	defer ts.Close()
	req, _ := http.NewRequest("POST", ts.URL+"/_vara/sessions?cookie=1", strings.NewReader(`{"username":"a","password":"b"}`))
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site login attempt: status %d, want 403", resp.StatusCode)
	}
	if resp.Header.Get("X-Request-Id") == "" {
		t.Fatal("refusals must carry a request id too")
	}
}
