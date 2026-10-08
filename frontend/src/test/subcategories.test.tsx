import {readFileSync, readdirSync} from "node:fs";
import {resolve} from "node:path";
import {render, screen, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import App from "../App";
import {KEYS} from "../lib/i18n";
import {filterApps, subCategories, subCategory} from "../lib/model";
import {api, resetApi} from "./apiMock";
import {app, apps, makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const games = [
    app({id: "dolphin", name: "Dolphin", category: "gaming/emulators"}),
    app({id: "mgba", name: "mGBA", category: "gaming/emulators"}),
    app({id: "vortex", name: "Vortex", category: "gaming/mods"}),
    app({id: "steam", name: "Steam", category: "gaming/launchers"}),
    app({id: "odd", name: "Odd one", category: "gaming"}),
];
const all = [...apps, ...games];

describe("sub-categories", () => {
    it("are the part of the category after the slash", () => {
        expect(subCategory(app({id: "x", category: "gaming/mods"}))).toBe("mods");
        expect(subCategory(app({id: "x", category: "media"}))).toBe("-");
    });
    it("count the apps of one category, biggest first", () => {
        expect(subCategories(all, "gaming")).toEqual([
            {sub: "emulators", count: 2}, {sub: "-", count: 1}, {sub: "launchers", count: 1}, {sub: "mods", count: 1},
        ]);
        expect(subCategories(all, "media")).toEqual([{sub: "-", count: 1}]);
    });
    it("filter a category without touching others", () => {
        const f = {query: "", category: "gaming", installed: "all" as const, openSourceOnly: false};
        expect(filterApps(all, {...f, sub: "emulators"}).map((a) => a.id)).toEqual(["dolphin", "mgba"]);
        expect(filterApps(all, {...f, sub: "-"}).map((a) => a.id)).toEqual(["odd"]);
        expect(filterApps(all, f).length).toBe(5);
        // With no category chosen the sub-category is ignored.
        expect(filterApps(all, {...f, category: "", sub: "mods"}).length).toBe(all.length);
    });
});

describe("the catalog", () => {
    beforeEach(() => {
        localStorage.setItem("winforge.tour", "1");
        resetApi(makeState({apps: all}));
    });

    async function open() {
        const user = userEvent.setup();
        render(<App/>);
        await screen.findByText(/catalog apps found/);
        await user.click(within(screen.getByRole("navigation", {name: "Main menu"})).getByRole("button", {name: /Catalog/}));
        return user;
    }
    const cards = () => screen.getAllByRole("article").map((c) => c.querySelector(".cardbody")?.getAttribute("aria-label"));

    it("shows sub-category chips for a big category and narrows to one", async () => {
        const user = await open();
        expect(screen.queryByRole("tablist", {name: "Gaming"})).not.toBeInTheDocument(); // only when a category is chosen
        await user.click(screen.getByRole("tab", {name: /Gaming/}));
        const subs = screen.getByRole("tablist", {name: "Gaming"});
        expect(within(subs).getByRole("tab", {name: /Emulators/})).toBeInTheDocument();
        await user.click(within(subs).getByRole("tab", {name: /Emulators/}));
        expect(cards().sort()).toEqual(["Dolphin", "mGBA"]);
        await user.click(within(subs).getByRole("tab", {name: "All"}));
        expect(cards().length).toBe(5);
    });

    it("forgets the sub-category when another category is chosen", async () => {
        const user = await open();
        await user.click(screen.getByRole("tab", {name: /Gaming/}));
        await user.click(within(screen.getByRole("tablist", {name: "Gaming"})).getByRole("tab", {name: /Mods/}));
        expect(cards()).toEqual(["Vortex"]);
        await user.click(screen.getByRole("tab", {name: /Media/}));
        expect(cards()).toEqual(["VLC"]);
        await user.click(screen.getByRole("tab", {name: /Gaming/}));
        expect(cards().length).toBe(5);
    });

    it("shows no sub-category row for a category without real sub-categories", async () => {
        const user = await open();
        await user.click(screen.getByRole("tab", {name: /Media/}));
        expect(screen.queryByRole("tablist", {name: "Media"})).not.toBeInTheDocument();
        void api;
    });
});

describe("sub-category names", () => {
    it("exist in both languages for every sub-category the catalog uses", () => {
        const dir = resolve(__dirname, "../../../catalogdata/apps");
        const used = new Set<string>();
        for (const f of readdirSync(dir).filter((x) => x.endsWith(".yml"))) {
            for (const m of readFileSync(resolve(dir, f), "utf8").matchAll(/^ {2}category: ([a-z]+)\/([a-z-]+)$/gm)) used.add(`sub.${m[1]}.${m[2]}`);
        }
        expect(used.size).toBeGreaterThanOrEqual(10);
        const keys = new Set(KEYS as string[]);
        expect([...used].filter((k) => !keys.has(k))).toEqual([]);
    });
});
