package sched

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSettingsAreAllowListed(t *testing.T) {
	ok := Settings{Mode: Notify, Frequency: Daily, Hour: 9, Minute: 30}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]Settings{
		"mode":         {Mode: "format-c", Frequency: Daily},
		"empty mode":   {Frequency: Daily},
		"frequency":    {Mode: Notify, Frequency: "hourly"},
		"weekday":      {Mode: Notify, Frequency: Weekly, Weekday: 7},
		"negative day": {Mode: Notify, Frequency: Weekly, Weekday: -1},
		"hour":         {Mode: Notify, Frequency: Daily, Hour: 24},
		"minute":       {Mode: Notify, Frequency: Daily, Minute: 60},
		"negative":     {Mode: Notify, Frequency: Daily, Hour: -1},
	} {
		if err := s.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A daily schedule ignores the weekday, so a stale value cannot make it invalid.
	if err := (Settings{Mode: Apply, Frequency: Daily, Weekday: 99}).Validate(); err != nil {
		t.Errorf("daily with a stray weekday: %v", err)
	}
}

func TestParseClock(t *testing.T) {
	for in, want := range map[string][2]int{"00:00": {0, 0}, "09:05": {9, 5}, "23:59": {23, 59}} {
		h, m, err := ParseClock(in)
		if err != nil || h != want[0] || m != want[1] {
			t.Errorf("%q = %d:%d %v", in, h, m, err)
		}
	}
	for _, bad := range []string{"", "9:00", "09:5", "24:00", "12:60", "ab:cd", "09-00", "09:00:00", " 09:00", "-1:00", "09:00; calc"} {
		if _, _, err := ParseClock(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q accepted (%v)", bad, err)
		}
	}
}

func TestNextRun(t *testing.T) {
	at := func(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, time.UTC) }
	// 2026-11-16 is a Monday.
	if day0.Weekday() != time.Monday {
		t.Fatal("test date is not a Monday")
	}
	daily := Settings{Mode: Notify, Frequency: Daily, Hour: 9, Minute: 30}
	cases := []struct {
		name string
		s    Settings
		now  time.Time
		want time.Time
	}{
		{"daily, time still ahead today", daily, at(16, 8, 0), at(16, 9, 30)},
		{"daily, exactly now counts", daily, at(16, 9, 30), at(16, 9, 30)},
		{"daily, passed today", daily, at(16, 9, 31), at(17, 9, 30)},
		{"daily across a month end", Settings{Frequency: Daily, Hour: 1}, time.Date(2026, 11, 30, 2, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 1, 0, 0, 0, time.UTC)},
		{"weekly, later this week", Settings{Frequency: Weekly, Weekday: 5, Hour: 18}, at(16, 10, 0), at(20, 18, 0)},
		{"weekly, today and ahead", Settings{Frequency: Weekly, Weekday: 1, Hour: 18}, at(16, 10, 0), at(16, 18, 0)},
		{"weekly, today and passed", Settings{Frequency: Weekly, Weekday: 1, Hour: 8}, at(16, 10, 0), at(23, 8, 0)},
		{"weekly, earlier weekday wraps to next week", Settings{Frequency: Weekly, Weekday: 0, Hour: 8}, at(16, 10, 0), at(22, 8, 0)},
		{"weekly across a year end", Settings{Frequency: Weekly, Weekday: 4}, time.Date(2026, 12, 31, 0, 0, 1, 0, time.UTC), time.Date(2027, 1, 7, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		if got := c.s.NextRun(c.now); !got.Equal(c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTaskArgumentsAreConstants(t *testing.T) {
	for _, m := range []Mode{Notify, Apply} {
		a := Args(m)
		if strings.Join(a, " ") != "updates --scheduled --mode "+string(m) {
			t.Errorf("%v", a)
		}
		back, ok := ModeFromArgs(strings.Join(a, " "))
		if !ok || back != m {
			t.Errorf("%v: %v %v", m, back, ok)
		}
	}
	for _, foreign := range []string{"", "updates", "updates --scheduled --mode delete", "updates --scheduled --mode apply --extra", "-x updates --scheduled --mode apply"} {
		if m, ok := ModeFromArgs(foreign); ok || m != Notify {
			t.Errorf("%q read as %v %v: an unknown command line must fall back to the safe mode and say it is foreign", foreign, m, ok)
		}
	}
}

func TestRunnerPath(t *testing.T) {
	files := map[string]bool{}
	has := func(p string) bool { return files[p] }
	desktop := `C:\Program Files\Sebas1705\WinForge\WinForge.exe`

	if _, err := RunnerPath(desktop, has); !errors.Is(err, ErrRunnerMissing) {
		t.Fatalf("nothing there: %v", err)
	}
	files[desktop] = true
	if got, err := RunnerPath(desktop, has); err != nil || got != desktop {
		t.Fatalf("installed layout (only WinForge.exe): %q %v", got, err)
	}
	// The headless run is the desktop program even when winforge-cli.exe is also there: no console flash.
	files[`C:\Program Files\Sebas1705\WinForge\winforge-cli.exe`] = true
	if got, _ := RunnerPath(desktop, has); got != desktop {
		t.Fatalf("desktop program first: %q", got)
	}
	// A development build in a temp folder is not a program to schedule.
	dev := `C:\Users\me\AppData\Local\Temp\go-build123\b001\exe\app.exe`
	files[dev] = true
	if _, err := RunnerPath(dev, has); !errors.Is(err, ErrRunnerMissing) {
		t.Fatalf("dev build: %v", err)
	}
	// ...unless winforge-cli.exe sits next to it.
	files[`C:\Users\me\AppData\Local\Temp\go-build123\b001\exe\winforge-cli.exe`] = true
	if got, err := RunnerPath(dev, has); err != nil || !strings.HasSuffix(got, "winforge-cli.exe") {
		t.Fatalf("cli fallback: %q %v", got, err)
	}
	for _, bad := range []string{"", `C:\x"y\WinForge.exe`, "C:\\x\nWinForge.exe"} {
		files[bad] = true
		if _, err := RunnerPath(bad, has); !errors.Is(err, ErrRunnerMissing) {
			t.Errorf("%q accepted", bad)
		}
	}
	if got, err := RunnerPath(`C:\a\winforge.EXE`, func(string) bool { return true }); err != nil || got != `C:\a\winforge.EXE` {
		t.Errorf("the name match ignores case: %q %v", got, err)
	}
}
