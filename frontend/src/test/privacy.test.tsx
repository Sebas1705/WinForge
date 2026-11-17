import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {PrivacyBackup, PrivacyChange, PrivacyTweak} from "../api";
import App from "../App";
import {DICTS, KEYS, type Key} from "../lib/i18n";
import {api, resetApi} from "./apiMock";
import {makeState} from "./fixtures";
// The Tasks list repeats what the page reports: look at the page's own text.
const pageText = async (x: string | RegExp) => (await screen.findAllByText(x)).find((e) => !e.closest(".taskviewer")) as HTMLElement;

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const CDM = "HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\ContentDeliveryManager";
const change = (o: Partial<PrivacyChange> & { name: string }): PrivacyChange => ({
    path: CDM, existed: false, oldType: "", old: "", type: "REG_DWORD", private: 0, error: false, ...o,
});
const tweak = (id: string, area: string, state: PrivacyTweak["state"], changes: PrivacyChange[]): PrivacyTweak => ({
    id, area, nameKey: `privacy.t.${id}.name`, whyKey: `privacy.t.${id}.why`, state, changes,
});
const fixture = (): PrivacyTweak[] => [
    tweak("advertising-id", "ads", "default", [change({path: "HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\AdvertisingInfo", name: "Enabled", existed: true, oldType: "REG_DWORD", old: "1"})]),
    tweak("tailored-experiences", "ads", "custom", [change({name: "TailoredExperiencesWithDiagnosticDataEnabled", existed: true, oldType: "REG_DWORD", old: "2"})]),
    tweak("start-suggestions", "suggestions", "private", [change({name: "SystemPaneSuggestionsEnabled", existed: true, oldType: "REG_DWORD", old: "0"}), change({name: "SubscribedContent-338388Enabled", existed: true, oldType: "REG_DWORD", old: "0"})]),
    tweak("start-most-used", "start", "default", [change({path: "HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Explorer\\Advanced", name: "Start_TrackProgs"})]),
    tweak("language-list", "web", "unknown", [change({name: "HttpAcceptLanguageOptOut", error: true})]),
];
const backup = (id: string, o: Partial<PrivacyBackup> = {}): PrivacyBackup => ({id, time: "2026-11-17T10:00:00Z", action: "apply", tweaks: ["advertising-id"], restored: null, ...o});

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi(makeState({admin: false}));
    api.PrivacyList.mockImplementation(async () => fixture());
});

async function open(lang?: "es") {
    if (lang) localStorage.setItem("winforge.settings", JSON.stringify({language: lang}));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(lang === "es" ? /apps del catálogo/ : /catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: lang === "es" ? "Menú principal" : "Main menu"})).getByRole("button", {name: lang === "es" ? /Privacidad/ : /Privacy/}));
    await screen.findByRole("heading", {name: lang === "es" ? "Privacidad" : "Privacy", level: 1});
    return user;
}
const box = (name: string) => screen.findByRole("checkbox", {name: `Make private: ${name}`}) as Promise<HTMLInputElement>;

