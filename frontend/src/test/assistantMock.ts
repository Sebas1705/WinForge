import {vi} from "vitest";
import type {AssistantInfo} from "../api";

/** The assistant calls as spies. apiMock.ts spreads these into its `api` and resets them. */
export const assistantApi = {
    AssistantInfo: vi.fn(), AssistantRevealExe: vi.fn(),
};

export const SAMPLE_EXE = "C:\\Program Files\\WinForge\\winforge-mcp.exe";
const json = (mode: string) => `{\n  "mcpServers": {\n    "winforge": {\n      "command": "C:\\\\Program Files\\\\WinForge\\\\winforge-mcp.exe",\n      "args": [\n        "--mode",\n        "${mode}"\n      ]\n    }\n  }\n}`;
const code = (suffix: string) => `claude mcp add winforge -- "${SAMPLE_EXE}"${suffix}`;

/** What the Go side returns when winforge-mcp.exe sits next to WinForge.exe. */
export const sampleAssistant: AssistantInfo = {
    exePath: SAMPLE_EXE, exists: true, installedNextToApp: true, modes: ["read", "plan", "full"],
    commands: {
        claudeCode: {read: code(""), plan: code(" --mode plan"), full: code(" --mode full")},
        claudeDesktopJson: json("read"),
        claudeDesktop: {read: json("read"), plan: json("plan"), full: json("full")},
    },
};

/** What it returns when the exe is not there (a portable WinForge.exe on its own). */
export const missingAssistant: AssistantInfo = {
    exePath: "", exists: false, installedNextToApp: false, modes: ["read", "plan", "full"],
    commands: {
        claudeCode: {read: "", plan: "", full: ""}, claudeDesktopJson: "", claudeDesktop: {read: "", plan: "", full: ""},
    },
};

export function resetAssistant(): void {
    for (const fn of Object.values(assistantApi)) fn.mockReset();
    assistantApi.AssistantInfo.mockResolvedValue(structuredClone(sampleAssistant));
    assistantApi.AssistantRevealExe.mockResolvedValue(undefined);
}
