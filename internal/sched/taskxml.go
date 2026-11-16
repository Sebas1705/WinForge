package sched

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"
)

// The task is registered from an XML document rather than from schtasks flags: flags cannot say
// "run as soon as possible if a run was missed", cannot limit the run time or refuse a second
// copy, and every word of a flag is one more place for a path to be misread.

const taskNS = "http://schemas.microsoft.com/windows/2004/02/mit/task"

// ExecutionLimit is the longest the task may run before Windows ends it. The runner stops itself
// earlier (see RunTimeout), so this is only the backstop.
const ExecutionLimit = "PT1H"

type xmlTask struct {
	XMLName          xml.Name `xml:"Task"`
	Version          string   `xml:"version,attr"`
	Xmlns            string   `xml:"xmlns,attr"`
	RegistrationInfo struct {
		Description string `xml:"Description"`
	} `xml:"RegistrationInfo"`
	Triggers struct {
		Calendar xmlCalendar `xml:"CalendarTrigger"`
	} `xml:"Triggers"`
	Principals struct {
		Principal struct {
			ID        string `xml:"id,attr"`
			LogonType string `xml:"LogonType"`
			RunLevel  string `xml:"RunLevel"`
		} `xml:"Principal"`
	} `xml:"Principals"`
	Settings xmlSettings `xml:"Settings"`
	Actions  struct {
		Context string `xml:"Context,attr"`
		Exec    struct {
			Command   string `xml:"Command"`
			Arguments string `xml:"Arguments"`
		} `xml:"Exec"`
	} `xml:"Actions"`
}

type xmlCalendar struct {
	StartBoundary string     `xml:"StartBoundary"`
	Enabled       string     `xml:"Enabled"`
	ByDay         *xmlByDay  `xml:"ScheduleByDay"`
	ByWeek        *xmlByWeek `xml:"ScheduleByWeek"`
}

type xmlAnyChild struct {
	XMLName xml.Name
}

type xmlByDay struct {
	DaysInterval int `xml:"DaysInterval"`
}

type xmlByWeek struct {
	WeeksInterval int `xml:"WeeksInterval"`
	DaysOfWeek    struct {
		Days []xmlAnyChild `xml:",any"`
	} `xml:"DaysOfWeek"`
}

type xmlSettings struct {
	MultipleInstancesPolicy    string `xml:"MultipleInstancesPolicy"`
	DisallowStartIfOnBatteries string `xml:"DisallowStartIfOnBatteries"`
	StopIfGoingOnBatteries     string `xml:"StopIfGoingOnBatteries"`
	AllowHardTerminate         string `xml:"AllowHardTerminate"`
	StartWhenAvailable         string `xml:"StartWhenAvailable"`
	RunOnlyIfNetworkAvailable  string `xml:"RunOnlyIfNetworkAvailable"`
	AllowStartOnDemand         string `xml:"AllowStartOnDemand"`
	Enabled                    string `xml:"Enabled"`
	Hidden                     string `xml:"Hidden"`
	RunOnlyIfIdle              string `xml:"RunOnlyIfIdle"`
	WakeToRun                  string `xml:"WakeToRun"`
	ExecutionTimeLimit         string `xml:"ExecutionTimeLimit"`
	Priority                   int    `xml:"Priority"`
}

