import type {KeyboardEvent as ReactKeyboardEvent, ReactNode} from "react";

export interface TabDef<T extends string> { id: T; label: string }

export const tabId = (id: string) => `starttab-${id}`;
export const panelId = (id: string) => `startpanel-${id}`;

/**
 * An accessible tablist: one Tab stop, the arrow keys (and Home/End) move between the tabs and select
 * them, as the tabs pattern says. The panels are rendered by the caller with `Panel`.
 */
export function TabList<T extends string>(p: { label: string; tabs: TabDef<T>[]; value: T; onChange: (id: T) => void }) {
    const keys = (e: ReactKeyboardEvent<HTMLDivElement>) => {
        if (!["ArrowRight", "ArrowLeft", "Home", "End"].includes(e.key)) return;
        const at = p.tabs.findIndex((x) => x.id === p.value);
        if (at < 0) return;
        e.preventDefault();
        const next = e.key === "Home" ? 0 : e.key === "End" ? p.tabs.length - 1 : (at + (e.key === "ArrowRight" ? 1 : -1) + p.tabs.length) % p.tabs.length;
        // A tab with tabIndex -1 can still take focus from script.
        document.getElementById(tabId(p.tabs[next].id))?.focus();
        p.onChange(p.tabs[next].id);
    };
    return (
        <div className="chips starttabs" role="tablist" aria-label={p.label} onKeyDown={keys}>
            {p.tabs.map((x) => (
                <button key={x.id} id={tabId(x.id)} role="tab" aria-selected={p.value === x.id} aria-controls={panelId(x.id)}
                        tabIndex={p.value === x.id ? 0 : -1} className={p.value === x.id ? "on" : ""} onClick={() => p.onChange(x.id)}>
                    {x.label}
                </button>
            ))}
        </div>
    );
}

/** The content of one tab. Hidden (not unmounted) while another tab is shown, so its list and filters stay. */
export function Panel(p: { id: string; active: boolean; children: ReactNode }) {
    return (
        <div role="tabpanel" id={panelId(p.id)} aria-labelledby={tabId(p.id)} hidden={!p.active} tabIndex={-1}>
            {p.children}
        </div>
    );
}
