# goapk

Wrap any webapp into a signed Android APK — no Android SDK, JDK, or Gradle required.

Download one binary, produce a signed APK.

## How it works

`goapk` is a **single self-contained executable**. The WebView activity DEX is embedded directly into the binary at compile time, so there is nothing to sideload or install alongside it. The output APK installs and runs on any Android 7+ device (API 24+).

## Install

### Download a pre-built binary

Grab the latest release for your platform from the [releases page](https://github.com/zapstore/goapk/releases) and put it on your `PATH`:

```bash
# macOS (Apple Silicon)
curl -L https://github.com/zapstore/goapk/releases/latest/download/goapk-darwin-arm64 -o goapk
chmod +x goapk
sudo mv goapk /usr/local/bin/

# Linux (x86-64)
curl -L https://github.com/zapstore/goapk/releases/latest/download/goapk-linux-amd64 -o goapk
chmod +x goapk
sudo mv goapk /usr/local/bin/
```

### Install from source

Requires Go 1.25+:

```bash
go install github.com/zapstore/goapk@latest
```

Or clone and build:

```bash
git clone https://github.com/zapstore/goapk
cd goapk
make        # builds ./goapk for current platform
```

## Usage

```
goapk build [flags] <output.apk>    Build an APK from web assets or a URL
goapk keygen [flags] [output.p12]   Generate a release keystore
goapk version                       Print version
```

---

## Examples

### 1. Wrap a remote PWA (simplest)

Point `goapk` at any PWA URL. It discovers the manifest, pulls the app name and icons automatically:

```bash
goapk build -s https://example.com --package com.example.app example.apk
```

`--package` is always required and must be a unique reverse-domain identifier.

You can override anything the manifest provides:

```bash
goapk build -s https://example.com \
  --name "Example" \
  --icon icon.png \
  --package com.example.app \
  example.apk
```

---

### 2. Wrap a local PWA

If your project has a `manifest.json` (or `manifest.webmanifest`), `goapk` reads the app name and icons from it automatically:

```bash
# After `npm run build` or equivalent
goapk build -s ./dist --package com.example.app app.apk
```

`-s` (or `--source`) points to the directory containing your built web files. `manifest.json` is auto-detected inside that directory.

---

### 3. Override manifest metadata

CLI flags always take precedence over `manifest.json`:

```bash
goapk build \
  -s ./dist \
  --package com.example.app \
  --name "My App" \
  --version-name "2.1.0" \
  --version-code 21 \
  --icon ./branding/icon-512.png \
  --icon-mono ./branding/icon-mono.png \
  app.apk
```

---

### 4. Release-signed APK with a keystore

For distribution outside the Play Store (e.g. Zapstore, direct sideload), generate a keystore once and reuse it for every build.

**Step 1 — generate a keystore:**

```bash
KEYSTORE_PASSWORD=secret goapk keygen --cn "My Company" release.keystore
```

This writes `release.keystore` and prints the certificate fingerprint. Keep this file safe — you need the same key to ship updates.

**Step 2 — build with the keystore:**

```bash
KEYSTORE_PASSWORD=secret goapk build \
  -s ./dist \
  --package com.example.app \
  --version-name "1.0.0" \
  --version-code 1 \
  --keystore release.keystore \
  app-release.apk
```

If `--keystore` is omitted, `goapk` generates a throwaway debug key automatically.

---

### 5. CI/CD pipeline

A minimal GitHub Actions step:

```yaml
- name: Build APK
  env:
    KEYSTORE_PASSWORD: ${{ secrets.KEYSTORE_PASSWORD }}
  run: |
    goapk build \
      -s ./dist \
      --package com.example.app \
      --version-name "${{ github.ref_name }}" \
      --version-code "${{ github.run_number }}" \
      --keystore release.keystore \
      app.apk
```

The binary is statically linked with no external dependencies, so it works in any CI environment without additional setup.

---

## All flags

### `goapk build`

| Flag | Description | Default |
|------|-------------|---------|
| `-s`, `--source <dir\|url>` | Local web assets directory or remote PWA URL | **required** |
| `--manifest <file>` | Path to `manifest.json` (local only; auto-detected) | — |
| `--name <name>` | App display name (overrides manifest) | — |
| `--package <pkg>` | Android package name, e.g. `com.example.app` | **required** |
| `--version-code <n>` | Version code integer | `1` |
| `--version-name <s>` | Version name string | `"1.0"` |
| `--icon <file>` | Color icon PNG (overrides manifest icons) | — |
| `--icon-mono <file>` | Monochrome icon PNG | — |
| `--min-sdk <n>` | Minimum API level | `24` (Android 7) |
| `--target-sdk <n>` | Target API level | `35` |
| `--keystore <file>` | PKCS12 keystore path | auto-generated debug key |
| `--keystore-pass <pass>` | Keystore password (or `KEYSTORE_PASSWORD` env var) | — |

### `goapk keygen`

| Flag / Env | Description | Default |
|------------|-------------|---------|
| `--cn <name>` | Certificate common name | `"Android Release"` |
| `KEYSTORE_PASSWORD` | Password to encrypt the keystore | no password |
| `[output.p12]` | Output path | `release.keystore` |

---

## Non-goals

- Google Play Store / AAB format
- Trusted Web Activity or Chrome Custom Tab modes
- Chromium or GeckoView embedding
- iOS support
