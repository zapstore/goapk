---
description: Product vision — what goapk is, who uses it, what success means
alwaysApply: true
---

# goapk — Vision

## What goapk Is

`goapk` wraps any webapp into a standalone Android APK without requiring the Android SDK, JDK,
Gradle, or any other Android toolchain. Download one binary, produce a signed APK.

It is used by developers building Zapstore-distributed apps, offline-first tools, and
sideload-first Android wrappers for web UIs.

## Who Uses It

- Web developers who want an Android app without learning Android development
- Zapstore app publishers distributing unsigned/self-signed APKs
- Builders of offline-first apps (including the "apocalypse use case": fully self-contained APKs
  with bundled assets and on-device LLM inference)
- CI/CD pipelines automating APK builds from web builds

## What Success Means

- A developer can wrap their PWA in one command
- The output APK installs and runs on any Android 7+ device
- Same inputs produce the same APK (reproducible builds)
- No Android SDK, JDK, or Gradle required on the developer's machine

## Non-Goals

- Play Store / AAB support
- Trusted Web Activity or Chrome Custom Tab modes
- Chromium/GeckoView embedding
- iOS support
- Compiling Java/Kotlin at `goapk build` time
