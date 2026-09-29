# WinForge

Fast, profile-based installation of Windows apps and developer toolchains.

WinForge scans the PC, shows which apps of a **curated catalog** are installed, and lets you save, export, import and apply **profiles** - from "Essentials" to "Rust", "Android" or "Containers" - installing only what is missing.

- **Catalog**: 86 apps, each pinned to a [winget](https://github.com/microsoft/winget-pkgs) package. winget manifests carry the vendor's own installer URL and a SHA-256, and are moderated - that is the reliability this project builds on instead of re-hosting or scraping installers.
- **Profiles**: base (Essentials, Everyday, Creator, Gaming) and developer (Developer base, Java/Kotlin, Android, Web/Node, Python, Go, Rust, .NET, C/C++, Containers, Cloud, Databases, AI tooling). Profiles `extend` each other and apps `require` each other, so `dev-rust` brings the MSVC build tools Rust links with, in the right order.
- **Recipes**: vetted, idempotent post-install steps (Git defaults, `rustup default stable-msvc`, WSL 2 as default, long paths, Developer Mode, `ANDROID_HOME`).
- **Detection**: winget's inventory, the Uninstall registry keys, PATH and known folders - so tools installed by hand or by a version manager count as installed.

## Trust model

| Thing | Where it lives | Can it run code? |
|---|---|---|
| Catalog apps | `catalogdata/apps/*.yml`, embedded in the binary | Only through winget, with the manifest's hashed installer |
| Recipes | `catalogdata/recipes/*.yml`, embedded | Yes - this is the only place scripts exist, reviewed in PRs |
| Profile files (yours or imported) | JSON, data only | **No.** They reference catalog ids and recipe ids; unknown ids are dropped, unknown fields rejected |

`winforge-verify` checks every catalog entry against `microsoft/winget-pkgs`: the package exists, its **publisher matches the one the catalog expects** (catches hijacked or typosquatted ids), and every installer is HTTPS. It writes `catalog.lock.json` (version, publisher, installer hosts and hashes) so any change is a reviewable diff. CI runs it on every push and weekly.

## Layout

```
catalogdata/     apps, profiles, recipes (YAML, embedded)
internal/
  catalog/       model, strict loader, validation, profile resolution
  system/        winget export, registry, PATH detection, elevation
  install/       plan (diff against the PC) and runner (winget + recipes)
  profile/       store, export/import, `winget import` export
  updater/       release lookup, semver compare, SHA-256-verified installer download
cmd/winforge-cli headless: scan | profiles | plan | apply | export | check
cmd/winforge-verify  catalog verifier
app.go, main.go, frontend/   Wails desktop app (Go + React/TypeScript)
```

The desktop stack, CI and release pipeline follow [Templetry's desktop app](https://github.com/Templetry/desktop). If the pattern proves out, a `go/wails-desktop` form in the Templetry catalog would generate the same skeleton.

## Adding an app

1. Find the winget id: `winget search <name>`.
2. Add an entry to a file in `catalogdata/apps/` (`publisher` must equal the manifest's `Publisher`; `detect` adds PATH/registry/folder hints for tools winget may not know about).
3. `go run ./cmd/winforge-verify -lock catalog.lock.json` and commit the lock diff.

Apps that are not in winget (Gradle, Maven) are deliberately absent rather than served from an unofficial source; projects use their wrappers.

## Develop

```bash
cd frontend && npm ci && cd ..
go test ./...                 # core logic
wails dev                     # app with hot reload (http://localhost:34115 for browser debugging)
wails build -nsis             # portable exe + installer
go run ./cmd/winforge-cli scan
```

Requires Go 1.26, Node 22 and the [Wails CLI](https://wails.io) v2.

## Releases, installer and updates

- **CI** (`ci.yml`): frontend tests + build, gofmt, vet and Go tests on Windows; portable-logic tests on Linux; the catalog verifier (weekly too); and an **installer job** that builds the NSIS installer, installs it silently, starts the app, uninstalls silently and fails if anything is left behind.
- **Release** (`release.yml`): push a tag `vX.Y.Z` to publish. Tests gate the build; the tag is stamped into the binary and the installer; assets are `WinForge-<tag>-windows-installer.exe`, the portable `.exe` and `SHA256SUMS`. Tags with a suffix (`-rc1`) publish as pre-releases so they never reach the updater. Run the workflow by hand for a dry run that publishes nothing.
- **Updater** (in the app, Appearance > Updates): checks GitHub's latest release at startup (can be turned off) and offers **Update now**. It downloads the installer, **verifies it against the release's `SHA256SUMS`** (and that the URLs belong to this repository), then launches it and quits. Development builds are never offered updates.

## Roadmap

- Portable/zip installs with pinned hashes for tools missing from winget (Gradle, Maven, Android command-line tools).
- Per-app options in profiles (scope, install location) and version pinning UI.
- Upgrade view (`winget upgrade` for catalog apps) and drift detection against a profile.
- Signed releases; Scoop/winget publication of WinForge itself.
- Spanish UI.

## Appearance

The gear button sets theme (system, dark, light), accent colour and list density; choices persist per user. The logo source is `frontend/src/logo.svg`; `build/appicon.png` and `build/windows/icon.ico` are rendered from it.
