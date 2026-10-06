// App icons are static files shipped with the app (frontend/public/icons, built
// by `winforge-icons`) plus an index mapping catalog id -> file name.
import {createContext} from "react";

export type IconIndex = Record<string, string>;

export const IconsContext = createContext<IconIndex>({});

/** Loads the icon index; a missing or broken index just means "no icons". */
export async function loadIcons(fetcher: typeof fetch = fetch): Promise<IconIndex> {
    try {
        const r = await fetcher("/icons/index.json");
        if (!r.ok) return {};
        const data: unknown = await r.json();
        if (!data || typeof data !== "object") return {};
        return Object.fromEntries(Object.entries(data as Record<string, unknown>).filter(([, v]) => typeof v === "string")) as IconIndex;
    } catch {
        return {};
    }
}

export function iconUrl(index: IconIndex, id: string): string | null {
    const f = index[id];
    return f ? `/icons/${encodeURIComponent(f)}` : null;
}
