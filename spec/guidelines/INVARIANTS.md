---
description: Non-negotiable invariants — APK correctness, signing, CLI behavior
alwaysApply: true
---

# goapk — Invariants

These are non-negotiable. Violating any invariant is a bug.

## APK Correctness

- The output APK MUST be installable via `adb install` and Android sideloading.
- `AndroidManifest.xml` MUST be valid binary XML with correct chunk headers and alignment.
- `resources.arsc` MUST be a valid resource table; app name and icon references must resolve.
- All ZIP entries MUST be in the correct order: manifest first, then dex, resources.arsc, res/, assets/, lib/.
- STORED (uncompressed) entries MUST be 4-byte aligned after zipalign.
- The APK signing block MUST be inserted at exactly the right position (between ZIP data and Central Directory).

## Signing

- Every output APK MUST be signed (APK Signature Scheme v2 minimum).
- If no keystore is provided, generate a debug keystore automatically — never produce an unsigned APK.
- Private key material MUST never be logged, printed, or included in error messages.
- Keystore passwords MUST never be logged.

## Manifest Metadata

- `android:package` MUST be a valid Java package name (no spaces, dots as separators).
- `android:exported="true"` MUST be set on the launcher activity (required since Android 12).
- `android:minSdkVersion` MUST be at least 24 (Android 7.0).

## CLI Behavior

- Status output goes to stderr. The output APK path goes to stdout on success.
- Exit 0 = success. Exit 1 = error. Exit 130 = Ctrl+C / context cancelled.
- `--output` path MUST be written atomically (write to temp, then rename).

## Error Handling

- All errors MUST be wrapped with context: `fmt.Errorf("doing X: %w", err)`.
- Errors MUST propagate up; never swallowed silently.
- User-facing error messages MUST be actionable.

## Context Cancellation

- All long-running operations MUST respect `context.Context`.
- Ctrl+C MUST cleanly cancel in-progress builds.
