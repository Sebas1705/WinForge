import {useEffect, useRef, useState} from "react";
import {AppIcon} from "./AppIcon";
import {Icon} from "./Icon";
import {Strip} from "./Strip";
import {useModal} from "./useModal";
import {Ring} from "./Visual";
import {t} from "../lib/i18n";
import type {App, Plan, Step, StepStatus} from "../lib/model";
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

const ICON: Record<StepStatus, "check" | "x" | "play" | "chevron"> = {pending: "chevron", running: "play", ok: "check", failed: "x", skipped: "chevron"};

export function RunModal(p: {
    run: RunState; admin: boolean; advanced: boolean; byId: Map<string, App>;
    onConfirm: () => void; onCancel: () => void; onClose: () => void; onAdmin: () => void;
    onScript: () => void; onCopy: () => void;
}) {
    const {run} = p;
    const started = run.running || run.failed !== null;
    const finished = started && !run.running;
    const stateOf = (s: Step): StepStatus => run.status[stepKey(s)] ?? "pending";
    const done = run.plan.steps.filter((s) => ["ok", "failed", "skipped"].includes(stateOf(s))).length;
    const failedCount = run.plan.steps.filter((s) => stateOf(s) === "failed").length;
    const current = run.plan.steps.find((s) => stateOf(s) === "running");
    const logRef = useRef<HTMLPreElement>(null);
    const dialog = useRef<HTMLDivElement>(null);
    const [now, setNow] = useState(Date.now());
    const [end, setEnd] = useState<number | null>(null);
    const [showLog, setShowLog] = useState(p.advanced);
    const [stopping, setStopping] = useState(false);

    // Escape closes the dialog only when nothing is running: stopping an
    // installation is a deliberate act with its own button.
    useModal(dialog, run.running ? undefined : p.onClose);

    useEffect(() => { logRef.current?.scrollTo?.(0, logRef.current.scrollHeight); }, [run.log, showLog]);
    useEffect(() => {
        if (!run.running) return;
        const id = window.setInterval(() => setNow(Date.now()), 500);
        return () => window.clearInterval(id);
    }, [run.running]);
    useEffect(() => { if (run.failed !== null && end === null) setEnd(Date.now()); }, [run.failed, end]);
    useEffect(() => { if (!run.running) setStopping(false); }, [run.running]);
    // A failed run should show its reason without a click.
    useEffect(() => { if (finished && failedCount > 0) setShowLog(true); }, [finished, failedCount]);

    const cells = run.plan.steps.map((s) => ({on: stateOf(s) === "ok", state: stateOf(s), name: s.name}));
    const heading = !started ? run.title : run.running ? t("run.installing") : failedCount > 0 ? t("run.errors") : t("run.done");
    const elapsed = run.startedAt ? formatElapsed((end ?? now) - run.startedAt) : "";
    const label = (s: Step) =>
        s.kind === "recipe" ? `${t("run.setup")}: ${s.name}` : s.kind === "upgrade" ? `${t("run.update")}: ${s.name}` : s.name;
    const stepIcon = (s: Step) => {
        const app = s.kind === "recipe" ? undefined : p.byId.get(s.id);
        return app ? <AppIcon id={app.id} name={app.name} category={app.category} size={26}/>
            : <span className="avatar recipe" style={{width: 26, height: 26}}><Icon name="settings" size={14}/></span>;
    };
    const currentApp = current && current.kind !== "recipe" ? p.byId.get(current.id) : undefined;

    return (
        <div className="overlay">
            <div className="modal" ref={dialog} role="dialog" aria-modal="true" aria-label={heading}>
                {started ? (
                    <div className="runhead">
                        <Ring size={92} stroke={9} value={cells.length ? done / cells.length : 0} tone={finished ? (failedCount ? "warn" : "ok") : "accent"}>
                            {finished ? <Icon name={failedCount ? "alert" : "check"} size={30}/> : <span className="ringnum">{done}<small>/{cells.length}</small></span>}
                        </Ring>
                        <div className="grow">
                            <h3>{heading}</h3>
                            {current && (
                                <p className="now">
                                    {currentApp ? <AppIcon id={currentApp.id} name={currentApp.name} category={currentApp.category} size={28}/> : null}
                                    <span>{t("run.working", {name: current.name})}</span>
                                </p>
                            )}
                            {finished && <p className="muted">{failedCount > 0 ? t("run.failedHint") : t("run.reboot")}</p>}
                            <Strip cells={cells} size="lg"/>
                            <span className="mono muted small">{t("run.stepOf", {done, total: cells.length})}{elapsed && ` · ${elapsed}`}</span>
                        </div>
                    </div>
                ) : <h3>{heading}</h3>}

                {!started && run.plan.needsAdmin && !p.admin && (
                    <div className="banner warn">
                        <Icon name="lock" size={16}/>
                        <span>{t("run.needsAdmin")}</span>
                        <button onClick={p.onAdmin}>{t("run.restartAdmin")}</button>
                    </div>
                )}

                <ul className="steps">
                    {run.plan.steps.map((s) => {
                        const st = stateOf(s);
                        return (
                            <li key={stepKey(s)} className={st}>
                                {stepIcon(s)}
                                <span className="grow">{label(s)}</span>
                                {s.version && s.kind === "upgrade" && p.advanced && <span className="mono muted">→ {s.version}</span>}
                                {s.admin && <span className="tag"><Icon name="lock" size={11}/></span>}
                                <span className={`stat ${st}`} aria-label={st}><Icon name={ICON[st]} size={15}/></span>
                            </li>
                        );
                    })}
                </ul>

                {started && (
                    <>
                        <button className="link toggle" onClick={() => setShowLog(!showLog)} aria-expanded={showLog}>
                            <Icon name="chevron" size={14} className={showLog ? "turn" : ""}/> {t("run.details")}
                        </button>
                        {showLog && <pre ref={logRef} className="log mono">{run.log.join("\n")}</pre>}
                    </>
                )}

                <div className="actions end">
                    {!started && run.canScript && p.advanced && <button className="ghost" onClick={p.onScript}>{t("run.exportScript")}</button>}
                    {started && showLog && <button className="ghost" onClick={p.onCopy}>{t("run.copyLog")}</button>}
                    <span className="grow"/>
                    {!started && <button onClick={p.onClose}>{t("common.cancel")}</button>}
                    {!started && <button className="primary big" onClick={p.onConfirm}><Icon name="download" size={16}/> {t("run.steps", {n: run.plan.steps.length})}</button>}
                    {run.running && (
                        <button className="danger" disabled={stopping} onClick={() => { setStopping(true); p.onCancel(); }}>
                            {stopping ? t("run.stopping") : t("run.stop")}
                        </button>
                    )}
                    {finished && <button className="primary" onClick={p.onClose}>{t("common.close")}</button>}
                </div>
            </div>
        </div>
    );
}

