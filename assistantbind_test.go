package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// assistantApp returns an App whose WinForge.exe lives in dir, and creates winforge-mcp.exe there when
// withMCP is set.
func assistantApp(t *testing.T, dir string, withMCP bool) (*App, string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mcp := filepath.Join(dir, assistantExeName)
	if withMCP {
		if err := os.WriteFile(mcp, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := testApp(t)
	a.assistantExe = func() (string, error) { return filepath.Join(dir, "WinForge.exe"), nil }
	return a, mcp
}

func TestAssistantInfoFindsTheExeNextToTheApp(t *testing.T) {
	a, mcp := assistantApp(t, filepath.Join(t.TempDir(), "Program Files", "Win Forge"), true)
	info, err := a.AssistantInfo()
	if err != nil {
		t.Fatal(err)
	}
	if info.ExePath != mcp || !info.Exists || !info.InstalledNextToApp {
		t.Fatalf("%+v", info)
	}
	if strings.Join(info.Modes, ",") != "read,plan,full" {
		t.Errorf("modes %v", info.Modes)
	}
	c := info.Commands.ClaudeCode
	// a path with spaces must be quoted, and the quotes come from us, not from the person
	if c.Read != `claude mcp add winforge -- "`+mcp+`"` {
		t.Errorf("read: %s", c.Read)
	}
	if c.Plan != c.Read+" --mode plan" || c.Full != c.Read+" --mode full" {
		t.Errorf("plan/full: %s | %s", c.Plan, c.Full)
	}
	if !strings.Contains(c.Read, "Program Files") {
		t.Error("the path with a space was lost")
	}
}

func TestAssistantDesktopJSONIsRealJSONWithTheRightPath(t *testing.T) {
	cases := []string{
		`C:\Program Files\WinForge\winforge-mcp.exe`,
		`C:\Users\José Ñandú\AppData\Local\Programs\WinForge\winforge-mcp.exe`,
		`D:\日本語 フォルダ\winforge-mcp.exe`,
		`\\server\share\a&b <c>\winforge-mcp.exe`,
	}
	for _, exe := range cases {
		for _, mode := range assistantModes {
			s, err := assistantDesktopJSON(exe, mode)
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				MCPServers map[string]struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
				} `json:"mcpServers"`
			}
			if err := json.Unmarshal([]byte(s), &doc); err != nil {
				t.Fatalf("%q: not JSON: %v\n%s", exe, err, s)
			}
			srv := doc.MCPServers["winforge"]
			if srv.Command != exe || len(srv.Args) != 2 || srv.Args[0] != "--mode" || srv.Args[1] != mode {
				t.Errorf("%q %s: %+v", exe, mode, srv)
			}
			// what the person pastes has every backslash doubled and keeps the readable name
			if !strings.Contains(s, strings.ReplaceAll(exe, `\`, `\\`)) {
				t.Errorf("backslashes not doubled in %s", s)
			}
		}
	}
	s, _ := assistantDesktopJSON(`C:\x\winforge-mcp.exe`, "read")
	want := "{\n  \"mcpServers\": {\n    \"winforge\": {\n      \"command\": \"C:\\\\x\\\\winforge-mcp.exe\",\n      \"args\": [\n        \"--mode\",\n        \"read\"\n      ]\n    }\n  }\n}"
	if s != want {
		t.Errorf("got\n%s\nwant\n%s", s, want)
	}
}

func TestAssistantInfoUnicodeUserFolder(t *testing.T) {
	a, mcp := assistantApp(t, filepath.Join(t.TempDir(), "Users", "José Ñandú", "WinForge"), true)
	info, err := a.AssistantInfo()
	if err != nil {
		t.Fatal(err)
	}
	if info.ExePath != mcp {
		t.Fatalf("%q", info.ExePath)
	}
	var doc map[string]map[string]struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(info.Commands.ClaudeDesktopJson), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["mcpServers"]["winforge"].Command != mcp {
		t.Errorf("json path %q != %q", doc["mcpServers"]["winforge"].Command, mcp)
	}
	if !strings.Contains(info.Commands.ClaudeCode.Read, "José Ñandú") {
		t.Errorf("%s", info.Commands.ClaudeCode.Read)
	}
	if info.Commands.ClaudeDesktopJson != info.Commands.ClaudeDesktop.Read {
		t.Error("claudeDesktopJson must be the read-mode snippet")
	}
	if !strings.Contains(info.Commands.ClaudeDesktop.Full, `"full"`) {
		t.Error("full snippet lacks the mode")
	}
}

func TestAssistantInfoMissingExe(t *testing.T) {
	a, _ := assistantApp(t, t.TempDir(), false)
	info, err := a.AssistantInfo()
	if err != nil {
		t.Fatal(err)
	}
	if info.ExePath != "" || info.Exists || info.InstalledNextToApp || info.Commands != (AssistantCommands{}) {
		t.Errorf("%+v", info)
	}
	if len(info.Modes) != 3 {
		t.Errorf("modes %v", info.Modes)
	}
}

func TestAssistantInfoDirectoryNamedLikeTheExe(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, assistantExeName), 0o755); err != nil {
		t.Fatal(err)
	}
	a, _ := assistantApp(t, dir, false)
	if info, _ := a.AssistantInfo(); info.Exists {
		t.Error("a folder is not the program")
	}
}

func TestAssistantInfoWhenOwnPathIsUnknown(t *testing.T) {
	a := testApp(t)
	a.assistantExe = func() (string, error) { return "", os.ErrNotExist }
	info, err := a.AssistantInfo()
	if err != nil || info.Exists || info.ExePath != "" {
		t.Errorf("%+v %v", info, err)
	}
}

func TestAssistantRevealExe(t *testing.T) {
	a, mcp := assistantApp(t, t.TempDir(), true)
	var got string
	a.reveal = func(p string) error { got = p; return nil }
	if err := a.AssistantRevealExe(); err != nil || got != mcp {
		t.Fatalf("%q %v", got, err)
	}
	// nothing to reveal when it is missing
	b, _ := assistantApp(t, t.TempDir(), false)
	called := false
	b.reveal = func(string) error { called = true; return nil }
	if err := b.AssistantRevealExe(); err == nil || called {
		t.Errorf("err %v called %v", err, called)
	}
}
