import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import App from "../App";
import {api, emit, resetApi} from "./apiMock";
import {healthResult, makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const withFixes = () => healthResult({
    ok: 1, total: 4,
    findings: [
        {key: "device-problems", group: "drivers", severity: "bad", params: {count: "2", devices: "Realtek ACPI\\RTK5452\\1"}, fixes: ["repair-missing-drivers"]},
        {key: "firewall", group: "security", severity: "warn", params: {profiles: "Public"}, fixes: ["repair-firewall"]},
        {key: "wu-drivers", group: "drivers", severity: "warn", params: {count: "1"}, fixes: ["repair-driver-updates"]},
        {key: "tpm", group: "firmware", severity: "unknown"},
        {key: "bios-age", group: "firmware", severity: "warn", params: {version: "1", date: "2020-01-01", months: "70", board: "X"}},
        {key: "pending-reboot", group: "windows", severity: "warn"},
        {key: "disk-health", group: "storage", severity: "ok"},
    ],
});

async function openHealth(result = withFixes(), admin = true) {
    resetApi(makeState({admin}));
    api.HealthScan.mockResolvedValue(result);
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(/catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: "Main menu"})).getByRole("button", {name: /PC health/}));
    await screen.findByRole("heading", {name: "Something needs fixing"});
    return user;
}

const step = (id: string, name: string) => ({kind: "recipe" as const, id, name, admin: true});

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
});

describe("fixing what the health page finds", () => {
    it("fixes one finding by itself: plan, run, then the PC is scanned again", async () => {
        const user = await openHealth();
        const card = screen.getByText("2 devices need a driver").closest("li") as HTMLElement;
        await user.click(within(card).getByRole("button", {name: "Fix it for me"}));
        expect(api.PlanRepairs).toHaveBeenCalledWith(["repair-missing-drivers"]);

        const dialog = await screen.findByRole("dialog", {name: "Repair this PC"});
        expect(within(dialog).getByText("repair-missing-drivers".length ? "Find drivers for devices that have none" : "")).toBeInTheDocument();
        expect(api.ApplyRepairs).not.toHaveBeenCalled();
        const scans = api.HealthScan.mock.calls.length;
        await user.click(within(dialog).getByRole("button", {name: "Run 1 steps"}));
        expect(api.ApplyRepairs).toHaveBeenCalledWith(["repair-missing-drivers"]);

        await emit.install({step: step("repair-missing-drivers", "x"), status: "start"});
        await emit.install({step: step("repair-missing-drivers", "x"), status: "ok"});
        await emit.done([]);
        await waitFor(() => expect(api.HealthScan.mock.calls.length).toBeGreaterThan(scans));
    });

    it("fixes everything fixable at once, each repair only once", async () => {
        const user = await openHealth();
        await user.click(screen.getByRole("button", {name: "Fix everything I can (3)"}));
        await waitFor(() => expect(api.PlanRepairs).toHaveBeenCalled());
        const ids = api.PlanRepairs.mock.calls[0][0] as string[];
        expect([...ids].sort()).toEqual(["repair-driver-updates", "repair-firewall", "repair-missing-drivers"]);
    });

    it("does not offer a fix for what only a person can fix, such as a BIOS update", async () => {
        const user = await openHealth();
        const bios = screen.getByText(/BIOS/i, {selector: "b"}).closest("li") as HTMLElement;
        expect(within(bios).queryByRole("button", {name: "Fix it for me"})).not.toBeInTheDocument();
        void user;
    });

    it("warns that repairs need administrator rights, and offers the restart", async () => {
        const user = await openHealth(withFixes(), false);
        const card = screen.getByText("2 devices need a driver").closest("li") as HTMLElement;
        await user.click(within(card).getByRole("button", {name: "Fix it for me"}));
        const dialog = await screen.findByRole("dialog", {name: "Repair this PC"});
        expect(within(dialog).getByText("Some steps need administrator rights.")).toBeInTheDocument();
        await user.click(within(dialog).getByRole("button", {name: "Restart as administrator"}));
        expect(api.RestartAsAdmin).toHaveBeenCalled();
    });

    it("says why a repair failed and retries only that", async () => {
        const user = await openHealth();
        const card = screen.getByText("2 devices need a driver").closest("li") as HTMLElement;
        await user.click(within(card).getByRole("button", {name: "Fix it for me"}));
        await user.click(await screen.findByRole("button", {name: "Run 1 steps"}));
        await emit.install({step: step("repair-missing-drivers", "x"), status: "start"});
        await emit.install({step: step("repair-missing-drivers", "x"), status: "failed", error: "recipe exited with code 1", reason: "installer"});
        await emit.done(["repair-missing-drivers"]);
        expect(await screen.findByText(/The installer reported an error/)).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Retry what failed"})).toBeInTheDocument();
    });

    it("has a maintenance tool to check and repair Windows files", async () => {
        const user = await openHealth();
        await user.click(screen.getByRole("button", {name: "Check and repair Windows files"}));
        expect(api.PlanRepairs).toHaveBeenCalledWith(["repair-windows-files"]);
        expect(await screen.findByRole("dialog", {name: "Repair this PC"})).toBeInTheDocument();
    });

    it("tells the person if a repair cannot even be planned", async () => {
        const user = await openHealth();
        api.PlanRepairs.mockRejectedValueOnce(new Error("\"x\" is not a repair"));
        const card = screen.getByText("2 devices need a driver").closest("li") as HTMLElement;
        await user.click(within(card).getByRole("button", {name: "Fix it for me"}));
        expect(await screen.findByText("\"x\" is not a repair")).toBeInTheDocument();
        expect(screen.queryByRole("dialog", {name: "Repair this PC"})).not.toBeInTheDocument();
    });
});

describe("restarting to finish repairs", () => {
    it("asks first, and the restart can be cancelled", async () => {
        const user = await openHealth();
        await user.click(screen.getByRole("button", {name: "Restart now"}));
        const sure = await screen.findByRole("alertdialog", {name: "Restart the PC?"});
        expect(api.RestartPC).not.toHaveBeenCalled();
        await user.click(within(sure).getByRole("button", {name: "Cancel"}));
        expect(api.RestartPC).not.toHaveBeenCalled();

        await user.click(screen.getByRole("button", {name: "Restart now"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Restart now"}));
        await waitFor(() => expect(api.RestartPC).toHaveBeenCalled());
        const banner = await screen.findByText("Restarting in 30 seconds.");
        expect(banner).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Cancel the restart"}));
        expect(api.CancelRestart).toHaveBeenCalled();
        expect(screen.queryByText("Restarting in 30 seconds.")).not.toBeInTheDocument();
    });
});
