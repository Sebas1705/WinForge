# WinForge

Fast, profile-based installation of Windows apps and developer toolchains.

WinForge scans the PC, shows which apps of a **curated catalog** are installed, and lets you save, export, import and apply **profiles** - from "Essentials" to "Rust", "Android" or "Containers" - installing only what is missing.

- **Catalog**: 744 apps (a large share open source, filterable), each pinned to a [winget](https://github.com/microsoft/winget-pkgs) package. winget manifests carry the vendor's own installer URL and a SHA-256, and are moderated - that is the reliability this project builds on instead of re-hosting or scraping installers.
- **Profiles**: 31 — base (Essentials, Everyday, Creator, Gaming, Runtimes, Open-source essentials, Privacy, System tools, …) and developer (Developer base, Java/Kotlin, Android, Web/Node, Python, Go, Rust, .NET, C/C++, Containers, Cloud, Databases, AI tooling). Profiles `extend` each other and apps `require` each other, so `dev-rust` brings the MSVC build tools Rust links with, in the right order.
- **Recipes**: vetted, idempotent post-install steps (Git defaults, `rustup default stable-msvc`, WSL 2 as default, long paths, Developer Mode, `ANDROID_HOME`).
- **Detection**: winget's inventory, the Uninstall registry keys, PATH and known folders - so tools installed by hand or by a version manager count as installed.
- **Install what you choose**: a whole profile, a subset of it (per-app checkboxes), or a single app from the catalog.
- **Updates**: lists catalog apps that have a newer version (parsed from `winget upgrade`, locale-independent) and updates the ones you tick. Apps outside the catalog are never touched.
- **Portable plans**: any plan can be exported as a readable PowerShell script (`winforge-cli script <profile>` too), so a setup can be replayed on a PC without WinForge.
- **Icons and links**: each app shows its own icon and opens a detail panel with its website, publisher, license and install command.
- **PC health**: firmware (BIOS age, UEFI, Secure Boot, TPM, virtualization), drivers (missing, unsigned, old, GPU freshness), storage (disk health, free space), security (antivirus, firewall, BitLocker), Windows (support status, pending restart) and Windows Update (pending drivers and firmware). Read-only, with links to the right vendor pages.
- **Made to be scanned, not read**: a store-style catalog of icon cards (grid or list), profiles as cards with their apps' icons and a progress ring, quick-start tiles and a "popular" shelf on the home screen, a health score with one card per area, short plain-language descriptions in Spanish and English for ~100 popular apps and every built-in profile, "?" help bubbles, and a three-screen first-run guide.
- **Simple or advanced**: simple (the default) hides ids, versions, publishers' links, logs and script/winget exports; advanced shows them. Switch in the sidebar.

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

## Growing the catalog

Entries are generated from winget-pkgs manifests, so every fact (name, publisher, homepage, license, description) is the manifest's, not something typed from memory.

```bash
# 1. add "category|Winget.Id[|catalog-id[|Display name]]" lines to a file in tools/catalog-lists/
# 2. import: skips ids already in the catalog, unknown ids, pre-release channels (Beta/Nightly/...),
#    ids in tools/catalog-lists/deny.txt, and manifests without an https homepage or https installers
go run ./cmd/winforge-import -list tools/catalog-lists/general.txt -out catalogdata/apps/general-more.yml
# 3. regenerate detection rules for every app (see below) and the verification lock
go run ./cmd/winforge-import -detect
go run ./cmd/winforge-verify -lock catalog.lock.json
```

- **Open source** is derived from each app's `license` (`catalog.IsOpenSource`, conservative: proprietary, source-available and unknown are false) and drives the "Open source" filter and badge.
- **Detection**: winget only knows about apps it installed or could correlate. `-detect` writes `catalogdata/detect/detect.yml` from each package's *Apps & features* name, so an app installed by hand is still recognised. Rules are version-tolerant (`Git` matches "Git version 2.50" but not "Git LFS").
- **Denying**: put an id in `deny.txt` with a reason to keep it out for good (adware-style bundlers, end-of-life runtimes, unofficial builds).
- The importer waits out GitHub's API limit and remembers ids that do not exist (`tools/catalog-lists/.notfound`, git-ignored); export `GITHUB_TOKEN` for the 5000/hour budget.

Apps that are not in winget (Gradle, Maven) are deliberately absent rather than served from an unofficial source; projects use their wrappers.

## PC health

`winforge-cli health [updates]` or the *PC health* page run one read-only PowerShell scan (CIM/WMI queries, no installs, no network) and turn it into checks. Passing checks count too, so the page can say "16 of 17 checks fine"; a check that needs administrator rights and cannot be answered is *unknown*, never a failure.

- **Drivers**: Windows' own in-box drivers (version `10.0.x`, "(Standard ...)" makers, dates like 2006) are never called old. Devices with a problem code are named by hardware id and vendor (`ACPI\RTK5452` is Realtek), because Windows often gives them no name.
- **BIOS and firmware**: WinForge shows the version, date and age, and links to the maker's support page and a search for your exact board. It does not guess "the latest version": vendors publish it in incompatible ways, and a wrong answer on firmware is worse than none. Windows Update is queried for pending driver and firmware items (it never downloads or installs).
- **Privacy**: no serial numbers, user names or addresses are collected, so an exported Markdown report can be pasted into a forum.
- **Wording**: findings are keys plus parameters, worded in the interface language; a test checks every key the Go analyzer can emit has text in both languages.
- Run `go test -tags live -run Live -v ./internal/health` on a Windows PC to see the raw scan.

## Plain-language text

Winget manifests are written in English for maintainers. For what people actually read: `catalogdata/featured.yml` gives ~100 popular apps a one-line description in each language (validated: known id, both languages, at most 60 characters; its order is the "Popular" shelf), and `frontend/src/lib/profileText.ts` does the same for every built-in profile (a test fails if a profile has none). Everything else falls back to the catalog's own text. Profiles you create keep your words.

## Icons

`go run ./cmd/winforge-icons` downloads each app's icon into `frontend/public/icons` (shipped inside the app, so the interface never contacts a vendor). Sources, in order: the GitHub owner avatar for GitHub-hosted projects, the winget manifest's own icon, the icons the homepage declares, then `/favicon.ico`. Content is identified by its bytes, SVGs with scripts are refused, and an icon that is byte-identical across three or more different brands (a host's default) is dropped, leaving the initials avatar. Icons are the projects' own marks, shown only to identify the app. **GitHub avatars are used only for organizations**: a personal account's avatar is a photo of a person, so projects owned by an individual show initials instead (`go run ./cmd/winforge-icons -github` re-applies the rule).

## Develop

```bash
cd frontend && npm ci && cd ..
go test ./...                 # core logic
wails dev                     # app with hot reload (http://localhost:34115 for browser debugging)
wails build -nsis             # portable exe + installer
go run ./cmd/winforge-cli scan
```

Requires Go 1.26, Node 22 and the [Wails CLI](https://wails.io) v2.

UI tests (`cd frontend && npm test`) drive the whole app against a mocked Go API: install flow, profiles, health, language parity, dialogs and focus handling. Dialogs share one modal stack (`useModal`), a boundary catches render crashes, and text on accent colors uses `--accent-ink` to keep contrast in both themes.

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

The gear button sets language (automatic, Spanish, English), theme (system, dark, light), accent colour and list density; choices persist per user. Catalog text (app names, descriptions, profile names) comes from the catalog and is not translated.

The visual language is "steel and ember": slate surfaces and one warm accent that lights only what is done or chosen. Its recurring motif is the **tally strip**, one cell per app (or per step during an install), lit when installed; the home screen shows every catalog app as one tick on a ruler. Fonts (Bricolage Grotesque, IBM Plex Sans and Mono, latin subsets) are bundled, so the app works offline. The logo source is `frontend/src/logo.svg`; `build/appicon.png` and `build/windows/icon.ico` are rendered from it.
