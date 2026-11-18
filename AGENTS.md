# goapk — Agent Instructions

Single-binary Go CLI that wraps any webapp into a signed Android APK without requiring the Android SDK.

All behavioral authority lives in `SPEC.md` (local-only, not pushed — see its vision,
architecture, invariants, and quality-bar sections). If this file conflicts, `SPEC.md` wins.

## File Ownership

| Path | Owner | AI May Modify |
|------|-------|---------------|
| `SPEC.md` | Human | No |
| `internal/**`, `main.go` | Shared | Yes |
| `java/**` | Human | Only if asked |
| `internal/embed/classes.dex` | Build artifact | Via `make dex` only |

## Key Commands

```bash
go build -o goapk .     # Build
go test ./...            # Tests
go vet ./...             # Lint
go mod tidy              # After dependency changes
make dex                 # Recompile WebView activity DEX (requires Android SDK)
make                     # Build for current platform
make help                # List targets; make build compiles and links on this machine
```

## Embedded Artifacts

`internal/embed/classes.dex` is a pre-compiled DEX stub committed to the repo.
To regenerate after Java changes: `make dex` (requires `d8` from Android SDK build-tools).
The stub contains no classes and produces an APK that assembles correctly but will
crash on device until replaced with the real compiled DEX.
