import {describe, expect, it} from "vitest";
import {dayOf, withApps} from "./drift";
import type {Profile} from "./model";

const mine: Profile = {id: "mine", name: "Mine", description: "d", kind: "custom", extends: ["base"], recipes: ["r"], apps: [{id: "git", version: "2.0"}, {id: "vlc"}]};

describe("adding apps to a profile", () => {
    it("keeps everything that was there, in order, and adds the new ones sorted", () => {
        const p = withApps(mine, ["zed", "7zip"]);
        expect(p.apps).toEqual([{id: "git", version: "2.0"}, {id: "vlc"}, {id: "7zip"}, {id: "zed"}]);
        expect(p).toMatchObject({id: "mine", name: "Mine", description: "d", kind: "custom", extends: ["base"], recipes: ["r"]});
    });

    it("never adds an app twice, and keeps a pinned version", () => {
        const p = withApps(mine, ["git", "git", "new", "new"]);
        expect(p.apps).toEqual([{id: "git", version: "2.0"}, {id: "vlc"}, {id: "new"}]);
    });

    it("works for a profile with no apps of its own, and does not change its input", () => {
        const before = JSON.stringify(mine);
        expect(withApps({id: "x", name: "X"}, ["a"]).apps).toEqual([{id: "a"}]);
        expect(withApps({id: "x", name: "X"}, []).apps).toEqual([]);
        withApps(mine, ["q"]);
        expect(JSON.stringify(mine)).toBe(before);
    });

    it("drops the fields the interface adds, so only a profile is saved", () => {
        const withExtras = {...mine, builtin: false, resolved: ["git"], resolvedRecipes: [], changes: 3} as Profile;
        expect(Object.keys(withApps(withExtras, ["a"])).sort()).toEqual(["apps", "description", "extends", "id", "kind", "name", "recipes"]);
    });
});

describe("naming a day", () => {
    it("formats in the language asked for", () => {
        expect(dayOf("2026-11-15T09:00:00Z", "en")).toMatch(/2026/);
        expect(dayOf("2026-11-15T09:00:00Z", "es")).toMatch(/2026/);
    });

    it("is empty for nothing or nonsense, never 'Invalid Date'", () => {
        expect(dayOf(undefined, "en")).toBe("");
        expect(dayOf("", "en")).toBe("");
        expect(dayOf("not a date", "en")).toBe("");
    });
});
