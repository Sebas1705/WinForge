import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {afterEach, beforeEach, describe, expect, it, vi} from "vitest";
import type {EnvFinding, EnvReport, Feature, WSLInfo} from "../api";
import App from "../App";
import {groupFindings, selectable} from "../lib/developer";
import {DICTS, KEYS, setLang} from "../lib/i18n";
import {Developer} from "../views/Developer";
import {api, resetApi} from "./apiMock";
import {makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const finding = (o: Partial<EnvFinding> & { id: string; kind: string }): EnvFinding => ({
    severity: "problem", scope: "user", subject: o.id, paths: [], fixable: true, fixKind: "remove-path-entry", ...o,
});
const report = (o: Partial<EnvReport> = {}): EnvReport => ({findings: [], userPath: [], machinePath: [], tools: [], vars: [], ...o});

/** One finding of every kind the Go side reports, plus a machine one and one this version does not know. */
const everyKind = (): EnvFinding[] => [
    finding({id: "u1", kind: "path-missing", subject: "C:\\gone"}),
    finding({id: "u2", kind: "path-not-dir", subject: "C:\\Windows\\notepad.exe"}),
    finding({id: "u3", kind: "path-duplicate", subject: "C:\\dup"}),
    finding({id: "u4", kind: "path-empty", subject: ""}),
    finding({id: "u5", kind: "path-relative", subject: "bin"}),
    finding({id: "u6", kind: "path-long", subject: "PATH", severity: "note", fixable: false, fixKind: ""}),
    finding({id: "u7", kind: "home-missing", subject: "JAVA_HOME", fixKind: "unset-variable"}),
    finding({id: "u8", kind: "home-invalid", subject: "GOROOT", fixKind: "unset-variable"}),
    finding({id: "u9", kind: "home-mismatch", subject: "PYTHONHOME", severity: "note", fixKind: "unset-variable"}),
    finding({id: "t1", kind: "tool-shadowed", subject: "node", severity: "note", fixable: false, fixKind: "", paths: ["C:\\a\\node.exe", "C:\\b\\node.exe"]}),
    finding({id: "t2", kind: "tool-store-alias", subject: "python", fixable: false, fixKind: ""}),
    finding({id: "m1", kind: "path-missing", subject: "C:\\machine-gone", scope: "machine", fixable: false, fixKind: ""}),
    finding({id: "x1", kind: "future-kind", subject: "something", severity: "note", fixable: false, fixKind: ""}),
];

const feat = (id: Feature["id"], o: Partial<Feature> = {}): Feature => ({
    id, state: "off", restartPending: false, enable: `feature-${id}-on`, disable: `feature-${id}-off`, needsAdmin: true, ...o,
});
const features = (): Feature[] => [
    feat("wsl", {state: "on"}),
    feat("vmp", {state: "off", restartPending: true}),
    feat("hyperv", {state: "unavailable"}),
    feat("sandbox", {state: "unknown"}),
    feat("netfx3", {state: "off"}),
];
const wslInfo = (): WSLInfo => ({
    installed: true, defaultVersion: 2,
    distros: [
        {name: "Ubuntu", state: "Running", version: 2, isDefault: true},
        {name: "Debian", state: "Stopped", version: 1, isDefault: false},
    ],
});

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi(makeState({admin: true}));
    api.FeaturesList.mockImplementation(async () => features());
    api.WSLInfo.mockImplementation(async () => wslInfo());
});
afterEach(() => setLang("en"));

function page(o: { admin?: boolean } = {}) {
    const onRun = vi.fn();
    const onAdmin = vi.fn();
    const say = vi.fn();
    const user = userEvent.setup();
    const view = render(<Developer admin={o.admin ?? true} say={say} onAdmin={onAdmin} onRun={onRun}/>);
    return {user, onRun, onAdmin, say, ...view};
}
const scan = async (user: ReturnType<typeof userEvent.setup>) => {
    await user.click(screen.getByRole("button", {name: /^Scan/}));
    await screen.findByRole("heading", {name: /Programs and where they run from/});
};

