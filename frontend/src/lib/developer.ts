import type {EnvFinding} from "../api";
import {t, tOpt, type Key} from "./i18n";

/** The kinds the Go side reports, in the order the page shows them. Unknown kinds follow, in the order they came. */
export const KIND_ORDER = [
    "path-missing", "path-not-dir", "path-duplicate", "path-empty", "path-relative", "path-long",
    "home-missing", "home-invalid", "home-mismatch", "tool-shadowed", "tool-store-alias",
] as const;

export interface KindGroup { kind: string; known: boolean; items: EnvFinding[] }

/** Findings grouped by kind: known kinds first in a fixed order, then any kind this version does not know. */
export function groupFindings(findings: EnvFinding[]): KindGroup[] {
    const by = new Map<string, EnvFinding[]>();
    for (const f of findings) by.set(f.kind, [...(by.get(f.kind) ?? []), f]);
    const known = KIND_ORDER.filter((k) => by.has(k)).map((k) => ({kind: k as string, known: true, items: by.get(k)!}));
    const other = [...by.keys()].filter((k) => !(KIND_ORDER as readonly string[]).includes(k)).map((k) => ({kind: k, known: false, items: by.get(k)!}));
    return [...known, ...other];
}

/** Only user-scope findings that Go marked fixable can be ticked; machine scope is report-only. */
export const selectable = (f: EnvFinding): boolean => f.scope === "user" && f.fixable && f.fixKind !== "";

export const groupTitle = (g: KindGroup): string => (g.known ? t(`dev.env.kind.${g.kind}` as Key) : t("dev.env.kind.other"));

/** One finding as a plain sentence. A kind this version does not know shows its raw name instead of failing. */
export function findingText(f: EnvFinding): string {
    const scope = t(f.scope === "machine" ? "dev.env.scope.machine" : "dev.env.scope.user");
    return tOpt(`dev.env.line.${f.kind}`, {subject: f.subject, scope}) ?? t("dev.env.line.other", {kind: f.kind, subject: f.subject});
}

/** What a fix will do to one finding, for the confirmation dialog. */
export function fixText(f: EnvFinding): string {
    const s = f.fixKind ? tOpt(`dev.env.fix.${f.fixKind}`, {subject: f.subject}) : null;
    return s ?? findingText(f);
}

/** A backup time as the person's local date and time; the raw text when it is not a date. */
export function backupTime(time: string): string {
    const d = new Date(time);
    return Number.isNaN(d.getTime()) ? time : d.toLocaleString();
}
