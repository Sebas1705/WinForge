package sched

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func buildXML(t *testing.T, exe string, s Settings) string {
	t.Helper()
	b, err := BuildTaskXML(Task{Exe: exe, Settings: s}, day0)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTaskXMLSafetyAndShape(t *testing.T) {
	x := buildXML(t, `C:\Program Files\Sebas1705\WinForge\WinForge.exe`, Settings{Mode: Apply, Frequency: Daily, Hour: 7, Minute: 5})
	for _, want := range []string{
		`<Command>C:\Program Files\Sebas1705\WinForge\WinForge.exe</Command>`, // spaces need no quoting: Command is its own element
		`<Arguments>updates --scheduled --mode apply</Arguments>`,
		`<StartBoundary>2026-11-16T07:05:00</StartBoundary>`,
		`<ScheduleByDay>`, `<DaysInterval>1</DaysInterval>`,
		`<LogonType>InteractiveToken</LogonType>`, // only while signed in
		`<RunLevel>LeastPrivilege</RunLevel>`,     // no administrator, no UAC
		`<StartWhenAvailable>true</StartWhenAvailable>`,
		`<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>`,
		`<ExecutionTimeLimit>PT1H</ExecutionTimeLimit>`,
		`<WakeToRun>false</WakeToRun>`,
		`<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>`,
	} {
		if !strings.Contains(x, want) {
			t.Errorf("missing %s in\n%s", want, x)
		}
	}
	for _, never := range []string{"HighestAvailable", "<UserId>", "<Password>", "S4U"} {
		if strings.Contains(x, never) {
			t.Errorf("the task must not contain %s", never)
		}
	}
	if RunTimeout >= time.Hour {
		t.Error("the run must stop itself before the task's own time limit")
	}
}

func TestTaskXMLWeekly(t *testing.T) {
	x := buildXML(t, `C:\a\WinForge.exe`, Settings{Mode: Notify, Frequency: Weekly, Weekday: 6, Hour: 23, Minute: 59})
	for _, want := range []string{`<ScheduleByWeek>`, `<WeeksInterval>1</WeeksInterval>`, `<Saturday></Saturday>`, `<StartBoundary>2026-11-16T23:59:00</StartBoundary>`} {
		if !strings.Contains(x, want) {
			t.Errorf("missing %s in\n%s", want, x)
		}
	}
	if strings.Contains(x, "ScheduleByDay") || strings.Contains(x, "Sunday") {
		t.Errorf("one weekly trigger on one day:\n%s", x)
	}
}

func TestTaskXMLEscapesWhatItEmbeds(t *testing.T) {
	x := buildXML(t, `C:\R&D\Win'Forge.exe`, Settings{Mode: Notify, Frequency: Daily})
	if !strings.Contains(x, `C:\R&amp;D\Win&#39;Forge.exe`) {
		t.Errorf("ampersand and quote must be escaped:\n%s", x)
	}
}

func TestTaskXMLRefusesWhatCouldBreakOut(t *testing.T) {
	good := Settings{Mode: Notify, Frequency: Daily}
	for _, exe := range []string{"", `C:\a"b.exe`, "C:\\a\r\nb.exe", `C:\<x>\a.exe`, `C:\a|b.exe`, "C:\\a\x00.exe"} {
		if _, err := BuildTaskXML(Task{Exe: exe, Settings: good}, day0); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q accepted (%v)", exe, err)
		}
	}
	if _, err := BuildTaskXML(Task{Exe: `C:\a.exe`, Settings: Settings{Mode: "x", Frequency: Daily}}, day0); !errors.Is(err, ErrInvalid) {
		t.Error("invalid settings must not produce a task")
	}
}

func TestTaskXMLRoundTrips(t *testing.T) {
	for _, s := range []Settings{
		{Mode: Notify, Frequency: Daily, Hour: 9, Minute: 0},
		{Mode: Apply, Frequency: Daily, Hour: 0, Minute: 1},
		{Mode: Notify, Frequency: Weekly, Weekday: 0, Hour: 12, Minute: 30},
		{Mode: Apply, Frequency: Weekly, Weekday: 6, Hour: 23, Minute: 59},
	} {
		raw := EncodeUTF16([]byte(buildXML(t, `C:\Apps\WinForge.exe`, s)))
		info, err := ParseTaskXML(raw)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Exists || !info.Enabled || info.Foreign || info.Settings != s || info.Exe != `C:\Apps\WinForge.exe` {
			t.Errorf("%+v -> %+v", s, info)
		}
	}
}

func TestEncodeUTF16(t *testing.T) {
	b := EncodeUTF16([]byte("<a>ñ€</a>"))
	if b[0] != 0xFF || b[1] != 0xFE {
		t.Fatal("a byte order mark is required")
	}
	u := make([]uint16, 0, len(b)/2)
	for i := 2; i < len(b); i += 2 {
		u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
	}
	got := string(utf16.Decode(u))
	if !strings.HasPrefix(got, `<?xml version="1.0" encoding="UTF-16"?>`) || !strings.Contains(got, "<a>ñ€</a>") {
		t.Errorf("%q", got)
	}
}

// What schtasks /Query /XML prints for a task somebody else made, with the UTF-16 declaration
// even though a pipe carries plain text.
const foreignTask = `<?xml version="1.0" encoding="UTF-16"?>

<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers>
    <TimeTrigger><StartBoundary>2026-01-01T03:00:00</StartBoundary><Enabled>true</Enabled></TimeTrigger>
  </Triggers>
  <Settings><Enabled>false</Enabled></Settings>
  <Actions Context="Author"><Exec><Command>C:\Tools\other.exe</Command><Arguments>--go</Arguments></Exec></Actions>
</Task>`

func TestAForeignTaskIsReportedAsForeign(t *testing.T) {
	info, err := ParseTaskXML([]byte(foreignTask))
	if err != nil {
		t.Fatal(err)
	}
	if !info.Exists || !info.Foreign || info.Enabled || info.Exe != `C:\Tools\other.exe` || info.Settings.Mode != Notify {
		t.Errorf("%+v", info)
	}
}

func TestATaskWithEditedArgumentsOrScheduleIsForeign(t *testing.T) {
	good := buildXML(t, `C:\a\WinForge.exe`, Settings{Mode: Apply, Frequency: Daily, Hour: 9})
	for name, edit := range map[string]func(string) string{
		"arguments": func(s string) string { return strings.Replace(s, "--mode apply", "--mode apply --extra", 1) },
		"interval":  func(s string) string { return strings.Replace(s, "<DaysInterval>1", "<DaysInterval>3", 1) },
		"disabled": func(s string) string {
			i := strings.Index(s, "<Settings>")
			j := i + strings.Index(s[i:], "<Enabled>true</Enabled>")
			return s[:j] + "<Enabled>false</Enabled>" + s[j+len("<Enabled>true</Enabled>"):]
		},
	} {
		got := edit(good)
		if got == good {
			t.Fatalf("%s: edit did not apply", name)
		}
		info, err := ParseTaskXML([]byte(got))
		if err != nil {
			t.Fatal(err)
		}
		if name == "disabled" {
			if info.Enabled || info.Foreign {
				t.Errorf("a switched-off task is off, not foreign: %+v", info)
			}
			continue
		}
		if !info.Foreign {
			t.Errorf("%s: %+v", name, info)
		}
	}
}

func TestUnreadableTaskXMLIsAnError(t *testing.T) {
	if _, err := ParseTaskXML([]byte("this is not xml")); !errors.Is(err, ErrTask) {
		t.Errorf("%v", err)
	}
}
