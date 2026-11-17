import {readFileSync, readdirSync} from "node:fs";
import {resolve} from "node:path";
import {describe, expect, it} from "vitest";

const src = resolve(__dirname, "..");
const css = ["style.css", "visual.css"].map((f) => readFileSync(resolve(src, f), "utf8")).join("\n");

describe("page containers", () => {
    // A view's root is `<div className="page <name>">`. If a CSS rule for `.<name>` alone sets
    // padding, margin or a grid, it hits the whole page: the "games" list once did, and the
    // page lost its padding and margins.
    const modifiers = new Set<string>();
    for (const f of readdirSync(resolve(src, "views")).filter((x) => x.endsWith(".tsx"))) {
        for (const m of readFileSync(resolve(src, "views", f), "utf8").matchAll(/className="page ([a-z-]+)"/g)) modifiers.add(m[1]);
    }

    it("finds the page modifiers", () => {
        expect(modifiers.size).toBeGreaterThanOrEqual(4);
        expect([...modifiers]).toContain("gamespage");
    });

    it("never styles a page modifier alone in a way that overrides the page's own spacing", () => {
        const offenders: string[] = [];
        for (const name of modifiers) {
            const rule = new RegExp(`(^|[},\\s])\\.${name}\\s*\\{([^}]*)\\}`, "g");
            for (const m of css.matchAll(rule)) {
                if (/\b(padding|margin|display\s*:\s*grid|list-style)\b/.test(m[2])) offenders.push(`.${name} { ${m[2].trim().slice(0, 60)} }`);
            }
        }
        expect(offenders).toEqual([]);
    });
});

describe("the window never scrolls as a whole", () => {
    // Screen-reader text and ring svgs are absolutely positioned. Inside a page that is not positioned they
    // belong to the document and push its height past the window, so the whole app scrolled and showed the
    // view cut in two (on Home). The document must not scroll, and pages must contain what is positioned in them.
    it("hides the document's own overflow", () => {
        expect(css).toMatch(/html,\s*body\s*\{\s*overflow:\s*hidden;?\s*\}/);
    });
    it("makes the page scrollers the containing block of what is positioned inside them", () => {
        expect(css).toMatch(/main,\s*\.page,\s*\.detail\s*\{\s*position:\s*relative;?\s*\}/);
    });
});
