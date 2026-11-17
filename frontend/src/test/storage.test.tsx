import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {BigFile, CleanCategory, DupGroup, FolderSize, StorageVolume} from "../api";
import App from "../App";
import {api, emit, resetApi} from "./apiMock";
import {makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const MB = 1024 ** 2;
const GB = 1024 ** 3;
const HOME = "C:\\Users\\me";

const cat = (id: string, bytes: number, o: Partial<CleanCategory> = {}): CleanCategory => ({id, risk: "safe", needsAdmin: false, available: true, bytes, files: 10, skipped: 0, ...o});
// Safe and ticked: 1 GB + 512 MB + 256 MB + 256 MB = 2 GB. Review (not ticked): 12 GB + 512 MB.
const cats = (): CleanCategory[] => [
    cat("temp-user", GB, {files: 1200}),
    cat("thumbnails", 512 * MB),
    cat("recycle-bin", 256 * MB),
    cat("temp-windows", 256 * MB, {needsAdmin: true}),
    cat("windows-old", 12 * GB, {risk: "review", needsAdmin: true, files: 90000}),
    cat("shader-cache", 512 * MB, {risk: "review"}),
    cat("cache-chrome", 400 * MB, {available: false}),
    cat("dev-npm", 0),
];

const volumes: StorageVolume[] = [
    {letter: "C:", label: "Windows", totalBytes: 100 * GB, freeBytes: 5 * GB},
    {letter: "D:", label: "", totalBytes: 1024 * GB, freeBytes: 512 * GB},
];

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi(makeState({admin: true}));
    api.StorageVolumes.mockResolvedValue(structuredClone(volumes));
    api.StorageScan.mockImplementation(async () => cats());
});

async function open(opts: { admin?: boolean; lang?: "es" } = {}) {
    if (opts.lang) localStorage.setItem("winforge.settings", JSON.stringify({language: opts.lang}));
    api.GetState.mockImplementation(async () => makeState({admin: opts.admin ?? true}));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(opts.lang === "es" ? /apps del catálogo/ : /catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: opts.lang === "es" ? "Menú principal" : "Main menu"})).getByRole("button", {name: opts.lang === "es" ? /Almacenamiento/ : /Storage/}));
    await screen.findByRole("heading", {name: opts.lang === "es" ? "Almacenamiento" : "Storage", level: 1});
    return user;
}

// The Tasks list repeats what a section reports, and sits above it: the section is the last match.
const lastText = async (x: string | RegExp) => (await screen.findAllByText(x)).at(-1) as HTMLElement;
const section = (name: string) => screen.getByRole("heading", {name, level: 2}).closest("section") as HTMLElement;
const checkbox = (name: string) => screen.getByRole("checkbox", {name}) as HTMLInputElement;

async function scanJunk(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByRole("button", {name: /Scan for junk/}));
    await screen.findByRole("checkbox", {name: "Select for cleaning: Recycle Bin"});
}

describe("storage: navigation", () => {
    it("adds a Storage tab after PC health, with its page", async () => {
        await open();
        const labels = within(screen.getByRole("navigation", {name: "Main menu"})).getAllByRole("button").map((b) => b.textContent ?? "");
        const at = (s: string) => labels.findIndex((l) => l.includes(s));
        expect(at("PC health")).toBeGreaterThanOrEqual(0);
        expect(at("Storage")).toBe(at("PC health") + 1);
        expect(screen.getByRole("heading", {name: "Free up space", level: 2})).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "Biggest things", level: 2})).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "Duplicate files", level: 2})).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "Optimize", level: 2})).toBeInTheDocument();
    });
});

describe("storage: drives", () => {
    it("draws one bar per drive with the share used", async () => {
        await open();
        const c = await screen.findByRole("meter", {name: "C: Windows, 95% used"});
        expect(c).toHaveAttribute("aria-valuenow", "95");
        expect(screen.getByRole("meter", {name: "D:, 50% used"})).toHaveAttribute("aria-valuenow", "50");
        const drives = section("Your drives");
        expect(within(drives).getByText("95 GB used of 100 GB · 5 GB free")).toBeInTheDocument();
        expect(within(drives).getByText("512 GB used of 1 TB · 512 GB free")).toBeInTheDocument();
        expect(within(drives).getByText("5% free")).toBeInTheDocument();
    });

    it("warns about a drive with less than 10% free, and only that one", async () => {
        await open();
        await screen.findByRole("meter", {name: /C: Windows/});
        const warnings = screen.getAllByText(/Low space/);
        expect(warnings).toHaveLength(1);
        expect(warnings[0]).toHaveTextContent("only 5 GB free");
        expect(warnings[0].closest("li")).toHaveClass("low");
    });

    it("does not warn at exactly 10% free, warns just under it", async () => {
        api.StorageVolumes.mockResolvedValue([
            {letter: "E:", label: "", totalBytes: 100 * GB, freeBytes: 10 * GB},
            {letter: "F:", label: "", totalBytes: 100 * GB, freeBytes: 9 * GB},
        ]);
        await open();
        await screen.findByRole("meter", {name: /E:/});
        expect(screen.getAllByText(/Low space/)).toHaveLength(1);
        expect(screen.getByText(/Low space/).closest("li")).toHaveTextContent("F:");
    });

    it("copes with a drive that has no size, and with no drives at all", async () => {
        api.StorageVolumes.mockResolvedValue([{letter: "X:", label: "Card", totalBytes: 0, freeBytes: 0}]);
        await open();
        expect(await screen.findByText("Size not available")).toBeInTheDocument();
        expect(screen.queryByText(/Low space/)).not.toBeInTheDocument();
        expect(screen.getByRole("meter", {name: "X: Card, 0% used"})).toBeInTheDocument();
    });

    it("says so when there are no drives, and reports a failed read", async () => {
        api.StorageVolumes.mockRejectedValue(new Error("volumes broke"));
        await open();
        expect(await screen.findByText("No drives were found.")).toBeInTheDocument();
        expect(screen.getByText("volumes broke")).toBeInTheDocument();
    });

    it("measures the drives again on Refresh", async () => {
        const user = await open();
        await screen.findByRole("meter", {name: /C: Windows/});
        const before = api.StorageVolumes.mock.calls.length;
        await user.click(screen.getByRole("button", {name: /Refresh/}));
        await waitFor(() => expect(api.StorageVolumes.mock.calls.length).toBe(before + 1));
    });
});

