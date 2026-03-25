package manifest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const maxBodySize = 2 << 20 // 2 MB

var (
	linkTagRE     = regexp.MustCompile(`(?is)<link\s[^>]*?>`)
	relManifestRE = regexp.MustCompile(`(?i)rel\s*=\s*["']manifest["']`)
	hrefRE        = regexp.MustCompile(`(?i)href\s*=\s*["']([^"']+)["']`)
)

// FetchFromURL discovers and fetches the Web App Manifest from a remote page.
// It GETs the HTML, finds <link rel="manifest" href="...">, fetches the
// manifest JSON, and returns the parsed manifest together with the resolved
// manifest URL (needed to resolve relative icon paths later).
func FetchFromURL(ctx context.Context, pageURL string) (*Manifest, *url.URL, error) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid URL %q: %w", pageURL, err)
	}

	html, err := httpGet(ctx, pageURL)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching %s: %w", pageURL, err)
	}

	href := findManifestHref(string(html))
	if href == "" {
		return nil, nil, fmt.Errorf("no <link rel=\"manifest\"> found at %s", pageURL)
	}

	ref, err := url.Parse(href)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid manifest href %q: %w", href, err)
	}
	manifestURL := base.ResolveReference(ref)

	data, err := httpGet(ctx, manifestURL.String())
	if err != nil {
		return nil, nil, fmt.Errorf("fetching manifest %s: %w", manifestURL, err)
	}

	m, err := Parse(data)
	if err != nil {
		return nil, nil, err
	}
	return m, manifestURL, nil
}

// ResolveIconURL resolves a manifest icon src against the manifest's own URL.
// Absolute URLs are returned as-is; relative paths are resolved against base.
func ResolveIconURL(src string, base *url.URL) string {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return src
	}
	ref, err := url.Parse(src)
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

// findManifestHref extracts the href from the first <link rel="manifest">
// tag in the HTML document.
func findManifestHref(html string) string {
	for _, tag := range linkTagRE.FindAllString(html, -1) {
		if !relManifestRE.MatchString(tag) {
			continue
		}
		m := hrefRE.FindStringSubmatch(tag)
		if len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

func httpGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
}
