// Package manifest parses Web App Manifest files (manifest.json / PWA manifests).
// Spec: https://www.w3.org/TR/appmanifest/
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manifest represents the parsed fields of a Web App Manifest relevant to APK building.
type Manifest struct {
	Name            string   `json:"name"`
	ShortName       string   `json:"short_name"`
	StartURL        string   `json:"start_url"`
	Display         string   `json:"display"`
	ThemeColor      string   `json:"theme_color"`
	BackgroundColor string   `json:"background_color"`
	Lang            string   `json:"lang"`
	Description     string   `json:"description"`
	Icons           []Icon   `json:"icons"`
	Permissions     []string `json:"permissions"`
}

// Icon represents a single icon entry in the manifest.
type Icon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose"`
}

// AppName returns the best name for the app: Name if set, otherwise ShortName.
func (m *Manifest) AppName() string {
	if m.Name != "" {
		return m.Name
	}
	return m.ShortName
}

// BestIcon returns the largest icon with the given purpose ("any" = color, "monochrome").
// Falls back to icons with no explicit purpose (treated as "any").
// Returns nil if no matching icon found.
func (m *Manifest) BestIcon(purpose string) *Icon {
	var best *Icon
	bestSize := 0
	for i := range m.Icons {
		ic := &m.Icons[i]
		p := strings.ToLower(strings.TrimSpace(ic.Purpose))
		if p == "" {
			p = "any"
		}
		// purpose may be space-separated list per spec
		purposes := strings.Fields(p)
		match := false
		for _, pp := range purposes {
			if pp == purpose {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		sz := largestDimension(ic.Sizes)
		if best == nil || sz > bestSize {
			best = ic
			bestSize = sz
		}
	}
	return best
}

// largestDimension parses a sizes string like "192x192" or "any" and returns the largest
// single dimension (width). Returns 0 for "any" or unparseable values.
func largestDimension(sizes string) int {
	if sizes == "" || strings.EqualFold(sizes, "any") {
		return 0
	}
	parts := strings.Fields(sizes)
	max := 0
	for _, p := range parts {
		p = strings.ToLower(p)
		var w, h int
		if _, err := fmt.Sscanf(p, "%dx%d", &w, &h); err == nil {
			if w > max {
				max = w
			}
		}
	}
	return max
}

// ParseFile reads and parses a manifest.json from the given path.
func ParseFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	return Parse(data)
}

// Parse parses a manifest.json from raw JSON bytes.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest JSON: %w", err)
	}
	return &m, nil
}

// FindInDir looks for a manifest.json in the given directory.
// Returns (path, nil) if found, ("", nil) if not present, or ("", err) on I/O error.
func FindInDir(dir string) (string, error) {
	candidates := []string{"manifest.json", "manifest.webmanifest"}
	for _, name := range candidates {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", nil
}
