import {describe, expect, it} from "vitest";
import {compareEntries, labelFits, layoutTreemap, readingOrder, tintOf, type TreeCell, type TreeEntry} from "./treemap";

const e = (name: string, size: number): TreeEntry => ({id: `id-${name}`, name, size});

/** Paints every cell on a grid: no pixel may be covered twice or left empty. */
function coverage(cells: TreeCell[], W: number, H: number): { gaps: number; overlaps: number } {
    const grid = new Uint8Array(W * H);
    for (const c of cells) {
        const {x, y, w, h} = c.rect;
        expect(x).toBeGreaterThanOrEqual(0); expect(y).toBeGreaterThanOrEqual(0);
        expect(x + w).toBeLessThanOrEqual(W); expect(y + h).toBeLessThanOrEqual(H);
        for (let j = y; j < y + h; j++) for (let i = x; i < x + w; i++) grid[j * W + i]++;
    }
    let gaps = 0, overlaps = 0;
    for (const v of grid) { if (v === 0) gaps++; else if (v > 1) overlaps++; }
    return {gaps, overlaps};
}

const mulberry = (seed: number) => () => { seed |= 0; seed = (seed + 0x6D2B79F5) | 0; let t = Math.imul(seed ^ (seed >>> 15), 1 | seed); t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t; return ((t ^ (t >>> 14)) >>> 0) / 4294967296; };

describe("treemap: degenerate input", () => {
    it("gives nothing for no entries, no room or no usable size", () => {
        expect(layoutTreemap([], 400, 300).cells).toEqual([]);
        expect(layoutTreemap([e("a", 5)], 0, 300).cells).toEqual([]);
        expect(layoutTreemap([e("a", 5)], 400, 0).cells).toEqual([]);
        expect(layoutTreemap([e("a", 5)], NaN, 300).cells).toEqual([]);
        expect(layoutTreemap([e("a", 0), e("b", -1)], 400, 300)).toMatchObject({cells: [], ignored: 2, total: 0});
    });

    it("ignores NaN, infinite, negative and zero sizes and counts them", () => {
        const r = layoutTreemap([e("a", 10), e("n", NaN), e("i", Infinity), e("neg", -5), e("z", 0)], 400, 300);
        expect(r.ignored).toBe(4);
        expect(r.cells.map((c) => c.name)).toEqual(["a"]);
        expect(r.total).toBe(10);
    });

    it("fills the whole rectangle with a single entry", () => {
        const r = layoutTreemap([e("only", 123)], 400, 300);
        expect(r.cells).toHaveLength(1);
        expect(r.cells[0]).toMatchObject({kind: "item", name: "only", share: 1, rank: 0, rect: {x: 0, y: 0, w: 400, h: 300}});
    });

    it("rounds a fractional container down to whole pixels", () => {
        const r = layoutTreemap([e("a", 1), e("b", 1)], 400.9, 300.4);
        expect(coverage(r.cells, 400, 300)).toEqual({gaps: 0, overlaps: 0});
    });
});

describe("treemap: ordering", () => {
    it("sorts by size descending, then by name, whatever the input order", () => {
        const a = layoutTreemap([e("b", 5), e("a", 5), e("c", 9), e("d", 1)], 600, 400, {minArea: 0});
        const b = layoutTreemap([e("d", 1), e("c", 9), e("a", 5), e("b", 5)], 600, 400, {minArea: 0});
        expect(a).toEqual(b);
        expect(a.cells.map((c) => c.name)).toEqual(["c", "a", "b", "d"]);
        expect(a.cells.map((c) => c.rank)).toEqual([0, 1, 2, 3]);
    });

    it("is deterministic and does not change its input", () => {
        const input = [e("x", 3), e("y", 7), e("z", 7)];
        const copy = structuredClone(input);
        expect(layoutTreemap(input, 500, 300)).toEqual(layoutTreemap(input, 500, 300));
        expect(input).toEqual(copy);
    });

    it("breaks a name tie by id", () => {
        expect(compareEntries({id: "1", name: "a", size: 1}, {id: "2", name: "a", size: 1})).toBeLessThan(0);
        expect(compareEntries({id: "2", name: "a", size: 1}, {id: "2", name: "a", size: 1})).toBe(0);
    });

    it("puts the biggest block in the top-left corner", () => {
        const r = layoutTreemap([e("small", 1), e("big", 100), e("mid", 10)], 500, 300, {minArea: 0});
        const big = r.cells.find((c) => c.name === "big")!;
        expect(big.rect.x).toBe(0); expect(big.rect.y).toBe(0);
    });
});

