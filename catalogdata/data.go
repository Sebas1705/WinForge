// Package catalogdata embeds the curated WinForge catalog: applications,
// profiles, recipes and generated detection rules. It holds data only;
// parsing and validation live in internal/catalog.
package catalogdata

import "embed"

//go:embed apps/*.yml profiles/*.yml recipes/*.yml detect/*.yml featured.yml taglines.yml emulation.yml games.yml
var FS embed.FS
