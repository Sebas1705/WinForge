import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {LeftoverCandidate, UninstallApp, UninstallListing} from "../api";
import App from "../App";
import {markedBytes, notInstalledMatches, uninstallError, visibleApps, whyText} from "../lib/uninstall";
import {api, emit, resetApi} from "./apiMock";
import {makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const app = (id: string, name: string, o: Partial<UninstallApp> = {}): UninstallApp => ({id, name, publisher: "Vendor", version: "1.0", ...o});
const apps = (): UninstallApp[] => [
    app("git", "Git", {publisher: "The Git Development Community", version: "2.50.1", admin: true, sizeBytes: 320 * 1024 * 1024}),
    app("vlc", "VLC", {publisher: "VideoLAN", version: "3.0.21"}),
    app("firefox", "Firefox", {publisher: "Mozilla", version: "140.0", sizeBytes: 1536}),
];
const listing = (o: Partial<UninstallListing> = {}): UninstallListing => ({apps: apps(), removed: [], ...o});

const left = (path: string, o: Partial<LeftoverCandidate> = {}): LeftoverCandidate => ({
    path, kind: "folder", place: "programfiles", sizeBytes: 5 * 1024 * 1024, files: 12, modified: "2026-03-05T10:00:00Z",
    why: {rule: "name", value: "VLC"}, ...o,
});
const folders = (): LeftoverCandidate[] => [
    left("C:\\Program Files\\VLC"),
    left("C:\\Users\\me\\AppData\\Roaming\\VLC", {place: "appdata", sizeBytes: 2048, files: 1}),
    left("C:\\Users\\me\\Desktop\\VLC.lnk", {kind: "shortcut", place: "desktop", sizeBytes: 900, files: 1}),
    left("C:\\Users\\me\\AppData\\Local\\Videolan\\VLC", {place: "localappdata", why: {rule: "nested", value: "VideoLAN\\VLC"}}),
];
const found = (cands: LeftoverCandidate[] = folders()) => ({appId: "vlc", appName: "VLC", candidates: cands, searched: [{place: "programfiles", path: "C:\\Program Files"}, {place: "startmenu", path: "C:\\x"}]});

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi(makeState({admin: true}));
    api.UninstallList.mockImplementation(async () => listing());
});

type User = ReturnType<typeof userEvent.setup>;

async function open(o: { admin?: boolean; lang?: "es" } = {}): Promise<User> {
    if (o.lang) localStorage.setItem("winforge.settings", JSON.stringify({language: o.lang}));
    api.GetState.mockImplementation(async () => makeState({admin: o.admin ?? true}));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(o.lang === "es" ? /apps del catálogo/ : /catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: o.lang === "es" ? "Menú principal" : "Main menu"})).getByRole("button", {name: o.lang === "es" ? /Desinstalar/ : /Uninstall/}));
    await screen.findByRole("heading", {name: o.lang === "es" ? "Desinstalar" : "Uninstall", level: 1});
    return user;
}
const box = (name: string) => screen.findByRole("checkbox", {name: `Select ${name} to uninstall`}) as Promise<HTMLInputElement>;
const step = (id: string, name: string) => ({kind: "uninstall" as const, id, name});

