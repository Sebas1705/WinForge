import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {StartupItem} from "../api";
import App from "../App";
import {visibleItems} from "../views/Startup";
import {api, resetApi} from "./apiMock";
import {makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const item = (name: string, o: Partial<StartupItem> = {}): StartupItem => ({
    id: `hkcu-run|${name}`, name, source: "hkcu-run", command: `"C:\\Program Files\\${name}\\${name}.exe" --background`, target: `C:\\Program Files\\${name}\\${name}.exe`,
    publisher: "", enabled: true, needsAdmin: false, missing: false, caution: false, ...o,
});
const list = (): StartupItem[] => [
    item("Discord", {publisher: "Discord Inc."}),
    item("OneDrive", {enabled: false, publisher: "Microsoft Corporation", caution: true}),
    item("Steam", {source: "hklm-run", id: "hklm-run|Steam", needsAdmin: true}),
    item("OldTool", {missing: true, enabled: false}),
    item("Notes", {source: "user-folder", id: "user-folder|Notes.lnk", command: "C:\\Users\\me\\Startup\\Notes.lnk", target: ""}),
];
const flip = (items: StartupItem[], id: string, enabled: boolean) => items.map((i) => (i.id === id ? {...i, enabled} : i));

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi(makeState({admin: true}));
    api.StartupList.mockImplementation(async () => list());
});

async function open(o: { admin?: boolean; lang?: "es" } = {}) {
    if (o.lang) localStorage.setItem("winforge.settings", JSON.stringify({language: o.lang}));
    api.GetState.mockImplementation(async () => makeState({admin: o.admin ?? true}));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(o.lang === "es" ? /apps del catálogo/ : /catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: o.lang === "es" ? "Menú principal" : "Main menu"})).getByRole("button", {name: o.lang === "es" ? /Arranque/ : /Startup/}));
    await screen.findByRole("heading", {name: o.lang === "es" ? "Arranque" : "Startup", level: 1});
    return user;
}
const sw = (name: string) => screen.findByRole("checkbox", {name: `Start with Windows: ${name}`}) as Promise<HTMLInputElement>;

describe("startup: the page", () => {
    it("lists every item with its origin, publisher and command, and counts how many start", async () => {
        await open();
        expect((await sw("Discord")).checked).toBe(true);
        expect((await sw("OneDrive")).checked).toBe(false);
        expect(screen.getByText("3 of 5 start with Windows")).toBeInTheDocument(); // Discord, Steam, Notes
        expect(screen.getByText("Publisher: Discord Inc.")).toBeInTheDocument();
        expect(screen.getByText("Startup folder")).toBeInTheDocument();
        expect(screen.getByText("All users", {selector: ".chip"})).toBeInTheDocument();
        expect(screen.getByText("Program not found")).toBeInTheDocument();
        expect(screen.getByText("Windows or security")).toBeInTheDocument();
        expect(screen.getAllByText("Administrator", {selector: ".chip"})).toHaveLength(1);
    });

    it("says when nothing starts, and shows a failed read as a message", async () => {
        api.StartupList.mockResolvedValueOnce([]);
        await open();
        expect(await screen.findByText("Nothing starts with Windows from these places.")).toBeInTheDocument();
        api.StartupList.mockRejectedValueOnce(new Error("registry unreadable"));
        const user = userEvent.setup();
        await user.click(screen.getByRole("button", {name: "Refresh"}));
        expect(await screen.findByText(/registry unreadable/)).toBeInTheDocument();
    });

    it("filters by text and by on/off, and says when nothing matches", async () => {
        const user = await open();
        await sw("Discord");
        await user.type(screen.getByRole("searchbox", {name: "Search"}), "microsoft");
        expect(screen.queryByRole("checkbox", {name: "Start with Windows: Discord"})).toBeNull();
        expect(await sw("OneDrive")).toBeInTheDocument();
        await user.clear(screen.getByRole("searchbox", {name: "Search"}));
        await user.selectOptions(screen.getByRole("combobox", {name: "Show"}), "off");
        expect(screen.queryByRole("checkbox", {name: "Start with Windows: Discord"})).toBeNull();
        expect(screen.getAllByRole("checkbox")).toHaveLength(2); // OneDrive, OldTool
        await user.type(screen.getByRole("searchbox", {name: "Search"}), "zzz");
        expect(screen.getByText("No startup item matches.")).toBeInTheDocument();
    });

    it("matches on name, publisher and command in visibleItems", () => {
        const all = list();
        expect(visibleItems(all, "DISCORD", "all").map((i) => i.name)).toEqual(["Discord"]);
        expect(visibleItems(all, "--background", "on").map((i) => i.name)).toEqual(["Discord", "Steam"]);
        expect(visibleItems(all, "", "off").map((i) => i.name)).toEqual(["OneDrive", "OldTool"]);
    });
});