var weekdayNames = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// BuildTaskXML writes the task definition. The runner path and every value go through the XML
// encoder, so nothing in them can end an element or start another. now supplies the date the
// schedule counts from (the time of day is the person's).
func BuildTaskXML(t Task, now time.Time) ([]byte, error) {
	if err := t.Settings.Validate(); err != nil {
		return nil, err
	}
	if t.Exe == "" || strings.ContainsAny(t.Exe, "\"\r\n\x00<>|") {
		return nil, fmt.Errorf("%w: program path", ErrInvalid)
	}
	var x xmlTask
	x.Version, x.Xmlns = "1.2", taskNS
	x.RegistrationInfo.Description = "WinForge: checks for updates to catalog apps (and installs them when you chose that)."
	x.Triggers.Calendar.StartBoundary = time.Date(now.Year(), now.Month(), now.Day(), t.Settings.Hour, t.Settings.Minute, 0, 0, time.UTC).Format("2006-01-02T15:04:05")
	x.Triggers.Calendar.Enabled = "true"
	if t.Settings.Frequency == Weekly {
		w := &xmlByWeek{WeeksInterval: 1}
		w.DaysOfWeek.Days = []xmlAnyChild{{XMLName: xml.Name{Local: weekdayNames[t.Settings.Weekday]}}}
		x.Triggers.Calendar.ByWeek = w
	} else {
		x.Triggers.Calendar.ByDay = &xmlByDay{DaysInterval: 1}
	}
	x.Principals.Principal.ID = "Author"
	x.Principals.Principal.LogonType = "InteractiveToken" // only while the person is signed in
	x.Principals.Principal.RunLevel = "LeastPrivilege"    // never elevated, so never a UAC prompt
	x.Settings = xmlSettings{
		MultipleInstancesPolicy:    "IgnoreNew",
		DisallowStartIfOnBatteries: "false",
		StopIfGoingOnBatteries:     "false",
		AllowHardTerminate:         "true",
		StartWhenAvailable:         "true", // run as soon as possible if the PC was off at the time
		RunOnlyIfNetworkAvailable:  "true",
		AllowStartOnDemand:         "true",
		Enabled:                    "true",
		Hidden:                     "false",
		RunOnlyIfIdle:              "false",
		WakeToRun:                  "false",
		ExecutionTimeLimit:         ExecutionLimit,
		Priority:                   7,
	}
	x.Actions.Context = "Author"
	x.Actions.Exec.Command = t.Exe
	x.Actions.Exec.Arguments = strings.Join(Args(t.Settings.Mode), " ")
	body, err := xml.MarshalIndent(x, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

// EncodeUTF16 is how schtasks wants the file: UTF-16 little endian with a byte order mark, and a
// declaration that says so.
func EncodeUTF16(doc []byte) []byte {
	text := `<?xml version="1.0" encoding="UTF-16"?>` + "\n" + string(doc)
	u := utf16.Encode([]rune(text))
	out := make([]byte, 0, 2+2*len(u))
	out = append(out, 0xFF, 0xFE)
	for _, c := range u {
		out = append(out, byte(c), byte(c>>8))
	}
	return out
}

var xmlDecl = regexp.MustCompile(`^\s*<\?xml[^>]*\?>`)

// decodeQueryOutput turns what `schtasks /Query /XML` printed into text. The document says UTF-16
// but a pipe carries the console's code page; both are handled.
func decodeQueryOutput(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		u := make([]uint16, 0, len(b)/2)
		for i := 2; i+1 < len(b); i += 2 {
			u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
		}
		return string(utf16.Decode(u))
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	return string(b)
}

// ParseTaskXML reads a task definition back into what the page shows. A task that is not one of
// WinForge's shapes is reported as Foreign, with whatever could be read.
func ParseTaskXML(raw []byte) (TaskInfo, error) {
	doc := xmlDecl.ReplaceAllString(decodeQueryOutput(raw), "")
	var x xmlTask
	if err := xml.Unmarshal([]byte(doc), &x); err != nil {
		return TaskInfo{}, fmt.Errorf("%w: unreadable task definition: %v", ErrTask, err)
	}
	info := TaskInfo{Exists: true, Exe: strings.TrimSpace(x.Actions.Exec.Command), Args: strings.TrimSpace(x.Actions.Exec.Arguments)}
	info.Enabled = x.Settings.Enabled != "false" && x.Triggers.Calendar.Enabled != "false"
	mode, ok := ModeFromArgs(info.Args)
	info.Settings.Mode = mode
	info.Foreign = !ok
	cal := x.Triggers.Calendar
	if t, err := time.Parse("2006-01-02T15:04:05", strings.SplitN(strings.TrimSpace(cal.StartBoundary), ".", 2)[0]); err == nil {
		info.Settings.Hour, info.Settings.Minute = t.Hour(), t.Minute()
	} else if t, err := time.Parse(time.RFC3339, strings.TrimSpace(cal.StartBoundary)); err == nil {
		info.Settings.Hour, info.Settings.Minute = t.Hour(), t.Minute()
	} else {
		info.Foreign = true
	}
	switch {
	case cal.ByDay != nil && cal.ByDay.DaysInterval == 1:
		info.Settings.Frequency = Daily
	case cal.ByWeek != nil && cal.ByWeek.WeeksInterval == 1 && len(cal.ByWeek.DaysOfWeek.Days) == 1:
		info.Settings.Frequency = Weekly
		day := cal.ByWeek.DaysOfWeek.Days[0].XMLName.Local
		found := false
		for i, n := range weekdayNames {
			if n == day {
				info.Settings.Weekday, found = i, true
			}
		}
		info.Foreign = info.Foreign || !found
	default:
		info.Settings.Frequency = Daily
		info.Foreign = true
	}
	return info, nil
}
