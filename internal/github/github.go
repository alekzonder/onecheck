// Package github is a minimal client for GitHub Releases: list the releases
// of a repository and download their assets.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// DefaultAPI is the public GitHub API.
const DefaultAPI = "https://api.github.com"

var repoRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Asset is a file attached to a release.
type Asset struct {
	Name string `json:"name"`
	// URL is the API URL of the asset.
	URL string `json:"url"`
}

// Release is a published release of a repository.
type Release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// Client reads releases. It is an interface so that tests need no network.
type Client interface {
	// Releases returns the published releases of "owner/repo", newest first.
	Releases(ctx context.Context, repo string) ([]Release, error)
	// Download writes the content of an asset to w.
	Download(ctx context.Context, a Asset, w io.Writer) error
}

type client struct {
	base  string
	token string
	http  *http.Client
}

// New returns a client for the API at baseURL (empty means DefaultAPI). The
// token is optional: it raises rate limits and gives access to private
// repositories.
func New(baseURL, token string) Client {
	if baseURL == "" {
		baseURL = DefaultAPI
	}
	return &client{base: strings.TrimRight(baseURL, "/"), token: token, http: &http.Client{Timeout: 5 * time.Minute}}
}

func (c *client) get(ctx context.Context, url, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "onecheck")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	defer resp.Body.Close()
	var apiErr struct {
		Message string `json:"message"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	_ = json.Unmarshal(body, &apiErr)
	msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
	if apiErr.Message != "" {
		msg += ": " + apiErr.Message
	}
	if (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) && c.token == "" {
		msg += " (set GITHUB_TOKEN to raise the rate limit)"
	}
	return nil, fmt.Errorf("%s", msg)
}

func (c *client) Releases(ctx context.Context, repo string) ([]Release, error) {
	if !repoRE.MatchString(repo) {
		return nil, fmt.Errorf("invalid repository %q: expected owner/repo", repo)
	}
	resp, err := c.get(ctx, c.base+"/repos/"+repo+"/releases?per_page=50", "application/vnd.github+json")
	if err != nil {
		return nil, fmt.Errorf("cannot list releases of %s: %w", repo, err)
	}
	defer resp.Body.Close()
	var all []Release
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return nil, fmt.Errorf("cannot list releases of %s: invalid response: %w", repo, err)
	}
	published := all[:0]
	for _, r := range all {
		if !r.Draft {
			published = append(published, r)
		}
	}
	return published, nil
}

func (c *client) Download(ctx context.Context, a Asset, w io.Writer) error {
	resp, err := c.get(ctx, a.URL, "application/octet-stream")
	if err != nil {
		return fmt.Errorf("cannot download %s: %w", a.Name, err)
	}
	defer resp.Body.Close()
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("cannot download %s: %w", a.Name, err)
	}
	return nil
}
