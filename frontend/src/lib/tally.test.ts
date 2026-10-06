import {describe, expect, it} from "vitest";
import type {App, ProfileInfo} from "./model";
import {avatarHue, categoryStats, closestToDone, formatElapsed, initials, profileTally, rulerOrder, sortApps} from "./tally";

const app = (id: string, category: string, installed = false): App => ({
    id, name: id.toUpperCase(), category, description: "", homepage: "https://x", winget: id, publisher: "P",
    installed, openSource: false,
});
const apps = [app("git", "dev/vcs", true), app("vscode", "dev/editors", true), app("go", "dev/languages"),
    app("vlc", "media", true), app("gimp", "design")];
const byId = new Map(apps.map((a) => [a.id, a]));
const profile = (id: string, resolved: string[]): ProfileInfo =>
    ({id, name: id, builtin: true, resolved, resolvedRecipes: []});

describe("tally", () => {
    it("counts installed cells and ignores unknown ids", () => {
        const t = profileTally(["git", "go", "nope"], byId);
        expect(t.have).toBe(1);
        expect(t.total).toBe(2);
        expect(t.cells.map((c) => c.on)).toEqual([true, false]);
    });
    it("summarises categories biggest first", () => {
        expect(categoryStats(apps)).toEqual([
            {top: "dev", have: 2, total: 3}, {top: "design", have: 0, total: 1}, {top: "media", have: 1, total: 1},
        ]);
    });
    it("orders the ruler by category size then name", () => {
        expect(rulerOrder(apps).map((a) => a.id)).toEqual(["git", "go", "vscode", "gimp", "vlc"]);
    });
});

describe("suggestions", () => {
    it("offers started, unfinished profiles, closest first", () => {
        const s = closestToDone([
            profile("empty", ["go", "gimp"]),
            profile("done", ["git", "vscode"]),
            profile("almost", ["git", "vscode", "go"]),
            profile("half", ["git", "go", "gimp"]),
        ], byId);
        expect(s.map((x) => x.profile.id)).toEqual(["almost", "half"]);
        expect(s[0]).toMatchObject({have: 2, total: 3, missing: 1});
    });
});

describe("helpers", () => {
    it("sorts apps three ways", () => {
        expect(sortApps(apps, "missing").slice(0, 2).map((a) => a.id)).toEqual(["gimp", "go"]);
        expect(sortApps(apps, "category")[0].category).toBe("design");
        expect(sortApps(apps, "name").map((a) => a.id)).toEqual(["gimp", "git", "go", "vlc", "vscode"]);
    });
    it("derives a stable avatar hue per top-level category", () => {
        expect(avatarHue("dev/vcs")).toBe(avatarHue("dev/editors"));
        expect(avatarHue("media")).not.toBe(avatarHue("dev"));
        expect(avatarHue("media")).toBeLessThan(360);
    });
    it("makes initials and elapsed time", () => {
        expect(initials("Visual Studio Code")).toBe("VS");
        expect(initials("7-Zip")).toBe("7Z");
        expect(initials("Git")).toBe("GI");
        expect(initials("!!!")).toBe("?");
        expect(formatElapsed(65_000)).toBe("1:05");
    });
});

describe("cards", () => {
    const withTag = {...app("git", "dev/vcs"), tagline: {en: "Version control", es: "Control de versiones"}};
    const list = [withTag, app("vlc", "media"), app("gimp", "design")];
    it("prefers the tagline in the interface language and falls back to the description", async () => {
        const {taglineFor} = await import("./tally");
        expect(taglineFor(withTag, "es")).toBe("Control de versiones");
        expect(taglineFor(withTag, "en")).toBe("Version control");
        expect(taglineFor({...list[1], description: "From the manifest"}, "es")).toBe("From the manifest");
    });
    it("lists featured apps in featured order, skipping ids that do not exist", async () => {
        const {featuredApps, profileApps} = await import("./tally");
        expect(featuredApps(list, ["gimp", "nope", "git"]).map((a) => a.id)).toEqual(["gimp", "git"]);
        expect(featuredApps(list, ["gimp", "git", "vlc"], 2)).toHaveLength(2);
        expect(profileApps(["vlc", "zzz", "git"], new Map(list.map((a) => [a.id, a]))).map((a) => a.id)).toEqual(["vlc", "git"]);
    });
});
