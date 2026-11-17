import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {BackupPreview, BackupResult, LeftoversResult, UninstallListing} from "../api";
import App from "../App";
import {api, emit, resetApi} from "./apiMock";
import {healthResult, makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

/**
 * Jobs started from Uninstall, PC health and Backup are kept in the task store: leaving the page and coming
 * back must still show them running (controls locked, no second call), and then how they ended.
 */

function pending<T>() {
    let resolve!: (v: T) => void;
    let reject!: (e: unknown) => void;
    const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
    return {promise, resolve, reject};
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

/** Goes to another page and back to `tab`, as the owner did. */
async function leaveAndReturn(user: User, tab: RegExp, heading: string) {
    await user.click(nav().getByRole("button", {name: /Home/}));
    await waitFor(() => expect(screen.queryByRole("heading", {name: heading, level: 1})).not.toBeInTheDocument());
    await user.click(nav().getByRole("button", {name: tab}));
    await screen.findByRole("heading", {name: heading, level: 1});
    return screen.getByRole("heading", {name: "Tasks", level: 2}).closest("section") as HTMLElement;
}

describe("uninstall: leftovers job survives leaving the page", () => {
    const listing = (): UninstallListing => ({apps: [], removed: [{id: "vlc", name: "VLC"}]});
    const result = (): LeftoversResult => ({
        appId: "vlc", appName: "VLC", searched: [{place: "programfiles", path: "C:\\Program Files"}],
        candidates: [{path: "C:\\Program Files\\VLC", kind: "folder", place: "programfiles", sizeBytes: 1024, files: 2, modified: "2026-03-05T10:00:00Z", why: {rule: "name", value: "VLC"}}],
    });

    async function start() {
        api.UninstallList.mockImplementation(async () => listing());
        const user = await openApp();
        await user.click(nav().getByRole("button", {name: /Uninstall/}));
        await user.click(await screen.findByRole("button", {name: "Look for leftovers of VLC"}));
        return user;
    }

    it("keeps the search running in the Tasks list, locks the buttons and lists how it ended", async () => {
        const find = pending<LeftoversResult>();
        api.LeftoversFind.mockReturnValue(find.promise);
        const user = await start();
        const tasks = await leaveAndReturn(user, /Uninstall/, "Uninstall");
        expect(within(tasks).getByText("Looking for leftovers of VLC…")).toBeInTheDocument();
        expect(within(tasks).getByText("Running")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Look for leftovers of VLC"})).toBeDisabled();
        expect(api.LeftoversFind).toHaveBeenCalledTimes(1);
        find.resolve(result());
        await waitFor(() => expect(within(tasks).getByText("Done")).toBeInTheDocument());
        expect(within(tasks).getByText("1 found")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Look for leftovers of VLC"})).toBeEnabled();
        expect(api.LeftoversFind).toHaveBeenCalledTimes(1);
    });

    it("lists a failed search with its reason", async () => {
        api.LeftoversFind.mockRejectedValue(new Error("that app is still installed: uninstall it first, then look for what it left"));
        const user = await start();
        const tasks = screen.getByRole("heading", {name: "Tasks", level: 2}).closest("section") as HTMLElement;
        await waitFor(() => expect(within(tasks).getByText("Failed")).toBeInTheDocument());
        expect(within(tasks).getByText("That app is still installed. Uninstall it first, then look for what it left.")).toBeInTheDocument();
        expect(user).toBeTruthy();
    });
});

describe("PC health: scan survives leaving the page", () => {
    it("keeps the scan running with its stage, locks the buttons, starts no second scan and lists the result", async () => {
        const scan = pending<ReturnType<typeof healthResult>>();
        api.HealthScan.mockReturnValue(scan.promise);
        const user = await openApp();
        await user.click(nav().getByRole("button", {name: /PC health/}));
        await screen.findByRole("heading", {name: "PC health", level: 1});
        await emit.scan("health", "drivers");
        const tasks = await leaveAndReturn(user, /PC health/, "PC health");
        expect(within(tasks).getByText("Scanning PC health…")).toBeInTheDocument();
        expect(within(tasks).getByText("Running")).toBeInTheDocument();
        expect(within(tasks).getByText("Listing installed drivers (the slowest part)")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: /Scanning/})).toBeDisabled();
        expect(api.HealthScan).toHaveBeenCalledTimes(1);
        const r = healthResult();
        scan.resolve(r);
        await waitFor(() => expect(within(tasks).getByText("Done")).toBeInTheDocument());
        expect(within(tasks).getByText(`${r.ok} of ${r.total} checks are fine.`)).toBeInTheDocument();
        expect(api.HealthScan).toHaveBeenCalledTimes(1);
    });

    it("lists a failed scan with its error", async () => {
        api.HealthScan.mockRejectedValue(new Error("timed out after 2m0s"));
        const user = await openApp();
        await user.click(nav().getByRole("button", {name: /PC health/}));
        const tasks = (await screen.findByRole("heading", {name: "Tasks", level: 2})).closest("section") as HTMLElement;
        await waitFor(() => expect(within(tasks).getByText("Failed")).toBeInTheDocument());
        expect(within(tasks).getByText("timed out after 2m0s")).toBeInTheDocument();
    });

    it("keeps the update check running after leaving the page", async () => {
        api.HealthScan.mockResolvedValue(healthResult());
        const upd = pending<ReturnType<typeof healthResult>>();
        api.HealthUpdates.mockReturnValue(upd.promise);
        const user = await openApp();
        await user.click(nav().getByRole("button", {name: /PC health/}));
        await user.click(await screen.findByRole("button", {name: "Check Windows Update"}));
        const tasks = await leaveAndReturn(user, /PC health/, "PC health");
        expect(within(tasks).getByText("Checking for Windows and driver updates…")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Asking Windows Update…"})).toBeDisabled();
        expect(api.HealthUpdates).toHaveBeenCalledTimes(1);
        upd.resolve(healthResult());
        await waitFor(() => expect(within(tasks).getAllByText("Done").length).toBeGreaterThan(0));
    });
});

describe("backup: jobs survive leaving the page", () => {
    const made = (): BackupResult => ({path: "C:\\b.zip", files: 5, bytes: 2138, apps: 1, folders: 0, verified: true, problems: 0});

    async function openBackup() {
        api.BackupSets.mockResolvedValue([{id: "git", files: 1, bytes: 90}]);
        const user = await openApp();
        await user.click(nav().getByRole("button", {name: /Settings backup/}));
        await screen.findByRole("heading", {name: "Settings backup", level: 1});
        await screen.findByText("Git");
        return user;
    }

    it("keeps a running backup in the Tasks list, locks Create, calls once and lists the result", async () => {
        const create = pending<BackupResult>();
        api.CreateBackup.mockReturnValue(create.promise);
        const user = await openBackup();
        await user.click(screen.getByRole("button", {name: "Create backup…"}));
        await waitFor(() => expect(api.CreateBackup).toHaveBeenCalledTimes(1));
        const tasks = await leaveAndReturn(user, /Settings backup/, "Settings backup");
        expect(within(tasks).getByText("Creating the backup…")).toBeInTheDocument();
        expect(within(tasks).getByText("Running")).toBeInTheDocument();
        await screen.findByText("Git");
        expect(screen.getByRole("button", {name: "Create backup…"})).toBeDisabled();
        create.resolve(made());
        await waitFor(() => expect(within(tasks).getByText("Done")).toBeInTheDocument());
        expect(within(tasks).getByText(/Backup saved to C:\\b.zip: 5 files, 2 KB\. Read back and checked/)).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Create backup…"})).toBeEnabled();
        expect(api.CreateBackup).toHaveBeenCalledTimes(1);
    });

    it("lists a failed backup with its error", async () => {
        api.CreateBackup.mockRejectedValue(new Error("disk full"));
        const user = await openBackup();
        await user.click(screen.getByRole("button", {name: "Create backup…"}));
        const tasks = screen.getByRole("heading", {name: "Tasks", level: 2}).closest("section") as HTMLElement;
        await waitFor(() => expect(within(tasks).getByText("Failed")).toBeInTheDocument());
        expect(within(tasks).getByText("disk full")).toBeInTheDocument();
    });

    it("keeps a running rebuild in the Tasks list and does not let another backup job start", async () => {
        const restore = pending<{ restored: number; unchanged: number; backedUp: number; extensions: number; skipped: string[] }>();
        api.RestoreSettings.mockReturnValue(restore.promise);
        const prev: BackupPreview = {
            host: "PC-VIEJO", app: "v0.6.0", createdAt: "2026-10-01 10:00", intact: true, checked: true, problems: [],
            sets: [{id: "git", files: 1, new: 1, changed: 0, same: 0}], apps: [], missing: 0, unknown: [], profile: null, folders: [],
        };
        api.PickBackup.mockResolvedValue(prev);
        const user = await openBackup();
        await user.click(screen.getByRole("button", {name: "Rebuild from a backup"}));
        await user.click(screen.getByRole("button", {name: "Choose a backup…"}));
        await screen.findByText(/From PC-VIEJO/);
        await user.click(screen.getByRole("button", {name: "Rebuild now"}));
        await waitFor(() => expect(api.RestoreSettings).toHaveBeenCalledTimes(1));
        const tasks = await leaveAndReturn(user, /Settings backup/, "Settings backup");
        expect(within(tasks).getByText("Rebuilding this PC from a backup…")).toBeInTheDocument();
        expect(within(tasks).getByText("Restoring settings…")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Create backup…"})).toBeDisabled();
        restore.resolve({restored: 1, unchanged: 0, backedUp: 0, extensions: 0, skipped: []});
        await waitFor(() => expect(within(tasks).getByText("Done")).toBeInTheDocument());
        expect(api.RestoreSettings).toHaveBeenCalledTimes(1);
    });
});
