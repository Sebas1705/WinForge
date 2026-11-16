import {useCallback, useEffect, useMemo, useState} from "react";
import {api, errText, type ServiceItem, type ServiceMode} from "../../api";
import {Icon} from "../../components/Icon";
import {ConfirmDialog} from "../../components/Modals";
import {t} from "../../lib/i18n";
import {domId, svcError} from "./svcErrors";

export type ServiceFilter = "all" | "running" | "stopped" | "changed";

export const SERVICE_MODES: ServiceMode[] = ["auto", "delayed", "manual", "disabled"];

const rank = (m: ServiceMode) => (m === "auto" || m === "delayed" ? 2 : m === "manual" ? 1 : 0);

/** Whether a change makes a service start less eagerly, which is what can break the services that need it. */
export function lowersStart(from: ServiceMode, to: ServiceMode): boolean { return rank(to) < rank(from); }

export const modeLabel = (m: ServiceMode) => t(`svc.services.mode.${m}`);
const stateLabel = (s: ServiceItem["state"]) => t(`svc.services.state.${s}`);

/** Services that match the search text (names, publisher, program) and the filter. */
export function visibleServices(items: ServiceItem[], query: string, filter: ServiceFilter): ServiceItem[] {
    const q = query.trim().toLowerCase();
    return items.filter((i) => {
        if (filter === "running" && i.state !== "running") return false;
        if (filter === "stopped" && i.state !== "stopped") return false;
        if (filter === "changed" && !i.changed) return false;
        return !q || `${i.displayName} ${i.name} ${i.publisher} ${i.binary}`.toLowerCase().includes(q);
    });
}

/**
 * The third-party services, each with a select for when it starts. Nothing is stopped, started or deleted:
 * only the start mode changes, it needs administrator rights, security software and services that others
 * need ask first, and "Restore original" appears for a service WinForge changed.
 */
