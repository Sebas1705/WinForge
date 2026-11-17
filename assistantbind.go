package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// assistantExeName is the local MCP server the installer and the release ship next to WinForge.exe.
const assistantExeName = "winforge-mcp.exe"

// assistantModes are the modes of winforge-mcp, safest first. The first one is the default.
var assistantModes = []string{"read", "plan", "full"}

// AssistantModeText holds one string per mode.
type AssistantModeText struct {
	Read string `json:"read"`
	Plan string `json:"plan"`
	Full string `json:"full"`
}

// AssistantCommands is the text the person can paste. Nothing here is run or written by WinForge.
type AssistantCommands struct {
	ClaudeCode        AssistantModeText `json:"claudeCode"`
	ClaudeDesktopJson string            `json:"claudeDesktopJson"` // read mode, the default
	ClaudeDesktop     AssistantModeText `json:"claudeDesktop"`     // the same snippet for each mode
}

// AssistantInfoResult says where winforge-mcp.exe is and what to paste to connect it.
type AssistantInfoResult struct {
	ExePath            string            `json:"exePath"` // "" when it is not next to WinForge.exe
	Exists             bool              `json:"exists"`
	InstalledNextToApp bool              `json:"installedNextToApp"`
	Commands           AssistantCommands `json:"commands"`
	Modes              []string          `json:"modes"`
}

// AssistantInfo looks for winforge-mcp.exe next to the running WinForge.exe and builds the commands
// for Claude Code and Claude Desktop. It executes nothing and writes nothing.
func (a *App) AssistantInfo() (AssistantInfoResult, error) {
	res := AssistantInfoResult{Modes: append([]string(nil), assistantModes...)}
	exe := a.assistantMCPPath()
	if exe == "" {
		return res, nil
	}
	res.ExePath, res.Exists, res.InstalledNextToApp = exe, true, true
	cmds, err := assistantCommands(exe)
	if err != nil {
		return res, err
	}
	res.Commands = cmds
	return res, nil
}

// AssistantRevealExe shows winforge-mcp.exe in Explorer.
func (a *App) AssistantRevealExe() error {
	exe := a.assistantMCPPath()
	if exe == "" {
		return errors.New("assistant:missing")
	}
	if a.reveal != nil {
		return a.reveal(exe)
	}
	// explorer.exe reports failure even when it opens, so its exit code is ignored.
	return exec.Command("explorer.exe", "/select,"+filepath.Clean(exe)).Start()
}

// assistantMCPPath is the absolute path of winforge-mcp.exe beside the running executable, or "".
func (a *App) assistantMCPPath() string {
	find := a.assistantExe
	if find == nil {
		find = os.Executable
	}
	self, err := find()
	if err != nil || self == "" {
		return ""
	}
	p := filepath.Join(filepath.Dir(self), assistantExeName)
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return ""
	}
	return p
}

func assistantCommands(exe string) (AssistantCommands, error) {
	code := func(mode string) string {
		s := "claude mcp add winforge -- " + `"` + exe + `"`
		if mode != "read" {
			s += " --mode " + mode
		}
		return s
	}
	var desktop AssistantModeText
	for _, m := range assistantModes {
		j, err := assistantDesktopJSON(exe, m)
		if err != nil {
			return AssistantCommands{}, err
		}
		switch m {
		case "read":
			desktop.Read = j
		case "plan":
			desktop.Plan = j
		case "full":
			desktop.Full = j
		}
	}
	return AssistantCommands{
		ClaudeCode:        AssistantModeText{Read: code("read"), Plan: code("plan"), Full: code("full")},
		ClaudeDesktopJson: desktop.Read,
		ClaudeDesktop:     desktop,
	}, nil
}

// assistantDesktopJSON is the claude_desktop_config.json snippet. It is built with a JSON encoder, so
// backslashes, quotes and non-ASCII names in the path are escaped by the encoder and never by hand.
func assistantDesktopJSON(exe, mode string) (string, error) {
	type server struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	doc := map[string]map[string]server{"mcpServers": {"winforge": {Command: exe, Args: []string{"--mode", mode}}}}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}
