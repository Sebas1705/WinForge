import {beforeEach, describe, expect, it} from "vitest";
import type {CleanCategory, DupGroup} from "../api";
import {setLang} from "./i18n";
import {
    categoryHint, categoryName, defaultRemovals, fileCount, folderName, groupWaste, initialSelection, keptIn, oldestPath, removalTotals,
    sortGroups, splitPath, toggleRemoval, visibleCategories,
} from "./storage";

const cat = (id: string, o: Partial<CleanCategory> = {}): CleanCategory => ({id, risk: "safe", needsAdmin: false, available: true, bytes: 100, files: 1, skipped: 0, ...o});
const group = (id: string, sizeBytes: number, files: [string, string][]): DupGroup => ({id, sizeBytes, files: files.map(([path, modified]) => ({path, modified}))});

beforeEach(() => setLang("en"));

describe("cleanup categories", () => {
    const all = [cat("a"), cat("b", {available: false}), cat("c", {bytes: 0}), cat("d", {risk: "review"}), cat("e", {bytes: -5})];
    it("shows only available, non-empty ones", () => {
        expect(visibleCategories(all).map((c) => c.id)).toEqual(["a", "d"]);
    });
    it("pre-ticks the safe ones and never a review one", () => {
        expect([...initialSelection(all)]).toEqual(["a"]);
    });
    it("names known ids and falls back to the id for an unknown one", () => {
        expect(categoryName("recycle-bin")).toBe("Recycle Bin");
        expect(categoryHint("recycle-bin")).not.toBe("");
        expect(categoryName("future-thing")).toBe("future-thing");
        expect(categoryHint("future-thing")).toBe("");
        setLang("es");
        expect(categoryName("recycle-bin")).toBe("Papelera de reciclaje");
    });
    it("counts files with the right plural", () => {
        expect(fileCount(0)).toBe("0 files");
        expect(fileCount(1)).toBe("1 file");
        expect(fileCount(2)).toBe("2 files");
        setLang("es");
        expect(fileCount(1)).toBe("1 archivo");
    });
});

describe("paths", () => {
    it("splits Windows and POSIX paths", () => {
        expect(splitPath("C:\\Users\\me\\a.iso")).toEqual({dir: "C:\\Users\\me", name: "a.iso"});
        expect(splitPath("/home/me/a.iso")).toEqual({dir: "/home/me", name: "a.iso"});
        expect(splitPath("a.iso")).toEqual({dir: "", name: "a.iso"});
    });
    it("names a folder by its last segment, and a drive root by itself", () => {
        expect(folderName("C:\\Users\\me\\Videos")).toBe("Videos");
        expect(folderName("C:\\Users\\me\\Videos\\")).toBe("Videos");
        expect(folderName("C:\\")).toBe("C:");
        expect(folderName("D:")).toBe("D:");
    });
});

describe("duplicate groups", () => {
    const g = group("g", 100, [["C:\\new.bin", "2024-05-01T00:00:00Z"], ["C:\\old.bin", "2020-01-01T00:00:00Z"], ["C:\\mid.bin", "2022-01-01T00:00:00Z"]]);

    it("computes the waste as size times the extra copies", () => {
        expect(groupWaste(g)).toBe(200);
        expect(groupWaste(group("x", 50, [["a", ""], ["b", ""]]))).toBe(50);
    });
    it("orders groups by waste, biggest first, keeping the order of ties", () => {
        const small = group("small", 10, [["a", ""], ["b", ""]]);
        const many = group("many", 10, [["a", ""], ["b", ""], ["c", ""], ["d", ""]]); // waste 30
        const big = group("big", 1000, [["a", ""], ["b", ""]]);
        const tie = group("tie", 10, [["a", ""], ["b", ""]]);
        expect(sortGroups([small, many, big, tie]).map((x) => x.id)).toEqual(["big", "many", "small", "tie"]);
    });
    it("keeps the oldest copy by default", () => {
        expect(oldestPath(g)).toBe("C:\\old.bin");
        expect([...defaultRemovals([g])].sort()).toEqual(["C:\\mid.bin", "C:\\new.bin"]);
    });
    it("keeps the first listed copy on equal dates and never prefers an unreadable date", () => {
        expect(oldestPath(group("t", 1, [["a", "2020-01-01T00:00:00Z"], ["b", "2020-01-01T00:00:00Z"]]))).toBe("a");
        expect(oldestPath(group("u", 1, [["a", "garbage"], ["b", "2021-01-01T00:00:00Z"]]))).toBe("b");
        expect(oldestPath(group("v", 1, [["a", "garbage"], ["b", "also bad"]]))).toBe("a");
    });
    it("lets the person move the kept copy but never mark them all", () => {
        let removed = defaultRemovals([g]);
        expect(keptIn(g, removed)).toEqual(["C:\\old.bin"]);
        // the last kept copy cannot be marked
        expect(toggleRemoval(g, removed, "C:\\old.bin")).toEqual(removed);
        // unmark another one, then the old one can go
        removed = toggleRemoval(g, removed, "C:\\new.bin");
        expect(keptIn(g, removed)).toEqual(["C:\\new.bin", "C:\\old.bin"]);
        removed = toggleRemoval(g, removed, "C:\\old.bin");
        expect(keptIn(g, removed)).toEqual(["C:\\new.bin"]);
        expect(toggleRemoval(g, removed, "C:\\new.bin").size).toBe(removed.size);
        // never an empty keep list, whatever the clicks
        for (const f of g.files) removed = toggleRemoval(g, removed, f.path);
        expect(keptIn(g, removed).length).toBeGreaterThanOrEqual(1);
    });
    it("does not touch the input set", () => {
        const before = defaultRemovals([g]);
        toggleRemoval(g, before, "C:\\new.bin");
        expect(before.size).toBe(2);
    });
    it("adds up what is marked", () => {
        const h = group("h", 10, [["x1", ""], ["x2", ""]]);
        expect(removalTotals([g, h], new Set(["C:\\new.bin", "C:\\mid.bin", "x2"]))).toEqual({files: 3, bytes: 210});
        expect(removalTotals([g, h], new Set())).toEqual({files: 0, bytes: 0});
    });
});
