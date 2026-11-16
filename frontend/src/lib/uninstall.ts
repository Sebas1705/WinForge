import {errText, type LeftoverCandidate, type LeftoverWhy, type UninstallApp} from "../api";
import {t, tOpt} from "./i18n";

/** Installed apps whose name or publisher contains the text, case-insensitively. Empty text shows all. */
export function visibleApps(apps: UninstallApp[], query: string): UninstallApp[] {
    const q = query.trim().toLowerCase();
    return q ? apps.filter((a) => `${a.name} ${a.publisher}`.toLowerCase().includes(q)) : apps;
}

/** Catalog apps that are not installed and whose name contains the text, for the manual leftovers search. Nothing for text under 2 characters. */
export function notInstalledMatches<T extends { id: string; name: string; installed: boolean }>(apps: T[], hidden: ReadonlySet<string>, query: string, max = 8): T[] {
    const q = query.trim().toLowerCase();
    if (q.length < 2) return [];
    return apps.filter((a) => !a.installed && !hidden.has(a.id) && a.name.toLowerCase().includes(q)).slice(0, max);
}

/** The sentence that says why a folder or shortcut was listed. */
export const whyText = (w: LeftoverWhy): string => tOpt(`uninst.why.${w.rule}`, {value: w.value}) ?? w.value;

/** Where a leftover sits, as a name for people. */
export const placeName = (id: string): string => tOpt(`uninst.place.${id}`) ?? id;

/** Why a path was not moved, as a phrase; the raw key for a reason this build does not know. */
export const reasonText = (key: string): string => tOpt(`uninst.reason.${key}`) ?? key;

/** Bytes of the ticked leftovers. */
export const markedBytes = (list: LeftoverCandidate[], marked: ReadonlySet<string>): number =>
    list.reduce((n, c) => (marked.has(c.path) ? n + c.sizeBytes : n), 0);

/** The Go side's refusals as sentences. Anything else is shown as it came. */
export function uninstallError(e: unknown): { text: string; refresh: boolean } {
    const m = errText(e);
    if (m.includes("still installed")) return {text: t("uninst.error.stillInstalled"), refresh: true};
    if (m.includes("not installed")) return {text: t("uninst.error.notInstalled"), refresh: true};
    if (m.includes("was not listed")) return {text: t("uninst.error.notListed"), refresh: false};
    return {text: m, refresh: false};
}

/** "Git, VLC and 7-Zip" is a list for people; commas keep it language-neutral. */
export const nameList = (names: string[]): string => names.join(", ");
