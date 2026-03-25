// Package build orchestrates the APK assembly pipeline.
//
// Pipeline stages (in order):
//  1. Manifest resolution — load manifest.json, merge CLI flags, apply defaults
//  2. Icon processing     — resize color + monochrome icons to all density buckets
//  3. Binary XML          — encode AndroidManifest.xml
//  4. Resource table      — generate resources.arsc
//  5. Asset packing       — copy web assets into assets/www/; write assets/config.json
//  6. ZIP assembly        — build the APK ZIP with correct entry order
//  7. zipalign            — align STORED entries to 4-byte boundaries
//  8. APK signing         — insert APK Signature Scheme v2 signing block
package build

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zapstore/goapk/internal/align"
	"github.com/zapstore/goapk/internal/config"
	"github.com/zapstore/goapk/internal/embed"
	"github.com/zapstore/goapk/internal/icon"
	"github.com/zapstore/goapk/internal/manifest"
	"github.com/zapstore/goapk/internal/res"
	"github.com/zapstore/goapk/internal/sign"
	apkzip "github.com/zapstore/goapk/internal/zip"
	"github.com/zapstore/goapk/internal/xmlbin"
)

// Build executes the full APK assembly pipeline and writes the signed APK to cfg.OutputPath.
// All intermediate work is done in memory; the output is written atomically.
func Build(ctx context.Context, cfg *config.BuildConfig) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// ---- Stage 1: Icon processing ----
	iconSizes, iconNames, iconSuffixes := densityArrays()
	colorIcons, err := icon.ResizeToAll(cfg.IconColor, iconSizes, iconNames)
	if err != nil {
		return fmt.Errorf("processing color icon: %w", err)
	}
	var monoIcons []icon.Sized
	if cfg.IconMono != "" {
		monoIcons, err = icon.ResizeToAll(cfg.IconMono, iconSizes, iconNames)
		if err != nil {
			return fmt.Errorf("processing monochrome icon: %w", err)
		}
	}

	// Encode icons to PNG bytes
	colorPNGs := make([][]byte, len(colorIcons))
	for i, ic := range colorIcons {
		colorPNGs[i], err = icon.EncodePNG(ic.Image)
		if err != nil {
			return fmt.Errorf("encoding color icon (%s): %w", ic.Name, err)
		}
	}
	var monoPNGs [][]byte
	for _, ic := range monoIcons {
		png, err := icon.EncodePNG(icon.Monochrome(ic.Image))
		if err != nil {
			return fmt.Errorf("encoding mono icon (%s): %w", ic.Name, err)
		}
		monoPNGs = append(monoPNGs, png)
	}

	// ---- Stage 2: Binary XML (AndroidManifest.xml) ----
	permissions := []string{"android.permission.INTERNET"}
	manifestBytes := xmlbin.EncodeManifest(xmlbin.ManifestParams{
		Package:       cfg.PackageName,
		VersionCode:   int32(cfg.VersionCode),
		VersionName:   cfg.VersionName,
		MinSDK:        int32(cfg.MinSDK),
		TargetSDK:     int32(cfg.TargetSDK),
		AppLabel:      config.ResIDAppName,
		AppIcon:       config.ResIDIconColor,
		ActivityClass: cfg.ActivityClass(),
		Permissions:   permissions,
	})

	// ---- Stage 3: resources.arsc ----
	// Build placeholder paths (the paths are stored as strings in resources.arsc;
	// actual PNG bytes go into the ZIP under res/ entries).
	colorPaths := make([]string, len(iconSuffixes))
	for i, suf := range iconSuffixes {
		colorPaths[i] = "res/mipmap-" + suf + "/ic_launcher.png"
	}
	var monoPaths []string
	if len(monoPNGs) > 0 {
		monoPaths = make([]string, len(iconSuffixes))
		for i, suf := range iconSuffixes {
			monoPaths[i] = "res/mipmap-" + suf + "/ic_launcher_mono.png"
		}
	}
	resourcesARSC := res.Encode(res.Params{
		AppName:   cfg.AppName,
		PkgName:   cfg.PackageName,
		IconPaths: colorPaths,
		MonoPaths: monoPaths,
	})

	// ---- Stage 4: DEX embedding ----
	dexBytes := embed.ClassesDEX

	// ---- Stage 5: Web asset packing ----
	assetEntries, err := packAssets(cfg)
	if err != nil {
		return fmt.Errorf("packing assets: %w", err)
	}

	// ---- Stage 6: ZIP assembly ----
	entries := []apkzip.Entry{
		apkzip.NewEntry("AndroidManifest.xml", manifestBytes),
		apkzip.NewEntry("classes.dex", dexBytes),
		{Name: "resources.arsc", Data: resourcesARSC, Stored: true},
	}

	// Add icon entries
	for i, suf := range iconSuffixes {
		entries = append(entries, apkzip.NewEntry(
			"res/mipmap-"+suf+"/ic_launcher.png", colorPNGs[i]))
	}
	for i, suf := range iconSuffixes {
		if i < len(monoPNGs) {
			entries = append(entries, apkzip.NewEntry(
				"res/mipmap-"+suf+"/ic_launcher_mono.png", monoPNGs[i]))
		}
	}

	entries = append(entries, assetEntries...)

	zipBytes, err := apkzip.Build(entries)
	if err != nil {
		return fmt.Errorf("assembling ZIP: %w", err)
	}

	// ---- Stage 7: zipalign ----
	aligned, err := align.Align(zipBytes, align.DefaultAlignment)
	if err != nil {
		return fmt.Errorf("zipalign: %w", err)
	}

	// ---- Stage 8: APK signing ----
	ks, err := loadOrGenerateKeystore(cfg)
	if err != nil {
		return fmt.Errorf("loading keystore: %w", err)
	}

	signed, err := sign.Sign(aligned, ks)
	if err != nil {
		return fmt.Errorf("signing APK: %w", err)
	}

	// ---- Write output atomically ----
	if err := writeAtomic(cfg.OutputPath, signed); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	return nil
}

