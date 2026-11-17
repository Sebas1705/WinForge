// The generated Wails bindings use their own model classes; the JSON shapes are
// identical, so the boundary is typed once here.
import * as Go from "../wailsjs/go/main/App";
import {EventsOn} from "../wailsjs/runtime/runtime";
import type {HealthResult} from "./lib/health";
import type {EOLReport} from "./lib/eol";
import type {BatteryInfo, HHView} from "./lib/hhist";
import type {InstallEvent, Plan, Profile, State, UpgradeInfo} from "./lib/model";

export interface ProfilesFolderInfo { path: string; default: string; custom: boolean; count: number; problem?: string }
export interface ProfilesFolderChange { copied: number; skipped: number; info: ProfilesFolderInfo }
export interface DriftApp { id: string; name: string }
export interface DriftChange { id: string; name: string; from: string; to: string }
export interface DriftInfo { hasBaseline: boolean; at?: string; removed: DriftApp[]; added: DriftApp[]; updated: DriftChange[]; count: number }
export interface CompareApp { id: string; name: string }
export interface CompareVersion { id: string; name: string; a: string; b: string }
export interface CompareUnknown { id: string; in: "a" | "b" | "both" }
export interface CompareSide {
    kind: "pc" | "file" | "code"; name: string; file: string; apps: number; versioned: boolean;
    unknownRecipes: string[]; unknownProfiles: string[]; profile: Profile | null;
}
export interface CompareResult {
    a: CompareSide; b: CompareSide; onlyA: CompareApp[]; onlyB: CompareApp[]; both: CompareApp[];
    differ: CompareVersion[]; unknown: CompareUnknown[];
}
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
export interface StartupItem {
    id: string; name: string; source: string; command: string; target: string; publisher: string;
    enabled: boolean; needsAdmin: boolean; missing: boolean; caution: boolean;
}
export interface EnvFinding {
    id: string; kind: string; severity: "problem" | "note"; scope: "user" | "machine"; subject: string; paths: string[];
    fixable: boolean; fixKind: "remove-path-entry" | "unset-variable" | "";
}
export interface EnvTool { name: string; found: string[] }
export interface EnvVar { name: string; scope: "user" | "machine"; value: string }
export interface EnvReport { findings: EnvFinding[]; userPath: string[]; machinePath: string[]; tools: EnvTool[]; vars: EnvVar[] }
export interface EnvFixResult { applied: number; failed: string[]; backupId: string }
export interface EnvBackup { id: string; time: string; changed: string[] }
export type PrivacyState = "private" | "default" | "custom" | "unknown";
export interface PrivacyChange {
    path: string; name: string; existed: boolean; oldType: string; old: string;
    /** The type and data applying writes. */
    type: string; private: number; error: boolean;
}
export interface PrivacyTweak { id: string; area: string; nameKey: string; whyKey: string; state: PrivacyState; changes: PrivacyChange[] }
export interface PrivacyResult { applied: number; skipped: string[]; failed: string[]; backupId: string }
export interface PrivacyBackup { id: string; time: string; action: "apply" | "reset"; tweaks: string[]; restored: string | null }
export interface PrivacyRestoreResult { restored: string[]; changedSince: string[]; failed: string[] }
export type FeatureId = "wsl" | "vmp" | "hyperv" | "sandbox" | "netfx3";
export interface Feature {
    id: FeatureId; state: "on" | "off" | "unavailable" | "unknown"; restartPending: boolean; enable: string; disable: string; needsAdmin: boolean;
}
export interface WSLDistro { name: string; state: string; version: number; isDefault: boolean }
export interface WSLInfo { installed: boolean; defaultVersion: number; distros: WSLDistro[] }
export type ServiceMode = "auto" | "delayed" | "manual" | "disabled";
/** A third-party Windows service. The only change offered is its start mode. */
export interface ServiceItem {
    id: string; name: string; displayName: string; publisher: string;
    state: "running" | "stopped" | "other"; startMode: ServiceMode; binary: string;
    needsAdmin: boolean; caution: boolean; dependents: string[]; changed: boolean; original: ServiceMode | "";
}
/** A third-party scheduled task that starts at log on or at startup. The only change is on/off. */
export interface TaskItem {
    id: string; name: string; folder: string; triggers: ("logon" | "boot")[]; action: string;
    state: "ready" | "disabled" | "running" | "unknown"; enabled: boolean; lastResult: number | null;
    needsAdmin: boolean; caution: boolean; changed: boolean; original: "enabled" | "disabled" | "";
}
export interface HistoryEntry {
    id: string; app: string; name: string; action: "install" | "update" | "rollback"; from: string; to: string;
    at: string; result: "ok" | "failed" | "restart"; source: "profile" | "update" | "single"; reason?: string; canRollback: boolean;
}
export interface HistoryFilter { app: string; result: string; limit: number }
export interface HistoryPage { entries: HistoryEntry[]; total: number; apps: { id: string; name: string }[] }
export interface UninstallApp { id: string; name: string; version?: string; publisher: string; admin?: boolean; sizeBytes?: number }
export interface UninstallRemoved { id: string; name: string }
export interface UninstallListing { apps: UninstallApp[]; removed: UninstallRemoved[] }
export interface UninstallVerified { removed: UninstallRemoved[]; still: UninstallRemoved[] }
export interface LeftoverWhy { rule: "name" | "publisherName" | "nested" | "wingetId" | "wingetPackage"; value: string }
export interface LeftoverCandidate {
    path: string; kind: "folder" | "shortcut"; place: string; sizeBytes: number; files: number; modified: string; why: LeftoverWhy;
}
export interface LeftoversResult { appId: string; appName: string; candidates: LeftoverCandidate[]; searched: { place: string; path: string }[] }
export interface LeftoversRecycleResult { moved: number; freedBytes: number; failed: { path: string; reason: string }[] }
export type AutoResult = "found" | "updated" | "failed" | "ignored" | "needs-admin" | "skipped";
export interface AutoItem { id: string; name: string; from: string; to: string; result: AutoResult; reason?: string; reboot?: boolean }
export interface AutoReport {
    id: string; started: string; finished: string; mode: "notify" | "apply"; scheduled: boolean;
    error?: string; items: AutoItem[]; notified?: boolean; fingerprint?: string;
    status: "ok" | "attention" | "partial" | "failed" | "error";
}
export interface AutoSettings { mode: "notify" | "apply"; frequency: "daily" | "weekly"; weekday: number; time: string }
export interface AutoState extends AutoSettings {
    on: boolean; enabled: boolean; foreign: boolean; nextRun?: string; canEnable: boolean;
    problem?: "runner-missing" | "task-program-missing"; runs: AutoReport[];
}
export interface ImportResult { profile: Profile; unknownApps: string[]; unknownRecipes: string[] }
export interface KitPlanApp { id: string; name: string; winget: string }
export interface KitNotIncluded { id: string; name: string; winget?: string; reason: string; detail?: string }
export interface KitCreatePlan {
    folder: string; apps: KitPlanApp[]; notIncluded: KitNotIncluded[]; needBytes: number; freeBytes: number; freeKnown: boolean;
    enough: boolean; existingKit: boolean; problem?: string;
}
export interface KitCreateResult {
    added: string[]; failed: KitNotIncluded[]; totalApps: number; bytes: number; cancelled: boolean; fingerprint: string; folder: string;
}
export interface KitProblem { kind: string; file?: string }
export interface KitApp {
    id: string; name: string; version: string; type: string; sizeBytes: number; admin?: boolean;
    state: "ready" | "problem" | "unsupported"; problems: KitProblem[]; reason?: string; detail?: string;
    installed?: boolean; installedVersion?: string;
}
export interface KitListing {
    folder: string; fingerprint: string; created: string; updated?: string; winforge: string; profile?: string;
    apps: KitApp[]; notIncluded: KitNotIncluded[]; unlisted: string[]; needsAdmin: boolean;
}
export interface KitVerified { installed: { id: string; name: string }[]; missing: { id: string; name: string }[] }
export interface KitProgress { task: "create" | "verify"; done: number; total: number; id?: string; name?: string; percent?: number }

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
    PickProfileFile(): Promise<string>;
    CompareFile(path: string): Promise<CompareResult>;
    CompareCode(code: string): Promise<CompareResult>;
    CompareFiles(pathA: string, pathB: string): Promise<CompareResult>;
    ExportScript(p: Profile): Promise<string>;
    ImportProfile(): Promise<ImportResult | null>;
    RestartAsAdmin(): Promise<void>;
    OpenURL(u: string): Promise<void>;
    /** The exact text of the diagnostic report (nothing is written or sent). */
    DiagPreview(): Promise<string>;
    /** Saves the last preview. An empty path asks where; "" comes back when cancelled. */
    DiagSave(path: string): Promise<string>;
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
    HealthHistory(): Promise<HHView>;
    HealthHistoryClear(): Promise<void>;
    Battery(): Promise<BatteryInfo>;
    EOLReport(): Promise<EOLReport>;
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
    StartupList(): Promise<StartupItem[]>;
    StartupSet(id: string, enabled: boolean): Promise<StartupItem[]>;
    StartupReveal(id: string): Promise<void>;
    ServicesList(): Promise<ServiceItem[]>;
    ServicesSetMode(id: string, mode: ServiceMode, confirmed: boolean): Promise<ServiceItem[]>;
    ServicesRestore(id: string, confirmed: boolean): Promise<ServiceItem[]>;
    TasksList(): Promise<TaskItem[]>;
    TasksSetEnabled(id: string, enabled: boolean, confirmed: boolean): Promise<TaskItem[]>;
    TasksRestore(id: string, confirmed: boolean): Promise<TaskItem[]>;
    UninstallList(): Promise<UninstallListing>;
    UninstallApps(ids: string[]): Promise<void>;
    UninstallVerify(ids: string[]): Promise<UninstallVerified>;
    LeftoversFind(appId: string): Promise<LeftoversResult>;
    LeftoversRecycle(paths: string[]): Promise<LeftoversRecycleResult>;
    StorageCancel(): Promise<void>;
    EnvScan(): Promise<EnvReport>;
    EnvFix(ids: string[]): Promise<EnvFixResult>;
    EnvBackups(): Promise<EnvBackup[]>;
    EnvRestore(id: string): Promise<void>;
    PrivacyList(): Promise<PrivacyTweak[]>;
    PrivacyApply(ids: string[]): Promise<PrivacyResult>;
    PrivacyReset(ids: string[]): Promise<PrivacyResult>;
    PrivacyBackups(): Promise<PrivacyBackup[]>;
    PrivacyRestore(id: string): Promise<PrivacyRestoreResult>;
    FeaturesList(): Promise<Feature[]>;
    WSLInfo(): Promise<WSLInfo>;
    HistoryList(filter: HistoryFilter): Promise<HistoryPage>;
    HistoryRollbackPlan(id: string): Promise<Plan>;
    HistoryRollback(id: string, skipNewer: boolean): Promise<void>;
    AutoState(): Promise<AutoState>;
    AutoEnable(s: AutoSettings, lang: string): Promise<AutoState>;
    AutoDisable(): Promise<AutoState>;
    AutoRunNow(): Promise<AutoReport>;
    AutoSetLanguage(lang: string): Promise<void>;
    LaunchTarget(): Promise<string>;
    KitPrepare(ids: string[], folder: string): Promise<KitCreatePlan>;
    KitCreate(ids: string[], folder: string, profileName: string): Promise<KitCreateResult>;
    KitOpen(folder: string): Promise<KitListing>;
    KitPlan(ids: string[]): Promise<Plan>;
    KitInstall(ids: string[]): Promise<void>;
    KitVerify(ids: string[]): Promise<KitVerified>;
    KitCancel(): Promise<void>;
};

export const on = {
    gameProgress: (fn: (e: { id: string; done: number; total: number }) => void): (() => void) => EventsOn("game:progress", fn),
    scan: (fn: (e: { scan: "pc" | "health"; step: string }) => void): (() => void) => EventsOn("scan:step", fn),
    install: (fn: (e: InstallEvent) => void): (() => void) => EventsOn("install", fn),
    done: (fn: (failed: string[] | null) => void): (() => void) => EventsOn("install:done", fn),
    storageProgress: (fn: (p: StorageProgress) => void): (() => void) => EventsOn("storage:progress", fn),
    kitProgress: (fn: (p: KitProgress) => void): (() => void) => EventsOn("kit:progress", fn),
    progress: (fn: (p: { done: number; total: number }) => void): (() => void) => EventsOn("update:progress", fn),
};

export const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));
