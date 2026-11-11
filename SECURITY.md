# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub:
[Report a vulnerability](https://github.com/Sebas1705/WinForge/security/advisories/new).
Do not open a public issue for a security problem. This is a one-person
project, so expect a reply within a few days, not hours.

## Supported versions

Only the latest release receives fixes.

## What to know about the releases

- **Releases are not code-signed.** The installer and the portable `.exe` carry
  no Authenticode signature, so Windows SmartScreen may warn on first run.
- Each release publishes a `SHA256SUMS` file. You can verify a download against
  it by hand.
- The in-app updater downloads the installer of the latest GitHub release and
  checks its SHA-256 against the `SHA256SUMS` of **the same release**. That
  protects against corrupted or truncated downloads and against a swapped
  installer file, but it is not a signature: whoever can publish to the
  repository's releases can publish matching hashes. The installer is saved to
  a fresh per-run directory that only the current user can access, and
  Windows shows a UAC prompt before it runs.
- Installing apps goes through winget; the catalog pins installer hashes in
  `catalog.lock.json`, which CI re-verifies against winget-pkgs.
