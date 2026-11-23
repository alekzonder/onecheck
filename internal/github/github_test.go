package github

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const releasesJSON = `[
  {"tag_name": "v1.3.0-rc1", "draft": false, "prerelease": true, "assets": []},
  {"tag_name": "v9.9.9", "draft": true, "prerelease": false, "assets": []},
  {"tag_name": "v1.2.0", "draft": false, "prerelease": false, "assets": [
    {"name": "onecheck_linux_amd64", "url": "BASE/repos/o/r/releases/assets/1"},
    {"name": "checksums.txt", "url": "BASE/repos/o/r/releases/assets/2"}
  ]}
]`

func server(t *testing.T) (*httptest.Server, *[]*http.Request) {
	t.Helper()
	var seen []*http.Request
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Clone(context.Background()))
		switch r.URL.Path {
		case "/repos/o/r/releases":
			_, _ = w.Write([]byte(strings.ReplaceAll(releasesJSON, "BASE", srv.URL)))
		case "/repos/o/r/releases/assets/1":
			_, _ = w.Write([]byte("binary-content"))
		case "/repos/o/limited/releases":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestReleasesParsesAndSendsToken(t *testing.T) {
	srv, seen := server(t)
	rels, err := New(srv.URL, "tok123").Releases(context.Background(), "o/r")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 2 || rels[0].Tag != "v1.3.0-rc1" || !rels[0].Prerelease || rels[1].Tag != "v1.2.0" {
		t.Fatalf("drafts must be dropped, order kept: %+v", rels)
	}
	if len(rels[1].Assets) != 2 || rels[1].Assets[0].Name != "onecheck_linux_amd64" {
		t.Errorf("assets = %+v", rels[1].Assets)
	}
	if got := (*seen)[0].Header.Get("Authorization"); got != "Bearer tok123" {
		t.Errorf("Authorization = %q", got)
	}
}

func TestNoTokenNoHeader(t *testing.T) {
	srv, seen := server(t)
	if _, err := New(srv.URL, "").Releases(context.Background(), "o/r"); err != nil {
		t.Fatal(err)
	}
	if got := (*seen)[0].Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want none", got)
	}
}

func TestDownload(t *testing.T) {
	srv, seen := server(t)
	c := New(srv.URL, "")
	rels, err := c.Releases(context.Background(), "o/r")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := c.Download(context.Background(), rels[1].Assets[0], &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "binary-content" {
		t.Errorf("content = %q", buf.String())
	}
	last := (*seen)[len(*seen)-1]
	if last.Header.Get("Accept") != "application/octet-stream" {
		t.Errorf("Accept = %q", last.Header.Get("Accept"))
	}
	if err := c.Download(context.Background(), Asset{Name: "x", URL: srv.URL + "/nope"}, &buf); err == nil {
		t.Error("a 404 download must be an error")
	}
}

func TestAPIError(t *testing.T) {
	srv, _ := server(t)
	c := New(srv.URL, "")
	_, err := c.Releases(context.Background(), "o/missing")
	if err == nil || !strings.Contains(err.Error(), "o/missing") || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v", err)
	}
	_, err = c.Releases(context.Background(), "o/limited")
	if err == nil || !strings.Contains(err.Error(), "rate limit") || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Errorf("a 403 must show the API message and hint at GITHUB_TOKEN, got %v", err)
	}
	if _, err := c.Releases(context.Background(), "not a repo"); err == nil {
		t.Error("an invalid repo name must be rejected")
	}
}
