import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {ServiceItem, TaskItem} from "../api";
import App from "../App";
import {svcEn, svcEs} from "../lib/i18n.svc";
import {lowersStart, visibleServices} from "../views/startup/Services";
import {resultText, visibleTasks} from "../views/startup/Tasks";
import {svcError} from "../views/startup/svcErrors";
import {api, resetApi} from "./apiMock";
import {makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const svc = (name: string, o: Partial<ServiceItem> = {}): ServiceItem => ({
    id: name, name, displayName: name, publisher: "", state: "stopped", startMode: "manual", binary: `C:\\Program Files\\${name}\\${name}.exe`,
    needsAdmin: true, caution: false, dependents: [], changed: false, original: "", ...o,
});
const services = (): ServiceItem[] => [
    svc("Docker Desktop Service", {id: "com.docker.service", name: "com.docker.service", publisher: "Docker Inc.", state: "running", startMode: "auto"}),
    svc("Steam Client Service", {publisher: "Valve Corporation", startMode: "delayed"}),
    svc("Kaspersky Service", {id: "AVP21", name: "AVP21", caution: true, state: "running", startMode: "auto"}),
    svc("Vendor Parent", {id: "Parent", name: "Parent", startMode: "auto", dependents: ["Vendor Child", "Vendor Other"]}),
    svc("Old Agent", {startMode: "disabled", changed: true, original: "auto"}),
];
const setMode = (items: ServiceItem[], id: string, startMode: ServiceItem["startMode"]) => items.map((i) => (i.id === id ? {...i, startMode} : i));

const task = (name: string, o: Partial<TaskItem> = {}): TaskItem => ({
    id: `\\${name}`, name, folder: "\\", triggers: ["logon"], action: `C:\\Program Files\\${name}\\${name}.exe`, state: "ready", enabled: true, lastResult: 0,
    needsAdmin: true, caution: false, changed: false, original: "", ...o,
});
const tasks = (): TaskItem[] => [
    task("GoogleUpdateTaskMachineCore", {triggers: ["logon", "boot"], lastResult: -2147024894}),
    task("MyBackup", {needsAdmin: false, lastResult: null}),
    task("OldSync", {state: "disabled", enabled: false, changed: true, original: "enabled", folder: "\\Acme\\"}),
    task("Avast Update", {caution: true, triggers: ["boot"]}),
];
const setEnabled = (items: TaskItem[], id: string, enabled: boolean) => items.map((i) => (i.id === id ? {...i, enabled, state: enabled ? "ready" as const : "disabled" as const} : i));

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi(makeState({admin: true}));
    api.ServicesList.mockImplementation(async () => services());
    api.TasksList.mockImplementation(async () => tasks());
});

async function open(o: { admin?: boolean; lang?: "es"; tab?: "services" | "tasks" } = {}) {
    if (o.lang) localStorage.setItem("winforge.settings", JSON.stringify({language: o.lang}));
    api.GetState.mockImplementation(async () => makeState({admin: o.admin ?? true}));
    const es = o.lang === "es";
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(es ? /apps del catálogo/ : /catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: es ? "Menú principal" : "Main menu"})).getByRole("button", {name: es ? /Arranque/ : /Startup/}));
    await screen.findByRole("heading", {name: es ? "Arranque" : "Startup", level: 1});
    if (o.tab) await user.click(screen.getByRole("tab", {name: es ? (o.tab === "services" ? "Servicios" : "Tareas programadas") : (o.tab === "services" ? "Services" : "Scheduled tasks")}));
    return user;
}
const mode = (name: string) => screen.findByRole("combobox", {name: `Start mode: ${name}`}) as Promise<HTMLSelectElement>;
const tswitch = (name: string) => screen.findByRole("checkbox", {name: `Scheduled task enabled: ${name}`}) as Promise<HTMLInputElement>;

