import {fireEvent, render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import App from "../App";
import {api, resetApi} from "./apiMock";

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

async function openPalette(user: ReturnType<typeof userEvent.setup>) {
    await user.keyboard("{Control>}k{/Control}");
    const dialog = await screen.findByRole("dialog", {name: "Command palette"});
    return {dialog, box: within(dialog).getByRole("combobox")};
}

const options = () => screen.getAllByRole("option").map((o) => o.querySelector(".grow")?.textContent);
const selected = () => screen.getByRole("option", {selected: true}).querySelector(".grow")?.textContent;

describe("the command palette", () => {
    it("opens with Ctrl+K, takes the typing, and lists pages and actions before anything is typed", async () => {
        const user = await openApp();
        const {dialog, box} = await openPalette(user);
        await waitFor(() => expect(box).toHaveFocus());
        expect(box).toHaveAttribute("aria-expanded", "true");
        expect(box).toHaveAttribute("aria-controls", within(dialog).getByRole("listbox").id);
        const list = options();
        for (const wanted of ["Home", "Profiles", "Catalog", "Updates", "PC health", "Storage", "Scan this PC again", "Check for updates"]) expect(list).toContain(wanted);
        expect(list.some((l) => l?.startsWith("Search the catalog"))).toBe(false); // nothing to search for yet
    });

    it("keeps the highlighted result in aria-activedescendant and moves it with the arrow keys, wrapping round", async () => {
        const user = await openApp();
        const {box} = await openPalette(user);
        const first = selected();
        const idOfSelected = () => screen.getByRole("option", {selected: true}).id;
        expect(box).toHaveAttribute("aria-activedescendant", idOfSelected());
        await user.keyboard("{ArrowDown}");
        expect(selected()).not.toBe(first);
        expect(box).toHaveAttribute("aria-activedescendant", idOfSelected());
        await user.keyboard("{ArrowUp}{ArrowUp}"); // above the first: the last one
        expect(selected()).toBe(options().at(-1));
        await user.keyboard("{ArrowDown}");
        expect(selected()).toBe(first);
        expect(box).toHaveFocus(); // focus never leaves the text box
    });

    it("runs the highlighted page with Enter, and closes", async () => {
        const user = await openApp();
        await openPalette(user);
        await user.keyboard("updates{Enter}");
        expect(screen.queryByRole("dialog", {name: "Command palette"})).not.toBeInTheDocument();
        expect(await screen.findByRole("heading", {level: 1, name: "Updates"})).toBeInTheDocument();
    });

    it("runs an action: Check for updates goes to the page and asks winget", async () => {
        const user = await openApp();
        await openPalette(user);
        await user.keyboard("check for updates{Enter}");
        expect(await screen.findByRole("heading", {level: 1, name: "Updates"})).toBeInTheDocument();
        await waitFor(() => expect(api.Upgrades).toHaveBeenCalled());
    });

    it("finds an app and opens its details", async () => {
        const user = await openApp();
        await openPalette(user);
        await user.keyboard("firef");
        expect(options()).toContain("Firefox");
        await user.keyboard("{Enter}");
        expect(await screen.findByRole("dialog", {name: "Firefox"})).toBeInTheDocument();
    });

    it("turns the text into a catalog search with the last choice, and fills the box", async () => {
        const user = await openApp();
        await openPalette(user);
        await user.keyboard("browser");
        const last = options().at(-1);
        expect(last).toBe("Search the catalog for “browser”");
        await user.click(screen.getAllByRole("option").at(-1)!);
        const box = await screen.findByPlaceholderText(/Search apps/);
        expect(box).toHaveValue("browser");
        await waitFor(() => expect(box).toHaveFocus());
    });

    it("searches again with the same text the second time", async () => {
        const user = await openApp();
        for (const round of [1, 2]) {
            await openPalette(user);
            await user.keyboard("zzqx");
            await user.click(screen.getAllByRole("option").at(-1)!);
            const box = await screen.findByPlaceholderText(/Search apps/);
            await waitFor(() => expect(box).toHaveValue("zzqx"));
            await user.clear(box); // the second round must put the text back
            expect(round).toBeGreaterThan(0);
            await user.keyboard("{Escape}");
        }
    });

    it("says when nothing matches, and Enter then does nothing", async () => {
        const user = await openApp();
        const {dialog} = await openPalette(user);
        await user.keyboard("q"); // one letter: no catalog search is offered yet, and nothing starts with q
        expect(screen.queryAllByRole("option")).toHaveLength(0);
        expect(within(dialog).getAllByText("Nothing matches").length).toBeGreaterThan(0);
        await user.keyboard("{Enter}");
        expect(screen.getByRole("dialog", {name: "Command palette"})).toBeInTheDocument(); // still open, nothing ran
    });

    it("always offers the catalog search for text that matches nothing else", async () => {
        const user = await openApp();
        await openPalette(user);
        await user.keyboard("qqqqqzzzz");
        expect(options()).toEqual(["Search the catalog for “qqqqqzzzz”"]);
    });

    it("announces how many results there are", async () => {
        const user = await openApp();
        const {dialog} = await openPalette(user);
        const live = dialog.querySelector("[aria-live=polite]") as HTMLElement;
        await waitFor(() => expect(live).toHaveTextContent(/^\d+ results$/));
        await user.keyboard("scan");
        await waitFor(() => expect(live).toHaveTextContent(String(screen.getAllByRole("option").length) + " results"));
    });

    it("closes with Escape and gives the focus back", async () => {
        const user = await openApp();
        const opener = screen.getByRole("button", {name: "Appearance and language"});
        opener.focus();
        await openPalette(user);
        await user.keyboard("{Escape}");
        expect(screen.queryByRole("dialog", {name: "Command palette"})).not.toBeInTheDocument();
        expect(opener).toHaveFocus();
    });

    it("closes when the dimmed background is clicked", async () => {
        const user = await openApp();
        const {dialog} = await openPalette(user);
        await user.click(dialog.parentElement as HTMLElement);
        expect(screen.queryByRole("dialog", {name: "Command palette"})).not.toBeInTheDocument();
    });

    it("is not opened over another dialog", async () => {
        const user = await openApp();
        await user.click(screen.getByRole("button", {name: "Appearance and language"}));
        await user.keyboard("{Control>}k{/Control}");
        expect(screen.queryByRole("dialog", {name: "Command palette"})).not.toBeInTheDocument();
    });

    it("switches between simple and advanced mode", async () => {
        const user = await openApp();
        await openPalette(user);
        await user.keyboard("advanced{Enter}");
        await waitFor(() => expect(JSON.parse(localStorage.getItem("winforge.settings") ?? "{}").detail).toBe("advanced"));
        await openPalette(user);
        expect(options()).toContain("Switch to simple mode");
    });

    it("opens a dialog the chosen action needs, with focus inside it", async () => {
        const user = await openApp();
        await openPalette(user);
        await user.keyboard("appearance{Enter}");
        const settings = await screen.findByRole("dialog", {name: "Appearance and language"});
        expect(settings).toContainElement(document.activeElement as HTMLElement);
        expect(screen.queryByRole("dialog", {name: "Command palette"})).not.toBeInTheDocument();
    });

    it("finds Spanish pages without the accent, and speaks Spanish", async () => {
        const user = await openApp();
        await user.click(screen.getByRole("button", {name: "Appearance and language"}));
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Español"}));
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Listo"}));
        await user.keyboard("{Control>}k{/Control}");
        const dialog = await screen.findByRole("dialog", {name: "Paleta de comandos"});
        const box = within(dialog).getByRole("combobox");
        expect(box).toHaveAttribute("placeholder", expect.stringContaining("Escribe"));
        await user.keyboard("catalogo");
        expect(options()[0]).toBe("Catálogo");
        expect(within(dialog).getByText("Buscar «catalogo» en el catálogo")).toBeInTheDocument();
    });

    it("does not treat what is typed as a pattern", async () => {
        const user = await openApp();
        const {box} = await openPalette(user);
        fireEvent.change(box, {target: {value: "[(*+?\\"}});
        expect(screen.getAllByRole("option").length).toBeGreaterThan(0); // at least the search entry; no crash
    });
});