describe("developer: helpers", () => {
    it("groups known kinds in a fixed order and keeps unknown kinds last", () => {
        const g = groupFindings([finding({id: "a", kind: "zzz"}), finding({id: "b", kind: "tool-shadowed"}), finding({id: "c", kind: "path-missing"}), finding({id: "d", kind: "path-missing"})]);
        expect(g.map((x) => x.kind)).toEqual(["path-missing", "tool-shadowed", "zzz"]);
        expect(g[0].items).toHaveLength(2);
        expect(g.map((x) => x.known)).toEqual([true, true, false]);
    });
    it("only lets fixable user findings be selected", () => {
        expect(selectable(finding({id: "a", kind: "path-missing"}))).toBe(true);
        expect(selectable(finding({id: "a", kind: "path-missing", scope: "machine"}))).toBe(false);
        expect(selectable(finding({id: "a", kind: "path-missing", fixable: false}))).toBe(false);
        expect(selectable(finding({id: "a", kind: "path-missing", fixKind: ""}))).toBe(false);
    });
    it("has the same keys in English and Spanish, none empty", () => {
        const dev = KEYS.filter((k) => k.startsWith("dev.") || k.startsWith("repair.feature-") || k === "nav.developer");
        expect(dev.length).toBeGreaterThan(80);
        for (const k of dev) {
            expect(DICTS.es[k], k).toBeTruthy();
            expect(DICTS.en[k], k).toBeTruthy();
        }
    });
});

