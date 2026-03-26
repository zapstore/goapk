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

	ksPass := b.KeystorePass
	if ksPass == "" {
		ksPass = os.Getenv("KEYSTORE_PASSWORD")
	}

	cfg, cleanup, err := build.ConfigFromCLI(
		ctx,
		b.Source, b.Manifest,
		b.Name, b.PackageName, b.VersionName,
		b.VersionCode, b.MinSDK, b.TargetSDK,
		b.Icon, b.IconMono,
		b.Permissions,
		b.Keystore, ksPass,
		b.Output,
	)
	if cleanup != nil {
		defer cleanup()
	}
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

	pass := os.Getenv("KEYSTORE_PASSWORD")
	ks, err := sign.GenerateKeystore(k.Output, cn, pass)
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
  goapk build [flags] <output.apk>    Build an APK from web assets or a URL
  goapk keygen [flags] [output.p12]   Generate a release keystore
  goapk version                       Print version

Build flags:
  -s, --source <dir|url>  Local web assets directory or remote PWA URL
  --manifest <file>       Path to manifest.json (local only; auto-detected)
  --name <name>           App display name (overrides manifest.json)
  --package <pkg>         Android package name, e.g. com.example.app (required)
  --version-code <n>      Version code integer (default 1)
  --version-name <s>      Version name string (default "1.0")
  --icon <file>           Color icon PNG (overrides manifest.json icons)
  --icon-mono <file>      Monochrome icon PNG
  --permissions <list>    Comma-separated web permissions (camera,microphone,geolocation,
                          notifications,nfc,bluetooth,background-sync)
  --min-sdk <n>           Minimum API level (default 24)
  --target-sdk <n>        Target API level (default 35)
  --keystore <file>       PKCS12 keystore path (debug key auto-generated if omitted)
  --keystore-pass <pass>  Keystore password (or KEYSTORE_PASSWORD env var)

Keygen flags:
  --cn <name>             Certificate common name (default: "Android Release")
  KEYSTORE_PASSWORD env   Password to encrypt the keystore (default: no password)

Examples:
  # Wrap a local PWA (manifest.json auto-detected):
  goapk build -s ./dist --package com.example.app app.apk

  # Wrap a remote PWA (manifest + icons auto-discovered):
  goapk build -s https://example.com --package com.example.app example.apk

  # Override name and icon for a remote PWA:
  goapk build -s https://example.com --name "Example" --icon icon.png \
    --package com.example.app example.apk

  # Generate a release keystore:
  KEYSTORE_PASSWORD=secret goapk keygen --cn "My Company" release.keystore
  KEYSTORE_PASSWORD=secret goapk build --keystore release.keystore \
    -s ./dist --package com.example.app app.apk
`, getVersion())
}
