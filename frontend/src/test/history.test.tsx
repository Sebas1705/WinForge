import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {HistoryEntry, HistoryPage} from "../api";
import App from "../App";
import {dayKey, groupByDay, historyError} from "../views/History";
import {api, emit, resetApi} from "./apiMock";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const at = (day: number, h: number, m = 0) => new Date(2026, 10, day, h, m).toISOString();
const entry = (id: string, app: string, o: Partial<HistoryEntry> = {}): HistoryEntry => ({
    id, app, name: app.toUpperCase(), action: "update", from: "1.0", to: "2.0", at: at(14, 10), result: "ok", source: "update", canRollback: false, ...o,
});
const page = (entries: HistoryEntry[], total = entries.length): HistoryPage => ({
    entries, total, apps: [{id: "git", name: "Git"}, {id: "vlc", name: "VLC"}],
});
const rollbackStep = {kind: "rollback" as const, id: "git", name: "Git", version: "1.0"};

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi();
});

async function openUpdates(lang: "en" | "es" = "en") {
    if (lang === "es") localStorage.setItem("winforge.settings", JSON.stringify({language: "es"}));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(lang === "es" ? /apps del catálogo/ : /catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: lang === "es" ? "Menú principal" : "Main menu"})).getByRole("button", {name: lang === "es" ? /Actualizaciones/ : /Updates/}));
    await screen.findByRole("heading", {name: lang === "es" ? "Historial" : "History", level: 2});
    return user;
}

describe("history: the list", () => {
    it("says so when WinForge has done nothing yet, in both languages", async () => {
        await openUpdates();
        expect(await screen.findByText(/has not installed or updated anything yet/)).toBeInTheDocument();
        expect(screen.queryByRole("combobox", {name: "App"})).toBeNull();
    });

    it("says so in Spanish", async () => {
        await openUpdates("es");
        expect(await screen.findByText(/todavía no ha instalado ni actualizado nada/)).toBeInTheDocument();
    });

    it("lists newest first, grouped by day, with versions, kind, result and source", async () => {
        api.HistoryList.mockResolvedValue(page([
            entry("a", "git", {at: at(14, 10, 30)}),
            entry("b", "vlc", {action: "install", from: "", to: "3.0", at: at(14, 9), source: "profile"}),
            entry("c", "git", {action: "update", from: "0.9", to: "1.0", at: at(12, 8), result: "restart"}),
        ]));
        await openUpdates();
        await screen.findByRole("heading", {name: "History", level: 2});
        const days = within(document.querySelector("section.history") as HTMLElement).getAllByRole("heading", {level: 3});
        expect(days).toHaveLength(2);
        expect(days[0].textContent).toMatch(/14/);
        expect(days[1].textContent).toMatch(/12/);
        const lists = screen.getAllByRole("list", {name: /History entries/});
        expect(within(lists[0]).getAllByRole("listitem")).toHaveLength(2);
        expect(within(lists[0]).getAllByRole("listitem")[0]).toHaveTextContent("1.0 → 2.0");
        expect(within(lists[0]).getAllByRole("listitem")[1]).toHaveTextContent(/Installed/);
        expect(within(lists[0]).getAllByRole("listitem")[1]).toHaveTextContent("now 3.0");
        expect(within(lists[0]).getAllByRole("listitem")[1]).toHaveTextContent("From a profile");
        expect(within(lists[1]).getByRole("listitem")).toHaveTextContent("Restart needed");
        expect(screen.getByText("Showing 3 of 3")).toBeInTheDocument();
    });

    it("filters by app and by result, asking the backend, and says when nothing matches", async () => {
        api.HistoryList.mockImplementation(async (f: { app: string; result: string }) => {
            const all = [entry("a", "git"), entry("b", "vlc", {result: "failed", reason: "network"})];
            const out = all.filter((e) => (!f.app || e.app === f.app) && (!f.result || e.result === f.result));
            return page(out);
        });
        const user = await openUpdates();
        await screen.findByRole("combobox", {name: "App"});
        expect(api.HistoryList).toHaveBeenLastCalledWith({app: "", result: "", limit: 20});
        await user.selectOptions(screen.getByRole("combobox", {name: "App"}), "git");
        await waitFor(() => expect(api.HistoryList).toHaveBeenLastCalledWith({app: "git", result: "", limit: 20}));
        await waitFor(() => expect(screen.getByText("Showing 1 of 1")).toBeInTheDocument());
        await user.selectOptions(screen.getByRole("combobox", {name: "Result"}), "failed");
        await waitFor(() => expect(api.HistoryList).toHaveBeenLastCalledWith({app: "git", result: "failed", limit: 20}));
        expect(await screen.findByText("No entry matches these filters.")).toBeInTheDocument();
        // the filters stay reachable so the person can undo them
        await user.selectOptions(screen.getByRole("combobox", {name: "App"}), "");
        expect(await screen.findByText(/Network|download/i, {selector: ".why"})).toBeInTheDocument();
    });

    it("shows more 20 at a time", async () => {
        const many = Array.from({length: 20}, (_, i) => entry(`e${i}`, "git", {at: at(14, 10, i)}));
        api.HistoryList.mockImplementation(async (f: { limit: number }) => page(many.slice(0, f.limit), 45));
        const user = await openUpdates();
        expect(await screen.findByText("Showing 20 of 45")).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Show more"}));
        await waitFor(() => expect(api.HistoryList).toHaveBeenLastCalledWith({app: "", result: "", limit: 40}));
    });

    it("shows a failed step with its plain reason, or says there is none", async () => {
        api.HistoryList.mockResolvedValue(page([
            entry("a", "git", {result: "failed", reason: "downgrade", to: ""}),
            entry("b", "vlc", {result: "failed", reason: "", to: ""}),
        ]));
        await openUpdates();
        expect(await screen.findByText(/refused to put an older version over the newer one/)).toBeInTheDocument();
        expect(screen.getByText(/has no reason for this failure/)).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: /Go back/})).toBeNull();
    });

    it("keeps working when the history cannot be read", async () => {
        api.HistoryList.mockRejectedValue(new Error("disk unreadable"));
        await openUpdates();
        expect(await screen.findByText(/Could not read the history: disk unreadable/)).toBeInTheDocument();
    });
});

