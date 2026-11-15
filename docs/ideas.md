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
| 3 | Developer cache cleanup | Where developers lose tens of GB; fits the Storage tab. Regenerable, but slow to download again. | Medium | Medium |
| 4 | Export a profile as a `winget configure` file (done) | Replays a setup on a PC without WinForge and fits in a repository. Script and `winget import` export already exist. | Low to medium | Low |
| 5 | Startup manager (programs, tasks, services; disable, never delete) | Boot time review; nothing covers it today. | Medium | Low (reversible) |
| 6 | Privacy and Windows cleanup as reversible recipes | Most asked-for in WinUtil and O&O. Each tweak needs a preview, an undo and a restore point. | Medium to high | High unless each one is reviewed |
| 7 | End-of-life and known-vulnerability report for installed software | Read-only, built on the catalog's verified publishers. | Medium | Low |
| 8 | Profile drift ("what changed on this PC since you applied it") (done) | Turns profiles into something maintained, not installed once. | Medium | Low |
| 9 | Profiles folder that can live in OneDrive or Dropbox (done) | Sync between PCs with no server and almost no code. | Low | Low |
| 10 | Action palette (done: Ctrl+K opens it) | Daily use by keyboard. | Low | Low |

## Not worth doing

- A registry cleaner, or a generic driver updater: they cause more damage than they fix. PC health
  already reports driver problems.
- Removing Defender or disabling UAC in bulk: against the trust model below.
- Running code from imported profiles: profiles stay data only (see the README's trust model).

## Suggested order

1. **A (done):** 1, 9, 10. Ignoring updates, a profiles folder that can live in OneDrive or Dropbox, and the
   command palette.
2. **B (done):** 4, 8. Profiles as portable configuration, and a record of how the PC drifts from them.
3. **C:** 3, 5. Most of the value for developers.
4. **D:** 2. Needs care because it acts while nobody is watching.
5. **Later:** 6, 7, each reviewed one recipe at a time.

## Constraints every idea has to respect

- Only catalog apps are ever touched; anything else on the PC is left alone.
- Profiles are data. Scripts exist only as reviewed recipes.
- Anything destructive asks first, says what it will do, and prefers the Recycle Bin or a reversible
  switch over deleting.
- Anything that changes Windows settings creates a restore point first and can be undone.
- Every screen works in Spanish and English, with the keyboard and with a screen reader.
