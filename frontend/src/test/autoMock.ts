import {vi} from "vitest";
import type {AutoReport, AutoSettings, AutoState} from "../api";

/** The automatic-updates calls as spies. apiMock.ts spreads these into its `api` and resets them. */
export const autoApi = {
    AutoState: vi.fn(), AutoEnable: vi.fn(), AutoDisable: vi.fn(), AutoRunNow: vi.fn(), AutoSetLanguage: vi.fn(), LaunchTarget: vi.fn(),
};

export const autoOff = (o: Partial<AutoState> = {}): AutoState => ({
    on: false, enabled: false, foreign: false, mode: "notify", frequency: "daily", weekday: 0, time: "09:00", canEnable: true, runs: [], ...o,
});

export const autoOn = (o: Partial<AutoState> = {}): AutoState => autoOff({
    on: true, enabled: true, mode: "notify", frequency: "daily", time: "09:30", nextRun: "2026-11-17T09:30:00Z", ...o,
});

export const autoRun = (o: Partial<AutoReport> = {}): AutoReport => ({
    id: "20261116T093000Z", started: "2026-11-16T09:30:00Z", finished: "2026-11-16T09:31:00Z", mode: "notify", scheduled: true,
    items: [], status: "ok", ...o,
});

export function resetAuto(): void {
    for (const fn of Object.values(autoApi)) fn.mockReset();
    autoApi.AutoState.mockResolvedValue(autoOff());
    autoApi.AutoEnable.mockImplementation(async (s: AutoSettings) => autoOn({...s}));
    autoApi.AutoDisable.mockResolvedValue(autoOff());
    autoApi.AutoRunNow.mockResolvedValue(autoRun({scheduled: false}));
    autoApi.AutoSetLanguage.mockResolvedValue(undefined);
    autoApi.LaunchTarget.mockResolvedValue("");
}