describe("startup: switching", () => {
    it("switches an item off without asking, sends only its id, shows the refreshed list and announces it", async () => {
        api.StartupSet.mockImplementation(async (id: string, on: boolean) => flip(list(), id, on));
        const user = await open();
        await user.click(await sw("Discord"));
        await waitFor(() => expect(api.StartupSet).toHaveBeenCalledWith("hkcu-run|Discord", false));
        await waitFor(async () => expect((await sw("Discord")).checked).toBe(false));
        expect(screen.getByText("Discord will no longer start with Windows.")).toBeInTheDocument();
        expect(screen.getByText(/2 of 5/)).toBeInTheDocument();
    });

    it("switches an item back on", async () => {
        api.StartupSet.mockImplementation(async (id: string, on: boolean) => flip(list(), id, on));
        const user = await open();
        await user.click(await sw("OldTool"));
        await waitFor(() => expect(api.StartupSet).toHaveBeenCalledWith("hkcu-run|OldTool", true));
        expect(await screen.findByText("OldTool will start with Windows.")).toBeInTheDocument();
    });

    it("asks before switching off something that looks like Windows or security, and does nothing on Cancel", async () => {
        api.StartupList.mockImplementation(async () => [item("SecurityHealth", {caution: true, publisher: "Microsoft Corporation"})]);
        api.StartupSet.mockResolvedValue([item("SecurityHealth", {caution: true, enabled: false})]);
        const user = await open();
        await user.click(await sw("SecurityHealth"));
        const dialog = await screen.findByRole("alertdialog", {name: "Switch off SecurityHealth?"});
        expect(within(dialog).getByText(/may reduce your protection/)).toBeInTheDocument();
        await user.click(within(dialog).getByRole("button", {name: "Cancel"}));
        expect(api.StartupSet).not.toHaveBeenCalled();
        await user.click(await sw("SecurityHealth"));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Switch off"}));
        await waitFor(() => expect(api.StartupSet).toHaveBeenCalledWith("hkcu-run|SecurityHealth", false));
    });

    it("does not ask when switching a Windows item back on", async () => {
        api.StartupList.mockImplementation(async () => [item("OneDrive", {caution: true, enabled: false})]);
        api.StartupSet.mockResolvedValue([item("OneDrive", {caution: true})]);
        const user = await open();
        await user.click(await sw("OneDrive"));
        await waitFor(() => expect(api.StartupSet).toHaveBeenCalledWith("hkcu-run|OneDrive", true));
        expect(screen.queryByRole("alertdialog")).toBeNull();
    });

    it("locks items for all users without administrator rights, and offers to restart", async () => {
        const user = await open({admin: false});
        expect(await sw("Steam")).toBeDisabled();
        expect(await sw("Discord")).toBeEnabled();
        expect(screen.getByText("Items for all users: 1. They can only be changed as administrator.")).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Restart as administrator"}));
        expect(api.RestartAsAdmin).toHaveBeenCalled();
    });

    it("lets an administrator switch items for all users, with no restart offer", async () => {
        api.StartupSet.mockImplementation(async (id: string, on: boolean) => flip(list(), id, on));
        const user = await open({admin: true});
        expect(screen.queryByRole("button", {name: "Restart as administrator"})).toBeNull();
        await user.click(await sw("Steam"));
        await waitFor(() => expect(api.StartupSet).toHaveBeenCalledWith("hklm-run|Steam", false));
    });

    it("explains a refusal for missing administrator rights, in a sentence", async () => {
        api.StartupSet.mockRejectedValue(new Error("needs administrator"));
        const user = await open();
        await user.click(await sw("Discord"));
        expect(await screen.findByText(/needs administrator rights\. Restart WinForge as administrator/i)).toBeInTheDocument();
        expect((await sw("Discord")).checked).toBe(true); // nothing changed
    });

    it("reads the list again when the item has disappeared", async () => {
        api.StartupSet.mockRejectedValue(new Error("that startup item no longer exists"));
        const user = await open();
        await user.click(await sw("Discord"));
        expect(await screen.findByText("That item is no longer there. The list was refreshed.")).toBeInTheDocument();
        await waitFor(() => expect(api.StartupList).toHaveBeenCalledTimes(2));
    });

    it("shows a file in Explorer only for items that start one", async () => {
        const user = await open();
        await sw("Discord");
        expect(screen.queryByRole("button", {name: "Show in folder: Notes"})).toBeNull();
        await user.click(screen.getByRole("button", {name: "Show in folder: Discord"}));
        expect(api.StartupReveal).toHaveBeenCalledWith("hkcu-run|Discord");
    });
});

describe("startup: Spanish", () => {
    it("has its own tab, texts and messages", async () => {
        api.StartupSet.mockImplementation(async (id: string, on: boolean) => flip(list(), id, on));
        const user = await open({lang: "es"});
        expect(await screen.findByRole("checkbox", {name: "Arrancar con Windows: Discord"})).toBeChecked();
        expect(screen.getByText("Carpeta Inicio")).toBeInTheDocument();
        expect(screen.getByText("Programa no encontrado")).toBeInTheDocument();
        await user.click(screen.getByRole("checkbox", {name: "Arrancar con Windows: Discord"}));
        expect(await screen.findByText("Discord ya no arrancará con Windows.")).toBeInTheDocument();
    });
});
