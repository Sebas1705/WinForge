import {useEffect, useRef, useState} from "react";
import type {HistoryEntry} from "../api";
import {AppIcon} from "../components/AppIcon";
import {History} from "./History";
import {Icon} from "../components/Icon";
import {useDismiss} from "../components/useModal";
import {getLang, t} from "../lib/i18n";
import type {App, UpgradeInfo} from "../lib/model";
import {PAUSE_DAYS, splitUpgrades} from "../lib/updates";

type Choice = { mode: "always" | "version" | "until"; days?: number };

/** Why an update is hidden, in words. */
function reason(u: UpgradeInfo): string {
    if (u.ignore === "version") return t("updates.ignoredVersion", {v: u.ignoreVersion ?? u.available});
    if (u.ignore === "until") {
        const d = u.ignoreUntil ? new Date(u.ignoreUntil) : null;
        return t("updates.ignoredUntil", {date: d && !Number.isNaN(d.getTime()) ? d.toLocaleDateString(getLang()) : "?"});
    }
    return t("updates.ignoredAlways");
}

/** The "Ignore" choices of one row: skip this version, pause for a while, or never. */
function IgnoreMenu(p: { u: UpgradeInfo; onChoose: (c: Choice) => void }) {
    const [open, setOpen] = useState(false);
    const ref = useRef<HTMLDivElement>(null);
    const trigger = useRef<HTMLButtonElement>(null);
    useDismiss(ref, open, () => setOpen(false));
    useEffect(() => { if (open) ref.current?.querySelector<HTMLElement>("[role=menuitem]")?.focus(); }, [open]);
    const close = (focusTrigger: boolean) => { setOpen(false); if (focusTrigger) trigger.current?.focus(); };
    const pick = (c: Choice) => { close(true); p.onChoose(c); };
    const onKeys = (e: React.KeyboardEvent<HTMLDivElement>) => {
        const items = [...e.currentTarget.querySelectorAll<HTMLElement>("[role=menuitem]")];
        const at = items.indexOf(document.activeElement as HTMLElement);
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault();
            items[(at + (e.key === "ArrowDown" ? 1 : -1) + items.length) % items.length]?.focus();
        } else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); close(true); }
        else if (e.key === "Tab") close(false);
    };
    return (
        <div className="menu" ref={ref}>
            <button ref={trigger} type="button" className="ghost sm" aria-haspopup="menu" aria-expanded={open}
                    aria-label={t("updates.ignoreFor", {name: p.u.name})} onClick={() => setOpen(!open)}>
                {t("updates.ignore")} ▾
            </button>
            {open && (
                <div className="menu-pop end" role="menu" onKeyDown={onKeys}>
                    <button role="menuitem" type="button" onClick={() => pick({mode: "version"})}>{t("updates.ignoreSkip", {v: p.u.available})}</button>
                    {PAUSE_DAYS.map((d) => (
                        <button key={d} role="menuitem" type="button" onClick={() => pick({mode: "until", days: d})}>{t("updates.ignorePause", {n: d})}</button>
                    ))}
                    <button role="menuitem" type="button" onClick={() => pick({mode: "always"})}>{t("updates.ignoreAlways")}</button>
                </div>
            )}
        </div>
    );
}

export function Updates(p: {
    upgrades: UpgradeInfo[] | null; busy: boolean;
    onCheck: () => void; onUpdate: (ids: string[]) => void;
    onIgnore: (id: string, c: Choice) => void; onStopIgnoring: (id: string) => void;
    byId: Map<string, App>; onDetail: (a: App) => void;
    historyKey: number; say: (m: string) => void; onRollback: (e: HistoryEntry, skipNewer: boolean) => void;
}) {
    const [picked, setPicked] = useState<Set<string>>(new Set());
    const [showIgnored, setShowIgnored] = useState(false);
    const {active: list, ignored} = splitUpgrades(p.upgrades ?? []);
    // Everything is ticked by default: the usual answer is "update it all".
    useEffect(() => { setPicked(new Set(splitUpgrades(p.upgrades ?? []).active.map((u) => u.id))); }, [p.upgrades]);
    const toggle = (id: string) => {
        const n = new Set(picked);
        if (n.has(id)) n.delete(id); else n.add(id);
        setPicked(n);
    };

    return (
        <div className="page">
            <header className="pagehead">
                <div>
                    <h1>{t("updates.title")}</h1>
                    <p className="muted">{t("updates.hint")}</p>
                </div>
                <span className="grow"/>
                <button disabled={p.busy} onClick={p.onCheck}>{p.busy ? t("updates.checking") : t("updates.check")}</button>
                <button className="primary" disabled={picked.size === 0}
                        onClick={() => p.onUpdate(picked.size === list.length ? [] : [...picked])}>
                    {picked.size === list.length && list.length > 0 ? t("updates.all") : t("updates.updateN", {n: picked.size})}
                </button>
            </header>

            {p.upgrades === null && !p.busy && <p className="muted empty">{t("updates.notChecked")}</p>}
            {p.upgrades !== null && list.length === 0 && <p className="muted empty">{t("updates.none")}</p>}
            <ul className="list">
                {list.map((u) => (
                    <li key={u.id} className={picked.has(u.id) ? "sel" : ""}>
                        <label>
                            <input type="checkbox" checked={picked.has(u.id)} onChange={() => toggle(u.id)} aria-label={t("updates.pick", {name: u.name})}/>
                            <AppIcon id={u.id} name={u.name} category={p.byId.get(u.id)?.category ?? ""}/>
                            <span className="grow">
                                <button type="button" className="link name" onClick={(e) => { e.preventDefault(); const a = p.byId.get(u.id); if (a) p.onDetail(a); }}>{u.name}</button>
                                <span className="muted small"> {u.publisher}</span>
                            </span>
                        </label>
                        <span className="mono muted">{t("updates.from", {from: u.current, to: u.available})}</span>
                        <IgnoreMenu u={u} onChoose={(c) => p.onIgnore(u.id, c)}/>
                    </li>
                ))}
            </ul>

            {ignored.length > 0 && (
                <section className="ignored" aria-labelledby="ignored-title">
                    <h2 id="ignored-title">
                        <button type="button" className="link toggle" aria-expanded={showIgnored} aria-controls="ignored-list" onClick={() => setShowIgnored(!showIgnored)}>
                            <Icon name="chevron" size={14} className={showIgnored ? "turn" : ""}/> {t("updates.ignored", {n: ignored.length})}
                        </button>
                    </h2>
                    {showIgnored && (
                        <ul className="list" id="ignored-list">
                            {ignored.map((u) => (
                                <li key={u.id}>
                                    <AppIcon id={u.id} name={u.name} category={p.byId.get(u.id)?.category ?? ""}/>
                                    <span className="grow">
                                        <b>{u.name}</b>
                                        <span className="muted small"> {reason(u)}</span>
                                    </span>
                                    <span className="mono muted">{t("updates.from", {from: u.current, to: u.available})}</span>
                                    <button type="button" className="sm" aria-label={t("updates.stopIgnoringFor", {name: u.name})} onClick={() => p.onStopIgnoring(u.id)}>{t("updates.stopIgnoring")}</button>
                                </li>
                            ))}
                        </ul>
                    )}
                </section>
            )}

            <History byId={p.byId} refreshKey={p.historyKey} say={p.say} onRollback={p.onRollback}/>
        </div>
    );
}
