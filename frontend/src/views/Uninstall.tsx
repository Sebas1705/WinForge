import {useCallback, useEffect, useMemo, useRef, useState} from "react";
import {api, errText, type LeftoverCandidate, type LeftoversRecycleResult, type LeftoversResult, type UninstallApp, type UninstallListing} from "../api";
import {Icon} from "../components/Icon";
import {ConfirmDialog} from "../components/Modals";
import {formatBytes, formatDate} from "../lib/format";
import type {App} from "../lib/model";
import {getLang, t} from "../lib/i18n";
import {markedBytes, nameList, notInstalledMatches, placeName, reasonText, uninstallError, visibleApps, whyText} from "../lib/uninstall";

type CatalogApp = Pick<App, "id" | "name" | "installed">;

interface Leftovers {
    appId: string;
    name: string;
    result: LeftoversResult | null; // null while searching
    candidates: LeftoverCandidate[];
    marked: Set<string>;
    recycled: LeftoversRecycleResult | null;
    confirming: boolean;
    busy: boolean;
}

const fileCount = (n: number): string => t(n === 1 ? "uninst.left.files.one" : "uninst.left.files.other", {n});

/**
 * Uninstalls catalog apps and, afterwards, reviews what they left behind. Nothing starts ticked, uninstalling
 * asks first, and leftovers only ever go to the Recycle Bin after a second question; the Go side only accepts
 * the paths its own search listed.
 */
