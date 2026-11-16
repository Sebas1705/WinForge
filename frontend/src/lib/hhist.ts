// Types mirroring the health history JSON (internal/healthhistory) and the battery report
// (internal/battery), plus the pure helpers the History section and the Battery card use.
import {formatBytes, formatDate} from "./format";
import {getLang, tOpt, t, type Key} from "./i18n";

export interface HHVolume { letter: string; freeBytes: number; totalBytes: number }
export interface HHDisk { name: string; media?: string; health?: string; wearPct: number | null; tempC: number | null; powerOnHours: number | null }
export interface HHSnapshot {
    at: string;
    score: number;
    total: number;
    areas: Record<string, number>;
    volumes: HHVolume[];
    disks: HHDisk[];
    battery: { designMWh: number; fullMWh: number; cycles: number | null } | null;
}
export interface HHStatement { code: string; params: Record<string, string> }
export interface HHView { snapshots: HHSnapshot[]; statements: HHStatement[]; enough: boolean }

export type BatteryVerdict = "good" | "worn" | "replace" | "unknown";
export interface BatteryUnit { chemistry?: string; designMWh: number; fullMWh: number; healthPct: number; cycles: number | null; verdict: BatteryVerdict }
export interface BatteryPoint { date: string; designMWh: number; fullMWh: number; healthPct: number; cycles: number | null }
export interface BatteryInfo {
    present: boolean;
    batteries: BatteryUnit[];
    designMWh: number;
    fullMWh: number;
    healthPct: number;
    cycles: number | null;
    verdict: BatteryVerdict;
    history: BatteryPoint[];
    goodMinPct: number;
    replaceBelowPct: number;
}

/** A point on a chart: x is a time in ms, y a finite number, label the date shown in the table. */
export interface ChartPoint { x: number; y: number }
export interface ChartSeries { id: string; name: string; points: ChartPoint[] }

const finite = (n: unknown): n is number => typeof n === "number" && Number.isFinite(n);

/** Keeps points that can be drawn (finite time and value), in time order. */
export function usable(points: ChartPoint[]): ChartPoint[] {
    return points.filter((p) => finite(p.x) && finite(p.y)).sort((a, b) => a.x - b.x);
}

function time(s: HHSnapshot): number { return Date.parse(s.at); }

/** Checks passing over time. */
export function scoreSeries(snaps: HHSnapshot[]): ChartSeries {
    return {id: "score", name: t("hh.chart.score"), points: usable(snaps.filter((s) => finite(s.score) && s.total > 0).map((s) => ({x: time(s), y: s.score})))};
}

/** Free space per volume, in GB. A volume only appears once it has been seen. */
export function driveSeries(snaps: HHSnapshot[]): ChartSeries[] {
    const letters = [...new Set(snaps.flatMap((s) => (s.volumes ?? []).map((v) => v.letter)))].sort();
    return letters.map((l) => ({
        id: l, name: `${l}:`,
        points: usable(snaps.flatMap((s) => (s.volumes ?? []).filter((v) => v.letter === l && finite(v.freeBytes) && v.freeBytes >= 0).map((v) => ({x: time(s), y: v.freeBytes / 2 ** 30})))),
    })).filter((s) => s.points.length > 0);
}

/** Battery capacity as a share of its design capacity, per snapshot that has a reading. */
export function batterySeries(snaps: HHSnapshot[]): ChartSeries {
    return {
        id: "battery", name: t("hh.chart.battery"),
        points: usable(snaps.filter((s) => s.battery && s.battery.designMWh > 0 && s.battery.fullMWh > 0)
            .map((s) => ({x: time(s), y: Math.min(100, (s.battery!.fullMWh / s.battery!.designMWh) * 100)}))),
    };
}

export interface Scale { x0: number; x1: number; y0: number; y1: number }

/** Axis ranges: time spans the data (a single point gets a day either side), value starts at yMin and ends at a round number above the data. */
export function scaleFor(series: ChartSeries[], fixed?: {y0: number; y1: number}): Scale | null {
    const pts = series.flatMap((s) => usable(s.points));
    if (pts.length === 0) return null;
    let x0 = Math.min(...pts.map((p) => p.x)), x1 = Math.max(...pts.map((p) => p.x));
    if (x0 === x1) { x0 -= 86400000; x1 += 86400000; }
    if (fixed) return {x0, x1, ...fixed};
    const max = Math.max(...pts.map((p) => p.y));
    return {x0, x1, y0: 0, y1: niceCeil(max)};
}

/** The next "round" number at or above v: 1, 2, 2.5, 5 times a power of ten. */
export function niceCeil(v: number): number {
    if (!finite(v) || v <= 0) return 1;
    const p = 10 ** Math.floor(Math.log10(v));
    for (const m of [1, 2, 2.5, 5, 10]) if (v <= m * p) return m * p;
    return 10 * p;
}

export function plotX(x: number, s: Scale, left: number, width: number): number {
    return left + ((x - s.x0) / (s.x1 - s.x0)) * width;
}
export function plotY(y: number, s: Scale, top: number, height: number): number {
    const f = (y - s.y0) / (s.y1 - s.y0);
    return top + height - Math.max(0, Math.min(1, f)) * height;
}

/** One statement from the Go side, in the current language. Bytes become "12 GB"; an unknown code says nothing. */
export function statementText(st: HHStatement): string {
    const p: Record<string, string> = {...st.params};
    if (p.bytes !== undefined) p.bytes = formatBytes(Number(p.bytes));
    if (p.days !== undefined) p.span = p.days === "1" ? t("hh.day1") : t("hh.days", {n: p.days});
    return tOpt(`hh.stmt.${st.code}`, p) ?? "";
}

export function dateText(ms: number): string {
    return Number.isFinite(ms) ? formatDate(new Date(ms).toISOString()) : "";
}

export function whText(mwh: number): string {
    if (!finite(mwh) || mwh <= 0) return "";
    return `${new Intl.NumberFormat(getLang(), {maximumFractionDigits: 1}).format(mwh / 1000)} Wh`;
}

export function pctText(p: number): string {
    return `${new Intl.NumberFormat(getLang(), {maximumFractionDigits: 1}).format(p)}%`;
}

export const VERDICT_KEY: Record<BatteryVerdict, Key> = {
    good: "hh.battery.verdict.good", worn: "hh.battery.verdict.worn", replace: "hh.battery.verdict.replace", unknown: "hh.battery.verdict.unknown",
};
