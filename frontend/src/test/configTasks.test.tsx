import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {EnvFinding, PrivacyChange, PrivacyTweak, ServiceItem, StartupItem, TaskItem} from "../api";
import App from "../App";
import {DICTS} from "../lib/i18n";
import {tasksConfigEn, tasksConfigEs} from "../lib/i18n.tasks.config";
import {api, resetApi} from "./apiMock";
import {makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

// Privacy, Developer and Startup register their long operations in the task store, so leaving the page and
// coming back still shows the job running (and the controls locked), and then how it ended.

function pending<T>() {
    let resolve!: (v: T) => void;
    let reject!: (e: unknown) => void;
    const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
    return {promise, resolve, reject};
}

const nav = () => within(screen.getByRole("navigation", {name: "Main menu"}));

async function open(label: RegExp, heading: string) {
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(/catalog apps found/);
    await user.click(nav().getByRole("button", {name: label}));
    await screen.findByRole("heading", {name: heading, level: 1});
    return user;
}

/** Leaves for Home and comes back, as a person changing tab would. */
async function leaveAndReturn(user: ReturnType<typeof userEvent.setup>, label: RegExp, heading: string) {
    await user.click(nav().getByRole("button", {name: /Home/}));
    await waitFor(() => expect(screen.queryByRole("heading", {name: heading, level: 1})).not.toBeInTheDocument());
    await user.click(nav().getByRole("button", {name: label}));
    await screen.findByRole("heading", {name: heading, level: 1});
}

const taskList = () => screen.getByRole("heading", {name: "Tasks", level: 2}).closest("section") as HTMLElement;

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi(makeState({admin: true}));
});

describe("task keys", () => {
    it("has the same keys in Spanish and English", () => {
        expect(Object.keys(tasksConfigEs).sort()).toEqual(Object.keys(tasksConfigEn).sort());
        for (const k of Object.keys(tasksConfigEn)) expect(DICTS.es[k as keyof typeof DICTS.es]).toBeTruthy();
    });
});

