// Package cli handles command-line flag parsing and subcommand dispatch.
package cli

// Command identifies which subcommand was invoked.
type Command int

const (
	CommandNone   Command = iota
	CommandBuild          // goapk build
	CommandKeygen         // goapk keygen
)

// Options holds the parsed result of all CLI flags.
type Options struct {
	Command Command
	Args    []string

	Global GlobalOptions
	Build  BuildOptions
	Keygen KeygenOptions
}

// GlobalOptions are flags available on all subcommands.
type GlobalOptions struct {
	Help    bool
	Version bool
}

// BuildOptions are flags for the `build` subcommand.
type BuildOptions struct {
	// Content — local directory or remote PWA URL
	Source   string
	Manifest string // explicit manifest.json path (local only); auto-detected if empty

	// App identity (override manifest.json)
	Name        string
	PackageName string
	VersionCode int
	VersionName string

	// Icons (override manifest.json)
	Icon     string // color icon PNG
	IconMono string // monochrome icon PNG

	// Permissions — comma-separated web permission names (e.g. "camera,microphone,geolocation")
	Permissions string

	// SDK
	MinSDK    int
	TargetSDK int

	// Signing
	Keystore     string
	KeystorePass string

	// Output
	Output string
}

// KeygenOptions are flags for the `keygen` subcommand.
type KeygenOptions struct {
	Output string
	CN     string // certificate common name
}
