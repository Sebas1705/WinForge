import {useCallback, useEffect, useMemo, useState} from "react";
import {api, errText, type PrivacyBackup, type PrivacyChange, type PrivacyTweak} from "../api";
import {Icon} from "../components/Icon";
import {ConfirmDialog} from "../components/Modals";
import {backupTime} from "../lib/developer";
import {t, tOpt, type Key} from "../lib/i18n";
import "../privacy.css";

const AREAS = ["ads", "suggestions", "start", "web", "feedback"];

const nameOf = (id: string) => tOpt(`privacy.t.${id}.name`) ?? id;
const listOf = (ids: string[]) => ids.map(nameOf).join(", ");

/** What a value holds now, as the preview words it. */
function nowText(c: PrivacyChange): string {
    if (c.error) return t("privacy.pv.unreadable");
    return c.existed ? t("privacy.pv.line", {type: c.oldType, value: c.old}) : t("privacy.pv.absent");
}

function Preview(p: { tweak: PrivacyTweak }) {
    const {tweak} = p;
    const resetting = tweak.state === "private";
    return (
        <details className="ppreview">
            <summary>{t("privacy.preview")}</summary>
            <ul className="plain small" aria-label={`${t("privacy.preview")}: ${nameOf(tweak.id)}`}>
                {tweak.changes.map((c) => (
                    <li key={c.path + c.name}>
                        <span className="mono pvpath">{c.path}</span>
                        <b className="mono">{c.name}</b>
                        <span className="pvline">
                            {t("privacy.pv.now")}: <span className="mono">{nowText(c)}</span>
                            {" → "}
                            {resetting ? t("privacy.afterReset") : t("privacy.pv.apply")}: <span className="mono">
                                {resetting ? t("privacy.pv.removed") : t("privacy.pv.line", {type: c.type, value: c.private})}
                            </span>
                        </span>
                    </li>
                ))}
            </ul>
            {tweak.state !== "default" && <p className="muted small">{t("privacy.pv.resetNote")}</p>}
        </details>
    );
}

function Row(p: { tweak: PrivacyTweak; checked: boolean; busy: boolean; onToggle: () => void; onReset: () => void }) {
    const {tweak: w} = p;
    const name = nameOf(w.id);
    const locked = w.state === "private" || w.state === "unknown";
    return (
        <li className={p.checked ? "on" : ""}>
            <div className="prow">
                <label>
                    <input type="checkbox" checked={p.checked} disabled={p.busy || locked} onChange={p.onToggle} aria-label={t("privacy.select", {name})}/>
                    <span className="grow">
                        <span className="catname">
                            <b>{name}</b>
                            <span className={w.state === "private" ? "chip done" : w.state === "default" ? "chip" : "chip review"}
                                  title={tOpt(`privacy.stateTip.${w.state}`) ?? undefined}>
                                {w.state === "private" && <Icon name="check" size={12}/>}
                                {(w.state === "custom" || w.state === "unknown") && <Icon name="alert" size={12}/>}
                                {tOpt(`privacy.state.${w.state}`) ?? w.state}
                            </span>
                        </span>
                        <small className="muted">{tOpt(w.whyKey) ?? ""}</small>
                        {w.state === "unknown" && <small className="muted">{t("privacy.stateTip.unknown")}</small>}
                    </span>
                </label>
                {w.state !== "default" && w.state !== "unknown" && (
                    <button className="sm" disabled={p.busy} title={t("privacy.resetTip")} aria-label={t("privacy.resetAria", {name})} onClick={p.onReset}>
                        <Icon name="back" size={13}/> {t("privacy.reset")}
                    </button>
                )}
            </div>
            <Preview tweak={w}/>
        </li>
    );
}

/**
 * A short, reviewed list of per-user Windows privacy settings. Each shows its state from the registry and the exact
 * values it would write. Nothing is ticked by default; "Apply selected" asks first, saves a backup of every value
 * before the first write, and the backups below put everything back exactly.
 */
