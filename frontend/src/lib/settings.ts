// User appearance settings: pure logic plus a thin, failure-tolerant
// localStorage wrapper (storage can be unavailable in some webviews).

export type Theme = "system" | "dark" | "light";
export type Density = "comfortable" | "compact";

export interface Settings {
    theme: Theme;
    accent: string;
    density: Density;
}

export const ACCENTS: { name: string; value: string }[] = [
    {name: "Blue", value: "#4f9dff"},
    {name: "Violet", value: "#8b6cff"},
    {name: "Green", value: "#3ecf8e"},
    {name: "Orange", value: "#ff9f43"},
    {name: "Pink", value: "#ff6b9d"},
];

export const DEFAULTS: Settings = {theme: "system", accent: ACCENTS[0].value, density: "comfortable"};

const KEY = "winforge.settings";

/** Coerces anything into valid settings, falling back per field. */
export function normalize(raw: unknown): Settings {
    const r = (raw && typeof raw === "object" ? raw : {}) as Record<string, unknown>;
    return {
        theme: r.theme === "dark" || r.theme === "light" || r.theme === "system" ? r.theme : DEFAULTS.theme,
        accent: typeof r.accent === "string" && /^#[0-9a-f]{6}$/i.test(r.accent) ? r.accent : DEFAULTS.accent,
        density: r.density === "compact" || r.density === "comfortable" ? r.density : DEFAULTS.density,
    };
}

export function load(): Settings {
    try {
        return normalize(JSON.parse(localStorage.getItem(KEY) ?? "null"));
    } catch {
        return DEFAULTS;
    }
}

export function save(s: Settings): void {
    try {
        localStorage.setItem(KEY, JSON.stringify(s));
    } catch { /* not persisted; the session still works */ }
}

export function resolveTheme(t: Theme, prefersDark: boolean): "dark" | "light" {
    return t === "system" ? (prefersDark ? "dark" : "light") : t;
}

/** Writes the settings to the document root, where the CSS reads them. */
export function apply(s: Settings, root: HTMLElement, prefersDark: boolean): void {
    root.dataset.theme = resolveTheme(s.theme, prefersDark);
    root.dataset.density = s.density;
    root.style.setProperty("--accent", s.accent);
}