// ConfigFromCLI resolves a BuildConfig from CLI options, reading manifest.json if available.
// CLI flags always override manifest.json fields.
func ConfigFromCLI(
	assetsDir, remoteURL, manifestPath string,
	name, pkg, versionName string,
	versionCode, minSDK, targetSDK int,
	iconColor, iconMono string,
	keystorePath, keystorePass string,
	outputPath string,
) (*config.BuildConfig, error) {
	cfg := &config.BuildConfig{
		AssetsDir:    assetsDir,
		RemoteURL:    remoteURL,
		PackageName:  pkg,
		AppName:      name,
		VersionCode:  versionCode,
		VersionName:  versionName,
		MinSDK:       minSDK,
		TargetSDK:    targetSDK,
		IconColor:    iconColor,
		IconMono:     iconMono,
		KeystorePath: keystorePath,
		KeystorePass: keystorePass,
		OutputPath:   outputPath,
	}

	// Try to load manifest.json — from explicit path or auto-detect in assets dir
	if manifestPath == "" && assetsDir != "" {
		if found, _ := manifest.FindInDir(assetsDir); found != "" {
			manifestPath = found
		}
	}
	if manifestPath != "" {
		mf, err := manifest.ParseFile(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("reading manifest.json: %w", err)
		}
		// Fill in from manifest only where CLI didn't provide values
		if cfg.AppName == "" {
			cfg.AppName = mf.AppName()
		}
		if cfg.IconColor == "" {
			if ic := mf.BestIcon("any"); ic != nil {
				cfg.IconColor = resolveIconPath(ic.Src, manifestPath, assetsDir)
			}
		}
		if cfg.IconMono == "" {
			if ic := mf.BestIcon("monochrome"); ic != nil {
				cfg.IconMono = resolveIconPath(ic.Src, manifestPath, assetsDir)
			}
		}
	}

	cfg.Defaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// packAssets gathers web asset files to include in the APK under assets/www/
// and writes assets/config.json for URL mode.
func packAssets(cfg *config.BuildConfig) ([]apkzip.Entry, error) {
	var entries []apkzip.Entry

	// Write config.json consumed by the WebView activity at runtime
	type apkConfig struct {
		StartURL string `json:"start_url,omitempty"`
	}
	ac := apkConfig{}
	if cfg.RemoteURL != "" {
		ac.StartURL = cfg.RemoteURL
	}
	cfgJSON, err := json.Marshal(ac)
	if err != nil {
		return nil, err
	}
	entries = append(entries, apkzip.NewEntry("assets/config.json", cfgJSON))

	if cfg.AssetsDir == "" {
		return entries, nil
	}

	// Walk the assets directory
	err = filepath.WalkDir(cfg.AssetsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(cfg.AssetsDir, path)
		if err != nil {
			return err
		}
		// Normalise path separators
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entries = append(entries, apkzip.NewEntry("assets/www/"+rel, data))
		return nil
	})
	return entries, err
}

