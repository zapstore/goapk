package sign

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"testing"

	apkzip "github.com/zapstore/goapk/internal/zip"
)

// TestSign_RoundTrip builds a ZIP, signs it, and verifies the result is still readable.
func TestSign_RoundTrip(t *testing.T) {
	// Build a realistic ZIP with a STORED entry (resources.arsc)
	entries := []apkzip.Entry{
		apkzip.NewEntry("AndroidManifest.xml", []byte("manifest")),
		{Name: "resources.arsc", Data: []byte("resources"), Stored: true},
		apkzip.NewEntry("classes.dex", []byte("dex")),
		apkzip.NewEntry("assets/config.json", []byte(`{"start_url":"https://x.com"}`)),
	}
	zipData, err := apkzip.Build(entries)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	t.Logf("pre-sign ZIP size: %d bytes", len(zipData))

	// Verify pre-sign ZIP is valid
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("pre-sign ZIP invalid: %v", err)
	}
	t.Logf("pre-sign entries: %d", len(r.File))

	ks, _ := GenerateDebugKeystore()
	signed, err := Sign(zipData, ks)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	t.Logf("signed APK size: %d bytes", len(signed))

	// Check EOCD
	eocdOff, err := findEOCDOffset(signed)
	if err != nil {
		t.Fatalf("EOCD not found: %v", err)
	}
	t.Logf("EOCD at offset %d (total size %d)", eocdOff, len(signed))

	cdOff, cdSize, err := parseCDFromEOCD(signed, eocdOff)
	if err != nil {
		t.Fatalf("parseCDFromEOCD: %v", err)
	}
	t.Logf("CD at offset %d, size %d", cdOff, cdSize)

	// Verify CD offset + size + EOCD = total
	if cdOff+cdSize != eocdOff {
		t.Errorf("CD end (%d) != EOCD offset (%d)", cdOff+cdSize, eocdOff)
	}

	// Check signing block magic is immediately before CD
	if cdOff < 16 {
		t.Fatalf("cdOff %d too small to have signing block before it", cdOff)
	}
	magic := string(signed[cdOff-16 : cdOff])
	if magic != APKSigningBlockMagic {
		t.Errorf("magic before CD = %q, want %q", magic, APKSigningBlockMagic)
	}

	// Dump EOCD bytes for inspection
	t.Logf("EOCD bytes: %x", signed[eocdOff:eocdOff+22])

	// Try to read as ZIP
	r, err = zip.NewReader(bytes.NewReader(signed), int64(len(signed)))
	if err != nil {
		// Dump the area around EOCD
		start := eocdOff - 10
		if start < 0 {
			start = 0
		}
		t.Logf("bytes around EOCD: %x", signed[start:eocdOff+22])

		// Check the EOCD signature
		eocdSig := binary.LittleEndian.Uint32(signed[eocdOff:])
		t.Logf("EOCD sig: 0x%08x (want 0x06054b50)", eocdSig)

		t.Fatalf("signed ZIP invalid: %v", err)
	}
	t.Logf("signed entries: %d", len(r.File))
}
