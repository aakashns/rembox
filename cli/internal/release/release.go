// Package release finds, downloads and verifies rembox releases, and installs
// a downloaded binary over the running one.
//
// Releases are built by `make dist` and published by
// .github/workflows/release.yml. Each one has a plain binary per platform,
// named without the version so that <BaseURL>/latest/download/<asset> always
// works, and their checksums:
//
//	rembox-darwin-arm64
//	rembox-linux-amd64
//	...
//	SHA256SUMS   (sha256sum's output: "<hex>  <file name>" per line)
//
// install.sh relies on the same layout, so keep the two in sync.
package release

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aakashns/rembox/cli/internal/version"
)

// DefaultBaseURL is where releases are published. If that changes, change
// install.sh too.
const DefaultBaseURL = "https://github.com/aakashns/rembox/releases"

// BaseURLEnv overrides DefaultBaseURL, so tests can point rembox and
// install.sh at pretend releases.
const BaseURLEnv = "REMBOX_RELEASES_URL"

// Size limits, so a bad or hostile server can't make us use unbounded memory.
const (
	maxBinaryBytes    = 128 << 20
	maxChecksumsBytes = 1 << 20
)

// ErrNoReleases is returned by Latest when nothing has been released yet.
var ErrNoReleases = errors.New("no releases have been published yet")

// Client talks to the release server.
type Client struct {
	BaseURL   string // e.g. DefaultBaseURL, without a trailing slash
	HTTP      *http.Client
	UserAgent string
}

// NewClient returns a client for $REMBOX_RELEASES_URL, or DefaultBaseURL.
func NewClient() *Client {
	base := strings.TrimSuffix(os.Getenv(BaseURLEnv), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	return &Client{
		BaseURL: base,
		HTTP: &http.Client{
			Timeout:       2 * time.Minute,
			CheckRedirect: httpsOnly,
		},
		UserAgent: "rembox/" + version.Version,
	}
}

// httpsOnly refuses redirects that would downgrade to plain HTTP.
func httpsOnly(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("too many redirects")
	}
	if req.URL.Scheme != "https" && via[0].URL.Scheme == "https" {
		return fmt.Errorf("refusing redirect to non-HTTPS URL %s", req.URL)
	}
	return nil
}

// AssetName is the binary's name for a platform, e.g. "rembox-darwin-arm64".
func AssetName(goos, goarch string) string {
	return fmt.Sprintf("rembox-%s-%s", goos, goarch)
}

// Latest returns the tag of the newest stable release, e.g. "v0.2.0".
//
// It reads the redirect from <BaseURL>/latest to <BaseURL>/tag/<tag> instead
// of calling the GitHub API, which limits unauthenticated clients to 60
// requests an hour.
func (c *Client) Latest(ctx context.Context) (string, error) {
	req, err := c.newRequest(ctx, c.BaseURL+"/latest")
	if err != nil {
		return "", err
	}
	noFollow := *c.HTTP
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := noFollow.Do(req)
	if err != nil {
		return "", fmt.Errorf("checking for the latest release: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		return "", fmt.Errorf("checking for the latest release: unexpected response %s from %s", resp.Status, req.URL)
	}
	loc, err := resp.Location()
	if err != nil {
		return "", fmt.Errorf("checking for the latest release: %w", err)
	}
	// With no releases, GitHub redirects to the release list instead of a tag.
	dir, tag := path.Split(strings.TrimSuffix(loc.Path, "/"))
	if path.Base(dir) != "tag" {
		return "", ErrNoReleases
	}
	if tag, err = url.PathUnescape(tag); err != nil || !strings.HasPrefix(tag, "v") || !version.Valid(tag) {
		return "", fmt.Errorf("latest release has an unexpected tag %q", tag)
	}
	return tag, nil
}

// FetchBinary downloads the rembox binary for a tag and platform and checks it
// against the release's SHA256SUMS.
func (c *Client) FetchBinary(ctx context.Context, tag, goos, goarch string) ([]byte, error) {
	if !strings.HasPrefix(tag, "v") || !version.Valid(tag) {
		return nil, fmt.Errorf("not a release tag: %q", tag)
	}
	asset := AssetName(goos, goarch)
	dir := c.BaseURL + "/download/" + url.PathEscape(tag) + "/"

	sums, err := c.get(ctx, dir+"SHA256SUMS", maxChecksumsBytes)
	if err != nil {
		return nil, err
	}
	want, err := findChecksum(sums, asset)
	if err != nil {
		return nil, fmt.Errorf("release %s: %w", tag, err)
	}
	bin, err := c.get(ctx, dir+asset, maxBinaryBytes)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(bin)
	if !bytes.Equal(got[:], want) {
		return nil, fmt.Errorf("checksum mismatch for %s: the download is corrupt or was tampered with", asset)
	}
	return bin, nil
}

func (c *Client) newRequest(ctx context.Context, u string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	return req, nil
}

// get downloads a URL, failing on non-200 responses or bodies over limit bytes.
func (c *Client) get(ctx context.Context, u string, limit int64) ([]byte, error) {
	req, err := c.newRequest(ctx, u)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("downloading %s: not found (is this platform supported?)", u)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: %s", u, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", u, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("downloading %s: larger than %d bytes", u, limit)
	}
	return body, nil
}

// findChecksum returns the sha256 listed for name in a SHA256SUMS file.
func findChecksum(sums []byte, name string) ([]byte, error) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		sum, err := hex.DecodeString(fields[0])
		if err != nil || len(sum) != sha256.Size {
			return nil, fmt.Errorf("SHA256SUMS has a malformed entry for %s", name)
		}
		return sum, nil
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading SHA256SUMS: %w", err)
	}
	return nil, fmt.Errorf("no checksum for %s (is this platform supported?)", name)
}
