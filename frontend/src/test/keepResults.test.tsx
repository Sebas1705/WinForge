import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {BackupPreview, BackupResult, CompareResult, KitCreatePlan, KitCreateResult, LeftoversResult, UninstallListing} from "../api";
import App from "../App";
import {api, resetApi} from "./apiMock";
import {healthResult, makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

/**
 * A page's own results (a comparison, the leftovers found, a backup preview, a created kit...) are kept while the
 * app runs: leaving for another page and coming back shows them as they were, without asking the Go side again,
 * and a job that ends while the page is away still lands in them.
 */

function pending<T>() {
    let resolve!: (v: T) => void;
    const promise = new Promise<T>((res) => { resolve = res; });
    return {promise, resolve};
}

type User = ReturnType<typeof userEvent.setup>;
const nav = () => within(screen.getByRole("navigation", {name: "Main menu"}));

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi(makeState({admin: true}));
});

async function openApp(): Promise<User> {
    api.GetState.mockImplementation(async () => makeState({admin: true}));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(/catalog apps found/);
    return user;
}

/** To Home and back to `tab`, waiting until the page really went away. */
async function leaveAndReturn(user: User, tab: RegExp, gone: () => HTMLElement | null) {
    await user.click(nav().getByRole("button", {name: /Home/}));
    await waitFor(() => expect(gone()).not.toBeInTheDocument());
    await user.click(nav().getByRole("button", {name: tab}));
}

const side = (o: Partial<CompareResult["a"]> = {}): CompareResult["a"] => ({
    kind: "pc", name: "", file: "", apps: 0, versioned: true, unknownRecipes: [], unknownProfiles: [], profile: null, ...o,
});
const compared = (): CompareResult => ({
    a: side(),
    b: side({kind: "file", name: "Laptop", file: "laptop.winforge.json", apps: 2, versioned: false, profile: {id: "imported", name: "Laptop", kind: "custom", apps: [{id: "firefox"}, {id: "git"}]}}),
    onlyA: [{id: "gimp", name: "GIMP"}], onlyB: [{id: "firefox", name: "Firefox"}, {id: "git", name: "Git"}], both: [], differ: [], unknown: [],
});

describe("compare keeps its result", () => {
    beforeEach(() => {
        api.PickProfileFile.mockResolvedValue("C:\\Users\\me\\laptop.winforge.json");
        api.CompareFile.mockResolvedValue(compared());
    });

    async function openCompare(user: User) {
        await user.click(nav().getByRole("button", {name: /Profiles/}));
        await user.click(await screen.findByRole("button", {name: "Compare…"}));
    }

    it("still shows the result after going Home and back, with the API called once", async () => {
        const user = await openApp();
        await openCompare(user);
        await user.click(screen.getByRole("button", {name: "Choose a file…"}));
        expect(await screen.findByRole("heading", {name: "Only on this PC (1)"})).toBeInTheDocument();
        await leaveAndReturn(user, /Profiles/, () => screen.queryByRole("heading", {level: 1, name: "Compare setups"}));
        expect(await screen.findByRole("heading", {level: 1, name: "Compare setups"})).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "Only on this PC (1)"})).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "Only in the file (2)"})).toBeInTheDocument();
        expect(screen.getByText("laptop.winforge.json")).toBeInTheDocument();
        expect(api.CompareFile).toHaveBeenCalledTimes(1);
        expect(api.PickProfileFile).toHaveBeenCalledTimes(1);
    });

    it("is dropped by the page's own back button, so the next visit starts clean", async () => {
        const user = await openApp();
        await openCompare(user);
        await user.click(screen.getByRole("button", {name: "Choose a file…"}));
        await screen.findByRole("heading", {name: "Only on this PC (1)"});
        await user.click(screen.getByRole("button", {name: "Back to profiles"}));
        await user.click(await screen.findByRole("button", {name: "Compare…"}));
        expect(screen.queryByRole("heading", {name: "Only on this PC (1)"})).not.toBeInTheDocument();
    });
});

