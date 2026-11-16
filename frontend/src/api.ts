// The generated Wails bindings use their own model classes; the JSON shapes are
// identical, so the boundary is typed once here.
import * as Go from "../wailsjs/go/main/App";
import {EventsOn} from "../wailsjs/runtime/runtime";
import type {HealthResult} from "./lib/health";
import type {InstallEvent, Plan, Profile, State, UpgradeInfo} from "./lib/model";

export interface ProfilesFolderInfo { path: string; default: string; custom: boolean; count: number; problem?: string }
export interface ProfilesFolderChange { copied: number; skipped: number; info: ProfilesFolderInfo }
export interface DriftApp { id: string; name: string }
export interface DriftChange { id: string; name: string; from: string; to: string }
export interface DriftInfo { hasBaseline: boolean; at?: string; removed: DriftApp[]; added: DriftApp[]; updated: DriftChange[]; count: number }
export interface UpdateInfo { current: string; latest: string; available: boolean; url: string; notes: string }
export interface BackupSet { id: string; files: number; bytes: number }
export interface BackupRequest { sets: string[]; apps: boolean; folders: string[] }
export interface BackupResult { path: string; files: number; bytes: number; apps: number; folders: number; verified: boolean; problems: number }
export interface SetPreview { id: string; files: number; new: number; changed: number; same: number }
export interface AppPreview { id: string; name: string; installed: boolean }
export interface FolderInfo { name: string; dir: string; files: number; bytes: number; skipped: number }
export interface BackupPreview {
    host: string; app: string; createdAt: string; intact: boolean; checked: boolean; problems: string[];
    sets: SetPreview[]; apps: AppPreview[]; missing: number; unknown: string[]; profile: Profile | null; folders: FolderInfo[];
}
export interface FolderResult { written: number; same: number; kept: number; skipped: string[]; dest: string }
export interface RestoreResult { restored: number; unchanged: number; backedUp: number; extensions: number; skipped: string[] }
export interface EmuSystem { id: string; en: string; es: string }
export interface EmuSource { id: string; system: string; kind: string; name: string; en: string; es: string; url: string; base?: boolean }
export interface RunSpec { exes: string[]; hints: string[]; args?: string[]; register?: string[] }
export interface Emulation { systems: EmuSystem[]; emulators: Record<string, string[]>; sources: EmuSource[]; run?: Record<string, RunSpec> }
export interface InstallableGame {
    id: string; name: string; system: string; en: string; es: string; license: string; homepage: string; url: string;
    sha256: string; size: number; kind: "zip" | "file"; file?: string; entry?: string;
}
export interface GameReceipt { id: string; sha256: string; url: string; files: number; installedAt: string }
export interface GamesView { root: string; games: InstallableGame[]; installed: Record<string, GameReceipt>; launchers: Record<string, string> }
export interface GameInstalled { receipt: GameReceipt; registered: string[]; registerError?: string }
export interface PatchResult { path: string; format: string; checked: boolean; headerless: boolean }
export interface StorageVolume { letter: string; label: string; totalBytes: number; freeBytes: number }
export interface CleanCategory { id: string; risk: "safe" | "review"; needsAdmin: boolean; available: boolean; bytes: number; files: number; skipped: number }
export interface CleanOutcome { id: string; freedBytes: number; files: number; failed: number; error?: string }
export interface CleanResult { freedBytes: number; files: number; failed: number; outcomes: CleanOutcome[] }
export interface BigFile { path: string; name: string; sizeBytes: number; modified: string }
export interface FolderSize { path: string; name: string; sizeBytes: number; files: number }
export interface DupFile { path: string; modified: string }
export interface DupGroup { id: string; sizeBytes: number; files: DupFile[] }
export interface RecycleResult { moved: number; freedBytes: number; failed: string[] }
export interface StorageDefaults { home: string; downloads: string; documents: string; pictures: string; videos: string }
export interface StorageProgress { task: "scan" | "clean" | "big" | "folders" | "dups" | "projects" | "projects-clean"; files: number; bytes: number; current?: string }
export interface ProjectJunk { path: string; project: string; projectPath: string; kind: string; sizeBytes: number; files: number; lastTouched: string }
export interface ProjectResult { freedBytes: number; removed: number; files: number; failed: string[]; reasons: string[] }
export interface VirtualDisk { path: string; name: string; kind: "wsl" | "docker"; sizeBytes: number; modified: string }
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
    ExportConfigure(p: Profile): Promise<string>;
    ProfileDrift(id: string): Promise<DriftInfo>;
    AcceptProfileState(id: string): Promise<void>;
    ExportScript(p: Profile): Promise<string>;
    ImportProfile(): Promise<ImportResult | null>;
    RestartAsAdmin(): Promise<void>;
    OpenURL(u: string): Promise<void>;
    CheckUpdate(): Promise<UpdateInfo>;
    InstallUpdate(): Promise<void>;
    Upgrades(): Promise<UpgradeInfo[]>;
    PlanUpgrades(ids: string[]): Promise<Plan>;
    ApplyUpgrades(ids: string[]): Promise<void>;
    IgnoreUpdate(id: string, mode: string, days: number): Promise<UpgradeInfo[]>;
    ProfilesFolder(): Promise<ProfilesFolderInfo>;
    SetProfilesFolder(dir: string, copyExisting: boolean): Promise<ProfilesFolderChange>;
    ResetProfilesFolder(): Promise<ProfilesFolderInfo>;
    StopIgnoringUpdate(id: string): Promise<UpgradeInfo[]>;
    HealthScan(): Promise<HealthResult>;
    HealthUpdates(): Promise<HealthResult>;
    SaveTextFile(name: string, content: string): Promise<string>;
    OpenLink(url: string): Promise<void>;
    ResumePending(): Promise<void>;
    PlanPending(): Promise<Plan>;
    DiscardPending(): Promise<void>;
    PlanUninstall(ids: string[]): Promise<Plan>;
    ApplyUninstall(ids: string[]): Promise<void>;
    InstallWinget(): Promise<void>;
    ShareCode(p: Profile): Promise<string>;
    ImportCode(code: string): Promise<ImportResult>;
    BackupSets(): Promise<BackupSet[]>;
    PickFolder(): Promise<string>;
    CreateBackup(req: BackupRequest): Promise<BackupResult>;
    PickBackup(): Promise<BackupPreview | null>;
    RestoreSettings(ids: string[]): Promise<RestoreResult>;
    RestoreFolders(names: string[], dest: string): Promise<FolderResult>;
    Emulation(): Promise<Emulation>;
    PatchROM(): Promise<PatchResult>;
    PlanRepairs(ids: string[]): Promise<Plan>;
    ApplyRepairs(ids: string[]): Promise<void>;
    RestartPC(): Promise<void>;
    CancelRestart(): Promise<void>;
    GamesState(): Promise<GamesView>;
    SetGamesRoot(): Promise<string>;
    InstallGame(id: string): Promise<GameInstalled>;
    RemoveGame(id: string): Promise<void>;
    PlayGame(id: string, emulator: string): Promise<void>;
    OpenGameFolder(id: string): Promise<void>;
    StorageVolumes(): Promise<StorageVolume[]>;
    StorageDefaults(): Promise<StorageDefaults>;
    StorageScan(): Promise<CleanCategory[]>;
    StorageClean(ids: string[]): Promise<CleanResult>;
    StorageBigFiles(root: string, limit: number): Promise<BigFile[]>;
    StorageFolders(root: string): Promise<FolderSize[]>;
    StorageDuplicates(roots: string[], minMB: number): Promise<DupGroup[]>;
    StorageRecycle(paths: string[]): Promise<RecycleResult>;
    StorageProjects(roots: string[], minDays: number): Promise<ProjectJunk[]>;
    StorageProjectsClean(paths: string[]): Promise<ProjectResult>;
    StorageVirtualDisks(): Promise<VirtualDisk[]>;
    StorageReveal(path: string): Promise<void>;
    StorageCancel(): Promise<void>;
};

export const on = {
    gameProgress: (fn: (e: { id: string; done: number; total: number }) => void): (() => void) => EventsOn("game:progress", fn),
    scan: (fn: (e: { scan: "pc" | "health"; step: string }) => void): (() => void) => EventsOn("scan:step", fn),
    install: (fn: (e: InstallEvent) => void): (() => void) => EventsOn("install", fn),
    done: (fn: (failed: string[] | null) => void): (() => void) => EventsOn("install:done", fn),
    storageProgress: (fn: (p: StorageProgress) => void): (() => void) => EventsOn("storage:progress", fn),
    progress: (fn: (p: { done: number; total: number }) => void): (() => void) => EventsOn("update:progress", fn),
};

export const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));
