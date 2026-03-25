---
description: Quality expectations — testing, anti-patterns, AI workflow
alwaysApply: true
---

# goapk — Quality Bar

## Testing

- Table-driven tests for all pure functions (xmlbin encoding, res generation, sign, align).
- Integration tests in `internal/build` that produce a real APK from testdata PWAs and verify:
  - ZIP structure (entry order, compression, alignment)
  - Signing block presence and correct position
  - Binary XML round-trip (encode → decode with shogo82148/androidbinary)
- `testdata/pwas/` contains minimal but realistic PWA fixtures.
- No real network calls in tests.

## Implementation Expectations

- Reference the nearest existing pattern before writing new code.
- Keep `main.go` thin — dispatch only, no business logic.
- Prefer extending existing packages over creating new ones.
- Binary format code MUST include a reference comment to the relevant spec section or AOSP source.

## Anti-Patterns

- Logging or printing private keys or passwords
- Producing an unsigned APK
- Using incorrect chunk alignment in binary XML or resources.arsc
- Modifying the ZIP central directory offset without patching the EOCD record
- Swallowing errors with `_ = err`

## Working With AI

- Spec-first for changes to the APK assembly pipeline or signing code.
- If binary format behavior is uncertain, reference AOSP source before guessing.
- Never modify `spec/guidelines/` without explicit permission.

## Knowledge Entries

After significant implementation decisions, record them in `spec/knowledge/DEC-XXX-*.md`.
