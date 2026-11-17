import {beforeEach, describe, expect, it} from "vitest";
import type {CompareResult, CompareSide} from "../api";
import {compareError, kindOf, saveName, sideLabel, summary} from "./compare";
import {setLang} from "./i18n";

const side = (o: Partial<CompareSide> = {}): CompareSide => ({
    kind: "pc", name: "", file: "", apps: 0, versioned: false, unknownRecipes: [], unknownProfiles: [], profile: null, ...o,
});
const res = (o: Partial<CompareResult> = {}): CompareResult => ({
    a: side(), b: side({kind: "file", name: "Laptop", file: "laptop.winforge.json"}),
    onlyA: [], onlyB: [], both: [], differ: [], unknown: [], ...o,
});
const apps = (...ids: string[]) => ids.map((id) => ({id, name: id}));

beforeEach(() => setLang("en"));

describe("kindOf", () => {
    it("tells the three comparisons apart", () => {
        expect(kindOf(res())).toBe("file");
        expect(kindOf(res({b: side({kind: "code"})}))).toBe("code");
        expect(kindOf(res({a: side({kind: "file"})}))).toBe("files");
    });
});

describe("compareError", () => {
    it("words Go's codes and leaves other failures visible", () => {
        expect(compareError(new Error("compare:too-big"))).toBe("That is too big to be a profile.");
        expect(compareError(new Error("compare:nope"))).toBe("The comparison failed: compare:nope");
        expect(compareError("plain")).toBe("The comparison failed: plain");
        setLang("es");
        expect(compareError(new Error("compare:not-code"))).toContain("código para compartir");
    });
});

describe("labels", () => {
    it("names a side in a sentence", () => {
        expect(sideLabel(side(), "a", "file")).toBe("this PC");
        expect(sideLabel(res().b, "b", "file")).toBe("the file “Laptop”");
        expect(sideLabel(side({kind: "code"}), "b", "code")).toBe("the share code");
        expect(sideLabel(side({kind: "file", name: "Desk"}), "a", "files")).toBe("file A “Desk”");
    });
    it("names a saved copy after the profile, then the file, then the code", () => {
        expect(saveName(side({name: "Laptop", file: "x.json"}))).toBe("Laptop");
        expect(saveName(side({file: "laptop.winforge.json"}))).toBe("laptop.winforge");
        expect(saveName(side({kind: "code"}))).toBe("the share code");
    });
});

describe("summary", () => {
    it("counts each group", () => {
        expect(summary(res({onlyA: apps("a"), onlyB: apps("b", "c"), both: apps("d")}))).toBe("In both: 1. Only with this PC: 1. Only with the file “Laptop”: 2.");
    });
    it("adds the versions and the unknown ids", () => {
        const s = summary(res({both: apps("d"), differ: [{id: "g", name: "g", a: "1", b: "2"}], unknown: [{id: "x", in: "b"}]}));
        expect(s).toContain("Versions differ for 1.");
        expect(s).toContain("Not in this catalog: 1.");
    });
    it("says the sides match, or that there is nothing to compare", () => {
        expect(summary(res({both: apps("a", "b")}))).toBe("Both sides have the same 2 apps.");
        expect(summary(res())).toBe("Neither side has any catalog apps, so there is nothing to compare.");
    });
});