describe("startup tabs", () => {
    it("shows an accessible tablist with Programs selected, and keeps the programs list working", async () => {
        api.StartupList.mockResolvedValue([{id: "hkcu-run|Discord", name: "Discord", source: "hkcu-run", command: "d.exe", target: "d.exe", publisher: "", enabled: true, needsAdmin: false, missing: false, caution: false}]);
        await open();
        const list = screen.getByRole("tablist", {name: "What starts with Windows"});
        const tabs = within(list).getAllByRole("tab");
        expect(tabs.map((x) => x.textContent)).toEqual(["Programs", "Services", "Scheduled tasks"]);
        expect(tabs.map((x) => x.getAttribute("aria-selected"))).toEqual(["true", "false", "false"]);
        expect(tabs.map((x) => x.tabIndex)).toEqual([0, -1, -1]);
        expect(await screen.findByRole("checkbox", {name: "Start with Windows: Discord"})).toBeInTheDocument();
        expect(screen.getByRole("tabpanel", {name: "Programs"})).toBeVisible();
        // Services and tasks are read only when their tab is opened.
        expect(api.ServicesList).not.toHaveBeenCalled();
        expect(api.TasksList).not.toHaveBeenCalled();
    });

    it("moves between tabs with the arrow keys, Home and End, and keeps one Tab stop", async () => {
        const user = await open();
        const tab = (n: string) => screen.getByRole("tab", {name: n});
        tab("Programs").focus();
        await user.keyboard("{ArrowRight}");
        expect(tab("Services")).toHaveAttribute("aria-selected", "true");
        expect(tab("Services")).toHaveFocus();
        expect(tab("Services").tabIndex).toBe(0);
        expect(tab("Programs").tabIndex).toBe(-1);
        await user.keyboard("{ArrowRight}");
        expect(tab("Scheduled tasks")).toHaveFocus();
        await user.keyboard("{ArrowRight}");
        expect(tab("Programs")).toHaveFocus(); // wraps
        await user.keyboard("{ArrowLeft}");
        expect(tab("Scheduled tasks")).toHaveAttribute("aria-selected", "true");
        await user.keyboard("{Home}");
        expect(tab("Programs")).toHaveAttribute("aria-selected", "true");
        await user.keyboard("{End}");
        expect(tab("Scheduled tasks")).toHaveAttribute("aria-selected", "true");
        expect(await screen.findByRole("tabpanel", {name: "Scheduled tasks"})).toBeVisible();
        expect(screen.queryByRole("tabpanel", {name: "Programs"})).toBeNull(); // hidden panels are out of the accessibility tree
    });

    it("keeps each tab's list and search when you come back", async () => {
        const user = await open({tab: "services"});
        await mode("Steam Client Service");
        await user.type(screen.getByRole("searchbox", {name: "Search"}), "valve");
        await user.click(screen.getByRole("tab", {name: "Programs"}));
        await user.click(screen.getByRole("tab", {name: "Services"}));
        expect(screen.getByRole("searchbox", {name: "Search"})).toHaveValue("valve");
        expect(api.ServicesList).toHaveBeenCalledTimes(1);
    });
});