describe("treemap: tiling", () => {
    it("tiles exactly for many shapes, sizes and entry counts (no gaps, no overlaps)", () => {
        const rnd = mulberry(7);
        for (const [W, H] of [[320, 240], [333, 241], [1200, 520], [500, 500], [97, 413], [1, 1], [50, 800]] as const) {
            for (const n of [1, 2, 3, 7, 25, 60]) {
                const entries = Array.from({length: n}, (_, i) => e(`f${i}`, Math.ceil(rnd() ** 3 * 1e9)));
                const r = layoutTreemap(entries, W, H, {minArea: 0, maxBlocks: 100});
                expect(coverage(r.cells, W, H), `${W}x${H} n=${n}`).toEqual({gaps: 0, overlaps: 0});
            }
        }
    });

    it("keeps areas proportional to sizes within rounding", () => {
        const r = layoutTreemap([e("a", 600), e("b", 300), e("c", 100)], 1000, 500, {minArea: 0});
        const px = 1000 * 500;
        for (const c of r.cells) {
            const got = c.rect.w * c.rect.h;
            expect(Math.abs(got - c.share * px)).toBeLessThan(2 * (c.rect.w + c.rect.h));
        }
    });

    it("sums shares to 1", () => {
        const r = layoutTreemap([e("a", 3), e("b", 2), e("c", 1)], 400, 300, {minArea: 0});
        expect(r.cells.reduce((s, c) => s + c.share, 0)).toBeCloseTo(1, 10);
    });

    it("keeps blocks reasonably square (squarified)", () => {
        const r = layoutTreemap(Array.from({length: 12}, (_, i) => e(`f${i}`, 100 - i * 5)), 600, 400, {minArea: 0});
        for (const c of r.cells) expect(Math.max(c.rect.w / c.rect.h, c.rect.h / c.rect.w)).toBeLessThan(5);
    });
});

describe("treemap: the small tail", () => {
    it("merges entries below the minimum area into one 'other' block that is last in rank", () => {
        const entries = [e("big", 1000), e("mid", 500), ...Array.from({length: 20}, (_, i) => e(`t${i}`, 2))];
        const r = layoutTreemap(entries, 400, 300);
        const other = r.cells.filter((c) => c.kind === "other");
        expect(other).toHaveLength(1);
        expect(other[0].merged).toHaveLength(20);
        expect(other[0].rank).toBeGreaterThan(r.cells.filter((c) => c.kind === "item").at(-1)!.rank);
        expect(other[0].size).toBe(40);
        expect(r.cells.map((c) => c.name).filter(Boolean)).toEqual(["big", "mid"]);
        expect(coverage(r.cells, 400, 300)).toEqual({gaps: 0, overlaps: 0});
    });

    it("does not make an 'other' block when everything is big enough", () => {
        expect(layoutTreemap([e("a", 1), e("b", 1)], 400, 300).cells.every((c) => c.kind === "item")).toBe(true);
    });

    it("never leaves a block under the minimum area, 'other' included, for thousands of entries", () => {
        const rnd = mulberry(3);
        const entries = Array.from({length: 5000}, (_, i) => e(`n${i}`, i < 12 ? 1e8 + i : Math.ceil(rnd() * 1e5)));
        const r = layoutTreemap(entries, 700, 420);
        const px = 700 * 420;
        expect(r.cells.filter((c) => c.kind === "other")).toHaveLength(1);
        expect(r.cells.length).toBeLessThanOrEqual(60);
        expect(r.cells.length).toBeGreaterThan(5);
        expect(coverage(r.cells, 700, 420)).toEqual({gaps: 0, overlaps: 0});
        for (const c of r.cells) expect(c.share * px).toBeGreaterThan(1800 * 0.9);
        const merged = r.cells.reduce((n, c) => n + c.merged.length, 0);
        expect(r.cells.filter((c) => c.kind === "item").length + merged).toBe(5000);
    });

    it("caps the number of blocks", () => {
        const r = layoutTreemap(Array.from({length: 200}, (_, i) => e(`f${i}`, 100)), 2000, 1000, {minArea: 0, maxBlocks: 30});
        expect(r.cells).toHaveLength(30);
        expect(r.cells.filter((c) => c.kind === "other")).toHaveLength(1);
        expect(r.cells.find((c) => c.kind === "other")!.merged).toHaveLength(200 - 29);
    });

    it("a single tiny entry beside a big one still tiles", () => {
        const r = layoutTreemap([e("a", 1e9), e("b", 1)], 300, 200);
        expect(coverage(r.cells, 300, 200)).toEqual({gaps: 0, overlaps: 0});
    });
});

describe("treemap: speed", () => {
    it("lays out 20000 entries well inside a frame budget", () => {
        const rnd = mulberry(11);
        const entries = Array.from({length: 20000}, (_, i) => e(`n${i}`, Math.ceil(rnd() * 1e9)));
        const t0 = performance.now();
        const r = layoutTreemap(entries, 1000, 600);
        expect(performance.now() - t0).toBeLessThan(500);
        expect(r.cells.length).toBeLessThanOrEqual(60);
    });
});

describe("treemap helpers", () => {
    it("reading order is top to bottom, then left to right", () => {
        const cells = [{rect: {x: 5, y: 10, w: 1, h: 1}, n: 3}, {rect: {x: 9, y: 0, w: 1, h: 1}, n: 2}, {rect: {x: 0, y: 0, w: 1, h: 1}, n: 1}];
        expect(readingOrder(cells).map((c) => c.n)).toEqual([1, 2, 3]);
    });

    it("tints by rank in five steps, only the first for a lone block", () => {
        expect(tintOf(0, 1)).toBe(0);
        expect([0, 1, 2, 3, 4, 5, 6, 7, 8, 9].map((r) => tintOf(r, 10))).toEqual([0, 0, 1, 1, 2, 2, 3, 3, 4, 4]);
        expect(tintOf(99, 10)).toBe(4);
    });

    it("says whether a label fits", () => {
        expect(labelFits({x: 0, y: 0, w: 200, h: 80}, 2)).toBe(true);
        expect(labelFits({x: 0, y: 0, w: 40, h: 80}, 1)).toBe(false);
        expect(labelFits({x: 0, y: 0, w: 200, h: 30}, 2)).toBe(false);
        expect(labelFits({x: 0, y: 0, w: 200, h: 30}, 1)).toBe(true);
    });
});
