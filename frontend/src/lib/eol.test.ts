import {beforeEach, describe, expect, it} from "vitest";
import {adviceText, counts, dayText, lineText, monthsOld, replacementText, spanText, whenText, type EOLFinding} from "./eol";
import {setLang} from "./i18n";

const f = (o: Partial<EOLFinding>): EOLFinding => ({
    appId: "a", product: "py", productName: "Python", version: "3.10.1", cycle: "3.10", status: "ended", end: "2026-10-01", days: -47,
    source: "https://endoflife.date/python", replace: [], apps: [], ...o,
});

describe("end of support helpers", () => {
    beforeEach(() => setLang("en"));

    it("writes a calendar day the same in any time zone and language", () => {
        expect(dayText("2026-10-01")).toBe("October 1, 2026");
        expect(dayText("2028-02-29")).toBe("February 29, 2028");
        setLang("es");
        expect(dayText("2026-10-01")).toBe("1 de octubre de 2026");
        expect(dayText("not a date")).toBe("not a date");
        expect(dayText("2026-02-30")).toBe("2026-02-30");
    });

    it("says how far a date is", () => {
        expect(spanText(1)).toBe("1 day");
        expect(spanText(-1)).toBe("1 day");
        expect(spanText(47)).toBe("47 days");
        expect(whenText(0)).toBe("today");
        expect(whenText(1)).toBe("in 1 day");
        expect(whenText(174)).toBe("in 174 days");
        expect(whenText(-1)).toBe("1 day ago");
        expect(whenText(-47)).toBe("47 days ago");
        setLang("es");
        expect(whenText(-47)).toBe("hace 47 días");
        expect(whenText(2)).toBe("dentro de 2 días");
        expect(whenText(0)).toBe("hoy");
    });

    it("counts months of age, at least one", () => {
        expect(monthsOld(366)).toBe(12);
        expect(monthsOld(534)).toBe(17);
        expect(monthsOld(0)).toBe(1);
    });

    it("counts findings by status and ignores a status it does not know", () => {
        expect(counts([f({}), f({status: "soon"}), f({status: "ok"}), f({status: "ok"}), f({status: "unknown"}), f({status: "weird" as never})]))
            .toEqual({ended: 1, soon: 1, ok: 2, unknown: 1});
    });

    it("words a release line with or without a date and with or without long-term support", () => {
        expect(replacementText({cycle: "3.15", end: "2031-10-31"})).toBe("3.15 until October 31, 2031");
        expect(replacementText({cycle: "21", end: "2029-12-31", lts: true})).toBe("21 (long-term support) until December 31, 2029");
        expect(replacementText({cycle: "1.27"})).toBe("1.27");
        expect(replacementText({cycle: "24", lts: true})).toBe("24 (long-term support)");
    });

    it("words the sentence for each status", () => {
        expect(lineText(f({}))).toBe("Support for Python 3.10 ended on October 1, 2026 (47 days ago).");
        expect(lineText(f({status: "soon", end: "2027-05-10", days: 174}))).toBe("Support for Python 3.10 ends on May 10, 2027 (in 174 days).");
        expect(lineText(f({status: "soon", end: "2026-11-17", days: 0}))).toBe("Support for Python 3.10 ends on November 17, 2026 (today).");
        expect(lineText(f({status: "ok", end: "2029-10-31", days: 800}))).toBe("Support for Python 3.10 lasts until October 31, 2029.");
        expect(lineText(f({status: "ok", end: undefined, days: undefined}))).toBe("Python 3.10 is a current release; no end date has been announced.");
        expect(lineText(f({status: "unknown", cycle: "", end: undefined, days: undefined}))).toContain("could not tell which Python release version 3.10.1 is");
        expect(lineText(f({status: "unknown", cycle: "", version: "", end: undefined, days: undefined}))).toContain("could not read a version number");
    });

    it("gives advice only for what needs it", () => {
        expect(adviceText(f({}))).toContain("Move to a supported one");
        expect(adviceText(f({status: "soon"}))).toContain("Plan to move");
        expect(adviceText(f({status: "unknown"}))).toContain("maker's own page");
        expect(adviceText(f({status: "ok"}))).toBe("");
    });
});
