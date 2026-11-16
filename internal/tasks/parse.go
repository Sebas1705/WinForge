package tasks

import (
	"encoding/xml"
	"strings"
)

// Trigger kinds WinForge lists: the two that start a task with Windows.
const (
	TriggerLogon = "logon" // At log on
	TriggerBoot  = "boot"  // At startup
)

// Parsed is what the task's XML definition says that matters here.
type Parsed struct {
	Triggers  []string // "logon" and/or "boot", only triggers that are enabled
	Command   string   // the first Exec action's program, as written
	Arguments string
	Author    string // RegistrationInfo/Author
	UserID    string // Principal/UserId ("" when the task runs for a group)
	GroupID   string
	RunLevel  string // LeastPrivilege or HighestAvailable
}

type xmlTrigger struct {
	XMLName xml.Name
	Enabled string `xml:"Enabled"`
}

type xmlTask struct {
	Reg struct {
		Author string `xml:"Author"`
	} `xml:"RegistrationInfo"`
	Triggers struct {
		Items []xmlTrigger `xml:",any"`
	} `xml:"Triggers"`
	Principals struct {
		Principal []struct {
			UserID   string `xml:"UserId"`
			GroupID  string `xml:"GroupId"`
			RunLevel string `xml:"RunLevel"`
		} `xml:"Principal"`
	} `xml:"Principals"`
	Actions struct {
		Exec []struct {
			Command   string `xml:"Command"`
			Arguments string `xml:"Arguments"`
		} `xml:"Exec"`
	} `xml:"Actions"`
}

// stripDeclaration removes the <?xml ... encoding="UTF-16"?> line: the text is already decoded, and
// Go's decoder refuses an encoding it was not asked to convert.
func stripDeclaration(s string) string {
	s = strings.TrimPrefix(s, string(rune(0xFEFF)))
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "<?xml") {
		if end := strings.Index(s, "?>"); end >= 0 {
			s = s[end+2:]
		}
	}
	return s
}

// ParseXML reads a task definition as exported by Windows (Export-ScheduledTask, schtasks /xml).
// Only logon and boot triggers are reported, and a trigger marked <Enabled>false</Enabled> does
// not count. Tag names are not localized, so the same parser reads any Windows language.
func ParseXML(def string) (Parsed, error) {
	var t xmlTask
	if err := xml.Unmarshal([]byte(stripDeclaration(def)), &t); err != nil {
		return Parsed{}, err
	}
	p := Parsed{Author: strings.TrimSpace(t.Reg.Author)}
	seen := map[string]bool{}
	for _, tr := range t.Triggers.Items {
		if strings.EqualFold(strings.TrimSpace(tr.Enabled), "false") {
			continue
		}
		kind := ""
		switch tr.XMLName.Local {
		case "LogonTrigger":
			kind = TriggerLogon
		case "BootTrigger":
			kind = TriggerBoot
		}
		if kind != "" && !seen[kind] {
			seen[kind] = true
			p.Triggers = append(p.Triggers, kind)
		}
	}
	if len(t.Principals.Principal) > 0 {
		pr := t.Principals.Principal[0]
		p.UserID, p.GroupID, p.RunLevel = strings.TrimSpace(pr.UserID), strings.TrimSpace(pr.GroupID), strings.TrimSpace(pr.RunLevel)
	}
	if len(t.Actions.Exec) > 0 {
		p.Command = strings.TrimSpace(t.Actions.Exec[0].Command)
		p.Arguments = strings.TrimSpace(t.Actions.Exec[0].Arguments)
	}
	return p, nil
}
