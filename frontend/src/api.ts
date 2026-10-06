// The generated Wails bindings use their own model classes; the JSON shapes are
// identical, so the boundary is typed once here.
import * as Go from "../wailsjs/go/main/App";
import {EventsOn} from "../wailsjs/runtime/runtime";
import type {HealthResult} from "./lib/health";
import type {InstallEvent, Plan, Profile, State, UpgradeInfo} from "./lib/model";

export interface UpdateInfo { current: string; latest: string; available: boolean; url: string; notes: string }
export interface ImportResult { profile: Profile; unknownApps: string[]; unknownRecipes: string[] }

export const api = Go as unknown as {
    GetState(): Promise<State>;
    Plan(p: Profile): Promise<Plan>;
    Apply(p: Profile): Promise<void>;
    Cancel(): Promise<void>;
    SaveProfile(p: Profile): Promise<void>;
    DeleteProfile(id: string): Promise<void>;
    ProfileFromPC(id: string, name: string, pin: boolean): Promise<Profile>;
    ExportProfile(p: Profile): Promise<string>;
    ExportWinget(p: Profile): Promise<string>;
    ExportScript(p: Profile): Promise<string>;
    ImportProfile(): Promise<ImportResult | null>;
    RestartAsAdmin(): Promise<void>;
    OpenURL(u: string): Promise<void>;
    CheckUpdate(): Promise<UpdateInfo>;
    InstallUpdate(): Promise<void>;
    Upgrades(): Promise<UpgradeInfo[]>;
    PlanUpgrades(ids: string[]): Promise<Plan>;
    ApplyUpgrades(ids: string[]): Promise<void>;
    HealthScan(): Promise<HealthResult>;
    HealthUpdates(): Promise<HealthResult>;
    SaveTextFile(name: string, content: string): Promise<string>;
    OpenLink(url: string): Promise<void>;
};

export const on = {
    install: (fn: (e: InstallEvent) => void): (() => void) => EventsOn("install", fn),
    done: (fn: (failed: string[] | null) => void): (() => void) => EventsOn("install:done", fn),
    progress: (fn: (p: { done: number; total: number }) => void): (() => void) => EventsOn("update:progress", fn),
};

export const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));
