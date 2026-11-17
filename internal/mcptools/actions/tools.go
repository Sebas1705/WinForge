// Package actions holds a group of MCP tools. See internal/mcpkit for the contract.
package actions

import "github.com/Sebas1705/WinForge/internal/mcpkit"

// Tools returns this group's tools. A group may add its own Deps and a ToolsWith(env, deps) for tests.
func Tools(env *mcpkit.Env) []mcpkit.Tool { return nil }
