import {useCallback, useEffect, useMemo, useRef, useState} from "react";
import {api, errText, type EnvBackup, type EnvFinding, type EnvFixResult, type EnvReport} from "../../api";
import {Icon} from "../../components/Icon";
import {ConfirmDialog} from "../../components/Modals";
import {backupTime, findingText, fixText, groupFindings, groupTitle, selectable, type KindGroup} from "../../lib/developer";
import {t, tOpt} from "../../lib/i18n";
import {runJob, useRefreshWhenOtherEnds} from "../../lib/runJob";
import {getTasks, runningOf, tasksOf, useTasks} from "../../lib/taskStore";

const PAGE = 15;

/**
 * The last scan, kept outside the component so a scan that ends while the page is not shown is not lost.
 * It is dropped when the Tasks list is empty (nothing has run since the store was reset or cleared).
 */
let lastReport: EnvReport | null = null;

function Finding(p: { f: EnvFinding; checked: boolean; busy: boolean; onToggle: () => void }) {
    const {f} = p;
    const text = findingText(f);
    const paths = f.paths.filter((x) => x !== f.subject || f.paths.length > 1);
    const body = (
        <span className="grow">
            <span className="catname">
                <b>{text}</b>
                {f.severity === "problem"
                    ? <span className="chip review"><Icon name="alert" size={12}/> {t("dev.env.severity.problem")}</span>
                    : <span className="chip"><Icon name="info" size={12}/> {t("dev.env.severity.note")}</span>}
                {f.scope === "machine" && <span className="chip admin"><Icon name="lock" size={12}/> {t("dev.env.machineChip")}</span>}
            </span>
            {f.scope === "machine" && <small className="muted">{t("dev.env.machineNote")}</small>}
            {paths.map((x) => <small key={x} className="mono muted" title={x}>{x}</small>)}
        </span>
    );
    if (!selectable(f)) return <li><div className="finding">{body}</div></li>;
    return (
        <li>
            <label className={p.checked ? "on" : ""}>
                <input type="checkbox" checked={p.checked} disabled={p.busy} onChange={p.onToggle} aria-label={t("dev.env.select", {text})}/>
                {body}
            </label>
        </li>
    );
}

