import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {beforeEach, describe, expect, it, vi} from "vitest";
import App from "../App";
import {DICTS, KEYS} from "../lib/i18n";
import {api, resetApi} from "./apiMock";
import {missingAssistant, sampleAssistant, SAMPLE_EXE} from "./assistantMock";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi();
});

async function openApp() {
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(/catalog apps found/);
    return user;
}

type User = Awaited<ReturnType<typeof openApp>>;

/** Replaces the clipboard after userEvent.setup() installed its own stub. */
function mockClipboard(impl: (text: string) => Promise<void>) {
    const writeText = vi.fn(impl);
    Object.defineProperty(navigator, "clipboard", {value: {writeText}, configurable: true});
    return writeText;
}

async function openSettings(user: User) {
    await user.click(screen.getByRole("button", {name: "Appearance and language"}));
    return within(screen.getByRole("dialog"));
}

async function openAssistant(user: User, name = "Connect an assistant…", title = "Assistant (MCP)", settingsOpen = false) {
    const settings = settingsOpen ? within(screen.getByRole("dialog")) : await openSettings(user);
    await user.click(settings.getByRole("button", {name}));
    return within(await screen.findByRole("dialog", {name: title}));
}

describe("assistant strings", () => {
    it("has every assistant.* key in Spanish and English, never empty, with the same placeholders", () => {
        const keys = KEYS.filter((k) => k.startsWith("assistant."));
        expect(keys.length).toBeGreaterThanOrEqual(35);
        const ph = (s: string) => (s.match(/\{\w+\}/g) ?? []).sort().join();
        for (const k of keys) {
            expect(DICTS.en[k].trim(), `en ${k}`).not.toBe("");
            expect(DICTS.es[k].trim(), `es ${k}`).not.toBe("");
            expect(ph(DICTS.es[k]), k).toBe(ph(DICTS.en[k]));
        }
    });

    it("says plainly what the tools return, that WinForge sends nothing and what each mode can change", () => {
        expect(DICTS.en["assistant.privacy.sent"]).toMatch(/sent to the assistant's model/);
        expect(DICTS.en["assistant.privacy.none"]).toMatch(/WinForge sends nothing itself/);
        expect(DICTS.en["assistant.privacy.read"]).toMatch(/never changes anything/);
        expect(DICTS.en["assistant.privacy.full"]).toMatch(/plans you approve in the assistant, one by one.*same administrator rights/s);
        expect(DICTS.es["assistant.privacy.sent"]).toMatch(/se envía al modelo del asistente/);
        expect(DICTS.es["assistant.privacy.none"]).toMatch(/WinForge no envía nada por sí mismo/);
        expect(DICTS.es["assistant.privacy.full"]).toMatch(/uno por uno.*permisos de administrador/s);
    });
});

describe("the Assistant (MCP) dialog", () => {
    it("opens from Settings, asks Go only when opened, and shows the privacy box and the intro", async () => {
        const user = await openApp();
        const settings = await openSettings(user);
        expect(settings.getByText("Assistant (MCP)")).toBeInTheDocument();
        expect(api.AssistantInfo).not.toHaveBeenCalled();
        await user.click(settings.getByRole("button", {name: "Connect an assistant…"}));
        const d = within(await screen.findByRole("dialog", {name: "Assistant (MCP)"}));
        expect(d.getByText(/lets an assistant on this PC|This lets an assistant on this PC/)).toBeInTheDocument();
        expect(d.getByRole("heading", {name: "Before you connect"})).toBeInTheDocument();
        expect(d.getByText(/sent to the assistant's model/)).toBeInTheDocument();
        expect(d.getByText(/WinForge sends nothing itself/)).toBeInTheDocument();
        expect(d.getByText(/Read only mode never changes anything/)).toBeInTheDocument();
        expect(d.getByText(/Full mode can change the PC only through plans you approve in the assistant, one by one/)).toBeInTheDocument();
        expect(d.getByText(/same administrator rights WinForge has/)).toBeInTheDocument();
        expect(d.getByText(/ask for your approval in the assistant/)).toBeInTheDocument();
        await waitFor(() => expect(api.AssistantInfo).toHaveBeenCalledTimes(1));
        expect(api.AssistantRevealExe).not.toHaveBeenCalled();
    });

    it("offers three modes, read only first and selected, each with its meaning", async () => {
        const user = await openApp();
        const d = await openAssistant(user);
        const group = d.getByRole("group", {name: "How much may the assistant do?"});
        const radios = within(group).getAllByRole("radio");
        expect(radios.map((r) => (r as HTMLInputElement).value)).toEqual(["read", "plan", "full"]);
        expect(radios[0]).toBeChecked();
        expect(d.getByRole("radio", {name: /Read only \(recommended\)/})).toHaveAccessibleDescription(/It cannot change anything/);
        expect(d.getByRole("radio", {name: "Plans"})).toHaveAccessibleDescription(/A plan is only a description: nothing is changed/);
        expect(d.getByRole("radio", {name: "Full"})).toHaveAccessibleDescription(/only after you approve that plan in the assistant, one by one/);
    });

    it("changes the commands with the mode, exactly as Go built them", async () => {
        const user = await openApp();
        const d = await openAssistant(user);
        const code = await d.findByRole("group", {name: "Command for Claude Code"});
        const desktop = d.getByRole("group", {name: "Settings for Claude Desktop"});
        expect(code.textContent).toBe(sampleAssistant.commands.claudeCode.read);
        expect(code.textContent).toBe(`claude mcp add winforge -- "${SAMPLE_EXE}"`);
        expect(desktop.textContent).toBe(sampleAssistant.commands.claudeDesktop.read);
        expect(d.getByText(/What to copy, for Read only \(recommended\)/)).toBeInTheDocument();

        await user.click(d.getByRole("radio", {name: "Plans"}));
        expect(code.textContent).toBe(`claude mcp add winforge -- "${SAMPLE_EXE}" --mode plan`);
        expect(JSON.parse(desktop.textContent ?? "").mcpServers.winforge.args).toEqual(["--mode", "plan"]);

        await user.click(d.getByRole("radio", {name: "Full"}));
        expect(code.textContent).toBe(`claude mcp add winforge -- "${SAMPLE_EXE}" --mode full`);
        expect(JSON.parse(desktop.textContent ?? "").mcpServers.winforge.args).toEqual(["--mode", "full"]);
        expect(JSON.parse(desktop.textContent ?? "").mcpServers.winforge.command).toBe(SAMPLE_EXE);
    });

    it("is usable with the arrow keys on the mode picker", async () => {
        const user = await openApp();
        const d = await openAssistant(user);
        await d.findByRole("group", {name: "Command for Claude Code"});
        d.getByRole("radio", {name: /Read only/}).focus();
        await user.keyboard("{ArrowDown}");
        expect(d.getByRole("radio", {name: "Plans"})).toBeChecked();
        expect(d.getByRole("group", {name: "Command for Claude Code"}).textContent).toMatch(/--mode plan$/);
    });

    it("copies the exact command of the chosen mode and says Copied in a status region", async () => {
        const user = await openApp();
        const writeText = mockClipboard(async () => {});
        const d = await openAssistant(user);
        await d.findByRole("group", {name: "Command for Claude Code"});
        await user.click(d.getByRole("radio", {name: "Full"}));
        const status = d.getAllByRole("status").filter((s) => s.classList.contains("assistcopied"));
        expect(status).toHaveLength(2);
        expect(status.every((s) => s.textContent === "")).toBe(true);

        await user.click(d.getByRole("button", {name: "Copy: Command for Claude Code"}));
        expect(writeText).toHaveBeenLastCalledWith(sampleAssistant.commands.claudeCode.full);
        await waitFor(() => expect(status[0]).toHaveTextContent("Copied"));
        expect(status[1]).toHaveTextContent(/^$/);

        await user.click(d.getByRole("button", {name: "Copy: Settings for Claude Desktop"}));
        expect(writeText).toHaveBeenLastCalledWith(sampleAssistant.commands.claudeDesktop.full);
        await waitFor(() => expect(status[1]).toHaveTextContent("Copied"));
        expect(status[0]).toHaveAttribute("aria-live", "polite");

        // changing the mode forgets the old "Copied": the text on screen is no longer what was copied
        await user.click(d.getByRole("radio", {name: "Plans"}));
        expect(status[0]).toHaveTextContent(/^$/);
    });

    it("survives a clipboard that refuses, and one that does not exist, and tells the person", async () => {
        const user = await openApp();
        mockClipboard(async () => { throw new Error("denied"); });
        const d = await openAssistant(user);
        await d.findByRole("group", {name: "Command for Claude Code"});
        await user.click(d.getByRole("button", {name: "Copy: Command for Claude Code"}));
        expect(await d.findByText("Could not copy. Select the text above and copy it yourself.")).toBeInTheDocument();
        expect(d.queryByText("Copied")).not.toBeInTheDocument();
        // the dialog is still alive: another mode, another copy
        await user.click(d.getByRole("radio", {name: "Plans"}));
        expect(d.getByRole("group", {name: "Command for Claude Code"}).textContent).toMatch(/--mode plan$/);

        Object.defineProperty(navigator, "clipboard", {value: undefined, configurable: true});
        await user.click(d.getByRole("button", {name: "Copy: Settings for Claude Desktop"}));
        expect(await d.findByText("Could not copy. Select the text above and copy it yourself.")).toBeInTheDocument();
        expect(screen.getByRole("dialog", {name: "Assistant (MCP)"})).toBeInTheDocument();
    });

    it("shows where the program is and reveals it in the folder", async () => {
        const user = await openApp();
        const d = await openAssistant(user);
        expect(await d.findByText(SAMPLE_EXE)).toBeInTheDocument();
        expect(d.getByText("winforge-mcp.exe is here:")).toBeInTheDocument();
        await user.click(d.getByRole("button", {name: "Show in folder"}));
        expect(api.AssistantRevealExe).toHaveBeenCalledTimes(1);
        expect(d.queryByRole("alert")).not.toBeInTheDocument();
    });

    it("reports a folder that could not be opened", async () => {
        api.AssistantRevealExe.mockRejectedValueOnce(new Error("gone"));
        const user = await openApp();
        const d = await openAssistant(user);
        await user.click(await d.findByRole("button", {name: "Show in folder"}));
        expect(await d.findByRole("alert")).toHaveTextContent("Could not open the folder: gone");
    });

    it("explains a missing program, shows no commands, and checks again on request", async () => {
        api.AssistantInfo.mockResolvedValueOnce(structuredClone(missingAssistant));
        const user = await openApp();
        const d = await openAssistant(user);
        expect(await d.findByText(/winforge-mcp.exe was not found next to WinForge.exe\. If you use the portable WinForge.exe, download winforge-mcp from the WinForge release/)).toBeInTheDocument();
        expect(d.queryByRole("group", {name: "Command for Claude Code"})).not.toBeInTheDocument();
        expect(d.queryByRole("button", {name: "Show in folder"})).not.toBeInTheDocument();
        expect(d.queryByRole("button", {name: /^Copy:/})).not.toBeInTheDocument();
        // the privacy box and the mode picker are still there
        expect(d.getByRole("heading", {name: "Before you connect"})).toBeInTheDocument();
        expect(d.getAllByRole("radio")).toHaveLength(3);

        await user.click(d.getByRole("button", {name: "Check again"}));
        expect(await d.findByRole("group", {name: "Command for Claude Code"})).toBeInTheDocument();
        expect(api.AssistantInfo).toHaveBeenCalledTimes(2);
    });

    it("reports a failed lookup and retries", async () => {
        api.AssistantInfo.mockRejectedValueOnce(new Error("boom"));
        const user = await openApp();
        const d = await openAssistant(user);
        expect(await d.findByRole("alert")).toHaveTextContent("Could not look for the program: boom");
        await user.click(d.getByRole("button", {name: "Check again"}));
        expect(await d.findByRole("group", {name: "Command for Claude Code"})).toBeInTheDocument();
    });

    it("closes with Escape only itself, keeps Settings, returns focus, and traps Tab", async () => {
        const user = await openApp();
        const settings = await openSettings(user);
        const opener = settings.getByRole("button", {name: "Connect an assistant…"});
        await user.click(opener);
        const dlg = await screen.findByRole("dialog", {name: "Assistant (MCP)"});
        await within(dlg).findByRole("group", {name: "Command for Claude Code"});
        expect(dlg.contains(document.activeElement)).toBe(true);
        for (let i = 0; i < 14; i++) {
            await user.tab();
            expect(dlg.contains(document.activeElement)).toBe(true);
        }
        await user.keyboard("{Escape}");
        await waitFor(() => expect(screen.queryByRole("dialog", {name: "Assistant (MCP)"})).not.toBeInTheDocument());
        expect(screen.getByRole("dialog", {name: "Appearance and language"})).toBeInTheDocument();
        expect(opener).toHaveFocus();
    });

    it("has the same content in Spanish", async () => {
        const user = await openApp();
        const settings = await openSettings(user);
        await user.click(settings.getByRole("button", {name: "Español"}));
        expect(settings.getByText("Asistente (MCP)")).toBeInTheDocument();
        await user.click(settings.getByRole("button", {name: "Conectar un asistente…"}));
        const d = within(await screen.findByRole("dialog", {name: "Asistente (MCP)"}));
        expect(d.getByRole("radio", {name: /Solo lectura \(recomendado\)/})).toBeChecked();
        expect(d.getByText(/WinForge no envía nada por sí mismo/)).toBeInTheDocument();
        const code = await d.findByRole("group", {name: "Comando para Claude Code"});
        await user.click(d.getByRole("radio", {name: "Completo"}));
        expect(code.textContent).toMatch(/--mode full$/);
        mockClipboard(async () => {});
        await user.click(d.getByRole("button", {name: "Copiar: Comando para Claude Code"}));
        expect(await d.findByText("Copiado")).toBeInTheDocument();
    });

    it("has the Spanish missing-program note", async () => {
        api.AssistantInfo.mockResolvedValueOnce(structuredClone(missingAssistant));
        const user = await openApp();
        await user.click((await openSettings(user)).getByRole("button", {name: "Español"}));
        const d = await openAssistant(user, "Conectar un asistente…", "Asistente (MCP)", true);
        expect(await d.findByText(/No se encontró winforge-mcp.exe junto a WinForge.exe/)).toBeInTheDocument();
        expect(d.getByRole("button", {name: "Comprobar de nuevo"})).toBeInTheDocument();
    });
});

describe("what assistive technology is told by the assistant dialog", () => {
    it("names every control, every code block and every status, and has no duplicate ids", async () => {
        const user = await openApp();
        const d = await openAssistant(user);
        await d.findByRole("group", {name: "Command for Claude Code"});
        const dlg = screen.getByRole("dialog", {name: "Assistant (MCP)"});
        expect(dlg).toHaveAttribute("aria-modal", "true");
        for (const b of d.getAllByRole("button")) expect(b, b.outerHTML).toHaveAccessibleName();
        for (const r of d.getAllByRole("radio")) { expect(r).toHaveAccessibleName(); expect(r).toHaveAccessibleDescription(); }
        for (const pre of dlg.querySelectorAll("pre")) {
            expect(pre).toHaveAccessibleName();
            expect(pre).toHaveAttribute("tabindex", "0"); // a scrollable box must be reachable with the keyboard
        }
        // copy buttons are told apart by what they copy
        const names = d.getAllByRole("button", {name: /^Copy:/}).map((b) => b.getAttribute("aria-label"));
        expect(new Set(names).size).toBe(2);
        // the visible word is in the accessible name (WCAG 2.5.3)
        for (const n of names) expect(n).toMatch(/^Copy/);
        const ids = [...dlg.querySelectorAll("[id]")].map((e) => e.id);
        expect(new Set(ids).size).toBe(ids.length);
        // every aria-describedby / labelledby points at something that exists
        for (const el of dlg.querySelectorAll("[aria-describedby], [aria-labelledby]")) {
            for (const id of (el.getAttribute("aria-describedby") ?? el.getAttribute("aria-labelledby") ?? "").split(" ")) {
                expect(dlg.querySelector(`[id="${id}"]`), id).not.toBeNull();
            }
        }
        // headings give a screen reader something to jump by
        expect(d.getAllByRole("heading").length).toBeGreaterThanOrEqual(5);
    });
});

describe("the assistant dialog's CSS", () => {
    const css = readFileSync(resolve(__dirname, "..", "assistant.css"), "utf8");
    it("keeps code and paths inside the dialog at narrow widths: wrap, scroll, shrink", () => {
        expect(css).toMatch(/\.modal\.assistdlg\s*\{[^}]*overflow:\s*auto/);
        expect(css).toMatch(/\.modal\.assistdlg\s*\{[^}]*min-width:\s*0/);
        expect(css).toMatch(/\.modal\.assistdlg\s*\{[^}]*width:\s*min\(\d+px,\s*100%\)/);
        expect(css).toMatch(/\.assistdlg > \*\s*\{\s*min-width:\s*0/);
        expect(css).toMatch(/\.assistcode\s*\{[^}]*max-width:\s*100%/);
        expect(css).toMatch(/\.assistcode\s*\{[^}]*white-space:\s*pre-wrap/);
        expect(css).toMatch(/\.assistcode\s*\{[^}]*overflow-wrap:\s*anywhere/);
        expect(css).toMatch(/\.assistcode\s*\{[^}]*overflow:\s*auto/);
        expect(css).toMatch(/\.assistpath\s*\{[^}]*overflow-wrap:\s*anywhere/);
        expect(css).toMatch(/\.assistmodetext\s*\{[^}]*min-width:\s*0/);
        expect(css).toMatch(/@media \(max-width: 520px\)\s*\{[^}]*\.modal\.assistdlg\s*\{[^}]*padding/);
    });
    it("shows keyboard focus on a mode card and uses only the repo's colour tokens", () => {
        expect(css).toMatch(/\.assistmode:focus-within\s*\{[^}]*outline:\s*2px solid var\(--focus\)/);
        expect(css).not.toMatch(/#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(/);
    });
});
