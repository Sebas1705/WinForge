# Assistant access (MCP)

`winforge-mcp.exe` is a local [MCP](https://modelcontextprotocol.io) server over stdio. It lets an assistant such as Claude ask this WinForge installation about the PC and, only if you allow it, act. It opens no port, makes no network call and has no dependency beyond the Go standard library (the small protocol is implemented in `internal/mcpserver`).

The installer puts it next to `WinForge.exe`; every release also carries it as `WinForge-mcp-<version>-windows-amd64.exe`. *Settings > Assistant (MCP)* shows the exact command to copy for the mode you choose.

## Register it

```bash
# Claude Code: read only (the default)
claude mcp add winforge -- "C:\Program Files\...\winforge-mcp.exe"
# plans, or plans and changes
claude mcp add winforge -- "C:\Program Files\...\winforge-mcp.exe" --mode plan
claude mcp add winforge -- "C:\Program Files\...\winforge-mcp.exe" --mode full
```

For Claude Desktop, add the same command and `args` (`["--mode", "read"]`) to `claude_desktop_config.json` under `mcpServers`.

## Modes

| Mode | What the assistant can use |
|---|---|
| `--mode read` (default) | Only tools that look. They change nothing. |
| `--mode plan` | Also tools that prepare a plan. A plan changes nothing and cannot be applied. |
| `--mode full` | Also `winforge_apply_plan`, which runs one plan prepared in this session, once. |

A tool the mode does not allow is not listed and is refused if called, with the flag that would enable it. `winforge-mcp --list-tools --mode <m>` prints exactly what a mode exposes.

## How a change happens

1. A `winforge_prepare_*` tool builds a plan from a fresh look at the PC (ids are checked against what the tool lists right now) and returns the plan id, a plain summary of exactly what would change, whether administrator rights are needed and when it expires.
2. The assistant shows you the summary. The tool descriptions and the server instructions tell it to wait for your approval.
3. Only \`winforge_apply_plan\` with that id runs it. Ids are random, work once and expire after 10 minutes (\`--plan-ttl\`); at most 32 plans are kept. When it runs, the plan checks the world again and refuses, changing nothing, if it differs from the plan.

Changes run one at a time and take the same run lock as \`winforge-cli apply\` and \`updates\`, so they never overlap each other or the window. A plan that needs administrator rights is refused unless the server runs elevated. The assistant's own client also asks you before calling \`winforge_apply_plan\`, because the tool is marked destructive.

## Tools

Read (26): \`winforge_status\`, \`winforge_catalog_search\`, \`winforge_installed_apps\`, \`winforge_updates_check\`, \`winforge_profiles_list\`, \`winforge_profile_get\`, \`winforge_install_plan\`, \`winforge_eol_report\`, \`winforge_history\`, \`winforge_compare_profile\`, \`winforge_health_scan\`, \`winforge_health_history\`, \`winforge_battery\`, \`winforge_storage_volumes\`, \`winforge_storage_scan\`, \`winforge_storage_biggest\`, \`winforge_storage_projects\`, \`winforge_virtual_disks\`, \`winforge_startup_list\`, \`winforge_services_list\`, \`winforge_tasks_list\`, \`winforge_env_scan\`, \`winforge_features_list\`, \`winforge_wsl_info\`, \`winforge_privacy_list\` and \`winforge_list_plans\`.

Prepare (17): \`winforge_prepare_install\`, \`_update\`, \`_uninstall\`, \`_storage_clean\`, \`_remove_project_junk\`, \`_privacy\`, \`_privacy_restore\`, \`_env_fix\`, \`_env_restore\`, \`_startup_toggle\`, \`_service_mode\`, \`_service_restore\`, \`_task_toggle\`, \`_task_restore\`, \`_feature\`, \`_optimize\` and \`winforge_discard_plan\`.

Act (1): \`winforge_apply_plan\`.

They do exactly what the matching pages of the app do and nothing else: only catalog apps, only the fixed cleanup categories, only the reviewed privacy settings, user variables only, services and tasks by start mode or on/off only, never a free-form command, script or path beyond what a tool lists.

## Privacy and trust

- **Everything a tool returns is sent to the assistant's model**, which may be a cloud service. WinForge itself sends nothing. Your user name and user folder are replaced before any output leaves the program (the same scrubber as the diagnostic report), but the apps, drivers, folders and settings the tools return are visible to the model.
- The list of installed software is personal data: \`winforge_catalog_search\` does not read it unless asked (\`withInstalled\`), and \`winforge_installed_apps\` says so in its description.
- Names, paths and values in results come from the PC and from software on it, so they could contain text written to steer an assistant. The server instructions tell the model to treat them as data. The real protection is that nothing changes without a plan you approved.
- Read tools never write. The PC-state tools are checked by tests that fail on any file, registry or process write.
- It runs with the rights of the account that starts it. Start the assistant elevated only if you want administrator plans to work.

## Not covered

No real MCP client has been used for long sessions; the end-to-end checks use a scripted client. Administrator-only plans (services, tasks, features, the DISM recipes) and real winget installs were never run for real through it.
