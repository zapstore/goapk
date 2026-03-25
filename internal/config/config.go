// Package config defines and validates the BuildConfig for an APK build.
package config

import (
	"fmt"
	"regexp"
	"strings"
)

// Density is an Android screen density bucket.
type Density struct {
	Name   string // e.g. "mdpi"
	DPI    int    // nominal DPI
	IconPx int    // launcher icon size in pixels
	Suffix string // res directory suffix e.g. "mdpi-v4"
}

// Densities lists all supported Android launcher icon density buckets, smallest to largest.
var Densities = []Density{
	{"mdpi", 160, 48, "mdpi-v4"},
	{"hdpi", 240, 72, "hdpi-v4"},
	{"xhdpi", 320, 96, "xhdpi-v4"},
	{"xxhdpi", 480, 144, "xxhdpi-v4"},
	{"xxxhdpi", 640, 192, "xxxhdpi-v4"},
}

// ResID constants for the app's own resources (package 0x7f).
const (
	ResIDAppName     = uint32(0x7f010000) // @string/app_name
	ResIDIconColor   = uint32(0x7f020000) // @mipmap/ic_launcher
	ResIDIconMono    = uint32(0x7f020001) // @mipmap/ic_launcher_mono
)

// BuildConfig holds all validated parameters for an APK build.
type BuildConfig struct {
	// App identity
	AppName     string // display name
	PackageName string // Java package name e.g. "com.example.myapp"
	VersionCode int    // integer version (default 1)
	VersionName string // display version (default "1.0")

	// SDK targeting
	MinSDK    int // default 24 (Android 7.0)
	TargetSDK int // default 35

	// Content
	AssetsDir string // path to local web assets directory (may be empty)
	RemoteURL string // remote URL to load at runtime (may be empty)

	// Icons — paths to source PNG files; at least one of IconColor must be set
	IconColor string // color (any-purpose) icon PNG path
	IconMono  string // monochrome icon PNG path (optional)

	// Signing
	KeystorePath string // PKCS12 keystore path; empty = generate debug key
	KeystorePass string // keystore password

	// Output
	OutputPath string // .apk output path
}

var packageNameRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*)+$`)

// Validate returns an error if the config has missing or invalid fields.
func (c *BuildConfig) Validate() error {
	if strings.TrimSpace(c.AppName) == "" {
		return fmt.Errorf("app name is required (--name or manifest.json)")
	}
	if !packageNameRE.MatchString(c.PackageName) {
		return fmt.Errorf("invalid package name %q: must be a valid Java package (e.g. com.example.app)", c.PackageName)
	}
	if c.VersionCode < 1 {
		return fmt.Errorf("version code must be >= 1")
	}
	if strings.TrimSpace(c.VersionName) == "" {
		return fmt.Errorf("version name is required")
	}
	if c.MinSDK < 24 {
		return fmt.Errorf("min SDK must be >= 24 (Android 7.0)")
	}
	if c.TargetSDK < c.MinSDK {
		return fmt.Errorf("target SDK (%d) must be >= min SDK (%d)", c.TargetSDK, c.MinSDK)
	}
	if c.AssetsDir == "" && c.RemoteURL == "" {
		return fmt.Errorf("either --assets or --url is required")
	}
	if c.IconColor == "" {
		return fmt.Errorf("an icon is required (--icon or manifest.json icons)")
	}
	if strings.TrimSpace(c.OutputPath) == "" {
		return fmt.Errorf("output path is required (--output)")
	}
	return nil
}

// ActivityClass returns the fully qualified class name for the WebView activity.
// The embedded classes.dex always defines this fixed runtime class; the user's
// package name controls app identity via the <manifest package="..."> attribute only.
func (c *BuildConfig) ActivityClass() string {
	return "com.zapstore.goapk.runtime.WebViewActivity"
}

// Defaults fills in any zero-value fields with their defaults.
func (c *BuildConfig) Defaults() {
	if c.VersionCode == 0 {
		c.VersionCode = 1
	}
	if c.VersionName == "" {
		c.VersionName = "1.0"
	}
	if c.MinSDK == 0 {
		c.MinSDK = 24
	}
	if c.TargetSDK == 0 {
		c.TargetSDK = 35
	}
}
