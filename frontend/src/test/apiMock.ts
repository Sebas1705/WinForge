import {act} from "@testing-library/react";
import {vi} from "vitest";
import type {InstallEvent, Profile, State} from "../lib/model";
import {makeState, planFor} from "./fixtures";

type Handler<T> = (e: T) => void;
const handlers = {
    scan: new Set<Handler<{ scan: "pc" | "health"; step: string }>>(),
    install: new Set<Handler<InstallEvent>>(),
    done: new Set<Handler<string[] | null>>(),
    progress: new Set<Handler<{ done: number; total: number }>>(),
};

/** Every call the app can make into Go, as a spy. Defaults describe a quiet, healthy PC. */
export const api = {
    GetState: vi.fn(), Plan: vi.fn(), Apply: vi.fn(), Cancel: vi.fn(), SaveProfile: vi.fn(), DeleteProfile: vi.fn(),
    ProfileFromPC: vi.fn(), ExportProfile: vi.fn(), ExportWinget: vi.fn(), ExportScript: vi.fn(), ImportProfile: vi.fn(),
    RestartAsAdmin: vi.fn(), OpenURL: vi.fn(), CheckUpdate: vi.fn(), InstallUpdate: vi.fn(), Upgrades: vi.fn(), PlanUpgrades: vi.fn(),
    ApplyUpgrades: vi.fn(), HealthScan: vi.fn(), HealthUpdates: vi.fn(), SaveTextFile: vi.fn(), OpenLink: vi.fn(),
};

export function resetApi(state: State = makeState()): void {
    for (const h of Object.values(handlers)) h.clear();
    for (const fn of Object.values(api)) fn.mockReset();
    api.GetState.mockImplementation(async () => structuredClone(state));
    api.Plan.mockImplementation(async (p: Profile) => planFor(p));
    api.Apply.mockResolvedValue(undefined);
    api.Cancel.mockResolvedValue(undefined);
    api.SaveProfile.mockResolvedValue(undefined);
    api.DeleteProfile.mockResolvedValue(undefined);
    api.OpenURL.mockResolvedValue(undefined);
    api.OpenLink.mockResolvedValue(undefined);
    api.RestartAsAdmin.mockResolvedValue(undefined);
    api.CheckUpdate.mockResolvedValue({current: "v0.0.0-test", latest: "", available: false, url: "", notes: ""});
    api.Upgrades.mockResolvedValue([]);
    api.SaveTextFile.mockResolvedValue("C:\\report.md");
    api.ExportProfile.mockResolvedValue("");
}

/** Stands in for the ./api module. */
export const moduleMock = {
    api,
    errText: (e: unknown) => (e instanceof Error ? e.message : String(e)),
    on: {
        scan: (fn: Handler<{ scan: "pc" | "health"; step: string }>) => { handlers.scan.add(fn); return () => handlers.scan.delete(fn); },
        install: (fn: Handler<InstallEvent>) => { handlers.install.add(fn); return () => handlers.install.delete(fn); },
        done: (fn: Handler<string[] | null>) => { handlers.done.add(fn); return () => handlers.done.delete(fn); },
        progress: (fn: Handler<{ done: number; total: number }>) => { handlers.progress.add(fn); return () => handlers.progress.delete(fn); },
    },
};

/** Events the Go side would emit while a plan runs. */
export const emit = {
    scan: (scan: "pc" | "health", step: string) => act(() => { handlers.scan.forEach((h) => h({scan, step})); }),
    install: (e: InstallEvent) => act(() => { handlers.install.forEach((h) => h(e)); }),
    done: (failed: string[] | null = []) => act(() => { handlers.done.forEach((h) => h(failed)); }),
};