describe("uninstall keeps the leftovers panel", () => {
    const listing = (): UninstallListing => ({apps: [], removed: [{id: "vlc", name: "VLC"}]});
    const found = (): LeftoversResult => ({
        appId: "vlc", appName: "VLC", searched: [{place: "programfiles", path: "C:\\Program Files"}],
        candidates: [
            {path: "C:\\Program Files\\VLC", kind: "folder", place: "programfiles", sizeBytes: 1024, files: 2, modified: "2026-03-05T10:00:00Z", why: {rule: "name", value: "VLC"}},
            {path: "C:\\Users\\me\\Desktop\\VLC.lnk", kind: "shortcut", place: "desktop", sizeBytes: 10, files: 0, modified: "2026-03-05T10:00:00Z", why: {rule: "name", value: "VLC"}},
        ],
    });
    const gone = () => screen.queryByRole("heading", {name: "Uninstall", level: 1});

    async function openUninstall(user: User) {
        await user.click(nav().getByRole("button", {name: /Uninstall/}));
        await user.click(await screen.findByRole("button", {name: "Look for leftovers of VLC"}));
    }

    beforeEach(() => { api.UninstallList.mockImplementation(async () => listing()); });

    it("keeps the panel and the ticked paths after leaving, and still asks before moving anything", async () => {
        api.LeftoversFind.mockResolvedValue(found());
        const user = await openApp();
        await openUninstall(user);
        const first = await screen.findByRole("checkbox", {name: "Move C:\\Program Files\\VLC to the Recycle Bin"});
        await user.click(first);
        await leaveAndReturn(user, /Uninstall/, gone);
        expect(await screen.findByRole("heading", {name: "Leftovers of VLC"})).toBeInTheDocument();
        expect(screen.getByRole("checkbox", {name: "Move C:\\Program Files\\VLC to the Recycle Bin"})).toBeChecked();
        expect(screen.getByRole("checkbox", {name: "Move C:\\Users\\me\\Desktop\\VLC.lnk to the Recycle Bin"})).not.toBeChecked();
        expect(api.LeftoversFind).toHaveBeenCalledTimes(1);
        // The kept selection is not acted on by itself: the question comes first.
        await user.click(screen.getByRole("button", {name: "Move selected to the Recycle Bin"}));
        expect(await screen.findByRole("alertdialog")).toBeInTheDocument();
        expect(api.LeftoversRecycle).not.toHaveBeenCalled();
    });

    it("fills the panel in when the search finishes while the page is away", async () => {
        const find = pending<LeftoversResult>();
        api.LeftoversFind.mockReturnValue(find.promise);
        const user = await openApp();
        await openUninstall(user);
        await screen.findByText(/Looking for leftovers|Searching/);
        await user.click(nav().getByRole("button", {name: /Home/}));
        await waitFor(() => expect(gone()).not.toBeInTheDocument());
        find.resolve(found());
        await waitFor(() => expect(api.LeftoversFind).toHaveBeenCalledTimes(1));
        await new Promise((r) => setTimeout(r, 20));
        await user.click(nav().getByRole("button", {name: /Uninstall/}));
        expect(await screen.findByRole("heading", {name: "Leftovers of VLC"})).toBeInTheDocument();
        expect(await screen.findByRole("checkbox", {name: "Move C:\\Program Files\\VLC to the Recycle Bin"})).not.toBeChecked();
        expect(api.LeftoversFind).toHaveBeenCalledTimes(1);
    });

    it("fills the panel in when the search ends after coming back to the page", async () => {
        const find = pending<LeftoversResult>();
        api.LeftoversFind.mockReturnValue(find.promise);
        const user = await openApp();
        await openUninstall(user);
        await leaveAndReturn(user, /Uninstall/, gone);
        expect(await screen.findByRole("heading", {name: "Leftovers of VLC"})).toBeInTheDocument();
        expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
        find.resolve(found());
        expect(await screen.findByRole("checkbox", {name: "Move C:\\Program Files\\VLC to the Recycle Bin"})).toBeInTheDocument();
        expect(api.LeftoversFind).toHaveBeenCalledTimes(1);
    });

    it("keeps what was ticked in the app list and the search box, without reading the list again", async () => {
        api.UninstallList.mockImplementation(async () => ({apps: [{id: "vlc", name: "VLC", version: "3.0", publisher: "VideoLAN", admin: false, sizeBytes: 0}], removed: []}));
        const user = await openApp();
        await user.click(nav().getByRole("button", {name: /Uninstall/}));
        await user.click(await screen.findByRole("checkbox", {name: /VLC/}));
        await leaveAndReturn(user, /Uninstall/, gone);
        expect(await screen.findByRole("checkbox", {name: /VLC/})).toBeChecked();
        expect(api.UninstallList).toHaveBeenCalledTimes(1);
    });
});