describe("services tab", () => {
    it("lists services with publisher, state, start mode and program, and counts the running ones", async () => {
        await open({tab: "services"});
        expect((await mode("Steam Client Service")).value).toBe("delayed");
        expect((await mode("Docker Desktop Service")).value).toBe("auto");
        expect((await mode("Old Agent")).value).toBe("disabled");
        expect(screen.getByText("Publisher: Valve Corporation")).toBeInTheDocument();
        expect(screen.getByText("C:\\Program Files\\Steam Client Service\\Steam Client Service.exe")).toBeInTheDocument();
        expect(screen.getByText("2 of 5 running")).toBeInTheDocument();
        expect(screen.getAllByText("Running", {selector: ".chip"})).toHaveLength(2);
        expect(screen.getAllByText("Stopped", {selector: ".chip"})).toHaveLength(3);
        // The service name shows only when it differs from the display name.
        expect(screen.getByText("com.docker.service", {selector: ".chip"})).toBeInTheDocument();
        const options = within(await mode("Steam Client Service")).getAllByRole("option").map((o) => o.textContent);
        expect(options).toEqual(["Automatic", "Automatic (delayed)", "Manual", "Disabled"]);
    });

    it("labels each select by the service and describes it with its state", async () => {
        await open({tab: "services"});
        const select = await mode("Docker Desktop Service");
        expect(select).toHaveAccessibleDescription("Running");
        expect(await mode("Old Agent")).toHaveAccessibleDescription("Stopped");
    });

    it("says when nothing is found and shows a failed read as a message", async () => {
        api.ServicesList.mockResolvedValueOnce([]);
        await open({tab: "services"});
        expect(await screen.findByText("No third-party services found.")).toBeInTheDocument();
        api.ServicesList.mockRejectedValueOnce(new Error("scm unavailable"));
        const user = userEvent.setup();
        await user.click(within(screen.getByRole("tabpanel", {name: "Services"})).getByRole("button", {name: "Refresh"}));
        expect(await screen.findByText(/scm unavailable/)).toBeInTheDocument();
    });

    it("filters by text and by state, and says when nothing matches", async () => {
        const user = await open({tab: "services"});
        await mode("Steam Client Service");
        const panel = within(screen.getByRole("tabpanel", {name: "Services"}));
        await user.type(panel.getByRole("searchbox", {name: "Search"}), "docker");
        expect(screen.queryByRole("combobox", {name: "Start mode: Steam Client Service"})).toBeNull();
        expect(await mode("Docker Desktop Service")).toBeInTheDocument();
        await user.clear(panel.getByRole("searchbox", {name: "Search"}));
        await user.selectOptions(panel.getByRole("combobox", {name: "Show"}), "running");
        expect(panel.getAllByRole("combobox", {name: /Start mode/})).toHaveLength(2);
        await user.selectOptions(panel.getByRole("combobox", {name: "Show"}), "changed");
        expect(panel.getAllByRole("combobox", {name: /Start mode/})).toHaveLength(1);
        await user.selectOptions(panel.getByRole("combobox", {name: "Show"}), "all");
        await user.type(panel.getByRole("searchbox", {name: "Search"}), "zzz");
        expect(panel.getByText("No service matches.")).toBeInTheDocument();
    });

    it("changes the start mode without asking, sends only id and mode, and announces it", async () => {
        api.ServicesSetMode.mockImplementation(async (id: string, m: ServiceItem["startMode"]) => setMode(services(), id, m));
        const user = await open({tab: "services"});
        await user.selectOptions(await mode("Steam Client Service"), "manual");
        await waitFor(() => expect(api.ServicesSetMode).toHaveBeenCalledWith("Steam Client Service", "manual", false));
        expect(await screen.findByText("Steam Client Service now starts: Manual.")).toBeInTheDocument();
        expect((await mode("Steam Client Service")).value).toBe("manual");
        expect(api.ServicesRestore).not.toHaveBeenCalled();
    });

    it("asks before changing security software, does nothing on Cancel, and sends the confirmation", async () => {
        api.ServicesSetMode.mockImplementation(async (id: string, m: ServiceItem["startMode"]) => setMode(services(), id, m));
        const user = await open({tab: "services"});
        await user.selectOptions(await mode("Kaspersky Service"), "disabled");
        const dialog = await screen.findByRole("alertdialog", {name: "Change Kaspersky Service?"});
        expect(within(dialog).getByText(/looks like security software/)).toBeInTheDocument();
        await user.click(within(dialog).getByRole("button", {name: "Cancel"}));
        expect(api.ServicesSetMode).not.toHaveBeenCalled();
        expect((await mode("Kaspersky Service")).value).toBe("auto"); // the select snapped back
        await user.selectOptions(await mode("Kaspersky Service"), "manual");
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Change it"}));
        await waitFor(() => expect(api.ServicesSetMode).toHaveBeenCalledWith("AVP21", "manual", true));
    });

    it("warns about the services that depend on one when its start mode is lowered, not when it is raised", async () => {
        api.ServicesSetMode.mockImplementation(async (id: string, m: ServiceItem["startMode"]) => setMode(services(), id, m));
        const user = await open({tab: "services"});
        await user.selectOptions(await mode("Vendor Parent"), "disabled");
        const dialog = await screen.findByRole("alertdialog", {name: "Change Vendor Parent?"});
        expect(within(dialog).getByText(/Other services need Vendor Parent: Vendor Child, Vendor Other/)).toBeInTheDocument();
        expect(within(dialog).getByRole("list", {name: "Needed by 2"})).toBeInTheDocument();
        await user.click(within(dialog).getByRole("button", {name: "Change it"}));
        await waitFor(() => expect(api.ServicesSetMode).toHaveBeenCalledWith("Parent", "disabled", true));
        // Automatic -> Automatic (delayed) is not lowering: no question.
        api.ServicesSetMode.mockClear();
        await user.selectOptions(await mode("Vendor Parent"), "delayed");
        expect(screen.queryByRole("alertdialog")).toBeNull();
        await waitFor(() => expect(api.ServicesSetMode).toHaveBeenCalledWith("Parent", "delayed", false));
    });

    it("shows how many services need one", async () => {
        await open({tab: "services"});
        await mode("Vendor Parent");
        expect(screen.getByText("Needed by 2")).toHaveAttribute("title", "Other services need this one: Vendor Child, Vendor Other.");
    });

    it("offers Restore original only for a service WinForge changed, and restores it", async () => {
        api.ServicesRestore.mockImplementation(async () => services().map((i) => (i.id === "Old Agent" ? {...i, startMode: "auto" as const, changed: false, original: "" as const} : i)));
        const user = await open({tab: "services"});
        await mode("Old Agent");
        expect(screen.getAllByRole("button", {name: /^Restore the original setting of/})).toHaveLength(1);
        expect(screen.getByText("Changed by WinForge", {selector: ".chip"})).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Restore the original setting of Old Agent"}));
        await waitFor(() => expect(api.ServicesRestore).toHaveBeenCalledWith("Old Agent", false));
        expect(await screen.findByText("Old Agent is back to its original setting: Automatic.")).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: /^Restore the original setting of/})).toBeNull();
    });

    it("shows the original mode on the restore button", async () => {
        await open({tab: "services"});
        await mode("Old Agent");
        expect(screen.getByRole("button", {name: "Restore the original setting of Old Agent"})).toHaveTextContent("Restore original: Automatic");
    });

    it("locks every service without administrator rights and offers to restart", async () => {
        const user = await open({admin: false, tab: "services"});
        expect(await mode("Steam Client Service")).toBeDisabled();
        expect(screen.getByRole("button", {name: "Restore the original setting of Old Agent"})).toBeDisabled();
        expect(screen.getByText("Changing any of these needs administrator rights (5 locked).")).toBeInTheDocument();
        await user.click(screen.getAllByRole("button", {name: "Restart as administrator"})[0]);
        expect(api.RestartAsAdmin).toHaveBeenCalled();
    });

    it("has no restart offer for an administrator", async () => {
        await open({admin: true, tab: "services"});
        await mode("Steam Client Service");
        expect(screen.queryByRole("button", {name: "Restart as administrator"})).toBeNull();
    });

    it.each([
        ["svc:needs-admin", /needs administrator rights/, false],
        ["svc:protected", /belongs to Windows or is protected/, true],
        ["svc:not-found", /no longer there/, true],
        ["svc:not-listed", /no longer there/, true],
        ["svc:not-applied", /did not apply the change/, true],
        ["svc:store-failed", /could not save the original setting/, false],
        ["svc:failed: Access is denied.", /The change failed: Access is denied\./, false],
    ])("explains %s in a sentence", async (code, text, refreshes) => {
        api.ServicesSetMode.mockRejectedValue(new Error(code));
        const user = await open({tab: "services"});
        await user.selectOptions(await mode("Steam Client Service"), "manual");
        expect(await screen.findByText(text)).toBeInTheDocument();
        expect((await mode("Steam Client Service")).value).toBe("delayed"); // nothing changed
        if (refreshes) await waitFor(() => expect(api.ServicesList).toHaveBeenCalledTimes(2));
        else expect(api.ServicesList).toHaveBeenCalledTimes(1);
    });

    it("asks when Go asks (a stale page), and then sends the confirmation", async () => {
        api.ServicesSetMode.mockRejectedValueOnce(new Error("svc:has-dependents")).mockImplementation(async (id: string, m: ServiceItem["startMode"]) => setMode(services(), id, m));
        const user = await open({tab: "services"});
        await user.selectOptions(await mode("Steam Client Service"), "manual");
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Change it"}));
        await waitFor(() => expect(api.ServicesSetMode).toHaveBeenLastCalledWith("Steam Client Service", "manual", true));
    });
});

