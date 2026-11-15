import {describe, expect, it} from "vitest";
import {fold, rank, score} from "./palette";

describe("folding", () => {
    it("ignores case and accents", () => {
        expect(fold("Catálogo")).toBe("catalogo");
        expect(fold("COPIA DE AJUSTES")).toBe("copia de ajustes");
        expect(fold("Ñandú")).toBe("nandu");
    });
});

describe("scoring one word", () => {
    it("orders whole, prefix, word start, substring and scattered letters", () => {
        const s = (w: string, t: string) => score(w, t);
        expect(s("scan", "Scan")).toBe(100);
        expect(s("sc", "Scan this PC")).toBe(90);
        expect(s("pc", "Scan this PC")).toBe(75);
        expect(s("can", "Scan this PC")).toBe(60);
        expect(s("sth", "Scan this PC")).toBe(25);
        expect(s("zzz", "Scan this PC")).toBe(0);
        expect([100, 90, 75, 60, 25].every((v, i, a) => i === 0 || a[i - 1] > v)).toBe(true);
    });

    it("does not accept scattered letters for one or two letters", () => {
        expect(score("sp", "Scan this PC")).toBe(0);
        expect(score("c", "xyz")).toBe(0);
    });

    it("treats punctuation in the query as plain text, never as a pattern", () => {
        expect(score("c++", "C++ tools")).toBe(90);
        expect(score(".net", "Install .NET SDK")).toBe(75);
        expect(score("(", "a (b)")).toBeGreaterThan(0);
        expect(() => score("[", "x")).not.toThrow();
        expect(score("", "anything")).toBe(0);
    });

    it("finds Spanish words without typing the accent", () => {
        expect(score("catalogo", "Catálogo")).toBe(100);
        expect(score("actualiz", "Actualizaciones")).toBe(90);
    });
});

describe("ranking", () => {
    const items = [
        {label: "Go to Catalog"},
        {label: "Scan this PC again"},
        {label: "Check for updates", keywords: "upgrade winget"},
        {label: "Run a PC health scan"},
        {label: "Open appearance settings", keywords: "theme dark light style"},
    ];

    it("keeps everything in order for an empty query", () => {
        expect(rank(items, "")).toEqual(items);
        expect(rank(items, "   ")).toEqual(items);
        expect(rank(items, "", 2)).toEqual(items.slice(0, 2));
    });

    it("needs every word to match, in any order", () => {
        expect(rank(items, "scan pc").map((i) => i.label)).toEqual(["Scan this PC again", "Run a PC health scan"]);
        expect(rank(items, "pc scan").map((i) => i.label).sort()).toEqual(["Run a PC health scan", "Scan this PC again"].sort());
        expect(rank(items, "scan banana")).toEqual([]);
    });

    it("finds by keywords too, but ranks a label match higher", () => {
        expect(rank(items, "theme").map((i) => i.label)).toEqual(["Open appearance settings"]);
        const both = [{label: "Other", keywords: "scan"}, {label: "Scan"}];
        expect(rank(both, "scan").map((i) => i.label)).toEqual(["Scan", "Other"]);
    });

    it("puts the best match first and keeps ties in their original order", () => {
        const list = [{label: "Update all"}, {label: "Updates"}, {label: "Check for updates"}];
        expect(rank(list, "updates").map((i) => i.label)).toEqual(["Updates", "Check for updates"]);
        const ties = [{label: "Alpha one"}, {label: "Alpha two"}, {label: "Alpha three"}];
        expect(rank(ties, "alpha").map((i) => i.label)).toEqual(["Alpha one", "Alpha two", "Alpha three"]);
    });

    it("honours the limit after ranking", () => {
        const many = Array.from({length: 20}, (_, i) => ({label: `Item ${i}`}));
        expect(rank(many, "item", 5)).toHaveLength(5);
    });

    it("does not mutate what it is given", () => {
        const copy = items.map((i) => ({...i}));
        rank(items, "scan");
        expect(items).toEqual(copy);
    });
});
