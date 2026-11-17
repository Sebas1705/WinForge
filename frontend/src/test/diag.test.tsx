import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import App from "../App";
import {DICTS, KEYS} from "../lib/i18n";
import {api, resetApi} from "./apiMock";
import {sampleReport} from "./diagMock";

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

async function openSettings(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByRole("button", {name: "Appearance and language"}));
    return within(screen.getByRole("dialog"));
}

describe("diagnostic strings", () => {
    it("has every diag.* key in Spanish and English, never empty", () => {
        const diagKeys = KEYS.filter((k) => k.startsWith("diag."));
        expect(diagKeys.length).toBeGreaterThanOrEqual(18);
        for (const k of diagKeys) {
            expect(DICTS.en[k].trim(), `en ${k}`).not.toBe("");
            expect(DICTS.es[k].trim(), `es ${k}`).not.toBe("");
            // the same placeholders in both languages
            const ph = (s: string) => (s.match(/\{\w+\}/g) ?? []).sort().join();
            expect(ph(DICTS.es[k]), k).toBe(ph(DICTS.en[k]));
        }
    });

    it("tells the person nothing is sent anywhere and what is and is not in the report", () => {
        expect(DICTS.en["diag.promise"]).toMatch(/sends nothing anywhere/);
        expect(DICTS.es["diag.promise"]).toMatch(/no envía nada a ningún sitio/);
        expect(DICTS.en["diag.excludes"]).toMatch(/installed on your PC.*environment variables.*registry/s);
        expect(DICTS.es["diag.excludes"]).toMatch(/software instalado.*variables de entorno.*registro/s);
    });
});

describe("the diagnostic report in Settings", () => {
    it("shows a Diagnostics section with a button, and opens a preview of exactly what Go built", async () => {
        const user = await openApp();
        const settings = await openSettings(user);
        expect(settings.getByText("Diagnostics")).toBeInTheDocument();
        expect(api.DiagPreview).not.toHaveBeenCalled(); // nothing is built until it is asked for
        await user.click(settings.getByRole("button", {name: "Save a diagnostic report…"}));

        const dlg = await screen.findByRole("dialog", {name: "Diagnostic report"});
        const d = within(dlg);
        expect(d.getByText(/WinForge sends nothing anywhere/)).toBeInTheDocument();
        expect(d.getByText(/It never contains: the list of software installed/)).toBeInTheDocument();
        const box = await d.findByRole("textbox", {name: "Exactly what will be saved"});
        await waitFor(() => expect(box).toHaveValue(sampleReport));
        expect(box).toHaveAttribute("readonly");
        expect(api.DiagPreview).toHaveBeenCalledTimes(1);
        expect(api.DiagSave).not.toHaveBeenCalled(); // previewing writes nothing
    });

    it("saves with the person's own file choice and says where, without sending anything", async () => {
        const user = await openApp();
        await user.click((await openSettings(user)).getByRole("button", {name: "Save a diagnostic report…"}));
        const d = within(await screen.findByRole("dialog", {name: "Diagnostic report"}));
        const save = d.getByRole("button", {name: "Save…"});
        await waitFor(() => expect(save).toBeEnabled());
        await user.click(save);
        await waitFor(() => expect(api.DiagSave).toHaveBeenCalledWith(""));
        expect(await d.findByText(/Saved to C:\\Reports\\winforge-diagnostics\.json\. WinForge has not sent it anywhere/)).toBeInTheDocument();
        expect(api.DiagPreview).toHaveBeenCalledTimes(1); // the saved text is the previewed one, not a new build
    });

    it("stays quiet when the person cancels the Save dialog", async () => {
        api.DiagSave.mockResolvedValue("");
        const user = await openApp();
        await user.click((await openSettings(user)).getByRole("button", {name: "Save a diagnostic report…"}));
        const d = within(await screen.findByRole("dialog", {name: "Diagnostic report"}));
        await waitFor(() => expect(d.getByRole("button", {name: "Save…"})).toBeEnabled());
        await user.click(d.getByRole("button", {name: "Save…"}));
        await waitFor(() => expect(api.DiagSave).toHaveBeenCalled());
        expect(d.queryByText(/Saved to/)).not.toBeInTheDocument();
        expect(d.queryByRole("alert")).not.toBeInTheDocument();
    });

    it("explains a save that failed and a path that is not allowed", async () => {
        api.DiagSave.mockRejectedValueOnce(new Error("diag:path"));
        const user = await openApp();
        await user.click((await openSettings(user)).getByRole("button", {name: "Save a diagnostic report…"}));
        const d = within(await screen.findByRole("dialog", {name: "Diagnostic report"}));
        await waitFor(() => expect(d.getByRole("button", {name: "Save…"})).toBeEnabled());
        await user.click(d.getByRole("button", {name: "Save…"}));
        expect(await d.findByText("That file name is not allowed. Choose a .json or .txt file.")).toBeInTheDocument();
        api.DiagSave.mockRejectedValueOnce(new Error("disk full"));
        await user.click(d.getByRole("button", {name: "Save…"}));
        expect(await d.findByText("The report could not be saved: disk full")).toBeInTheDocument();
    });

    it("reports a report that could not be prepared and lets the person try again", async () => {
        api.DiagPreview.mockRejectedValueOnce(new Error("boom"));
        const user = await openApp();
        await user.click((await openSettings(user)).getByRole("button", {name: "Save a diagnostic report…"}));
        const d = within(await screen.findByRole("dialog", {name: "Diagnostic report"}));
        expect(await d.findByRole("alert")).toHaveTextContent("The report could not be prepared: boom");
        expect(d.getByRole("button", {name: "Save…"})).toBeDisabled();
        await user.click(d.getByRole("button", {name: "Try again"}));
        await waitFor(() => expect(d.getByRole("textbox")).toHaveValue(sampleReport));
        expect(d.getByRole("button", {name: "Save…"})).toBeEnabled();
    });

    it("is usable with the keyboard: Escape closes only the report, Settings stays", async () => {
        const user = await openApp();
        await user.click((await openSettings(user)).getByRole("button", {name: "Save a diagnostic report…"}));
        await screen.findByRole("dialog", {name: "Diagnostic report"});
        await user.keyboard("{Escape}");
        await waitFor(() => expect(screen.queryByRole("dialog", {name: "Diagnostic report"})).not.toBeInTheDocument());
        expect(screen.getByRole("dialog", {name: "Appearance and language"})).toBeInTheDocument();
        // and Tab never leaves the report dialog while it is open
        await user.click(within(screen.getByRole("dialog", {name: "Appearance and language"})).getByRole("button", {name: "Save a diagnostic report…"}));
        const dlg = await screen.findByRole("dialog", {name: "Diagnostic report"});
        for (let i = 0; i < 8; i++) {
            await user.tab();
            expect(dlg.contains(document.activeElement)).toBe(true);
        }
    });

    it("speaks Spanish when the language is Spanish", async () => {
        const user = await openApp();
        const settings = await openSettings(user);
        await user.click(settings.getByRole("button", {name: "Español"}));
        expect(settings.getByText("Diagnóstico")).toBeInTheDocument();
        await user.click(settings.getByRole("button", {name: "Guardar un informe de diagnóstico…"}));
        const d = within(await screen.findByRole("dialog", {name: "Informe de diagnóstico"}));
        expect(d.getByText(/WinForge no envía nada a ningún sitio/)).toBeInTheDocument();
        expect(await d.findByRole("textbox", {name: "Exactamente lo que se guardará"})).toBeInTheDocument();
        await waitFor(() => expect(d.getByRole("button", {name: "Guardar…"})).toBeEnabled());
        await user.click(d.getByRole("button", {name: "Guardar…"}));
        expect(await d.findByText(/Guardado en C:\\Reports\\winforge-diagnostics\.json\. WinForge no lo ha enviado a ningún sitio/)).toBeInTheDocument();
    });
});

