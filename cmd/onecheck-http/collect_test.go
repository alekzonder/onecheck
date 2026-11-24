package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alekzonder/onecheck/pkg/module"
)

var allFields = []string{"status", "headers", "body", "duration", "tls_expires_in"}

func run(t *testing.T, params map[string]any, limit int64) (map[string]any, error) {
	t.Helper()
	raw, _ := json.Marshal(params)
	return collect(context.Background(), module.Request{Params: raw, Fields: allFields, MaxFieldSize: limit})
}

func TestHTTPBasic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Method", r.Method)
		w.Header().Set("X-Got-Header", r.Header.Get("X-Token"))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("echo:" + string(body)))
	}))
	defer srv.Close()

	f, err := run(t, map[string]any{"url": srv.URL, "method": "POST", "body": "ping", "headers": map[string]string{"X-Token": "abc"}}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	headers := f["headers"].(map[string]any)
	if f["status"] != 201 || f["body"] != "echo:ping" || headers["x-method"] != "POST" || headers["x-got-header"] != "abc" {
		t.Errorf("fields = %v", f)
	}
	if d, _ := f["duration"].(float64); d <= 0 {
		t.Errorf("duration = %v", f["duration"])
	}
	if _, has := f["tls_expires_in"]; has {
		t.Error("tls_expires_in must be absent for plain http")
	}
}

func TestHTTPDefaultsToGET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Method))
	}))
	defer srv.Close()
	f, err := run(t, map[string]any{"url": srv.URL}, 1024)
	if err != nil || f["body"] != "GET" || f["status"] != 200 {
		t.Errorf("fields = %v, err = %v", f, err)
	}
}

func TestHTTPHeadersLowercased(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Set-Cookie", "a=1")
		w.Header().Add("Set-Cookie", "b=2")
		w.Header().Set("Content-Type", "application/json")
	}))
	defer srv.Close()
	f, err := run(t, map[string]any{"url": srv.URL}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	headers := f["headers"].(map[string]any)
	if headers["set-cookie"] != "a=1, b=2" || headers["content-type"] != "application/json" {
		t.Errorf("headers = %v", headers)
	}
}

func TestHTTPNoFollow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/new", http.StatusFound) })
	mux.HandleFunc("/new", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("new")) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f, err := run(t, map[string]any{"url": srv.URL + "/old"}, 1024)
	if err != nil || f["status"] != 200 || f["body"] != "new" {
		t.Errorf("redirects are followed by default: %v %v", f, err)
	}
	f, err = run(t, map[string]any{"url": srv.URL + "/old", "follow_redirects": false}, 1024)
	if err != nil || f["status"] != 302 {
		t.Errorf("follow_redirects=false must return the redirect itself: %v %v", f, err)
	}
	if f["headers"].(map[string]any)["location"] != "/new" {
		t.Errorf("headers = %v", f["headers"])
	}
}

func TestHTTPTLSExpiry(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()

	if _, err := run(t, map[string]any{"url": srv.URL}, 1024); err == nil {
		t.Error("an untrusted certificate must be an error by default")
	}
	f, err := run(t, map[string]any{"url": srv.URL, "insecure_skip_verify": true}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if exp, _ := f["tls_expires_in"].(float64); exp <= 0 {
		t.Errorf("tls_expires_in = %v", f["tls_expires_in"])
	}
}

func TestHTTPConnError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if _, err := run(t, map[string]any{"url": url}, 1024); err == nil {
		t.Error("connection failure must be an error")
	}
}

func TestHTTPBodyLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", 5000)))
	}))
	defer srv.Close()
	f, err := run(t, map[string]any{"url": srv.URL}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := f["body"].(string); len(body) != 101 {
		t.Errorf("body is %d bytes; want limit+1", len(body))
	}
}

func TestParamsValidation(t *testing.T) {
	for _, params := range []string{`{}`, `{"url":"ftp://x"}`, `{"url":"http://x","nope":1}`} {
		if _, err := collect(context.Background(), module.Request{Params: json.RawMessage(params)}); err == nil {
			t.Errorf("params %s must be rejected", params)
		}
	}
}

func TestDescriptionCoversFields(t *testing.T) {
	for _, f := range allFields {
		if _, ok := mod.Description.Fields[f]; !ok {
			t.Errorf("field %q is not described", f)
		}
	}
	if !json.Valid(mod.Description.ParamsSchema) {
		t.Error("params_schema is not valid JSON")
	}
}

func TestSafeURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://u:p@h.example/a/b?t=secret#f": "https://h.example/a/b",
		"http://h:8080/":                       "http://h:8080/",
		"http://[::1":                          "<invalid url>",
		"not a url":                            "<invalid url>",
	} {
		if got := safeURL(in); got != want {
			t.Errorf("safeURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRequestErrorHidesSecrets(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	closed := strings.TrimPrefix(srv.URL, "http://")
	srv.Close()
	_, err := run(t, map[string]any{"url": "http://user:pass@" + closed + "/health?token=secret"}, 1024)
	if err == nil {
		t.Fatal("connection failure must be an error")
	}
	msg := err.Error()
	if strings.Contains(msg, "secret") || strings.Contains(msg, "pass") || strings.Contains(msg, "user") {
		t.Errorf("the error shows secrets: %s", msg)
	}
	if !strings.Contains(msg, closed+"/health") {
		t.Errorf("the error must name host and path: %s", msg)
	}
}

func TestRedirectErrorHidesSecrets(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	target := dead.URL + "/next?token=secret"
	dead.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusFound)
	}))
	defer srv.Close()
	_, err := run(t, map[string]any{"url": srv.URL + "/?first=hidden"}, 1024)
	if err == nil {
		t.Fatal("a redirect to a closed port must be an error")
	}
	if msg := err.Error(); strings.Contains(msg, "secret") || strings.Contains(msg, "hidden") {
		t.Errorf("the error shows a query string: %s", msg)
	}
}

func TestUnparsableURLIsNotEchoed(t *testing.T) {
	_, err := run(t, map[string]any{"url": "http://h/\x7f?token=secret"}, 1024)
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("the error shows the query string: %s", err)
	}
}