describe("backup keeps its results", () => {
    const gone = () => screen.queryByRole("heading", {name: "Settings backup", level: 1});
    const prev: BackupPreview = {
        host: "PC-VIEJO", app: "v0.6.0", createdAt: "2026-10-01 10:00", intact: true, checked: true, problems: [],
        sets: [{id: "git", files: 1, new: 1, changed: 0, same: 0}], apps: [], missing: 0, unknown: [], profile: null, folders: [],
    };

    async function openBackup() {
        api.BackupSets.mockResolvedValue([{id: "git", files: 1, bytes: 90}]);
        const user = await openApp();
        await user.click(nav().getByRole("button", {name: /Settings backup/}));
        await screen.findByText("Git");
        return user;
    }

    it("keeps the chosen backup's preview and what was ticked, and its verdict", async () => {
        api.PickBackup.mockResolvedValue(prev);
        const user = await openBackup();
        await user.click(screen.getByRole("button", {name: "Rebuild from a backup"}));
        await user.click(screen.getByRole("button", {name: "Choose a backup…"}));
        await screen.findByText(/From PC-VIEJO/);
        await user.click(screen.getByRole("checkbox", {name: "Git"})); // un-tick the pre-ticked set
        await leaveAndReturn(user, /Settings backup/, gone);
        expect(await screen.findByText(/From PC-VIEJO/)).toBeInTheDocument();
        expect(screen.getByText("Checked: nothing was altered or damaged.")).toBeInTheDocument();
        expect(screen.getByRole("checkbox", {name: "Git"})).not.toBeChecked();
        expect(api.PickBackup).toHaveBeenCalledTimes(1);
    });

    it("keeps the outcome of the rebuild", async () => {
        api.PickBackup.mockResolvedValue(prev);
        api.RestoreSettings.mockResolvedValue({restored: 1, unchanged: 0, backedUp: 0, extensions: 0, skipped: []});
        const user = await openBackup();
        await user.click(screen.getByRole("button", {name: "Rebuild from a backup"}));
        await user.click(screen.getByRole("button", {name: "Choose a backup…"}));
        await screen.findByText(/From PC-VIEJO/);
        await user.click(screen.getByRole("button", {name: "Rebuild now"}));
        await waitFor(() => expect(api.RestoreSettings).toHaveBeenCalledTimes(1));
        const notes = await screen.findAllByRole("status");
        expect(notes.some((n) => /1/.test(n.textContent ?? ""))).toBe(true);
        const before = screen.getAllByRole("listitem").filter((li) => li.closest("ul.notes")).map((li) => li.textContent);
        expect(before.length).toBeGreaterThan(0);
        await leaveAndReturn(user, /Settings backup/, gone);
        await screen.findByText(/From PC-VIEJO/);
        expect(screen.getAllByRole("listitem").filter((li) => li.closest("ul.notes")).map((li) => li.textContent)).toEqual(before);
        expect(api.RestoreSettings).toHaveBeenCalledTimes(1);
    });

    it("keeps the verdict of a backup that was made", async () => {
        const made: BackupResult = {path: "D:\\copia.zip", files: 3, bytes: 4096, verified: true, problems: 0, apps: 0, folders: 0};
        api.CreateBackup.mockResolvedValue(made);
        const user = await openBackup();
        await user.click(screen.getByRole("button", {name: "Create backup…"}));
        await waitFor(() => expect(document.querySelector("p.resultline")).not.toBeNull());
        const line = () => document.querySelector("p.resultline") as HTMLElement;
        const text = line().textContent;
        expect(text).toContain("Backup saved to D:\\copia.zip");
        await leaveAndReturn(user, /Settings backup/, gone);
        await screen.findByText("Git");
        expect(line()).toHaveTextContent(text ?? "");
        expect(api.CreateBackup).toHaveBeenCalledTimes(1);
        expect(api.BackupSets).toHaveBeenCalledTimes(1);
    });
});

