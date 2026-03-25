package manifest

import (
	"testing"
)

func TestParse(t *testing.T) {
	raw := []byte(`{
		"name": "My App",
		"short_name": "MyApp",
		"start_url": "/",
		"icons": [
			{"src": "icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any"},
			{"src": "icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any"},
			{"src": "icon-mono.png", "sizes": "512x512", "type": "image/png", "purpose": "monochrome"}
		]
	}`)

	m, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if m.AppName() != "My App" {
		t.Errorf("AppName() = %q, want %q", m.AppName(), "My App")
	}

	best := m.BestIcon("any")
	if best == nil {
		t.Fatal("BestIcon(any) returned nil")
	}
	if best.Src != "icon-512.png" {
		t.Errorf("BestIcon(any).Src = %q, want icon-512.png", best.Src)
	}

	mono := m.BestIcon("monochrome")
	if mono == nil {
		t.Fatal("BestIcon(monochrome) returned nil")
	}
	if mono.Src != "icon-mono.png" {
		t.Errorf("BestIcon(monochrome).Src = %q, want icon-mono.png", mono.Src)
	}
}

func TestParse_ShortNameFallback(t *testing.T) {
	raw := []byte(`{"short_name": "Short"}`)
	m, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if m.AppName() != "Short" {
		t.Errorf("AppName() = %q, want Short", m.AppName())
	}
}

func TestLargestDimension(t *testing.T) {
	tests := []struct {
		sizes string
		want  int
	}{
		{"192x192", 192},
		{"512x512", 512},
		{"48x48 96x96", 96},
		{"any", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := largestDimension(tt.sizes)
		if got != tt.want {
			t.Errorf("largestDimension(%q) = %d, want %d", tt.sizes, got, tt.want)
		}
	}
}
