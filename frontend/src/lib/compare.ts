import type {CompareResult, CompareSide} from "../api";
import {t, type Key} from "./i18n";

export type CompareKind = "file" | "code" | "files";

/** Which comparison a result came from. */
export function kindOf(r: CompareResult): CompareKind {
    if (r.a.kind === "file") return "files";
    return r.b.kind === "code" ? "code" : "file";
}

/** How many rows a long list shows before "show more". */
export const LIST_LIMIT = 12;

/**
 * Go sends failures as "compare:<code>" (never prose); this words them in the language of the interface.
 * Anything else (a failure from outside the comparison) is shown as it came.
 */
export function compareError(e: unknown): string {
    const text = e instanceof Error ? e.message : String(e);
    const m = /compare:([a-z-]+)/.exec(text);
    const key = m ? (`cmp.err.${m[1]}` as Key) : null;
    if (key && t(key) !== key) return t(key);
    return t("cmp.err.other", {e: text});
}

/** A side in a sentence: "this PC", "the file “Laptop”", "file A". */
export function sideLabel(side: CompareSide, role: "a" | "b", kind: CompareKind): string {
    const base = side.kind === "pc" ? t("cmp.side.pc")
        : kind === "files" ? t(role === "a" ? "cmp.side.a" : "cmp.side.b")
            : side.kind === "code" ? t("cmp.side.code") : t("cmp.side.file");
    const name = side.name;
    return name ? t("cmp.side.withName", {base, name}) : base;
}

/** The name a saved copy of a side starts from. */
export function saveName(side: CompareSide): string {
    return side.name || side.file.replace(/\.json$/i, "") || t("cmp.side.code");
}

/** The one-sentence summary at the top of a result. */
export function summary(r: CompareResult): string {
    const kind = kindOf(r);
    const a = sideLabel(r.a, "a", kind);
    const b = sideLabel(r.b, "b", kind);
    const both = r.both.length;
    if (both === 0 && r.differ.length === 0 && r.onlyA.length === 0 && r.onlyB.length === 0) {
        return r.unknown.length ? t("cmp.summary.none") + " " + t("cmp.summary.unknown", {n: r.unknown.length}) : t("cmp.summary.none");
    }
    const parts: string[] = [];
    if (r.onlyA.length === 0 && r.onlyB.length === 0 && r.differ.length === 0) {
        parts.push(t("cmp.summary.same", {n: both}));
    } else {
        parts.push(t("cmp.summary", {both, a, b, onlyA: r.onlyA.length, onlyB: r.onlyB.length}));
        if (r.differ.length > 0) parts.push(t("cmp.summary.differ", {n: r.differ.length}));
    }
    if (r.unknown.length > 0) parts.push(t("cmp.summary.unknown", {n: r.unknown.length}));
    return parts.join(" ");
}
