import {describe, expect, it} from "vitest";
import {applyEvent, categories, filterApps, selectionProfile, slugify, type App} from "./model";

const app = (o: Partial<App>): App => ({
    id: "x", name: "X", category: "dev/vcs", description: "", homepage: "https://x",
    winget: "X.X", publisher: "P", installed: false, openSource: false, ...o,
});

describe("filterApps", () => {
    const apps = [
        app({id: "git", name: "Git", category: "dev/vcs", installed: true}),
        app({id: "vlc", name: "VLC media player", category: "media", publisher: "VideoLAN"}),
    ];
    it("matches name, publisher and winget id case-insensitively", () => {
        expect(filterApps(apps, {query: "videolan", category: "", installed: "all", openSourceOnly: false})).toHaveLength(1);
        expect(filterApps(apps, {query: "GIT", category: "", installed: "all", openSourceOnly: false})[0].id).toBe("git");
    });
    it("filters by top-level category and installed state", () => {
        expect(filterApps(apps, {query: "", category: "dev", installed: "all", openSourceOnly: false})).toHaveLength(1);
        expect(filterApps(apps, {query: "", category: "", installed: "missing", openSourceOnly: false})[0].id).toBe("vlc");
    });
    it("can restrict to open-source apps", () => {
        const list = [app({id: "a", openSource: true}), app({id: "b"})];
        expect(filterApps(list, {query: "", category: "", installed: "all", openSourceOnly: true}).map((a) => a.id)).toEqual(["a"]);
    });
    it("lists top-level categories once", () => {
        expect(categories([...apps, app({category: "dev/cli"})])).toEqual(["dev", "media"]);
    });
});

describe("profiles", () => {
    it("slugifies names, accents included", () => {
        expect(slugify("Mi Entorno Dev!")).toBe("mi-entorno-dev");
        expect(slugify("Diseño Gráfico")).toBe("diseno-grafico");
        expect(slugify("!!!")).toBe("profile");
    });
    it("builds a sorted selection profile", () => {
        expect(selectionProfile(["vlc", "git"], "My PC").apps?.map((a) => a.id)).toEqual(["git", "vlc"]);
    });
});

describe("applyEvent", () => {
    const step = {kind: "app" as const, id: "git", name: "Git"};
    it("moves through the lifecycle and ignores output lines", () => {
        expect(applyEvent("pending", {step, status: "start"})).toBe("running");
        expect(applyEvent("running", {step, status: "output", line: "…"})).toBe("running");
        expect(applyEvent("running", {step, status: "failed"})).toBe("failed");
    });
});