describe("storage: free up space", () => {
    it("lists only the categories that exist here and hold something, with a hint each", async () => {
        const user = await open();
        expect(api.StorageScan).not.toHaveBeenCalled(); // never scans on its own
        await scanJunk(user);
        const clean = section("Free up space");
        for (const name of ["Your temporary files", "Picture preview cache", "Recycle Bin", "Windows temporary files", "Previous Windows version", "Graphics shader caches"]) {
            expect(within(clean).getByText(name)).toBeInTheDocument();
        }
        expect(within(clean).queryByText("Chrome cache")).not.toBeInTheDocument(); // not available
        expect(within(clean).queryByText("npm cache")).not.toBeInTheDocument(); // empty
        expect(within(clean).getByText(/Scratch files your apps leave in your user folder/)).toBeInTheDocument();
        const row = checkbox("Select for cleaning: Your temporary files").closest("label") as HTMLElement;
        expect(row).toHaveTextContent("1 GB");
        expect(row).toHaveTextContent("1200 files");
        expect(within(clean).getByText("Found 14.5 GB that can be cleaned.")).toBeInTheDocument();
    });

    it("badges what needs administrator rights and what to review first", async () => {
        const user = await open();
        await scanJunk(user);
        const rowOf = (name: string) => checkbox(`Select for cleaning: ${name}`).closest("label") as HTMLElement;
        expect(within(rowOf("Windows temporary files")).getByText("Needs admin")).toBeInTheDocument();
        expect(within(rowOf("Windows temporary files")).queryByText("Review first")).not.toBeInTheDocument();
        expect(within(rowOf("Previous Windows version")).getByText("Needs admin")).toBeInTheDocument();
        expect(within(rowOf("Previous Windows version")).getByText("Review first")).toBeInTheDocument();
        expect(within(rowOf("Graphics shader caches")).getByText("Review first")).toBeInTheDocument();
        expect(within(rowOf("Graphics shader caches")).queryByText("Needs admin")).not.toBeInTheDocument();
        expect(within(rowOf("Recycle Bin")).queryByText("Needs admin")).not.toBeInTheDocument();
    });

    it("ticks the safe categories after a scan and leaves the review ones alone", async () => {
        const user = await open();
        await scanJunk(user);
        for (const n of ["Your temporary files", "Picture preview cache", "Recycle Bin", "Windows temporary files"]) expect(checkbox(`Select for cleaning: ${n}`).checked, n).toBe(true);
        for (const n of ["Previous Windows version", "Graphics shader caches"]) expect(checkbox(`Select for cleaning: ${n}`).checked, n).toBe(false);
        expect(screen.getByText("4 selected · 2 GB")).toBeInTheDocument();
    });

    it("updates the total as boxes are ticked and unticked", async () => {
        const user = await open();
        await scanJunk(user);
        await user.click(checkbox("Select for cleaning: Previous Windows version"));
        expect(screen.getByText("5 selected · 14 GB")).toBeInTheDocument();
        await user.click(checkbox("Select for cleaning: Your temporary files"));
        expect(screen.getByText("4 selected · 13 GB")).toBeInTheDocument();
        for (const n of ["Picture preview cache", "Recycle Bin", "Windows temporary files", "Previous Windows version"]) await user.click(checkbox(`Select for cleaning: ${n}`));
        expect(screen.getByText("Nothing selected")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: /Clean selected/})).toBeDisabled();
    });

    it("says when there is nothing to clean", async () => {
        api.StorageScan.mockResolvedValue([cat("dev-npm", 0), cat("cache-chrome", 5, {available: false})]);
        const user = await open();
        await user.click(screen.getByRole("button", {name: /Scan for junk/}));
        expect(await screen.findByText("Nothing to clean: no junk files were found.")).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: /Clean selected/})).not.toBeInTheDocument();
        expect(screen.getByRole("button", {name: /Scan again/})).toBeInTheDocument();
    });

    it("shows a category the build does not know yet by its id, without crashing", async () => {
        api.StorageScan.mockResolvedValue([cat("future-thing", 5 * MB)]);
        const user = await open();
        await user.click(screen.getByRole("button", {name: /Scan for junk/}));
        expect(await screen.findByRole("checkbox", {name: "Select for cleaning: future-thing"})).toBeChecked();
    });

    it("tells a person who is not an administrator which ticked items will be skipped, with the restart action", async () => {
        const user = await open({admin: false});
        await scanJunk(user);
        const hint = screen.getByText(/Administrator rights are needed for: Windows temporary files\./);
        expect(hint).not.toHaveTextContent("Previous Windows version"); // not ticked
        await user.click(checkbox("Select for cleaning: Previous Windows version"));
        expect(screen.getByText(/needed for: Windows temporary files, Previous Windows version\./)).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Restart as administrator"}));
        expect(api.RestartAsAdmin).toHaveBeenCalled();
        // cleaning is still possible: it runs for the rest
        expect(screen.getByRole("button", {name: /Clean selected/})).toBeEnabled();
    });

    it("shows no administrator hint once those items are unticked", async () => {
        const user = await open({admin: false});
        await scanJunk(user);
        expect(screen.getByText(/Administrator rights are needed/)).toBeInTheDocument();
        await user.click(checkbox("Select for cleaning: Windows temporary files"));
        expect(screen.queryByText(/Administrator rights are needed/)).not.toBeInTheDocument();
    });

    it("shows no administrator hint to an administrator", async () => {
        const user = await open({admin: true});
        await scanJunk(user);
        expect(checkbox("Select for cleaning: Windows temporary files")).toBeChecked();
        expect(screen.queryByText(/Administrator rights are needed/)).not.toBeInTheDocument();
    });

    it("asks before cleaning, lists what will go, and does nothing on Cancel", async () => {
        const user = await open();
        await scanJunk(user);
        await user.click(screen.getByRole("button", {name: /Clean selected/}));
        const dialog = await screen.findByRole("alertdialog", {name: "Clean the selected items?"});
        expect(dialog).toHaveTextContent("cannot be undone");
        expect(dialog).toHaveTextContent("Files that are in use are skipped");
        expect(dialog).toHaveTextContent("The Recycle Bin is emptied for good");
        const items = within(dialog).getAllByRole("listitem").map((li) => li.textContent);
        expect(items).toEqual(["Your temporary files (1 GB)", "Picture preview cache (512 MB)", "Recycle Bin (256 MB)", "Windows temporary files (256 MB)"]);
        await user.click(within(dialog).getByRole("button", {name: "Cancel"}));
        expect(api.StorageClean).not.toHaveBeenCalled();
        expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
        expect(api.StorageScan).toHaveBeenCalledTimes(1);
    });

    it("does not mention emptying the Recycle Bin when it is not ticked", async () => {
        const user = await open();
        await scanJunk(user);
        await user.click(checkbox("Select for cleaning: Recycle Bin"));
        await user.click(screen.getByRole("button", {name: /Clean selected/}));
        const dialog = await screen.findByRole("alertdialog");
        expect(dialog).not.toHaveTextContent("Recycle Bin is emptied");
        expect(within(dialog).queryByText(/Recycle Bin \(/)).not.toBeInTheDocument();
    });

    it("cleans exactly the ticked ids, shows what was freed, then scans again and measures the drives again", async () => {
        api.StorageClean.mockResolvedValue({freedBytes: 1.5 * GB, files: 120, failed: 0, outcomes: []});
        const user = await open();
        await scanJunk(user);
        await user.click(checkbox("Select for cleaning: Windows temporary files")); // untick one
        await user.click(checkbox("Select for cleaning: Graphics shader caches")); // tick a review one
        await screen.findByRole("meter", {name: /C: Windows/});
        const volumeReads = api.StorageVolumes.mock.calls.length;
        api.StorageScan.mockImplementation(async () => [cat("thumbnails", 100 * MB)]);

        await user.click(screen.getByRole("button", {name: /Clean selected/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Clean"}));

        expect(api.StorageClean).toHaveBeenCalledTimes(1);
        expect(api.StorageClean).toHaveBeenCalledWith(["temp-user", "thumbnails", "recycle-bin", "shader-cache"]);
        expect(await lastText("Freed 1.5 GB (120 files).")).toBeInTheDocument();
        await waitFor(() => expect(api.StorageScan).toHaveBeenCalledTimes(2));
        expect(api.StorageVolumes.mock.calls.length).toBeGreaterThan(volumeReads);
        // the result stays while the list now shows what is left
        await waitFor(() => expect(screen.queryByText("Your temporary files")).not.toBeInTheDocument());
        expect(screen.getAllByText("Freed 1.5 GB (120 files).").length).toBeGreaterThan(0);
        expect(screen.getByRole("checkbox", {name: "Select for cleaning: Picture preview cache"})).toBeChecked();
    });

    it("keeps keyboard focus in the section after cleaning, instead of dropping it to the page", async () => {
        api.StorageClean.mockResolvedValue({freedBytes: 1.5 * GB, files: 120, failed: 0, outcomes: []});
        const user = await open();
        await scanJunk(user);
        await user.click(screen.getByRole("button", {name: /Clean selected/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Clean"}));
        const status = (await lastText("Freed 1.5 GB (120 files).")).closest("[role=status]") as HTMLElement;
        await waitFor(() => expect(document.activeElement).toBe(status));
    });

    it("moves focus to the section heading when cleaning fails and there is no result", async () => {
        api.StorageClean.mockRejectedValue(new Error("boom"));
        const user = await open();
        await scanJunk(user);
        await user.click(screen.getByRole("button", {name: /Clean selected/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Clean"}));
        await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("heading", {name: "Free up space"})));
    });

    it("explains failures: files in use are skipped, and each category error is named", async () => {
        api.StorageClean.mockResolvedValue({
            freedBytes: 10 * MB, files: 3, failed: 7,
            outcomes: [{id: "temp-user", freedBytes: 10 * MB, files: 3, failed: 5}, {id: "windows-update", freedBytes: 0, files: 0, failed: 2, error: "access denied"}],
        });
        const user = await open();
        await scanJunk(user);
        await user.click(screen.getByRole("button", {name: /Clean selected/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Clean"}));
        expect(await lastText("Freed 10 MB (3 files).")).toBeInTheDocument();
        expect(screen.getByText(/7 items could not be removed\. Files that are in use are skipped/)).toBeInTheDocument();
        expect(screen.getByText("Downloaded update files: access denied")).toBeInTheDocument();
    });

    it("turns the backend's short codes into plain sentences", async () => {
        api.StorageClean.mockResolvedValue({
            freedBytes: 0, files: 0, failed: 0,
            outcomes: [{id: "windows-update", freedBytes: 0, files: 0, failed: 0, error: "needs-admin"}, {id: "dev-npm", freedBytes: 0, files: 0, failed: 0, error: "unknown category"}],
        });
        const user = await open();
        await scanJunk(user);
        await user.click(screen.getByRole("button", {name: /Clean selected/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Clean"}));
        const item = await screen.findByText((c) => c.startsWith("Downloaded update files: needs administrator rights, so it was not touched."));
        expect(item).toBeInTheDocument();
        expect(screen.getByText((c) => c.endsWith("is not a known category, so it was ignored."))).toBeInTheDocument();
        expect(screen.queryByText((c) => c.includes("needs-admin"))).not.toBeInTheDocument();
    });

    it("says nothing was freed rather than 0 B", async () => {
        const user = await open();
        await scanJunk(user);
        await user.click(screen.getByRole("button", {name: /Clean selected/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Clean"}));
        expect(await lastText("Nothing was freed.")).toBeInTheDocument();
    });

    it("shows a call that fails as a message and keeps the list", async () => {
        api.StorageClean.mockRejectedValue(new Error("clean exploded"));
        const user = await open();
        await scanJunk(user);
        await user.click(screen.getByRole("button", {name: /Clean selected/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Clean"}));
        expect(await lastText("clean exploded")).toBeInTheDocument();
        expect(screen.getByRole("checkbox", {name: "Select for cleaning: Recycle Bin"})).toBeInTheDocument();
        expect(api.StorageScan).toHaveBeenCalledTimes(1); // no rescan after a failure
    });

    it("shows a failed scan as a message", async () => {
        api.StorageScan.mockRejectedValue(new Error("scan exploded"));
        const user = await open();
        await user.click(screen.getByRole("button", {name: /Scan for junk/}));
        expect(await lastText("scan exploded")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: /Scan for junk/})).toBeEnabled();
    });
});

describe("storage: live progress and cancel", () => {
    function pending<T>() {
        let resolve!: (v: T) => void;
        let reject!: (e: unknown) => void;
        const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
        return {promise, resolve, reject};
    }

    it("shows the counters of a running scan, ignores other tasks' events, and Cancel stops it", async () => {
        const scan = pending<CleanCategory[]>();
        api.StorageScan.mockReturnValue(scan.promise);
        const user = await open();
        await user.click(screen.getByRole("button", {name: /Scan for junk/}));

        const status = await lastText("Looking for files to clean…");
        expect(status.closest("[role=status]")).toBeInTheDocument();
        await emit.storageProgress({task: "scan", files: 1234, bytes: 5 * MB, current: "C:\\Windows\\Temp\\a.tmp"});
        expect(await screen.findByText("1234 files · 5 MB so far")).toBeInTheDocument();
        expect(screen.getAllByText("C:\\Windows\\Temp\\a.tmp").length).toBeGreaterThan(0);
        await emit.storageProgress({task: "dups", files: 99, bytes: 1, current: "D:\\other"});
        expect(screen.queryByText("D:\\other")).not.toBeInTheDocument();
        expect(screen.getByText("1234 files · 5 MB so far")).toBeInTheDocument();

        // one long job at a time: the other starting buttons wait
        expect(screen.getByRole("button", {name: /Scan again|Scan for junk/})).toBeDisabled();
        expect(screen.getByRole("button", {name: "Find the biggest"})).toBeDisabled();
        expect(screen.getByRole("button", {name: "Find duplicates"})).toBeDisabled();

        await user.click(screen.getByRole("button", {name: "Cancel: Looking for files to clean…"}));
        expect(api.StorageCancel).toHaveBeenCalledTimes(1);

        scan.reject(new Error("scan cancelled"));
        await waitFor(() => expect(screen.queryByRole("button", {name: "Cancel: Looking for files to clean…"})).not.toBeInTheDocument());
        expect(screen.queryByText("scan cancelled")).not.toBeInTheDocument(); // a stop is not an error
        expect(screen.getByRole("button", {name: /Scan for junk/})).toBeEnabled();
        expect(screen.getByRole("button", {name: "Find the biggest"})).toBeEnabled();
    });

    it("keeps what a stopped scan found, and says the list may be incomplete", async () => {
        const scan = pending<CleanCategory[]>();
        api.StorageScan.mockReturnValue(scan.promise);
        const user = await open();
        await user.click(screen.getByRole("button", {name: /Scan for junk/}));
        await user.click(await screen.findByRole("button", {name: /^Cancel: /}));
        scan.resolve([cat("thumbnails", 64 * MB)]);
        expect(await screen.findByRole("checkbox", {name: "Select for cleaning: Picture preview cache"})).toBeInTheDocument();
        expect(screen.getByText("Stopped early: the list below may be incomplete.")).toBeInTheDocument();
    });

    it("shows a cleaning in progress, with Cancel", async () => {
        const clean = pending<{ freedBytes: number; files: number; failed: number; outcomes: [] }>();
        api.StorageClean.mockReturnValue(clean.promise);
        const user = await open();
        await scanJunk(user);
        await user.click(screen.getByRole("button", {name: /Clean selected/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Clean"}));
        expect(await lastText("Cleaning…")).toBeInTheDocument();
        await emit.storageProgress({task: "clean", files: 7, bytes: 2048});
        expect((await screen.findAllByText("7 files · 2 KB so far")).length).toBeGreaterThan(0);
        expect(screen.getByRole("checkbox", {name: "Select for cleaning: Recycle Bin"})).toBeDisabled();
        await user.click(screen.getByRole("button", {name: "Cancel: Cleaning…"}));
        expect(api.StorageCancel).toHaveBeenCalled();
        clean.resolve({freedBytes: 0, files: 0, failed: 0, outcomes: []});
        // Once it ends the status goes; the Tasks list keeps it as stopped.
        await waitFor(() => expect(screen.queryByRole("button", {name: "Cancel: Cleaning…"})).not.toBeInTheDocument());
    });

    it("keeps a running clean in view after leaving the page and coming back, and lists how it ended", async () => {
        const clean = pending<{ freedBytes: number; files: number; failed: number; outcomes: [] }>();
        api.StorageClean.mockReturnValue(clean.promise);
        const user = await open();
        await scanJunk(user);
        await user.click(screen.getByRole("button", {name: /Clean selected/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Clean"}));
        await lastText("Cleaning…");
        const nav = () => within(screen.getByRole("navigation", {name: "Main menu"}));
        await user.click(nav().getByRole("button", {name: /Home/}));
        await waitFor(() => expect(screen.queryByRole("heading", {name: "Storage", level: 1})).not.toBeInTheDocument());
        await user.click(nav().getByRole("button", {name: /Storage/}));
        await screen.findByRole("heading", {name: "Storage", level: 1});
        // The job is still there: in the Tasks list, in its section, and the page does not start a second one.
        const tasks = screen.getByRole("heading", {name: "Tasks", level: 2}).closest("section") as HTMLElement;
        expect(within(tasks).getByText("Cleaning…")).toBeInTheDocument();
        expect(within(tasks).getByText("Running")).toBeInTheDocument();
        expect(screen.getAllByRole("button", {name: "Cancel: Cleaning…"}).length).toBeGreaterThan(0);
        expect(screen.getByRole("button", {name: /Scan again/})).toBeDisabled(); // the list scanned before leaving is still kept
        await emit.storageProgress({task: "clean", files: 9, bytes: 4096});
        expect((await screen.findAllByText("9 files · 4 KB so far")).length).toBeGreaterThan(0);
        clean.resolve({freedBytes: 3 * MB, files: 5, failed: 0, outcomes: []});
        await waitFor(() => expect(within(tasks).getAllByText("Done").length).toBeGreaterThan(0));
        expect(within(tasks).getByText("Freed 3 MB (5 files).")).toBeInTheDocument();
        expect(api.StorageClean).toHaveBeenCalledTimes(1);
    });

    it("reports a Cancel that could not be sent", async () => {
        api.StorageCancel.mockRejectedValue(new Error("cancel failed"));
        const scan = pending<CleanCategory[]>();
        api.StorageScan.mockReturnValue(scan.promise);
        const user = await open();
        await user.click(screen.getByRole("button", {name: /Scan for junk/}));
        await user.click(await screen.findByRole("button", {name: /^Cancel: /}));
        expect(await screen.findByText("cancel failed")).toBeInTheDocument();
        scan.resolve([]);
    });
});

const file = (name: string, mb: number, dir = `${HOME}\\Downloads`, modified = "2024-03-05T12:00:00Z"): BigFile => ({path: `${dir}\\${name}`, name, sizeBytes: mb * MB, modified});
const folder = (name: string, mb: number, files = 10, dir = HOME): FolderSize => ({path: `${dir}\\${name}`, name, sizeBytes: mb * MB, files});

describe("storage: biggest things", () => {
    it("starts in the home folder and only reads it when asked", async () => {
        const user = await open();
        const sec = section("Biggest things");
        await waitFor(() => expect(within(sec).getByText(HOME)).toBeInTheDocument());
        expect(api.StorageBigFiles).not.toHaveBeenCalled();
        api.StorageBigFiles.mockResolvedValue([file("movie.mkv", 4096), file("setup.iso", 700, `${HOME}\\Desktop`)]);
        await user.click(within(sec).getByRole("button", {name: "Find the biggest"}));
        expect(api.StorageBigFiles).toHaveBeenCalledWith(HOME, 50);
        const table = await within(sec).findByRole("table");
        const rows = within(table).getAllByRole("row").slice(1);
        expect(rows).toHaveLength(2);
        expect(within(rows[0]).getByText("movie.mkv")).toBeInTheDocument();
        expect(within(rows[0]).getByText("4 GB")).toBeInTheDocument();
        expect(within(rows[0]).getByText(/Mar.*2024/)).toBeInTheDocument();
        expect(within(rows[0]).getByText(`${HOME}\\Downloads`)).toBeInTheDocument();
        expect(within(rows[1]).getByText("700 MB")).toBeInTheDocument();
        expect(within(rows[1]).getByText(`${HOME}\\Desktop`)).toBeInTheDocument();
        expect(within(sec).getByRole("button", {name: "Scan again"})).toBeInTheDocument();
    });

    it("says when a folder has no files, and never starts without a folder", async () => {
        api.StorageDefaults.mockResolvedValue({home: "", downloads: "", documents: "", pictures: "", videos: ""});
        await open();
        const sec = section("Biggest things");
        expect(within(sec).getByRole("button", {name: "Find the biggest"})).toBeDisabled();
        expect(within(sec).getByText("Choose a folder to look in.")).toBeInTheDocument();
    });

    it("says so when nothing is found", async () => {
        const user = await open();
        const sec = section("Biggest things");
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Find the biggest"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Find the biggest"}));
        expect(await within(sec).findByText("No files were found here.")).toBeInTheDocument();
        await user.click(within(sec).getByRole("button", {name: "Folders"}));
        expect(await within(sec).findByText("No folders were found here.")).toBeInTheDocument();
    });

    it("shows a file in its folder", async () => {
        api.StorageBigFiles.mockResolvedValue([file("movie.mkv", 4096)]);
        const user = await open();
        const sec = section("Biggest things");
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Find the biggest"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Find the biggest"}));
        await user.click(await within(sec).findByRole("button", {name: "Show in folder: movie.mkv"}));
        expect(api.StorageReveal).toHaveBeenCalledWith(`${HOME}\\Downloads\\movie.mkv`);
    });

    async function withTwoFiles() {
        api.StorageBigFiles.mockResolvedValue([file("movie.mkv", 4096), file("setup.iso", 700)]);
        const user = await open();
        const sec = section("Biggest things");
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Find the biggest"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Find the biggest"}));
        await within(sec).findByRole("table");
        return {user, sec};
    }

    it("moves a file to the Recycle Bin only after a confirmation", async () => {
        api.StorageRecycle.mockResolvedValue({moved: 1, freedBytes: 4096 * MB, failed: []});
        const {user, sec} = await withTwoFiles();
        await user.click(within(sec).getByRole("button", {name: "Move to Recycle Bin: movie.mkv"}));
        const dialog = await screen.findByRole("alertdialog", {name: "Move to the Recycle Bin?"});
        expect(dialog).toHaveTextContent("movie.mkv (4 GB)");
        expect(dialog).toHaveTextContent("restore it from there");
        await user.click(within(dialog).getByRole("button", {name: "Cancel"}));
        expect(api.StorageRecycle).not.toHaveBeenCalled();
        expect(within(sec).getByText("movie.mkv")).toBeInTheDocument();

        await user.click(within(sec).getByRole("button", {name: "Move to Recycle Bin: movie.mkv"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Move to Recycle Bin"}));
        expect(api.StorageRecycle).toHaveBeenCalledWith([`${HOME}\\Downloads\\movie.mkv`]);
        await waitFor(() => expect(within(sec).queryByText("movie.mkv")).not.toBeInTheDocument());
        expect(within(sec).getByText("setup.iso")).toBeInTheDocument();
        expect(await screen.findByText("Moved 1 to the Recycle Bin (4 GB). Empty it to get the space back.")).toBeInTheDocument();
    });

    it("keeps a file that could not be moved and says which", async () => {
        api.StorageRecycle.mockResolvedValue({moved: 0, freedBytes: 0, failed: [`${HOME}\\Downloads\\movie.mkv`]});
        const {user, sec} = await withTwoFiles();
        await user.click(within(sec).getByRole("button", {name: "Move to Recycle Bin: movie.mkv"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Move to Recycle Bin"}));
        expect(await screen.findByText("1 could not be moved: movie.mkv")).toBeInTheDocument();
        expect(within(sec).getByText("movie.mkv")).toBeInTheDocument();
    });

    it("shows the reason when the Recycle Bin call is refused", async () => {
        api.StorageRecycle.mockRejectedValue(new Error("path was not found by a scan"));
        const {user, sec} = await withTwoFiles();
        await user.click(within(sec).getByRole("button", {name: "Move to Recycle Bin: setup.iso"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Move to Recycle Bin"}));
        expect(await screen.findByText("path was not found by a scan")).toBeInTheDocument();
        expect(within(sec).getByText("setup.iso")).toBeInTheDocument();
        expect(within(sec).getByRole("button", {name: "Move to Recycle Bin: setup.iso"})).toBeEnabled();
    });

    it("lists folders with bars relative to the biggest, and goes into one with a breadcrumb back", async () => {
        api.StorageFolders.mockImplementation(async (root: string) =>
            root === HOME ? [folder("Videos", 8192), folder("Documents", 2048), folder("Music", 1)]
                : [folder("Holidays", 4096, 3, `${HOME}\\Videos`)]);
        api.StorageBigFiles.mockResolvedValue([file("clip.mp4", 50, `${HOME}\\Videos`)]);
        const user = await open();
        const sec = section("Biggest things");
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Folders"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Folders"}));
        await user.click(within(sec).getByRole("button", {name: "Find the biggest"}));
        expect(api.StorageFolders).toHaveBeenCalledWith(HOME);
        const items = await within(sec).findAllByRole("button", {name: /^Look inside /});
        expect(items.map((b) => b.getAttribute("aria-label"))).toEqual(["Look inside Videos", "Look inside Documents", "Look inside Music"]);
        const width = (b: HTMLElement) => (b.querySelector(".bar i") as HTMLElement).style.width;
        expect(width(items[0])).toBe("100%");
        expect(width(items[1])).toBe("25%");
        expect(width(items[2])).toBe("2%"); // never an invisible bar
        expect(items[0]).toHaveTextContent("8 GB");
        expect(items[0]).toHaveTextContent("10 files");
        expect(within(sec).queryByRole("navigation", {name: "Folder path"})).not.toBeInTheDocument();

        await user.click(items[0]);
        expect(api.StorageFolders).toHaveBeenLastCalledWith(`${HOME}\\Videos`);
        expect(await within(sec).findByRole("button", {name: "Look inside Holidays"})).toBeInTheDocument();
        expect(within(sec).queryByRole("button", {name: "Look inside Music"})).not.toBeInTheDocument();
        const crumbs = within(sec).getByRole("navigation", {name: "Folder path"});
        expect(within(crumbs).getByRole("button", {name: HOME})).toBeInTheDocument();
        expect(within(crumbs).getByText("Videos")).toHaveAttribute("aria-current", "location");
        expect(within(sec).getAllByText(`${HOME}\\Videos`).length).toBeGreaterThan(0);

        // the Files tab now looks in the folder we are in
        await user.click(within(sec).getByRole("button", {name: "Files"}));
        await waitFor(() => expect(api.StorageBigFiles).toHaveBeenCalledWith(`${HOME}\\Videos`, 50));
        expect(await within(sec).findByText("clip.mp4")).toBeInTheDocument();

        // and the breadcrumb goes back
        await user.click(within(crumbs).getByRole("button", {name: HOME}));
        expect(within(sec).queryByRole("navigation", {name: "Folder path"})).not.toBeInTheDocument();
        expect(within(sec).getByText(HOME)).toBeInTheDocument();
        await user.click(within(sec).getByRole("button", {name: "Folders"}));
        expect(await within(sec).findByRole("button", {name: "Look inside Music"})).toBeInTheDocument();
        expect(api.StorageFolders).toHaveBeenCalledTimes(2); // the root was remembered, not read again
    });

    it("drills down more than one level and goes back to a middle step", async () => {
        api.StorageFolders.mockImplementation(async (root: string) => [folder("Sub", 10, 1, root)]);
        const user = await open();
        const sec = section("Biggest things");
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Folders"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Folders"}));
        await user.click(within(sec).getByRole("button", {name: "Find the biggest"}));
        await user.click(await within(sec).findByRole("button", {name: "Look inside Sub"}));
        await user.click(await within(sec).findByRole("button", {name: "Look inside Sub"}));
        await user.click(await within(sec).findByRole("button", {name: "Look inside Sub"}));
        expect(api.StorageFolders).toHaveBeenLastCalledWith(`${HOME}\\Sub\\Sub\\Sub`);
        const crumbs = within(sec).getByRole("navigation", {name: "Folder path"});
        expect(within(crumbs).getAllByRole("button")).toHaveLength(3); // home, Sub, Sub; the last Sub is the page we are on
        await user.click(within(crumbs).getAllByRole("button")[1]);
        expect(within(sec).getByRole("navigation", {name: "Folder path"})).toBeInTheDocument();
        expect(within(within(sec).getByRole("navigation", {name: "Folder path"})).getAllByRole("button")).toHaveLength(1);
        expect(api.StorageFolders).toHaveBeenCalledTimes(4);
    });

    it("chooses another folder with the folder picker and reads it right away", async () => {
        api.PickFolder.mockResolvedValue("D:\\Data");
        api.StorageBigFiles.mockResolvedValue([file("big.bin", 900, "D:\\Data")]);
        const user = await open();
        const sec = section("Biggest things");
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Choose folder…"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Choose folder…"}));
        expect(api.StorageBigFiles).toHaveBeenCalledWith("D:\\Data", 50);
        expect(await within(sec).findByText("big.bin")).toBeInTheDocument();
        expect(within(sec).getAllByText("D:\\Data").length).toBeGreaterThan(0);
    });

    it("ignores a cancelled folder picker", async () => {
        api.PickFolder.mockResolvedValue("");
        const user = await open();
        const sec = section("Biggest things");
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Choose folder…"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Choose folder…"}));
        expect(api.StorageBigFiles).not.toHaveBeenCalled();
        expect(within(sec).getByText(HOME)).toBeInTheDocument();
    });

    it("shows a failed read as a message and does not retry by itself", async () => {
        api.StorageBigFiles.mockRejectedValue(new Error("folder unreadable"));
        const user = await open();
        const sec = section("Biggest things");
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Find the biggest"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Find the biggest"}));
        expect(await lastText("folder unreadable")).toBeInTheDocument();
        await new Promise((r) => setTimeout(r, 50));
        expect(api.StorageBigFiles).toHaveBeenCalledTimes(1);
        expect(within(sec).getByRole("button", {name: "Find the biggest"})).toBeEnabled();
    });

    it("shows live counters while measuring files", async () => {
        let done!: (v: BigFile[]) => void;
        api.StorageBigFiles.mockReturnValue(new Promise<BigFile[]>((r) => { done = r; }));
        const user = await open();
        const sec = section("Biggest things");
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Find the biggest"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Find the biggest"}));
        expect(await within(sec).findByText("Measuring files…")).toBeInTheDocument();
        await emit.storageProgress({task: "big", files: 42, bytes: 3 * GB, current: "C:\\x"});
        expect(await within(sec).findByText("42 files · 3 GB so far")).toBeInTheDocument();
        await user.click(within(sec).getByRole("button", {name: "Cancel: Measuring files…"}));
        expect(api.StorageCancel).toHaveBeenCalled();
        done([]);
        await waitFor(() => expect(within(sec).queryByText("Measuring files…")).not.toBeInTheDocument());
    });
});

const dup = (id: string, mb: number, files: [string, string][]): DupGroup => ({id, sizeBytes: mb * MB, files: files.map(([path, modified]) => ({path, modified}))});
// waste: g-small 10 MB, g-mid 15 MB (4 copies of 5), g-big 100 MB
const groups = (): DupGroup[] => [
    dup("g-small", 10, [["C:\\a\\small-new.zip", "2024-06-01T00:00:00Z"], ["C:\\a\\small-old.zip", "2021-06-01T00:00:00Z"]]),
    dup("g-mid", 5, [["C:\\b\\mid1.dat", "2023-01-01T00:00:00Z"], ["C:\\b\\mid2.dat", "2020-01-01T00:00:00Z"], ["C:\\b\\mid3.dat", "2022-01-01T00:00:00Z"], ["C:\\b\\mid4.dat", "2024-01-01T00:00:00Z"]]),
    dup("g-big", 100, [["C:\\c\\big-copy.iso", "2025-01-01T00:00:00Z"], ["C:\\c\\big-orig.iso", "2019-01-01T00:00:00Z"]]),
];
const dupSection = () => section("Duplicate files");

async function scanDups(opts: { lang?: "es" } = {}) {
    api.StorageDuplicates.mockImplementation(async () => groups());
    const user = await open(opts);
    const sec = dupSection();
    await waitFor(() => expect(within(sec).getByRole("button", {name: opts.lang ? "Buscar duplicados" : "Find duplicates"})).toBeEnabled());
    await user.click(within(sec).getByRole("button", {name: opts.lang ? "Buscar duplicados" : "Find duplicates"}));
    await within(sec).findAllByText(opts.lang ? /copias idénticas/ : /identical copies/);
    return {user, sec};
}

describe("storage: duplicates", () => {
    it("starts with the Downloads, Documents, Pictures and Videos folders, and lets people remove and add folders", async () => {
        api.PickFolder.mockResolvedValue("D:\\Backups");
        const user = await open();
        const sec = dupSection();
        const names = () => within(sec).getAllByRole("button").map((b) => b.getAttribute("aria-label")).filter((l) => l?.startsWith("Remove folder"));
        await waitFor(() => expect(names()).toHaveLength(4));
        expect(names()).toEqual([`Remove folder: ${HOME}\\Downloads`, `Remove folder: ${HOME}\\Documents`, `Remove folder: ${HOME}\\Pictures`, `Remove folder: ${HOME}\\Videos`]);

        await user.click(within(sec).getByRole("button", {name: `Remove folder: ${HOME}\\Pictures`}));
        expect(names()).toHaveLength(3);
        await user.click(within(sec).getByRole("button", {name: "Add folder…"}));
        expect(await within(sec).findByText("D:\\Backups")).toBeInTheDocument();
        // a folder that is already there is not added twice, whatever its case or trailing slash
        api.PickFolder.mockResolvedValue(`${HOME.toLowerCase()}\\downloads\\`);
        await user.click(within(sec).getByRole("button", {name: "Add folder…"}));
        expect(names()).toHaveLength(4);

        await user.click(within(sec).getByRole("button", {name: "Find duplicates"}));
        expect(api.StorageDuplicates).toHaveBeenCalledWith([`${HOME}\\Downloads`, `${HOME}\\Documents`, `${HOME}\\Videos`, "D:\\Backups"], 10);
    });

    it("cannot search without a folder", async () => {
        const user = await open();
        const sec = dupSection();
        await waitFor(() => expect(within(sec).getAllByRole("button", {name: /^Remove folder/})).toHaveLength(4));
        for (const b of within(sec).getAllByRole("button", {name: /^Remove folder/})) await user.click(b);
        expect(within(sec).getByText("Add at least one folder to search.")).toBeInTheDocument();
        expect(within(sec).getByRole("button", {name: "Find duplicates"})).toBeDisabled();
    });

    it("ignores a cancelled folder picker", async () => {
        api.PickFolder.mockResolvedValue("");
        const user = await open();
        const sec = dupSection();
        await waitFor(() => expect(within(sec).getAllByRole("button", {name: /^Remove folder/})).toHaveLength(4));
        await user.click(within(sec).getByRole("button", {name: "Add folder…"}));
        expect(within(sec).getAllByRole("button", {name: /^Remove folder/})).toHaveLength(4);
    });

    it("sends the minimum size that was chosen (1, 10 or 100 MB)", async () => {
        const user = await open();
        const sec = dupSection();
        const select = within(sec).getByRole("combobox", {name: /Ignore files smaller than/}) as HTMLSelectElement;
        expect([...select.options].map((o) => o.textContent)).toEqual(["1 MB", "10 MB", "100 MB"]);
        expect(select.value).toBe("10");
        await user.selectOptions(select, "100");
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Find duplicates"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Find duplicates"}));
        expect(api.StorageDuplicates).toHaveBeenCalledWith(expect.any(Array), 100);
        expect(await within(sec).findByText("No duplicate files were found.")).toBeInTheDocument();
    });

    it("lists the groups with the most wasted space first", async () => {
        const {sec} = await scanDups();
        const heads = within(sec).getAllByText(/identical copies of/).map((e) => e.textContent);
        expect(heads).toEqual(["2 identical copies of 100 MB", "4 identical copies of 5 MB", "2 identical copies of 10 MB"]);
        const waste = within(sec).getAllByText(/can be freed/).map((e) => e.textContent).filter((s) => s?.startsWith("100") || s?.startsWith("15") || s?.startsWith("10 "));
        expect(waste).toEqual(["100 MB can be freed", "15 MB can be freed", "10 MB can be freed"]);
    });

    it("keeps the oldest copy of each group by default and marks the others", async () => {
        const {sec} = await scanDups();
        const box = (p: string) => within(sec).getByRole("checkbox", {name: `Move to Recycle Bin: ${p}`}) as HTMLInputElement;
        expect(box("C:\\c\\big-orig.iso").checked).toBe(false);
        expect(box("C:\\c\\big-copy.iso").checked).toBe(true);
        expect(box("C:\\b\\mid2.dat").checked).toBe(false); // 2020 is the oldest of four
        for (const p of ["C:\\b\\mid1.dat", "C:\\b\\mid3.dat", "C:\\b\\mid4.dat"]) expect(box(p).checked, p).toBe(true);
        expect(box("C:\\a\\small-old.zip").checked).toBe(false);
        expect(box("C:\\a\\small-new.zip").checked).toBe(true);
        // 5 copies marked: 100 + 3*5 + 10 MB
        expect(within(sec).getByText("5 files, 125 MB can be freed")).toBeInTheDocument();
        // the copy that stays says so, and cannot be marked
        expect(box("C:\\c\\big-orig.iso")).toBeDisabled();
        expect(box("C:\\c\\big-copy.iso")).toBeEnabled();
        expect(within(box("C:\\c\\big-orig.iso").closest("label") as HTMLElement).getByText("Kept")).toBeInTheDocument();
        expect(within(box("C:\\c\\big-copy.iso").closest("label") as HTMLElement).queryByText("Kept")).not.toBeInTheDocument();
    });

    it("lets people change which copy stays, but never mark every copy of a group", async () => {
        const {user, sec} = await scanDups();
        const box = (p: string) => within(sec).getByRole("checkbox", {name: `Move to Recycle Bin: ${p}`}) as HTMLInputElement;
        const pair = ["C:\\c\\big-orig.iso", "C:\\c\\big-copy.iso"];
        const marked = () => pair.filter((p) => box(p).checked);

        // unmark the copy: now both stay, so nothing is locked
        await user.click(box("C:\\c\\big-copy.iso"));
        expect(marked()).toEqual([]);
        expect(box(pair[0])).toBeEnabled();
        expect(box(pair[1])).toBeEnabled();
        expect(within(sec).getByText("4 files, 25 MB can be freed")).toBeInTheDocument();

        // mark the original instead: the other one is now the locked one
        await user.click(box("C:\\c\\big-orig.iso"));
        expect(marked()).toEqual(["C:\\c\\big-orig.iso"]);
        expect(box("C:\\c\\big-copy.iso")).toBeDisabled();
        expect(box("C:\\c\\big-copy.iso").checked).toBe(false);
        expect(box("C:\\c\\big-orig.iso")).toBeEnabled();

        // whatever is clicked, at least one copy of the group always stays
        for (let i = 0; i < 3; i++) for (const p of pair) { if (!box(p).disabled) await user.click(box(p)); }
        expect(marked().length).toBeLessThan(pair.length);

        // in a group of four: mark three, the fourth locks; unmark one and it frees up
        const four = ["C:\\b\\mid1.dat", "C:\\b\\mid2.dat", "C:\\b\\mid3.dat", "C:\\b\\mid4.dat"];
        const unchecked = () => four.filter((p) => !box(p).checked);
        expect(unchecked()).toEqual(["C:\\b\\mid2.dat"]);
        await user.click(box("C:\\b\\mid1.dat"));
        expect(unchecked()).toHaveLength(2);
        for (const p of unchecked()) expect(box(p)).toBeEnabled();
        await user.click(box("C:\\b\\mid2.dat"));
        expect(unchecked()).toEqual(["C:\\b\\mid1.dat"]);
        expect(box("C:\\b\\mid1.dat")).toBeDisabled();
    });

    it("updates the summary as copies are marked, and disables the button when none is", async () => {
        const {user, sec} = await scanDups();
        await user.click(within(sec).getByRole("checkbox", {name: "Move to Recycle Bin: C:\\c\\big-copy.iso"}));
        expect(within(sec).getByText("4 files, 25 MB can be freed")).toBeInTheDocument();
        await user.click(within(sec).getByRole("checkbox", {name: "Move to Recycle Bin: C:\\a\\small-new.zip"}));
        expect(within(sec).getByText("3 files, 15 MB can be freed")).toBeInTheDocument();
        for (const p of ["mid1.dat", "mid3.dat", "mid4.dat"]) await user.click(within(sec).getByRole("checkbox", {name: `Move to Recycle Bin: C:\\b\\${p}`}));
        expect(within(sec).getByText("No files selected")).toBeInTheDocument();
        expect(within(sec).getByRole("button", {name: "Move selected to Recycle Bin"})).toBeDisabled();
    });

    it("says what a single marked file frees with the singular", async () => {
        api.StorageDuplicates.mockResolvedValue([dup("one", 7, [["C:\\x\\a.bin", "2020-01-01T00:00:00Z"], ["C:\\x\\b.bin", "2021-01-01T00:00:00Z"]])]);
        const user = await open();
        const sec = dupSection();
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Find duplicates"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Find duplicates"}));
        expect(await within(sec).findByText("1 file, 7 MB can be freed")).toBeInTheDocument();
    });

    it("asks first, sends only the marked paths, and scans again afterwards", async () => {
        api.StorageRecycle.mockResolvedValue({moved: 5, freedBytes: 125 * MB, failed: []});
        const {user, sec} = await scanDups();
        await user.click(within(sec).getByRole("checkbox", {name: "Move to Recycle Bin: C:\\b\\mid4.dat"})); // let one more copy stay
        await user.click(within(sec).getByRole("button", {name: "Move selected to Recycle Bin"}));
        const dialog = await screen.findByRole("alertdialog", {name: "Move to the Recycle Bin?"});
        expect(dialog).toHaveTextContent("4 files (120 MB) go to the Recycle Bin. One copy of each stays where it is.");
        await user.click(within(dialog).getByRole("button", {name: "Cancel"}));
        expect(api.StorageRecycle).not.toHaveBeenCalled();
        expect(api.StorageDuplicates).toHaveBeenCalledTimes(1);

        api.StorageDuplicates.mockResolvedValue([]);
        await user.click(within(sec).getByRole("button", {name: "Move selected to Recycle Bin"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Move to Recycle Bin"}));
        expect(api.StorageRecycle).toHaveBeenCalledTimes(1);
        const sent = (api.StorageRecycle.mock.calls[0][0] as string[]).slice().sort();
        expect(sent).toEqual(["C:\\a\\small-new.zip", "C:\\b\\mid1.dat", "C:\\b\\mid3.dat", "C:\\c\\big-copy.iso"].sort());
        expect(sent).not.toContain("C:\\c\\big-orig.iso");
        expect(sent).not.toContain("C:\\b\\mid4.dat");
        await waitFor(() => expect(api.StorageDuplicates).toHaveBeenCalledTimes(2));
        expect(await within(sec).findByText("No duplicate files were found.")).toBeInTheDocument();
        expect(await screen.findByText("Moved 5 to the Recycle Bin (125 MB). Empty it to get the space back.")).toBeInTheDocument();
    });

    it("shows the reason when the move is refused, and does not scan again", async () => {
        api.StorageRecycle.mockRejectedValue(new Error("would remove every copy"));
        const {user, sec} = await scanDups();
        await user.click(within(sec).getByRole("button", {name: "Move selected to Recycle Bin"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Move to Recycle Bin"}));
        expect(await screen.findByText("would remove every copy")).toBeInTheDocument();
        expect(api.StorageDuplicates).toHaveBeenCalledTimes(1);
        expect(within(sec).getByText("5 files, 125 MB can be freed")).toBeInTheDocument(); // the choices are still there
        expect(within(sec).getByRole("button", {name: "Move selected to Recycle Bin"})).toBeEnabled();
    });

    it("lists failures of the move by file name", async () => {
        api.StorageRecycle.mockResolvedValue({moved: 4, freedBytes: 25 * MB, failed: ["C:\\c\\big-copy.iso"]});
        api.StorageDuplicates.mockImplementation(async () => groups());
        const {user, sec} = await scanDups();
        await user.click(within(sec).getByRole("button", {name: "Move selected to Recycle Bin"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Move to Recycle Bin"}));
        expect(await screen.findByText("1 could not be moved: big-copy.iso")).toBeInTheDocument();
    });

    it("shows 25 groups at a time", async () => {
        api.StorageDuplicates.mockResolvedValue(Array.from({length: 30}, (_, i) =>
            dup(`g${i}`, 1 + i, [[`C:\\d\\${i}-a.bin`, "2020-01-01T00:00:00Z"], [`C:\\d\\${i}-b.bin`, "2021-01-01T00:00:00Z"]])));
        const user = await open();
        const sec = dupSection();
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Find duplicates"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Find duplicates"}));
        await within(sec).findAllByText(/identical copies/);
        expect(within(sec).getAllByText(/identical copies/)).toHaveLength(25);
        // all 30 groups count in the summary, shown or not
        expect(within(sec).getByText(/^30 files, /)).toBeInTheDocument();
        await user.click(within(sec).getByRole("button", {name: "Show more (5)"}));
        expect(within(sec).getAllByText(/identical copies/)).toHaveLength(30);
        expect(within(sec).queryByRole("button", {name: /Show more/})).not.toBeInTheDocument();
    });

    it("shows live counters while searching and can be cancelled", async () => {
        let done!: (v: DupGroup[]) => void;
        api.StorageDuplicates.mockReturnValue(new Promise<DupGroup[]>((r) => { done = r; }));
        const user = await open();
        const sec = dupSection();
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Find duplicates"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Find duplicates"}));
        expect(await within(sec).findByText("Looking for duplicate files…")).toBeInTheDocument();
        await emit.storageProgress({task: "dups", files: 500, bytes: 2 * GB});
        expect(await within(sec).findByText("500 files · 2 GB so far")).toBeInTheDocument();
        expect(within(sec).getByRole("button", {name: "Add folder…"})).toBeDisabled();
        await user.click(within(sec).getByRole("button", {name: "Cancel: Looking for duplicate files…"}));
        expect(api.StorageCancel).toHaveBeenCalled();
        done([]);
        await waitFor(() => expect(within(sec).queryByText("Looking for duplicate files…")).not.toBeInTheDocument());
    });
});

describe("storage: optimize", () => {
    it("has a card per maintenance task, and warns that turning hibernation off also turns off Fast Startup", async () => {
        await open();
        const sec = section("Optimize");
        for (const n of ["Re-trim solid-state drives", "Defragment hard drives", "Clean old Windows update files", "Turn off hibernation"]) {
            expect(within(sec).getByRole("button", {name: `Run: ${n}`})).toBeInTheDocument();
        }
        expect(within(sec).getByText(/Also turns off Fast Startup/)).toBeInTheDocument();
        expect(within(sec).getByText(/about as much space as your RAM/)).toBeInTheDocument();
    });

    it.each([
        ["Re-trim solid-state drives", "optimize-trim"],
        ["Defragment hard drives", "optimize-defrag"],
        ["Clean old Windows update files", "optimize-component-cleanup"],
        ["Turn off hibernation", "optimize-hibernate-off"],
    ])("plans %s through the repair runner and opens the run dialog", async (title, id) => {
        const user = await open();
        await user.click(within(section("Optimize")).getByRole("button", {name: `Run: ${title}`}));
        expect(api.PlanRepairs).toHaveBeenCalledWith([id]);
        const dialog = await screen.findByRole("dialog", {name: "Optimize storage"});
        expect(within(dialog).getByText(title)).toBeInTheDocument(); // the step has a human name, not the raw id
        expect(api.ApplyRepairs).not.toHaveBeenCalled();
        await user.click(within(dialog).getByRole("button", {name: "Run 1 step"}));
        expect(api.ApplyRepairs).toHaveBeenCalledWith([id]);
    });

    it("measures the drives again when a maintenance run finishes, without scanning PC health", async () => {
        const user = await open();
        await screen.findByRole("meter", {name: /C: Windows/});
        await user.click(within(section("Optimize")).getByRole("button", {name: "Run: Turn off hibernation"}));
        const dialog = await screen.findByRole("dialog", {name: "Optimize storage"});
        await user.click(within(dialog).getByRole("button", {name: "Run 1 step"}));
        const step = {kind: "recipe" as const, id: "optimize-hibernate-off", name: "x", admin: true};
        const reads = api.StorageVolumes.mock.calls.length;
        await emit.install({step, status: "start"});
        await emit.install({step, status: "ok"});
        await emit.done([]);
        await waitFor(() => expect(api.StorageVolumes.mock.calls.length).toBeGreaterThan(reads));
        expect(api.HealthScan).not.toHaveBeenCalled();
    });
});

describe("storage: Spanish", () => {
    it("shows the tab, the drives and the junk scan in Spanish, with decimal commas", async () => {
        api.StorageScan.mockResolvedValue([cat("temp-user", 1.5 * GB, {files: 3}), cat("windows-old", 12 * GB, {risk: "review", needsAdmin: true})]);
        const user = await open({lang: "es", admin: false});
        expect(await screen.findByRole("meter", {name: "C: Windows, 95% usado"})).toBeInTheDocument();
        expect(screen.getByText("95 GB usados de 100 GB · 5 GB libres")).toBeInTheDocument();
        expect(screen.getByText(/Poco espacio: solo quedan 5 GB libres/)).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: /Buscar basura/}));
        const row = (await screen.findByRole("checkbox", {name: "Marcar para limpiar: Tus archivos temporales"})).closest("label") as HTMLElement;
        expect(row).toHaveTextContent("1,5 GB");
        expect(row).toHaveTextContent("3 archivos");
        const old = screen.getByRole("checkbox", {name: "Marcar para limpiar: Versión anterior de Windows"}).closest("label") as HTMLElement;
        expect(within(old).getByText("Revísalo")).toBeInTheDocument();
        expect(within(old).getByText("Pide admin")).toBeInTheDocument();
        expect(screen.getByText("1 marcados · 1,5 GB")).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: /Limpiar lo marcado/}));
        const dialog = await screen.findByRole("alertdialog", {name: "¿Limpiar lo marcado?"});
        expect(dialog).toHaveTextContent("no se puede deshacer");
        expect(within(dialog).getByText("Tus archivos temporales (1,5 GB)")).toBeInTheDocument();
        expect(within(dialog).getByRole("button", {name: "Cancelar"})).toBeInTheDocument();
    });

    it("shows biggest things, duplicates and optimize in Spanish", async () => {
        api.StorageBigFiles.mockResolvedValue([file("peli.mkv", 1536)]);
        api.StorageDuplicates.mockImplementation(async () => groups());
        const user = await open({lang: "es"});
        const big = section("Lo más grande");
        await waitFor(() => expect(within(big).getByRole("button", {name: "Buscar lo más grande"})).toBeEnabled());
        await user.click(within(big).getByRole("button", {name: "Buscar lo más grande"}));
        const table = await within(big).findByRole("table");
        expect(within(table).getByText("1,5 GB")).toBeInTheDocument();
        expect(within(table).getByText(/mar/)).toBeInTheDocument(); // "5 mar 2024"
        expect(within(table).getByRole("button", {name: "Mover a la Papelera: peli.mkv"})).toBeInTheDocument();
        expect(within(table).getByRole("button", {name: "Mostrar en la carpeta: peli.mkv"})).toBeInTheDocument();
        await user.click(within(big).getByRole("button", {name: "Carpetas"}));

        const dups = section("Archivos duplicados");
        await waitFor(() => expect(within(dups).getByRole("button", {name: "Buscar duplicados"})).toBeEnabled());
        await user.click(within(dups).getByRole("button", {name: "Buscar duplicados"}));
        expect(await within(dups).findByText("2 copias idénticas de 100 MB")).toBeInTheDocument();
        expect(within(dups).getByText("5 archivos, se pueden liberar 125 MB")).toBeInTheDocument();
        expect(within(dups).getAllByText("Se queda").length).toBe(3);

        const opt = section("Optimizar");
        expect(within(opt).getByText(/También desactiva el Inicio rápido/)).toBeInTheDocument();
        await user.click(within(opt).getByRole("button", {name: "Ejecutar: Desactivar la hibernación"}));
        expect(api.PlanRepairs).toHaveBeenCalledWith(["optimize-hibernate-off"]);
        const run = await screen.findByRole("dialog", {name: "Optimizar el almacenamiento"});
        expect(within(run).getByText("Desactivar la hibernación")).toBeInTheDocument();
    });
});

