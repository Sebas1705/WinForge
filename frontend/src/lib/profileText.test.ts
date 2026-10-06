import {describe, expect, it} from "vitest";
import {readdirSync, readFileSync} from "node:fs";
import {resolve} from "node:path";
import {PROFILE_IDS, profileText} from "./profileText";

// Every profile id the catalog defines must have plain-language text.
const dir = resolve(__dirname, "../../../catalogdata/profiles");
const catalogIds = readdirSync(dir).filter((f) => f.endsWith(".yml"))
    .flatMap((f) => [...readFileSync(resolve(dir, f), "utf8").matchAll(/^- id: (\S+)/gm)].map((m) => m[1]));

describe("profile text", () => {
    it("covers every built-in profile, and nothing that does not exist", () => {
        expect(catalogIds.length).toBeGreaterThanOrEqual(30);
        expect(catalogIds.filter((id) => !PROFILE_IDS.includes(id))).toEqual([]);
        expect(PROFILE_IDS.filter((id) => !catalogIds.includes(id))).toEqual([]);
    });
    it("returns the plain text in the requested language", () => {
        const p = {id: "gaming", name: "Gaming", description: "Game launchers and chat.", builtin: true};
        expect(profileText(p, "es").name).toBe("Juegos");
        expect(profileText(p, "en").desc).toBe("Game stores and chat.");
    });
    it("leaves profiles the user made untouched, even if the id matches a built-in", () => {
        const mine = {id: "gaming", name: "My games", description: "Mine", builtin: false};
        expect(profileText(mine, "es")).toEqual({name: "My games", desc: "Mine"});
    });
    it("falls back to the catalog text for an unknown built-in", () => {
        expect(profileText({id: "new-one", name: "New", description: "Fresh", builtin: true}, "es")).toEqual({name: "New", desc: "Fresh"});
    });
});
