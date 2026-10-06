import type {ReactNode} from "react";

// A small stroke icon set drawn on a 24px grid. Icons carry meaning that words
// would otherwise have to: every category, status and action has one, so a
// screen can be scanned by shape and color before it is read.
const PATHS = {
    home: <path d="M3 11.5 12 4l9 7.5M6 10v10h12V10"/>,
    layers: <path d="M12 3l9 5-9 5-9-5zM3 13l9 5 9-5"/>,
    grid: <path d="M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z"/>,
    list: <path d="M8 6h13M8 12h13M8 18h13M3.5 6h.01M3.5 12h.01M3.5 18h.01"/>,
    download: <path d="M12 4v11m0 0-4-4m4 4 4-4M5 20h14"/>,
    pulse: <path d="M3 12h4l2-5 4 10 2-5h6"/>,
    globe: <><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c3 3.5 3 14.5 0 18M12 3c-3 3.5-3 14.5 0 18"/></>,
    code: <path d="M8 8l-5 4 5 4M16 8l5 4-5 4M14 5l-4 14"/>,
    play: <><circle cx="12" cy="12" r="9"/><path d="M10 8.5v7l6-3.5z"/></>,
    gamepad: <><rect x="3" y="8" width="18" height="10" rx="5"/><path d="M8 11v4M6 13h4M15.5 12h.01M18 14h.01"/></>,
    chat: <path d="M4 5h16v11H9l-5 4z"/>,
    palette: <><path d="M12 3a9 9 0 100 18c1.6 0 2.2-1.1 1.6-2.2-.5-1 0-2 1.4-2h2a3 3 0 003-3c0-5.6-3.6-10.8-8-10.8z"/><circle cx="8" cy="11" r=".8"/><circle cx="12" cy="7.5" r=".8"/><circle cx="16" cy="10" r=".8"/></>,
    book: <path d="M5 4h10a3 3 0 013 3v13H8a3 3 0 01-3-3zM5 17a3 3 0 013-3h10"/>,
    briefcase: <><rect x="3" y="7" width="18" height="13" rx="2"/><path d="M9 7V5h6v2M3 13h18"/></>,
    shield: <path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z"/>,
    wrench: <path d="M15 5a4 4 0 00-3.5 5.5L4 18l2 2 7.5-7.5A4 4 0 0019 9l-2.4 2.4-2-.4-.6-2z"/>,
    network: <><circle cx="12" cy="5" r="2"/><circle cx="5" cy="19" r="2"/><circle cx="19" cy="19" r="2"/><path d="M12 7v5M12 12l-6 5M12 12l6 5"/></>,
    sparkles: <path d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8zM19 16l.8 2.2L22 19l-2.2.8L19 22l-.8-2.2L16 19l2.2-.8z"/>,
    tool: <path d="M4 20l9-9M13 11l-2-2 4-4a3.5 3.5 0 014.5 4.5l-4 4zM6 4l3 3-2 2-3-3z"/>,
    cpu: <><rect x="6" y="6" width="12" height="12" rx="2"/><rect x="9" y="9" width="6" height="6"/><path d="M9 3v3M15 3v3M9 18v3M15 18v3M3 9h3M3 15h3M18 9h3M18 15h3"/></>,
    memory: <><rect x="2" y="8" width="20" height="8" rx="1.5"/><path d="M6 8v8M10 8v8M14 8v8M18 8v8M5 16v3M19 16v3"/></>,
    monitor: <><rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/></>,
    board: <><rect x="4" y="4" width="16" height="16" rx="2"/><path d="M8 8h3v3H8zM14 8h2M14 12h2M8 15h8"/></>,
    disk: <><ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/></>,
    windows: <path d="M3 5.5l7-1v7H3zM11 4.4l10-1.4v8.5H11zM3 12.5h7v7l-7-1zM11 12.5h10V21l-10-1.4z"/>,
    bios: <><rect x="5" y="3" width="14" height="18" rx="2"/><path d="M9 8h6M9 12h6M9 16h3"/></>,
    check: <path d="M5 12.5l4.5 4.5L19 7"/>,
    alert: <path d="M12 4l9 16H3zM12 10v4M12 17h.01"/>,
    info: <><circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/></>,
    x: <path d="M6 6l12 12M18 6L6 18"/>,
    trash: <path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3"/>,
    archive: <path d="M3 5h18v4H3zM5 9v10h14V9M10 13h4"/>,
    external: <path d="M14 4h6v6M20 4l-9 9M18 14v5H5V6h5"/>,
    help: <><circle cx="12" cy="12" r="9"/><path d="M9.5 9.5a2.5 2.5 0 114 2c-1 .7-1.5 1.2-1.5 2.5M12 17h.01"/></>,
    search: <><circle cx="11" cy="11" r="7"/><path d="M20 20l-4-4"/></>,
    star: <path d="M12 3l2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1L3.2 9.5l6.1-.9z"/>,
    settings: <><circle cx="12" cy="12" r="3"/><path d="M12 3v3M12 18v3M3 12h3M18 12h3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M18.4 5.6l-2.1 2.1M7.7 16.3l-2.1 2.1"/></>,
    refresh: <path d="M20 11a8 8 0 10-2 6M20 4v7h-7"/>,
    copy: <><rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V6a2 2 0 00-2-2H6a2 2 0 00-2 2v8a2 2 0 002 2h2"/></>,
    back: <path d="M15 5l-7 7 7 7"/>,
    chevron: <path d="M9 5l7 7-7 7"/>,
    plus: <path d="M12 5v14M5 12h14"/>,
    lock: <><rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V8a4 4 0 018 0v3"/></>,
    heart: <path d="M12 20s-8-5-8-11a4.5 4.5 0 018-2.5A4.5 4.5 0 0120 9c0 6-8 11-8 11z"/>,
    rocket: <path d="M5 19c0-3 1.5-5 3-6M14 4c4 0 6 2 6 6-2 4-5 6-9 7l-4-4c1-4 3-7 7-9zM15 9h.01M9 17l-3 .5L6.5 21"/>,
    code2: <path d="M9 6l-6 6 6 6M15 6l6 6-6 6"/>,
} satisfies Record<string, ReactNode>;

export type IconName = keyof typeof PATHS;

export function Icon({name, size = 18, className}: { name: IconName; size?: number; className?: string }) {
    return (
        <svg className={className} viewBox="0 0 24 24" width={size} height={size} fill="none" stroke="currentColor"
             strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
            {PATHS[name]}
        </svg>
    );
}

/** The icon for a top-level catalog category. */
const CATEGORY: Record<string, IconName> = {
    browsers: "globe", dev: "code", media: "play", gaming: "gamepad", communication: "chat", design: "palette",
    education: "book", productivity: "briefcase", security: "shield", system: "wrench", network: "network",
    ai: "sparkles", utilities: "tool", runtimes: "cpu",
};
export const categoryIcon = (top: string): IconName => CATEGORY[top] ?? "grid";