describe("privacy: the page", () => {
    it("sits after Developer in the menu and lists every setting with its state, nothing ticked", async () => {
        const user = await open();
        const names = within(screen.getByRole("navigation", {name: "Main menu"})).getAllByRole("button").map((b) => b.textContent ?? "");
        expect(names.findIndex((n) => /Privacy/.test(n))).toBe(names.findIndex((n) => /Developer/.test(n)) + 1);

        expect((await box("Advertising ID for apps")).checked).toBe(false);
        for (const c of screen.getAllByRole("checkbox")) expect((c as HTMLInputElement).checked).toBe(false);
        expect(screen.getByRole("button", {name: "Apply selected (0)"})).toBeDisabled();
        expect(screen.getByText("1 of 5 are set to private")).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "Advertising and personalization", level: 2})).toBeInTheDocument();
        expect(screen.getByText("Other value")).toBeInTheDocument();
        expect(screen.getByText("Private", {selector: ".chip"})).toBeInTheDocument();
        expect(screen.getAllByText("Windows default", {selector: ".chip"})).toHaveLength(2);
        // Already private and unreadable settings cannot be ticked.
        expect(await box("Suggested apps in Start")).toBeDisabled();
        expect(await box("Language list for websites")).toBeDisabled();
        expect(screen.getByText(/No restore point is made/)).toBeInTheDocument();
        expect(screen.getByText(/Left out on purpose/)).toBeInTheDocument();
        expect(user).toBeTruthy();
    });

    it("shows the exact registry values in a preview", async () => {
        const user = await open();
        await box("Advertising ID for apps");
        const li = screen.getByText("Advertising ID for apps", {selector: "b"}).closest("li") as HTMLElement;
        await user.click(within(li).getByText("Exact changes"));
        expect(within(li).getByText(/AdvertisingInfo$/)).toBeInTheDocument();
        expect(within(li).getByText("Enabled")).toBeInTheDocument();
        expect(within(li).getByText("REG_DWORD 1")).toBeInTheDocument();
        expect(within(li).getByText("REG_DWORD 0")).toBeInTheDocument();
        // A value that is absent is said so; a private setting previews the reset instead.
        const mu = screen.getByText("Most used apps in Start", {selector: "b"}).closest("li") as HTMLElement;
        expect(within(mu).getByText("not set")).toBeInTheDocument();
        const pr = screen.getByText("Suggested apps in Start", {selector: "b"}).closest("li") as HTMLElement;
        expect(within(pr).getAllByText("removed")).toHaveLength(2);
    });

    it("asks first, shows the preview, and changes nothing until confirmed", async () => {
        const user = await open();
        await user.click(await box("Advertising ID for apps"));
        await user.click(await box("Most used apps in Start"));
        await user.click(screen.getByRole("button", {name: "Apply selected (2)"}));
        const dlg = await screen.findByRole("alertdialog", {name: "Make the selected settings private (2)?"});
        expect(within(dlg).getByText(/Enabled: REG_DWORD 1 → REG_DWORD 0/)).toBeInTheDocument();
        expect(within(dlg).getByText(/Start_TrackProgs: not set → REG_DWORD 0/)).toBeInTheDocument();
        expect(api.PrivacyApply).not.toHaveBeenCalled();
        await user.click(within(dlg).getByRole("button", {name: "Cancel"}));
        expect(api.PrivacyApply).not.toHaveBeenCalled();

        api.PrivacyApply.mockResolvedValueOnce({applied: 2, skipped: [], failed: [], backupId: "20261117T100000Z"});
        api.PrivacyBackups.mockResolvedValue([backup("20261117T100000Z", {tweaks: ["advertising-id", "start-most-used"]})]);
        await user.click(screen.getByRole("button", {name: "Apply selected (2)"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Apply"}));
        await waitFor(() => expect(api.PrivacyApply).toHaveBeenCalledWith(["advertising-id", "start-most-used"]));
        expect(await screen.findByText("Changed 2. A backup was saved.")).toBeInTheDocument();
        expect(await pageText(/Some changes show up/)).toBeInTheDocument();
        expect(api.PrivacyList).toHaveBeenCalledTimes(2); // read again after the change
        expect((await box("Advertising ID for apps")).checked).toBe(false);
        expect(await screen.findByRole("button", {name: "Undo last change"})).toBeInTheDocument();
    });

    it("reports what was skipped or failed", async () => {
        const user = await open();
        await user.click(await box("Advertising ID for apps"));
        api.PrivacyApply.mockResolvedValueOnce({applied: 0, skipped: ["start-most-used"], failed: ["advertising-id"], backupId: ""});
        await user.click(screen.getByRole("button", {name: "Apply selected (1)"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Apply"}));
        expect(await screen.findByText("Nothing was changed.")).toBeInTheDocument();
        expect(screen.getByText("Already as asked: Most used apps in Start")).toBeInTheDocument();
        expect(screen.getByText("Could not change: Advertising ID for apps")).toBeInTheDocument();
    });

    it("resets one setting to the Windows default without asking, and offers no reset when it already is", async () => {
        const user = await open();
        await box("Advertising ID for apps");
        expect(screen.queryByRole("button", {name: "Reset to Windows default: Advertising ID for apps"})).not.toBeInTheDocument();
        api.PrivacyReset.mockResolvedValueOnce({applied: 1, skipped: [], failed: [], backupId: "20261117T100000Z"});
        await user.click(screen.getByRole("button", {name: "Reset to Windows default: Suggested apps in Start"}));
        await waitFor(() => expect(api.PrivacyReset).toHaveBeenCalledWith(["start-suggestions"]));
        expect(await screen.findByText("Back to the Windows default: 1. A backup was saved.")).toBeInTheDocument();
        expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
        // A setting with some other value can be reset too; an unreadable one cannot.
        expect(screen.getByRole("button", {name: "Reset to Windows default: Tips and ads based on diagnostic data"})).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: "Reset to Windows default: Language list for websites"})).not.toBeInTheDocument();
    });

    it("undoes the last change and says what someone else had edited since", async () => {
        api.PrivacyBackups.mockResolvedValue([backup("20261117T120000Z", {restored: "2026-11-17T12:30:00Z"}), backup("20261117T110000Z", {tweaks: ["advertising-id", "tailored-experiences"]}), backup("20261117T100000Z")]);
        const user = await open();
        api.PrivacyRestore.mockResolvedValueOnce({restored: ["advertising-id", "tailored-experiences"], changedSince: ["tailored-experiences"], failed: []});
        await user.click(await screen.findByRole("button", {name: "Undo last change"}));
        await waitFor(() => expect(api.PrivacyRestore).toHaveBeenCalledWith("20261117T110000Z")); // newest not yet restored
        expect(await pageText(/Something else had changed these since.*Tips and ads based on diagnostic data/)).toBeInTheDocument();
    });

    it("lists earlier backups and restores one after confirming", async () => {
        api.PrivacyBackups.mockResolvedValue([backup("20261117T100000Z", {action: "reset", restored: "2026-11-17T12:30:00Z"})]);
        const user = await open();
        expect(await screen.findByText(/reset to default, Advertising ID for apps/)).toBeInTheDocument();
        expect(screen.getByText("restored", {selector: ".chip"})).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: "Undo last change"})).not.toBeInTheDocument(); // everything restored
        await user.click(screen.getByRole("button", {name: /^Restore the backup of/}));
        api.PrivacyRestore.mockResolvedValueOnce({restored: ["advertising-id"], changedSince: [], failed: []});
        const dlg = await screen.findByRole("alertdialog", {name: "Restore this backup?"});
        expect(api.PrivacyRestore).not.toHaveBeenCalled();
        await user.click(within(dlg).getByRole("button", {name: "Restore"}));
        await waitFor(() => expect(api.PrivacyRestore).toHaveBeenCalledWith("20261117T100000Z"));
        expect(await pageText("Your earlier values are back.")).toBeInTheDocument();
    });

    it("shows a failed read as a message and a failed change through the toast", async () => {
        api.PrivacyList.mockRejectedValueOnce(new Error("registry unavailable"));
        await open();
        expect(await screen.findByText("Could not read your settings: registry unavailable")).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: /Apply selected/})).not.toBeInTheDocument();
    });

    it("works in Spanish", async () => {
        const user = await open("es");
        expect((await screen.findByRole("checkbox", {name: "Hacer privado: ID de publicidad para apps"}) as HTMLInputElement).checked).toBe(false);
        expect(screen.getByText("1 de 5 están en privado")).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "Publicidad y personalización", level: 2})).toBeInTheDocument();
        expect(screen.getAllByText("Valor de Windows", {selector: ".chip"})).toHaveLength(2);
        await user.click(screen.getByRole("checkbox", {name: "Hacer privado: ID de publicidad para apps"}));
        await user.click(screen.getByRole("button", {name: "Aplicar seleccionados (1)"}));
        const dlg = await screen.findByRole("alertdialog", {name: "¿Hacer privados los ajustes seleccionados (1)?"});
        expect(within(dlg).getByText(/Enabled: REG_DWORD 1 → REG_DWORD 0/)).toBeInTheDocument();
    });
});

describe("privacy: texts", () => {
    // The ids of the Go tweak table (internal/privacy/tweaks.go): each needs a name and a why in both languages.
    const ids = ["advertising-id", "tailored-experiences", "language-list", "start-suggestions", "settings-suggestions", "tips-welcome",
        "lockscreen-tips", "start-most-used", "start-recent-items", "start-web-search", "feedback-prompts"];
    it("has a name and a reason for every tweak, and a title for every group, in es and en", () => {
        const keys = [...ids.flatMap((id) => [`privacy.t.${id}.name`, `privacy.t.${id}.why`]), ...["ads", "suggestions", "start", "web", "feedback"].map((a) => `privacy.area.${a}`),
            ...["private", "default", "custom", "unknown"].map((s) => `privacy.state.${s}`), "nav.privacy"];
        for (const k of keys) {
            expect(KEYS, k).toContain(k as Key);
            expect(DICTS.en[k as Key], `en ${k}`).toBeTruthy();
            expect(DICTS.es[k as Key], `es ${k}`).toBeTruthy();
        }
    });
});
