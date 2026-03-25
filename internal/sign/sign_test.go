package sign

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestGenerateDebugKeystore(t *testing.T) {
	ks, err := GenerateDebugKeystore()
	if err != nil {
		t.Fatalf("GenerateDebugKeystore: %v", err)
	}
	if ks.PrivKey == nil {
		t.Error("PrivKey is nil")
	}
	if ks.Cert == nil {
		t.Error("Cert is nil")
	}
	pem := ks.ExportCertPEM()
	if len(pem) == 0 {
		t.Error("ExportCertPEM returned empty string")
	}
}

func TestSign_BlockPresent(t *testing.T) {
	apkData := buildMinimalZIP(t)

	ks, err := GenerateDebugKeystore()
	if err != nil {
		t.Fatalf("GenerateDebugKeystore: %v", err)
	}

	signed, err := Sign(apkData, ks)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Verify that "APK Sig Block 42" magic appears in the output
	if !bytes.Contains(signed, []byte(apkSigningBlockMagic)) {
		t.Error("signing block magic not found in signed APK")
	}

	// Verify the output is a valid ZIP
	r, err := zip.NewReader(bytes.NewReader(signed), int64(len(signed)))
	if err != nil {
		t.Errorf("signed APK is not a valid ZIP: %v", err)
	}
	if len(r.File) == 0 {
		t.Error("signed APK has no entries")
	}
}

func TestSign_EOCDOffset(t *testing.T) {
	apkData := buildMinimalZIP(t)
	ks, _ := GenerateDebugKeystore()
	signed, err := Sign(apkData, ks)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// The EOCD's CD offset should now point past the signing block
	eocdOff, err := findEOCDOffset(signed)
	if err != nil {
		t.Fatalf("findEOCDOffset: %v", err)
	}
	cdOff, _, err := parseCDFromEOCD(signed, eocdOff)
	if err != nil {
		t.Fatalf("parseCDFromEOCD: %v", err)
	}

	// Before the CD, we should find the signing block magic
	if cdOff < 16 {
		t.Fatalf("cdOff too small: %d", cdOff)
	}
	magic := string(signed[cdOff-16 : cdOff])
	if magic != apkSigningBlockMagic {
		t.Errorf("magic at CD-16 = %q, want %q", magic, apkSigningBlockMagic)
	}
}

// buildMinimalZIP creates a minimal but valid ZIP in memory.
func buildMinimalZIP(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("AndroidManifest.xml")
	f.Write([]byte("stub"))
	w.Close()
	return buf.Bytes()
}
