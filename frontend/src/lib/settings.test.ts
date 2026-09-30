import {describe, expect, it} from "vitest";
import {DEFAULTS, apply, normalize, resolveTheme} from "./settings";

describe("settings", () => {
    it("falls back per field on garbage", () => {
        expect(normalize(null)).toEqual(DEFAULTS);
        expect(normalize({theme: "neon", accent: "red", density: "compact"}))
            .toEqual({...DEFAULTS, density: "compact"});
    });
    it("keeps valid values", () => {
        expect(normalize({theme: "light", accent: "#AABBCC", density: "compact"}))
            .toEqual({theme: "light", accent: "#AABBCC", density: "compact", checkUpdates: true, language: "auto"});
        expect(normalize({language: "es"}).language).toBe("es");
        expect(normalize({language: "klingon"}).language).toBe("auto");
        expect(normalize({checkUpdates: false}).checkUpdates).toBe(false);
    });
    it("resolves system theme from the OS preference", () => {
        expect(resolveTheme("system", true)).toBe("dark");
        expect(resolveTheme("system", false)).toBe("light");
        expect(resolveTheme("dark", false)).toBe("dark");
    });
    it("applies to the document root", () => {
        const root = document.createElement("html");
        apply({theme: "system", accent: "#3ecf8e", density: "compact", checkUpdates: true, language: "auto"}, root, false);
        expect(root.dataset.theme).toBe("light");
        expect(root.dataset.density).toBe("compact");
        expect(root.style.getPropertyValue("--accent")).toBe("#3ecf8e");
    });
});
