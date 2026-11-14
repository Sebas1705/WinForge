import {useCallback, useDeferredValue, useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent} from "react";
import {AppCard} from "../components/AppCard";
import {AppIcon} from "../components/AppIcon";
import {Icon, categoryIcon} from "../components/Icon";
import {categoryLabel, getLang, subLabel, t} from "../lib/i18n";
import {filterApps, subCategories, type App, type Filter} from "../lib/model";
import {categoryStats, featuredApps, sortApps, taglineFor, topCategory, type SortKey} from "../lib/tally";

export const POPULAR = "__popular";
type View = "grid" | "list";

function loadView(): View {
    try { return localStorage.getItem("winforge.catalogView") === "list" ? "list" : "grid"; } catch { return "grid"; }
}

export function Catalog(p: {
    apps: App[]; featured: string[]; selection: Set<string>; setSelection: (s: Set<string>) => void;
    initialCategory: string; advanced: boolean;
    onInstall: () => void; onInstallOne: (a: App) => void; onSave: () => void; openURL: (u: string) => void;
    onDetail: (a: App) => void;
    /** Whether the single-key "/" shortcut is on (a setting). Ctrl+K is always on. */
    slashSearch?: boolean;
}) {
    const [f, setF] = useState<Filter>({query: "", category: p.initialCategory, installed: "all", openSourceOnly: false, sub: ""});
    const [sort, setSort] = useState<SortKey>("name");
    const [view, setView] = useState<View>(loadView);
    const search = useRef<HTMLInputElement>(null);
    useEffect(() => { setF((x) => ({...x, category: p.initialCategory, sub: ""})); }, [p.initialCategory]);
    useEffect(() => { try { localStorage.setItem("winforge.catalogView", view); } catch { /* not persisted */ } }, [view]);

    // "/" jumps to search unless the user is typing, a dialog is open (focus must stay inside it), or a
    // modifier is held. AltGr arrives as Ctrl+Alt on some layouts, where "/" is a normal character.
    const slash = p.slashSearch ?? true;
    useEffect(() => {
        if (!slash) return;
        const key = (e: KeyboardEvent) => {
            if (e.key !== "/" || e.defaultPrevented || e.isComposing || e.metaKey || (e.ctrlKey && !e.altKey)) return;
            const el = e.target as HTMLElement | null;
            const typing = el && (el.tagName === "INPUT" || el.tagName === "SELECT" || el.tagName === "TEXTAREA" || el.isContentEditable);
            if (typing || document.querySelector(".overlay, .drawer-overlay")) return;
            e.preventDefault();
            search.current?.focus();
        };
        window.addEventListener("keydown", key);
        return () => window.removeEventListener("keydown", key);
    }, [slash]);

    // Typing stays instant; the 876-card list catches up a moment later.
    const query = useDeferredValue(f.query);
    const popular = f.category === POPULAR;
    const source = useMemo(() => (popular ? featuredApps(p.apps, p.featured) : p.apps), [popular, p.apps, p.featured]);
    const stats = useMemo(() => categoryStats(p.apps), [p.apps]);
    const list = useMemo(() => {
        const filtered = filterApps(source, {...f, query, category: popular ? "" : f.category});
        // The popular shelf keeps its curated order unless the person picks a sort.
        return popular && sort === "name" ? filtered : sortApps(filtered, sort);
    }, [source, f.category, f.sub, f.installed, f.openSourceOnly, query, sort, popular]);
    // Big categories (Gaming, Development) split into sub-categories; small ones show none.
    const subs = useMemo(() => (f.category && !popular ? subCategories(p.apps, f.category) : []), [p.apps, f.category, popular]);
    const byId = useMemo(() => new Map(p.apps.map((a) => [a.id, a])), [p.apps]);
    const missing = [...p.selection].filter((id) => !byId.get(id)?.installed).length;

    // A stable handler lets unchanged cards skip re-rendering.
    const selection = useRef(p.selection);
    selection.current = p.selection;
    const setSelection = p.setSelection;
    const toggle = useCallback((id: string) => {
        const n = new Set(selection.current);
        if (n.has(id)) n.delete(id); else n.add(id);
        setSelection(n);
    }, [setSelection]);
    const selectVisible = () => p.setSelection(new Set([...p.selection, ...list.filter((a) => !a.installed).map((a) => a.id)]));
    // Tabs are one Tab stop; the arrow keys move between them, as the tabs pattern says.
    const tabKeys = (e: ReactKeyboardEvent<HTMLDivElement>) => {
        if (e.key !== "ArrowRight" && e.key !== "ArrowLeft" && e.key !== "Home" && e.key !== "End") return;
        const tabs = [...e.currentTarget.querySelectorAll<HTMLElement>("[role=tab]")];
        const at = tabs.indexOf(document.activeElement as HTMLElement);
        if (at < 0) return;
        e.preventDefault();
        const next = e.key === "Home" ? 0 : e.key === "End" ? tabs.length - 1 : (at + (e.key === "ArrowRight" ? 1 : -1) + tabs.length) % tabs.length;
        tabs[next].focus();
        tabs[next].click();
    };
    const chip = (key: string, icon: Parameters<typeof Icon>[0]["name"], label: string, count: number) => (
        <button key={key} role="tab" aria-selected={f.category === key} tabIndex={f.category === key ? 0 : -1} className={f.category === key ? "on" : ""} onClick={() => setF({...f, category: key, sub: ""})}>
            <Icon name={icon} size={15}/>{label} <em>{count}</em>
        </button>
    );

    return (
        <div className="catalog">
            <h1 className="sr-only">{t("nav.catalog")}</h1>
            <div className="toolbar">
                <div className="searchbox">
                    <Icon name="search" size={16}/>
                    <input ref={search} className="search" aria-label={t("catalog.searchLabel")} aria-keyshortcuts={slash ? "/ Control+K" : "Control+K"} placeholder={t(slash ? "catalog.search" : "catalog.searchNoSlash")} value={f.query}
                           onChange={(e) => setF({...f, query: e.target.value})}/>
                </div>
                <select aria-label={t("catalog.state.all")} value={f.installed}
                        onChange={(e) => setF({...f, installed: e.target.value as Filter["installed"]})}>
                    <option value="all">{t("catalog.state.all")}</option>
                    <option value="installed">{t("catalog.state.installed")}</option>
                    <option value="missing">{t("catalog.state.missing")}</option>
                </select>
                <select aria-label={t("common.sort")} value={sort} onChange={(e) => setSort(e.target.value as SortKey)}>
                    <option value="name">{t("catalog.sort.name")}</option>
                    <option value="category">{t("catalog.sort.category")}</option>
                    <option value="missing">{t("catalog.sort.missing")}</option>
                </select>
                <label className="check"><input type="checkbox" checked={f.openSourceOnly}
                       onChange={(e) => setF({...f, openSourceOnly: e.target.checked})}/> {t("catalog.oss")}</label>
                <div className="seg" role="group">
                    <button className={view === "grid" ? "on" : ""} aria-pressed={view === "grid"} title={t("catalog.view.grid")} aria-label={t("catalog.view.grid")} onClick={() => setView("grid")}><Icon name="grid" size={15}/></button>
                    <button className={view === "list" ? "on" : ""} aria-pressed={view === "list"} title={t("catalog.view.list")} aria-label={t("catalog.view.list")} onClick={() => setView("list")}><Icon name="list" size={15}/></button>
                </div>
            </div>
            <div className="chips" role="tablist" aria-label={t("nav.catalog")} onKeyDown={tabKeys}>
                {chip("", "layers", t("catalog.all"), p.apps.length)}
                {p.featured.length > 0 && chip(POPULAR, "star", t("catalog.popular"), p.featured.length)}
                {stats.map((s) => chip(s.top, categoryIcon(s.top), categoryLabel(s.top), s.total))}
            </div>

            {subs.length > 1 && (
                <div className="chips subchips" role="tablist" aria-label={categoryLabel(f.category)} onKeyDown={tabKeys}>
                    <button role="tab" aria-selected={!f.sub} tabIndex={!f.sub ? 0 : -1} className={!f.sub ? "on" : ""} onClick={() => setF({...f, sub: ""})}>{t("catalog.all")}</button>
                    {subs.map((s) => (
                        <button key={s.sub} role="tab" aria-selected={f.sub === s.sub} tabIndex={f.sub === s.sub ? 0 : -1} className={f.sub === s.sub ? "on" : ""} onClick={() => setF({...f, sub: s.sub})}>
                            {subLabel(f.category, s.sub)} <em>{s.count}</em>
                        </button>
                    ))}
                </div>
            )}

            {view === "grid" ? (
                <div className="cardgrid">
                    {list.map((a) => (
                        <AppCard key={a.id} app={a} selected={p.selection.has(a.id)} onToggle={toggle} lang={getLang()}
                                 onOpen={p.onDetail} onInstall={p.onInstallOne}/>
                    ))}
                    {list.length === 0 && <p className="empty muted">{t("catalog.empty")}</p>}
                </div>
            ) : (
                <ul className="list">
                    {list.map((a) => (
                        <li key={a.id} className={p.selection.has(a.id) ? "sel" : ""}>
                            <label>
                                <input type="checkbox" checked={p.selection.has(a.id)} onChange={() => toggle(a.id)} aria-label={a.name}/>
                                <AppIcon id={a.id} name={a.name} category={a.category}/>
                                <span className="grow">
                                    <button type="button" className="link name" onClick={(e) => { e.preventDefault(); p.onDetail(a); }}>{a.name}</button>
                                    <span className="muted small"> {categoryLabel(topCategory(a))}</span>
                                    {a.openSource && <span className="tag oss">{t("badge.oss")}</span>}
                                    <span className="desc muted">{taglineFor(a, getLang())}</span>
                                </span>
                            </label>
                            <div className="meta">
                                {a.installed
                                    ? <span className="chip done"><Icon name="check" size={13}/>{t("catalog.installedBadge")}</span>
                                    : <button className="mini" onClick={() => p.onInstallOne(a)}>{t("common.install")}</button>}
                                {p.advanced && <button type="button" className="linkbtn mono site" title={a.homepage} onClick={() => p.openURL(a.homepage)}>{a.publisher} ↗</button>}
                            </div>
                        </li>
                    ))}
                    {list.length === 0 && <li className="empty muted">{t("catalog.empty")}</li>}
                </ul>
            )}

            <footer>
                <span className="mono muted small">{t("catalog.shown", {shown: list.length, total: p.apps.length})} · {t("catalog.selected", {n: p.selection.size, m: missing})}</span>
                <span className="grow"/>
                <button className="ghost" onClick={selectVisible}>{t("catalog.selectVisible")}</button>
                <button disabled={!p.selection.size} onClick={() => p.setSelection(new Set())}>{t("common.clear")}</button>
                <button disabled={!p.selection.size} onClick={p.onSave}>{t("catalog.saveProfile")}</button>
                <button className="primary" disabled={!missing} onClick={p.onInstall}>{t("catalog.installN", {n: missing || ""})}</button>
            </footer>
        </div>
    );
}
