import {act, render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {afterEach, beforeEach, describe, expect, it, vi} from "vitest";
import type {FolderSize} from "../api";
import App from "../App";
import {setLang} from "../lib/i18n";
import {MAP_MAX_BLOCKS, SpaceMap, mapHeight} from "../views/storage/SpaceMap";
import {api, resetApi} from "./apiMock";
import {makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const MB = 1024 ** 2;
const GB = 1024 ** 3;
const HOME = "C:\\Users\\me";
const folder = (name: string, bytes: number, files = 10, dir = HOME): FolderSize => ({path: `${dir}\\${name}`, name, sizeBytes: bytes, files});

// A ResizeObserver that reports the width the test chooses, as a browser does when the container is laid out.
let width = 800;
const observers = new Set<{ cb: ResizeObserverCallback }>();
class FakeRO {
    private me: { cb: ResizeObserverCallback };
    constructor(cb: ResizeObserverCallback) { this.me = {cb}; observers.add(this.me); }
    observe() { this.me.cb([{contentRect: {width}} as ResizeObserverEntry], this as unknown as ResizeObserver); }
    unobserve() {}
    disconnect() { observers.delete(this.me); }
}
const resizeTo = (w: number) => act(() => { width = w; for (const o of observers) o.cb([{contentRect: {width: w}} as ResizeObserverEntry], {} as ResizeObserver); });

beforeEach(() => { width = 800; observers.clear(); vi.stubGlobal("ResizeObserver", FakeRO); setLang("en"); });
afterEach(() => { vi.unstubAllGlobals(); setLang("en"); });

const blocks = () => screen.getAllByRole("button").filter((b) => b.classList.contains("tm-block"));
const px = (el: HTMLElement, prop: "left" | "top" | "width" | "height") => parseFloat(el.style[prop]);

function mount(folders: FolderSize[], over: Partial<Parameters<typeof SpaceMap>[0]> = {}) {
    const onOpen = vi.fn(), onReveal = vi.fn();
    const utils = render(<SpaceMap folders={folders} root={HOME} locked={false} onOpen={onOpen} onReveal={onReveal} {...over}/>);
    return {...utils, onOpen, onReveal, user: userEvent.setup()};
}

const sample = () => [folder("Videos", 8 * GB, 3), folder("Documents", 2 * GB), folder("Music", 1 * GB, 1), folder("Tiny", 40 * MB)];

describe("space map: blocks", () => {
    it("draws one real button per folder with a name that carries size and share, and merges the tail into Other", () => {
        mount(sample());
        const names = blocks().map((b) => b.getAttribute("aria-label"));
        expect(names).toContain("Videos 8 GB, 72% of the folders shown");
        expect(names).toContain("Documents 2 GB, 18% of the folders shown");
        expect(names).toContain("Music 1 GB, 9% of the folders shown");
        const other = blocks().find((b) => b.classList.contains("tm-other"))!;
        expect(other).toHaveAttribute("aria-disabled", "true");
        expect(other.getAttribute("aria-label")).toMatch(/^Other \(1\) 40 MB, less than 1% of the folders shown/);
        expect(other).toHaveAttribute("title", "1 smaller folders together: 40 MB");
    });

    it("gives a tooltip with the full path and size, and a group label and summary", () => {
        mount(sample());
        expect(screen.getByRole("button", {name: /^Videos /})).toHaveAttribute("title", `${HOME}\\Videos\n8 GB`);
        const group = screen.getByRole("group", {name: "Space map of me"});
        const desc = document.getElementById(group.getAttribute("aria-describedby")!)!;
        expect(desc).toHaveTextContent("The biggest is Videos, 8 GB, 72% of the folders shown.");
        expect(desc).toHaveTextContent("arrow keys");
        expect(screen.getByText("4 folders, 11 GB in total")).toBeInTheDocument();
    });

    it("tiles the container: every block is inside it, areas follow sizes, and the biggest comes first in the DOM", () => {
        mount(sample());
        const all = blocks();
        const grid = all[0].parentElement as HTMLElement;
        expect(all[0]).toHaveAccessibleName(/^Videos/);
        expect(px(grid, "height")).toBe(mapHeight(800));
        let covered = 0;
        for (const b of all) {
            expect(px(b, "left")).toBeGreaterThanOrEqual(1); expect(px(b, "top")).toBeGreaterThanOrEqual(1);
            expect(px(b, "left") + px(b, "width")).toBeLessThanOrEqual(800 - 1);
            expect(px(b, "top") + px(b, "height")).toBeLessThanOrEqual(px(grid, "height") - 1);
            covered += (px(b, "width") + 2) * (px(b, "height") + 2);
        }
        expect(covered).toBe(800 * mapHeight(800));
        const area = (n: RegExp) => { const b = screen.getByRole("button", {name: n}); return px(b, "width") * px(b, "height"); };
        expect(area(/^Videos/)).toBeGreaterThan(area(/^Documents/) * 3);
    });

    it("shows name and size only where they fit and never lets text spill (ellipsis styles live in the CSS)", () => {
        width = 320;
        const small = Array.from({length: 12}, (_, i) => folder(`s${String(i).padStart(2, "0")}`, 286));
        mount([folder("Big", 6560), ...small]);
        const big = screen.getByRole("button", {name: /^Big /});
        expect(within(big).getByText("Big")).toBeInTheDocument();
        expect(big).toHaveTextContent("6.4 KB");
        const smalls = blocks().filter((b) => /^s\d\d /.test(b.getAttribute("aria-label") ?? ""));
        expect(smalls.length).toBeGreaterThan(0);
        expect(smalls.some((b) => b.querySelector(".tm-name") === null)).toBe(true); // too small for a label, still a button with a name
        for (const b of smalls) expect(b.getAttribute("aria-label")).toMatch(/KB|B,/);
    });

    it("re-lays out when the container is resized, and keeps its aspect sensible", async () => {
        mount(sample());
        const before = blocks().map((b) => px(b, "width"));
        await resizeTo(320);
        const grid = blocks()[0].parentElement as HTMLElement;
        expect(px(grid, "height")).toBe(240); // minimum height
        expect(blocks().map((b) => px(b, "width"))).not.toEqual(before);
        for (const b of blocks()) expect(px(b, "left") + px(b, "width")).toBeLessThanOrEqual(319);
        await resizeTo(1800);
        expect(px(blocks()[0].parentElement as HTMLElement, "height")).toBe(520); // maximum height
    });

    it("computes heights from 320 px up to wide windows", () => {
        expect([0, 320, 500, 800, 1200, 4000].map(mapHeight)).toEqual([240, 240, 300, 480, 520, 520]);
    });

    it("draws nothing before the container has a width", () => {
        width = 0;
        mount(sample());
        expect(screen.queryAllByRole("button").filter((b) => b.classList.contains("tm-block"))).toHaveLength(0);
    });

    it("still draws when the browser has no ResizeObserver", () => {
        vi.unstubAllGlobals();
        // @ts-expect-error simulate a missing API
        delete globalThis.ResizeObserver;
        mount(sample());
        expect(blocks().length).toBeGreaterThan(0);
    });

    it("caps a huge folder at the maximum number of blocks, with one Other block that holds the rest", () => {
        width = 1200;
        const many = Array.from({length: 3000}, (_, i) => folder(`dir${i}`, i < 100 ? GB + i : 1000 + i));
        mount(many);
        expect(blocks().length).toBeLessThanOrEqual(MAP_MAX_BLOCKS);
        expect(blocks().length).toBeGreaterThan(30);
        expect(blocks().filter((b) => b.classList.contains("tm-other"))).toHaveLength(1);
        expect(screen.getByText(/^3000 folders, 100 GB in total/)).toBeInTheDocument();
    });

    it("says so when there is nothing with a size, and ignores bad sizes", () => {
        const {unmount} = mount([]);
        expect(screen.getByRole("status")).toHaveTextContent("No folders with a size were found here.");
        unmount();
        mount([folder("a", 0), folder("b", NaN), folder("c", -4)]);
        expect(screen.getByRole("status")).toHaveTextContent("No folders with a size were found here.");
        expect(screen.queryAllByRole("button").filter((b) => b.classList.contains("tm-block"))).toHaveLength(0);
    });

    it("draws a lone folder across the whole map", () => {
        mount([folder("Only", 5 * GB)]);
        const [b] = blocks();
        expect(px(b, "width")).toBe(798);
        expect(b).toHaveAccessibleName("Only 5 GB, 100% of the folders shown");
    });

    it("tints by size rank with data attributes, not by name", () => {
        mount(sample());
        const tint = (n: RegExp) => screen.getByRole("button", {name: n}).getAttribute("data-tint");
        expect(tint(/^Videos/)).toBe("0");
        expect(Number(tint(/^Music/))).toBeGreaterThan(Number(tint(/^Videos/)));
        expect(blocks().find((b) => b.classList.contains("tm-other"))!.hasAttribute("data-tint")).toBe(false);
    });
});

describe("space map: using it", () => {
    it("goes into a folder on click, but not into Other", async () => {
        const {user, onOpen} = mount(sample());
        await user.click(screen.getByRole("button", {name: /^Documents /}));
        expect(onOpen).toHaveBeenCalledWith(`${HOME}\\Documents`);
        onOpen.mockClear();
        await user.click(blocks().find((b) => b.classList.contains("tm-other"))!);
        expect(onOpen).not.toHaveBeenCalled();
    });

    it("has one Tab stop; arrow keys move in reading order, Home and End jump, Enter and Space open", async () => {
        const {user, onOpen} = mount(sample());
        const all = blocks();
        expect(all.filter((b) => b.tabIndex === 0)).toHaveLength(1);
        expect(all[0].tabIndex).toBe(0);
        await user.tab();
        expect(all[0]).toHaveFocus();
        await user.keyboard("{ArrowRight}");
        expect(all[1]).toHaveFocus();
        expect(all[1].tabIndex).toBe(0);
        expect(all[0].tabIndex).toBe(-1);
        await user.keyboard("{ArrowDown}");
        expect(all[2]).toHaveFocus();
        await user.keyboard("{ArrowLeft}{ArrowUp}");
        expect(all[0]).toHaveFocus();
        await user.keyboard("{ArrowUp}"); // no wrapping past the start
        expect(all[0]).toHaveFocus();
        await user.keyboard("{End}");
        expect(all[all.length - 1]).toHaveFocus();
        await user.keyboard("{Home}");
        expect(all[0]).toHaveFocus();
        await user.keyboard("{Enter}");
        expect(onOpen).toHaveBeenLastCalledWith(`${HOME}\\Videos`);
        await user.keyboard("{ArrowRight}{ }");
        expect(onOpen).toHaveBeenLastCalledWith(`${HOME}\\Documents`);
        // the last stop is not Tab trapped: Tab leaves the map
        await user.tab();
        expect(blocks().some((b) => b === document.activeElement)).toBe(false);
    });

    it("moves in the same order the blocks appear on screen (top to bottom, then left to right)", async () => {
        const {user} = mount(sample());
        const all = blocks();
        const pos = all.map((b) => [px(b, "top"), px(b, "left")]);
        expect([...pos].sort((a, b) => a[0] - b[0] || a[1] - b[1])).toEqual(pos);
        await user.tab();
        for (let i = 1; i < all.length; i++) { await user.keyboard("{ArrowRight}"); expect(all[i]).toHaveFocus(); }
    });

    it("offers Look inside and Show in Explorer for the focused block, and no Recycle Bin action", async () => {
        const {user, onOpen, onReveal} = mount(sample());
        const actions = screen.getByRole("group", {name: "Actions for the selected block"});
        expect(actions).toHaveTextContent("arrow keys"); // hint until something is selected
        await user.tab();
        expect(actions).toHaveTextContent("Selected: Videos, 8 GB");
        await user.click(within(actions).getByRole("button", {name: "Show in Explorer"}));
        expect(onReveal).toHaveBeenCalledWith(`${HOME}\\Videos`);
        await user.click(within(actions).getByRole("button", {name: "Look inside"}));
        expect(onOpen).toHaveBeenCalledWith(`${HOME}\\Videos`);
        expect(actions).toHaveTextContent("Folders cannot be moved to the Recycle Bin from here.");
        expect(screen.queryByRole("button", {name: /Recycle/})).not.toBeInTheDocument();
    });

    it("explains Other when it is focused, without offering actions", async () => {
        const {user} = mount(sample());
        await user.tab();
        await user.keyboard("{End}");
        const actions = screen.getByRole("group", {name: "Actions for the selected block"});
        expect(actions).toHaveTextContent("1 smaller folders together: 40 MB");
        expect(within(actions).queryByRole("button")).not.toBeInTheDocument();
    });

    it("disables the blocks while a scan is running", () => {
        mount(sample(), {locked: true});
        for (const b of blocks().filter((x) => !x.classList.contains("tm-other"))) expect(b).toBeDisabled();
    });

    it("toggles the table with aria-expanded and lists every folder with the same numbers", async () => {
        const {user} = mount(sample());
        const toggle = screen.getByRole("button", {name: "Show as table"});
        expect(toggle).toHaveAttribute("aria-expanded", "false");
        expect(screen.queryByRole("table")).not.toBeInTheDocument();
        await user.click(toggle);
        expect(toggle).toHaveAttribute("aria-expanded", "true");
        expect(toggle).toHaveAccessibleName("Hide table");
        expect(toggle).toHaveAttribute("aria-controls", screen.getByRole("table").closest("div")!.id);
        const table = screen.getByRole("table", {name: "Folders in me, biggest first"});
        const rows = within(table).getAllByRole("row").slice(1).map((r) => Array.from(r.querySelectorAll("th,td")).map((c) => c.textContent));
        expect(rows).toEqual([
            ["Videos", "8 GB", "72%", "3 files"],
            ["Documents", "2 GB", "18%", "10 files"],
            ["Music", "1 GB", "9%", "1 file"],
            ["Tiny", "40 MB", "less than 1%", "10 files"], // the folder merged into Other is still listed
        ]);
        await user.click(toggle);
        expect(screen.queryByRole("table")).not.toBeInTheDocument();
    });

    it("keeps a long table to a screenful and says how many are left out", async () => {
        const many = Array.from({length: 500}, (_, i) => folder(`d${i}`, 100 + i));
        const {user} = mount(many);
        await user.click(screen.getByRole("button", {name: "Show as table"}));
        expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(1 + 200 + 1);
        expect(screen.getByText("…and 300 more, smaller folders")).toBeInTheDocument();
    });

    it("reads in Spanish", async () => {
        setLang("es");
        const {user} = mount(sample());
        expect(screen.getByRole("button", {name: "Videos 8 GB, 72 % de las carpetas mostradas"})).toBeInTheDocument();
        expect(screen.getByRole("group", {name: "Mapa de espacio de me"})).toBeInTheDocument();
        expect(blocks().find((b) => b.classList.contains("tm-other"))!.getAttribute("aria-label")).toMatch(/^Otras \(1\) 40 MB, menos del 1 %/);
        await user.tab();
        expect(screen.getByRole("button", {name: "Ver dentro"})).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Mostrar en el Explorador"})).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Mostrar como tabla"}));
        expect(screen.getByRole("columnheader", {name: "Proporción"})).toBeInTheDocument();
    });
});

