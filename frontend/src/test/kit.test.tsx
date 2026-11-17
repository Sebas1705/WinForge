import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {KitApp, KitCreatePlan, KitCreateResult, KitListing} from "../api";
import App from "../App";
import {groupFingerprint, installable, kitError, kitNeedsReopen, notIncludedText, problemText, unsupportedText} from "../lib/kit";
import {setLang} from "../lib/i18n";
import {api, emit, resetApi} from "./apiMock";
import {makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const FP = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";
const plan = (o: Partial<KitCreatePlan> = {}): KitCreatePlan => ({
    folder: "E:\\kit", apps: [{id: "gimp", name: "GIMP", winget: "GIMP.GIMP"}], notIncluded: [], needBytes: 150 * 1024 * 1024, freeBytes: 8 * 1024 ** 3,
    freeKnown: true, enough: true, existingKit: false, ...o,
});
const created = (o: Partial<KitCreateResult> = {}): KitCreateResult => ({
    added: ["gimp"], failed: [], totalApps: 1, bytes: 90 * 1024 * 1024, cancelled: false, fingerprint: FP, folder: "E:\\kit", ...o,
});
const kapp = (id: string, name: string, o: Partial<KitApp> = {}): KitApp => ({
    id, name, version: "1.0", type: "inno", sizeBytes: 5 * 1024 * 1024, state: "ready", problems: [], ...o,
});
const listing = (apps: KitApp[], o: Partial<KitListing> = {}): KitListing => ({
    folder: "E:\\kit", fingerprint: FP, created: "2026-11-17T10:00:00Z", winforge: "v0.15.0", profile: "Dev", apps, notIncluded: [], unlisted: [], needsAdmin: false, ...o,
});

beforeEach(() => {
    setLang("en");
    localStorage.setItem("winforge.tour", "1");
    resetApi(makeState({admin: true}));
});

type User = ReturnType<typeof userEvent.setup>;

async function openApp(lang?: "es", admin = true): Promise<User> {
    if (lang) localStorage.setItem("winforge.settings", JSON.stringify({language: lang}));
    api.GetState.mockImplementation(async () => makeState({admin}));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(lang === "es" ? /apps del catálogo/ : /catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: lang === "es" ? "Menú principal" : "Main menu"})).getByRole("button", {name: lang === "es" ? /Perfiles/ : /Profiles/}));
    return user;
}

async function openCreate(user: User, es = false) {
    await user.click(await screen.findByRole("button", {name: /Mine/}));
    await user.click(await screen.findByRole("button", {name: es ? /Más opciones/ : /More options/}));
    await user.click(screen.getByRole("menuitem", {name: es ? "Crear kit sin conexión…" : "Create offline kit…"}));
    return screen.findByRole("region", {name: es ? "Crear un kit sin conexión" : "Create an offline kit"});
}