describe("uninstall: the list", () => {
    it("lists installed catalog apps with version, publisher, size and the administrator badge, none ticked", async () => {
        await open();
        expect((await box("Git")).checked).toBe(false);
        for (const cb of screen.getAllByRole("checkbox")) expect(cb).not.toBeChecked();
        expect(screen.getByText("Version 2.50.1 · Publisher: The Git Development Community")).toBeInTheDocument();
        expect(screen.getByText("About 320 MB")).toBeInTheDocument();
        expect(screen.getByText("About 1.5 KB")).toBeInTheDocument();
        expect(screen.getByText("Administrator", {selector: ".chip"})).toBeInTheDocument(); // only Git
        expect(screen.getByText("0 of 3 selected")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Uninstall selected"})).toBeDisabled();
    });

    it("says when nothing is installed and when the read failed", async () => {
        api.UninstallList.mockResolvedValueOnce({apps: [], removed: []});
        await open();
        expect(await screen.findByText("No catalog app is installed on this PC.")).toBeInTheDocument();
        api.UninstallList.mockRejectedValueOnce(new Error("scan broke"));
        await userEvent.setup().click(screen.getByRole("button", {name: "Refresh"}));
        expect(await screen.findByText("scan broke")).toBeInTheDocument();
    });

    it("searches by name or publisher and says when nothing matches", async () => {
        const user = await open();
        await box("Git");
        await user.type(screen.getByRole("searchbox", {name: "Search installed apps"}), "mozilla");
        expect(screen.queryByRole("checkbox", {name: "Select Git to uninstall"})).toBeNull();
        expect(await box("Firefox")).toBeInTheDocument();
        await user.clear(screen.getByRole("searchbox", {name: "Search installed apps"}));
        await user.type(screen.getByRole("searchbox", {name: "Search installed apps"}), "zzz");
        expect(screen.getByText("No installed app matches.")).toBeInTheDocument();
    });

    it("can be driven from the keyboard", async () => {
        const user = await open();
        const git = await box("Git");
        git.focus();
        await user.keyboard(" ");
        expect(git.checked).toBe(true);
        expect(screen.getByText("1 of 3 selected")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Uninstall selected (1)"})).toBeEnabled();
        await user.click(screen.getByRole("button", {name: "Clear selection"}));
        expect(git.checked).toBe(false);
    });

    it("keeps only helper logic honest: search, matches, bytes, errors and reasons", () => {
        expect(visibleApps(apps(), "VIDEOLAN").map((a) => a.id)).toEqual(["vlc"]);
        expect(visibleApps(apps(), "  ").length).toBe(3);
        const cat = [{id: "a", name: "Alpha", installed: false}, {id: "b", name: "Alphabet", installed: true}, {id: "c", name: "Alpine", installed: false}];
        expect(notInstalledMatches(cat, new Set(), "al").map((a) => a.id)).toEqual(["a", "c"]);
        expect(notInstalledMatches(cat, new Set(["a"]), "al").map((a) => a.id)).toEqual(["c"]);
        expect(notInstalledMatches(cat, new Set(), "a")).toEqual([]); // one letter would match everything
        expect(markedBytes(folders(), new Set([folders()[0].path, "C:\\nope"]))).toBe(5 * 1024 * 1024);
        expect(uninstallError(new Error("that app is still installed: x")).refresh).toBe(true);
        expect(uninstallError(new Error("that path was not listed by the last search")).refresh).toBe(false);
        expect(uninstallError(new Error("boom")).text).toBe("boom");
        expect(whyText({rule: "name", value: "Git"})).toBe("Its name is the app's name, “Git”.");
    });
});

describe("uninstall: removing apps", () => {
    async function select(user: User, ...names: string[]) {
        for (const n of names) await user.click(await box(n));
    }

    it("asks first, naming each app and saying it cannot be undone and that data may remain", async () => {
        const user = await open();
        await select(user, "VLC", "Firefox");
        await user.click(screen.getByRole("button", {name: "Uninstall selected (2)"}));
        const sure = await screen.findByRole("alertdialog", {name: "Uninstall these 2 apps?"});
        expect(within(sure).getByText("VLC")).toBeInTheDocument();
        expect(within(sure).getByText("Firefox")).toBeInTheDocument();
        expect(sure).toHaveTextContent("WinForge cannot undo this");
        expect(sure).toHaveTextContent("install the app again from the catalog");
        expect(sure).toHaveTextContent("own data (settings, saves, licenses) may stay");
        expect(api.PlanUninstall).not.toHaveBeenCalled();
        expect(within(sure).getByRole("button", {name: "Cancel"})).toHaveFocus(); // the safe answer is the focused one
        await user.click(within(sure).getByRole("button", {name: "Cancel"}));
        expect(api.PlanUninstall).not.toHaveBeenCalled();
        expect(api.UninstallApps).not.toHaveBeenCalled();
        expect((await box("VLC")).checked).toBe(true); // asking again later keeps the choice
    });

    it("plans, runs through the shared runner, verifies by scanning again and offers the leftovers", async () => {
        const user = await open();
        await select(user, "VLC", "Firefox");
        await user.click(screen.getByRole("button", {name: "Uninstall selected (2)"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Uninstall"}));
        await waitFor(() => expect(api.PlanUninstall).toHaveBeenCalledWith(["vlc", "firefox"]));
        const run = await screen.findByRole("dialog", {name: "Uninstall 2 apps"});
        expect(api.UninstallApps).not.toHaveBeenCalled();
        await user.click(within(run).getByRole("button", {name: "Run 2 steps"}));
        expect(api.UninstallApps).toHaveBeenCalledWith(["vlc", "firefox"]);
        // The scan after the run finds VLC gone and Firefox still there.
        api.UninstallVerify.mockResolvedValue({removed: [{id: "vlc", name: "VLC"}], still: [{id: "firefox", name: "Firefox"}]});
        api.UninstallList.mockImplementation(async () => listing({apps: apps().filter((a) => a.id !== "vlc"), removed: [{id: "vlc", name: "VLC"}]}));
        await emit.install({step: step("vlc", "VLC"), status: "start"});
        await emit.install({step: step("vlc", "VLC"), status: "ok"});
        await emit.install({step: step("firefox", "Firefox"), status: "start"});
        await emit.install({step: step("firefox", "Firefox"), status: "ok"});
        await emit.done([]);
        await waitFor(() => expect(api.UninstallVerify).toHaveBeenCalledWith(["vlc", "firefox"]));
        expect(await screen.findByText("Still installed after the uninstall: Firefox. It may have been cancelled or failed.")).toBeInTheDocument();
        await user.click(within(run).getByRole("button", {name: "Close"}));
        expect(await screen.findByRole("heading", {name: "Uninstalled in this session", level: 2})).toBeInTheDocument();
        expect(screen.queryByRole("checkbox", {name: "Select VLC to uninstall"})).toBeNull();
        expect(screen.getByRole("button", {name: "Look for leftovers of VLC"})).toBeInTheDocument();
    });

    it("names a single app in the run title and verifies even if the dialog was closed before the end", async () => {
        const user = await open();
        await select(user, "Git");
        await user.click(screen.getByRole("button", {name: "Uninstall selected (1)"}));
        const sure = await screen.findByRole("alertdialog", {name: "Uninstall this app?"});
        await user.click(within(sure).getByRole("button", {name: "Uninstall"}));
        const run = await screen.findByRole("dialog", {name: "Remove “Git”"});
        await user.click(within(run).getByRole("button", {name: "Run 1 step"}));
        await emit.done([]);
        await waitFor(() => expect(api.UninstallVerify).toHaveBeenCalledWith(["git"]));
    });

    it("never verifies a run that did not start, and shows the reason the start failed", async () => {
        api.UninstallApps.mockRejectedValueOnce(new Error("an installation is already running"));
        const user = await open();
        await select(user, "VLC");
        await user.click(screen.getByRole("button", {name: "Uninstall selected (1)"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Uninstall"}));
        await user.click(await screen.findByRole("button", {name: "Run 1 step"}));
        expect(await screen.findByText("an installation is already running")).toBeInTheDocument();
        await emit.done([]); // some other run ends
        expect(api.UninstallVerify).not.toHaveBeenCalled();
    });

    it("shows the plain reason when a step fails, like any other run", async () => {
        const user = await open();
        await select(user, "Git");
        await user.click(screen.getByRole("button", {name: "Uninstall selected (1)"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Uninstall"}));
        const run = await screen.findByRole("dialog");
        await user.click(within(run).getByRole("button", {name: "Run 1 step"}));
        await emit.install({step: step("git", "Git"), status: "start"});
        await emit.install({step: step("git", "Git"), status: "failed", error: "winget exited with code 0x8a150049", reason: "admin"});
        await emit.done(["git"]);
        expect(await within(run).findByText("Needs administrator rights. Restart WinForge as administrator and continue.")).toBeInTheDocument();
    });

    it("says which apps need administrator rights, and offers to restart as administrator", async () => {
        const user = await open({admin: false});
        await select(user, "Git", "VLC");
        expect(screen.getByText("1 of the selected apps need administrator rights.")).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Restart as administrator"}));
        expect(api.RestartAsAdmin).toHaveBeenCalled();
        await user.click(screen.getByRole("button", {name: "Uninstall selected (2)"}));
        expect(await screen.findByRole("alertdialog")).toHaveTextContent("1 need administrator rights: restart WinForge as administrator first, or they may fail.");
    });

    it("does not nag about administrator rights when running as administrator", async () => {
        const user = await open({admin: true});
        await select(user, "Git");
        expect(screen.queryByText(/need administrator rights/)).toBeNull();
        await user.click(screen.getByRole("button", {name: "Uninstall selected (1)"}));
        expect(await screen.findByRole("alertdialog")).not.toHaveTextContent("administrator rights: restart");
    });
});

describe("uninstall: leftovers", () => {
    const withRemoved = () => api.UninstallList.mockImplementation(async () => listing({apps: apps().filter((a) => a.id !== "vlc"), removed: [{id: "vlc", name: "VLC"}]}));

    async function search(user: User) {
        withRemoved();
        api.LeftoversFind.mockResolvedValue(found());
        await open();
        await user.click(await screen.findByRole("button", {name: "Look for leftovers of VLC"}));
        return screen.findByRole("region", {name: "Leftovers of VLC"});
    }

    it("shows each candidate with its path, size, date and why it matched, with a warning and nothing ticked", async () => {
        const user = userEvent.setup();
        // open() builds its own user; the search helper only needs one for the click
        const panel = await search(user);
        expect(api.LeftoversFind).toHaveBeenCalledWith("vlc");
        expect(within(panel).getByRole("note")).toHaveTextContent("app data can hold settings, saves and licenses");
        const boxes = within(panel).getAllByRole("checkbox");
        expect(boxes).toHaveLength(4);
        for (const b of boxes) expect(b).not.toBeChecked();
        expect(within(panel).getByText("C:\\Program Files\\VLC")).toBeInTheDocument();
        expect(within(panel).getAllByText("Its name is the app's name, “VLC”.")).toHaveLength(3);
        expect(within(panel).getByText("It is the app's folder, “VideoLAN\\VLC”, inside its publisher's folder.")).toBeInTheDocument();
        expect(within(panel).getAllByText("5 MB")).toHaveLength(2);
        expect(within(panel).getByText("900 B")).toBeInTheDocument();
        expect(within(panel).getAllByText("12 files · Last changed Mar 5, 2026").length).toBe(2);
        expect(within(panel).getByText("Shortcut")).toBeInTheDocument();
        expect(within(panel).getByText("Desktop", {selector: ".chip"})).toBeInTheDocument();
        expect(within(panel).getByText(/Searched: Program Files, Start menu\./)).toBeInTheDocument();
        expect(within(panel).getByText("Nothing selected")).toBeInTheDocument();
        expect(within(panel).getByRole("button", {name: "Move selected to the Recycle Bin"})).toBeDisabled();
    });

    it("moves only what was ticked, after asking, and reports each failure", async () => {
        const user = userEvent.setup();
        const panel = await search(user);
        await user.click(within(panel).getByRole("checkbox", {name: "Move C:\\Program Files\\VLC to the Recycle Bin"}));
        await user.click(within(panel).getByRole("checkbox", {name: "Move C:\\Users\\me\\AppData\\Roaming\\VLC to the Recycle Bin"}));
        expect(within(panel).getByText("2 selected, 5 MB")).toBeInTheDocument();
        await user.click(within(panel).getByRole("button", {name: "Move selected to the Recycle Bin"}));
        const sure = await screen.findByRole("alertdialog", {name: "Move these 2 items to the Recycle Bin?"});
        expect(within(sure).getByText("C:\\Program Files\\VLC")).toBeInTheDocument();
        expect(sure).toHaveTextContent("restored from the Recycle Bin");
        expect(api.LeftoversRecycle).not.toHaveBeenCalled();
        api.LeftoversRecycle.mockResolvedValue({moved: 1, freedBytes: 5 * 1024 * 1024, failed: [{path: "C:\\Users\\me\\AppData\\Roaming\\VLC", reason: "notmoved"}]});
        await user.click(within(sure).getByRole("button", {name: "Move to Recycle Bin"}));
        await waitFor(() => expect(api.LeftoversRecycle).toHaveBeenCalledWith(["C:\\Program Files\\VLC", "C:\\Users\\me\\AppData\\Roaming\\VLC"]));
        expect(await within(panel).findByText("Moved 1 item to the Recycle Bin (5 MB).")).toBeInTheDocument();
        expect(within(panel).getByText("Not moved (1); they are still listed:")).toBeInTheDocument();
        expect(within(panel).getByText("C:\\Users\\me\\AppData\\Roaming\\VLC: the Recycle Bin could not take it (in use, or no rights)", {selector: "li"})).toBeInTheDocument();
        // What moved is off the list, what failed is still there and still ticked.
        expect(within(panel).queryByRole("checkbox", {name: "Move C:\\Program Files\\VLC to the Recycle Bin"})).toBeNull();
        expect(within(panel).getByRole("checkbox", {name: "Move C:\\Users\\me\\AppData\\Roaming\\VLC to the Recycle Bin"})).toBeChecked();
        expect(within(panel).getAllByRole("checkbox")).toHaveLength(3);
    });

    it("moves nothing when the person says no", async () => {
        const user = userEvent.setup();
        const panel = await search(user);
        await user.click(within(panel).getByRole("checkbox", {name: "Move C:\\Users\\me\\Desktop\\VLC.lnk to the Recycle Bin"}));
        await user.click(within(panel).getByRole("button", {name: "Move selected to the Recycle Bin"}));
        await user.click(within(await screen.findByRole("alertdialog", {name: "Move this item to the Recycle Bin?"})).getByRole("button", {name: "Cancel"}));
        expect(api.LeftoversRecycle).not.toHaveBeenCalled();
        expect(within(panel).getByRole("checkbox", {name: "Move C:\\Users\\me\\Desktop\\VLC.lnk to the Recycle Bin"})).toBeChecked();
    });

    it("says when nothing was found, and can search again or close", async () => {
        const user = userEvent.setup();
        withRemoved();
        api.LeftoversFind.mockResolvedValue(found([]));
        await open();
        await user.click(await screen.findByRole("button", {name: "Look for leftovers of VLC"}));
        const panel = await screen.findByRole("region", {name: "Leftovers of VLC"});
        expect(await within(panel).findByText("Nothing that matches VLC was found in the places searched.")).toBeInTheDocument();
        await user.click(within(panel).getByRole("button", {name: "Search again"}));
        await waitFor(() => expect(api.LeftoversFind).toHaveBeenCalledTimes(2));
        await user.click(within(panel).getByRole("button", {name: "Close"}));
        expect(screen.queryByRole("region", {name: "Leftovers of VLC"})).toBeNull();
    });

    it("explains a refusal in plain words and refreshes the list when the app is back", async () => {
        const user = userEvent.setup();
        withRemoved();
        api.LeftoversFind.mockRejectedValue(new Error("that app is still installed: uninstall it first, then look for what it left"));
        await open();
        await user.click(await screen.findByRole("button", {name: "Look for leftovers of VLC"}));
        expect((await screen.findAllByText("That app is still installed. Uninstall it first, then look for what it left.")).length).toBeGreaterThan(0); // the toast and the Tasks list
        expect(screen.queryByRole("region", {name: "Leftovers of VLC"})).toBeNull();
        await waitFor(() => expect(api.UninstallList.mock.calls.length).toBeGreaterThan(1));
    });

    it("explains a request the backend refused", async () => {
        const user = userEvent.setup();
        const panel = await search(user);
        api.LeftoversRecycle.mockRejectedValue(new Error('that path was not listed by the last search, so it is not moved: search again: "x"'));
        await user.click(within(panel).getByRole("checkbox", {name: "Move C:\\Program Files\\VLC to the Recycle Bin"}));
        await user.click(within(panel).getByRole("button", {name: "Move selected to the Recycle Bin"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Move to Recycle Bin"}));
        expect((await screen.findAllByText("That item was not in the last search. Search again.")).length).toBeGreaterThan(0); // the toast and the Tasks list
        expect(within(panel).getByRole("checkbox", {name: "Move C:\\Program Files\\VLC to the Recycle Bin"})).toBeEnabled(); // still there to retry
    });

    it("offers a manual search only for catalog apps that are not installed", async () => {
        const user = await open();
        api.LeftoversFind.mockResolvedValue({appId: "gimp", appName: "GIMP", candidates: [], searched: []});
        const field = screen.getByRole("searchbox", {name: "App name"});
        await user.type(field, "g");
        expect(screen.queryByRole("list", {name: "Apps that are not installed"})).toBeNull(); // one letter is not enough
        await user.type(field, "i");
        // Git is installed (it is in the list above), so only GIMP is offered for "gi".
        const results = await screen.findByRole("list", {name: "Apps that are not installed"});
        expect(within(results).getAllByRole("button")).toHaveLength(1);
        await user.click(within(results).getByRole("button", {name: "Look for leftovers of GIMP"}));
        expect(api.LeftoversFind).toHaveBeenCalledWith("gimp");
        await user.clear(field);
        await user.type(field, "vlc"); // installed: never offered
        expect(screen.getByText("No catalog app that is not installed matches.")).toBeInTheDocument();
    });

    it("lets a keyboard user reach the results panel: it takes focus when it opens", async () => {
        const user = userEvent.setup();
        const panel = await search(user);
        await waitFor(() => expect(panel).toHaveFocus());
    });
});

describe("uninstall: Spanish", () => {
    it("shows the page, the question and the leftovers in Spanish", async () => {
        api.UninstallList.mockImplementation(async () => listing({removed: [{id: "vlc", name: "VLC"}]}));
        api.LeftoversFind.mockResolvedValue(found());
        const user = await open({lang: "es"});
        expect(screen.getByText("0 de 3 elegidas")).toBeInTheDocument();
        await user.click(await screen.findByRole("checkbox", {name: "Elegir Git para desinstalar"}));
        await user.click(screen.getByRole("button", {name: "Desinstalar las elegidas (1)"}));
        const sure = await screen.findByRole("alertdialog", {name: "¿Desinstalar esta app?"});
        expect(sure).toHaveTextContent("WinForge no puede deshacerlo");
        await user.click(within(sure).getByRole("button", {name: "Cancelar"}));
        await user.click(screen.getByRole("button", {name: "Buscar restos de VLC"}));
        const panel = await screen.findByRole("region", {name: "Restos de VLC"});
        expect(within(panel).getByRole("note")).toHaveTextContent("los datos de una app pueden contener ajustes, partidas y licencias");
        expect(within(panel).getAllByText("Carpeta", {selector: ".chip"})).toHaveLength(3);
        expect(within(panel).getByText("Archivos de programa", {selector: ".chip"})).toBeInTheDocument();
        expect(within(panel).getAllByText("Su nombre es el de la app, «VLC».").length).toBe(3);
        expect(within(panel).getByRole("button", {name: "Mover lo elegido a la Papelera"})).toBeDisabled();
        expect(within(panel).getByText("No hay nada elegido")).toBeInTheDocument();
        expect(within(panel).getAllByText("12 archivos · Último cambio: 5 mar 2026").length).toBe(2);
    });
});