describe("space map in Biggest things", () => {
    beforeEach(() => {
        localStorage.setItem("winforge.tour", "1");
        resetApi(makeState({admin: true}));
        api.StorageVolumes.mockResolvedValue([]);
        api.StorageScan.mockResolvedValue([]);
        api.StorageFolders.mockImplementation(async (root: string) =>
            root === HOME ? [folder("Videos", 8 * GB, 3), folder("Documents", 2 * GB), folder("Music", 1 * GB)] : [folder("Holidays", 4 * GB, 3, `${HOME}\\Videos`)]);
        api.StorageBigFiles.mockResolvedValue([]);
    });

    async function open(lang?: "es") {
        if (lang) localStorage.setItem("winforge.settings", JSON.stringify({language: lang}));
        api.GetState.mockImplementation(async () => makeState({admin: true}));
        const user = userEvent.setup();
        render(<App/>);
        await screen.findByText(lang ? /apps del catálogo/ : /catalog apps found/);
        await user.click(within(screen.getByRole("navigation", {name: lang ? "Menú principal" : "Main menu"})).getByRole("button", {name: lang ? /Almacenamiento/ : /Storage/}));
        await screen.findByRole("heading", {name: lang ? "Almacenamiento" : "Storage", level: 1});
        const sec = screen.getByRole("heading", {name: lang ? "Lo más grande" : "Biggest things", level: 2}).closest("section") as HTMLElement;
        return {user, sec};
    }

    it("adds a Map tab that reads the same folder list, drills in like the Folders tab and goes back by the breadcrumb", async () => {
        const {user, sec} = await open();
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Map"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Map"}));
        expect(within(sec).getByRole("button", {name: "Map"})).toHaveAttribute("aria-pressed", "true");
        await user.click(within(sec).getByRole("button", {name: "Find the biggest"}));
        expect(api.StorageFolders).toHaveBeenCalledWith(HOME);
        expect(api.StorageBigFiles).not.toHaveBeenCalled();
        const videos = await within(sec).findByRole("button", {name: /^Videos 8 GB, 73%/});

        await user.click(videos);
        expect(api.StorageFolders).toHaveBeenLastCalledWith(`${HOME}\\Videos`);
        expect(await within(sec).findByRole("button", {name: /^Holidays 4 GB, 100%/})).toBeInTheDocument();
        const crumbs = within(sec).getByRole("navigation", {name: "Folder path"});
        expect(within(crumbs).getByText("Videos")).toHaveAttribute("aria-current", "location");

        // the Folders tab shows the same list for the folder we are in, without reading it again
        await user.click(within(sec).getByRole("button", {name: "Folders"}));
        expect(await within(sec).findByRole("button", {name: "Look inside Holidays"})).toBeInTheDocument();
        expect(api.StorageFolders).toHaveBeenCalledTimes(2);

        await user.click(within(sec).getByRole("button", {name: "Map"}));
        await user.click(within(crumbs).getByRole("button", {name: HOME}));
        expect(await within(sec).findByRole("button", {name: /^Documents 2 GB/})).toBeInTheDocument();
        expect(within(sec).queryByRole("navigation", {name: "Folder path"})).not.toBeInTheDocument();
        expect(api.StorageFolders).toHaveBeenCalledTimes(2); // the root was remembered
    });

    it("shows a folder in Explorer from the action row", async () => {
        const {user, sec} = await open();
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Map"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Map"}));
        await user.click(within(sec).getByRole("button", {name: "Find the biggest"}));
        const block = await within(sec).findByRole("button", {name: /^Music /});
        await act(async () => block.focus());
        await user.click(within(sec).getByRole("button", {name: "Show in Explorer"}));
        expect(api.StorageReveal).toHaveBeenCalledWith(`${HOME}\\Music`);
    });

    it("keeps the Files tab and its Recycle Bin action as they were", async () => {
        api.StorageBigFiles.mockResolvedValue([{path: `${HOME}\\a.iso`, name: "a.iso", sizeBytes: GB, modified: "2024-03-05T12:00:00Z"}]);
        const {user, sec} = await open();
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Find the biggest"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Find the biggest"}));
        expect(await within(sec).findByRole("button", {name: "Move to Recycle Bin: a.iso"})).toBeInTheDocument();
        await user.click(within(sec).getByRole("button", {name: "Map"}));
        expect(within(sec).queryByRole("button", {name: /Recycle/})).not.toBeInTheDocument();
    });

    it("labels the tab in Spanish", async () => {
        const {sec} = await open("es");
        expect(within(sec).getByRole("button", {name: "Mapa"})).toBeInTheDocument();
    });

    it("says so when the folder has no folders", async () => {
        api.StorageFolders.mockResolvedValue([]);
        const {user, sec} = await open();
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Map"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Map"}));
        await user.click(within(sec).getByRole("button", {name: "Find the biggest"}));
        expect(await within(sec).findByText("No folders with a size were found here.")).toBeInTheDocument();
    });
});
