import {useCallback, useEffect, useMemo, useState} from "react";
import {api, errText, type StartupItem} from "../../api";
import {Icon} from "../../components/Icon";
import {ConfirmDialog} from "../../components/Modals";
import {t, tOpt} from "../../lib/i18n";
import {splitPath} from "../../lib/storage";
import {useStartupJob} from "./useStartupJob";

export type Filter = "all" | "on" | "off";

/** The Go side's error texts, as sentences. Anything else is shown as it came. */
export function startupError(e: unknown): { text: string; refresh: boolean } {
    const m = errText(e);
    if (m.includes("needs administrator")) return {text: t("startup.error.needsAdmin"), refresh: false};
    if (m.includes("no longer exists") || m.includes("not in the last list")) return {text: t("startup.error.gone"), refresh: true};
    return {text: m, refresh: false};
}

/** Items that match the search text (name, publisher or command) and the on/off filter. */
export function visibleItems(items: StartupItem[], query: string, filter: Filter): StartupItem[] {
    const q = query.trim().toLowerCase();
    return items.filter((i) => {
        if (filter === "on" && !i.enabled) return false;
        if (filter === "off" && i.enabled) return false;
        return !q || `${i.name} ${i.publisher} ${i.command}`.toLowerCase().includes(q);
    });
}

/**
 * What starts with Windows from the Run keys and the Startup folders, with a switch for each item. Switching
 * off never deletes anything: the Run value or the file stays, and the same switch turns it back on. Items for
 * all users need administrator rights, and anything that looks like Windows or security software asks first.
 */
export function Programs(p: { admin: boolean; say: (m: string) => void; onAdmin: () => void }) {
    const [items, setItems] = useState<StartupItem[] | null>(null);
    const [query, setQuery] = useState("");
    const [filter, setFilter] = useState<Filter>("all");
    const [asking, setAsking] = useState<StartupItem | null>(null);
    const [note, setNote] = useState("");

    const load = useCallback(async () => {
        try { setItems(await api.StartupList()); } catch (e) { p.say(errText(e)); setItems((v) => v ?? []); }
    }, [p.say]);
    useEffect(() => { void load(); }, [load]);
    // One change at a time, from the task store: it still holds after leaving the page and coming back.
    const {busy, job} = useStartupJob("programs", p.say, () => void load());

    const set = async (item: StartupItem, enabled: boolean) => {
        const done = t(enabled ? "startup.changed.on" : "startup.changed.off", {name: item.name});
        const r = await job(t(enabled ? "tasks.startup.on" : "tasks.startup.off", {name: item.name}), () => api.StartupSet(item.id, enabled), () => done, (e) => startupError(e).text);
        if (!r) return;
        if (r.ok) { setItems(r.value); setNote(done); return; }
        const err = startupError(r.error);
        p.say(err.text);
        if (err.refresh) void load();
    };

    const flip = (item: StartupItem) => {
        if (item.enabled && item.caution) setAsking(item);
        else void set(item, !item.enabled);
    };

    const reveal = (item: StartupItem) => { api.StartupReveal(item.id).catch((e) => p.say(errText(e))); };

    const list = items ?? [];
    const shown = useMemo(() => visibleItems(list, query, filter), [list, query, filter]);
    const enabledCount = list.filter((i) => i.enabled).length;
    const adminOnly = list.filter((i) => i.needsAdmin).length;

    return (
        <>
            <header className="pagehead">
                <p className="muted grow">{t("startup.intro")}</p>
                <button disabled={busy} onClick={() => { setNote(""); void load(); }}><Icon name="refresh" size={15}/> {t("startup.refresh")}</button>
            </header>

            {items === null && <p className="muted" role="status">{t("startup.loading")}</p>}
            {items !== null && list.length === 0 && <p className="muted" role="status">{t("startup.empty")}</p>}

            {list.length > 0 && (
                <>
                    <div className="toolrow">
                        <label className="check">{t("startup.search")}
                            <input type="search" value={query} placeholder={t("startup.search.placeholder")} onChange={(e) => setQuery(e.target.value)}/>
                        </label>
                        <label className="check">{t("startup.filter")}
                            <select value={filter} onChange={(e) => setFilter(e.target.value as Filter)}>
                                <option value="all">{t("startup.filter.all")}</option>
                                <option value="on">{t("startup.filter.on")}</option>
                                <option value="off">{t("startup.filter.off")}</option>
                            </select>
                        </label>
                        <span className="muted" role="status">{t("startup.summary", {on: enabledCount, total: list.length})}</span>
                    </div>
                    {!p.admin && adminOnly > 0 && (
                        <p className="muted small hint adminhint"><Icon name="lock" size={14}/> <span>{t("startup.adminHint", {n: adminOnly})}</span>
                            <button className="mini" onClick={p.onAdmin}>{t("startup.restartAdmin")}</button></p>
                    )}
                    <p className="sr-only" role="status" aria-live="polite">{note}</p>
                    {shown.length === 0 && <p className="muted" role="status">{t("startup.noMatch")}</p>}
                    <ul className="cleanlist startuplist" aria-label={t("startup.list")}>
                        {shown.map((i) => {
                            const locked = i.needsAdmin && !p.admin;
                            return (
                                <li key={i.id}>
                                    <label className={i.enabled ? "on" : ""} title={locked ? t("startup.badge.adminTip") : i.command}>
                                        <input type="checkbox" checked={i.enabled} disabled={busy || locked} onChange={() => flip(i)}
                                               aria-label={t("startup.switch", {name: i.name})}/>
                                        <span className="grow">
                                            <span className="catname">
                                                <b>{i.name}</b>
                                                <span className="chip">{tOpt(`startup.origin.${i.source}`) ?? i.source}</span>
                                                {i.needsAdmin && <span className="chip admin" title={t("startup.badge.adminTip")}><Icon name="lock" size={12}/> {t("startup.badge.admin")}</span>}
                                                {i.caution && <span className="chip review" title={t("startup.badge.cautionTip")}><Icon name="alert" size={12}/> {t("startup.badge.caution")}</span>}
                                                {i.missing && <span className="chip review" title={t("startup.badge.missingTip")}><Icon name="alert" size={12}/> {t("startup.badge.missing")}</span>}
                                            </span>
                                            {i.publisher && <small className="muted">{t("startup.publisher", {name: i.publisher})}</small>}
                                            <small className="mono muted" title={i.command}>{i.command}</small>
                                        </span>
                                        <span className="muted small nowrap">{i.enabled ? t("startup.on") : t("startup.off")}</span>
                                    </label>
                                    {i.target && (
                                        <button className="sm iconbtn" title={t("startup.reveal")} aria-label={`${t("startup.reveal")}: ${i.name}`} onClick={() => reveal(i)}>
                                            <Icon name="external" size={15}/>
                                        </button>
                                    )}
                                </li>
                            );
                        })}
                    </ul>
                </>
            )}

            {asking && (
                <ConfirmDialog title={t("startup.confirmTitle", {name: asking.name})} danger confirm={t("startup.confirm")}
                               body={t("startup.confirmBody", {name: asking.name})}
                               onConfirm={() => void set(asking, false)} onClose={() => setAsking(null)}>
                    <p className="mono small muted">{splitPath(asking.command).name || asking.command}</p>
                </ConfirmDialog>
            )}
        </>
    );
}
