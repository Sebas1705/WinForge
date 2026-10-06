// The "tally strip" data: one cell per app, lit when the app is installed.
// Pure functions, so the numbers on screen are testable.
import type {App, ProfileInfo} from "./model";

export interface Tally {
    have: number;
    total: number;
    /** One entry per app, in profile order: true when installed. */
    cells: { id: string; name: string; on: boolean }[];
}

export function profileTally(resolved: string[], byId: Map<string, App>): Tally {
    const cells = resolved.flatMap((id) => {
        const a = byId.get(id);
        return a ? [{id, name: a.name, on: a.installed}] : [];
    });
    return {have: cells.filter((c) => c.on).length, total: cells.length, cells};
}

export const topCategory = (a: App) => a.category.split("/")[0];

export interface CategoryStat { top: string; have: number; total: number }

/** Installed/total per top-level category, biggest first. */
export function categoryStats(apps: App[]): CategoryStat[] {
    const m = new Map<string, CategoryStat>();
    for (const a of apps) {
        const s = m.get(topCategory(a)) ?? {top: topCategory(a), have: 0, total: 0};
        s.total++;
        if (a.installed) s.have++;
        m.set(topCategory(a), s);
    }
    return [...m.values()].sort((x, y) => y.total - x.total || x.top.localeCompare(y.top));
}

/** Every app grouped by category (biggest first) then name: the order of the ruler. */
export function rulerOrder(apps: App[]): App[] {
    const rank = new Map(categoryStats(apps).map((s, i) => [s.top, i]));
    return [...apps].sort((a, b) =>
        (rank.get(topCategory(a)) ?? 0) - (rank.get(topCategory(b)) ?? 0) || a.name.localeCompare(b.name));
}

export interface Suggestion { profile: ProfileInfo; have: number; total: number; missing: number }

/** Profiles that are started but not finished, closest to done first. */
export function closestToDone(profiles: ProfileInfo[], byId: Map<string, App>, limit = 3): Suggestion[] {
    return profiles
        .map((p) => {
            const t = profileTally(p.resolved, byId);
            return {profile: p, have: t.have, total: t.total, missing: t.total - t.have};
        })
        .filter((s) => s.have > 0 && s.missing > 0 && s.missing <= 12)
        .sort((a, b) => a.missing - b.missing || b.have - a.have || a.profile.name.localeCompare(b.profile.name))
        .slice(0, limit);
}

export type SortKey = "name" | "category" | "missing";

export function sortApps(apps: App[], key: SortKey): App[] {
    const byName = (a: App, b: App) => a.name.localeCompare(b.name, undefined, {sensitivity: "base"});
    const copy = [...apps];
    switch (key) {
        case "category":
            return copy.sort((a, b) => a.category.localeCompare(b.category) || byName(a, b));
        case "missing":
            return copy.sort((a, b) => Number(a.installed) - Number(b.installed) || byName(a, b));
        default:
            return copy.sort(byName);
    }
}

/** Stable hue (0-359) for a category, used to tint app avatars. */
export function avatarHue(category: string): number {
    let h = 0;
    for (const c of category.split("/")[0]) h = (h * 31 + c.charCodeAt(0)) % 360;
    return h;
}

export function initials(name: string): string {
    const words = name.replace(/[^\p{L}\p{N} ]/gu, " ").split(/\s+/).filter(Boolean);
    if (words.length === 0) return "?";
    return (words.length === 1 ? words[0].slice(0, 2) : words[0][0] + words[1][0]).toUpperCase();
}

export function formatElapsed(ms: number): string {
    const s = Math.max(0, Math.floor(ms / 1000));
    return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

/** The one-line description to show on a card: the plain-language tagline when there is one. */
export function taglineFor(a: App, lang: "en" | "es"): string {
    return a.tagline?.[lang] || a.description;
}

/** Popular apps, in the catalog's featured order, limited to ones that exist. */
export function featuredApps(apps: App[], featured: string[], limit = featured.length): App[] {
    const byId = new Map(apps.map((a) => [a.id, a]));
    return featured.flatMap((id) => byId.get(id) ?? []).slice(0, limit);
}

/** The apps of a profile, in order, for its icon stack. */
export function profileApps(resolved: string[], byId: Map<string, App>): App[] {
    return resolved.flatMap((id) => byId.get(id) ?? []);
}
