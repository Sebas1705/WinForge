import {describe, expect, it} from "vitest";
import type {UpgradeInfo} from "./model";
import {PAUSE_DAYS, pendingUpgrades, splitUpgrades} from "./updates";

const up = (id: string, extra: Partial<UpgradeInfo> = {}): UpgradeInfo => ({id, name: id, publisher: "", current: "1", available: "2", ...extra});

describe("splitting updates", () => {
    it("keeps ignored updates apart and in order", () => {
        const list = [up("a"), up("b", {ignore: "always"}), up("c"), up("d", {ignore: "until", ignoreUntil: "2026-12-01T00:00:00Z"})];
        const {active, ignored} = splitUpgrades(list);
        expect(active.map((u) => u.id)).toEqual(["a", "c"]);
        expect(ignored.map((u) => u.id)).toEqual(["b", "d"]);
    });

    it("handles an empty list and a list with nothing ignored", () => {
        expect(splitUpgrades([])).toEqual({active: [], ignored: []});
        expect(splitUpgrades([up("a")]).ignored).toEqual([]);
    });

    it("counts only what is not ignored, and stays null before the first check", () => {
        expect(pendingUpgrades(null)).toBeNull();
        expect(pendingUpgrades([up("a", {ignore: "version", ignoreVersion: "2"})])).toEqual([]);
        expect(pendingUpgrades([up("a"), up("b", {ignore: "always"})])?.map((u) => u.id)).toEqual(["a"]);
    });

    it("offers pauses the backend accepts", () => {
        for (const d of PAUSE_DAYS) expect(d >= 1 && d <= 365).toBe(true);
    });
});
