import {afterEach, describe, expect, it} from "vitest";
import {batterySeries, driveSeries, niceCeil, plotX, plotY, scaleFor, scoreSeries, statementText, usable, whText, type HHSnapshot} from "./hhist";
import {setLang} from "./i18n";

const GiB = 2 ** 30;
const snap = (at: string, o: Partial<HHSnapshot> = {}): HHSnapshot => ({at, score: 15, total: 20, areas: {}, volumes: [], disks: [], battery: null, ...o});

afterEach(() => setLang("en"));

describe("chart series", () => {
    it("builds the score series in time order and ignores impossible snapshots", () => {
        const s = scoreSeries([
            snap("2026-10-03T09:00:00Z", {score: 17}), snap("2026-10-01T09:00:00Z", {score: 15}),
            snap("2026-10-02T09:00:00Z", {score: Number.NaN}), snap("not a date"), snap("2026-10-04T09:00:00Z", {total: 0}),
        ]);
        expect(s.points.map((p) => p.y)).toEqual([15, 17]);
    });

    it("builds free space per drive in GB and skips negative or NaN readings", () => {
        const vol = (letter: string, free: number) => ({letter, freeBytes: free, totalBytes: 500 * GiB});
        const series = driveSeries([
            snap("2026-10-01T09:00:00Z", {volumes: [vol("C", 100 * GiB), vol("D", 300 * GiB)]}),
            snap("2026-10-02T09:00:00Z", {volumes: [vol("C", Number.NaN), vol("D", -5)]}),
            snap("2026-10-03T09:00:00Z", {volumes: [vol("C", 90 * GiB)]}),
            snap("2026-10-04T09:00:00Z"), // an old snapshot without volumes at all
        ]);
        expect(series.map((s) => s.id)).toEqual(["C", "D"]);
        expect(series[0].points.map((p) => p.y)).toEqual([100, 90]);
        expect(series[1].points).toHaveLength(1);
        expect(driveSeries([snap("2026-10-01T09:00:00Z")])).toEqual([]);
    });

    it("builds battery capacity as a share of the original, never above 100", () => {
        const b = (full: number, design = 50000) => ({designMWh: design, fullMWh: full, cycles: null});
        const s = batterySeries([snap("2026-10-01T09:00:00Z", {battery: b(45000)}), snap("2026-10-02T09:00:00Z", {battery: b(60000)}), snap("2026-10-03T09:00:00Z", {battery: b(0)}), snap("2026-10-04T09:00:00Z")]);
        expect(s.points.map((p) => p.y)).toEqual([90, 100]);
    });

    it("drops points that cannot be drawn", () => {
        expect(usable([{x: 2, y: 1}, {x: Number.NaN, y: 1}, {x: 1, y: Number.POSITIVE_INFINITY}, {x: 1, y: 3}])).toEqual([{x: 1, y: 3}, {x: 2, y: 1}]);
    });
});

describe("chart scale", () => {
    it("has no scale without data, and gives one point a day either side", () => {
        expect(scaleFor([])).toBeNull();
        expect(scaleFor([{id: "a", name: "a", points: [{x: Number.NaN, y: 1}]}])).toBeNull();
        const s = scaleFor([{id: "a", name: "a", points: [{x: 1_000_000_000, y: 42}]}])!;
        expect(s.x1 - s.x0).toBe(2 * 86400000);
        expect(s.y0).toBe(0);
        expect(s.y1).toBe(50);
    });

    it("rounds the top up to a round number and keeps plotted points inside the plot", () => {
        expect(niceCeil(0)).toBe(1);
        expect(niceCeil(-4)).toBe(1);
        expect(niceCeil(Number.NaN)).toBe(1);
        expect(niceCeil(87)).toBe(100);
        expect(niceCeil(101)).toBe(200);
        expect(niceCeil(2.2)).toBe(2.5);
        const s = {x0: 0, x1: 10, y0: 0, y1: 100};
        expect(plotX(5, s, 50, 200)).toBe(150);
        expect(plotY(0, s, 10, 100)).toBe(110);
        expect(plotY(100, s, 10, 100)).toBe(10);
        expect(plotY(500, s, 10, 100)).toBe(10); // above the scale: clamped
        expect(plotY(-5, s, 10, 100)).toBe(110);
    });
});

describe("statement text", () => {
    it("words every code in both languages, with sizes and day counts", () => {
        const free = {code: "free-fell", params: {drive: "C", bytes: String(12 * GiB), days: "30"}};
        expect(statementText(free)).toBe("Free space on C: fell 12 GB in 30 days.");
        expect(statementText({code: "free-fell", params: {drive: "C", bytes: String(2 * GiB), days: "1"}})).toBe("Free space on C: fell 2 GB in 1 day.");
        expect(statementText({code: "wear-rose", params: {disk: "NVMe", from: "4", to: "6", days: "20"}})).toBe("Wear of NVMe rose from 4% to 6% in 20 days.");
        setLang("es");
        expect(statementText(free)).toBe("Espacio libre en C: bajó 12 GB en 30 días.");
        expect(statementText({code: "steady", params: {days: "1"}})).toBe("Sin cambios notables en los últimos 1 día.");
    });

    it("says nothing for a code it does not know", () => {
        expect(statementText({code: "future-thing", params: {}})).toBe("");
    });
});

describe("battery numbers", () => {
    it("writes watt-hours, and nothing for a missing reading", () => {
        expect(whText(48200)).toBe("48.2 Wh");
        expect(whText(0)).toBe("");
        expect(whText(Number.NaN)).toBe("");
        expect(whText(-3)).toBe("");
    });
});
