import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import App from "../App";
import type {BatteryInfo, HHSnapshot, HHView} from "../lib/hhist";
import {api, resetApi} from "./apiMock";
import {healthResult, makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const GiB = 2 ** 30;
const snap = (day: number, o: Partial<HHSnapshot> = {}): HHSnapshot => ({
    at: new Date(Date.UTC(2026, 9, day, 9)).toISOString(), score: 15, total: 20, areas: {storage: 1},
    volumes: [{letter: "C", freeBytes: 200 * GiB, totalBytes: 500 * GiB}], disks: [], battery: null, ...o,
});
const view = (snapshots: HHSnapshot[], statements: HHView["statements"] = [], enough = statements.length > 0): HHView => ({snapshots, statements, enough});

const laptopHistory = (): HHView => view([
    snap(1, {score: 15, battery: {designMWh: 50000, fullMWh: 45000, cycles: 200}, disks: [{name: "Samsung 990", media: "SSD", health: "Healthy", wearPct: 4, tempC: 40, powerOnHours: 1200}]}),
    snap(11, {score: 16, volumes: [{letter: "C", freeBytes: 190 * GiB, totalBytes: 500 * GiB}, {letter: "D", freeBytes: 900 * GiB, totalBytes: 2000 * GiB}], battery: {designMWh: 50000, fullMWh: 43000, cycles: 210}}),
    snap(21, {score: 18, volumes: [{letter: "C", freeBytes: 188 * GiB, totalBytes: 500 * GiB}, {letter: "D", freeBytes: 899 * GiB, totalBytes: 2000 * GiB}], battery: {designMWh: 50000, fullMWh: 42000, cycles: 220},
        disks: [{name: "Samsung 990", media: "SSD", health: "Healthy", wearPct: 6, tempC: null, powerOnHours: null}]}),
], [
    {code: "free-fell", params: {drive: "C", bytes: String(12 * GiB), days: "20"}},
    {code: "wear-rose", params: {disk: "Samsung 990", from: "4", to: "6", days: "20"}},
    {code: "battery-fell", params: {from: "90", to: "84", days: "20"}},
]);

const laptop = (o: Partial<BatteryInfo> = {}): BatteryInfo => ({
    present: true, designMWh: 50000, fullMWh: 42000, healthPct: 84, cycles: 312, verdict: "good", goodMinPct: 80, replaceBelowPct: 60,
    batteries: [{designMWh: 50000, fullMWh: 42000, healthPct: 84, cycles: 312, verdict: "good"}],
    history: [{date: "2026-09-28", designMWh: 50000, fullMWh: 43000, healthPct: 86, cycles: 300}, {date: "2026-10-05", designMWh: 50000, fullMWh: 42000, healthPct: 84, cycles: 312}],
    ...o,
});

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi(makeState());
    api.HealthScan.mockResolvedValue(healthResult());
});

async function open(lang?: "es") {
    if (lang) localStorage.setItem("winforge.settings", JSON.stringify({language: lang}));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(lang ? /apps del catálogo/ : /catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: lang ? "Menú principal" : "Main menu"})).getByRole("button", {name: lang ? /Salud del PC/ : /PC health/}));
    await screen.findByRole("heading", {name: lang ? "Historial" : "History", level: 2});
    return user;
}