// debugKeystorePath returns the path to the persistent per-user debug keystore.
// It lives at ~/.goapk/debug.keystore so it survives across builds.
func debugKeystorePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".goapk")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "debug.keystore"), nil
}

// loadOrGenerateKeystore returns the keystore specified in cfg, or loads/creates
// the persistent debug keystore at ~/.goapk/debug.keystore.
func loadOrGenerateKeystore(cfg *config.BuildConfig) (*sign.Keystore, error) {
	if cfg.KeystorePath != "" {
		pass := cfg.KeystorePass
		if pass == "" {
			pass = os.Getenv("KEYSTORE_PASSWORD")
		}
		return sign.LoadKeystore(cfg.KeystorePath, pass)
	}

	path, err := debugKeystorePath()
	if err != nil {
		return sign.GenerateDebugKeystore()
	}

	if _, err := os.Stat(path); err == nil {
		return sign.LoadKeystore(path, "")
	}

	return sign.GenerateKeystore(path, "Android Debug", "")
}

// writeAtomic writes data to path by writing to a temp file and renaming.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".goapk-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// resolveIconPath converts an icon src (relative or root-relative to the assets dir) to a
// file system path. Root-relative paths (starting with "/") are resolved against assetsDir.
// Absolute URLs (http/https) cannot be resolved to a local file and return "".
func resolveIconPath(src, manifestPath, assetsDir string) string {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return ""
	}
	// Root-relative path: /icon.png → assetsDir/icon.png
	if strings.HasPrefix(src, "/") {
		if assetsDir != "" {
			return filepath.Join(assetsDir, src[1:])
		}
		return ""
	}
	base := assetsDir
	if base == "" && manifestPath != "" {
		base = filepath.Dir(manifestPath)
	}
	if base == "" {
		return src
	}
	return filepath.Join(base, src)
}

// densityArrays returns parallel slices of sizes, density names, and APK directory suffixes.
func densityArrays() (sizes []int, names []string, suffixes []string) {
	for _, d := range config.Densities {
		sizes = append(sizes, d.IconPx)
		names = append(names, d.Name)
		suffixes = append(suffixes, d.Suffix)
	}
	return
}

// ResolveConfig is like ConfigFromCLI but accepts a pre-parsed manifest path for testing.
// Exported for use in tests.
func ResolveConfig(cfg *config.BuildConfig, manifestPath string) error {
	if manifestPath == "" {
		return nil
	}
	mf, err := manifest.ParseFile(manifestPath)
	if err != nil {
		return err
	}
	if cfg.AppName == "" {
		cfg.AppName = mf.AppName()
	}
	_ = bytes.NewBuffer // keep import
	return nil
}
