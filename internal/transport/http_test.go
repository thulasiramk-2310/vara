package transport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSuggestDirNamesTheCloneFromTheLastPathSegment(t *testing.T) {
	cases := map[string]string{
		"../upstream":                    "upstream",
		"/srv/repos/project":             "project",
		"/srv/repos/project/":            "project",
		`C:\work\project`:                "project",
		`C:\work\project\`:               "project",
		"file:///srv/repos/project":      "project",
		"http://hub.example/project":     "project",
		"https://hub.example:8080/a/b/c": "c",
		"vara://hub.example/project/":    "project",
		"project":                        "project",
	}
	for in, want := range cases {
		if got := SuggestDir(in); got != want {
			t.Errorf("SuggestDir(%q) = %q, want %q", in, got, want)
		}
	}
}

// A transport sends the credentials it was given, and only those.
func TestTransportPresentsTheChosenCredentials(t *testing.T) {
	var seen []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"head":"refs/heads/main","refs":[],"caps":[]}`))
	}))
	defer ts.Close()

	tr, err := OpenHTTP(ts.URL + "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.HeadTarget(); err != nil {
		t.Fatalf("head: %v", err)
	}
	tr.SetBasicAuth("ada", "s3cret:with:colons")
	tr2, _ := OpenHTTP(ts.URL + "/repo")
	tr2.SetBearerToken("tok-123")
	_, _ = tr2.HeadTarget()
	tr3, _ := OpenHTTP(ts.URL + "/repo")
	tr3.SetBasicAuth("ada", "s3cret:with:colons")
	_, _ = tr3.HeadTarget()

	if len(seen) != 3 || seen[0] != "" {
		t.Fatalf("a transport with no credentials must send none: %q", seen)
	}
	if seen[1] != "Bearer tok-123" {
		t.Errorf("bearer header = %q", seen[1])
	}
	// "ada:s3cret:with:colons" base64, the colon in the secret kept intact
	if !strings.HasPrefix(seen[2], "Basic ") || seen[2] != "Basic YWRhOnMzY3JldDp3aXRoOmNvbG9ucw==" {
		t.Errorf("basic header = %q", seen[2])
	}
}

// The advertisement fetched by one call is reused by the next, so asking for the HEAD
// right after listing the refs costs no second request.
func TestHeadTargetReusesTheAdvertisement(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"head":"refs/heads/trunk","refs":[],"caps":[]}`))
	}))
	defer ts.Close()
	tr, err := OpenHTTP(ts.URL + "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.ListRefs(); err != nil {
		t.Fatal(err)
	}
	head, err := tr.HeadTarget()
	if err != nil || head != "refs/heads/trunk" || calls != 1 {
		t.Fatalf("head %q, err %v, %d requests (want 1)", head, err, calls)
	}
}

func TestOpenHTTPRejectsBadURLs(t *testing.T) {
	for _, bad := range []string{"ftp://hub/repo", "http://hub", "http://hub/", "http:///repo", "://nope", "gopher://x/y"} {
		if _, err := OpenHTTP(bad); err == nil {
			t.Errorf("OpenHTTP(%q) should be rejected", bad)
		}
	}
	if _, err := OpenHTTP("vara://hub.example/repo"); err != nil {
		t.Errorf("a vara:// address is valid: %v", err)
	}
}
