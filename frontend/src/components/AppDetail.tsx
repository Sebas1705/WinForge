import {useEffect} from "react";
import {AppIcon} from "./AppIcon";
import {categoryLabel, t} from "../lib/i18n";
import type {App} from "../lib/model";
import {topCategory} from "../lib/tally";

/** Side panel with everything the catalog knows about one app, and its links. */
export function AppDetail(p: {
    app: App; byId: Map<string, App>;
    onClose: () => void; onInstall: (a: App) => void; openURL: (u: string) => void; onCopy: (text: string) => void;
}) {
    const {app: a} = p;
    useEffect(() => {
        const esc = (e: KeyboardEvent) => { if (e.key === "Escape") p.onClose(); };
        window.addEventListener("keydown", esc);
        return () => window.removeEventListener("keydown", esc);
    });
    const command = `winget install --id ${a.winget} --exact`;
    const needs = (a.requires ?? []).map((id) => p.byId.get(id)?.name ?? id);

    return (
        <div className="drawer-overlay" onClick={p.onClose}>
            <aside className="drawer" role="dialog" aria-modal="true" aria-label={a.name} onClick={(e) => e.stopPropagation()}>
                <button className="icon close" aria-label={t("common.close")} onClick={p.onClose}>✕</button>
                <header>
                    <AppIcon id={a.id} name={a.name} category={a.category} size={64}/>
                    <div>
                        <h2>{a.name}</h2>
                        <span className="muted mono">{a.publisher}</span>
                    </div>
                </header>
                <p className="tags">
                    <span className="pill">{categoryLabel(topCategory(a))}</span>
                    {a.openSource && <span className="tag oss">{t("badge.oss")}</span>}
                    {a.admin && <span className="tag">{t("badge.admin")}</span>}
                </p>
                <p>{a.description}</p>

                <div className="actions">
                    <button className="primary" onClick={() => p.openURL(a.homepage)}>{t("detail.website")} ↗</button>
                    {a.installed
                        ? <span className="pill ok mono">{t("catalog.installed", {v: a.version ?? ""})}</span>
                        : <button onClick={() => p.onInstall(a)}>{t("common.install")}</button>}
                </div>

                <dl>
                    <dt>{t("detail.wingetId")}</dt><dd className="mono">{a.winget}</dd>
                    {a.license && <><dt>{t("detail.license")}</dt><dd>{a.license}</dd></>}
                    {needs.length > 0 && <><dt>{t("detail.requires")}</dt><dd>{needs.join(", ")}</dd></>}
                    {a.installed && a.sources && a.sources.length > 0 && <><dt>{t("detail.detected")}</dt><dd className="mono">{a.sources.join(", ")}</dd></>}
                    <dt>{t("detail.homepage")}</dt>
                    <dd><a href="#" className="mono" onClick={(e) => { e.preventDefault(); p.openURL(a.homepage); }}>{a.homepage}</a></dd>
                </dl>
                <button className="ghost" onClick={() => p.onCopy(command)}>{t("detail.copyCommand")}</button>
            </aside>
        </div>
    );
}