describe("storage: results survive leaving the page", () => {
    const nav = () => within(screen.getByRole("navigation", {name: "Main menu"}));
    async function away(user: ReturnType<typeof userEvent.setup>) {
        await user.click(nav().getByRole("button", {name: /Home/}));
        await waitFor(() => expect(screen.queryByRole("heading", {name: "Storage", level: 1})).not.toBeInTheDocument());
    }
    async function back(user: ReturnType<typeof userEvent.setup>) {
        await user.click(nav().getByRole("button", {name: /Storage/}));
        await screen.findByRole("heading", {name: "Storage", level: 1});
    }

    it("keeps the junk list and the ticks, and does not scan again", async () => {
        const user = await open();
        await scanJunk(user);
        await user.click(checkbox("Select for cleaning: Recycle Bin")); // untick a safe one
        await user.click(checkbox("Select for cleaning: Previous Windows version")); // tick a review one
        await away(user);
        await back(user);
        expect(checkbox("Select for cleaning: Recycle Bin").checked).toBe(false);
        expect(checkbox("Select for cleaning: Previous Windows version").checked).toBe(true);
        expect(checkbox("Select for cleaning: Your temporary files").checked).toBe(true);
        expect(screen.getByText(/^4 selected · /)).toBeInTheDocument();
        expect(screen.getByRole("button", {name: /Scan again/})).toBeInTheDocument();
        expect(api.StorageScan).toHaveBeenCalledTimes(1);
    });

    it("shows the result of a clean that finished while the page was away", async () => {
        let done!: (v: { freedBytes: number; files: number; failed: number; outcomes: [] }) => void;
        api.StorageClean.mockReturnValue(new Promise((r) => { done = r; }));
        const user = await open();
        await scanJunk(user);
        await user.click(screen.getByRole("button", {name: /Clean selected/}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Clean"}));
        await lastText("Cleaning…");
        await away(user);
        api.StorageScan.mockImplementation(async () => [cat("thumbnails", 100 * MB)]);
        done({freedBytes: 2 * GB, files: 77, failed: 0, outcomes: []});
        await waitFor(() => expect(api.StorageScan).toHaveBeenCalledTimes(2)); // the rescan after the clean also ran
        await back(user);
        expect(await lastText("Freed 2 GB (77 files).")).toBeInTheDocument();
        expect(screen.getByRole("checkbox", {name: "Select for cleaning: Picture preview cache"})).toBeInTheDocument();
        expect(screen.queryByRole("checkbox", {name: "Select for cleaning: Recycle Bin"})).not.toBeInTheDocument();
        expect(api.StorageClean).toHaveBeenCalledTimes(1);
    });

    it("keeps the folder, the breadcrumb and the list of Biggest things without reading again", async () => {
        api.StorageFolders.mockImplementation(async (root: string) =>
            root === HOME ? [folder("Videos", 8192), folder("Music", 1)] : [folder("Holidays", 4096, 3, `${HOME}\\Videos`)]);
        const user = await open();
        let sec = section("Biggest things");
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Folders"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Folders"}));
        await user.click(within(sec).getByRole("button", {name: "Find the biggest"}));
        await user.click(await within(sec).findByRole("button", {name: "Look inside Videos"}));
        await within(sec).findByRole("button", {name: "Look inside Holidays"});
        expect(api.StorageFolders).toHaveBeenCalledTimes(2);

        await away(user);
        await back(user);
        sec = section("Biggest things");
        expect(await within(sec).findByRole("button", {name: "Look inside Holidays"})).toBeInTheDocument();
        const crumbs = within(sec).getByRole("navigation", {name: "Folder path"});
        expect(within(crumbs).getByText("Videos")).toHaveAttribute("aria-current", "location");
        await new Promise((r) => setTimeout(r, 50));
        expect(api.StorageFolders).toHaveBeenCalledTimes(2);
        expect(api.StorageBigFiles).not.toHaveBeenCalled();
        // the way back up still works, from the remembered root
        await user.click(within(crumbs).getByRole("button", {name: HOME}));
        expect(await within(sec).findByRole("button", {name: "Look inside Music"})).toBeInTheDocument();
        expect(api.StorageFolders).toHaveBeenCalledTimes(2);
    });

    it("keeps the Biggest things file list too", async () => {
        api.StorageBigFiles.mockResolvedValue([file("movie.mkv", 4096)]);
        const user = await open();
        let sec = section("Biggest things");
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Find the biggest"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Find the biggest"}));
        await within(sec).findByText("movie.mkv");
        await away(user);
        await back(user);
        sec = section("Biggest things");
        expect(within(sec).getByText("movie.mkv")).toBeInTheDocument();
        expect(within(sec).getByRole("button", {name: "Scan again"})).toBeInTheDocument();
        expect(api.StorageBigFiles).toHaveBeenCalledTimes(1);
    });

    it("keeps the duplicates found, the marks, the folders and the minimum size", async () => {
        const {user, sec: first} = await scanDups();
        await user.selectOptions(within(first).getByRole("combobox", {name: /Ignore files smaller than/}), "100");
        await user.click(within(first).getByRole("button", {name: "Remove folder: " + HOME + "\\Pictures"}));
        await user.click(within(first).getByRole("checkbox", {name: "Move to Recycle Bin: C:\\c\\big-copy.iso"})); // let it stay
        await away(user);
        await back(user);
        const sec = dupSection();
        expect(within(sec).getAllByText(/identical copies of/)).toHaveLength(3);
        expect((within(sec).getByRole("checkbox", {name: "Move to Recycle Bin: C:\\c\\big-copy.iso"}) as HTMLInputElement).checked).toBe(false);
        expect(within(sec).getByText("4 files, 25 MB can be freed")).toBeInTheDocument();
        expect((within(sec).getByRole("combobox", {name: /Ignore files smaller than/}) as HTMLSelectElement).value).toBe("100");
        expect(within(sec).getAllByRole("button", {name: /^Remove folder/})).toHaveLength(3);
        expect(within(sec).getByRole("button", {name: "Scan again"})).toBeInTheDocument();
        expect(api.StorageDuplicates).toHaveBeenCalledTimes(1);
    });

    it("keeps the old project folders found and which are ticked", async () => {
        const proj = (name: string, mb: number) => ({path: `${HOME}\\code\\${name}\\node_modules`, project: name, projectPath: `${HOME}\\code\\${name}`, kind: "node_modules", sizeBytes: mb * MB, files: 5, lastTouched: "2024-01-01T00:00:00Z"});
        api.StorageProjects.mockResolvedValue([proj("alpha", 300), proj("beta", 200)]);
        const user = await open();
        const sec = section("Old project folders");
        await waitFor(() => expect(within(sec).getByRole("button", {name: "Find old project folders"})).toBeEnabled());
        await user.click(within(sec).getByRole("button", {name: "Find old project folders"}));
        await user.click(await within(sec).findByRole("checkbox", {name: `Delete ${HOME}\\code\\alpha\\node_modules`}));
        await away(user);
        await back(user);
        const again = section("Old project folders");
        expect((within(again).getByRole("checkbox", {name: `Delete ${HOME}\\code\\alpha\\node_modules`}) as HTMLInputElement).checked).toBe(true);
        expect((within(again).getByRole("checkbox", {name: `Delete ${HOME}\\code\\beta\\node_modules`}) as HTMLInputElement).checked).toBe(false);
        expect(within(again).getByText(/300 MB can be freed/)).toBeInTheDocument();
        expect(api.StorageProjects).toHaveBeenCalledTimes(1);
    });
});
