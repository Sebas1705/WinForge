// Squarified treemap (Bruls, Huizing, van Wijk) for the space map in "Biggest things".
// Pure and deterministic: the same entries and size always give the same blocks. Blocks are
// whole pixels and tile the rectangle exactly, with no gap and no overlap.

export interface TreeEntry { id: string; name: string; size: number }

export interface Rect { x: number; y: number; w: number; h: number }

export interface TreeCell {
    /** "item" is one entry; "other" merges the tail of small entries and is not meant to be opened. */
    kind: "item" | "other";
    /** Entry id; "other" for the merged block. */
    id: string;
    /** Entry name, or "" for the merged block (the page words it). */
    name: string;
    size: number;
    /** Share of the total of the entries laid out, 0..1. */
    share: number;
    /** Entries merged into an "other" block (their ids, biggest first); empty for an item. */
    merged: string[];
    /** 0 for the biggest block; "other" is always last in rank. */
    rank: number;
    rect: Rect;
}

export interface TreeOptions {
    /** Entries that would get less area than this (px squared) go into "other". Default 1800. */
    minArea?: number;
    /** Most blocks, "other" included. Default 60. */
    maxBlocks?: number;
}

export interface TreeResult {
    cells: TreeCell[];
    /** Sum of the sizes that count. */
    total: number;
    /** Entries ignored for having no usable size (NaN, infinite, zero or negative). */
    ignored: number;
}

export const DEFAULT_MIN_AREA = 1800;
export const DEFAULT_MAX_BLOCKS = 60;

/** Size desc, then name asc, then id asc: a total order, so the layout never depends on input order. */
export function compareEntries(a: TreeEntry, b: TreeEntry): number {
    if (a.size !== b.size) return b.size - a.size;
    if (a.name !== b.name) return a.name < b.name ? -1 : 1;
    return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
}

interface Box { x0: number; y0: number; x1: number; y1: number }

/** Worst aspect ratio of a row of areas laid along a side of length `side`. */
function worst(areas: number[], sum: number, side: number): number {
    if (sum <= 0 || side <= 0) return Number.POSITIVE_INFINITY;
    let max = 0, min = Number.POSITIVE_INFINITY;
    for (const a of areas) { if (a > max) max = a; if (a < min) min = a; }
    const s2 = side * side, sum2 = sum * sum;
    return Math.max((s2 * max) / sum2, sum2 / (s2 * min));
}

/**
 * Lays areas (already in descending order, and summing to the box area) into the box. Returns float
 * boxes, one per area in order. Edges are shared values, never accumulated sums, so neighbours agree
 * exactly and the last one in a row/column ends on the container's own edge.
 */
function squarify(areas: number[], box: Box): Box[] {
    const out: Box[] = new Array(areas.length);
    let rest = {...box};
    let i = 0;
    while (i < areas.length) {
        const w = rest.x1 - rest.x0, h = rest.y1 - rest.y0;
        const side = Math.min(w, h);
        // grow the row while its worst aspect ratio does not get worse
        let j = i + 1, sum = areas[i];
        let cur = worst([areas[i]], sum, side);
        while (j < areas.length) {
            const nextSum = sum + areas[j];
            const next = worst(areas.slice(i, j + 1), nextSum, side);
            if (next > cur) break;
            sum = nextSum; cur = next; j++;
        }
        const last = j >= areas.length;
        const horizontal = w >= h; // the row is a column of cells stacked along the short side
        if (horizontal) {
            // a column at the left: width = sum / h
            const x1 = last ? rest.x1 : rest.x0 + (sum / h);
            let y = rest.y0;
            for (let k = i; k < j; k++) {
                const y1 = k === j - 1 ? rest.y1 : y + (areas[k] / sum) * h;
                out[k] = {x0: rest.x0, y0: y, x1, y1};
                y = y1;
            }
            rest = {x0: x1, y0: rest.y0, x1: rest.x1, y1: rest.y1};
        } else {
            // a row at the top: height = sum / w
            const y1 = last ? rest.y1 : rest.y0 + (sum / w);
            let x = rest.x0;
            for (let k = i; k < j; k++) {
                const x1 = k === j - 1 ? rest.x1 : x + (areas[k] / sum) * w;
                out[k] = {x0: x, y0: rest.y0, x1, y1};
                x = x1;
            }
            rest = {x0: rest.x0, y0: y1, x1: rest.x1, y1: rest.y1};
        }
        i = j;
    }
    return out;
}

