import type {AutoItem, AutoReport, AutoSettings} from "../api";
import {getLang, t, tOpt} from "./i18n";

/** What a Go error was, in words. Go sends a code ("auto:busy ..."); the sentence is ours, in the person's language. */
export function autoError(e: unknown): string {
    const m = e instanceof Error ? e.message : String(e);
    const hit = /auto:([a-z-]+)(.*)/s.exec(m);
    const known = hit ? tOpt(`auto.err.${hit[1]}`) : null;
    if (!hit || !known) return m;
    // What Windows itself said about a refused task is its own text; it helps whoever has to fix it.
    const detail = hit[1] === "task-failed" ? hit[2].replace(/^[:\s]+/, "").trim() : "";
    return detail ? `${known} (${detail})` : known;
}

/** Why a whole run could not do its job, from the code in the report. */
export function runErrorText(code: string): string {
    return tOpt(`auto.error.${code}`) ?? code;
}

/** The name of a weekday (0 = Sunday) in the interface language. */
export function dayName(n: number): string {
    // 15 November 2026 is a Sunday.
    return new Intl.DateTimeFormat(getLang(), {weekday: "long"}).format(new Date(2026, 10, 15 + n));
}

const cap = (s: string) => s.charAt(0).toLocaleUpperCase(getLang()) + s.slice(1);
export const dayLabel = (n: number): string => cap(dayName(n));

/** "every day at 09:30" / "every Friday at 18:00". */
export function whenText(s: Pick<AutoSettings, "frequency" | "weekday" | "time">): string {
    return s.frequency === "weekly"
        ? t("auto.when.weekly", {day: dayName(s.weekday), time: s.time})
        : t("auto.when.daily", {time: s.time});
}

export const modeText = (m: "notify" | "apply"): string => t(m === "apply" ? "auto.mode.apply" : "auto.mode.notify");

export function dateText(iso: string): string {
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? "?" : d.toLocaleString(getLang(), {dateStyle: "medium", timeStyle: "short"});
}

/** The counts of a run, as short phrases ("2 updated", "1 needs you"); nothing counted is left out. */
export function summaryParts(items: AutoItem[]): string[] {
    const n = (r: AutoItem["result"]) => items.filter((i) => i.result === r).length;
    const parts: string[] = [];
    if (n("found")) parts.push(t("auto.sum.found", {n: n("found")}));
    if (n("updated")) parts.push(t("auto.sum.updated", {n: n("updated")}));
    if (n("needs-admin")) parts.push(t("auto.sum.needs", {n: n("needs-admin")}));
    if (n("failed")) parts.push(t("auto.sum.failed", {n: n("failed")}));
    if (n("ignored")) parts.push(t("auto.sum.ignored", {n: n("ignored")}));
    return parts.length ? parts : [t("auto.sum.nothing")];
}

/** Why one app ended as it did: why it failed, or how it is ignored. */
export function itemNote(i: AutoItem): string {
    if (i.result === "failed" && i.reason) return tOpt(`auto.reason.${i.reason}`) ?? "";
    if (i.result === "skipped" && i.reason === "cancelled") return t("auto.reason.cancelled-run");
    if (i.result === "ignored" && i.reason) return tOpt(`auto.ignore.${i.reason}`) ?? "";
    return "";
}

/** True when the check found nothing to update (ignored apps are not something to update). */
export function upToDate(r: AutoReport): boolean {
    return !r.error && !r.items.some((i) => i.result === "found" || i.result === "needs-admin");
}
