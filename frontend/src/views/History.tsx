import {useEffect, useId, useRef, useState} from "react";
import {api, errText, type HistoryEntry, type HistoryPage} from "../api";
import {AppIcon} from "../components/AppIcon";
import {Icon} from "../components/Icon";
import {ConfirmDialog} from "../components/Modals";
import {getLang, t, tOpt} from "../lib/i18n";
import type {App} from "../lib/model";

/** How many entries one "Show more" adds. */
export const PAGE = 20;

/** The Go side's codes, as sentences. Anything else is shown as it came. */
export function historyError(e: unknown, name = ""): { text: string; refresh: boolean } {
    const m = errText(e);
    if (m.includes("history.unknown")) return {text: t("hist.error.unknown"), refresh: true};
    if (m.includes("history.notRollbackable")) return {text: t("hist.error.notRollbackable"), refresh: true};
    if (m.includes("history.moved")) return {text: t("hist.error.moved", {name}), refresh: true};
    return {text: m, refresh: false};
}

/** Local calendar day of a time, as a sortable key; "" when the time cannot be read. */
export function dayKey(at: string): string {
    const d = new Date(at);
    if (Number.isNaN(d.getTime())) return "";
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

/** Entries (already newest first) grouped by local day, keeping their order. */
export function groupByDay(entries: HistoryEntry[]): { day: string; at: string; entries: HistoryEntry[] }[] {
    const groups: { day: string; at: string; entries: HistoryEntry[] }[] = [];
    for (const e of entries) {
        const day = dayKey(e.at);
        const last = groups[groups.length - 1];
        if (last && last.day === day) last.entries.push(e);
        else groups.push({day, at: e.at, entries: [e]});
    }
    return groups;
}

function versions(e: HistoryEntry): string {
    if (e.from && e.to) return t("hist.versionChange", {from: e.from, to: e.to});
    if (e.to) return t("hist.versionTo", {to: e.to});
    if (e.from) return t("hist.versionFrom", {from: e.from});
    return t("hist.versionUnknown");
}

const time = (at: string) => {
    const d = new Date(at);
    return Number.isNaN(d.getTime()) ? "" : d.toLocaleTimeString(getLang(), {hour: "2-digit", minute: "2-digit"});
};
const dayLabel = (at: string) => {
    const d = new Date(at);
    return Number.isNaN(d.getTime()) ? "?" : d.toLocaleDateString(getLang(), {weekday: "long", day: "numeric", month: "long", year: "numeric"});
};

/**
 * What WinForge installed, updated and rolled back. An update whose previous version is known, while the app is
 * still at the version it installed, has a "Go back" button; it asks first, says what can go wrong and offers to
 * skip the newer version so the app is not updated straight back. The run itself goes through the same window as
 * every install (onRollback); this view never changes anything.
 */
export function History(p: {
    byId: Map<string, App>; refreshKey: number; say: (m: string) => void;
    onRollback: (e: HistoryEntry, skipNewer: boolean) => void;
}) {
    const [page, setPage] = useState<HistoryPage | null>(null);
    const [app, setApp] = useState("");
    const [result, setResult] = useState("");
    const [limit, setLimit] = useState(PAGE);
    const [failed, setFailed] = useState("");
    const [asking, setAsking] = useState<HistoryEntry | null>(null);
    const [skip, setSkip] = useState(false);
    const asked = useRef(0);
    const headId = useId();

    useEffect(() => {
        const n = ++asked.current; // an older answer must not replace a newer one
        api.HistoryList({app, result, limit}).then(
            (r) => { if (n === asked.current) { setPage(r); setFailed(""); } },
            (e) => { if (n === asked.current) { setFailed(errText(e)); setPage((v) => v ?? {entries: [], total: 0, apps: []}); } },
        );
    }, [app, result, limit, p.refreshKey]);

    const filtered = app !== "" || result !== "";
    const entries = page?.entries ?? [];
    const total = page?.total ?? 0;
    const known = page?.apps ?? [];
    // An app filter that no longer exists (it was the only entry) is dropped, not left selecting nothing.
    useEffect(() => { if (app && page && !page.apps.some((a) => a.id === app)) setApp(""); }, [page, app]);

    const ask = (e: HistoryEntry) => { setSkip(false); setAsking(e); };
    const rollback = (e: HistoryEntry) => p.onRollback(e, skip);

    return (
        <section className="history" aria-labelledby={headId}>
            <h2 id={headId}>{t("hist.title")}</h2>
            <p className="muted small">{t("hist.intro")}</p>

            {page === null && <p className="muted">{t("hist.loading")}</p>}
            {failed && <p className="muted" role="alert">{t("hist.loadFailed", {error: failed})}</p>}
            {page !== null && !filtered && total === 0 && !failed && <p className="muted">{t("hist.empty")}</p>}

            {page !== null && (filtered || total > 0) && (
                <>
                    <div className="toolrow">
                        <label className="check">{t("hist.filterApp")}
                            <select value={app} onChange={(e) => { setApp(e.target.value); setLimit(PAGE); }}>
                                <option value="">{t("hist.allApps")}</option>
                                {known.map((a) => <option key={a.id} value={a.id}>{p.byId.get(a.id)?.name ?? a.name}</option>)}
                            </select>
                        </label>
                        <label className="check">{t("hist.filterResult")}
                            <select value={result} onChange={(e) => { setResult(e.target.value); setLimit(PAGE); }}>
                                <option value="">{t("hist.allResults")}</option>
                                <option value="ok">{t("hist.result.ok")}</option>
                                <option value="failed">{t("hist.result.failed")}</option>
                                <option value="restart">{t("hist.result.restart")}</option>
                            </select>
                        </label>
                        <span className="muted" role="status">{t("hist.summary", {shown: entries.length, total})}</span>
                    </div>
                    {entries.length === 0 && <p className="muted" role="status">{t("hist.noMatch")}</p>}
                    {groupByDay(entries).map((g) => (
                        <div key={g.day || g.at} className="histday">
                            <h3>{dayLabel(g.at)}</h3>
                            <ul className="list histlist" aria-label={`${t("hist.list")}: ${dayLabel(g.at)}`}>
                                {g.entries.map((e) => {
                                    const cat = p.byId.get(e.app);
                                    const name = cat?.name ?? e.name;
                                    const why = e.result === "failed" ? (e.reason ? tOpt(`run.reason.${e.reason}`) ?? e.reason : t("hist.noReason")) : "";
                                    return (
                                        <li key={e.id}>
                                            <AppIcon id={e.app} name={name} category={cat?.category ?? ""}/>
                                            <span className="grow histmain">
                                                <span className="catname">
                                                    <b>{name}</b>
                                                    <span className="chip">{t(`hist.action.${e.action}` as "hist.action.update")}</span>
                                                    <span className={e.result === "ok" ? "chip done" : e.result === "failed" ? "chip review" : "chip"}>
                                                        {e.result === "failed" && <Icon name="alert" size={12}/>}
                                                        {e.result === "ok" && <Icon name="check" size={12}/>}
                                                        {t(`hist.result.${e.result}` as "hist.result.ok")}
                                                    </span>
                                                </span>
                                                <small className="muted">{t(`hist.source.${e.source}` as "hist.source.single")} · {time(e.at)}</small>
                                                {why && <small className="why">{why}</small>}
                                            </span>
                                            <span className="mono muted">{versions(e)}</span>
                                            {e.canRollback && (
                                                <button type="button" className="sm" aria-label={t("hist.goBackFor", {name, version: e.from})} onClick={() => ask(e)}>
                                                    <Icon name="back" size={14}/> {t("hist.goBack", {version: e.from})}
                                                </button>
                                            )}
                                        </li>
                                    );
                                })}
                            </ul>
                        </div>
                    ))}
                    {entries.length < total && (
                        <button type="button" className="histmore" onClick={() => setLimit(limit + PAGE)}>{t("hist.showMore")}</button>
                    )}
                </>
            )}

            {asking && (
                <ConfirmDialog title={t("hist.confirmTitle", {name: p.byId.get(asking.app)?.name ?? asking.name, version: asking.from})} confirm={t("hist.confirm")}
                               body={t("hist.confirmBody", {name: p.byId.get(asking.app)?.name ?? asking.name, version: asking.from, current: asking.to})}
                               onConfirm={() => rollback(asking)} onClose={() => setAsking(null)}>
                    <p className="muted small">{t("hist.confirmWarn", {name: p.byId.get(asking.app)?.name ?? asking.name})}</p>
                    <label className="check">
                        <input type="checkbox" checked={skip} onChange={(e) => setSkip(e.target.checked)}/>
                        {t("hist.confirmSkip", {newer: asking.to, name: p.byId.get(asking.app)?.name ?? asking.name})}
                    </label>
                </ConfirmDialog>
            )}
        </section>
    );
}
