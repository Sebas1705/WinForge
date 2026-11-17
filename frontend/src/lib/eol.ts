// Types mirroring the end-of-support report (internal/eol) and the plain-language helpers the
// "End of support" section uses. The Go side sends facts (a status, dates, cycles); every sentence
// is built here so it can be Spanish or English.
import {getLang, t} from "./i18n";

export type EOLStatus = "ended" | "soon" | "ok" | "unknown";

export interface EOLReplacement { cycle: string; end?: string; lts?: boolean }
export interface EOLFinding {
    appId: string;
    product: string;
    productName: string;
    version: string;
    /** The matched release line; "" when unknown. */
    cycle: string;
    status: EOLStatus;
    /** Last day of support, YYYY-MM-DD; absent when none is announced. */
    end?: string;
    /** End minus today in days; negative once it ended. Absent with `end`. */
    days?: number;
    lts?: boolean;
    source: string;
    replace: EOLReplacement[];
    /** Catalog apps that install one of the supported lines. */
    apps: string[];
    /** The installed app always installs the newest release, so updating it is the way to move on. */
    selfUpdate?: boolean;
}
export interface EOLReport {
    /** Day the bundled table was written, YYYY-MM-DD. */
    asOf: string;
    ageDays: number;
    stale: boolean;
    findings: EOLFinding[];
}

/** A YYYY-MM-DD day in the interface language. It is a calendar date, so no time zone shifts it. */
export function dayText(day: string): string {
    const d = new Date(`${day}T00:00:00Z`);
    // A day JavaScript would roll over (February 30) is not a date: show it as written.
    if (!/^\d{4}-\d{2}-\d{2}$/.test(day) || Number.isNaN(d.getTime()) || d.toISOString().slice(0, 10) !== day) return day;
    return new Intl.DateTimeFormat(getLang(), {dateStyle: "long", timeZone: "UTC"}).format(d);
}

/** "1 day" / "47 days". */
export function spanText(days: number): string {
    const n = Math.abs(Math.round(days));
    return n === 1 ? t("eol.span.day") : t("eol.span.days", {n});
}

/** When a date is, relative to today: "today", "in 12 days", "47 days ago". */
export function whenText(days: number): string {
    if (days === 0) return t("eol.when.today");
    return days > 0 ? t("eol.when.in", {span: spanText(days)}) : t("eol.when.ago", {span: spanText(days)});
}

/** The age of the table in whole months, at least 1 once it is stale, for the warning. */
export function monthsOld(ageDays: number): number {
    return Math.max(1, Math.floor(ageDays / 30.4375));
}

export function counts(fs: EOLFinding[]): Record<EOLStatus, number> {
    const c: Record<EOLStatus, number> = {ended: 0, soon: 0, ok: 0, unknown: 0};
    for (const f of fs) if (f.status in c) c[f.status]++;
    return c;
}

/** "3.15", "3.14 (long-term support) until 31 October 2031". */
export function replacementText(r: EOLReplacement): string {
    const vars = {cycle: r.cycle, date: r.end ? dayText(r.end) : ""};
    if (r.end) return t(r.lts ? "eol.rep.ltsUntil" : "eol.rep.until", vars);
    return t(r.lts ? "eol.rep.lts" : "eol.rep.open", vars);
}

/** The sentence about the release's date. */
export function lineText(f: EOLFinding): string {
    const vars = {product: f.productName, cycle: f.cycle, version: f.version};
    switch (f.status) {
        case "ended":
            return t("eol.line.ended", {...vars, date: dayText(f.end ?? ""), when: whenText(f.days ?? -1)});
        case "soon":
            return t("eol.line.soon", {...vars, date: dayText(f.end ?? ""), when: whenText(f.days ?? 0)});
        case "ok":
            return f.end ? t("eol.line.ok", {...vars, date: dayText(f.end)}) : t("eol.line.okNoDate", vars);
        default:
            return f.version ? t("eol.line.unknown", vars) : t("eol.line.unknownNoVersion", vars);
    }
}

/** What to do about it, in plain words. Empty for a release that is fine. */
export function adviceText(f: EOLFinding): string {
    switch (f.status) {
        case "ended": return t("eol.do.ended");
        case "soon": return t("eol.do.soon");
        case "unknown": return t("eol.do.unknown");
        default: return "";
    }
}
