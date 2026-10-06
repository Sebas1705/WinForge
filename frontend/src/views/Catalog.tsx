import {useEffect, useMemo, useRef, useState} from "react";
import {categoryLabel, t} from "../lib/i18n";
import {filterApps, type App, type Filter} from "../lib/model";
import {AppIcon} from "../components/AppIcon";
import {categoryStats, sortApps, topCategory, type SortKey} from "../lib/tally";

export function Catalog(p: {
    apps: App[]; selection: Set<string>; setSelection: (s: Set<string>) => void;
    initialCategory: string;
    onInstall: () => void; onInstallOne: (a: App) => void; onSave: () => void; openURL: (u: string) => void;
    onDetail: (a: App) => void;
}) {
    const [f, setF] = useState<Filter>({query: "", category: p.initialCategory, installed: "all", openSourceOnly: false});
    const [sort, setSort] = useState<SortKey>("name");
    const search = useRef<HTMLInputElement>(null);
    useEffect(() => { setF((x) => ({...x, category: p.initialCategory})); }, [p.initialCategory]);

    // "/" jumps to search unless the user is already typing.
    useEffect(() => {
        const key = (e: KeyboardEvent) => {
            const tag = (e.target as HTMLElement | null)?.tagName;
            if (e.key === "/" && tag !== "INPUT" && tag !== "SELECT" && tag !== "TEXTAREA") { e.preventDefault(); search.current?.focus(); }
        };
        window.addEventListener("keydown", key);
        return () => window.removeEventListener("keydown", key);
    }, []);

    const stats = useMemo(() => categoryStats(p.apps), [p.apps]);
    const list = useMemo(() => sortApps(filterApps(p.apps, f), sort), [p.apps, f, sort]);
    const byId = useMemo(() => new Map(p.apps.map((a) => [a.id, a])), [p.apps]);
    const missing = [...p.selection].filter((id) => !byId.get(id)?.installed).length;

    const toggle = (id: string) => {
        const n = new Set(p.selection);
        if (n.has(id)) n.delete(id); else n.add(id);
        p.setSelection(n);
    };
    const selectVisible = () => p.setSelection(new Set([...p.selection, ...list.filter((a) => !a.installed).map((a) => a.id)]));

    return (
        <div className="catalog">
            <div className="toolbar">
                <input ref={search} className="search" placeholder={t("catalog.search")} value={f.query}
                       onChange={(e) => setF({...f, query: e.target.value})}/>
                <select aria-label={t("catalog.state.all")} value={f.installed}
                        onChange={(e) => setF({...f, installed: e.target.value as Filter["installed"]})}>
                    <option value="all">{t("catalog.state.all")}</option>
                    <option value="installed">{t("catalog.state.installed")}</option>
                    <option value="missing">{t("catalog.state.missing")}</option>
                </select>
                <select aria-label="Sort" value={sort} onChange={(e) => setSort(e.target.value as SortKey)}>
                    <option value="name">{t("catalog.sort.name")}</option>
                    <option value="category">{t("catalog.sort.category")}</option>
                    <option value="missing">{t("catalog.sort.missing")}</option>
                </select>
                <label className="check"><input type="checkbox" checked={f.openSourceOnly}
                       onChange={(e) => setF({...f, openSourceOnly: e.target.checked})}/> {t("catalog.oss")}</label>
            </div>
            <div className="chips" role="tablist">
                <button role="tab" aria-selected={f.category === ""} className={f.category === "" ? "on" : ""} onClick={() => setF({...f, category: ""})}>
                    {t("catalog.all")} <em>{p.apps.length}</em>
                </button>
                {stats.map((s) => (
                    <button key={s.top} role="tab" aria-selected={f.category === s.top} className={f.category === s.top ? "on" : ""}
                            onClick={() => setF({...f, category: s.top})}>
                        {categoryLabel(s.top)} <em>{s.total}</em>
                    </button>
                ))}
            </div>

            <ul className="list">
                {list.map((a) => (
                    <li key={a.id} className={p.selection.has(a.id) ? "sel" : ""}>
                        <label>
                            <input type="checkbox" checked={p.selection.has(a.id)} onChange={() => toggle(a.id)} aria-label={a.name}/>
                            <AppIcon id={a.id} name={a.name} category={a.category}/>
                            <span className="grow">
                                <button type="button" className="link name" onClick={(e) => { e.preventDefault(); p.onDetail(a); }}>{a.name}</button>
                                <span className="muted small"> {categoryLabel(topCategory(a))}</span>
                                {a.admin && <span className="tag">{t("badge.admin")}</span>}
                                {a.openSource && <span className="tag oss" title={a.license}>{t("badge.oss")}</span>}
                                <span className="desc muted">{a.description}</span>
                            </span>
                        </label>
                        <div className="meta">
                            {a.installed
                                ? <span className="pill ok mono">{t("catalog.installed", {v: a.version ?? ""})}</span>
                                : <button className="mini" onClick={() => p.onInstallOne(a)}>{t("common.install")}</button>}
                            <a href="#" className="mono site" title={a.homepage} onClick={(e) => { e.preventDefault(); p.openURL(a.homepage); }}>{a.publisher} ↗</a>
                        </div>
                    </li>
                ))}
                {list.length === 0 && <li className="empty muted">{t("catalog.empty")}</li>}
            </ul>

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
