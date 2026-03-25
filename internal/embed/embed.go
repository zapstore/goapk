// Package embed holds pre-compiled Android artifacts embedded in the goapk binary.
package embed

import _ "embed"

// ClassesDEX is the pre-compiled WebView activity DEX.
// Replace internal/embed/classes.dex by running `make dex` (requires Android SDK).
// The committed file is a minimal stub (no classes); it produces an APK that assembles
// correctly but will crash on device until replaced with the real compiled DEX.
//
//go:embed classes.dex
var ClassesDEX []byte
