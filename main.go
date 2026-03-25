package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/zapstore/goapk/internal/build"
	"github.com/zapstore/goapk/internal/cli"
	"github.com/zapstore/goapk/internal/sign"
)

var version = "dev"

func getVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	os.Exit(run(ctx))
}

func run(ctx context.Context) int {
	opts := cli.ParseCommand()

	if opts.Global.Version {
		fmt.Printf("goapk %s\n", getVersion())
		return 0
	}

	switch opts.Command {
	case cli.CommandBuild:
		return runBuild(ctx, opts)
	case cli.CommandKeygen:
		return runKeygen(opts)
	default:
		printUsage()
		return 0
	}
}

func runBuild(ctx context.Context, opts *cli.Options) int {
	b := opts.Build

	// Resolve keystore password from flag or environment
	ksPass := b.KeystorePass
	if ksPass == "" {
		ksPass = os.Getenv("KEYSTORE_PASSWORD")
	}

	cfg, err := build.ConfigFromCLI(
		b.AssetsDir, b.URL, b.Manifest,
		b.Name, b.PackageName, b.VersionName,
		b.VersionCode, b.MinSDK, b.TargetSDK,
		b.Icon, b.IconMono,
		b.Keystore, ksPass,
		b.Output,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	if err := build.Build(ctx, cfg); err != nil {
		if errors.Is(err, context.Canceled) {
			return 130
		}
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	fmt.Println(cfg.OutputPath)
	return 0
}

func runKeygen(opts *cli.Options) int {
	k := opts.Keygen
	if k.Output == "" {
		k.Output = "release.keystore"
	}
	cn := k.CN
	if cn == "" {
		cn = "Android Release"
	}

	ks, err := sign.GenerateKeystore(k.Output, cn, "android")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating keystore: %s\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "Keystore written: %s\n", k.Output)
	fmt.Fprintf(os.Stderr, "Certificate:\n%s\n", ks.ExportCertPEM())
	return 0
}

func printUsage() {
	fmt.Printf(`goapk %s — wrap any webapp into a signed Android APK

Usage:
  goapk build [flags]     Build an APK from web assets or a URL
  goapk keygen [flags]    Generate a release keystore
  goapk version           Print version

Build flags:
  --assets <dir>          Local web assets directory
  --url <url>             Remote URL to wrap (alternative to --assets)
  --manifest <file>       Path to manifest.json (auto-detected in --assets dir)
  --name <name>           App display name (overrides manifest.json)
  --package <pkg>         Android package name, e.g. com.example.app (required)
  --version-code <n>      Version code integer (default 1)
  --version-name <s>      Version name string (default "1.0")
  --icon <file>           Color icon PNG (overrides manifest.json icons)
  --icon-mono <file>      Monochrome icon PNG
  --min-sdk <n>           Minimum API level (default 24)
  --target-sdk <n>        Target API level (default 35)
  --keystore <file>       PKCS12 keystore path (debug key auto-generated if omitted)
  --keystore-pass <pass>  Keystore password (or KEYSTORE_PASSWORD env var)
  --output <file>         Output APK path (required)

Keygen flags:
  --output <file>         Output keystore path (default: release.keystore)
  --cn <name>             Certificate common name (default: "Android Release")

Examples:
  # Wrap a local PWA (manifest.json auto-detected):
  goapk build --assets ./dist/ --package com.example.app --output app.apk

  # Wrap a remote URL:
  goapk build --url https://example.com --name "Example" --package com.example.app \
    --icon icon.png --output example.apk

  # Generate a release keystore:
  goapk keygen --output release.keystore --cn "My Company"
`, getVersion())
}
