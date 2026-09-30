import type {App} from "../lib/model";
import {rulerOrder, topCategory} from "../lib/tally";
import {useMemo} from "react";

export interface Cell { on: boolean; name?: string; state?: string }

/**
 * The tally strip: one cell per item, lit ember when done. It is the app's one
 * recurring motif, and every lit cell is a fact (an installed app, a finished
 * step), never decoration.
 */
export function Strip({cells, size = "md", label}: { cells: Cell[]; size?: "sm" | "md" | "lg"; label?: string }) {
    const on = cells.filter((c) => c.on).length;
    return (
        <div className={`strip ${size}${cells.length > 120 ? " dense" : ""}`} role="img" aria-label={label ?? `${on}/${cells.length}`}>
            {cells.map((c, i) => <i key={i} className={(c.on ? "on " : "") + (c.state ?? "")} title={c.name}/>)}
        </div>
    );
}

/** Every catalog app as one tick, grouped by category with a gap between groups. */
export function Ruler({apps}: { apps: App[] }) {
    const groups = useMemo(() => {
        const out: { top: string; items: App[] }[] = [];
        for (const a of rulerOrder(apps)) {
            const top = topCategory(a);
            if (out.length === 0 || out[out.length - 1].top !== top) out.push({top, items: []});
            out[out.length - 1].items.push(a);
        }
        return out;
    }, [apps]);
    return (
        <div className="ruler" role="img" aria-label={`${apps.filter((a) => a.installed).length}/${apps.length}`}>
            {groups.map((g) => (
                <div className="ruler-group" key={g.top} style={{flexGrow: g.items.length}}>
                    {g.items.map((a) => <i key={a.id} className={a.installed ? "on" : ""} title={a.name}/>)}
                </div>
            ))}
        </div>
    );
}
