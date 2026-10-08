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

The gaming section (`gaming/emulators`, `gaming/mods`, `gaming/engines`, `gaming/launchers`) was built from `winget search --tag emulator|emulation|retro|mod` plus keyword searches; the list is `tools/catalog-lists/gaming.txt` and only apps that exist in winget-pkgs can be in it (RPCS3, Snes9x or Cheat Engine, for example, are not published there, so they are not here). Cards get a Spanish and English line from `catalogdata/taglines.yml`, which, unlike `featured.yml`, does not add the app to the popular shelf. The catalog shows a sub-category row under big categories such as Gaming and Development.

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

## When things go wrong

A failed step says what to do about it in plain words (needs administrator rights, another installer is running, the download failed, out of disk space...), decided from winget's exit code and the installer's own. **Retry what failed** runs only the steps that did not finish; when administrator rights are the cause, one button restarts WinForge elevated. What a run leaves undone is stored in `%AppData%\WinForge\pending.json`, so after a Windows restart, a crash or an elevated relaunch the app offers to continue where it stopped.

## Settings backup

The *Settings backup* page has two halves.

**Create a backup** puts into one zip any of: the list of catalog apps installed on this PC (as a WinForge profile), the configuration of ~30 apps, and folders you add yourself (documents, projects, saves; up to 2 GB). The settings come from a fixed list in `internal/settings/sets.go`: VS Code and its forks (with the extension list), JetBrains IDEs, Sublime, Vim/Neovim, Git, GitHub CLI, Windows Terminal, Alacritty, WezTerm, PowerShell, shell and Starship, SSH config, Docker Desktop, WSL, Cargo, Notepad++, PowerToys, Flow Launcher, AltSnap, AutoHotkey, ShareX, OBS scenes, VLC, mpv and winget. Only listed files are read; browser profiles, SSH keys, tokens, saved logins and stream keys are never on the list (a test enforces it). Every entry gets a SHA-256 in the manifest, and the file is read back and verified right after it is written.

**Rebuild from a backup** is a three-stage wizard for a new PC: (1) install the apps the backup lists and this PC lacks, with the usual plan, progress and retry; (2) restore the settings, after the apps exist, with a preview of what is new, changed or already identical; (3) restore the folders into a place you choose. It checks the archive first and refuses entries that were altered or added after the backup was made. Existing files are never lost: a replaced setting is kept as `*.winforge-bak`, and folders never overwrite anything. Entries that do not match the fixed list, or try to leave their folder, are skipped.

## Games for your emulators

The *Games for emulators* page lists the emulators WinForge finds installed on this PC and, for each system they play, where to look for games: free and open-source games, homebrew, ROM hacks, fan translations and randomizers (`catalogdata/emulation.yml`). Every entry is a link to the project's own page, opened in your browser. **WinForge does not download or host games**, and a test refuses links to sites that share commercial games without permission. Hacks, translations and randomizers are patches with no game data, so the page says they need your own copy of the original.

**Installable games.** For the systems your emulators play, the page also lists free games that can be installed in one click (`catalogdata/games.yml`: the freeware adventures ScummVM publishes, Freedoom, Libbet, the public-domain Mystery House...). Only games whose authors allow free redistribution are listed, each with the SHA-256 of the exact file: a download that does not match is discarded, downloads are https-only from `downloads.scummvm.org` and `github.com`, and a zip cannot write outside the game's folder. Games go to `Documents\WinForge Games\<system>\<game>` (changeable), and each folder carries a receipt so only what WinForge installed is ever removed. WinForge then finds the emulator on disk (PATH, Program Files, WinGet packages) to start the game with **Play**, or tells ScummVM about it with `--add`; emulators whose command line is not known are not launched, and the page says where the game is. Hacks, translations and randomizers are not installed this way: they need your own copy of the original game.

The same page has a **patch tool**: choose your game and an IPS, UPS or BPS patch and WinForge writes `<game> (patched)` next to it. UPS and BPS carry the checksum of the game they were made for, so a wrong version is refused with a clear message; an old 512-byte SNES copier header is handled. The original is never changed and nothing existing is overwritten (`internal/patch`).

## Sharing and shortcuts

A profile can be copied as a **share code**: one line of text (`WF1.…`) to paste in a message, imported through *Profiles > Import from code*. `Ctrl+K` (or `/`) jumps to catalog search from anywhere. Apps can be uninstalled from their detail panel, after a confirmation.

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
