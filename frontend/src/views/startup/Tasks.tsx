import {useCallback, useEffect, useMemo, useState} from "react";
import {api, errText, type TaskItem} from "../../api";
import {Icon} from "../../components/Icon";
import {ConfirmDialog} from "../../components/Modals";
import {t} from "../../lib/i18n";
import {domId, svcError} from "./svcErrors";

export type TaskFilter = "all" | "on" | "off" | "changed";

/** Tasks that match the search text (name, folder, program) and the filter. */
export function visibleTasks(items: TaskItem[], query: string, filter: TaskFilter): TaskItem[] {
    const q = query.trim().toLowerCase();
    return items.filter((i) => {
        if (filter === "on" && !i.enabled) return false;
        if (filter === "off" && i.enabled) return false;
        if (filter === "changed" && !i.changed) return false;
        return !q || `${i.name} ${i.folder} ${i.action}`.toLowerCase().includes(q);
    });
}

/** The Task Scheduler's result code as the person would look it up: 0 is success, anything else is hex. */
export function resultText(code: number | null): string {
    if (code === null) return t("svc.tasks.result.none");
    if (code === 0) return t("svc.tasks.result.ok");
    return t("svc.tasks.result.code", {code: `0x${(code >>> 0).toString(16).toUpperCase().padStart(8, "0")}`});
}

/**
 * The third-party scheduled tasks that start at log on or at startup, each with a switch. A task is never
 * deleted, run or edited: it is only turned on or off. The user's own tasks need no administrator rights,
 * the others do; security software asks first, and "Restore original" appears for a task WinForge changed.
 */
