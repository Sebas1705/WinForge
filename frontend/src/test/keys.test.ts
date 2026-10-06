import {readdirSync, readFileSync} from "node:fs";
import {resolve} from "node:path";
import {describe, expect, it} from "vitest";
import {DICTS, KEYS} from "../lib/i18n";

// Some keys are built at run time (home.tile.<profile>, group.<group>, link.<kind>, cat.<category>).
// TypeScript cannot check those, so this does: each must exist in both languages.
const keys = new Set(KEYS as string[]);
const apps = resolve(__dirname, "../../../catalogdata/apps");
const topCategories = [...new Set(readdirSync(apps).filter((f) => f.endsWith(".yml"))
    .flatMap((f) => [...readFileSync(resolve(apps, f), "utf8").matchAll(/^ {2}category: (\S+)/gm)].map((m) => m[1].split("/")[0])))];

describe("dynamic interface keys", () => {
    it("has a label for every category in the catalog", () => {
        expect(topCategories.length).toBeGreaterThanOrEqual(12);
        expect(topCategories.filter((c) => !keys.has(`cat.${c}`))).toEqual([]);
    });
    it("has the quick-start tile labels, health groups and link kinds", () => {
        const wanted = [
            ...["everyday", "gaming", "dev-base", "creator", "privacy", "opensource"].map((id) => `home.tile.${id}`),
            ...["firmware", "drivers", "storage", "security", "windows"].map((g) => `group.${g}`),
            ...["support", "download", "search", "settings"].map((k) => `link.${k}`),
        ];
        expect(wanted.filter((k) => !keys.has(k))).toEqual([]);
    });
    it("never leaves a key empty in either language", () => {
        for (const lang of ["en", "es"] as const) {
            const empty = KEYS.filter((k) => !DICTS[lang][k].trim());
            expect(empty, `${lang} has empty strings`).toEqual([]);
        }
    });
});
