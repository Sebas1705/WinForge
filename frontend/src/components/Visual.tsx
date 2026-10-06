import {useContext, type ReactNode} from "react";
import {AppIcon} from "./AppIcon";
import {Icon} from "./Icon";
import {IconsContext, iconUrl} from "../lib/icons";
import type {App} from "../lib/model";

/** A circular progress gauge with something readable in the middle. */
export function Ring({value, size = 64, stroke = 7, tone = "accent", children}: {
    value: number; size?: number; stroke?: number; tone?: "accent" | "ok" | "warn" | "bad" | "muted"; children?: ReactNode;
}) {
    const v = Math.max(0, Math.min(1, value));
    const r = (size - stroke) / 2;
    const c = 2 * Math.PI * r;
    return (
        <div className={`ring ${tone}`} style={{width: size, height: size}} role="img" aria-label={`${Math.round(v * 100)}%`}>
            <svg viewBox={`0 0 ${size} ${size}`} width={size} height={size} aria-hidden>
                <circle className="track" cx={size / 2} cy={size / 2} r={r} fill="none" strokeWidth={stroke}/>
                <circle className="fill" cx={size / 2} cy={size / 2} r={r} fill="none" strokeWidth={stroke} strokeLinecap="round"
                        strokeDasharray={`${c * v} ${c}`} transform={`rotate(-90 ${size / 2} ${size / 2})`}/>
            </svg>
            <div className="ring-center">{children}</div>
        </div>
    );
}

/** Overlapping app icons: a profile's contents at a glance, no reading required. */
export function IconStack({apps, max = 5, size = 34}: { apps: App[]; max?: number; size?: number }) {
    const index = useContext(IconsContext);
    // Apps that have a real icon go first, so the stack looks like the profile.
    const ordered = [...apps].sort((a, b) => Number(!!iconUrl(index, b.id)) - Number(!!iconUrl(index, a.id)));
    const shown = ordered.slice(0, max);
    const rest = apps.length - shown.length;
    return (
        <div className="stack-icons" style={{height: size}}>
            {shown.map((a, i) => (
                <span key={a.id} style={{marginLeft: i === 0 ? 0 : -size * 0.28, zIndex: shown.length - i}}>
                    <AppIcon id={a.id} name={a.name} category={a.category} size={size}/>
                </span>
            ))}
            {rest > 0 && <span className="more" style={{height: size, minWidth: size, marginLeft: -size * 0.28}}>+{rest}</span>}
        </div>
    );
}

/** A small "?" that explains a term in plain words, on hover or keyboard focus. */
export function Tip({text}: { text: string }) {
    return (
        <span className="tip" tabIndex={0} role="note" aria-label={text} data-tip={text}>
            <Icon name="help" size={14}/>
        </span>
    );
}
