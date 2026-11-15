import type {Profile} from "./model";

/** The profile with more apps in it. Apps already listed are not added twice, and nothing else in it changes. */
export function withApps(profile: Profile, ids: string[]): Profile {
    const have = new Set((profile.apps ?? []).map((a) => a.id));
    const added = [...new Set(ids)].filter((id) => !have.has(id)).sort().map((id) => ({id}));
    return {
        id: profile.id,
        name: profile.name,
        description: profile.description,
        kind: profile.kind,
        extends: profile.extends,
        recipes: profile.recipes,
        apps: [...(profile.apps ?? []), ...added],
    };
}

/** A day, in the language of the interface, from an RFC 3339 time; "" when it cannot be read. */
export function dayOf(iso: string | undefined, lang: string): string {
    if (!iso) return "";
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? "" : d.toLocaleDateString(lang);
}
