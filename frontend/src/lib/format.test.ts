import {describe, expect, it} from "vitest";
import {formatBytes, formatDate, percent} from "./format";

describe("formatBytes", () => {
    it("reads zero, negative and broken values as 0 B", () => {
        for (const v of [0, -1, -1e12, Number.NaN, Number.NEGATIVE_INFINITY, Number.POSITIVE_INFINITY]) expect(formatBytes(v, "en"), String(v)).toBe("0 B");
    });
    it("keeps plain bytes below 1 KB", () => {
        expect(formatBytes(1, "en")).toBe("1 B");
        expect(formatBytes(1023, "en")).toBe("1023 B");
        expect(formatBytes(1023.9, "en")).toBe("1023 B");
    });
    it("steps up at 1024", () => {
        expect(formatBytes(1024, "en")).toBe("1 KB");
        expect(formatBytes(1536, "en")).toBe("1.5 KB");
        expect(formatBytes(1024 ** 2, "en")).toBe("1 MB");
        expect(formatBytes(1024 ** 3 * 1.5, "en")).toBe("1.5 GB");
        expect(formatBytes(1024 ** 4 * 2, "en")).toBe("2 TB");
    });
    it("uses one decimal below 100 and none from 100", () => {
        expect(formatBytes(12.34 * 1024 ** 2, "en")).toBe("12.3 MB");
        expect(formatBytes(512 * 1024 ** 3, "en")).toBe("512 GB");
        expect(formatBytes(150.6 * 1024 ** 2, "en")).toBe("151 MB");
    });
    it("never shows 1024 of a unit: it moves to the next one", () => {
        expect(formatBytes(1024 ** 2 - 1, "en")).toBe("1 MB");
        expect(formatBytes(1024 ** 3 - 100, "en")).toBe("1 GB");
    });
    it("stops at the largest unit", () => {
        expect(formatBytes(1024 ** 5 * 3, "en")).toBe("3 PB");
        expect(formatBytes(1024 ** 6, "en")).toBe("1,024 PB");
    });
    it("uses the decimal mark of the language", () => {
        expect(formatBytes(1536, "es")).toBe("1,5 KB");
        expect(formatBytes(1536, "en")).toBe("1.5 KB");
    });
});

describe("formatDate", () => {
    it("writes the date in the language", () => {
        expect(formatDate("2024-03-05T12:00:00Z", "en")).toMatch(/Mar/);
        expect(formatDate("2024-03-05T12:00:00Z", "en")).toContain("2024");
        expect(formatDate("2024-03-05T12:00:00Z", "es")).toMatch(/mar/);
        expect(formatDate("2024-03-05T12:00:00Z", "es")).not.toBe(formatDate("2024-03-05T12:00:00Z", "en"));
    });
    it("is empty when the date is missing, unreadable or the Go zero time", () => {
        expect(formatDate("", "en")).toBe("");
        expect(formatDate("not a date", "en")).toBe("");
        expect(formatDate("0001-01-01T00:00:00Z", "en")).toBe("");
    });
});

describe("percent", () => {
    it("rounds and clamps", () => {
        expect(percent(1, 3)).toBe(33);
        expect(percent(2, 3)).toBe(67);
        expect(percent(5, 5)).toBe(100);
        expect(percent(9, 5)).toBe(100);
        expect(percent(-3, 5)).toBe(0);
    });
    it("is 0 without a positive whole", () => {
        expect(percent(5, 0)).toBe(0);
        expect(percent(5, -1)).toBe(0);
        expect(percent(Number.NaN, 10)).toBe(0);
    });
});