export function Privacy(p: { say: (m: string) => void }) {
    const {say} = p;
    const [tweaks, setTweaks] = useState<PrivacyTweak[] | null>(null);
    const [backups, setBackups] = useState<PrivacyBackup[]>([]);
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [asking, setAsking] = useState(false);
    const [restoring, setRestoring] = useState<PrivacyBackup | null>(null);
    const [lines, setLines] = useState<string[]>([]);

    const loadBackups = useCallback(async () => {
        try { setBackups(await api.PrivacyBackups()); } catch (e) { say(errText(e)); }
    }, [say]);

    const load = useCallback(async () => {
        setError("");
        try {
            setTweaks(await api.PrivacyList());
            setSelected(new Set());
        } catch (e) { setError(errText(e)); setTweaks([]); }
    }, []);

    useEffect(() => { void load(); void loadBackups(); }, [load, loadBackups]);

    const list = tweaks ?? [];
    const chosen = list.filter((w) => selected.has(w.id) && w.state !== "private" && w.state !== "unknown");
    const privateCount = list.filter((w) => w.state === "private").length;
    const lastUndoable = backups.find((b) => !b.restored) ?? null;
    const groups = useMemo(() => AREAS.map((a) => ({area: a, items: list.filter((w) => w.area === a)})).filter((g) => g.items.length > 0), [list]);

    const toggle = (id: string) => setSelected((s) => {
        const n = new Set(s);
        if (n.has(id)) n.delete(id); else n.add(id);
        return n;
    });

    const run = async (ids: string[], reset: boolean) => {
        if (ids.length === 0) return;
        setBusy(true);
        const out: string[] = [];
        try {
            const r = await (reset ? api.PrivacyReset(ids) : api.PrivacyApply(ids));
            out.push(r.applied > 0 ? t(reset ? "privacy.resetResult" : "privacy.result", {n: r.applied}) : t("privacy.resultNone"));
            if (r.skipped.length > 0) out.push(t("privacy.resultSkipped", {list: listOf(r.skipped)}));
            if (r.failed.length > 0) out.push(t("privacy.resultFailed", {list: listOf(r.failed)}));
            if (r.applied > 0) out.push(t("privacy.restartHint"));
        } catch (e) { say(errText(e)); setBusy(false); return; }
        setLines(out);
        setBusy(false);
        await Promise.all([load(), loadBackups()]);
    };

    const restore = async (b: PrivacyBackup) => {
        setBusy(true);
        const out: string[] = [];
        try {
            const r = await api.PrivacyRestore(b.id);
            if (r.failed.length > 0) out.push(t("privacy.undoneFailed", {list: listOf(r.failed)}));
            if (r.restored.length > 0) out.push(r.changedSince.length > 0 ? t("privacy.undoneChanged", {list: listOf(r.changedSince)}) : t("privacy.undone"));
        } catch (e) { say(errText(e)); setBusy(false); return; }
        setLines(out);
        setBusy(false);
        await Promise.all([load(), loadBackups()]);
    };

    return (
        <div className="page privacypage">
            <header className="pagehead">
                <div>
                    <h1>{t("privacy.title")}</h1>
                    <p className="muted">{t("privacy.intro")}</p>
                </div>
            </header>
            <p className="muted small"><Icon name="shield" size={14}/> {t("privacy.safety")}</p>
            <p className="muted small">{t("privacy.excluded")}</p>

            <div className="toolrow">
                <button disabled={busy} onClick={() => { setLines([]); void load(); void loadBackups(); }}><Icon name="refresh" size={15}/> {t("privacy.refresh")}</button>
                {lastUndoable && (
                    <button disabled={busy} onClick={() => void restore(lastUndoable)}><Icon name="back" size={15}/> {t("privacy.undoLast")}</button>
                )}
                {tweaks !== null && !error && <span className="muted" role="status">{t("privacy.summary", {n: privateCount, total: list.length})}</span>}
            </div>

            {tweaks === null && <p className="muted" role="status">{t("privacy.loading")}</p>}
            {error && <p className="muted" role="alert">{t("privacy.error", {error})}</p>}
            {lines.length > 0 && (
                <div className="resultline" role="status">
                    {lines.map((l) => <p key={l}>{l}</p>)}
                </div>
            )}

            {groups.map((g) => {
                const title = t(`privacy.area.${g.area}` as Key);
                return (
                    <section key={g.area} aria-label={title}>
                        <h2 className="subhead">{title}</h2>
                        <ul className="cleanlist privacylist" aria-label={t("privacy.list", {area: title})}>
                            {g.items.map((w) => (
                                <Row key={w.id} tweak={w} checked={selected.has(w.id)} busy={busy} onToggle={() => toggle(w.id)} onReset={() => void run([w.id], true)}/>
                            ))}
                        </ul>
                    </section>
                );
            })}

            {list.length > 0 && (
                <div className="toolrow">
                    <button className="primary" disabled={busy || chosen.length === 0} onClick={() => setAsking(true)}>
                        <Icon name="shield" size={15}/> {t("privacy.apply", {n: chosen.length})}
                    </button>
                    <span className="muted small">{t("privacy.hint")}</span>
                </div>
            )}

            <h2 className="subhead">{t("privacy.backups")}</h2>
            {backups.length === 0 ? <p className="muted small">{t("privacy.backupsNone")}</p> : (
                <ul className="plain small privacybackups" aria-label={t("privacy.backups")}>
                    {backups.map((b) => (
                        <li key={b.id} className="backupitem">
                            <span className="grow">
                                {t("privacy.backupItem", {time: backupTime(b.time), action: t(`privacy.backupAction.${b.action}` as Key), names: listOf(b.tweaks)})}
                                {b.restored && <> <span className="chip">{t("privacy.backupRestored")}</span></>}
                            </span>
                            <button className="sm" disabled={busy} aria-label={t("privacy.restoreAria", {time: backupTime(b.time)})} onClick={() => setRestoring(b)}>
                                {t("privacy.restore")}
                            </button>
                        </li>
                    ))}
                </ul>
            )}

            {asking && (
                <ConfirmDialog title={t("privacy.confirmTitle", {n: chosen.length})} body={t("privacy.confirmBody")} confirm={t("privacy.confirm")}
                               onConfirm={() => void run(chosen.map((w) => w.id), false)} onClose={() => setAsking(false)}>
                    <ul className="plain small pvconfirm" aria-label={t("privacy.confirmTitle", {n: chosen.length})}>
                        {chosen.map((w) => (
                            <li key={w.id}>
                                <b>{nameOf(w.id)}</b>
                                <ul className="plain">
                                    {w.changes.map((c) => (
                                        <li key={c.path + c.name} className="mono">
                                            {c.path}\{c.name}: {nowText(c)} → {t("privacy.pv.line", {type: c.type, value: c.private})}
                                        </li>
                                    ))}
                                </ul>
                            </li>
                        ))}
                    </ul>
                </ConfirmDialog>
            )}
            {restoring && (
                <ConfirmDialog title={t("privacy.restoreTitle")} body={t("privacy.restoreBody")} confirm={t("privacy.restore")}
                               onConfirm={() => void restore(restoring)} onClose={() => setRestoring(null)}>
                    <p className="small muted">{t("privacy.backupItem", {time: backupTime(restoring.time), action: t(`privacy.backupAction.${restoring.action}` as Key), names: listOf(restoring.tweaks)})}</p>
                </ConfirmDialog>
            )}
        </div>
    );
}
