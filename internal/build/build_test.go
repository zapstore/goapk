package build

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zapstore/goapk/internal/config"
	"github.com/zapstore/goapk/internal/sign"
)

const testPWADir = "../../testdata/pwas/minimal"

func TestBuild_MinimalPWA(t *testing.T) {
	if _, err := os.Stat(filepath.Join(testPWADir, "icon-512.png")); err != nil {
		t.Skip("testdata icons not present; run `go generate ./testdata/...` to create them")
	}

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "test.apk")

	cfg := &config.BuildConfig{
		AppName:     "Minimal PWA",
		PackageName: "com.example.minimalpwa",
		VersionCode: 1,
		VersionName: "1.0",
		MinSDK:      24,
		TargetSDK:   35,
		AssetsDir:   testPWADir,
		IconColor:   filepath.Join(testPWADir, "icon-512.png"),
		IconMono:    filepath.Join(testPWADir, "icon-mono.png"),
		OutputPath:  outPath,
	}

	if err := Build(context.Background(), cfg); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// ---- Verify the output ----
	apkData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading output APK: %v", err)
	}

	// 1. Must be a valid ZIP
	r, err := zip.NewReader(bytes.NewReader(apkData), int64(len(apkData)))
	if err != nil {
		t.Fatalf("output is not a valid ZIP: %v", err)
	}

	// 2. Must contain required entries
	required := []string{
		"AndroidManifest.xml",
		"classes.dex",
		"resources.arsc",
		"assets/config.json",
		"assets/www/index.html",
		"assets/www/manifest.json",
	}
	found := map[string]bool{}
	for _, f := range r.File {
		found[f.Name] = true
	}
	for _, name := range required {
		if !found[name] {
			t.Errorf("missing required entry: %s", name)
		}
	}

	// 3. Must contain icon entries for at least one density
	hasIcon := false
	for name := range found {
		if len(name) > 4 && name[:4] == "res/" {
			hasIcon = true
			break
		}
	}
	if !hasIcon {
		t.Error("no res/ entries found (expected mipmap icons)")
	}

	// 4. Must contain APK Sig Block 42 magic
	if !bytes.Contains(apkData, []byte(sign.APKSigningBlockMagic)) {
		t.Error("APK signing block not found in output")
	}

	// 5. resources.arsc must be STORED (uncompressed)
	for _, f := range r.File {
		if f.Name == "resources.arsc" {
			if f.Method != zip.Store {
				t.Errorf("resources.arsc method = %d, want STORE (0)", f.Method)
			}
		}
	}
}

func TestBuild_URLMode(t *testing.T) {
	if _, err := os.Stat(filepath.Join(testPWADir, "icon-512.png")); err != nil {
		t.Skip("testdata icons not present")
	}

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "url.apk")

	cfg := &config.BuildConfig{
		AppName:     "URL App",
		PackageName: "com.example.urlapp",
		VersionCode: 1,
		VersionName: "1.0",
		MinSDK:      24,
		TargetSDK:   35,
		RemoteURL:   "https://example.com",
		IconColor:   filepath.Join(testPWADir, "icon-512.png"),
		OutputPath:  outPath,
	}

	if err := Build(context.Background(), cfg); err != nil {
		t.Fatalf("Build (URL mode): %v", err)
	}

	apkData, _ := os.ReadFile(outPath)
	r, err := zip.NewReader(bytes.NewReader(apkData), int64(len(apkData)))
	if err != nil {
		t.Fatalf("output is not a valid ZIP: %v", err)
	}

	// config.json must contain the URL
	for _, f := range r.File {
		if f.Name == "assets/config.json" {
			rc, _ := f.Open()
			var buf bytes.Buffer
			buf.ReadFrom(rc)
			rc.Close()
			if !bytes.Contains(buf.Bytes(), []byte("https://example.com")) {
				t.Errorf("config.json does not contain the start URL: %s", buf.String())
			}
		}
	}
}

func TestConfigFromCLI_ManifestAutoDetect(t *testing.T) {
	cfg, err := ConfigFromCLI(
		testPWADir,       // assetsDir
		"",               // url
		"",               // manifest (auto-detect)
		"",               // name (from manifest)
		"com.example.t",  // package
		"",               // versionName
		0,                // versionCode
		0,                // minSDK
		0,                // targetSDK
		"",               // iconColor (from manifest)
		"",               // iconMono
		"",               // keystore
		"",               // keystorePass
		"out.apk",        // output
	)
	if err != nil {
		t.Fatalf("ConfigFromCLI: %v", err)
	}
	if cfg.AppName != "Minimal PWA" {
		t.Errorf("AppName = %q, want %q", cfg.AppName, "Minimal PWA")
	}
	if cfg.IconColor == "" {
		t.Error("IconColor should be auto-detected from manifest.json")
	}
}
