import {AppIcon} from "./AppIcon";
import {Icon} from "./Icon";
import {Tip} from "./Visual";
import {getLang, t} from "../lib/i18n";
import type {App} from "../lib/model";
import {taglineFor} from "../lib/tally";

/**
 * A store-style card: big icon, name, one plain line, and the one action that
 * matters. Everything else (ids, publisher, license) lives in the detail panel.
 */
export function AppCard(p: {
    app: App; selected?: boolean; compact?: boolean;
    onToggle?: () => void; onOpen: () => void; onInstall: () => void;
}) {
    const a = p.app;
    return (
        <article className={"appcard" + (p.selected ? " sel" : "") + (p.compact ? " compact" : "")}>
            {p.onToggle && (
                <label className="pick" title={a.name}>
                    <input type="checkbox" checked={!!p.selected} onChange={p.onToggle} aria-label={a.name}/>
                </label>
            )}
            <button className="cardbody" onClick={p.onOpen} aria-label={a.name}>
                <AppIcon id={a.id} name={a.name} category={a.category} size={p.compact ? 48 : 56}/>
                <b>{a.name}</b>
                <span className="line">{taglineFor(a, getLang())}</span>
            </button>
            <footer>
                <span className="badges">
                    {a.openSource && <span className="chip oss"><Icon name="heart" size={12}/>{t("badge.oss")}<Tip text={t("catalog.help.oss")}/></span>}
                    {a.admin && <span className="chip admin"><Icon name="lock" size={12}/><Tip text={t("catalog.help.admin")}/></span>}
                </span>
                {a.installed
                    ? <span className="chip done"><Icon name="check" size={13}/>{t("catalog.installedBadge")}</span>
                    : <button className="primary sm" onClick={p.onInstall}>{t("common.install")}</button>}
            </footer>
        </article>
    );
}
