package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alekzonder/onecheck/internal/version"
	"github.com/alekzonder/onecheck/pkg/module"
)

type params struct {
	URL                string            `json:"url"`
	Method             string            `json:"method"`
	Headers            map[string]string `json:"headers"`
	Body               string            `json:"body"`
	FollowRedirects    *bool             `json:"follow_redirects"`
	InsecureSkipVerify bool              `json:"insecure_skip_verify"`
}

func collect(ctx context.Context, req module.Request) (map[string]any, error) {
	var p params
	if err := module.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(p.URL, "http://") && !strings.HasPrefix(p.URL, "https://") {
		return nil, errors.New("invalid params: url must start with http:// or https://")
	}
	if p.Method == "" {
		p.Method = http.MethodGet
	}
	httpReq, err := http.NewRequestWithContext(ctx, strings.ToUpper(p.Method), p.URL, strings.NewReader(p.Body))
	if err != nil {
		return nil, requestError(err)
	}
	httpReq.Header.Set("User-Agent", "onecheck-http/"+version.Version)
	for name, value := range p.Headers {
		httpReq.Header.Set(name, value)
		if strings.EqualFold(name, "Host") {
			httpReq.Host = value
		}
	}

	start := time.Now()
	resp, err := client(p).Do(httpReq)
	if err != nil {
		return nil, requestError(err)
	}
	defer resp.Body.Close()
	var body io.Reader = resp.Body
	if req.MaxFieldSize > 0 {
		// One byte over the limit lets the SDK flag the value as truncated.
		body = io.LimitReader(resp.Body, req.MaxFieldSize+1)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	headers := map[string]any{}
	for name, values := range resp.Header {
		headers[strings.ToLower(name)] = strings.Join(values, ", ")
	}
	out := map[string]any{
		"status":   resp.StatusCode,
		"headers":  headers,
		"body":     string(data),
		"duration": time.Since(start).Seconds(),
	}
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		out["tls_expires_in"] = time.Until(resp.TLS.PeerCertificates[0].NotAfter).Seconds()
	}
	return out, nil
}

// safeURL is the part of a URL that may be shown in an error: scheme, host
// and path. The query string and the userinfo can hold tokens and passwords.
func safeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "<invalid url>"
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}).String()
}

// requestError rebuilds the error of a failed request around safeURL. Go puts
// the whole URL into it, after ${VAR} expansion and after redirects, and the
// error text ends up in results, events, logs and notifications.
func requestError(err error) error {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	return fmt.Errorf("%s %q: %w", ue.Op, safeURL(ue.URL), ue.Err)
}

func client(p params) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: p.InsecureSkipVerify} //nolint:gosec // opt-in per rule
	c := &http.Client{Transport: transport}
	if p.FollowRedirects != nil && !*p.FollowRedirects {
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return c
}
