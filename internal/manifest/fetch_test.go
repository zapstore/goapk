package manifest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestFindManifestHref(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			"standard",
			`<link rel="manifest" href="/manifest.json">`,
			"/manifest.json",
		},
		{
			"reversed attributes",
			`<link href="/app.webmanifest" rel="manifest">`,
			"/app.webmanifest",
		},
		{
			"single quotes",
			`<link rel='manifest' href='/m.json'>`,
			"/m.json",
		},
		{
			"extra attributes",
			`<link rel="manifest" crossorigin="use-credentials" href="/manifest.json">`,
			"/manifest.json",
		},
		{
			"multiline",
			"<link\n  rel=\"manifest\"\n  href=\"/manifest.json\"\n>",
			"/manifest.json",
		},
		{
			"none",
			`<link rel="stylesheet" href="/style.css">`,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findManifestHref(tt.html)
			if got != tt.want {
				t.Errorf("findManifestHref() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveIconURL(t *testing.T) {
	base, _ := url.Parse("https://example.com/assets/manifest.json")
	tests := []struct {
		src  string
		want string
	}{
		{"icon.png", "https://example.com/assets/icon.png"},
		{"/icon.png", "https://example.com/icon.png"},
		{"https://cdn.example.com/icon.png", "https://cdn.example.com/icon.png"},
	}
	for _, tt := range tests {
		got := ResolveIconURL(tt.src, base)
		if got != tt.want {
			t.Errorf("ResolveIconURL(%q) = %q, want %q", tt.src, got, tt.want)
		}
	}
}

func TestFetchFromURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html><head>
<link rel="manifest" href="/manifest.json">
</head><body></body></html>`))
	})
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"name": "Test PWA",
			"short_name": "Test",
			"icons": [
				{"src": "icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any"}
			]
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	m, manifestURL, err := FetchFromURL(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatalf("FetchFromURL: %v", err)
	}
	if m.AppName() != "Test PWA" {
		t.Errorf("AppName = %q, want %q", m.AppName(), "Test PWA")
	}
	if manifestURL.Path != "/manifest.json" {
		t.Errorf("manifest URL path = %q, want /manifest.json", manifestURL.Path)
	}
	if ic := m.BestIcon("any"); ic == nil || ic.Src != "icon-512.png" {
		t.Error("expected to find any-purpose icon")
	}
}

func TestFetchFromURL_NoManifest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>No manifest here</body></html>`))
	}))
	defer srv.Close()

	_, _, err := FetchFromURL(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for page without manifest link")
	}
}