describe("developer: environment doctor", () => {
    it("does not scan by itself and says that scanning only reads", async () => {
        page();
        expect(screen.getByText(/Scanning only reads: it changes nothing/)).toBeInTheDocument();
        expect(api.EnvScan).not.toHaveBeenCalled();
    });

    it("renders one sentence for every kind, groups them, and counts problems and notes", async () => {
        api.EnvScan.mockResolvedValue(report({findings: everyKind(), userPath: ["a", "b"], machinePath: ["c"]}));
        const {user} = page();
        await scan(user);
        expect(screen.getByText("9 to fix, 4 to know about")).toBeInTheDocument();
        expect(screen.getByText("User PATH: 2 entries. System PATH: 1 entries.")).toBeInTheDocument();
        for (const text of [
            "The folder C:\\gone is in your user PATH but does not exist.",
            "C:\\Windows\\notepad.exe is in your user PATH but it is a file, not a folder.",
            "C:\\dup appears more than once in your user PATH.",
            "your user PATH has an empty entry.",
            "bin in your user PATH is a relative path, so it points somewhere different in every folder.",
            "your user PATH is very long, and some programs ignore the end of it.",
            "JAVA_HOME points to a folder that does not exist.",
            "GOROOT points to a folder that does not look like an install of that tool.",
            "PYTHONHOME points to a different install than the one that runs from the command line.",
            "node is installed more than once. The first one on PATH is the one that runs.",
            "python on PATH is a Microsoft Store shortcut, not a real install.",
            "The folder C:\\machine-gone is in the system PATH but does not exist.",
        ]) expect(screen.getByText(text), text).toBeInTheDocument();
        for (const h of ["PATH folders that do not exist", "PATH entries that are files", "Repeated PATH entries", "Empty PATH entries", "Relative PATH entries", "PATH too long",
            "Variables pointing to a missing folder", "Variables pointing to the wrong folder", "Variables that disagree with the tool that runs", "Programs installed more than once",
            "Microsoft Store shortcuts hiding a program", "Other findings"]) expect(screen.getByRole("heading", {name: new RegExp(h)}), h).toBeInTheDocument();
        // every install of a shadowed tool is shown as text
        expect(screen.getByText("C:\\a\\node.exe")).toBeInTheDocument();
        expect(screen.getByText("C:\\b\\node.exe")).toBeInTheDocument();
    });

    it("still shows a kind it does not know, with its raw name", async () => {
        api.EnvScan.mockResolvedValue(report({findings: [finding({id: "x", kind: "future-kind", subject: "thing", fixable: false, fixKind: ""})]}));
        const {user} = page();
        await scan(user);
        expect(screen.getByText("future-kind: thing")).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: /Other findings/})).toBeInTheDocument();
    });

    it("says all is good when there are no findings", async () => {
        const {user} = page();
        await scan(user);
        expect(screen.getByText(/All good\. Nothing is wrong/)).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: /Fix selected/})).toBeNull();
    });

    it("shows a scan error as a message, and can scan again", async () => {
        api.EnvScan.mockRejectedValueOnce(new Error("registry unreadable"));
        const {user} = page();
        await user.click(screen.getByRole("button", {name: "Scan"}));
        expect(await screen.findByRole("alert")).toHaveTextContent("The scan failed: registry unreadable");
        await user.click(screen.getByRole("button", {name: "Scan"}));
        expect(await screen.findByText(/All good/)).toBeInTheDocument();
    });

    it("lists tools with the winner and the hidden installs, and explains a Store alias", async () => {
        api.EnvScan.mockResolvedValue(report({
            tools: [
                {name: "node", found: ["C:\\a\\node.exe", "C:\\b\\node.exe"]},
                {name: "python", found: ["C:\\Users\\me\\AppData\\Local\\Microsoft\\WindowsApps\\python.exe"]},
                {name: "go", found: []},
            ],
            findings: [finding({id: "t2", kind: "tool-store-alias", subject: "python", fixable: false, fixKind: ""})],
        }));
        const {user} = page();
        await scan(user);
        const table = screen.getByRole("table");
        const node = within(table).getByRole("row", {name: /node/});
        expect(within(node).getByText("C:\\a\\node.exe")).toBeInTheDocument();
        expect(within(node).getByText("C:\\b\\node.exe")).toBeInTheDocument();
        expect(within(within(table).getByRole("row", {name: /python/})).getByText("Microsoft Store shortcut")).toBeInTheDocument();
        expect(within(within(table).getByRole("row", {name: /^go/})).getByText("Not found")).toBeInTheDocument();
        expect(screen.getByText(/It only opens the Microsoft Store/)).toBeInTheDocument();
    });

    it("lists the variables in a collapsible block", async () => {
        api.EnvScan.mockResolvedValue(report({vars: [{name: "JAVA_HOME", scope: "user", value: "C:\\jdk"}]}));
        const {user} = page();
        await scan(user);
        expect(screen.getByText("Variables (1)")).toBeInTheDocument();
        expect(screen.getByText("C:\\jdk")).toBeInTheDocument();
    });

    it("shows a first page of a huge group and the rest on demand", async () => {
        const many = Array.from({length: 40}, (_, i) => finding({id: `p${i}`, kind: "path-missing", subject: `C:\\gone${i}`}));
        api.EnvScan.mockResolvedValue(report({findings: many}));
        const {user} = page();
        await scan(user);
        expect(screen.getAllByRole("checkbox")).toHaveLength(15);
        await user.click(screen.getByRole("button", {name: "Show more (25)"}));
        expect(screen.getAllByRole("checkbox")).toHaveLength(30);
        await user.click(screen.getByRole("button", {name: "Show more (10)"}));
        expect(screen.getAllByRole("checkbox")).toHaveLength(40);
        expect(screen.queryByRole("button", {name: /Show more/})).toBeNull();
    });

    it("makes only user fixable findings selectable, none ticked, and machine ones say they need administrator", async () => {
        api.EnvScan.mockResolvedValue(report({findings: everyKind()}));
        const {user} = page();
        await scan(user);
        const boxes = screen.getAllByRole("checkbox") as HTMLInputElement[];
        expect(boxes).toHaveLength(8); // u1-u5 and u7-u9; not the note-only u6, the tools, the machine or the unknown one
        expect(boxes.every((b) => !b.checked)).toBe(true);
        const machine = screen.getByText("The folder C:\\machine-gone is in the system PATH but does not exist.").closest("li")!;
        expect(within(machine).queryByRole("checkbox")).toBeNull();
        expect(within(machine).getByText("Needs administrator. WinForge only reports it.")).toBeInTheDocument();
    });

    it("sends exactly the ticked ids after a confirmation that lists what changes", async () => {
        api.EnvScan.mockResolvedValue(report({findings: everyKind()}));
        api.EnvFix.mockResolvedValue({applied: 2, failed: [], backupId: "b1"});
        const {user} = page();
        await scan(user);
        expect(screen.getByRole("button", {name: "Fix selected (0)"})).toBeDisabled();
        await user.click(screen.getByRole("checkbox", {name: "Fix: The folder C:\\gone is in your user PATH but does not exist."}));
        await user.click(screen.getByRole("checkbox", {name: "Fix: JAVA_HOME points to a folder that does not exist."}));
        await user.click(screen.getByRole("button", {name: "Fix selected (2)"}));
        const dialog = await screen.findByRole("alertdialog", {name: "Fix the selected items (2)?"});
        expect(within(dialog).getByText(/Only your own user variables change/)).toBeInTheDocument();
        expect(within(dialog).getByText(/A backup is kept first/)).toBeInTheDocument();
        expect(within(dialog).getByText(/New terminals see the change/)).toBeInTheDocument();
        expect(within(dialog).getByText("Remove C:\\gone from your user PATH")).toBeInTheDocument();
        expect(within(dialog).getByText("Delete your user variable JAVA_HOME")).toBeInTheDocument();
        await user.click(within(dialog).getByRole("button", {name: "Fix"}));
        await waitFor(() => expect(api.EnvFix).toHaveBeenCalledTimes(1));
        expect(api.EnvFix).toHaveBeenCalledWith(["u1", "u7"]);
        expect(await screen.findByText("Fixed 2. A backup was saved.")).toBeInTheDocument();
        await waitFor(() => expect(api.EnvScan).toHaveBeenCalledTimes(2)); // read again after the fix
    });

    it("does nothing when the confirmation is cancelled, and can be opened by keyboard", async () => {
        api.EnvScan.mockResolvedValue(report({findings: everyKind()}));
        const {user} = page();
        await scan(user);
        screen.getByRole("checkbox", {name: /JAVA_HOME/}).focus();
        await user.keyboard(" ");
        expect(screen.getByRole("checkbox", {name: /JAVA_HOME/})).toBeChecked();
        screen.getByRole("button", {name: "Fix selected (1)"}).focus();
        await user.keyboard("{Enter}");
        const dialog = await screen.findByRole("alertdialog");
        await user.click(within(dialog).getByRole("button", {name: "Cancel"}));
        expect(api.EnvFix).not.toHaveBeenCalled();
        expect(screen.queryByRole("alertdialog")).toBeNull();
        expect(screen.getByRole("checkbox", {name: /JAVA_HOME/})).toBeChecked();
    });

    it("lists the entries that could not be changed", async () => {
        api.EnvScan.mockResolvedValue(report({findings: [finding({id: "u1", kind: "path-missing", subject: "C:\\gone"})]}));
        api.EnvFix.mockResolvedValue({applied: 0, failed: ["u1"], backupId: ""});
        const {user} = page();
        await scan(user);
        await user.click(screen.getByRole("checkbox"));
        await user.click(screen.getByRole("button", {name: "Fix selected (1)"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Fix"}));
        expect(await screen.findByText("Nothing was changed.")).toBeInTheDocument();
        expect(screen.getByText("Could not change: u1")).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: "Undo"})).toBeNull();
    });

    it("undoes a fix by restoring its backup, then reads again", async () => {
        api.EnvScan.mockResolvedValue(report({findings: [finding({id: "u1", kind: "path-missing", subject: "C:\\gone"})]}));
        api.EnvFix.mockResolvedValue({applied: 1, failed: [], backupId: "b9"});
        const {user} = page();
        await scan(user);
        await user.click(screen.getByRole("checkbox"));
        await user.click(screen.getByRole("button", {name: "Fix selected (1)"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Fix"}));
        await user.click(await screen.findByRole("button", {name: "Undo"}));
        await waitFor(() => expect(api.EnvRestore).toHaveBeenCalledWith("b9"));
        expect(await screen.findByText(/Your earlier values are back/)).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: "Undo"})).toBeNull();
        await waitFor(() => expect(api.EnvScan).toHaveBeenCalledTimes(3));
    });

    it("shows a failed fix as a message", async () => {
        api.EnvScan.mockResolvedValue(report({findings: [finding({id: "u1", kind: "path-missing", subject: "C:\\gone"})]}));
        api.EnvFix.mockRejectedValue(new Error("not in the last scan"));
        const {user, say} = page();
        await scan(user);
        await user.click(screen.getByRole("checkbox"));
        await user.click(screen.getByRole("button", {name: "Fix selected (1)"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Fix"}));
        await waitFor(() => expect(say).toHaveBeenCalledWith("not in the last scan"));
    });

    it("lists past backups and restores one only after asking", async () => {
        api.EnvBackups.mockResolvedValue([
            {id: "old1", time: "not a date", changed: ["PATH", "JAVA_HOME"]},
            {id: "old2", time: "2026-11-01T10:00:00Z", changed: ["GOROOT"]},
        ]);
        const {user} = page();
        expect(await screen.findByText("not a date: PATH, JAVA_HOME")).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Restore the backup of not a date"}));
        const dialog = await screen.findByRole("alertdialog", {name: "Restore this backup?"});
        await user.click(within(dialog).getByRole("button", {name: "Cancel"}));
        expect(api.EnvRestore).not.toHaveBeenCalled();
        await user.click(screen.getByRole("button", {name: "Restore the backup of not a date"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Restore"}));
        await waitFor(() => expect(api.EnvRestore).toHaveBeenCalledWith("old1"));
    });

    it("says there are no backups yet", async () => {
        page();
        expect(await screen.findByText(/No backups yet/)).toBeInTheDocument();
    });
});

describe("developer: Windows features and WSL", () => {
    it("has a card per feature with its state, what it is for and the restart note", async () => {
        page();
        const list = await screen.findByRole("list", {name: "Windows features"});
        const cards = within(list).getAllByRole("listitem");
        expect(cards).toHaveLength(5);
        expect(within(cards[0]).getByText("On")).toBeInTheDocument();
        expect(within(cards[0]).getByText(/Runs Linux distributions inside Windows/)).toBeInTheDocument();
        expect(within(cards[1]).getByText("Off")).toBeInTheDocument();
        expect(within(cards[1]).getByText(/Restart pending/)).toBeInTheDocument();
        expect(within(cards[2]).getByText("Not available on this edition")).toBeInTheDocument();
        expect(within(cards[3]).getByText("Unknown")).toBeInTheDocument();
        expect(screen.getAllByText(/Restart pending/)).toHaveLength(1);
    });

    it("turns a feature on with its recipe id and no question", async () => {
        const {user, onRun} = page();
        await user.click(await screen.findByRole("button", {name: "Turn on: Virtual Machine Platform"}));
        expect(onRun).toHaveBeenCalledWith(["feature-vmp-on"]);
        expect(screen.queryByRole("alertdialog")).toBeNull();
    });

    it("asks before turning a feature off, and does nothing on Cancel", async () => {
        const {user, onRun} = page();
        await user.click(await screen.findByRole("button", {name: "Turn off: Windows Subsystem for Linux"}));
        const dialog = await screen.findByRole("alertdialog", {name: "Turn off Windows Subsystem for Linux?"});
        await user.click(within(dialog).getByRole("button", {name: "Cancel"}));
        expect(onRun).not.toHaveBeenCalled();
        await user.click(screen.getByRole("button", {name: "Turn off: Windows Subsystem for Linux"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Turn off"}));
        expect(onRun).toHaveBeenCalledWith(["feature-wsl-off"]);
    });

    it("disables the button for features that are unavailable or unknown", async () => {
        const {user, onRun} = page();
        const hyperv = await screen.findByRole("button", {name: "Turn on: Hyper-V"});
        const sandbox = screen.getByRole("button", {name: "Turn on: Windows Sandbox"});
        expect(hyperv).toBeDisabled();
        expect(sandbox).toBeDisabled();
        await user.click(hyperv);
        expect(onRun).not.toHaveBeenCalled();
    });

    it("hints about administrator rights when not admin, and offers the restart", async () => {
        const {user, onAdmin} = page({admin: false});
        expect(await screen.findByText("Changing Windows features needs administrator rights.")).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Restart as administrator"}));
        expect(onAdmin).toHaveBeenCalled();
    });

    it("has no administrator hint when already admin", async () => {
        page({admin: true});
        await screen.findByRole("list", {name: "Windows features"});
        expect(screen.queryByText(/needs administrator rights/)).toBeNull();
    });

    it("shows the WSL default version and its distributions", async () => {
        page();
        expect(await screen.findByText("WSL is installed. New distributions use WSL 2 by default.")).toBeInTheDocument();
        const list = screen.getByRole("list", {name: "Linux distributions"});
        const ubuntu = within(list).getAllByRole("listitem")[0];
        expect(within(ubuntu).getByText("Ubuntu")).toBeInTheDocument();
        expect(within(ubuntu).getByText("Default")).toBeInTheDocument();
        expect(within(ubuntu).getByText("Running")).toBeInTheDocument();
        expect(within(ubuntu).getByText("WSL 2")).toBeInTheDocument();
        const debian = within(list).getAllByRole("listitem")[1];
        expect(within(debian).getByText("Stopped")).toBeInTheDocument();
        expect(within(debian).queryByText("Default")).toBeNull();
    });

    it("says when WSL is not installed or has no distributions", async () => {
        api.WSLInfo.mockResolvedValue({installed: false, defaultVersion: 0, distros: []});
        const {unmount} = page();
        expect(await screen.findByText(/WSL is not installed/)).toBeInTheDocument();
        unmount();
        api.WSLInfo.mockResolvedValue({installed: true, defaultVersion: 2, distros: []});
        page();
        expect(await screen.findByText("No Linux distributions are installed.")).toBeInTheDocument();
    });

    it("shows read errors as messages without hiding the rest", async () => {
        api.FeaturesList.mockRejectedValue(new Error("dism failed"));
        api.WSLInfo.mockRejectedValue(new Error("wsl.exe missing"));
        page();
        expect(await screen.findByText("Could not read the Windows features: dism failed")).toBeInTheDocument();
        expect(await screen.findByText("Could not read WSL: wsl.exe missing")).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "Environment doctor"})).toBeInTheDocument();
    });

    it("reads again when Refresh is pressed and when the refresh key changes", async () => {
        const {user, rerender, say, onRun, onAdmin} = page();
        await screen.findByRole("list", {name: "Windows features"});
        await user.click(screen.getByRole("button", {name: "Refresh"}));
        await waitFor(() => expect(api.FeaturesList).toHaveBeenCalledTimes(2));
        rerender(<Developer admin say={say} onAdmin={onAdmin} onRun={onRun} refreshKey={1}/>);
        await waitFor(() => expect(api.FeaturesList).toHaveBeenCalledTimes(3));
    });

    it("names a feature this version does not know by its id", async () => {
        api.FeaturesList.mockResolvedValue([feat("wsl", {state: "off"}), {...feat("wsl"), id: "newthing" as Feature["id"], enable: "feature-newthing-on"}]);
        page();
        expect(await screen.findByRole("button", {name: "Turn on: newthing"})).toBeInTheDocument();
    });
});

describe("developer: in the app", () => {
    async function openTab(lang?: "es") {
        if (lang) localStorage.setItem("winforge.settings", JSON.stringify({language: lang}));
        const user = userEvent.setup();
        render(<App/>);
        await screen.findByText(lang ? /apps del catálogo/ : /catalog apps found/);
        const nav = screen.getByRole("navigation", {name: lang ? "Menú principal" : "Main menu"});
        await user.click(within(nav).getByRole("button", {name: lang ? /Desarrollo/ : /Developer/}));
        await screen.findByRole("heading", {name: lang ? "Desarrollo" : "Developer", level: 1});
        return user;
    }

    it("has a Developer tab after Startup that opens the page", async () => {
        const user = userEvent.setup();
        render(<App/>);
        await screen.findByText(/catalog apps found/);
        const names = within(screen.getByRole("navigation", {name: "Main menu"})).getAllByRole("button").map((b) => b.textContent ?? "");
        const startup = names.findIndex((n) => /Startup/.test(n));
        expect(names.findIndex((n) => /Developer/.test(n))).toBe(startup + 1);
        await user.click(within(screen.getByRole("navigation", {name: "Main menu"})).getByRole("button", {name: /Developer/}));
        expect(await screen.findByRole("heading", {name: "Environment doctor"})).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "Windows features and WSL"})).toBeInTheDocument();
    });

    it("runs a feature through the repair runner with the run title and a named step, then reads the features again", async () => {
        api.PlanRepairs.mockImplementation(async (ids: string[]) => ({steps: ids.map((id) => ({kind: "recipe", id, name: "raw name", admin: true})), alreadyInstalled: [], needsAdmin: true}));
        const user = await openTab();
        await user.click(await screen.findByRole("button", {name: "Turn on: Virtual Machine Platform"}));
        await waitFor(() => expect(api.PlanRepairs).toHaveBeenCalledWith(["feature-vmp-on"]));
        const run = await screen.findByRole("dialog", {name: "Windows features"});
        expect(within(run).getByText("Turn on Virtual Machine Platform")).toBeInTheDocument();
        expect(api.FeaturesList).toHaveBeenCalledTimes(1);
        expect(api.HealthScan).not.toHaveBeenCalled();
    });

    it("is in Spanish: tab, sentences, features and run step names", async () => {
        api.EnvScan.mockResolvedValue(report({findings: [finding({id: "u7", kind: "home-missing", subject: "JAVA_HOME", fixKind: "unset-variable"})]}));
        const user = await openTab("es");
        expect(screen.getByRole("heading", {name: "Doctor del entorno"})).toBeInTheDocument();
        expect(await screen.findByRole("button", {name: "Activar: Plataforma de máquina virtual"})).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Desactivar: Subsistema de Windows para Linux"})).toBeInTheDocument();
        expect(screen.getByText("No disponible en esta edición")).toBeInTheDocument();
        expect(screen.getByText("WSL está instalado. Las distribuciones nuevas usan WSL 2 por defecto.")).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Escanear"}));
        expect(await screen.findByText("JAVA_HOME apunta a una carpeta que no existe.")).toBeInTheDocument();
        await user.click(screen.getByRole("checkbox", {name: /Arreglar: JAVA_HOME/}));
        await user.click(screen.getByRole("button", {name: "Arreglar lo marcado (1)"}));
        const dialog = await screen.findByRole("alertdialog", {name: "¿Arreglar los elementos marcados (1)?"});
        expect(within(dialog).getByText("Borrar tu variable de usuario JAVA_HOME")).toBeInTheDocument();
    });
});