/**
 * Lays the entries out in a width x height rectangle. Entries with a size that is not a positive finite
 * number are ignored. The smallest entries (below `minArea`, or beyond `maxBlocks`) merge into one
 * "other" block, so a folder with thousands of entries still gives a handful of legible blocks.
 */
export function layoutTreemap(entries: readonly TreeEntry[], width: number, height: number, opts: TreeOptions = {}): TreeResult {
    const W = Number.isFinite(width) ? Math.floor(width) : 0;
    const H = Number.isFinite(height) ? Math.floor(height) : 0;
    const good = entries.filter((e) => Number.isFinite(e.size) && e.size > 0);
    const ignored = entries.length - good.length;
    const total = good.reduce((s, e) => s + e.size, 0);
    if (good.length === 0 || W < 1 || H < 1 || !(total > 0)) return {cells: [], total: Number.isFinite(total) ? total : 0, ignored};

    const minArea = Math.max(0, opts.minArea ?? DEFAULT_MIN_AREA);
    const maxBlocks = Math.max(2,Math.floor(opts.maxBlocks ?? DEFAULT_MAX_BLOCKS));
    const sorted = [...good].sort(compareEntries);
    const area = W * H;
    const areaOf = (size: number) => (size / total) * area;

    // How many entries stay as blocks: those big enough, within the cap.
    let keep = 0;
    while (keep < sorted.length && keep < maxBlocks && areaOf(sorted[keep].size) >= minArea) keep++;
    keep = Math.max(1, keep);
    const tailSum = (from: number) => { let s = 0; for (let k = from; k < sorted.length; k++) s += sorted[k].size; return s; };
    // With a tail, the merged block counts towards the cap. It may itself be small (it is not opened, and
    // has no label); pulling big entries into it just to make it bigger would hide the wrong things.
    if (keep < sorted.length) keep = Math.max(1, Math.min(keep, maxBlocks - 1));
    const shown = sorted.slice(0, keep);
    const tail = sorted.slice(keep);

    type Item = { entry?: TreeEntry; size: number; merged: string[] };
    const items: Item[] = shown.map((entry) => ({entry, size: entry.size, merged: []}));
    if (tail.length > 0) items.push({size: tailSum(keep), merged: tail.map((e) => e.id)});
    // "other" may be bigger than a few blocks; lay out by size so the squarify order assumption holds.
    const order = items.map((it, idx) => ({it, idx})).sort((a, b) => (b.it.size - a.it.size) || (a.idx - b.idx)).map((x) => x.it);

    const sum = order.reduce((s, it) => s + it.size, 0);
    const boxes = squarify(order.map((it) => (it.size / sum) * area), {x0: 0, y0: 0, x1: W, y1: H});
    const cells: TreeCell[] = order.map((it, k) => {
        const b = boxes[k];
        const x0 = Math.round(b.x0), y0 = Math.round(b.y0), x1 = Math.round(b.x1), y1 = Math.round(b.y1);
        return {
            kind: it.entry ? "item" : "other",
            id: it.entry ? it.entry.id : "other",
            name: it.entry ? it.entry.name : "",
            size: it.size,
            share: it.size / total,
            merged: it.merged,
            rank: 0,
            rect: {x: x0, y: y0, w: x1 - x0, h: y1 - y0},
        };
    });
    // rank: by size, the merged block last
    const ranked = cells.filter((c) => c.kind === "item");
    ranked.forEach((c, r) => { c.rank = r; });
    cells.forEach((c) => { if (c.kind === "other") c.rank = ranked.length; });
    return {cells, total, ignored};
}

/** Cells in reading order: top to bottom, then left to right. Used for keyboard movement and the DOM order. */
export function readingOrder<T extends { rect: Rect }>(cells: readonly T[]): T[] {
    return [...cells].sort((a, b) => (a.rect.y - b.rect.y) || (a.rect.x - b.rect.x));
}

/** Tint level 0 (biggest) .. levels-1 (smallest) from the rank among `count` item blocks. */
export function tintOf(rank: number, count: number, levels = 5): number {
    if (count <= 1) return 0;
    return Math.min(levels - 1, Math.floor((rank * levels) / count));
}

/** Whether a label of `lines` text lines fits a block (px), with room for padding. */
export function labelFits(rect: Rect, lines: 1 | 2, minW = 64, line = 16, pad = 8): boolean {
    return rect.w >= minW && rect.h >= lines * line + pad;
}
