import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {describe, expect, it} from "vitest";
import {DICTS, KEYS} from "../lib/i18n";

// Every settings set the Go side knows needs a name and a line of help in both languages.
const go = readFileSync(resolve(__dirname, "../../../internal/settings/sets.go"), "utf8");
const ids = [...go.matchAll(/ID: "([a-z-]+)"/g), ...go.matchAll(/vscodeLike\("([a-z-]+)"/g)].map((m) => m[1]);
const keys = new Set(KEYS as string[]);

describe("settings backup sets", () => {
    it("are found in the Go source", () => {
        expect(ids.length).toBeGreaterThanOrEqual(25);
    });
    it("each have a name and a hint, in both languages", () => {
        const missing = ids.flatMap((id) => [`backup.set.${id}`, `backup.set.${id}.hint`]).filter((k) => !keys.has(k));
        expect(missing).toEqual([]);
        for (const lang of ["en", "es"] as const) {
            for (const id of ids) expect(DICTS[lang][`backup.set.${id}` as never], `${lang} ${id}`).toBeTruthy();
        }
    });
    it("have an icon in the backup page", () => {
        const page = readFileSync(resolve(__dirname, "../views/Backup.tsx"), "utf8");
        const lacking = ids.filter((id) => !new RegExp(`(^|[\\s,{"])"?${id.replace("-", "\\-")}"?:`).test(page.slice(0, page.indexOf("const size"))));
        expect(lacking).toEqual([]);
    });
});
