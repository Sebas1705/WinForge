import {useCallback, useEffect, useRef, useState} from "react";
import {api, type AutoItem, type AutoReport, type AutoSettings, type AutoState} from "../api";
import {Icon} from "../components/Icon";
import {ConfirmDialog} from "../components/Modals";
import {autoError, dateText, dayLabel, itemNote, modeText, runErrorText, summaryParts, upToDate, whenText} from "../lib/auto";
import {getLang, t} from "../lib/i18n";
import "../auto.css";

const DEFAULTS: AutoSettings = {mode: "notify", frequency: "daily", weekday: 1, time: "09:00"};
const DAYS = [1, 2, 3, 4, 5, 6, 0]; // Monday first

const formOf = (s: AutoState): AutoSettings => ({mode: s.mode, frequency: s.frequency, weekday: s.weekday, time: s.time});
const lang = (): "es" | "en" => (getLang() === "es" ? "es" : "en");

function Items(p: { items: AutoItem[] }) {
    if (p.items.length === 0) return <p className="muted small">{t("auto.run.noItems")}</p>;
    return (
        <ul className="autoitems">
            {p.items.map((i) => {
                const note = itemNote(i);
                return (
                    <li key={i.id} className={`r-${i.result}`}>
                        <span className="grow">
                            <b>{i.name}</b> <span className="mono muted">{t("updates.from", {from: i.from, to: i.to})}</span>
                        </span>
                        <span className="autoresult">
                            {(i.result === "failed" || i.result === "needs-admin") && <Icon name="alert" size={13}/>}
                            {i.result === "updated" && <Icon name="check" size={13}/>}
                            {t(`auto.result.${i.result}`)}
                            {note && <span className="muted"> ({note})</span>}
                            {i.reboot && <span className="muted"> ({t("auto.run.restart")})</span>}
                        </span>
                    </li>
                );
            })}
        </ul>
    );
}

function Run(p: { run: AutoReport }) {
    const [open, setOpen] = useState(false);
    const date = dateText(p.run.started);
    const head = t("auto.run.title", {date, mode: modeText(p.run.mode)});
    const id = `auto-run-${p.run.id}`;
    return (
        <li className={`autorun s-${p.run.status}`}>
            <div className="autorunhead">
                <span className="grow">
                    <b>{head}</b>
                    <span className="chip">{(p.run.status === "failed" || p.run.status === "partial" || p.run.status === "error") && <Icon name="alert" size={12}/>}{t(`auto.status.${p.run.status}`)}</span>
                    <span className="muted small"> {p.run.error ? runErrorText(p.run.error) : summaryParts(p.run.items).join(" · ")}</span>
                </span>
                <button type="button" className="sm ghost" aria-expanded={open} aria-controls={id}
                        aria-label={t(open ? "auto.run.hide" : "auto.run.show", {date})} onClick={() => setOpen(!open)}>
                    <Icon name="chevron" size={14} className={open ? "turn" : ""}/>
                </button>
            </div>
            {open && <div id={id}><Items items={p.run.items}/></div>}
        </li>
    );
}

/**
 * The "Automatic updates" card: a per-user Windows scheduled task that checks for updates (and, when
 * the person chose "Update for me", installs the ones that may be installed unattended), with the
 * last runs underneath. Every sentence is ours; Go only sends codes.
 */
