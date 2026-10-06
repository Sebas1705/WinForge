// Types mirror the JSON the Go backend sends, and the pure helpers the views
// share. Kept free of Wails imports so they can be unit-tested.

export interface App {
    id: string;
    name: string;
    category: string;
    description: string;
    homepage: string;
    license?: string;
    winget: string;
    publisher: string;
    admin?: boolean;
    openSource: boolean;
    requires?: string[];
    recipes?: string[];
    tagline?: { en: string; es: string };
    installed: boolean;
    version?: string;
    sources?: string[];
}

export interface ProfileApp {
    id: string;
    version?: string;
}

export interface Profile {
    id: string;
    name: string;
    description?: string;
    kind?: string;
    extends?: string[];
    apps?: ProfileApp[];
    recipes?: string[];
}

export interface ProfileInfo extends Profile {
    builtin: boolean;
    resolved: string[];
    resolvedRecipes: string[];
}

export interface State {
    version: string;
    admin: boolean;
    apps: App[];
    profiles: ProfileInfo[];
    featured: string[];
    wingetError?: string;
}

export interface Step {
    kind: "app" | "recipe" | "upgrade";
    id: string;
    name: string;
    version?: string;
    admin?: boolean;
}

export interface UpgradeInfo {
    id: string;
    name: string;
    publisher: string;
    current: string;
    available: string;
}

export interface Plan {
    steps: Step[];
    alreadyInstalled: string[];
    skippedRecipes?: string[];
    needsAdmin: boolean;
}

export type StepStatus = "pending" | "running" | "ok" | "failed" | "skipped";

export interface InstallEvent {
    step: Step;
    status: "start" | "output" | "ok" | "skipped" | "failed";
    line?: string;
    error?: string;
}

export interface Filter {
    query: string;
    category: string;
    installed: "all" | "installed" | "missing";
    openSourceOnly: boolean;
}

export function categories(apps: App[]): string[] {
    return [...new Set(apps.map((a) => a.category.split("/")[0]))].sort();
}

export function filterApps(apps: App[], f: Filter): App[] {
    const q = f.query.trim().toLowerCase();
    return apps.filter((a) => {
        if (f.category && a.category.split("/")[0] !== f.category) return false;
        if (f.installed === "installed" && !a.installed) return false;
        if (f.installed === "missing" && a.installed) return false;
        if (f.openSourceOnly && !a.openSource) return false;
        if (!q) return true;
        return [a.id, a.name, a.description, a.publisher, a.category, a.winget]
            .some((s) => s.toLowerCase().includes(q));
    });
}

export function slugify(name: string): string {
    const s = name.toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "")
        .replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 60);
    return s || "profile";
}

/** Builds an unsaved profile from a set of selected app ids. */
export function selectionProfile(ids: Iterable<string>, name = "Selection"): Profile {
    return {
        id: slugify(name),
        name,
        kind: "custom",
        apps: [...ids].sort().map((id) => ({id})),
    };
}

/** Next status of a step after an event; events for other statuses keep it. */
export function applyEvent(prev: StepStatus, e: InstallEvent): StepStatus {
    switch (e.status) {
        case "start": return "running";
        case "ok": return "ok";
        case "failed": return "failed";
        case "skipped": return "skipped";
        default: return prev;
    }
}