describe("offline kit keeps the created kit", () => {
    const FP = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";
    const plan: KitCreatePlan = {
        folder: "E:\\kit", apps: [{id: "gimp", name: "GIMP", winget: "GIMP.GIMP"}], notIncluded: [], needBytes: 150 * 1024 * 1024, freeBytes: 8 * 1024 ** 3,
        freeKnown: true, enough: true, existingKit: false,
    };
    const made: KitCreateResult = {added: ["gimp"], failed: [], totalApps: 1, bytes: 90 * 1024 * 1024, cancelled: false, fingerprint: FP, folder: "E:\\kit"};

    it("shows the result and fingerprint again after going Home and back", async () => {
        api.PickFolder.mockResolvedValue("E:\\kit");
        api.KitPrepare.mockResolvedValue(plan);
        api.KitCreate.mockResolvedValue(made);
        const user = await openApp();
        await user.click(nav().getByRole("button", {name: /Profiles/}));
        await user.click(await screen.findByRole("button", {name: /Mine/}));
        await user.click(await screen.findByRole("button", {name: /More options/}));
        await user.click(screen.getByRole("menuitem", {name: "Create offline kit…"}));
        const panel = within(await screen.findByRole("region", {name: "Create an offline kit"}));
        await user.click(panel.getByRole("button", {name: /Choose folder/}));
        await user.click(await panel.findByRole("button", {name: /Create kit/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Create kit"}));
        expect(await panel.findByText("Fingerprint of the kit")).toBeInTheDocument();
        await leaveAndReturn(user, /Profiles/, () => screen.queryByRole("region", {name: "Create an offline kit"}));
        const again = within(await screen.findByRole("region", {name: "Create an offline kit"}));
        expect(again.getByText("Fingerprint of the kit")).toBeInTheDocument();
        expect(again.getByText("The kit is ready. Apps: 1, 90 MB.")).toBeInTheDocument();
        expect(again.getByText("E:\\kit", {exact: false})).toBeInTheDocument();
        expect(api.KitCreate).toHaveBeenCalledTimes(1);
    });
});

describe("health pages keep what they read", () => {
    it("does not read the history again when the page is shown again", async () => {
        api.HealthScan.mockResolvedValue(healthResult());
        const user = await openApp();
        await user.click(nav().getByRole("button", {name: /PC health/}));
        await screen.findByRole("heading", {name: "History", level: 2});
        await waitFor(() => expect(api.HealthHistory).toHaveBeenCalled());
        await screen.findByText(/End of support|eol/i, {}, {timeout: 50}).catch(() => null);
        await new Promise((r) => setTimeout(r, 30));
        const history = api.HealthHistory.mock.calls.length, eol = api.EOLReport.mock.calls.length;
        await leaveAndReturn(user, /PC health/, () => screen.queryByRole("heading", {name: "History", level: 2}));
        await screen.findByRole("heading", {name: "History", level: 2});
        await new Promise((r) => setTimeout(r, 30));
        expect(api.HealthHistory).toHaveBeenCalledTimes(history);
        expect(api.EOLReport).toHaveBeenCalledTimes(eol);
    });
});
