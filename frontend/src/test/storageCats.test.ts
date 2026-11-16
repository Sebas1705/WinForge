import {describe, expect, it} from "vitest";
import {DICTS, KEYS} from "../lib/i18n";

// The cleanup categories the Go side reports. Each needs a name and a one-line hint in both
// languages; an id missing here still renders (as its raw id), but never should for these.
const IDS = [
    "temp-user", "temp-windows", "recycle-bin", "thumbnails", "crash-dumps", "delivery-optimization", "windows-update", "windows-old",
    "cache-chrome", "cache-edge", "cache-firefox", "cache-brave", "shader-cache", "dev-npm", "dev-pip", "dev-nuget", "dev-go", "dev-gradle",
    "dev-yarn", "dev-pnpm", "dev-cargo", "dev-gradle-wrapper", "dev-android", "docker-build-cache", "docker-dangling",
];
const OPTIMIZE = ["optimize-trim", "optimize-defrag", "optimize-component-cleanup", "optimize-hibernate-off"];
const keys = new Set(KEYS as string[]);
const text = (lang: "en" | "es", k: string) => (DICTS[lang] as Record<string, string>)[k];

describe("storage cleanup categories", () => {
    it("each have a name and a hint, in both languages", () => {
        const missing = IDS.flatMap((id) => [`storage.cat.${id}`, `storage.cat.${id}.hint`]).filter((k) => !keys.has(k));
        expect(missing).toEqual([]);
        for (const lang of ["en", "es"] as const) {
            for (const id of IDS) {
                for (const k of [`storage.cat.${id}`, `storage.cat.${id}.hint`]) expect(text(lang, k), `${lang} ${k}`).toBeTruthy();
            }
        }
    });
    it("have a Spanish text that is not the English one", () => {
        const same = IDS.flatMap((id) => [`storage.cat.${id}`, `storage.cat.${id}.hint`]).filter((k) => text("en", k) === text("es", k));
        expect(same).toEqual([]);
    });
    it("leave no key in the dictionary for a category that is not in the list", () => {
        const extra = KEYS.filter((k) => k.startsWith("storage.cat.") && !k.endsWith(".hint")).map((k) => k.slice("storage.cat.".length)).filter((id) => !IDS.includes(id));
        expect(extra).toEqual([]);
    });
});

describe("storage maintenance steps", () => {
    it("are named for the run dialog in both languages", () => {
        for (const lang of ["en", "es"] as const) {
            for (const id of OPTIMIZE) expect(text(lang, `repair.${id}`), `${lang} ${id}`).toBeTruthy();
        }
    });
});