async function openCatalog(user: ReturnType<typeof userEvent.setup>) {
    await user.click(within(screen.getByRole("navigation", {name: "Main menu"})).getByRole("button", {name: /Catalog/}));
    return screen.getByRole("textbox", {name: "Search apps"});
}

describe("Not here? Suggest this app", () => {
    it("is not shown while the search finds apps or before anything is typed", async () => {
        const user = await openApp();
        const search = await openCatalog(user);
        expect(screen.queryByRole("button", {name: /Suggest this app/})).not.toBeInTheDocument();
        await user.type(search, "firefox");
        expect(await screen.findAllByText(/Firefox/)).not.toHaveLength(0);
        expect(screen.queryByRole("button", {name: /Suggest this app/})).not.toBeInTheDocument();
    });

    it("opens a pre-filled GitHub issue in the browser with only the typed text, and says it will", async () => {
        const user = await openApp();
        const search = await openCatalog(user);
        await user.type(search, "Zorblax & Co #1");
        const link = await screen.findByRole("button", {name: "Not here? Suggest this app"});
        expect(link).toHaveAccessibleDescription(/opens your web browser on GitHub.*“Zorblax & Co #1”.*Only the text you typed in the search box/);
        expect(api.OpenURL).not.toHaveBeenCalled();
        await user.click(link);
        expect(api.OpenURL).toHaveBeenCalledTimes(1);
        const url = new URL(api.OpenURL.mock.calls[0][0] as string);
        expect(url.origin + url.pathname).toBe("https://github.com/Sebas1705/WinForge/issues/new");
        expect(url.searchParams.get("title")).toBe("App request: Zorblax & Co #1");
        expect(url.searchParams.get("app-name")).toBe("Zorblax & Co #1");
        expect(url.searchParams.get("labels")).toBe("app-request");
        expect(url.hash).toBe("");
    });

    it("works from the keyboard", async () => {
        const user = await openApp();
        const search = await openCatalog(user);
        await user.type(search, "Zorblax");
        const link = await screen.findByRole("button", {name: /Suggest this app/});
        link.focus();
        await user.keyboard("{Enter}");
        expect(api.OpenURL).toHaveBeenCalledTimes(1);
    });

    it("is in the list view too, and in Spanish", async () => {
        const user = await openApp();
        await user.click(screen.getByRole("button", {name: "Appearance and language"}));
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Español"}));
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Listo"}));
        await user.click(within(screen.getByRole("navigation")).getByRole("button", {name: /Catálogo/}));
        await user.click(screen.getByRole("button", {name: "Lista"}));
        await user.type(screen.getByRole("textbox", {name: "Buscar apps"}), "Zorblax");
        const link = await screen.findByRole("button", {name: /¿No está aquí\? Sugiere esta app/});
        expect(link).toHaveAccessibleDescription(/Esto abre tu navegador en GitHub.*«Zorblax»/);
        await user.click(link);
        expect(api.OpenURL).toHaveBeenCalledTimes(1);
    });

    it("cuts a pasted paragraph instead of putting it in the address", async () => {
        const user = await openApp();
        const search = await openCatalog(user);
        await user.click(search);
        await user.paste("zzz ".repeat(200));
        await user.click(await screen.findByRole("button", {name: /Suggest this app/}));
        const url = new URL(api.OpenURL.mock.calls[0][0] as string);
        expect((url.searchParams.get("app-name") ?? "").length).toBeLessThanOrEqual(80);
        expect(api.OpenURL.mock.calls[0][0].length).toBeLessThan(4000);
    });
});
