package cli

import (
	"flag"
	"os"
)

// ParseCommand parses os.Args and returns an Options struct.
func ParseCommand() *Options {
	opts := &Options{}

	if len(os.Args) < 2 {
		return opts
	}

	switch os.Args[1] {
	case "build":
		opts.Command = CommandBuild
		opts.Args = parseBuild(&opts.Build, os.Args[2:])
	case "keygen":
		opts.Command = CommandKeygen
		opts.Args = parseKeygen(&opts.Keygen, os.Args[2:])
	case "help", "--help", "-h":
		opts.Global.Help = true
	case "version", "--version", "-v":
		opts.Global.Version = true
	}

	return opts
}

func parseBuild(b *BuildOptions, args []string) []string {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)

	fs.StringVar(&b.AssetsDir, "assets", "", "local web assets directory")
	fs.StringVar(&b.URL, "url", "", "remote URL to wrap")
	fs.StringVar(&b.Manifest, "manifest", "", "path to manifest.json (auto-detected if not set)")
	fs.StringVar(&b.Name, "name", "", "app display name")
	fs.StringVar(&b.PackageName, "package", "", "Android package name (required)")
	fs.IntVar(&b.VersionCode, "version-code", 0, "version code integer (default 1)")
	fs.StringVar(&b.VersionName, "version-name", "", "version name string (default \"1.0\")")
	fs.StringVar(&b.Icon, "icon", "", "color icon PNG path")
	fs.StringVar(&b.IconMono, "icon-mono", "", "monochrome icon PNG path")
	fs.IntVar(&b.MinSDK, "min-sdk", 0, "minimum API level (default 24)")
	fs.IntVar(&b.TargetSDK, "target-sdk", 0, "target API level (default 35)")
	fs.StringVar(&b.Keystore, "keystore", "", "PKCS12 keystore path (debug key generated if omitted)")
	fs.StringVar(&b.KeystorePass, "keystore-pass", "", "keystore password (or KEYSTORE_PASSWORD env var)")
	fs.StringVar(&b.Output, "output", "", "output APK path (required)")

	// Ignore parse errors; unrecognised flags fall through to positional args
	_ = fs.Parse(args)
	return fs.Args()
}

func parseKeygen(k *KeygenOptions, args []string) []string {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)

	fs.StringVar(&k.Output, "output", "release.keystore", "output keystore path")
	fs.StringVar(&k.CN, "cn", "Android Release", "certificate common name")

	_ = fs.Parse(args)
	return fs.Args()
}