export function Tasks(p: { admin: boolean; say: (m: string) => void; onAdmin: () => void }) {
    const [items, setItems] = useState<TaskItem[] | null>(null);
    const [busy, setBusy] = useState<string | null>(null);
    const [query, setQuery] = useState("");
    const [filter, setFilter] = useState<TaskFilter>("all");
    const [asking, setAsking] = useState<{ item: TaskItem; enabled: boolean | "restore" } | null>(null);
    const [note, setNote] = useState("");

    const load = useCallback(async () => {
        try { setItems(await api.TasksList()); } catch (e) { p.say(errText(e)); setItems((v) => v ?? []); }
    }, [p.say]);
    useEffect(() => { void load(); }, [load]);

    const run = async (item: TaskItem, enabled: boolean | "restore", confirmed: boolean) => {
        setBusy(item.id);
        try {
            const next = enabled === "restore" ? await api.TasksRestore(item.id, confirmed) : await api.TasksSetEnabled(item.id, enabled, confirmed);
            setItems(next);
            if (enabled === "restore") {
                setNote(t("svc.tasks.restored", {name: item.name, value: t(`svc.tasks.restore.value.${item.original || "enabled"}`)}));
            } else setNote(t(enabled ? "svc.tasks.changed.on" : "svc.tasks.changed.off", {name: item.name}));
        } catch (e) {
            const err = svcError(e);
            if (err.ask) setAsking({item, enabled});
            else {
                p.say(err.text);
                if (err.refresh) void load();
            }
        } finally { setBusy(null); }
    };

    const flip = (item: TaskItem) => {
        if (item.enabled && item.caution) setAsking({item, enabled: false});
        else void run(item, !item.enabled, false);
    };

    const restore = (item: TaskItem) => {
        // Turning a security task back off (its original was off) asks first, like switching it off does.
        if (item.caution && item.original === "disabled") setAsking({item, enabled: "restore"});
        else void run(item, "restore", false);
    };

    const list = items ?? [];
    const shown = useMemo(() => visibleTasks(list, query, filter), [list, query, filter]);
    const onCount = list.filter((i) => i.enabled).length;
    const locked = p.admin ? 0 : list.filter((i) => i.needsAdmin).length;

    return (
        <>
            <header className="pagehead">
                <p className="muted grow">{t("svc.tasks.intro")}</p>
                <button disabled={busy !== null} onClick={() => { setNote(""); void load(); }}><Icon name="refresh" size={15}/> {t("svc.refresh")}</button>
            </header>

            {items === null && <p className="muted" role="status">{t("svc.loading")}</p>}
            {items !== null && list.length === 0 && <p className="muted" role="status">{t("svc.tasks.empty")}</p>}

            {list.length > 0 && (
                <>
                    <div className="toolrow">
                        <label className="check">{t("svc.search")}
                            <input type="search" value={query} placeholder={t("svc.tasks.search.placeholder")} onChange={(e) => setQuery(e.target.value)}/>
                        </label>
                        <label className="check">{t("svc.filter")}
                            <select value={filter} onChange={(e) => setFilter(e.target.value as TaskFilter)}>
                                <option value="all">{t("svc.filter.all")}</option>
                                <option value="on">{t("svc.tasks.filter.on")}</option>
                                <option value="off">{t("svc.tasks.filter.off")}</option>
                                <option value="changed">{t("svc.filter.changed")}</option>
                            </select>
                        </label>
                        <span className="muted" role="status">{t("svc.tasks.summary", {on: onCount, total: list.length})}</span>
                    </div>
                    {locked > 0 && (
                        <p className="muted small hint adminhint"><Icon name="lock" size={14}/> <span>{t("svc.adminHint", {n: locked})}</span>
                            <button className="mini" onClick={p.onAdmin}>{t("svc.restartAdmin")}</button></p>
                    )}
                    <p className="sr-only" role="status" aria-live="polite">{note}</p>
                    {shown.length === 0 && <p className="muted" role="status">{t("svc.tasks.noMatch")}</p>}
                    <ul className="cleanlist startuplist" aria-label={t("svc.tasks.list")}>
                        {shown.map((i) => {
                            const lockedRow = i.needsAdmin && !p.admin;
                            const off = busy !== null || lockedRow;
                            const stateId = domId("taskstate", i.id);
                            return (
                                <li key={i.id}>
                                    <div className={`vdisk svcrow${i.enabled ? " on" : ""}`}>
                                        <input type="checkbox" checked={i.enabled} disabled={off} onChange={() => flip(i)} aria-label={t("svc.tasks.switch", {name: i.name})}
                                               aria-describedby={stateId} title={lockedRow ? t("svc.badge.adminTip") : undefined}/>
                                        <span className="grow">
                                            <span className="catname">
                                                <b>{i.name}</b>
                                                {i.triggers.map((g) => <span key={g} className="chip">{t(`svc.tasks.trigger.${g}`)}</span>)}
                                                <span className="chip" id={stateId}>{t("svc.tasks.state", {state: t(`svc.tasks.state.${i.state}`)})}</span>
                                                {i.needsAdmin && <span className="chip admin" title={t("svc.badge.adminTip")}><Icon name="lock" size={12}/> {t("svc.badge.admin")}</span>}
                                                {i.caution && <span className="chip review" title={t("svc.badge.cautionTip")}><Icon name="alert" size={12}/> {t("svc.badge.caution")}</span>}
                                                {i.changed && <span className="chip done">{t("svc.badge.changed")}</span>}
                                            </span>
                                            <small className="muted">{t("svc.tasks.folder", {folder: i.folder})} · {resultText(i.lastResult)}</small>
                                            <small className="mono muted" title={i.action}>{i.action}</small>
                                        </span>
                                        {i.changed && i.original && (
                                            <button className="sm" disabled={off} aria-label={t("svc.restore.label", {name: i.name, value: t(`svc.tasks.restore.value.${i.original}`)})} onClick={() => restore(i)}>
                                                {t("svc.restore", {value: t(`svc.tasks.restore.value.${i.original}`)})}
                                            </button>
                                        )}
                                    </div>
                                </li>
                            );
                        })}
                    </ul>
                </>
            )}

            {asking && (
                <ConfirmDialog title={t("svc.tasks.confirmTitle", {name: asking.item.name})} danger confirm={t("svc.tasks.confirm")}
                               body={t("svc.tasks.confirmCaution", {name: asking.item.name})}
                               onConfirm={() => void run(asking.item, asking.enabled, true)} onClose={() => setAsking(null)}>
                    <p className="mono small muted">{asking.item.action}</p>
                </ConfirmDialog>
            )}
        </>
    );
}