describe("history: going back", () => {
    const withUndo = () => {
        api.HistoryList.mockResolvedValue(page([
            entry("u1", "git", {canRollback: true}),
            entry("u0", "vlc", {from: "2.0", to: "3.0", at: at(12, 9)}),
        ]));
        api.HistoryRollbackPlan.mockResolvedValue({steps: [rollbackStep], alreadyInstalled: [], needsAdmin: true});
    };

    it("offers 'Go back' only on an update that can be undone", async () => {
        withUndo();
        await openUpdates();
        const buttons = await screen.findAllByRole("button", {name: /Go back/});
        expect(buttons).toHaveLength(1);
        expect(buttons[0]).toHaveAccessibleName("Go back to 1.0: Git");
        expect(buttons[0]).toHaveTextContent("Go back to 1.0");
    });

    it("asks first, says what can go wrong, and does nothing on Cancel", async () => {
        withUndo();
        const user = await openUpdates();
        await user.click(await screen.findByRole("button", {name: "Go back to 1.0: Git"}));
        const dialog = await screen.findByRole("alertdialog", {name: "Go back to Git 1.0?"});
        expect(within(dialog).getByText(/install version 1.0 of Git over the current 2.0/)).toBeInTheDocument();
        expect(within(dialog).getByText(/refuse to install an older version over a newer one/)).toBeInTheDocument();
        expect(within(dialog).getByText(/data and settings may not work with an older version/)).toBeInTheDocument();
        expect(within(dialog).getByRole("checkbox", {name: /Also skip version 2.0/})).not.toBeChecked();
        await user.click(within(dialog).getByRole("button", {name: "Cancel"}));
        expect(screen.queryByRole("alertdialog")).toBeNull();
        expect(api.HistoryRollbackPlan).not.toHaveBeenCalled();
        expect(api.HistoryRollback).not.toHaveBeenCalled();
    });

    it("previews the plan, then rolls back by entry id only, without skipping unless asked", async () => {
        withUndo();
        const user = await openUpdates();
        await user.click(await screen.findByRole("button", {name: "Go back to 1.0: Git"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Go back"}));
        const run = await screen.findByRole("dialog", {name: "Go back Git to 1.0"});
        expect(api.HistoryRollbackPlan).toHaveBeenCalledWith("u1");
        expect(api.HistoryRollback).not.toHaveBeenCalled(); // the plan window comes first
        await user.click(within(run).getByRole("button", {name: "Run 1 step"}));
        expect(api.HistoryRollback).toHaveBeenCalledWith("u1", false);
        expect(api.IgnoreUpdate).not.toHaveBeenCalled(); // the skip rule is the backend's, after it worked
    });

    it("passes the choice to also skip the newer version", async () => {
        withUndo();
        const user = await openUpdates();
        await user.click(await screen.findByRole("button", {name: "Go back to 1.0: Git"}));
        const dialog = await screen.findByRole("alertdialog");
        await user.click(within(dialog).getByRole("checkbox", {name: /Also skip version 2.0/}));
        await user.click(within(dialog).getByRole("button", {name: "Go back"}));
        await user.click(within(await screen.findByRole("dialog", {name: "Go back Git to 1.0"})).getByRole("button", {name: "Run 1 step"}));
        expect(api.HistoryRollback).toHaveBeenCalledWith("u1", true);
    });

    it("shows the plain reason when the downgrade is refused, and reads the history again", async () => {
        withUndo();
        const user = await openUpdates();
        await user.click(await screen.findByRole("button", {name: "Go back to 1.0: Git"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Go back"}));
        const run = await screen.findByRole("dialog", {name: "Go back Git to 1.0"});
        await user.click(within(run).getByRole("button", {name: "Run 1 step"}));
        const reads = api.HistoryList.mock.calls.length;
        await emit.install({step: rollbackStep, status: "start"});
        await emit.install({step: rollbackStep, status: "failed", error: "winget exited with code 0x8a15002b", reason: "downgrade"});
        await emit.done(["rollback:git"]);
        expect(await within(run).findByText(/refused to put an older version over the newer one/)).toBeInTheDocument();
        await waitFor(() => expect(api.HistoryList.mock.calls.length).toBeGreaterThan(reads));
    });

    it("shows the reason when winget has no such version", async () => {
        withUndo();
        const user = await openUpdates();
        await user.click(await screen.findByRole("button", {name: "Go back to 1.0: Git"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Go back"}));
        const run = await screen.findByRole("dialog", {name: "Go back Git to 1.0"});
        await user.click(within(run).getByRole("button", {name: "Run 1 step"}));
        await emit.install({step: rollbackStep, status: "failed", error: "x", reason: "version"});
        await emit.done(["rollback:git"]);
        expect(await within(run).findByText(/does not have that version any more/)).toBeInTheDocument();
    });

    it("explains a refusal from the backend (the app moved on) and refreshes the list", async () => {
        withUndo();
        api.HistoryRollbackPlan.mockRejectedValue(new Error("history.moved"));
        const user = await openUpdates();
        await user.click(await screen.findByRole("button", {name: "Go back to 1.0: Git"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Go back"}));
        expect(await screen.findByText(/Git is not at the version that update installed any more/)).toBeInTheDocument();
        expect(screen.queryByRole("dialog", {name: /Go back Git/})).toBeNull();
    });

    it("works in Spanish", async () => {
        withUndo();
        const user = await openUpdates("es");
        await user.click(await screen.findByRole("button", {name: "Volver a 1.0: Git"}));
        const dialog = await screen.findByRole("alertdialog", {name: "¿Volver a Git 1.0?"});
        expect(within(dialog).getByText(/Algunos instaladores se niegan/)).toBeInTheDocument();
        expect(within(dialog).getByRole("checkbox", {name: /Saltar también la versión 2.0/})).toBeInTheDocument();
        await user.click(within(dialog).getByRole("button", {name: "Volver atrás"}));
        const run = await screen.findByRole("dialog", {name: "Volver Git a 1.0"});
        await user.click(within(run).getByRole("button", {name: "Ejecutar 1 paso"}));
        expect(api.HistoryRollback).toHaveBeenCalledWith("u1", false);
    });

    it("can be reached and operated with the keyboard", async () => {
        withUndo();
        const user = await openUpdates();
        const go = await screen.findByRole("button", {name: "Go back to 1.0: Git"});
        go.focus();
        await user.keyboard("{Enter}");
        const dialog = await screen.findByRole("alertdialog");
        expect(dialog.contains(document.activeElement)).toBe(true); // focus moved into the dialog
        await user.keyboard("{Escape}");
        expect(screen.queryByRole("alertdialog")).toBeNull();
        expect(api.HistoryRollbackPlan).not.toHaveBeenCalled();
    });
});

describe("history: helpers", () => {
    it("groups consecutive entries by local day and keeps their order", () => {
        const g = groupByDay([entry("a", "git", {at: at(14, 23, 59)}), entry("b", "git", {at: at(14, 0, 1)}), entry("c", "git", {at: at(13, 12)})]);
        expect(g.map((x) => x.entries.map((e) => e.id))).toEqual([["a", "b"], ["c"]]);
        expect(dayKey("not a date")).toBe("");
    });

    it("turns the backend's codes into sentences and leaves other errors alone", () => {
        expect(historyError(new Error("history.unknown")).refresh).toBe(true);
        expect(historyError(new Error("history.notRollbackable")).text).toBe("That change cannot be undone from here.");
        expect(historyError(new Error("history.moved"), "Git").text).toMatch(/^Git is not at the version/);
        expect(historyError(new Error("disk on fire"))).toEqual({text: "disk on fire", refresh: false});
    });
});
