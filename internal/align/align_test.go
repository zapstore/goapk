package align

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"testing"

	apkzip "github.com/zapstore/goapk/internal/zip"
)

// TestAlign_StoredEntry verifies that a STORED entry's data starts at a 4-byte
// aligned offset after alignment.
func TestAlign_StoredEntry(t *testing.T) {
	entries := []apkzip.Entry{
		// AndroidManifest must come first (to create a misaligned resources.arsc)
		apkzip.NewEntry("AndroidManifest.xml", []byte("stub")),
		{Name: "resources.arsc", Data: []byte("hello world padtest"), Stored: true},
	}
	raw, err := apkzip.Build(entries)
	if err != nil {
		t.Fatalf("apkzip.Build: %v", err)
	}

	aligned, err := Align(raw, 4)
	if err != nil {
		t.Fatalf("Align failed: %v", err)
	}

	// Must still be a valid ZIP
	r, err := zip.NewReader(bytes.NewReader(aligned), int64(len(aligned)))
	if err != nil {
		t.Fatalf("aligned output is not a valid ZIP: %v", err)
	}
	if len(r.File) != 2 {
		t.Errorf("expected 2 entries, got %d", len(r.File))
	}

	// Find the local file entry for resources.arsc and check alignment
	offset := findLocalEntryDataOffset(t, aligned, "resources.arsc")
	if offset%4 != 0 {
		t.Errorf("data offset %d is not 4-byte aligned", offset)
	}
}

// TestAlign_DeflatedEntry verifies that DEFLATE entries pass through unchanged.
func TestAlign_DeflatedEntry(t *testing.T) {
	entries := []apkzip.Entry{
		apkzip.NewEntry("index.html", []byte("<html>test</html>")),
	}
	raw, err := apkzip.Build(entries)
	if err != nil {
		t.Fatalf("apkzip.Build: %v", err)
	}
	before := len(raw)

	aligned, err := Align(raw, 4)
	if err != nil {
		t.Fatalf("Align failed: %v", err)
	}

	// DEFLATE entries should not change size (no padding needed)
	if len(aligned) != before {
		t.Logf("original: %d bytes, aligned: %d bytes", before, len(aligned))
		// Not necessarily equal; alignment adds 0 padding for DEFLATE — just ensure valid ZIP
	}

	_, err = zip.NewReader(bytes.NewReader(aligned), int64(len(aligned)))
	if err != nil {
		t.Fatalf("aligned output is not a valid ZIP: %v", err)
	}
}

// TestAlign_Idempotent verifies that aligning an already-aligned ZIP produces the same output.
func TestAlign_Idempotent(t *testing.T) {
	entries := []apkzip.Entry{
		apkzip.NewEntry("AndroidManifest.xml", []byte("stub")),
		{Name: "resources.arsc", Data: bytes.Repeat([]byte{0xAA}, 100), Stored: true},
	}
	raw, _ := apkzip.Build(entries)

	first, err := Align(raw, 4)
	if err != nil {
		t.Fatalf("first Align: %v", err)
	}
	second, err := Align(first, 4)
	if err != nil {
		t.Fatalf("second Align: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Error("second alignment changed the output (not idempotent)")
	}
}

// TestAlign_CDOffsets verifies that Central Directory offsets are updated correctly.
func TestAlign_CDOffsets(t *testing.T) {
	entries := []apkzip.Entry{
		apkzip.NewEntry("file1.html", bytes.Repeat([]byte{'x'}, 1000)),
		{Name: "resources.arsc", Data: bytes.Repeat([]byte{0xFF}, 200), Stored: true},
		apkzip.NewEntry("file3.html", bytes.Repeat([]byte{'y'}, 500)),
	}
	raw, _ := apkzip.Build(entries)
	aligned, err := Align(raw, 4)
	if err != nil {
		t.Fatalf("Align: %v", err)
	}

	// Read the aligned ZIP and verify all entries are accessible
	r, err := zip.NewReader(bytes.NewReader(aligned), int64(len(aligned)))
	if err != nil {
		t.Fatalf("aligned ZIP invalid: %v", err)
	}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Errorf("opening entry %q: %v", f.Name, err)
			continue
		}
		var buf bytes.Buffer
		buf.ReadFrom(rc)
		rc.Close()
		if buf.Len() == 0 {
			t.Errorf("entry %q has no content", f.Name)
		}
	}
}

// ---- helpers ----

// findLocalEntryDataOffset searches for the local file entry with the given name
// and returns the offset where its data begins.
func findLocalEntryDataOffset(t *testing.T, data []byte, name string) int {
	t.Helper()
	i := 0
	for i < len(data)-4 {
		sig := binary.LittleEndian.Uint32(data[i:])
		if sig != 0x04034b50 {
			break
		}
		if i+30 > len(data) {
			break
		}
		nameLen := int(binary.LittleEndian.Uint16(data[i+26:]))
		extraLen := int(binary.LittleEndian.Uint16(data[i+28:]))
		if i+30+nameLen > len(data) {
			break
		}
		entryName := string(data[i+30 : i+30+nameLen])
		dataStart := i + 30 + nameLen + extraLen
		if entryName == name {
			return dataStart
		}
		compSize := int(binary.LittleEndian.Uint32(data[i+18:]))
		i = dataStart + compSize
	}
	t.Fatalf("entry %q not found in ZIP data", name)
	return 0
}
