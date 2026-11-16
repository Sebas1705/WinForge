package sched

import (
	"fmt"
	"strings"
)

// The toast is worded here, in Go, because the scheduled run has no window to do it. Both languages
// are complete sentences with their own plurals; the language comes from Language.

type words struct {
	available func(n int) string // notify: "3 updates available"
	updated   func(n int) string // apply: "Updated 2 apps"
	needs     func(n int) string // apply: "1 needs you"
	needsOnly func(n int) string // apply, nothing updated: "1 app needs you"
	more      func(n int) string // "and 2 more"
	restart   string
}

var toastWords = map[string]words{
	"en": {
		available: func(n int) string { return plural(n, "1 update available", "%d updates available") },
		updated:   func(n int) string { return plural(n, "Updated 1 app", "Updated %d apps") },
		needs:     func(n int) string { return plural(n, "1 needs you", "%d need you") },
		needsOnly: func(n int) string { return plural(n, "1 app needs you", "%d apps need you") },
		more:      func(n int) string { return fmt.Sprintf(" and %d more", n) },
		restart:   "Restart Windows to finish.",
	},
	"es": {
		available: func(n int) string { return plural(n, "1 actualización disponible", "%d actualizaciones disponibles") },
		updated:   func(n int) string { return plural(n, "Actualizada 1 aplicación", "Actualizadas %d aplicaciones") },
		needs:     func(n int) string { return plural(n, "1 te necesita", "%d te necesitan") },
		needsOnly: func(n int) string { return plural(n, "1 aplicación te necesita", "%d aplicaciones te necesitan") },
		more:      func(n int) string { return fmt.Sprintf(" y %d más", n) },
		restart:   "Reinicia Windows para terminar.",
	},
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return fmt.Sprintf(many, n)
}

// listNames joins up to three names and counts the rest.
func listNames(names []string, w words) string {
	if len(names) <= 3 {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:3], ", ") + w.more(len(names)-3)
}

func namesOf(r Report, results ...string) []string {
	var out []string
	for _, it := range r.Items {
		for _, want := range results {
			if it.Result == want {
				out = append(out, it.Name)
				break
			}
		}
	}
	return out
}

// Compose words the toast for a run. ok is false when nothing happened that the person should hear
// about: nothing found, nothing updated, nothing waiting. A run that could not even check says nothing
// either (its log says why), so a flaky network never turns into a daily nag.
func Compose(r Report, lang string) (Toast, bool) {
	w, have := toastWords[lang]
	if !have {
		w = toastWords["en"]
	}
	found, admin := r.Count(ResultFound), r.Count(ResultNeedsAdmin)
	updated, failed := r.Count(ResultUpdated), r.Count(ResultFailed)
	t := Toast{Launch: LaunchUpdates}
	if r.Mode == Notify {
		n := found + admin
		if n == 0 {
			return Toast{}, false
		}
		t.Title = w.available(n)
		t.Body = listNames(namesOf(r, ResultFound, ResultNeedsAdmin), w)
		return t, true
	}
	needs := admin + failed
	switch {
	case updated > 0 && needs > 0:
		t.Title = w.updated(updated) + ", " + w.needs(needs)
	case updated > 0:
		t.Title = w.updated(updated)
	case needs > 0:
		t.Title = w.needsOnly(needs)
	default:
		return Toast{}, false
	}
	if needs > 0 {
		t.Body = listNames(namesOf(r, ResultNeedsAdmin, ResultFailed), w)
	} else {
		t.Body = listNames(namesOf(r, ResultUpdated), w)
	}
	if r.Restart() {
		t.Body += " " + w.restart
	}
	t.Body = strings.TrimSpace(t.Body)
	return t, true
}