describe("scheduled tasks tab", () => {
    it("lists tasks with trigger, folder, program, state and last result", async () => {
        await open({tab: "tasks"});
        expect((await tswitch("GoogleUpdateTaskMachineCore")).checked).toBe(true);
        expect((await tswitch("OldSync")).checked).toBe(false);
        expect(screen.getAllByText("At log on", {selector: ".chip"})).toHaveLength(3);
        expect(screen.getAllByText("At startup", {selector: ".chip"})).toHaveLength(2);
        expect(screen.getByText("State: Disabled")).toBeInTheDocument();
        expect(screen.getByText(/Folder: \\Acme\\ · Last result: succeeded/)).toBeInTheDocument();
        expect(screen.getByText(/Last result: code 0x80070002/)).toBeInTheDocument();
        expect(screen.getByText(/Last result: not available/)).toBeInTheDocument();
        expect(screen.getByText("3 of 4 are on")).toBeInTheDocument();
        expect(await tswitch("OldSync")).toHaveAccessibleDescription("State: Disabled");
    });

    it("turns a task off and on, sending only its id", async () => {
        api.TasksSetEnabled.mockImplementation(async (id: string, on: boolean) => setEnabled(tasks(), id, on));
        const user = await open({tab: "tasks"});
        await user.click(await tswitch("GoogleUpdateTaskMachineCore"));
        await waitFor(() => expect(api.TasksSetEnabled).toHaveBeenCalledWith("\\GoogleUpdateTaskMachineCore", false, false));
        expect(await screen.findByText("GoogleUpdateTaskMachineCore is off.")).toBeInTheDocument();
        await user.click(await tswitch("OldSync"));
        await waitFor(() => expect(api.TasksSetEnabled).toHaveBeenCalledWith("\\OldSync", true, false));
        expect(await screen.findByText("OldSync is on.")).toBeInTheDocument();
    });

    it("asks before turning off security software, and not before turning it on", async () => {
        api.TasksSetEnabled.mockImplementation(async (id: string, on: boolean) => setEnabled(tasks(), id, on));
        const user = await open({tab: "tasks"});
        await user.click(await tswitch("Avast Update"));
        const dialog = await screen.findByRole("alertdialog", {name: "Turn off Avast Update?"});
        await user.click(within(dialog).getByRole("button", {name: "Cancel"}));
        expect(api.TasksSetEnabled).not.toHaveBeenCalled();
        await user.click(await tswitch("Avast Update"));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Turn off"}));
        await waitFor(() => expect(api.TasksSetEnabled).toHaveBeenCalledWith("\\Avast Update", false, true));
    });

    it("locks tasks that need administrator rights, but not the user's own", async () => {
        const user = await open({admin: false, tab: "tasks"});
        expect(await tswitch("GoogleUpdateTaskMachineCore")).toBeDisabled();
        expect(await tswitch("MyBackup")).toBeEnabled();
        expect(screen.getByText("Changing any of these needs administrator rights (3 locked).")).toBeInTheDocument();
        await user.click(screen.getAllByRole("button", {name: "Restart as administrator"})[0]);
        expect(api.RestartAsAdmin).toHaveBeenCalled();
    });

    it("offers Restore original only for a task WinForge changed", async () => {
        api.TasksRestore.mockImplementation(async () => tasks().map((i) => (i.id === "\\OldSync" ? {...i, enabled: true, state: "ready" as const, changed: false, original: "" as const} : i)));
        const user = await open({tab: "tasks"});
        await tswitch("OldSync");
        expect(screen.getAllByRole("button", {name: /^Restore the original setting of/})).toHaveLength(1);
        expect(screen.getByRole("button", {name: "Restore the original setting of OldSync"})).toHaveTextContent("Restore original: on");
        await user.click(screen.getByRole("button", {name: "Restore the original setting of OldSync"}));
        await waitFor(() => expect(api.TasksRestore).toHaveBeenCalledWith("\\OldSync", false));
        expect(await screen.findByText("OldSync is back to its original setting: on.")).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: /^Restore the original setting of/})).toBeNull();
    });

    it("filters by text and state, says when nothing starts and explains refusals", async () => {
        const user = await open({tab: "tasks"});
        await tswitch("OldSync");
        const panel = within(screen.getByRole("tabpanel", {name: "Scheduled tasks"}));
        await user.selectOptions(panel.getByRole("combobox", {name: "Show"}), "off");
        expect(panel.getAllByRole("checkbox")).toHaveLength(1);
        await user.selectOptions(panel.getByRole("combobox", {name: "Show"}), "changed");
        expect(panel.getAllByRole("checkbox")).toHaveLength(1);
        await user.selectOptions(panel.getByRole("combobox", {name: "Show"}), "all");
        await user.type(panel.getByRole("searchbox", {name: "Search"}), "acme");
        expect(panel.getAllByRole("checkbox")).toHaveLength(1); // folder \Acme\
        await user.clear(panel.getByRole("searchbox", {name: "Search"}));
        api.TasksSetEnabled.mockRejectedValue(new Error("task:protected"));
        await user.click(await tswitch("MyBackup"));
        expect(await screen.findByText(/belongs to Windows or is protected/)).toBeInTheDocument();
        await waitFor(() => expect(api.TasksList).toHaveBeenCalledTimes(2));
    });

    it("says when no task starts with Windows", async () => {
        api.TasksList.mockResolvedValue([]);
        await open({tab: "tasks"});
        expect(await screen.findByText("No third-party scheduled task starts with Windows.")).toBeInTheDocument();
    });
});

