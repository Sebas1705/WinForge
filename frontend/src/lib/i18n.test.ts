import {beforeEach, describe, expect, it} from "vitest";
import {DICTS, KEYS, categoryLabel, resolveLang, setLang, t} from "./i18n";

const placeholders = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

describe("i18n", () => {
    beforeEach(() => setLang("en"));

    it("has every key in both languages with the same placeholders", () => {
        for (const k of KEYS) {
            expect(DICTS.es[k], `es is missing ${k}`).toBeTruthy();
            expect(placeholders(DICTS.es[k]), `placeholders of ${k}`).toEqual(placeholders(DICTS.en[k]));
        }
    });
    it("fills placeholders and leaves unknown ones visible", () => {
        expect(t("home.foundOf", {total: 744})).toBe("of 744 catalog apps found");
        expect(t("home.foundOf")).toContain("{total}");
        setLang("es");
        expect(t("home.missing", {n: 3})).toBe("faltan 3");
    });
    it("resolves automatic language from the OS", () => {
        expect(resolveLang("auto", "es-ES")).toBe("es");
        expect(resolveLang("auto", "en-US")).toBe("en");
        expect(resolveLang("auto", "fr-FR")).toBe("en");
        expect(resolveLang("es", "en-US")).toBe("es");
    });
    it("labels categories and falls back to the raw name", () => {
        setLang("es");
        expect(categoryLabel("network")).toBe("Red");
        expect(categoryLabel("brand-new")).toBe("brand-new");
    });
});
