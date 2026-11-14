import {render, screen, waitFor, within} from "@testing-library/react";
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

async function go(user: ReturnType<typeof userEvent.setup>, tab: RegExp) {
    await user.click(within(screen.getByRole("navigation", {name: "Main menu"})).getByRole("button", {name: tab}));
}

// What a screen reader is given, as the accessibility tree exposes it. These do not replace listening with a
// real screen reader; they keep the names, roles and states that one depends on from regressing.
describe("what assistive technology is told", () => {
    it("gives the home page a heading that includes the count the ring only shows as a number", async () => {
        await openApp();
        expect(screen.getByRole("heading", {level: 1})).toHaveAccessibleName(/^\d+ of \d+ catalog apps found$/);
    });

    it("never exposes a bare percentage or fraction as an image", async () => {
        const user = await openApp();
        for (const tab of [/Home/, /Profiles/, /Catalog/, /Updates/, /PC health/]) {
            await go(user, tab);
            for (const img of screen.queryAllByRole("img")) {
                const name = img.getAttribute("aria-label") ?? "";
                if (name) expect(name, `an image on ${tab}`).not.toMatch(/^[\d\s%/.,]+$/);
            }
        }
    });

    it("tells apart the Install buttons of different apps, and the name still contains the visible word", async () => {
        const user = await openApp();
        await go(user, /Catalog/);
        const names = screen.getAllByRole("button", {name: /^Install /}).map((b) => b.getAttribute("aria-label")).filter((n): n is string => n !== null);
        expect(names.length).toBeGreaterThan(1);
        expect(new Set(names).size).toBe(names.length);
        for (const n of names) expect(n).toMatch(/^Install \S/);
    });

    it("labels the search boxes and gives the catalog a heading and its shortcuts", async () => {
        const user = await openApp();
        await go(user, /Catalog/);
        expect(screen.getByRole("heading", {level: 1, name: "Catalog"})).toBeInTheDocument();
        const box = screen.getByRole("textbox", {name: "Search apps"});
        expect(box).toHaveAttribute("aria-keyshortcuts", "/ Control+K");
        await go(user, /Profiles/);
        expect(screen.getByRole("textbox", {name: "Search profiles"})).toBeInTheDocument();
    });

    it("names the status filter by what it does, not by its current value, and says what the card checkbox is for", async () => {
        const user = await openApp();
        await go(user, /Catalog/);
        const status = screen.getByRole("combobox", {name: "Status"});
        expect(status).toHaveDisplayValue("Any state");
        expect(screen.getByRole("checkbox", {name: "Select Firefox"})).toBeInTheDocument();
        expect(screen.queryByRole("checkbox", {name: "Firefox"})).not.toBeInTheDocument();
    });

    it("says what each update checkbox is for, in both languages", async () => {
        api.Upgrades.mockResolvedValue([{id: "git", name: "Git", publisher: "Vendor", current: "1.0", available: "2.0"}]);
        const user = await openApp();
        await go(user, /Updates/);
        expect(await screen.findByRole("checkbox", {name: "Include Git in the update"})).toBeChecked();
        expect(screen.queryByRole("checkbox", {name: "Git"})).not.toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Appearance and language"}));
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Español"}));
        expect(await screen.findByRole("checkbox", {name: "Incluir Git en la actualización"})).toBeInTheDocument();
    });

    it("keeps the help icon out of the page heading's name, but readable on its own", async () => {
        const user = await openApp();
        for (const [tab, name] of [[/Profiles/, "Profiles"], [/PC health/, "PC health"]] as const) {
            await go(user, tab);
            const heading = screen.getByRole("heading", {level: 1});
            expect(heading).toHaveAccessibleName(name);
            const help = within(heading).getByRole("img");
            expect(help.getAttribute("aria-label")).toMatch(/\w{4,}/);
            expect(help).toHaveAttribute("tabindex", "0");
        }
    });

    it("moves between catalog tabs with the arrow keys, as one Tab stop", async () => {
        const user = await openApp();
        await go(user, /Catalog/);
        const tablist = screen.getAllByRole("tablist")[0];
        const tabs = within(tablist).getAllByRole("tab");
        expect(tabs.filter((t) => t.tabIndex === 0)).toHaveLength(1); // roving tabindex
        const selected = () => within(tablist).getByRole("tab", {selected: true});
        selected().focus();
        await user.keyboard("{ArrowRight}");
        await waitFor(() => expect(selected()).toBe(tabs[1]));
        expect(tabs[1]).toHaveFocus();
        await user.keyboard("{End}");
        await waitFor(() => expect(selected()).toBe(tabs[tabs.length - 1]));
        await user.keyboard("{ArrowRight}"); // wraps
        await waitFor(() => expect(selected()).toBe(tabs[0]));
        await user.keyboard("{ArrowLeft}");
        await waitFor(() => expect(selected()).toBe(tabs[tabs.length - 1]));
        await user.keyboard("{Home}");
        await waitFor(() => expect(selected()).toBe(tabs[0]));
    });

    it("spells out counts that were shown as 1/2, which a screen reader may read as a fraction", async () => {
        await openApp();
        const tile = document.querySelector(".quicktile");
        expect(tile).not.toBeNull();
        const count = tile!.querySelector(".mono")!;
        expect(count.querySelector("[aria-hidden=true]")?.textContent).toMatch(/^\d+\/\d+$/);
        expect(count.querySelector(".sr-only")?.textContent).toMatch(/^\d+ of \d+$/);
    });
});