describe("helpers", () => {
    it("lowersStart ranks automatic and delayed together", () => {
        expect(lowersStart("auto", "delayed")).toBe(false);
        expect(lowersStart("delayed", "auto")).toBe(false);
        expect(lowersStart("auto", "manual")).toBe(true);
        expect(lowersStart("manual", "disabled")).toBe(true);
        expect(lowersStart("disabled", "manual")).toBe(false);
        expect(lowersStart("manual", "manual")).toBe(false);
    });

    it("filters services and tasks", () => {
        expect(visibleServices(services(), "VALVE", "all").map((i) => i.id)).toEqual(["Steam Client Service"]);
        expect(visibleServices(services(), "", "running").map((i) => i.id)).toEqual(["com.docker.service", "AVP21"]);
        expect(visibleServices(services(), "", "stopped")).toHaveLength(3);
        expect(visibleServices(services(), "com.docker", "all")).toHaveLength(1);
        expect(visibleTasks(tasks(), "", "off").map((i) => i.name)).toEqual(["OldSync"]);
        expect(visibleTasks(tasks(), "acme", "all").map((i) => i.name)).toEqual(["OldSync"]);
        expect(visibleTasks(tasks(), "", "changed")).toHaveLength(1);
    });

    it("turns result codes into text", () => {
        expect(resultText(null)).toBe("Last result: not available");
        expect(resultText(0)).toBe("Last result: succeeded");
        expect(resultText(-2147024894)).toBe("Last result: code 0x80070002");
        expect(resultText(267009)).toBe("Last result: code 0x00041301");
    });

    it("turns Go's codes into sentences and keeps anything else as it came", () => {
        expect(svcError(new Error("task:needs-admin")).text).toMatch(/administrator/);
        expect(svcError(new Error("task:needs-confirm")).ask).toBe(true);
        expect(svcError(new Error("svc:has-dependents")).ask).toBe(true);
        expect(svcError(new Error("svc:no-original")).refresh).toBe(true);
        expect(svcError(new Error("svc:bad-mode")).text).toMatch(/not a valid/);
        expect(svcError("something else").text).toBe("something else");
    });
});

