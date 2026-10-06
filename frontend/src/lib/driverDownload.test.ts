import {describe, expect, it} from "vitest";
import {driverDownload} from "./health";

describe("driverDownload", () => {
    it("goes to the chip maker's driver page when it is known", () => {
        expect(driverDownload({device: "Realtek Audio", manufacturer: "Realtek Semiconductor Corp."})).toContain("realtek.com");
        expect(driverDownload({device: "Radeon", manufacturer: "Advanced Micro Devices, Inc."})).toContain("amd.com");
    });
    it("falls back to the Microsoft Update Catalog for the exact device", () => {
        const u = driverDownload({device: "Foo Bar #1", manufacturer: "Acme"});
        expect(u).toBe("https://www.catalog.update.microsoft.com/Search.aspx?q=Foo%20Bar%20%231");
    });
});
