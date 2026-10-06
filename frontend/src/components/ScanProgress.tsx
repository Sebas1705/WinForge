import {useEffect, useState} from "react";
import {Icon} from "./Icon";
import {Ring} from "./Visual";
import {t, type Key} from "../lib/i18n";

/** The stages each scan reports, in order. Names match the Go side. */
export const SCAN_STEPS = {
    pc: ["registry", "winget", "match"],
    health: ["system", "firmware", "security", "hardware", "storage", "drivers", "devices", "report"],
} as const;
export type ScanKind = keyof typeof SCAN_STEPS;

/**
 * What a scan is doing right now: a checklist where finished stages are ticked,
 * the current one pulses and the rest wait, plus the seconds spent so far.
 * Without it a slow scan looks like a frozen window.
 */
export function ScanProgress(p: { kind: ScanKind; current: string | null; compact?: boolean }) {
    const steps = SCAN_STEPS[p.kind];
    const at = p.current ? steps.indexOf(p.current as never) : -1;
    const [secs, setSecs] = useState(0);
    useEffect(() => {
        const started = Date.now();
        setSecs(0);
        const id = window.setInterval(() => setSecs(Math.floor((Date.now() - started) / 1000)), 1000);
        return () => window.clearInterval(id);
    }, [p.kind]);

    return (
        <div className={"scanprogress" + (p.compact ? " compact" : "")} role="status" aria-live="polite">
            <Ring size={p.compact ? 56 : 84} stroke={p.compact ? 6 : 8} value={Math.max(at, 0) / steps.length} tone="accent">
                <span className="ringnum">{Math.max(at, 0)}<small>/{steps.length}</small></span>
            </Ring>
            <div className="grow">
                <b>{at >= 0 ? t(`scan.${p.kind}.${steps[at]}` as Key) : t("scan.starting")}</b>
                <p className="muted small">{t("scan.elapsed", {s: secs})}{secs >= 20 && at >= 0 ? ` · ${t("scan.slow")}` : ""}</p>
                {!p.compact && (
                    <ul className="scansteps">
                        {steps.map((s, i) => (
                            <li key={s} className={i < at ? "done" : i === at ? "now" : ""}>
                                {i <= at ? <Icon name={i < at ? "check" : "pulse"} size={14}/> : <span className="pending" aria-hidden="true"/>}
                                {t(`scan.${p.kind}.${s}.short` as Key)}
                            </li>
                        ))}
                    </ul>
                )}
            </div>
        </div>
    );
}
