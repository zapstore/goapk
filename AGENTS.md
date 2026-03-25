# goapk — Agent Instructions

Single-binary Go CLI that wraps any webapp into a signed Android APK without requiring the Android SDK.

All behavioral authority lives in `spec/guidelines/`. If this file conflicts, guidelines win.

## Quick Reference

| What | Where |
|------|-------|
| Architecture & patterns | `spec/guidelines/ARCHITECTURE.md` |
| Non-negotiable rules | `spec/guidelines/INVARIANTS.md` |
| Quality standards | `spec/guidelines/QUALITY_BAR.md` |
| Product vision | `spec/guidelines/VISION.md` |
| Active work | `spec/work/` |
| Decisions & learnings | `spec/knowledge/` |

## File Ownership

| Path | Owner | AI May Modify |
|------|-------|---------------|
| `spec/guidelines/*` | Human | No |
| `spec/work/*.md` | AI | Yes |
| `spec/knowledge/*.md` | AI | Yes |
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
make all                 # Cross-compile all targets
```

## Embedded Artifacts

`internal/embed/classes.dex` is a pre-compiled DEX stub committed to the repo.
To regenerate after Java changes: `make dex` (requires `d8` from Android SDK build-tools).
The stub contains no classes and produces an APK that assembles correctly but will
crash on device until replaced with the real compiled DEX.
