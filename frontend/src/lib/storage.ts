import type {CleanCategory, DupGroup} from "../api";
import {t, tOpt} from "./i18n";

/** The categories worth showing: the ones this PC has and that hold something. */
export const visibleCategories = (cats: CleanCategory[]): CleanCategory[] => cats.filter((c) => c.available && c.bytes > 0);

/** Safe categories start ticked; "review" ones never do. */
export const initialSelection = (cats: CleanCategory[]): Set<string> => new Set(visibleCategories(cats).filter((c) => c.risk === "safe").map((c) => c.id));

/** Display name of a cleanup category: its translation, or the raw id for one this build does not know yet. */
export const categoryName = (id: string): string => tOpt(`storage.cat.${id}`) ?? id;

/** One-line hint of a cleanup category; empty for an unknown id. */
export const categoryHint = (id: string): string => tOpt(`storage.cat.${id}.hint`) ?? "";

/** "1 file" / "12 files". */
export const fileCount = (n: number): string => t(n === 1 ? "storage.files.one" : "storage.files.other", {n});

/** Directory and file name of a Windows (or POSIX) path. */
export function splitPath(path: string): { dir: string; name: string } {
    const i = Math.max(path.lastIndexOf("\\"), path.lastIndexOf("/"));
    return i < 0 ? {dir: "", name: path} : {dir: path.slice(0, i), name: path.slice(i + 1)};
}

/** Last segment of a folder path, or the whole path for a drive root such as "C:\". */
export function folderName(path: string): string {
    const trimmed = path.replace(/[\\/]+$/, "");
    return splitPath(trimmed).name || path;
}

const stamp = (iso: string): number => {
    const ms = Date.parse(iso);
    return Number.isNaN(ms) ? Number.POSITIVE_INFINITY : ms; // an unreadable date never wins "oldest"
};

/** Bytes that removing all but one copy would free. */
export const groupWaste = (g: DupGroup): number => g.sizeBytes * Math.max(0, g.files.length - 1);

/** Groups with the most wasted space first; ties keep their order. */
export const sortGroups = (groups: DupGroup[]): DupGroup[] =>
    groups.map((g, i) => ({g, i})).sort((a, b) => groupWaste(b.g) - groupWaste(a.g) || a.i - b.i).map((x) => x.g);

/** The copy that stays by default: the oldest one (the first listed on a tie). */
export function oldestPath(g: DupGroup): string {
    let best = g.files[0];
    for (const f of g.files) if (stamp(f.modified) < stamp(best.modified)) best = f;
    return best.path;
}

/** Every copy except the oldest of each group is marked for the Recycle Bin. */
export function defaultRemovals(groups: DupGroup[]): Set<string> {
    const out = new Set<string>();
    for (const g of groups) {
        const keep = oldestPath(g);
        for (const f of g.files) if (f.path !== keep) out.add(f.path);
    }
    return out;
}

/** Copies of this group that are not marked, i.e. that would stay. */
export const keptIn = (g: DupGroup, removed: ReadonlySet<string>): string[] => g.files.filter((f) => !removed.has(f.path)).map((f) => f.path);

/**
 * Marks or unmarks one copy. Marking the last copy that would stay is refused: a group
 * never loses every copy.
 */
export function toggleRemoval(g: DupGroup, removed: ReadonlySet<string>, path: string): Set<string> {
    const next = new Set(removed);
    if (next.has(path)) next.delete(path);
    else if (keptIn(g, removed).length > 1) next.add(path);
    return next;
}

/** What the marked copies add up to. */
export function removalTotals(groups: DupGroup[], removed: ReadonlySet<string>): { files: number; bytes: number } {
    let files = 0;
    let bytes = 0;
    for (const g of groups) {
        for (const f of g.files) if (removed.has(f.path)) { files++; bytes += g.sizeBytes; }
    }
    return {files, bytes};
}
