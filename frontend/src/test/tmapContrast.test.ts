import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {describe, expect, it} from "vitest";
import {ACCENTS} from "../lib/settings";

// Contrast of the space map's label text against every block fill, computed from the real CSS tokens in
// tmap.css for each accent, both themes and all six design styles. The map's tokens are solid colours that
// do not depend on the style, so what must hold is that no style (or other stylesheet) redefines them.

const src = resolve(__dirname, "..");
const read = (f: string) => readFileSync(resolve(src, f), "utf8");
const tmap = read("tmap.css");

const rgb = (hex: string): [number, number, number] => {
    const m = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex.trim());
    if (!m) throw new Error(`not a #rrggbb colour: ${hex}`);
    return [parseInt(m[1], 16), parseInt(m[2], 16), parseInt(m[3], 16)];
};
const lin = (v: number) => { const c = v / 255; return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4; };
const lum = ([r, g, b]: number[]) => 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
const contrast = (a: number[], b: number[]) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + 0.05) / (y + 0.05); };
/** color-mix(in srgb, a p%, b): a straight blend of the channels. */
const mix = (a: number[], pct: number, b: number[]) => a.map((v, i) => (v * pct) / 100 + (b[i] * (100 - pct)) / 100);

/** Declarations of the rule whose selector is exactly `selector`. */
function block(selector: string): string {
    const esc = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const m = new RegExp(`(?:^|\\})\\s*${esc}\\s*\\{([^}]*)\\}`, "m").exec(tmap);
    if (!m) throw new Error(`no rule for ${selector}`);
    return m[1];
}
const token = (decls: string, name: string): string => {
    const m = new RegExp(`${name}:\\s*([^;]+);`).exec(decls);
    if (!m) throw new Error(`no ${name}`);
    return m[1].trim();
};
const pct = (decls: string, name: string): number => parseFloat(token(decls, name));

const base = block(".tmap");
const themes = {
    dark: {base: rgb(token(base, "--tm-base")), ink: rgb(token(base, "--tm-ink"))},
    light: (() => { const d = block(":root[data-theme=light] .tmap"); return {base: rgb(token(d, "--tm-base")), ink: rgb(token(d, "--tm-ink"))}; })(),
};
const tints = [0, 1, 2, 3, 4].map((i) => pct(base, `--tm-t${i}`));
const other = pct(base, "--tm-other");
const hatch = pct(base, "--tm-hatch");

// every accent a person can pick, plus the extremes in case a custom colour ever appears
const accents: [string, string][] = [...ACCENTS.map((a): [string, string] => [a.name, a.value]), ["white", "#ffffff"], ["black", "#000000"]];
const styles = ["classic", "minimal", "material", "liquid", "neumorphic", "brutalist"];

describe("space map contrast", () => {
    it("reads the tokens it tests from the stylesheet", () => {
        expect(tints).toHaveLength(5);
        expect(tints.every((v) => v > 0 && v < 100)).toBe(true);
        expect(tints).toEqual([...tints].sort((a, b) => b - a)); // ordered: rank 0 is the strongest
        expect(accents.length).toBeGreaterThanOrEqual(7);
        expect(token(base, "--tm-base").startsWith("#") && token(base, "--tm-ink").startsWith("#")).toBe(true);
    });

    it("builds every fill from these tokens and keeps the label on the solid ink", () => {
        expect(tmap).toMatch(/--tm-fill: color-mix\(in srgb, var\(--accent\) var\(--tm-t0\), var\(--tm-base\)\)/);
        for (let i = 0; i < 5; i++) expect(tmap).toContain(`--accent) var(--tm-t${i}), var(--tm-base)`);
        expect(tmap).toMatch(/color: var\(--tm-ink\)/);
        expect(tmap).toMatch(/opacity: 1;/); // a disabled block must not fade its text
    });

    for (const style of styles) {
        for (const theme of ["dark", "light"] as const) {
            describe(`${style} / ${theme}`, () => {
                const {base: b, ink} = themes[theme];

                it("is not redefined by the design style or the page styles", () => {
                    for (const f of ["styles.css", "style.css", "visual.css"]) expect(read(f)).not.toMatch(/--tm-|\.tm-|\.tmap/);
                });

                for (const [name, value] of accents) {
                    it(`label text is at least 4.5:1 on every block fill with the ${name} accent`, () => {
                        const accent = rgb(value);
                        tints.forEach((p, i) => {
                            const fill = mix(accent, p, b);
                            expect(contrast(ink, fill), `tint ${i} (${p}%)`).toBeGreaterThanOrEqual(4.5);
                        });
                    });
                }

                it("label text is at least 4.5:1 on the hatched 'other' block, on its fill and on its stripes", () => {
                    const fill = mix(ink, other, b);
                    const stripe = mix(ink, hatch, b);
                    expect(contrast(ink, fill)).toBeGreaterThanOrEqual(4.5);
                    expect(contrast(ink, stripe)).toBeGreaterThanOrEqual(4.5);
                    expect(contrast(ink, b)).toBeGreaterThanOrEqual(4.5);
                });

                it("the focus ring and selection border (the ink) is at least 3:1 against every fill", () => {
                    for (const [, value] of accents) for (const p of tints) expect(contrast(ink, mix(rgb(value), p, b))).toBeGreaterThanOrEqual(3);
                });

                it("the stripes differ from the fill, so 'other' is told apart by pattern and not only by tone", () => {
                    expect(contrast(mix(ink, hatch, b), mix(ink, other, b))).toBeGreaterThan(1.05);
                });
            });
        }
    }

    it("separates neighbouring tints by a gap and an ink border, not by colour alone", () => {
        expect(tmap).toMatch(/border: 1px solid color-mix\(in srgb, var\(--tm-ink\) 60%, var\(--tm-base\)\)/);
        // two blocks side by side: consecutive tint fills are close in tone, so the border carries the edge
        const [a, c] = [mix(rgb("#4f9dff"), tints[1], themes.dark.base), mix(rgb("#4f9dff"), tints[2], themes.dark.base)];
        expect(contrast(a, c)).toBeLessThan(1.5);
        const edge = mix(themes.dark.ink, 60, themes.dark.base);
        expect(contrast(edge, a)).toBeGreaterThanOrEqual(3);
        expect(contrast(mix(themes.light.ink, 60, themes.light.base), mix(rgb("#4f9dff"), tints[1], themes.light.base))).toBeGreaterThanOrEqual(3);
    });
});
