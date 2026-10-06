import {describe, expect, it} from "vitest";
import {iconUrl, loadIcons} from "./icons";

const respond = (body: unknown, ok = true) => (async () => ({ok, json: async () => body})) as unknown as typeof fetch;

describe("icons", () => {
    it("keeps only string entries of the index", async () => {
        const idx = await loadIcons(respond({git: "git.ico", bad: 5, nested: {a: 1}}));
        expect(idx).toEqual({git: "git.ico"});
    });
    it("treats network errors, bad status and bad json as no icons", async () => {
        expect(await loadIcons((async () => { throw new Error("offline"); }) as unknown as typeof fetch)).toEqual({});
        expect(await loadIcons(respond({}, false))).toEqual({});
        expect(await loadIcons(respond(null))).toEqual({});
    });
    it("builds escaped urls and returns null for unknown apps", () => {
        expect(iconUrl({a: "a b.png"}, "a")).toBe("/icons/a%20b.png");
        expect(iconUrl({}, "zzz")).toBeNull();
    });
});