describe("PC health history", () => {
    it("shows the statements, the charts and the latest drive readings", async () => {
        api.HealthHistory.mockResolvedValue(laptopHistory());
        api.Battery.mockResolvedValue(laptop());
        await open();
        expect(await screen.findByText("Free space on C: fell 12 GB in 20 days.")).toBeInTheDocument();
        expect(screen.getByText("Wear of Samsung 990 rose from 4% to 6% in 20 days.")).toBeInTheDocument();
        expect(screen.getByText("Battery capacity fell from 90% to 84% of the original in 20 days.")).toBeInTheDocument();
        expect(screen.getByText("3 scans saved")).toBeInTheDocument();
        // Each chart is an image with a text description, so it is not only visual.
        const score = screen.getByRole("img", {name: "Checks passing"});
        expect(score).toHaveAccessibleDescription(/3 scans from .* first 15, latest 18/);
        const free = screen.getByRole("img", {name: "Free space per drive (GB)"});
        expect(free).toHaveAccessibleDescription(/C:: first 200 GB, latest 188 GB.*D:: first 900 GB, latest 899 GB/);
        expect(screen.getByRole("img", {name: "Battery capacity (% of original)"})).toBeInTheDocument();
        // Several drives: a legend names each one (marker shape and dash differ, not only colour).
        expect(screen.getAllByText(/C:: first 200 GB, latest 188 GB/).length).toBeGreaterThan(0);
        // Latest drive readings: unreported values say so.
        const row = screen.getByRole("row", {name: /Samsung 990/});
        expect(within(row).getByText("6%")).toBeInTheDocument();
        expect(within(row).getAllByText("not reported")).toHaveLength(2);
        expect(screen.getByText(/only available when the drive reports them/)).toBeInTheDocument();
    });

    it("offers every chart as a table, reachable by keyboard", async () => {
        api.HealthHistory.mockResolvedValue(laptopHistory());
        const user = await open();
        const toggles = await screen.findAllByRole("button", {name: "Show as table"});
        expect(toggles).toHaveLength(3);
        expect(toggles[0]).toHaveAttribute("aria-expanded", "false");
        toggles[0].focus();
        await user.keyboard("{Enter}");
        const table = await screen.findByRole("table", {name: "Checks passing, one row per scan"});
        expect(within(table).getAllByRole("row")).toHaveLength(4); // header + 3 scans
        expect(within(table).getAllByRole("columnheader").map((c) => c.textContent)).toEqual(["Date", "Checks passing"]);
        expect(within(table).getByText("18")).toBeInTheDocument();
        expect(screen.getAllByRole("button", {name: "Hide table"})[0]).toHaveAttribute("aria-expanded", "true");
        // The drive table has a column per drive and a dash where a drive had no reading.
        await user.click(screen.getAllByRole("button", {name: "Show as table"})[0]);
        const drives = await screen.findByRole("table", {name: "Free space per drive (GB), one row per scan"});
        expect(within(drives).getAllByRole("columnheader").map((c) => c.textContent)).toEqual(["Date", "C:", "D:"]);
        expect(within(drives).getAllByText("–")).toHaveLength(1);
        await user.click(screen.getAllByRole("button", {name: "Hide table"})[0]);
        expect(screen.queryByRole("table", {name: "Checks passing, one row per scan"})).toBeNull();
    });

    it("says nothing about trends until there is enough data, and says what is missing", async () => {
        api.HealthHistory.mockResolvedValue(view([snap(1)], []));
        await open();
        expect(await screen.findByText("Trends appear once there are scans from two different days.")).toBeInTheDocument();
        expect(screen.getByText("1 scans saved")).toBeInTheDocument();
        expect(screen.getByRole("img", {name: "Checks passing"})).toHaveAccessibleDescription(/one scan on/);
        expect(screen.queryByRole("img", {name: "Battery capacity (% of original)"})).toBeNull(); // no reading, no chart
        expect(screen.queryByRole("list", {name: /./})).toBeNull();
    });

    it("shows an empty history, a failed read and a loading state without breaking the page", async () => {
        api.HealthHistory.mockResolvedValue(view([]));
        await open();
        expect(await screen.findByText("No history yet. It starts with the next scan.")).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: "Clear history"})).toBeNull(); // nothing to clear
        expect(screen.getByRole("heading", {name: "Something needs fixing"})).toBeInTheDocument();
    });

    it("shows a failed history read as a message", async () => {
        api.HealthHistory.mockRejectedValue(new Error("file locked"));
        await open();
        expect(await screen.findByRole("alert")).toHaveTextContent("The history could not be read: file locked");
    });

    it("survives snapshots with missing or impossible values", async () => {
        api.HealthHistory.mockResolvedValue(view([
            {...snap(1), volumes: undefined as unknown as HHSnapshot["volumes"], disks: undefined as unknown as HHSnapshot["disks"]},
            snap(5, {score: Number.NaN, volumes: [{letter: "C", freeBytes: Number.NaN, totalBytes: -1}]}),
            snap(9, {total: 0, volumes: []}),
        ], [{code: "from-the-future", params: {}}], true));
        await open();
        expect(await screen.findByText("3 scans saved")).toBeInTheDocument();
        expect(screen.getByRole("img", {name: "Checks passing"})).toBeInTheDocument();
        expect(screen.queryByRole("img", {name: "Free space per drive (GB)"})).toBeNull();
        expect(screen.getByText("Trends appear once there are scans from two different days.")).toBeInTheDocument(); // the only statement was unknown
    });

    it("asks before clearing, does nothing on cancel, and reloads after clearing", async () => {
        api.HealthHistory.mockResolvedValue(laptopHistory());
        const user = await open();
        await user.click(await screen.findByRole("button", {name: "Clear history"}));
        const dialog = await screen.findByRole("alertdialog", {name: "Clear the history?"});
        expect(within(dialog).getByText(/cannot be undone/)).toBeInTheDocument();
        await user.click(within(dialog).getByRole("button", {name: "Cancel"}));
        expect(api.HealthHistoryClear).not.toHaveBeenCalled();
        expect(screen.queryByRole("alertdialog")).toBeNull();

        api.HealthHistory.mockResolvedValue(view([]));
        await user.click(screen.getByRole("button", {name: "Clear history"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Clear history"}));
        expect(api.HealthHistoryClear).toHaveBeenCalledTimes(1);
        expect(await screen.findByText("History cleared.")).toBeInTheDocument();
        expect(await screen.findByText("No history yet. It starts with the next scan.")).toBeInTheDocument();
    });

    it("reports a failed clear and keeps the history", async () => {
        api.HealthHistory.mockResolvedValue(laptopHistory());
        api.HealthHistoryClear.mockRejectedValueOnce(new Error("access denied"));
        const user = await open();
        await user.click(await screen.findByRole("button", {name: "Clear history"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Clear history"}));
        expect(await screen.findByText("access denied")).toBeInTheDocument();
        expect(screen.getByText("3 scans saved")).toBeInTheDocument();
    });

    it("reads the history again after a new scan", async () => {
        api.HealthHistory.mockResolvedValue(view([snap(1)]));
        const user = await open();
        await screen.findByText("1 scans saved");
        api.HealthHistory.mockResolvedValue(laptopHistory());
        api.HealthScan.mockResolvedValue(healthResult({report: {...healthResult().report, collectedAt: "2026-10-07T10:00:00Z"}}));
        await user.click(screen.getByRole("button", {name: "Scan"}));
        expect(await screen.findByText("3 scans saved")).toBeInTheDocument();
    });

    it("is in Spanish with the same content", async () => {
        api.HealthHistory.mockResolvedValue(laptopHistory());
        api.Battery.mockResolvedValue(laptop());
        const user = await open("es");
        expect(await screen.findByText("Espacio libre en C: bajó 12 GB en 20 días.")).toBeInTheDocument();
        expect(screen.getByText("El desgaste de Samsung 990 subió del 4% al 6% en 20 días.")).toBeInTheDocument();
        expect(screen.getByRole("img", {name: "Comprobaciones correctas"})).toBeInTheDocument();
        expect(screen.getByText("Lecturas de los discos en el último análisis")).toBeInTheDocument();
        await user.click(screen.getAllByRole("button", {name: "Ver como tabla"})[0]);
        expect(await screen.findByRole("table", {name: "Comprobaciones correctas, una fila por análisis"})).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Borrar historial"}));
        expect(await screen.findByRole("alertdialog", {name: "¿Borrar el historial?"})).toBeInTheDocument();
    });
});

describe("battery card", () => {
    it("gives a laptop a plain verdict, its numbers and the weekly readings", async () => {
        api.Battery.mockResolvedValue(laptop());
        await open();
        const card = (await screen.findByRole("heading", {name: "Battery"})).closest("section") as HTMLElement;
        expect(within(card).getByText("Good: a full charge holds 84% of the original capacity.")).toBeInTheDocument();
        expect(within(card).getByText("50 Wh")).toBeInTheDocument();
        expect(within(card).getAllByText("42 Wh").length).toBeGreaterThan(0); // the stat and the latest weekly row
        expect(within(card).getByText("312")).toBeInTheDocument();
        expect(within(card).getByText(/Good is 80% or more.*worn is 60% to 80%.*under 60%.*off by a few percent/)).toBeInTheDocument();
        const rows = within(card).getAllByRole("row");
        expect(rows).toHaveLength(3); // header + two weeks
        expect(within(rows[1]).getByText("86%")).toBeInTheDocument();
    });

    it.each([
        ["worn", 70, "Worn: a full charge holds 70% of the original capacity, so it lasts noticeably less than when new."],
        ["replace", 45, "Replace soon: a full charge holds only 45% of the original capacity."],
    ] as const)("words the %s verdict", async (verdict, pct, text) => {
        api.Battery.mockResolvedValue(laptop({verdict, healthPct: pct}));
        await open();
        expect(await screen.findByText(text)).toBeInTheDocument();
    });

    it("claims nothing when Windows gives no usable capacity or cycle count", async () => {
        api.Battery.mockResolvedValue(laptop({verdict: "unknown", healthPct: 0, designMWh: 0, fullMWh: 0, cycles: null, history: []}));
        await open();
        const card = (await screen.findByRole("heading", {name: "Battery"})).closest("section") as HTMLElement;
        expect(within(card).getByText(/Unknown: Windows did not report a usable capacity/)).toBeInTheDocument();
        expect(within(card).getAllByText("not reported")).toHaveLength(4);
        expect(within(card).queryByText("0%")).toBeNull();
        expect(within(card).queryByRole("table")).toBeNull();
    });

    it("lists each battery when there are two", async () => {
        api.Battery.mockResolvedValue(laptop({batteries: [
            {designMWh: 30000, fullMWh: 27000, healthPct: 90, cycles: null, verdict: "good"},
            {designMWh: 20000, fullMWh: 0, healthPct: 0, cycles: null, verdict: "unknown"},
        ]}));
        await open();
        expect(await screen.findByText(/Battery 1: 90% · 27 Wh \/ 30 Wh/)).toBeInTheDocument();
        expect(screen.getByText("Battery 2: not reported")).toBeInTheDocument();
    });

    it("says a desktop has no battery and shows no verdict", async () => {
        await open(); // the default mock answers present: false
        expect(await screen.findByText("Windows reports no battery on this PC, so there is nothing to track.")).toBeInTheDocument();
        expect(screen.queryByText(/Good:|Worn:|Replace soon:/)).toBeNull();
    });

    it("shows a failed read as a short note", async () => {
        api.Battery.mockRejectedValue(new Error("powercfg failed"));
        await open();
        expect(await screen.findByText("The battery report could not be read: powercfg failed")).toBeInTheDocument();
    });

    it("is in Spanish", async () => {
        api.Battery.mockResolvedValue(laptop());
        await open("es");
        expect(await screen.findByText(/Buena: una carga completa conserva el 84%/)).toBeInTheDocument();
        expect(screen.getByText("Ciclos de carga")).toBeInTheDocument();
        await waitFor(() => expect(screen.getByText(/Buena es el 80% o más/)).toBeInTheDocument());
    });
});