export function Services(p: { admin: boolean; say: (m: string) => void; onAdmin: () => void }) {
    const [items, setItems] = useState<ServiceItem[] | null>(null);
    const [busy, setBusy] = useState<string | null>(null);
    const [query, setQuery] = useState("");
    const [filter, setFilter] = useState<ServiceFilter>("all");
    const [asking, setAsking] = useState<{ item: ServiceItem; mode: ServiceMode | "restore" } | null>(null);
    const [note, setNote] = useState("");

    const load = useCallback(async () => {
        try { setItems(await api.ServicesList()); } catch (e) { p.say(errText(e)); setItems((v) => v ?? []); }
    }, [p.say]);
    useEffect(() => { void load(); }, [load]);

    const run = async (item: ServiceItem, mode: ServiceMode | "restore", confirmed: boolean) => {
        setBusy(item.id);
        try {
            const next = mode === "restore" ? await api.ServicesRestore(item.id, confirmed) : await api.ServicesSetMode(item.id, mode, confirmed);
            setItems(next);
            const now = next.find((x) => x.id === item.id);
            const shownMode = now ? modeLabel(now.startMode) : mode === "restore" ? modeLabel(item.original as ServiceMode) : modeLabel(mode);
            setNote(t(mode === "restore" ? "svc.services.restored" : "svc.services.changed", {name: item.displayName, mode: shownMode}));
        } catch (e) {
            const err = svcError(e);
            if (err.ask) setAsking({item, mode});
            else {
                p.say(err.text);
                if (err.refresh) void load();
            }
        } finally { setBusy(null); }
    };

    /** Asks first for security software and for lowering a service others need; otherwise just does it. */
    const change = (item: ServiceItem, mode: ServiceMode | "restore") => {
        const target = mode === "restore" ? item.original : mode;
        if (!target || target === item.startMode) return;
        const risky = item.caution || (item.dependents.length > 0 && lowersStart(item.startMode, target as ServiceMode));
        if (risky) setAsking({item, mode});
        else void run(item, mode, false);
    };

    const list = items ?? [];
    const shown = useMemo(() => visibleServices(list, query, filter), [list, query, filter]);
    const runningCount = list.filter((i) => i.state === "running").length;
    const locked = !p.admin ? list.length : 0;

    const askBody = (item: ServiceItem, mode: ServiceMode | "restore") => {
        const target = mode === "restore" ? (item.original as ServiceMode) : mode;
        const parts: string[] = [];
        if (item.caution) parts.push(t("svc.confirmCaution", {name: item.displayName}));
        if (item.dependents.length > 0 && target && lowersStart(item.startMode, target)) {
            parts.push(t("svc.confirmDependents", {name: item.displayName, list: item.dependents.join(", ")}));
        }
        return parts.join(" ") || t("svc.confirmCaution", {name: item.displayName});
    };

    return (
        <>
            <header className="pagehead">
                <p className="muted grow">{t("svc.services.intro")}</p>
                <button disabled={busy !== null} onClick={() => { setNote(""); void load(); }}><Icon name="refresh" size={15}/> {t("svc.refresh")}</button>
            </header>

            {items === null && <p className="muted" role="status">{t("svc.loading")}</p>}
            {items !== null && list.length === 0 && <p className="muted" role="status">{t("svc.services.empty")}</p>}

            {list.length > 0 && (
                <>
                    <div className="toolrow">
                        <label className="check">{t("svc.search")}
                            <input type="search" value={query} placeholder={t("svc.services.search.placeholder")} onChange={(e) => setQuery(e.target.value)}/>
                        </label>
                        <label className="check">{t("svc.filter")}
                            <select value={filter} onChange={(e) => setFilter(e.target.value as ServiceFilter)}>
                                <option value="all">{t("svc.filter.all")}</option>
                                <option value="running">{t("svc.services.filter.running")}</option>
                                <option value="stopped">{t("svc.services.filter.stopped")}</option>
                                <option value="changed">{t("svc.filter.changed")}</option>
                            </select>
                        </label>
                        <span className="muted" role="status">{t("svc.services.summary", {running: runningCount, total: list.length})}</span>
                    </div>
                    {locked > 0 && (
                        <p className="muted small hint adminhint"><Icon name="lock" size={14}/> <span>{t("svc.adminHint", {n: locked})}</span>
                            <button className="mini" onClick={p.onAdmin}>{t("svc.restartAdmin")}</button></p>
                    )}
                    <p className="sr-only" role="status" aria-live="polite">{note}</p>
                    {shown.length === 0 && <p className="muted" role="status">{t("svc.services.noMatch")}</p>}
                    <ul className="cleanlist startuplist" aria-label={t("svc.services.list")}>
                        {shown.map((i) => {
                            const stateId = domId("svcstate", i.id);
                            const off = busy !== null || !p.admin;
                            return (
                                <li key={i.id}>
                                    <div className="vdisk svcrow">
                                        <span className="grow">
                                            <span className="catname">
                                                <b>{i.displayName}</b>
                                                {i.displayName !== i.name && <span className="chip mono">{i.name}</span>}
                                                <span className="chip" id={stateId}>{stateLabel(i.state)}</span>
                                                {!p.admin && <span className="chip admin" title={t("svc.badge.adminTip")}><Icon name="lock" size={12}/> {t("svc.badge.admin")}</span>}
                                                {i.caution && <span className="chip review" title={t("svc.badge.cautionTip")}><Icon name="alert" size={12}/> {t("svc.badge.caution")}</span>}
                                                {i.dependents.length > 0 && <span className="chip" title={t("svc.services.dependentsTip", {list: i.dependents.join(", ")})}>{t("svc.services.dependents", {n: i.dependents.length})}</span>}
                                                {i.changed && <span className="chip done">{t("svc.badge.changed")}</span>}
                                            </span>
                                            {i.publisher && <small className="muted">{t("svc.services.publisher", {name: i.publisher})}</small>}
                                            <small className="mono muted" title={i.binary}>{i.binary}</small>
                                        </span>
                                        <select value={i.startMode} disabled={off} aria-label={t("svc.services.mode", {name: i.displayName})} aria-describedby={stateId}
                                                title={!p.admin ? t("svc.badge.adminTip") : undefined}
                                                onChange={(e) => change(i, e.target.value as ServiceMode)}>
                                            {SERVICE_MODES.map((m) => <option key={m} value={m}>{modeLabel(m)}</option>)}
                                        </select>
                                        {i.changed && i.original && (
                                            <button className="sm" disabled={off} aria-label={t("svc.restore.label", {name: i.displayName})} onClick={() => change(i, "restore")}>
                                                {t("svc.restore", {value: modeLabel(i.original)})}
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
                <ConfirmDialog title={t("svc.services.confirmTitle", {name: asking.item.displayName})} danger={asking.item.caution} confirm={t("svc.confirm")}
                               body={askBody(asking.item, asking.mode)}
                               onConfirm={() => void run(asking.item, asking.mode, true)} onClose={() => setAsking(null)}>
                    {asking.item.dependents.length > 0 && (
                        <ul className="confirmlist" aria-label={t("svc.services.dependents", {n: asking.item.dependents.length})}>
                            {asking.item.dependents.map((d) => <li key={d}>{d}</li>)}
                        </ul>
                    )}
                </ConfirmDialog>
            )}
        </>
    );
}