function Tools(p: { report: EnvReport }) {
    const {report} = p;
    const aliases = new Set(report.findings.filter((f) => f.kind === "tool-store-alias").map((f) => f.subject.toLowerCase()));
    return (
        <>
            <h3 className="subhead">{t("dev.env.tools")}</h3>
            <p className="muted small">{t("dev.env.toolsIntro")}</p>
            {report.tools.length === 0 ? <p className="muted small">{t("dev.env.toolsNone")}</p> : (
                <div className="tablewrap" tabIndex={0} role="region" aria-label={t("dev.env.tools")}>
                    <table>
                        <thead>
                            <tr>
                                <th scope="col">{t("dev.env.toolName")}</th>
                                <th scope="col">{t("dev.env.toolWins")}</th>
                                <th scope="col">{t("dev.env.toolOthers")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {report.tools.map((x) => (
                                <tr key={x.name}>
                                    <th scope="row">{x.name}</th>
                                    <td>
                                        {x.found.length === 0 ? <span className="muted">{t("dev.env.toolMissing")}</span> : <span className="mono">{x.found[0]}</span>}
                                        {aliases.has(x.name.toLowerCase()) && <> <span className="chip review"><Icon name="alert" size={12}/> {t("dev.env.storeChip")}</span></>}
                                    </td>
                                    <td>{x.found.slice(1).map((o) => <div key={o} className="mono muted">{o}</div>)}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
            {aliases.size > 0 && <p className="muted small">{t("dev.env.storeBody")}</p>}
        </>
    );
}

/**
 * Reads PATH and the tool variables (JAVA_HOME and the like) and explains what is wrong. Scanning only reads.
 * Only user-scope findings can be fixed, only the ones the person ticks, and a backup is kept so it can be undone.
 */
export function EnvDoctor(p: { say: (m: string) => void }) {
    const {say} = p;
    const [report, setReport] = useState<EnvReport | null>(() => {
        if (tasksOf(getTasks(), "developer").length === 0) lastReport = null;
        return lastReport;
    });
    // Busy comes from the task store, so it still holds after leaving the page and coming back.
    const running = runningOf(useTasks("developer"));
    const busy = running !== undefined;
    const own = useRef(false);
    const [error, setError] = useState("");
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [asking, setAsking] = useState(false);
    const [restoring, setRestoring] = useState<EnvBackup | null>(null);
    const [result, setResult] = useState<EnvFixResult | null>(null);
    const [undone, setUndone] = useState(false);
    const [backups, setBackups] = useState<EnvBackup[]>([]);
    const [shown, setShown] = useState<Record<string, number>>({});

    const loadBackups = useCallback(async () => {
        try { setBackups(await api.EnvBackups()); } catch (e) { say(errText(e)); }
    }, [say]);
    useEffect(() => { void loadBackups(); }, [loadBackups]);

    const scan = useCallback(async () => {
        setError("");
        own.current = true;
        const r = await runJob("developer", say, {
            title: t("tasks.dev.scan"), tag: "scan",
            summarize: (v: EnvReport) => {
                const problems = v.findings.filter((f) => f.severity === "problem").length;
                return v.findings.length === 0 ? t("dev.env.good") : t("dev.env.summary", {problems, notes: v.findings.length - problems});
            },
        }, () => api.EnvScan());
        if (!r) { own.current = false; return; }
        if (!r.ok) { setError(errText(r.error)); return; }
        lastReport = r.value;
        setReport(r.value);
        setSelected(new Set());
        setShown({});
    }, [say]);

    // A job that ended while this page was away: read its outcome (the report, the backups) again.
    useRefreshWhenOtherEnds(running, own, (tag) => {
        if (tag === "scan") { setReport(lastReport); setSelected(new Set()); setShown({}); return; }
        void loadBackups();
        if (report !== null) void scan();
    });

    const findings = report?.findings ?? [];
    const groups = useMemo(() => groupFindings(findings), [findings]);
    const problems = findings.filter((f) => f.severity === "problem").length;
    const chosen = findings.filter((f) => selected.has(f.id) && selectable(f));

    const toggle = (id: string) => setSelected((s) => {
        const n = new Set(s);
        if (n.has(id)) n.delete(id); else n.add(id);
        return n;
    });

    const fix = async () => {
        const ids = chosen.map((f) => f.id);
        if (ids.length === 0) return;
        setUndone(false);
        own.current = true;
        const r = await runJob("developer", say, {
            title: t("tasks.dev.fix", {n: ids.length}), tag: "fix",
            summarize: (v: EnvFixResult) => (v.applied > 0 ? t("dev.env.result", {n: v.applied}) : t("dev.env.resultNone")),
        }, () => api.EnvFix(ids));
        if (!r) { own.current = false; return; }
        if (!r.ok) { say(errText(r.error)); return; }
        setResult(r.value);
        await loadBackups();
        await scan();
    };

    const restore = async (id: string, fromResult: boolean) => {
        own.current = true;
        const r = await runJob("developer", say, {title: t("tasks.dev.restore"), tag: "restore", summarize: () => t("dev.env.undone")}, () => api.EnvRestore(id));
        if (!r) { own.current = false; return; }
        if (!r.ok) { say(errText(r.error)); return; }
        setUndone(true);
        if (fromResult) setResult(null);
        await loadBackups();
        await scan();
    };

    const more = (g: KindGroup) => setShown((s) => ({...s, [g.kind]: (s[g.kind] ?? PAGE) + PAGE}));

    return (
        <section aria-labelledby="dev-env">
            <div className="sectionhead">
                <h2 id="dev-env">{t("dev.env.title")}</h2>
                <button className="primary" disabled={busy} onClick={() => { setResult(null); setUndone(false); void scan(); }}>
                    <Icon name="search" size={15}/> {report === null ? t("dev.env.scan") : t("dev.env.rescan")}
                </button>
            </div>
            <p className="muted small">{t("dev.env.intro")}</p>

            {busy && report === null && <p className="muted" role="status">{t("dev.env.scanning")}</p>}
            {error && <p className="muted" role="alert">{t("dev.env.error", {error})}</p>}

            {(result || undone) && (
                <div className="resultline" role="status">
                    {result && (
                        <>
                            <p>{result.applied > 0 ? t("dev.env.result", {n: result.applied}) : t("dev.env.resultNone")}</p>
                            {result.failed.length > 0 && <p className="muted small">{t("dev.env.resultFailed", {list: result.failed.join(", ")})}</p>}
                            {result.backupId && <p><button className="sm" disabled={busy} onClick={() => void restore(result.backupId, true)}><Icon name="back" size={13}/> {t("dev.env.undo")}</button></p>}
                        </>
                    )}
                    {!result && undone && <p>{t("dev.env.undone")}</p>}
                </div>
            )}

            {report !== null && !error && (
                <>
                    {findings.length === 0 ? (
                        <p className="resultline" role="status"><Icon name="check" size={15}/> {t("dev.env.good")}</p>
                    ) : (
                        <>
                            <p role="status"><b>{t("dev.env.summary", {problems, notes: findings.length - problems})}</b></p>
                            {groups.map((g) => {
                                const limit = shown[g.kind] ?? PAGE;
                                return (
                                    <div key={g.kind}>
                                        <h3 className="subhead">{groupTitle(g)} <span className="muted small">({g.items.length})</span></h3>
                                        <ul className="cleanlist" aria-label={t("dev.env.list", {kind: groupTitle(g)})}>
                                            {g.items.slice(0, limit).map((f) => (
                                                <Finding key={f.id} f={f} checked={selected.has(f.id)} busy={busy} onToggle={() => toggle(f.id)}/>
                                            ))}
                                        </ul>
                                        {g.items.length > limit && <p><button className="sm" onClick={() => more(g)}>{t("dev.env.more", {n: g.items.length - limit})}</button></p>}
                                    </div>
                                );
                            })}
                            {findings.some(selectable) && (
                                <div className="toolrow">
                                    <button className="primary" disabled={busy || chosen.length === 0} onClick={() => setAsking(true)}>
                                        <Icon name="wrench" size={15}/> {t("dev.env.fix", {n: chosen.length})}
                                    </button>
                                    <span className="muted small">{t("dev.env.fixHint")}</span>
                                </div>
                            )}
                        </>
                    )}
                    <p className="muted small">{t("dev.env.paths", {user: report.userPath.length, machine: report.machinePath.length})}</p>
                    <Tools report={report}/>
                    {report.vars.length > 0 && (
                        <details className="envvars">
                            <summary>{t("dev.env.vars", {n: report.vars.length})}</summary>
                            <div className="tablewrap" tabIndex={0} role="region" aria-label={t("dev.env.vars", {n: report.vars.length})}>
                                <table>
                                    <thead>
                                        <tr>
                                            <th scope="col">{t("dev.env.varName")}</th>
                                            <th scope="col">{t("dev.env.varScope")}</th>
                                            <th scope="col">{t("dev.env.varValue")}</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {report.vars.map((v) => (
                                            <tr key={`${v.scope}|${v.name}`}>
                                                <th scope="row">{v.name}</th>
                                                <td>{tOpt(`dev.env.varScope.${v.scope}`) ?? v.scope}</td>
                                                <td className="mono">{v.value}</td>
                                            </tr>
                                        ))}
                                    </tbody>
                                </table>
                            </div>
                        </details>
                    )}
                </>
            )}

            <h3 className="subhead">{t("dev.env.backups")}</h3>
            {backups.length === 0 ? <p className="muted small">{t("dev.env.backupsNone")}</p> : (
                <ul className="plain small">
                    {backups.map((b) => (
                        <li key={b.id} className="backupitem">
                            <span className="grow">{t("dev.env.backupItem", {time: backupTime(b.time), changed: b.changed.join(", ")})}</span>
                            <button className="sm" disabled={busy} aria-label={t("dev.env.restoreAria", {time: backupTime(b.time)})} onClick={() => setRestoring(b)}>
                                {t("dev.env.restore")}
                            </button>
                        </li>
                    ))}
                </ul>
            )}

            {asking && (
                <ConfirmDialog title={t("dev.env.confirmTitle", {n: chosen.length})} body={t("dev.env.confirmBody")} confirm={t("dev.env.confirm")}
                               onConfirm={() => void fix()} onClose={() => setAsking(false)}>
                    <ul className="plain small" aria-label={t("dev.env.confirmTitle", {n: chosen.length})}>
                        {chosen.map((f) => <li key={f.id} className="mono">{fixText(f)}</li>)}
                    </ul>
                </ConfirmDialog>
            )}
            {restoring && (
                <ConfirmDialog title={t("dev.env.restoreTitle")} body={t("dev.env.restoreBody")} confirm={t("dev.env.restore")}
                               onConfirm={() => void restore(restoring.id, false)} onClose={() => setRestoring(null)}>
                    <p className="small muted">{t("dev.env.backupItem", {time: backupTime(restoring.time), changed: restoring.changed.join(", ")})}</p>
                </ConfirmDialog>
            )}
        </section>
    );
}
