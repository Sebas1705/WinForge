package storage

// The helpers other packages (the uninstall leftovers search) need to apply the same safety rules this
// package applies to its own cleanups, exported rather than copied so there is one definition of
// "protected".

// ProtectedDirs are the folders a removal may never be, nor contain: the profile and its data folders,
// ProgramData, the Windows folder, the drive root and the person's own document folders.
func ProtectedDirs(e Env) []string { return protectedDirs(e) }

// SafeRoot says whether a folder may be removed at all: an absolute, clean path that is not a drive
// root, a protected folder, or a folder that contains one.
func SafeRoot(e Env, dir string) error { return safeRoot(e, dir) }

// SameDir compares two folder paths the way Windows does: ignoring case and a trailing separator.
func SameDir(a, b string) bool { return sameDir(a, b) }

// Within reports whether path is strictly inside dir.
func Within(dir, path string) bool { return within(dir, path) }
