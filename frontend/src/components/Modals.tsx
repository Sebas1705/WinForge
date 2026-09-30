import {useEffect, useRef, useState} from "react";
import {Strip} from "./Strip";
import {t} from "../lib/i18n";
import type {Plan, StepStatus} from "../lib/model";
import * as prefs from "../lib/settings";
import {formatElapsed} from "../lib/tally";

export interface RunState {
    title: string;
    plan: Plan;
    status: Record<string, StepStatus>;
    log: string[];
    running: boolean;
    failed: string[] | null;
    startedAt: number | null;
    /** Whether the plan can be exported as a script (profile plans only). */
    canScript: boolean;
}

export const stepKey = (s: { kind: string; id: string }) => `${s.kind}:${s.id}`;

const ICON: Record<StepStatus, string> = {pending: "○", running: "◔", ok: "✓", failed: "✗", skipped: "–"};

export function RunModal(p: {
    run: RunState; admin: boolean;
    onConfirm: () => void; onCancel: () => void; onClose: () => void; onAdmin: () => void;
    onScript: () => void; onCopy: () => void;
}) {
    const {run} = p;
    const started = run.running || run.failed !== null;
    const done = run.plan.steps.filter((s) => ["ok", "failed", "skipped"].includes(run.status[stepKey(s)] ?? "")).length;
    const logRef = useRef<HTMLPreElement>(null);
    const [now, setNow] = useState(Date.now());
    const [end, setEnd] = useState<number | null>(null);

    useEffect(() => { logRef.current?.scrollTo(0, logRef.current.scrollHeight); }, [run.log]);
    useEffect(() => {
        if (!run.running) return;
        const id = window.setInterval(() => setNow(Date.now()), 500);
        return () => window.clearInterval(id);
    }, [run.running]);
    useEffect(() => { if (run.failed !== null && end === null) setEnd(Date.now()); }, [run.failed, end]);
    useEffect(() => {
        const esc = (e: KeyboardEvent) => { if (e.key === "Escape" && !run.running) p.onClose(); };
        window.addEventListener("keydown", esc);
        return () => window.removeEventListener("keydown", esc);
    });

    const cells = run.plan.steps.map((s) => {
        const st = run.status[stepKey(s)] ?? "pending";
        return {on: st === "ok", state: st, name: s.name};
    });
    const heading = !started ? run.title : run.running ? t("run.installing") : run.failed?.length ? t("run.errors") : t("run.done");
    const elapsed = run.startedAt ? formatElapsed((end ?? now) - run.startedAt) : "";
    const label = (s: { kind: string; name: string }) =>
        s.kind === "recipe" ? `${t("run.setup")}: ${s.name}` : s.kind === "upgrade" ? `${t("run.update")}: ${s.name}` : s.name;

    return (
        <div className="overlay">
            <div className="modal" role="dialog" aria-modal="true" aria-label={heading}>
                <h3>{heading}</h3>
                {started && (
                    <div className="progress">
                        <Strip cells={cells} size="lg"/>
                        <span className="mono muted">{t("run.stepOf", {done, total: cells.length})}{elapsed && ` · ${elapsed}`}</span>
                    </div>
                )}
                {!started && run.plan.needsAdmin && !p.admin && (
                    <div className="banner warn">
                        <span>{t("run.needsAdmin")}</span>
                        <button onClick={p.onAdmin}>{t("run.restartAdmin")}</button>
                    </div>
                )}
                <ul className="steps">
                    {run.plan.steps.map((s) => {
                        const st = run.status[stepKey(s)] ?? "pending";
                        return (
                            <li key={stepKey(s)} className={st}>
                                <span className="ico" aria-hidden>{ICON[st]}</span>
                                <span>{label(s)}</span>
                                {s.version && s.kind === "upgrade" && <span className="mono muted">→ {s.version}</span>}
                                {s.admin && <span className="tag">{t("badge.admin")}</span>}
                            </li>
                        );
                    })}
                </ul>
                {started && <pre ref={logRef} className="log mono">{run.log.join("\n")}</pre>}
                {started && !run.running && run.failed !== null && <p className="muted small">{t("run.reboot")}</p>}
                <div className="actions end">
                    {!started && run.canScript && <button className="ghost" onClick={p.onScript}>{t("run.exportScript")}</button>}
                    {started && <button className="ghost" onClick={p.onCopy}>{t("run.copyLog")}</button>}
                    <span className="grow"/>
                    {!started && <button onClick={p.onClose}>{t("common.cancel")}</button>}
                    {!started && <button className="primary" onClick={p.onConfirm}>{t("run.steps", {n: run.plan.steps.length})}</button>}
                    {run.running && <button className="danger" onClick={p.onCancel}>{t("run.stop")}</button>}
                    {started && !run.running && <button className="primary" onClick={p.onClose}>{t("common.close")}</button>}
                </div>
            </div>
        </div>
    );
}