describe("privacy: jobs in the Tasks list", () => {
    const change = (name: string): PrivacyChange => ({path: "HKEY_CURRENT_USER\\Software\\X", name, existed: true, oldType: "REG_DWORD", old: "1", type: "REG_DWORD", private: 0, error: false});
    const tweaks = (): PrivacyTweak[] => [
        {id: "advertising-id", area: "ads", nameKey: "privacy.t.advertising-id.name", whyKey: "privacy.t.advertising-id.why", state: "default", changes: [change("Enabled")]},
    ];
    const apply = async (user: ReturnType<typeof userEvent.setup>) => {
        await user.click(await screen.findByRole("checkbox", {name: "Make private: Advertising ID for apps"}));
        await user.click(screen.getByRole("button", {name: "Apply selected (1)"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Apply"}));
    };

    beforeEach(() => {
        api.PrivacyList.mockImplementation(async () => tweaks());
        api.PrivacyBackups.mockResolvedValue([{id: "b1", time: "2026-11-17T10:00:00Z", action: "apply", tweaks: ["advertising-id"], restored: null}]);
    });

    it("keeps applying in view after leaving the page and coming back, then lists how it ended", async () => {
        const job = pending<{ applied: number; skipped: string[]; failed: string[]; backupId: string }>();
        api.PrivacyApply.mockReturnValue(job.promise);
        const user = await open(/Privacy/, "Privacy");
        await apply(user);
        await waitFor(() => expect(api.PrivacyApply).toHaveBeenCalledTimes(1));
        await leaveAndReturn(user, /Privacy/, "Privacy");
        expect(within(taskList()).getByText("Applying 1 privacy settings")).toBeInTheDocument();
        expect(within(taskList()).getByText("Running")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Refresh"})).toBeDisabled();
        expect(await screen.findByRole("button", {name: /^Restore the backup of/})).toBeDisabled();
        expect(api.PrivacyApply).toHaveBeenCalledTimes(1);
        const lists = api.PrivacyList.mock.calls.length;

        job.resolve({applied: 1, skipped: [], failed: [], backupId: "b1"});
        await waitFor(() => expect(within(taskList()).getAllByText("Done").length).toBeGreaterThan(0));
        expect(within(taskList()).getByText(/Changed 1\. A backup was saved\./)).toBeInTheDocument();
        expect(within(taskList()).queryByRole("button", {name: /^Stop/})).not.toBeInTheDocument();
        // The page that was shown again reads the settings again, since the job ended while it was away.
        await waitFor(() => expect(api.PrivacyList.mock.calls.length).toBeGreaterThan(lists));
        expect(screen.getByRole("button", {name: "Refresh"})).toBeEnabled();
        expect(api.PrivacyApply).toHaveBeenCalledTimes(1);
    });

    it("lists a failed apply with its error", async () => {
        api.PrivacyApply.mockRejectedValue(new Error("registry is locked"));
        const user = await open(/Privacy/, "Privacy");
        await apply(user);
        await waitFor(() => expect(within(taskList()).getByText("Failed")).toBeInTheDocument());
        expect(within(taskList()).getByText("registry is locked")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Refresh"})).toBeEnabled();
    });
});

describe("developer: jobs in the Tasks list", () => {
    const finding: EnvFinding = {id: "u1", kind: "path-missing", severity: "problem", scope: "user", subject: "C:\\gone", paths: [], fixable: true, fixKind: "remove-path-entry"};

    it("keeps a running scan in view after leaving the page and coming back, and shows its result", async () => {
        const job = pending<{ findings: EnvFinding[]; userPath: string[]; machinePath: string[]; tools: []; vars: [] }>();
        api.EnvScan.mockReturnValue(job.promise);
        api.FeaturesList.mockResolvedValue([]);
        api.WSLInfo.mockResolvedValue({installed: false, defaultVersion: 0, distros: []});
        const user = await open(/Developer/, "Developer");
        await user.click(screen.getByRole("button", {name: /^Scan/}));
        await waitFor(() => expect(api.EnvScan).toHaveBeenCalledTimes(1));
        await leaveAndReturn(user, /Developer/, "Developer");
        expect(within(taskList()).getByText("Scanning PATH and tool variables")).toBeInTheDocument();
        expect(within(taskList()).getByText("Running")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: /^Scan/})).toBeDisabled();
        expect(api.EnvScan).toHaveBeenCalledTimes(1);

        job.resolve({findings: [finding], userPath: [], machinePath: [], tools: [], vars: []});
        await waitFor(() => expect(within(taskList()).getAllByText("Done").length).toBeGreaterThan(0));
        expect(within(taskList()).getByText("1 to fix, 0 to know about")).toBeInTheDocument();
        expect(await screen.findByRole("button", {name: "Scan again"})).toBeEnabled();
        // The scan finished while the page was away, and its result is still there.
        expect(await screen.findByRole("heading", {name: /Programs and where they run from/})).toBeInTheDocument();
        expect(api.EnvScan).toHaveBeenCalledTimes(1);
    });

    it("lists a failed scan with its error", async () => {
        api.EnvScan.mockRejectedValue(new Error("powershell missing"));
        api.FeaturesList.mockResolvedValue([]);
        api.WSLInfo.mockResolvedValue({installed: false, defaultVersion: 0, distros: []});
        const user = await open(/Developer/, "Developer");
        await user.click(screen.getByRole("button", {name: /^Scan/}));
        await waitFor(() => expect(within(taskList()).getByText("Failed")).toBeInTheDocument());
        expect(within(taskList()).getByText("powershell missing")).toBeInTheDocument();
    });
});

describe("startup: jobs in the Tasks list", () => {
    const prog = (name: string): StartupItem => ({
        id: `hkcu-run|${name}`, name, source: "hkcu-run", command: `C:\\${name}.exe`, target: `C:\\${name}.exe`, publisher: "", enabled: true, needsAdmin: false, missing: false, caution: false,
    });
    const svc = (name: string): ServiceItem => ({
        id: name, name, displayName: name, publisher: "", state: "stopped", startMode: "manual", binary: `C:\\${name}.exe`, needsAdmin: true, caution: false, dependents: [], changed: false, original: "",
    });
    const sched = (name: string): TaskItem => ({
        id: `\\${name}`, name, folder: "\\", triggers: ["logon"], action: `C:\\${name}.exe`, state: "ready", enabled: true, lastResult: 0, needsAdmin: false, caution: false, changed: false, original: "",
    });

    beforeEach(() => {
        api.StartupList.mockImplementation(async () => [prog("Discord"), prog("Steam")]);
        api.ServicesList.mockImplementation(async () => [svc("Agent")]);
        api.TasksList.mockImplementation(async () => [sched("MyBackup")]);
    });

    it("keeps a program switch in view after leaving the page and coming back, and locks the switches", async () => {
        const job = pending<StartupItem[]>();
        api.StartupSet.mockReturnValue(job.promise);
        const user = await open(/Startup/, "Startup");
        await user.click(await screen.findByRole("checkbox", {name: "Start with Windows: Discord"}));
        await waitFor(() => expect(api.StartupSet).toHaveBeenCalledTimes(1));
        await leaveAndReturn(user, /Startup/, "Startup");
        expect(within(taskList()).getByText("Turning off at startup: Discord")).toBeInTheDocument();
        expect(within(taskList()).getByText("Running")).toBeInTheDocument();
        expect(await screen.findByRole("checkbox", {name: "Start with Windows: Steam"})).toBeDisabled();
        expect(screen.getByRole("button", {name: "Refresh"})).toBeDisabled();
        expect(api.StartupSet).toHaveBeenCalledTimes(1);
        const reads = api.StartupList.mock.calls.length;

        job.resolve([{...prog("Discord"), enabled: false}, prog("Steam")]);
        await waitFor(() => expect(within(taskList()).getAllByText("Done").length).toBeGreaterThan(0));
        expect(within(taskList()).getByText("Discord will no longer start with Windows.")).toBeInTheDocument();
        await waitFor(() => expect(api.StartupList.mock.calls.length).toBeGreaterThan(reads)); // read again: it ended while away
        await waitFor(() => expect(screen.getByRole("checkbox", {name: "Start with Windows: Steam"})).toBeEnabled());
        expect(api.StartupSet).toHaveBeenCalledTimes(1);
    });

    it("lists a failed program switch with its error, and unlocks the switches", async () => {
        api.StartupSet.mockRejectedValue(new Error("registry is locked"));
        const user = await open(/Startup/, "Startup");
        await user.click(await screen.findByRole("checkbox", {name: "Start with Windows: Discord"}));
        await waitFor(() => expect(within(taskList()).getByText("Failed")).toBeInTheDocument());
        expect(within(taskList()).getByText("registry is locked")).toBeInTheDocument();
        expect(screen.getByRole("checkbox", {name: "Start with Windows: Steam"})).toBeEnabled();
    });

    it("keeps a service change in view after leaving the page and coming back", async () => {
        const job = pending<ServiceItem[]>();
        api.ServicesSetMode.mockReturnValue(job.promise);
        const user = await open(/Startup/, "Startup");
        await user.click(screen.getByRole("tab", {name: "Services"}));
        await user.selectOptions(await screen.findByRole("combobox", {name: "Start mode: Agent"}), "disabled");
        await waitFor(() => expect(api.ServicesSetMode).toHaveBeenCalledTimes(1));
        await leaveAndReturn(user, /Startup/, "Startup");
        await user.click(screen.getByRole("tab", {name: "Services"}));
        expect(within(taskList()).getByText(/^Setting Agent to start /)).toBeInTheDocument();
        expect(within(taskList()).getByText("Running")).toBeInTheDocument();
        expect(await screen.findByRole("combobox", {name: "Start mode: Agent"})).toBeDisabled();
        expect(api.ServicesSetMode).toHaveBeenCalledTimes(1);

        job.resolve([{...svc("Agent"), startMode: "disabled"}]);
        await waitFor(() => expect(within(taskList()).getAllByText("Done").length).toBeGreaterThan(0));
        expect(within(taskList()).getByText(/^Agent now starts: /)).toBeInTheDocument();
        await waitFor(() => expect(screen.getByRole("combobox", {name: "Start mode: Agent"})).toBeEnabled());
    });

    it("lists a failed service change with its sentence", async () => {
        api.ServicesSetMode.mockRejectedValue(new Error("svc:needs-admin"));
        const user = await open(/Startup/, "Startup");
        await user.click(screen.getByRole("tab", {name: "Services"}));
        await user.selectOptions(await screen.findByRole("combobox", {name: "Start mode: Agent"}), "disabled");
        await waitFor(() => expect(within(taskList()).getByText("Failed")).toBeInTheDocument());
        expect(within(taskList()).getByText(/needs administrator rights/)).toBeInTheDocument();
    });

    it("keeps a scheduled task change in view after leaving the page and coming back", async () => {
        const job = pending<TaskItem[]>();
        api.TasksSetEnabled.mockReturnValue(job.promise);
        const user = await open(/Startup/, "Startup");
        await user.click(screen.getByRole("tab", {name: "Scheduled tasks"}));
        await user.click(await screen.findByRole("checkbox", {name: "Scheduled task enabled: MyBackup"}));
        await waitFor(() => expect(api.TasksSetEnabled).toHaveBeenCalledTimes(1));
        await leaveAndReturn(user, /Startup/, "Startup");
        await user.click(screen.getByRole("tab", {name: "Scheduled tasks"}));
        expect(within(taskList()).getByText("Turning off the scheduled task MyBackup")).toBeInTheDocument();
        expect(await screen.findByRole("checkbox", {name: "Scheduled task enabled: MyBackup"})).toBeDisabled();
        expect(api.TasksSetEnabled).toHaveBeenCalledTimes(1);

        job.resolve([{...sched("MyBackup"), enabled: false, state: "disabled"}]);
        await waitFor(() => expect(within(taskList()).getAllByText("Done").length).toBeGreaterThan(0));
        expect(within(taskList()).getByText("MyBackup is off.")).toBeInTheDocument();
        await waitFor(() => expect(screen.getByRole("checkbox", {name: "Scheduled task enabled: MyBackup"})).toBeEnabled());
    });
});
