// Package catalogdata embeds the curated WinForge catalog: applications,
// profiles and recipes. It holds data only; parsing and validation live in
// internal/catalog.
package catalogdata

import "embed"

//go:embed apps/*.yml profiles/*.yml recipes/*.yml
var FS embed.FS