describe("Spanish", () => {
    it("has its own tabs, texts and messages for services and tasks", async () => {
        api.ServicesSetMode.mockImplementation(async (id: string, m: ServiceItem["startMode"]) => setMode(services(), id, m));
        api.TasksSetEnabled.mockImplementation(async (id: string, on: boolean) => setEnabled(tasks(), id, on));
        const user = await open({lang: "es", tab: "services"});
        const select = (await screen.findByRole("combobox", {name: "Tipo de inicio: Steam Client Service"})) as HTMLSelectElement;
        expect(within(select).getAllByRole("option").map((o) => o.textContent)).toEqual(["Automático", "Automático (retrasado)", "Manual", "Deshabilitado"]);
        expect(screen.getByText("Editor: Valve Corporation")).toBeInTheDocument();
        expect(screen.getByText("2 de 5 en ejecución")).toBeInTheDocument();
        await user.selectOptions(select, "manual");
        expect(await screen.findByText("Steam Client Service ahora arranca así: Manual.")).toBeInTheDocument();
        await user.selectOptions(await screen.findByRole("combobox", {name: "Tipo de inicio: Kaspersky Service"}), "disabled");
        expect(await screen.findByRole("alertdialog", {name: "¿Cambiar Kaspersky Service?"})).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Cancelar"}));

        await user.click(screen.getByRole("tab", {name: "Tareas programadas"}));
        const sw = (await screen.findByRole("checkbox", {name: "Tarea programada activada: MyBackup"})) as HTMLInputElement;
        expect(screen.getAllByText("Al iniciar sesión", {selector: ".chip"}).length).toBeGreaterThan(0);
        expect(screen.getByText(/Último resultado: código 0x80070002/)).toBeInTheDocument();
        await user.click(sw);
        expect(await screen.findByText("MyBackup está desactivada.")).toBeInTheDocument();
    });

    it("explains a refusal in Spanish", async () => {
        api.ServicesSetMode.mockRejectedValue(new Error("svc:needs-admin"));
        const user = await open({lang: "es", tab: "services"});
        await user.selectOptions(await screen.findByRole("combobox", {name: "Tipo de inicio: Steam Client Service"}), "manual");
        expect(await screen.findByText(/requiere permisos de administrador/)).toBeInTheDocument();
    });

    it("has exactly the English keys, with no empty text", () => {
        expect(Object.keys(svcEs).sort()).toEqual(Object.keys(svcEn).sort());
        for (const [k, v] of Object.entries(svcEs)) expect(v.trim(), k).not.toBe("");
        // Placeholders match between languages.
        for (const k of Object.keys(svcEn) as (keyof typeof svcEn)[]) {
            const ph = (s: string) => (s.match(/\{\w+\}/g) ?? []).sort().join();
            expect(ph(svcEs[k]), k).toBe(ph(svcEn[k]));
        }
    });
});
