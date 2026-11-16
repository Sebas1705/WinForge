package sched

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// The scheduled run has no window to ask, and the language the person picked lives in the window's
// storage, so the desktop app copies it here whenever it changes (and when the schedule is turned on).

type langFile struct {
	Language string `json:"language"`
}

func langPath(dir string) string { return filepath.Join(dir, "settings.json") }

// NormalizeLanguage maps any language tag to one WinForge has: Spanish for "es" and "es-*", English for the rest.
func NormalizeLanguage(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "es" || strings.HasPrefix(tag, "es-") || strings.HasPrefix(tag, "es_") {
		return "es"
	}
	return "en"
}

// SaveLanguage stores the language. With onlyIfKept it does nothing unless a language was stored
// before, so merely starting the app never creates files for someone who does not use the schedule.
func SaveLanguage(dir, lang string, onlyIfKept bool) error {
	lang = NormalizeLanguage(lang)
	path := langPath(dir)
	cur, have := readLang(path)
	if onlyIfKept && !have {
		return nil
	}
	if have && cur == lang {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, _ := json.Marshal(langFile{Language: lang})
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readLang(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var f langFile
	if json.Unmarshal(b, &f) != nil || f.Language == "" {
		return "", false
	}
	return NormalizeLanguage(f.Language), true
}

// Language is the language of the person's toasts: the one the app stored, else the Windows display
// language (fallback is a function so tests do not depend on this PC).
func Language(dir string, fallback func() string) string {
	if l, ok := readLang(langPath(dir)); ok {
		return l
	}
	if fallback != nil {
		return NormalizeLanguage(fallback())
	}
	return "en"
}