describe("offline kit: creating one from a profile", () => {
    it("picks a folder, shows what would be downloaded and the space, and asks before downloading", async () => {
        api.PickFolder.mockResolvedValue("E:\\kit");
        api.KitPrepare.mockResolvedValue(plan({notIncluded: [{id: "store", name: "Store app", reason: "store"}]}));
        api.KitCreate.mockResolvedValue(created());
        const user = await openApp();
        const panel = within(await openCreate(user));
        expect(panel.getByText("No folder chosen yet.")).toBeInTheDocument();
        expect(panel.getByRole("button", {name: /Create kit/})).toBeDisabled();
        await user.click(panel.getByRole("button", {name: /Choose folder/}));
        expect(await panel.findByText("1 app will be downloaded.")).toBeInTheDocument();
        expect(api.KitPrepare).toHaveBeenCalledWith(["gimp"], "E:\\kit"); // the profile's resolved apps, installed or not
        expect(panel.getByText(/Space needed: about 150 MB.*Free on that drive: 8 GB/)).toBeInTheDocument();
        expect(panel.getByText("E:\\kit", {exact: false})).toBeInTheDocument();
        expect(panel.getByText("Store app", {selector: "b"})).toBeInTheDocument();
        expect(panel.getByText(/Microsoft Store app: winget cannot download/)).toBeInTheDocument();
        // Nothing is downloaded until the question is answered.
        await user.click(panel.getByRole("button", {name: /Create kit/}));
        const dialog = await screen.findByRole("alertdialog");
        expect(dialog).toHaveTextContent(/1 installers into E:\\kit/);
        expect(api.KitCreate).not.toHaveBeenCalled();
        await user.click(within(dialog).getByRole("button", {name: "Cancel"}));
        expect(api.KitCreate).not.toHaveBeenCalled();
        await user.click(panel.getByRole("button", {name: /Create kit/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Create kit"}));
        expect(api.KitCreate).toHaveBeenCalledWith(["gimp"], "E:\\kit", "Mine");
        expect(await panel.findByText("The kit is ready. Apps: 1, 90 MB.")).toBeInTheDocument();
        expect(panel.getByText("01234567 89abcdef 01234567 89abcdef 01234567 89abcdef 01234567 89abcdef")).toBeInTheDocument();
    });

    it("shows progress per app, can be stopped, and keeps what finished", async () => {
        api.PickFolder.mockResolvedValue("E:\\kit");
        api.KitPrepare.mockResolvedValue(plan({apps: [{id: "gimp", name: "GIMP", winget: "G"}, {id: "git", name: "Git", winget: "H"}]}));
        let finish: (r: KitCreateResult) => void = () => {};
        api.KitCreate.mockImplementation(() => new Promise<KitCreateResult>((res) => { finish = res; }));
        const user = await openApp();
        const panel = within(await openCreate(user));
        await user.click(panel.getByRole("button", {name: /Choose folder/}));
        await user.click(await panel.findByRole("button", {name: /Create kit/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Create kit"}));
        expect(await panel.findByText("Starting…")).toBeInTheDocument();
        emit.kitProgress({task: "create", done: 1, total: 2, id: "git", name: "Git", percent: 40});
        expect(await panel.findByText("Downloading Git (2 of 2), 40%")).toBeInTheDocument();
        const bar = panel.getByRole("progressbar");
        expect(bar).toHaveAttribute("value", "70"); // (1 app done + 40%) of 2
        // Other tasks' events are not shown here.
        emit.kitProgress({task: "verify", done: 0, total: 9, name: "Other"});
        expect(panel.queryByText(/Other/)).not.toBeInTheDocument();
        await user.click(panel.getByRole("button", {name: "Stop creating the kit"}));
        expect(api.KitCancel).toHaveBeenCalledTimes(1);
        finish(created({cancelled: true, failed: [{id: "git", name: "Git", reason: "cancelled"}]}));
        expect(await panel.findByText("Stopped. What was already downloaded is kept in the kit.")).toBeInTheDocument();
        expect(panel.getByText(/Creation was stopped before this app/)).toBeInTheDocument();
        expect(panel.getByText("The kit works with the apps that were added.")).toBeInTheDocument();
    });

    it("lists failures per app and still says the kit works", async () => {
        api.PickFolder.mockResolvedValue("E:\\kit");
        api.KitPrepare.mockResolvedValue(plan());
        api.KitCreate.mockResolvedValue(created({
            added: [], totalApps: 1, failed: [
                {id: "a", name: "Alpha", reason: "download", detail: "0x8a150008"}, {id: "b", name: "Beta", reason: "type", detail: "zip"},
                {id: "c", name: "Gamma", reason: "switches"}, {id: "d", name: "Delta", reason: "hash"}, {id: "e", name: "Eps", reason: "space"},
            ],
        }));
        const user = await openApp();
        const panel = within(await openCreate(user));
        await user.click(panel.getByRole("button", {name: /Choose folder/}));
        await user.click(await panel.findByRole("button", {name: /Create kit/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Create kit"}));
        const list = within(await panel.findByRole("list", {name: "Apps that are not in the kit"}));
        expect(list.getByText(/winget could not download it \(0x8a150008\)/)).toBeInTheDocument();
        expect(list.getByText(/Its installer is a zip package, which cannot be installed offline safely/)).toBeInTheDocument();
        expect(list.getByText(/no known silent switches/)).toBeInTheDocument();
        expect(list.getByText(/does not match the checksum/)).toBeInTheDocument();
        expect(list.getByText(/not enough free space on the drive/)).toBeInTheDocument();
        expect(panel.getByText("The kit works with the apps that were added.")).toBeInTheDocument();
    });

    it("refuses a folder that cannot be used, warns about space and notes an existing kit", async () => {
        api.PickFolder.mockResolvedValue("E:\\docs");
        api.KitPrepare.mockResolvedValue(plan({problem: "folder", apps: []}));
        const user = await openApp();
        const panel = within(await openCreate(user));
        await user.click(panel.getByRole("button", {name: /Choose folder/}));
        expect(await panel.findByText(/That folder cannot be used: it must be empty/)).toBeInTheDocument();
        expect(panel.getByRole("button", {name: /Create kit/})).toBeDisabled();

        api.PickFolder.mockResolvedValue("E:\\small");
        api.KitPrepare.mockResolvedValue(plan({enough: false, freeBytes: 20 * 1024 * 1024, existingKit: true}));
        await user.click(panel.getByRole("button", {name: /Change folder/}));
        expect(await panel.findByText(/may not be enough free space/)).toBeInTheDocument();
        expect(panel.getByText(/already holds a kit/)).toBeInTheDocument();
        expect(panel.getByRole("button", {name: /Create kit/})).toBeEnabled(); // a warning, not a ban: the estimate can be wrong

        api.PickFolder.mockResolvedValue("");
        await user.click(panel.getByRole("button", {name: /Change folder/}));
        expect(api.KitPrepare).toHaveBeenCalledTimes(2); // a cancelled dialog changes nothing
    });

    it("says an unreadable free space plainly and shows a refusal in the person's language", async () => {
        api.PickFolder.mockResolvedValue("E:\\kit");
        api.KitPrepare.mockResolvedValue(plan({freeKnown: false}));
        api.KitCreate.mockRejectedValue(new Error("kit:busy"));
        const user = await openApp();
        const panel = within(await openCreate(user));
        await user.click(panel.getByRole("button", {name: /Choose folder/}));
        expect(await panel.findByText(/could not read how much is free/)).toBeInTheDocument();
        await user.click(panel.getByRole("button", {name: /Create kit/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Create kit"}));
        expect((await screen.findAllByText("Another kit task is still running.")).length).toBeGreaterThan(0); // the toast and the Tasks list
    });

    it("speaks Spanish", async () => {
        api.PickFolder.mockResolvedValue("E:\\kit");
        api.KitPrepare.mockResolvedValue(plan({notIncluded: [{id: "z", name: "Zip", reason: "type", detail: "zip"}]}));
        api.KitCreate.mockResolvedValue(created());
        const user = await openApp("es");
        const panel = within(await openCreate(user, true));
        await user.click(panel.getByRole("button", {name: /Elegir carpeta/}));
        expect(await panel.findByText("Se descargará 1 app.")).toBeInTheDocument();
        expect(panel.getByText(/Espacio necesario: unos 150 MB/)).toBeInTheDocument();
        expect(panel.getByText(/paquete zip, que no se puede instalar sin conexión/)).toBeInTheDocument();
        await user.click(panel.getByRole("button", {name: /Crear kit/}));
        const dialog = await screen.findByRole("alertdialog");
        expect(dialog).toHaveTextContent("No se instala nada en este PC");
        await user.click(within(dialog).getByRole("button", {name: "Crear kit"}));
        expect(await panel.findByText(/El kit está listo. Apps: 1, 90 MB/)).toBeInTheDocument();
    });
});

async function openInstall(user: User, es = false) {
    await user.click(await screen.findByRole("button", {name: es ? /Instalar desde un kit sin conexión/ : /Install from an offline kit/}));
    return screen.findByRole("region", {name: es ? "Instalar desde un kit sin conexión" : "Install from an offline kit"});
}

const mixed = (): KitApp[] => [
    kapp("firefox", "Firefox", {version: "140.0", type: "msi"}),
    kapp("git", "Git", {admin: true, version: "2.50.1"}),
    kapp("vlc", "VLC", {installed: true, installedVersion: "3.0.21"}),
    kapp("gimp", "GIMP", {state: "problem", problems: [{kind: "changed", file: "gimp.exe"}, {kind: "extra", file: "version.dll"}]}),
    kapp("zipper", "Zipper", {state: "unsupported", type: "zip", reason: "type", detail: "zip"}),
];

describe("offline kit: installing from one", () => {
    it("verifies the kit, lists every app with its state and lets only verified ones be ticked", async () => {
        api.PickFolder.mockResolvedValue("E:\\kit");
        api.KitOpen.mockResolvedValue(listing(mixed(), {notIncluded: [{id: "store", name: "Store app", reason: "store"}], unlisted: ["Other.App"]}));
        const user = await openApp();
        const panel = within(await openInstall(user));
        await user.click(panel.getByRole("button", {name: /Choose kit folder/}));
        expect(api.KitOpen).toHaveBeenCalledWith("E:\\kit");
        expect(await panel.findByText(/Made on Nov 17, 2026 by WinForge v0\.15\.0 from the profile Dev/)).toBeInTheDocument();
        expect(panel.getByText("01234567 89abcdef 01234567 89abcdef 01234567 89abcdef 01234567 89abcdef")).toBeInTheDocument();
        const list = within(panel.getByRole("list", {name: "Apps in the kit"}));
        expect(panel.getByRole("list", {name: "Apps in the kit"}).children).toHaveLength(5);
        // Only verified and not-yet-installed apps have a checkbox.
        expect(list.getAllByRole("checkbox").map((c) => c.getAttribute("aria-label"))).toEqual(["Select Firefox to install from the kit", "Select Git to install from the kit"]);
        expect(list.getByText("Already installed")).toBeInTheDocument();
        expect(list.getByText("Not safe to run")).toBeInTheDocument();
        expect(list.getByText("Cannot install offline")).toBeInTheDocument();
        expect(list.getAllByText("Verified")).toHaveLength(2);
        expect(list.getByText(/gimp\.exe was changed: its checksum differs/)).toBeInTheDocument();
        expect(list.getByText(/version\.dll is in the folder but not in the kit's list/)).toBeInTheDocument();
        expect(list.getByText(/Its installer is a zip package\. Only msi and exe installers/)).toBeInTheDocument();
        expect(list.getByText(/Version 3\.0\.21|installed: 3\.0\.21/, {exact: false})).toBeInTheDocument();
        expect(within(panel.getByRole("list", {name: "Apps in the kit"}).children[1] as HTMLElement).getByText("Administrator")).toBeInTheDocument();
        expect(panel.getByText("Store app", {selector: "b"})).toBeInTheDocument();
        expect(panel.getByText(/Ignored because kit\.json does not list it: Other\.App/)).toBeInTheDocument();
        // Nothing ran: opening only reads.
        expect(api.KitInstall).not.toHaveBeenCalled();
        expect(api.KitPlan).not.toHaveBeenCalled();
        expect(panel.getByRole("button", {name: /Install selected/})).toBeDisabled();
    });

    it("installs the ticked apps after a question, through the usual run window, and checks the PC afterwards", async () => {
        api.PickFolder.mockResolvedValue("E:\\kit");
        api.KitOpen.mockResolvedValue(listing(mixed()));
        api.KitVerify.mockResolvedValue({installed: [{id: "firefox", name: "Firefox"}], missing: [{id: "git", name: "Git"}]});
        const user = await openApp();
        const panel = within(await openInstall(user));
        await user.click(panel.getByRole("button", {name: /Choose kit folder/}));
        await user.click(await panel.findByRole("checkbox", {name: "Select Firefox to install from the kit"}));
        await user.click(panel.getByRole("checkbox", {name: "Select Git to install from the kit"}));
        expect(panel.getByText("2 of 2 selected")).toBeInTheDocument();
        await user.click(panel.getByRole("button", {name: /Install selected \(2\)/}));
        const dialog = await screen.findByRole("alertdialog");
        expect(dialog).toHaveTextContent(/2 installers from E:\\kit/);
        expect(api.KitPlan).not.toHaveBeenCalled();
        await user.click(within(dialog).getByRole("button", {name: "Install"}));
        await waitFor(() => expect(api.KitInstall).toHaveBeenCalledWith(["firefox", "git"]));
        expect(api.KitPlan).toHaveBeenCalledWith(["firefox", "git"]);
        // The run window is open and running at once (the person already answered).
        const run = await screen.findByRole("dialog");
        expect(run).toHaveTextContent("0 of 2");
        emit.install({step: {kind: "kit", id: "firefox", name: "Firefox"}, status: "start"});
        emit.install({step: {kind: "kit", id: "firefox", name: "Firefox"}, status: "ok", reason: "reboot"});
        emit.install({step: {kind: "kit", id: "git", name: "Git"}, status: "failed", error: "the installer exited with code 740", reason: "admin"});
        emit.done(["git"]);
        await waitFor(() => expect(api.KitVerify).toHaveBeenCalledWith(["firefox", "git"]));
        expect(await screen.findByText(/Installed from the kit: Firefox./)).toBeInTheDocument();
        expect(screen.getByText(/Not detected after the install: Git\./)).toBeInTheDocument();
    });

    it("marks what is installed after a run and unticks it", async () => {
        api.PickFolder.mockResolvedValue("E:\\kit");
        api.KitOpen.mockResolvedValue(listing(mixed()));
        const user = await openApp();
        const panel = within(await openInstall(user));
        await user.click(panel.getByRole("button", {name: /Choose kit folder/}));
        await user.click(await panel.findByRole("checkbox", {name: "Select Firefox to install from the kit"}));
        await user.click(panel.getByRole("button", {name: /Install selected \(1\)/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Install"}));
        await waitFor(() => expect(api.KitInstall).toHaveBeenCalled());
        api.KitVerify.mockResolvedValue({installed: [{id: "firefox", name: "Firefox"}, {id: "vlc", name: "VLC"}], missing: []});
        emit.done([]);
        await waitFor(() => expect(panel.queryByRole("checkbox", {name: "Select Firefox to install from the kit"})).not.toBeInTheDocument());
        expect(panel.getAllByText("Already installed")).toHaveLength(2);
    });

    it("warns when administrator rights are missing, and offers to restart", async () => {
        api.PickFolder.mockResolvedValue("E:\\kit");
        api.KitOpen.mockResolvedValue(listing(mixed(), {needsAdmin: true}));
        const user = await openApp(undefined, false);
        const panel = within(await openInstall(user));
        await user.click(panel.getByRole("button", {name: /Choose kit folder/}));
        await user.click(await panel.findByRole("checkbox", {name: "Select Git to install from the kit"}));
        expect(panel.getByText("1 of the selected apps need administrator rights.")).toBeInTheDocument();
        await user.click(panel.getByRole("button", {name: "Restart as administrator"}));
        expect(api.RestartAsAdmin).toHaveBeenCalled();
        await user.click(panel.getByRole("button", {name: /Install selected/}));
        expect(await screen.findByText(/1 need administrator rights: restart WinForge as administrator first/)).toBeInTheDocument();
    });

    it("shows progress while verifying and can stop it", async () => {
        api.PickFolder.mockResolvedValue("E:\\kit");
        let finish: (l: KitListing) => void = () => {};
        api.KitOpen.mockImplementation(() => new Promise<KitListing>((res) => { finish = res; }));
        const user = await openApp();
        const panel = within(await openInstall(user));
        await user.click(panel.getByRole("button", {name: /Choose kit folder/}));
        expect(await panel.findByText("Reading the kit…")).toBeInTheDocument();
        emit.kitProgress({task: "verify", done: 1, total: 4, id: "git", name: "Git"});
        expect(await panel.findByText("Checking Git (2 of 4)")).toBeInTheDocument();
        expect(panel.getByRole("progressbar")).toHaveAttribute("value", "25");
        emit.kitProgress({task: "create", done: 3, total: 4, name: "Other"});
        expect(panel.queryByText(/Other/)).not.toBeInTheDocument();
        await user.click(panel.getByRole("button", {name: "Stop checking the kit"}));
        expect(api.KitCancel).toHaveBeenCalled();
        finish(listing([kapp("git", "Git")]));
        expect(await panel.findByRole("list", {name: "Apps in the kit"})).toBeInTheDocument();
    });

    it("says why a folder cannot be opened, in the person's language", async () => {
        api.PickFolder.mockResolvedValue("E:\\nothing");
        api.KitOpen.mockRejectedValueOnce(new Error("kit:nokit"));
        const user = await openApp();
        const panel = within(await openInstall(user));
        await user.click(panel.getByRole("button", {name: /Choose kit folder/}));
        expect((await screen.findAllByText("There is no WinForge kit in that folder.")).length).toBeGreaterThan(0); // the toast and the Tasks list
        api.KitOpen.mockRejectedValueOnce(new Error("kit:newer"));
        await user.click(panel.getByRole("button", {name: /Choose kit folder/}));
        expect((await screen.findAllByText(/made by a newer WinForge/)).length).toBeGreaterThan(0);
        api.PickFolder.mockResolvedValue("");
        await user.click(panel.getByRole("button", {name: /Choose kit folder/}));
        expect(api.KitOpen).toHaveBeenCalledTimes(2);
    });

    it("says nothing can be installed when every app is installed, broken or unsupported", async () => {
        api.PickFolder.mockResolvedValue("E:\\kit");
        api.KitOpen.mockResolvedValue(listing(mixed().slice(2)));
        const user = await openApp();
        const panel = within(await openInstall(user));
        await user.click(panel.getByRole("button", {name: /Choose kit folder/}));
        expect(await panel.findByText(/Nothing in this kit can be installed/)).toBeInTheDocument();
        expect(panel.queryAllByRole("checkbox")).toHaveLength(0);
    });

    it("is reachable by keyboard: the panel takes focus and the list is tab-operable", async () => {
        api.PickFolder.mockResolvedValue("E:\\kit");
        api.KitOpen.mockResolvedValue(listing(mixed()));
        const user = await openApp();
        const region = await openInstall(user);
        expect(region).toHaveFocus();
        const panel = within(region);
        await user.click(panel.getByRole("button", {name: /Choose kit folder/}));
        const box = await panel.findByRole("checkbox", {name: "Select Firefox to install from the kit"});
        box.focus();
        await user.keyboard(" ");
        expect(box).toBeChecked();
        await user.keyboard("{Tab}{ }");
        expect(panel.getByRole("checkbox", {name: "Select Git to install from the kit"})).toBeChecked();
        await user.click(panel.getByRole("button", {name: "Clear selection"}));
        expect(box).not.toBeChecked();
        await user.click(panel.getByRole("button", {name: "Select all that can be installed"}));
        expect(panel.getByText("2 of 2 selected")).toBeInTheDocument();
    });

    it("speaks Spanish", async () => {
        api.PickFolder.mockResolvedValue("E:\\kit");
        api.KitOpen.mockResolvedValue(listing(mixed()));
        const user = await openApp("es");
        const panel = within(await openInstall(user, true));
        await user.click(panel.getByRole("button", {name: /Elegir carpeta del kit/}));
        expect(await panel.findByText("No es seguro ejecutarla")).toBeInTheDocument();
        expect(panel.getByText("Ya instalada")).toBeInTheDocument();
        expect(panel.getByText("No se puede instalar sin conexión")).toBeInTheDocument();
        expect(panel.getByText(/se ha modificado: su suma de verificación no coincide/)).toBeInTheDocument();
        await user.click(panel.getByRole("checkbox", {name: "Elegir Firefox para instalar desde el kit"}));
        await user.click(panel.getByRole("button", {name: /Instalar las elegidas \(1\)/}));
        expect(await screen.findByRole("alertdialog")).toHaveTextContent("ejecutará 1 instaladores de E:\\kit");
    });
});

describe("offline kit: helpers and styles", () => {
    it("turns the Go refusals into sentences and recognises the ones that need the kit opened again", () => {
        expect(kitError(new Error("kit:nokit"))).toBe("There is no WinForge kit in that folder.");
        expect(kitError(new Error("kit:notlisted vlc"))).toMatch(/not in the kit you opened/);
        expect(kitError(new Error("disk exploded"))).toBe("disk exploded");
        expect(kitError(new Error("kit:unheardof"))).toBe("kit:unheardof");
        expect(kitNeedsReopen(new Error("kit:notopen"))).toBe(true);
        expect(kitNeedsReopen(new Error("kit:busy"))).toBe(false);
    });

    it("words every problem kind, not-included reason and run reason the Go side can send", () => {
        const kinds = ["invalid", "unknownapp", "missingdir", "link", "notfile", "extra", "missing", "size", "changed", "manifesthash", "manifestmismatch", "manifest", "duplicate", "unreadable"];
        for (const kind of kinds) expect(problemText({kind, file: "a.exe"}), kind).not.toMatch(/^kit\.|^[a-z]+$/);
        const reasons = ["store", "type", "switches", "download", "manifest", "hash", "space", "cancelled", "unknown", "invalidname"];
        for (const reason of reasons) expect(notIncludedText({id: "x", name: "X", reason, detail: "zip"}), reason).not.toBe(reason);
        expect(problemText({kind: "link", file: "a.exe"})).toBe("a.exe is a link, not a plain file or folder.");
        expect(unsupportedText({reason: "switches"})).toMatch(/no known silent switches/);
        expect(installable(kapp("a", "A"))).toBe(true);
        expect(installable(kapp("a", "A", {installed: true}))).toBe(false);
        expect(installable(kapp("a", "A", {state: "problem"}))).toBe(false);
        expect(groupFingerprint("abcdefghijklmnop")).toBe("abcdefgh ijklmnop");
    });

    it("keeps the panels from widening the page: every list can shrink and wrap", () => {
        const css = readFileSync(resolve(__dirname, "..", "kit.css"), "utf8");
        expect(css).toMatch(/\.kitlist\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\)/);
        expect(css).toMatch(/\.kitrow \.body\s*\{[^}]*min-width:\s*0/);
        expect(css).toMatch(/\.kitcard\s*\{[^}]*min-width:\s*0/);
        expect(css).toMatch(/\.kitrow \.name b\s*\{[^}]*overflow-wrap:\s*anywhere/);
        expect(css).toMatch(/\.kitprint\s*\{[^}]*overflow-wrap:\s*anywhere/);
        // Only the repo's colour tokens: no hard-coded colours, so six styles x light/dark keep their contrast.
        expect(css).not.toMatch(/#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(/);
    });
});

describe("offline kit: jobs outlive the page", () => {
    const nav = () => within(screen.getByRole("navigation", {name: "Main menu"}));
    const tasks = () => within(screen.getByRole("heading", {name: "Tasks", level: 2}).closest("section") as HTMLElement);

    /** Leaves for Home and comes back; the page is shown as it was left (an open profile stays open), so the list is reached with the back button when asked. */
    async function leaveAndReturn(user: User, toList = true) {
        await user.click(nav().getByRole("button", {name: /Home/}));
        await waitFor(() => expect(screen.queryByRole("heading", {level: 1, name: /Mine|Profiles/})).not.toBeInTheDocument());
        await user.click(nav().getByRole("button", {name: /Profiles/}));
        if (toList && !screen.queryByRole("button", {name: /Install from an offline kit/})) await user.click(await screen.findByRole("button", {name: /All profiles/}));
        await screen.findByRole("button", {name: /Install from an offline kit/});
    }

    it("keeps creating a kit after leaving Profiles, blocks a second kit job, and lists how it ended", async () => {
        api.PickFolder.mockResolvedValue("E:\kit");
        api.KitPrepare.mockResolvedValue(plan());
        let finish: (r: KitCreateResult) => void = () => {};
        api.KitCreate.mockImplementation(() => new Promise<KitCreateResult>((res) => { finish = res; }));
        const user = await openApp();
        const panel = within(await openCreate(user));
        await user.click(panel.getByRole("button", {name: /Choose folder/}));
        await user.click(await panel.findByRole("button", {name: /Create kit/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Create kit"}));
        await panel.findByText("Starting…");
        await leaveAndReturn(user);
        // Still running, in the Tasks list, with a way to stop it.
        expect(tasks().getByText("Creating an offline kit: Mine")).toBeInTheDocument();
        expect(tasks().getByText("Running")).toBeInTheDocument();
        expect(tasks().getByRole("button", {name: "Stop: Creating an offline kit: Mine"})).toBeInTheDocument();
        emit.kitProgress({task: "create", done: 0, total: 1, name: "GIMP"});
        expect(await tasks().findByText("GIMP (1 of 1)")).toBeInTheDocument();
        // Opening a kit now is refused: the Go side runs one kit task at a time.
        await user.click(screen.getByRole("button", {name: /Install from an offline kit/}));
        const install = within(await screen.findByRole("region", {name: "Install from an offline kit"}));
        expect(install.getByRole("button", {name: /Choose kit folder/})).toBeDisabled();
        expect(api.PickFolder).toHaveBeenCalledTimes(1);
        expect(api.KitOpen).not.toHaveBeenCalled();
        finish(created());
        await waitFor(() => expect(tasks().getAllByText("Done").length).toBeGreaterThan(0));
        expect(tasks().getByText("The kit is ready. Apps: 1, 90 MB.")).toBeInTheDocument();
        expect(api.KitCreate).toHaveBeenCalledTimes(1);
        await waitFor(() => expect(install.getByRole("button", {name: /Choose kit folder/})).toBeEnabled());
    });

    it("lists a failed kit creation with its error", async () => {
        api.PickFolder.mockResolvedValue("E:\kit");
        api.KitPrepare.mockResolvedValue(plan());
        api.KitCreate.mockRejectedValue(new Error("kit:busy"));
        const user = await openApp();
        const panel = within(await openCreate(user));
        await user.click(panel.getByRole("button", {name: /Choose folder/}));
        await user.click(await panel.findByRole("button", {name: /Create kit/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Create kit"}));
        await waitFor(() => expect(tasks().getAllByText("Failed").length).toBeGreaterThan(0));
        expect(tasks().getByText("Another kit task is still running.")).toBeInTheDocument();
    });

    it("keeps checking a kit after leaving Profiles, and lists the result", async () => {
        api.PickFolder.mockResolvedValue("E:\kit");
        let finish: (l: KitListing) => void = () => {};
        api.KitOpen.mockImplementation(() => new Promise<KitListing>((res) => { finish = res; }));
        const user = await openApp();
        const panel = within(await openInstall(user));
        await user.click(panel.getByRole("button", {name: /Choose kit folder/}));
        await panel.findByText("Reading the kit…");
        await leaveAndReturn(user);
        expect(tasks().getByText("Checking the offline kit in E:\kit")).toBeInTheDocument();
        // The panel is still open where it was left, still reading, and fills in when the check ends.
        const again = within(await screen.findByRole("region", {name: "Install from an offline kit"}));
        expect(again.getByRole("button", {name: /Choose kit folder/})).toBeDisabled();
        expect(await again.findByText("Reading the kit…")).toBeInTheDocument();
        finish(listing([kapp("git", "Git"), kapp("gimp", "GIMP")]));
        expect(await again.findByRole("checkbox", {name: "Select Git to install from the kit"})).toBeInTheDocument();
        await waitFor(() => expect(tasks().getAllByText("Done").length).toBeGreaterThan(0));
        expect(tasks().getByText("2 apps checked.")).toBeInTheDocument();
        expect(api.KitOpen).toHaveBeenCalledTimes(1);
    });
});