export function NameDialog(p: { title: string; initial: string; onOk: (n: string) => void; onClose: () => void }) {
    const [v, setV] = useState(p.initial);
    const submit = () => { if (v.trim()) { p.onOk(v.trim()); p.onClose(); } };
    return (
        <div className="overlay" onClick={p.onClose}>
            <div className="modal small" role="dialog" aria-modal="true" aria-label={p.title} onClick={(e) => e.stopPropagation()}>
                <h3>{p.title}</h3>
                <input autoFocus value={v} placeholder={t("dlg.name")} onChange={(e) => setV(e.target.value)}
                       onKeyDown={(e) => { if (e.key === "Enter") submit(); if (e.key === "Escape") p.onClose(); }}/>
                <div className="actions end">
                    <button onClick={p.onClose}>{t("common.cancel")}</button>
                    <button className="primary" disabled={!v.trim()} onClick={submit}>{t("dlg.save")}</button>
                </div>
            </div>
        </div>
    );
}

function Segmented<T extends string>(p: { value: T; options: [T, string][]; onChange: (v: T) => void }) {
    return (
        <div className="seg" role="group">
            {p.options.map(([v, label]) => (
                <button key={v} className={p.value === v ? "on" : ""} aria-pressed={p.value === v} onClick={() => p.onChange(v)}>{label}</button>
            ))}
        </div>
    );
}

export function SettingsDialog(p: {
    version: string; value: prefs.Settings; onChange: (s: prefs.Settings) => void; onCheck: () => void; onClose: () => void;
}) {
    const set = (patch: Partial<prefs.Settings>) => p.onChange({...p.value, ...patch});
    useEffect(() => {
        const esc = (e: KeyboardEvent) => { if (e.key === "Escape") p.onClose(); };
        window.addEventListener("keydown", esc);
        return () => window.removeEventListener("keydown", esc);
    });
    return (
        <div className="overlay" onClick={p.onClose}>
            <div className="modal small" role="dialog" aria-modal="true" aria-label={t("settings.title")} onClick={(e) => e.stopPropagation()}>
                <h3>{t("settings.title")}</h3>
                <div className="field"><span>{t("settings.language")}</span>
                    <Segmented value={p.value.language} onChange={(language) => set({language})}
                               options={[["auto", t("settings.auto")], ["es", "Español"], ["en", "English"]]}/>
                </div>
                <div className="field"><span>{t("settings.theme")}</span>
                    <Segmented value={p.value.theme} onChange={(theme) => set({theme})}
                               options={[["system", t("settings.system")], ["dark", t("settings.dark")], ["light", t("settings.light")]]}/>
                </div>
                <div className="field"><span>{t("settings.accent")}</span>
                    <div className="swatches">
                        {prefs.ACCENTS.map((c) => (
                            <button key={c.value} title={c.name} aria-label={c.name} aria-pressed={p.value.accent === c.value}
                                    className={"swatch" + (p.value.accent === c.value ? " on" : "")} style={{background: c.value}}
                                    onClick={() => set({accent: c.value})}/>
                        ))}
                    </div>
                </div>
                <div className="field"><span>{t("settings.density")}</span>
                    <Segmented value={p.value.density} onChange={(density) => set({density})}
                               options={[["comfortable", t("settings.comfortable")], ["compact", t("settings.compact")]]}/>
                </div>
                <div className="field"><span>{t("settings.updates")}</span>
                    <label className="check"><input type="checkbox" checked={p.value.checkUpdates}
                           onChange={(e) => set({checkUpdates: e.target.checked})}/> {t("settings.checkAtStart")}</label>
                </div>
                <div className="field"><span className="muted mono">{t("settings.version", {v: p.version})}</span>
                    <button onClick={p.onCheck}>{t("settings.checkNow")}</button>
                </div>
                <div className="actions end"><button className="primary" onClick={p.onClose}>{t("common.done")}</button></div>
            </div>
        </div>
    );
}