export function Uninstall(p: {
    apps: CatalogApp[]; admin: boolean; say: (m: string) => void; onAdmin: () => void;
    /** Bumped when a run finished, so the list is read again. */
    refreshKey: number;
    onUninstall: (apps: UninstallApp[]) => void;
}) {
    const [listing, setListing] = useState<UninstallListing | null>(null);
    const [query, setQuery] = useState("");
    const [marked, setMarked] = useState<Set<string>>(new Set());
    const [asking, setAsking] = useState(false);
    const [manual, setManual] = useState("");
    const [left, setLeft] = useState<Leftovers | null>(null);
    const panel = useRef<HTMLElement>(null);
    const search = useRef(0); // which search the screen is waiting for; an older answer is ignored

    const load = useCallback(async () => {
        try {
            const l = await api.UninstallList();
            setListing(l);
            setMarked((m) => new Set([...m].filter((id) => l.apps.some((a) => a.id === id))));
        } catch (e) { p.say(errText(e)); setListing((v) => v ?? {apps: [], removed: []}); }
    }, [p.say]);
    useEffect(() => { void load(); }, [load, p.refreshKey]);

    const apps = listing?.apps ?? [];
    const shown = useMemo(() => visibleApps(apps, query), [apps, query]);
    const chosen = apps.filter((a) => marked.has(a.id));
    const needAdmin = chosen.filter((a) => a.admin).length;

    const toggle = (id: string) => setMarked((m) => { const n = new Set(m); if (n.has(id)) n.delete(id); else n.add(id); return n; });

    const find = async (appId: string, name: string) => {
        const mine = ++search.current;
        setLeft({appId, name, result: null, candidates: [], marked: new Set(), recycled: null, confirming: false, busy: true});
        try {
            const r = await api.LeftoversFind(appId);
            if (mine !== search.current) return;
            setLeft({appId, name: r.appName || name, result: r, candidates: r.candidates, marked: new Set(), recycled: null, confirming: false, busy: false});
        } catch (e) {
            if (mine !== search.current) return;
            const err = uninstallError(e);
            p.say(err.text);
            setLeft(null);
            if (err.refresh) void load();
        }
    };
    useEffect(() => { if (left) panel.current?.focus(); }, [left?.appId, left?.result != null]);

    const toggleLeft = (path: string) => setLeft((l) => {
        if (!l) return l;
        const n = new Set(l.marked);
        if (n.has(path)) n.delete(path); else n.add(path);
        return {...l, marked: n};
    });

    const recycle = async () => {
        if (!left) return;
        const paths = left.candidates.filter((c) => left.marked.has(c.path)).map((c) => c.path);
        if (paths.length === 0) return;
        setLeft({...left, busy: true});
        try {
            const r = await api.LeftoversRecycle(paths);
            const stay = new Set(r.failed.filter((f) => f.reason !== "missing").map((f) => f.path));
            setLeft((l) => (l ? {
                ...l, busy: false, recycled: r,
                candidates: l.candidates.filter((c) => !paths.includes(c.path) || stay.has(c.path)),
                marked: new Set([...l.marked].filter((x) => stay.has(x))),
            } : l));
        } catch (e) {
            const err = uninstallError(e);
            p.say(err.text);
            setLeft((l) => (l ? {...l, busy: false} : l));
            if (err.refresh) void load();
        }
    };

    const removed = listing?.removed ?? [];
    const hidden = useMemo(() => new Set([...apps.map((a) => a.id), ...removed.map((r) => r.id)]), [apps, removed]);
    const matches = useMemo(() => notInstalledMatches(p.apps, hidden, manual), [p.apps, hidden, manual]);
    const leftCount = left ? left.candidates.filter((c) => left.marked.has(c.path)).length : 0;
    const searching = left !== null && left.result === null;

    return (
        <div className="page uninstpage">
            <header className="pagehead">
                <div>
                    <h1>{t("uninst.title")}</h1>
                    <p className="muted">{t("uninst.intro")}</p>
                </div>
                <button disabled={listing === null} onClick={() => void load()}><Icon name="refresh" size={15}/> {t("uninst.refresh")}</button>
            </header>

            {listing === null && <p className="muted" role="status">{t("uninst.loading")}</p>}
            {listing !== null && apps.length === 0 && <p className="muted" role="status">{t("uninst.empty")}</p>}

            {apps.length > 0 && (
                <section aria-label={t("uninst.list")}>
                    <div className="toolrow">
                        <label className="check">{t("uninst.search")}
                            <input type="search" value={query} placeholder={t("uninst.search.placeholder")} onChange={(e) => setQuery(e.target.value)}/>
                        </label>
                        <span className="muted" role="status">{t("uninst.summary", {n: chosen.length, total: apps.length})}</span>
                        <button className="sm" disabled={chosen.length === 0} onClick={() => setMarked(new Set())}>{t("uninst.clear")}</button>
                    </div>
                    {!p.admin && needAdmin > 0 && (
                        <p className="muted small hint adminhint"><Icon name="lock" size={14}/> <span>{t("uninst.adminHint", {n: needAdmin})}</span>
                            <button className="mini" onClick={p.onAdmin}>{t("uninst.restartAdmin")}</button></p>
                    )}
                    {shown.length === 0 && <p className="muted" role="status">{t("uninst.noMatch")}</p>}
                    <ul className="cleanlist" aria-label={t("uninst.list")}>
                        {shown.map((a) => (
                            <li key={a.id}>
                                <label className={marked.has(a.id) ? "on" : ""}>
                                    <input type="checkbox" checked={marked.has(a.id)} onChange={() => toggle(a.id)} aria-label={t("uninst.select", {name: a.name})}/>
                                    <span className="grow">
                                        <span className="catname">
                                            <b>{a.name}</b>
                                            {a.admin && <span className="chip admin" title={t("uninst.badge.adminTip")}><Icon name="lock" size={12}/> {t("uninst.badge.admin")}</span>}
                                        </span>
                                        <small className="muted">
                                            {[a.version ? t("uninst.version", {v: a.version}) : "", a.publisher ? t("uninst.publisher", {name: a.publisher}) : ""].filter(Boolean).join(" · ")}
                                        </small>
                                    </span>
                                    {!!a.sizeBytes && <span className="cleansize muted small nowrap">{t("uninst.size", {size: formatBytes(a.sizeBytes)})}</span>}
                                </label>
                            </li>
                        ))}
                    </ul>
                    <div className="cleanbar">
                        <span className="grow" aria-hidden="true"/>
                        <button className="danger" disabled={chosen.length === 0} onClick={() => setAsking(true)}>
                            <Icon name="trash" size={15}/> {chosen.length > 0 ? t("uninst.button.n", {n: chosen.length}) : t("uninst.button")}
                        </button>
                    </div>
                </section>
            )}

            {removed.length > 0 && (
                <section aria-labelledby="uninst-removed">
                    <h2 id="uninst-removed">{t("uninst.removed.title")}</h2>
                    <p className="muted small">{t("uninst.removed.intro")}</p>
                    <ul className="plain">
                        {removed.map((r) => (
                            <li key={r.id} className="toolrow">
                                <b>{r.name}</b>
                                <button className="sm" disabled={searching || left?.busy} aria-label={t("uninst.leftovers.lookFor", {name: r.name})} onClick={() => void find(r.id, r.name)}>
                                    <Icon name="search" size={14}/> {t("uninst.leftovers.look")}
                                </button>
                            </li>
                        ))}
                    </ul>
                </section>
            )}

            <section aria-labelledby="uninst-manual">
                <h2 id="uninst-manual">{t("uninst.manual.title")}</h2>
                <p className="muted small">{t("uninst.manual.intro")}</p>
                <div className="toolrow">
                    <label className="check">{t("uninst.manual.search")}
                        <input type="search" value={manual} placeholder={t("uninst.manual.placeholder")} onChange={(e) => setManual(e.target.value)}/>
                    </label>
                </div>
                {manual.trim().length >= 2 && matches.length === 0 && <p className="muted" role="status">{t("uninst.manual.none")}</p>}
                {matches.length > 0 && (
                    <ul className="plain" aria-label={t("uninst.manual.results")}>
                        {matches.map((a) => (
                            <li key={a.id} className="toolrow">
                                <b>{a.name}</b>
                                <button className="sm" disabled={searching || left?.busy} aria-label={t("uninst.leftovers.lookFor", {name: a.name})} onClick={() => void find(a.id, a.name)}>
                                    <Icon name="search" size={14}/> {t("uninst.leftovers.look")}
                                </button>
                            </li>
                        ))}
                    </ul>
                )}
            </section>

            {left && (
                <section ref={panel} tabIndex={-1} aria-labelledby="uninst-left" className="leftpanel">
                    <h2 id="uninst-left">{t("uninst.left.title", {name: left.name})}</h2>
                    <div className="leftwarn" role="note"><Icon name="alert" size={18}/><p>{t("uninst.left.warning")}</p></div>
                    {searching && <p className="muted" role="status">{t("uninst.left.searching")}</p>}
                    {left.result && (
                        <>
                            <p className="muted small">{t("uninst.left.searched", {places: nameList(left.result.searched.map((s) => placeName(s.place)))})}</p>
                            {left.recycled && (
                                <div className="resultline" role="status">
                                    <p>{t(left.recycled.moved === 1 ? "uninst.left.result.one" : "uninst.left.result.other", {n: left.recycled.moved, size: formatBytes(left.recycled.freedBytes)})}</p>
                                    {left.recycled.failed.length > 0 && (
                                        <>
                                            <p className="muted small">{t("uninst.left.failed", {n: left.recycled.failed.length})}</p>
                                            <ul className="plain small">
                                                {left.recycled.failed.map((f) => <li key={f.path} className="muted mono">{t("uninst.left.failedLine", {path: f.path, reason: reasonText(f.reason)})}</li>)}
                                            </ul>
                                        </>
                                    )}
                                </div>
                            )}
                            {left.candidates.length === 0 && !left.recycled && <p className="muted" role="status">{t("uninst.left.none", {name: left.name})}</p>}
                            {left.candidates.length > 0 && (
                                <ul className="cleanlist leftlist" aria-label={t("uninst.left.list")}>
                                    {left.candidates.map((c) => (
                                        <li key={c.path}>
                                            <label className={left.marked.has(c.path) ? "on" : ""}>
                                                <input type="checkbox" checked={left.marked.has(c.path)} disabled={left.busy} onChange={() => toggleLeft(c.path)}
                                                       aria-label={t("uninst.left.check", {path: c.path})}/>
                                                <span className="grow">
                                                    <span className="catname">
                                                        <b>{c.path.split(/[\\/]/).pop()}</b>
                                                        <span className="chip">{t(c.kind === "shortcut" ? "uninst.left.kind.shortcut" : "uninst.left.kind.folder")}</span>
                                                        <span className="chip">{placeName(c.place)}</span>
                                                    </span>
                                                    <small className="mono muted">{c.path}</small>
                                                    <small className="muted">{whyText(c.why)}</small>
                                                    <small className="muted">
                                                        {[c.kind === "folder" ? fileCount(c.files) : "", formatDate(c.modified, getLang()) ? t("uninst.left.modified", {date: formatDate(c.modified, getLang())}) : ""].filter(Boolean).join(" · ")}
                                                    </small>
                                                </span>
                                                <span className="cleansize"><b className="mono">{formatBytes(c.sizeBytes)}</b></span>
                                            </label>
                                        </li>
                                    ))}
                                </ul>
                            )}
                        </>
                    )}
                    <div className="cleanbar">
                        <span className="grow" role="status">
                            {left.candidates.length > 0 && (leftCount > 0 ? t("uninst.left.summary", {n: leftCount, size: formatBytes(markedBytes(left.candidates, left.marked))}) : t("uninst.left.summaryNone"))}
                        </span>
                        <button disabled={left.busy} onClick={() => void find(left.appId, left.name)}><Icon name="refresh" size={15}/> {t("uninst.left.again")}</button>
                        <button disabled={left.busy} onClick={() => { search.current++; setLeft(null); }}>{t("uninst.left.close")}</button>
                        <button className="danger" disabled={left.busy || leftCount === 0} onClick={() => setLeft({...left, confirming: true})}>
                            <Icon name="trash" size={15}/> {t("uninst.left.recycle")}
                        </button>
                    </div>
                </section>
            )}

            {asking && (
                <ConfirmDialog title={t(chosen.length === 1 ? "uninst.confirmTitle.one" : "uninst.confirmTitle.other", {n: chosen.length})} danger confirm={t("uninst.confirm")}
                               body={t("uninst.confirmBody")}
                               onConfirm={() => { const list = chosen; setMarked(new Set()); p.onUninstall(list); }} onClose={() => setAsking(false)}>
                    <ul className="confirmlist">{chosen.map((a) => <li key={a.id}><b>{a.name}</b>{a.version ? ` ${a.version}` : ""}</li>)}</ul>
                    {!p.admin && needAdmin > 0 && <p className="muted small">{t("uninst.confirmAdmin", {n: needAdmin})}</p>}
                </ConfirmDialog>
            )}
            {left?.confirming && (
                <ConfirmDialog title={t(leftCount === 1 ? "uninst.left.confirmTitle.one" : "uninst.left.confirmTitle.other", {n: leftCount})} danger confirm={t("uninst.left.confirm")}
                               body={t("uninst.left.confirmBody")} onConfirm={() => void recycle()} onClose={() => setLeft((l) => (l ? {...l, confirming: false} : l))}>
                    <ul className="confirmlist">{left.candidates.filter((c) => left.marked.has(c.path)).map((c) => <li key={c.path} className="mono small">{c.path}</li>)}</ul>
                </ConfirmDialog>
            )}
        </div>
    );
}