export function AutoUpdates() {
    const [st, setSt] = useState<AutoState | null>(null);
    const [form, setForm] = useState<AutoSettings>(DEFAULTS);
    const [open, setOpen] = useState(false);
    const [busy, setBusy] = useState<"save" | "off" | "check" | null>(null);
    const [msg, setMsg] = useState("");
    const [err, setErr] = useState("");
    const [check, setCheck] = useState<AutoReport | null>(null);
    const [asking, setAsking] = useState(false);
    const [showRuns, setShowRuns] = useState(false);
    const opener = useRef<HTMLButtonElement>(null);

    const load = useCallback(async () => {
        try {
            const s = await api.AutoState();
            setSt(s);
            setForm(s.on ? formOf(s) : DEFAULTS);
        } catch (e) { setErr(autoError(e)); }
    }, []);
    useEffect(() => { void load(); }, [load]);

    const valid = /^([01]\d|2[0-3]):[0-5]\d$/.test(form.time);
    const unchangedApply = st?.on === true && st.mode === "apply";

    const save = async () => {
        setBusy("save"); setErr(""); setMsg("");
        try {
            const next = await api.AutoEnable({...form, weekday: form.frequency === "weekly" ? form.weekday : 0}, lang());
            setSt(next);
            setForm(formOf(next));
            setOpen(false);
            setMsg(t("auto.done.on", {mode: modeText(next.mode), when: whenText(next)}));
            setTimeout(() => opener.current?.focus(), 0);
        } catch (e) { setErr(autoError(e)); } finally { setBusy(null); }
    };
    const submit = () => {
        if (!valid) return;
        if (form.mode === "apply" && !unchangedApply) setAsking(true); // unattended installs are asked about first
        else void save();
    };
    const turnOff = async () => {
        setBusy("off"); setErr(""); setMsg("");
        try {
            const next = await api.AutoDisable();
            setSt(next);
            setForm(DEFAULTS);
            setOpen(false);
            setMsg(t("auto.done.off"));
        } catch (e) { setErr(autoError(e)); } finally { setBusy(null); }
    };
    const checkNow = async () => {
        setBusy("check"); setErr(""); setMsg(t("auto.status.checking")); setCheck(null);
        try {
            const r = await api.AutoRunNow();
            setCheck(r);
            setMsg(r.error ? runErrorText(r.error) : upToDate(r) ? t("auto.run.none") : summaryParts(r.items).join(", "));
        } catch (e) { setMsg(""); setErr(autoError(e)); } finally { setBusy(null); }
    };

    const problem = st?.problem ? t(`auto.problem.${st.problem}`) : "";
    const status = !st ? "" : !st.on
        ? t("auto.status.off")
        : t("auto.status.on", {mode: modeText(st.mode), when: whenText(st)});

    return (
        <section className="autocard" aria-labelledby="auto-title">
            <h2 id="auto-title">{t("auto.title")}</h2>
            <p className="muted small">{t("auto.intro")}</p>

            {st && (
                <>
                    <p className="autostatus"><b>{status}</b>
                        {st.on && st.enabled && st.nextRun && <span className="muted"> {t("auto.status.next", {date: dateText(st.nextRun)})}</span>}
                    </p>
                    {st.on && !st.enabled && <p className="autowarn"><Icon name="alert" size={14}/> {t("auto.status.switchedOff")}</p>}
                    {st.on && st.foreign && <p className="autowarn"><Icon name="alert" size={14}/> {t("auto.status.foreign")}</p>}
                    {problem && <p className="autowarn" role="alert"><Icon name="alert" size={14}/> {problem}</p>}
                    <div className="actions tight">
                        <button ref={opener} type="button" aria-expanded={open} aria-controls="auto-form" disabled={!st.on && !st.canEnable}
                                onClick={() => setOpen(!open)}>{st.on ? t("auto.change") : t("auto.setup")}</button>
                        <button type="button" disabled={busy !== null} onClick={() => void checkNow()} title={t("auto.checkNow.hint")} aria-describedby="auto-check-hint">
                            <Icon name="refresh" size={14}/> {t("auto.checkNow")}
                        </button>
                        <span id="auto-check-hint" className="sr-only">{t("auto.checkNow.hint")}</span>
                        {st.on && <button type="button" className="danger" disabled={busy !== null} onClick={() => void turnOff()}>{t("auto.turnOff")}</button>}
                    </div>
                </>
            )}

            {/* A polite live region without role=status: the page already has one status line of its own. */}
            <p className="sr-only" aria-live="polite" aria-atomic="true">{msg}</p>
            {msg && <p className="muted small" aria-hidden="true">{msg}</p>}
            {err && <p className="autowarn" role="alert"><Icon name="alert" size={14}/> {err}</p>}

            {st && open && (
                <form id="auto-form" className="autoform" onSubmit={(e) => { e.preventDefault(); submit(); }}>
                    <fieldset>
                        <legend>{t("auto.mode.legend")}</legend>
                        {(["notify", "apply"] as const).map((m) => (
                            <label key={m} className="autochoice">
                                <input type="radio" name="auto-mode" checked={form.mode === m} onChange={() => setForm({...form, mode: m})}/>
                                <span><b>{t(`auto.mode.${m}`)}</b><small className="muted">{t(`auto.mode.${m}.hint`)}</small></span>
                            </label>
                        ))}
                    </fieldset>
                    <fieldset>
                        <legend>{t("auto.freq.legend")}</legend>
                        {(["daily", "weekly"] as const).map((f) => (
                            <label key={f} className="autochoice">
                                <input type="radio" name="auto-freq" checked={form.frequency === f} onChange={() => setForm({...form, frequency: f})}/>
                                <span><b>{t(`auto.freq.${f}`)}</b></span>
                            </label>
                        ))}
                        <div className="autorow">
                            {form.frequency === "weekly" && (
                                <label className="check">{t("auto.weekday")}
                                    <select value={form.weekday} onChange={(e) => setForm({...form, weekday: Number(e.target.value)})}>
                                        {DAYS.map((d) => <option key={d} value={d}>{dayLabel(d)}</option>)}
                                    </select>
                                </label>
                            )}
                            <label className="check">{t("auto.time")}
                                <input type="time" value={form.time} required aria-invalid={!valid} onChange={(e) => setForm({...form, time: e.target.value})}/>
                            </label>
                        </div>
                    </fieldset>
                    <p className="muted small">{t("auto.note")}</p>
                    <div className="actions end">
                        <button type="button" onClick={() => { setOpen(false); setForm(st.on ? formOf(st) : DEFAULTS); opener.current?.focus(); }}>{t("common.cancel")}</button>
                        <button type="submit" className="primary" disabled={busy !== null || !valid}>{st.on ? t("auto.save") : t("auto.turnOn")}</button>
                    </div>
                </form>
            )}

            {check && (
                <div className="autocheck" role="region" aria-label={t("auto.run.result")}>
                    <h3>{t("auto.run.result")}</h3>
                    {check.error
                        ? <p className="autowarn"><Icon name="alert" size={14}/> {runErrorText(check.error)}</p>
                        : upToDate(check) && check.items.length === 0 ? <p className="muted">{t("auto.run.none")}</p> : <Items items={check.items}/>}
                </div>
            )}

            {st && (
                <div className="autoruns">
                    <h3>
                        <button type="button" className="link toggle" aria-expanded={showRuns} aria-controls="auto-runs" onClick={() => setShowRuns(!showRuns)}>
                            <Icon name="chevron" size={14} className={showRuns ? "turn" : ""}/> {t("auto.runs", {n: st.runs.length})}
                        </button>
                    </h3>
                    {showRuns && (st.runs.length === 0
                        ? <p className="muted small" id="auto-runs">{t("auto.runs.empty")}</p>
                        : <ul className="autorunlist" id="auto-runs" aria-label={t("auto.runs.list")}>{st.runs.map((r) => <Run key={r.id} run={r}/>)}</ul>)}
                </div>
            )}

            {asking && (
                <ConfirmDialog title={t("auto.confirm.title")} confirm={t("auto.confirm.ok")}
                               body={t("auto.confirm.body", {when: whenText(form)})}
                               onConfirm={() => void save()} onClose={() => setAsking(false)}>
                    <ul className="autopromise">
                        {([1, 2, 3, 4, 5, 6] as const).map((n) => <li key={n}>{t(`auto.confirm.${n}`)}</li>)}
                    </ul>
                </ConfirmDialog>
            )}
        </section>
    );
}