export function NameDialog(p: { title: string; initial: string; onOk: (n: string) => void; onClose: () => void }) {
    const [v, setV] = useState(p.initial);
    const dialog = useRef<HTMLDivElement>(null);
    useModal(dialog, p.onClose);
    const submit = () => { if (v.trim()) { p.onOk(v.trim()); p.onClose(); } };
    return (
        <div className="overlay" onClick={p.onClose}>
            <div className="modal small" ref={dialog} role="dialog" aria-modal="true" aria-label={p.title} onClick={(e) => e.stopPropagation()}>
                <h3>{p.title}</h3>
                <input data-autofocus value={v} maxLength={60} placeholder={t("dlg.name")} aria-label={t("dlg.name")} onChange={(e) => setV(e.target.value)}
                       onKeyDown={(e) => { if (e.key === "Enter") submit(); }}/>
                <div className="actions end">
                    <button onClick={p.onClose}>{t("common.cancel")}</button>
                    <button className="primary" disabled={!v.trim()} onClick={submit}>{t("dlg.save")}</button>
                </div>
            </div>
        </div>
    );
}

/** Asks before something that cannot be undone. The safe answer is the focused one. */
export function ConfirmDialog(p: { title: string; body: string; confirm: string; danger?: boolean; onConfirm: () => void; onClose: () => void }) {
    const dialog = useRef<HTMLDivElement>(null);
    useModal(dialog, p.onClose);
    return (
        <div className="overlay" onClick={p.onClose}>
            <div className="modal small" ref={dialog} role="alertdialog" aria-modal="true" aria-label={p.title} onClick={(e) => e.stopPropagation()}>
                <h3>{p.title}</h3>
                <p className="muted">{p.body}</p>
                <div className="actions end">
                    <button data-autofocus onClick={p.onClose}>{t("common.cancel")}</button>
                    <button className={p.danger ? "danger" : "primary"} onClick={() => { p.onConfirm(); p.onClose(); }}>{p.confirm}</button>
                </div>
            </div>
        </div>
    );
}

export function Segmented<T extends string>(p: { value: T; options: [T, string][]; onChange: (v: T) => void }) {
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
    const dialog = useRef<HTMLDivElement>(null);
    useModal(dialog, p.onClose);
    return (
        <div className="overlay" onClick={p.onClose}>
            <div className="modal small" ref={dialog} role="dialog" aria-modal="true" aria-label={t("settings.title")} onClick={(e) => e.stopPropagation()}>
                <h3>{t("settings.title")}</h3>
                <div className="field"><span>{t("settings.detail")}</span>
                    <Segmented value={p.value.detail} onChange={(detail) => set({detail})}
                               options={[["simple", t("mode.simple")], ["advanced", t("mode.advanced")]]}/>
                </div>
                <p className="muted small nogap">{t("mode.hint")}</p>
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

/** First-run guide: three screens, each one idea, one picture. */
export function Tour(p: { onClose: () => void }) {
    const [i, setI] = useState(0);
    const dialog = useRef<HTMLDivElement>(null);
    useModal(dialog, p.onClose);
    const slides = [
        {icon: "layers" as const, title: t("tour.1.title"), body: t("tour.1.body")},
        {icon: "check" as const, title: t("tour.2.title"), body: t("tour.2.body")},
        {icon: "pulse" as const, title: t("tour.3.title"), body: t("tour.3.body")},
    ];
    const s = slides[i];
    const last = i === slides.length - 1;
    return (
        <div className="overlay">
            <div className="modal tour" ref={dialog} role="dialog" aria-modal="true" aria-label={s.title}>
                <span className="bubble hero"><Icon name={s.icon} size={44}/></span>
                <h3>{s.title}</h3>
                <p>{s.body}</p>
                <div className="dots" aria-hidden>{slides.map((_, n) => <i key={n} className={n === i ? "on" : ""}/>)}</div>
                <div className="actions end">
                    {!last && <button className="ghost" onClick={p.onClose}>{t("tour.skip")}</button>}
                    <button className="primary big" onClick={() => (last ? p.onClose() : setI(i + 1))}>{last ? t("tour.start") : t("tour.next")}</button>
                </div>
            </div>
        </div>
    );
}
