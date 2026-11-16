# Feature ideas

What could come next, ranked, with the reasoning. This is a list of candidates, not a promise: an
idea becomes a decision when it gets an ADR or a plan.

Written on 2026-11-14 from what v0.10.x already does and from a look at comparable tools:
[UniGetUI](https://www.neowin.net/software/unigetui-330/), Microsoft's
[`winget configure`](https://learn.microsoft.com/windows/package-manager/winget/configure),
[WinUtil](https://www.ghacks.net/2024/08/12/windows-utility-winutil-is-a-one-stop-shop-to-customize-windows/)
and [Ninite Pro](https://ninite.com/pro-classic). Those comparisons come from secondary sources; check
the tool's own repository before copying a behavior.

## What exists today

Catalog and detection, profiles, recipes, updates (apply what is ticked), PC health and repairs,
storage cleanup, settings backup, emulation and games, six design styles, keyboard and screen-reader
support. See the [README](../README.md).

Not present: anything that runs on a schedule, a startup manager, cleanup of developer caches
(Docker/WSL disks, `cargo`, `pnpm`, stale `node_modules`).

## Candidates

| # | Idea | Why | Effort | Risk |
|---|---|---|---|---|
| 1 | Ignore, skip a version or pause updates per app (done) | Updates apply what is ticked, with no memory. The most asked-for control in UniGetUI. | Low | Low |
| 2 | Scheduled automatic updates (a scheduled task running `winforge-cli`, with a notification) | It is what Ninite Pro charges for. Today WinForge only acts while open. | Medium | Medium |
| 3 | Developer cache cleanup (done) | Where developers lose tens of GB; fits the Storage tab. Regenerable, but slow to download again. | Medium | Medium |
| 4 | Export a profile as a `winget configure` file (done) | Replays a setup on a PC without WinForge and fits in a repository. Script and `winget import` export already exist. | Low to medium | Low |
| 5 | Startup manager (done for Run keys and Startup folders; tasks and services are not listed; disable, never delete) | Boot time review; nothing covers it today. | Medium | Low (reversible) |
| 6 | Privacy and Windows cleanup as reversible recipes | Most asked-for in WinUtil and O&O. Each tweak needs a preview, an undo and a restore point. | Medium to high | High unless each one is reviewed |
| 7 | End-of-life and known-vulnerability report for installed software | Read-only, built on the catalog's verified publishers. | Medium | Low |
| 8 | Profile drift ("what changed on this PC since you applied it") (done) | Turns profiles into something maintained, not installed once. | Medium | Low |
| 9 | Profiles folder that can live in OneDrive or Dropbox (done) | Sync between PCs with no server and almost no code. | Low | Low |
| 10 | Action palette (done: Ctrl+K opens it) | Daily use by keyboard. | Low | Low |

### Second round (2026-11-16)

Written after versions A to C shipped (v0.13.0). Built from what WinForge now has, the gaps those versions
left open, and what comparable tools are known for (UniGetUI backups and notifications, Microsoft's
[Windows developer configuration](https://github.com/microsoft/WindowsDeveloperConfig), uninstallers and
PATH tools). A web search for each tool's most requested features found little, so none of the "why" below
is measured demand: check the other tool's own repository before copying a behavior.

| # | Idea | Why | Effort | Risk |
|---|---|---|---|---|
| 11 | Update and install history, with "go back to the previous version" (`winget install --version`) | Updates are the one action people regret. WinForge already knows the version before and after; it only does not keep it. | Medium | Low |
| 12 | Uninstall center: catalog apps first, then the leftovers (AppData, ProgramData, empty Start menu entries) shown for review and sent to the Recycle Bin | Completes the lifecycle (install, update, ignore, remove). Leftovers are the part people pay Revo-style tools for. | Medium to high | Medium: leftovers are guessed, so each is listed, never deleted by default |
| 13 | (done) Environment doctor: broken or duplicate PATH entries, `JAVA_HOME` or `ANDROID_HOME` pointing nowhere, several Pythons or JDKs and which one wins | A developer-only problem that no cleanup or health scan looks at, and the owner's own PCs are exactly that case. Read-only first, fixes later. | Medium | Low (read-only), medium (fixes: user variables only, with an undo list) |
| 14 | (done) Windows features and WSL setup as recipes (WSL, Virtual Machine Platform, Hyper-V, Sandbox, .NET 3.5), with the restart handled | The first thing a new developer PC needs. A recipe fits the existing runner, restore point and retry. | Medium | Medium: needs administrator rights and a restart |
| 15 | Health history: keep each scan's score and show the trend, plus a laptop battery report (`powercfg /batteryreport`) | Today a scan is a snapshot. A falling SSD or battery figure is only visible over time. | Low to medium | Low |
| 16 | Services and scheduled tasks viewer, third-party only, with a reversible switch (completes #5) | The half of "what runs at boot" that the startup page leaves out. | Medium | Medium: only items not from Microsoft, startup type only, never delete |
| 17 | Offline kit: download the installers of a profile with their hashes to a folder or USB, and install from it | For a PC with no network or a slow one. Hashes already come from winget manifests. | Medium | Low |
| 18 | Compare this PC with a profile file or an exported setup from another PC | Drift (#8) compares with the past; this compares two machines. | Low to medium | Low |
| 19 | Tray mode with an update count and a notification (needs #2) | The visible side of scheduled updates. Without it, scheduling surprises people. | Medium | Low |
| 20 | Space map (treemap) in Biggest things | The same data as today, easier to read. Pure interface work. | Medium | Low |
| 21 | Diagnostic bundle (versions, scan results, log; no file names outside WinForge's own) and a "suggest this app" link that opens a pre-filled GitHub issue | Makes bug reports and catalog growth cheap. Nothing is sent by WinForge itself. | Low | Low |
| 22 | Unattended mode for `winforge-cli`: machine-readable report and stable exit codes | For provisioning scripts and Intune-style use. Mostly formatting what exists. | Low | Low |

## Not worth doing

- A registry cleaner, or a generic driver updater: they cause more damage than they fix. PC health
  already reports driver problems.
- Removing Defender or disabling UAC in bulk: against the trust model below.
- Running code from imported profiles: profiles stay data only (see the README's trust model).

## Suggested order

1. **A (done):** 1, 9, 10. Ignoring updates, a profiles folder that can live in OneDrive or Dropbox, and the
   command palette.
2. **B (done):** 4, 8. Profiles as portable configuration, and a record of how the PC drifts from them.
3. **C (done):** 3, 5. Developer caches, Docker, old project folders and virtual disks in Storage, and a startup manager.
4. **D:** 2, 19. Needs care because it acts while nobody is watching; the tray and notification make it visible.
5. **E:** 11, 12. Managing what is installed: go back a version, and remove an app with its leftovers.
6. **F (done):** 13, 14. The developer PC: an environment doctor, and WSL and Windows features as recipes.
7. **G:** 15, 16. What the PC does over time and at boot: health history, services and tasks.
8. **Later:** 6, 7 (each reviewed one recipe at a time), then 17, 18, 20, 21, 22.

## Constraints every idea has to respect

- Only catalog apps are ever touched; anything else on the PC is left alone.
- Profiles are data. Scripts exist only as reviewed recipes.
- Anything destructive asks first, says what it will do, and prefers the Recycle Bin or a reversible
  switch over deleting.
- Anything that changes Windows settings creates a restore point first and can be undone.
- Every screen works in Spanish and English, with the keyboard and with a screen reader.
