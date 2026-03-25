package xmlbin

import (
	"encoding/binary"
	"testing"
)

// TestEncodeManifest verifies that EncodeManifest produces a valid binary XML blob:
//   - starts with the XML chunk type (0x0003)
//   - contains the package name string
//   - chunk size field matches actual length
func TestEncodeManifest(t *testing.T) {
	p := ManifestParams{
		Package:       "com.example.test",
		VersionCode:   42,
		VersionName:   "1.2.3",
		MinSDK:        24,
		TargetSDK:     35,
		AppLabel:      0x7f010000,
		AppIcon:       0x7f020000,
		ActivityClass: "com.example.test.MainActivity",
		Permissions:   []string{"android.permission.INTERNET"},
	}

	data := EncodeManifest(p)

	if len(data) < 8 {
		t.Fatalf("output too short: %d bytes", len(data))
	}

	// Check XML chunk type
	chunkType := binary.LittleEndian.Uint16(data[0:])
	if chunkType != chunkXML {
		t.Errorf("chunk type = 0x%04x, want 0x%04x", chunkType, chunkXML)
	}

	// Check chunk size matches actual length
	declaredSize := binary.LittleEndian.Uint32(data[4:])
	if int(declaredSize) != len(data) {
		t.Errorf("declared size = %d, actual = %d", declaredSize, len(data))
	}

	// Check the package name appears somewhere in the blob (as UTF-16LE)
	pkg := "com.example.test"
	if !containsUTF16(data, pkg) {
		t.Errorf("package name %q not found in binary XML", pkg)
	}

	// Check activity class appears
	if !containsUTF16(data, p.ActivityClass) {
		t.Errorf("activity class %q not found in binary XML", p.ActivityClass)
	}
}

func TestEncodeManifest_StringPool(t *testing.T) {
	p := ManifestParams{
		Package:       "test.pkg",
		VersionCode:   1,
		VersionName:   "1.0",
		MinSDK:        24,
		TargetSDK:     35,
		AppLabel:      0x7f010000,
		AppIcon:       0x7f020000,
		ActivityClass: "test.pkg.MainActivity",
		Permissions:   nil,
	}

	data := EncodeManifest(p)

	// Second chunk should be the string pool (type 0x0001), starting at offset 8
	if len(data) < 16 {
		t.Fatal("output too short for string pool check")
	}
	spType := binary.LittleEndian.Uint16(data[8:])
	if spType != chunkStringPool {
		t.Errorf("second chunk type = 0x%04x, want 0x%04x (string pool)", spType, chunkStringPool)
	}
}

// containsUTF16 searches for s encoded as UTF-16LE within data.
func containsUTF16(data []byte, s string) bool {
	runes := []rune(s)
	encoded := make([]byte, len(runes)*2)
	for i, r := range runes {
		binary.LittleEndian.PutUint16(encoded[i*2:], uint16(r))
	}
	// Naive substring search
	for i := 0; i <= len(data)-len(encoded); i++ {
		if string(data[i:i+len(encoded)]) == string(encoded) {
			return true
		}
	}
	return false
}

func TestBoolVal(t *testing.T) {
	if BoolVal(true).Data != 0xFFFFFFFF {
		t.Errorf("BoolVal(true).Data = 0x%x, want 0xFFFFFFFF", BoolVal(true).Data)
	}
	if BoolVal(false).Data != 0 {
		t.Errorf("BoolVal(false).Data = 0x%x, want 0", BoolVal(false).Data)
	}
}
