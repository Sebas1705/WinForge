import {useEffect, useMemo, useState} from "react";
import {api, errText} from "../api";
import {AppIcon} from "../components/AppIcon";
import {Icon, type IconName} from "../components/Icon";
import {IconStack, Ring, Tip} from "../components/Visual";
import {getLang, t} from "../lib/i18n";
import {profileText} from "../lib/profileText";
import type {App, Profile, ProfileInfo, State} from "../lib/model";
import {selectionProfile} from "../lib/model";
import {profileApps, profileTally} from "../lib/tally";

export function Profiles(p: {
    state: State; focus: string | null; advanced: boolean;
    onInstall: (p: Profile) => void; onChanged: () => Promise<void>; say: (m: string) => void;
    onImport: () => void; onFromPC: () => void; onEdit: (p: ProfileInfo) => void;
    onDetail: (a: App) => void; onFocusDone: () => void;
}) {
    const [sel, setSel] = useState<string | null>(p.focus);
    const [q, setQ] = useState("");
    useEffect(() => { if (p.focus) { setSel(p.focus); p.onFocusDone(); } }, [p.focus]);
    const apps = useMemo(() => new Map(p.state.apps.map((a) => [a.id, a])), [p.state.apps]);
    const current = sel ? p.state.profiles.find((x) => x.id === sel) : undefined;
    const pt = (x: ProfileInfo) => profileText(x, getLang());

    if (current) {
        return <ProfileDetail profile={current} apps={apps} advanced={p.advanced} onBack={() => setSel(null)}
                              onInstall={p.onInstall} onChanged={async () => { await p.onChanged(); setSel(null); }} say={p.say}
                              onEdit={p.onEdit} onDetail={p.onDetail}/>;
    }

    const needle = q.trim().toLowerCase();
    const match = (x: ProfileInfo) => !needle || `${pt(x).name} ${pt(x).desc}`.toLowerCase().includes(needle);
    const groups: [string, IconName, ProfileInfo[]][] = [
        [t("profiles.group.general"), "home", p.state.profiles.filter((x) => x.builtin && x.kind !== "dev" && match(x))],
        [t("profiles.group.dev"), "code", p.state.profiles.filter((x) => x.builtin && x.kind === "dev" && match(x))],
        [t("profiles.group.mine"), "star", p.state.profiles.filter((x) => !x.builtin && match(x))],
    ];

    return (
        <div className="page">
            <header className="pagehead">
                <div>
                    <h1>{t("nav.profiles")} <Tip text={t("profiles.help")}/></h1>
                </div>
                <span className="grow"/>
                <div className="searchbox">
                    <Icon name="search" size={16}/>
                    <input className="search" placeholder={t("profiles.search")} value={q} onChange={(e) => setQ(e.target.value)}/>
                </div>
                <button onClick={p.onFromPC}><Icon name="plus" size={14}/> {t("profiles.fromPC")}</button>
                <button className="ghost" onClick={p.onImport}>{t("profiles.import")}</button>
            </header>
            {groups.every(([, , l]) => l.length === 0) && <p className="muted empty">{t("profiles.none")}</p>}
            {groups.map(([title, icon, list]) => list.length > 0 && (
                <section key={title}>
                    <h2><Icon name={icon} size={18}/> {title}</h2>
                    <div className="cardgrid profiles">
                        {list.map((x) => {
                            const tl = profileTally(x.resolved, apps);
                            const full = tl.total > 0 && tl.have === tl.total;
                            return (
                                <button key={x.id} className={"profilecard" + (full ? " full" : "")} onClick={() => setSel(x.id)}>
                                    <IconStack apps={profileApps(x.resolved, apps)} max={5} size={38}/>
                                    <b>{pt(x).name}</b>
                                    <span className="line muted">{pt(x).desc}</span>
                                    <span className="foot">
                                        <span className="mono muted small">{t("profiles.appCount", {n: tl.total})}</span>
                                        <Ring size={34} stroke={4} value={tl.total ? tl.have / tl.total : 0} tone={full ? "ok" : "accent"}>
                                            {full ? <Icon name="check" size={14}/> : <span className="mono tiny">{tl.have}</span>}
                                        </Ring>
                                    </span>
                                </button>
                            );
                        })}
                    </div>
                </section>
            ))}
        </div>
    );
}

function ProfileDetail(p: {
    profile: ProfileInfo; apps: Map<string, App>; advanced: boolean;
    onBack: () => void; onInstall: (p: Profile) => void; onChanged: () => Promise<void>; say: (m: string) => void;
    onEdit: (p: ProfileInfo) => void; onDetail: (a: App) => void;
}) {
    const {profile: current} = p;
    const text = profileText(current, getLang());
    const [skip, setSkip] = useState<Set<string>>(new Set());
    const [menu, setMenu] = useState(false);
    const tally = profileTally(current.resolved, p.apps);
    const toInstall = current.resolved.filter((id) => !skip.has(id) && !p.apps.get(id)?.installed);
    const missingAll = tally.total - tally.have;
    const full = tally.total > 0 && missingAll === 0;

    const installSelected = () => {
        // Untouched selection = the whole profile, its own setup steps included.
        if (skip.size === 0) p.onInstall(current);
        else p.onInstall(selectionProfile(toInstall, text.name));
    };
    const run = (fn: () => Promise<string | void>, ok?: string) => {
        setMenu(false);
        return fn().then((r) => { if (r) p.say(t("toast.savedTo", {path: r})); else if (ok) p.say(ok); }).catch((e) => p.say(errText(e)));
    };

    return (
        <div className="page detail-page">
            <button className="ghost back" onClick={p.onBack}><Icon name="back" size={16}/> {t("profiles.back")}</button>
            <header className="profilehead">
                <IconStack apps={profileApps(current.resolved, p.apps)} max={7} size={52}/>
                <div className="grow">
                    <h1>{text.name}</h1>
                    <p className="muted">{text.desc}</p>
                </div>
                <Ring size={84} stroke={9} value={tally.total ? tally.have / tally.total : 0} tone={full ? "ok" : "accent"}>
                    {full ? <Icon name="check" size={26}/> : <span className="ringnum">{tally.have}<small>/{tally.total}</small></span>}
                </Ring>
            </header>

            <div className="actions">
                <button className="primary big" disabled={toInstall.length === 0} onClick={installSelected}>
                    <Icon name="download" size={18}/>
                    {full ? t("profiles.complete") : skip.size === 0 ? t("profiles.installMissing") : t("profiles.installN", {n: toInstall.length})}
                </button>
                <button onClick={() => p.onEdit(current)}>{t("profiles.edit")}</button>
                <div className="menu">
                    <button className="ghost" aria-expanded={menu} onClick={() => setMenu(!menu)}>{t("profiles.more")} ▾</button>
                    {menu && (
                        <div className="menu-pop" role="menu">
                            <button role="menuitem" onClick={() => run(() => api.ExportProfile(current))}>{t("profiles.export")}</button>
                            {p.advanced && <button role="menuitem" title={t("profiles.exportWingetHint")} onClick={() => run(() => api.ExportWinget(current))}>{t("profiles.exportWinget")}</button>}
                            {p.advanced && <button role="menuitem" title={t("profiles.exportScriptHint")} disabled={missingAll === 0} onClick={() => run(() => api.ExportScript(current))}>{t("profiles.exportScript")}</button>}
                            {!current.builtin && <button role="menuitem" className="danger" onClick={() => run(async () => { await api.DeleteProfile(current.id); await p.onChanged(); }, t("toast.deleted"))}>{t("profiles.delete")}</button>}
                        </div>
                    )}
                </div>
            </div>

            <div className="tilegrid">
                {current.resolved.map((id) => {
                    const a = p.apps.get(id);
                    if (!a) return null;
                    const off = skip.has(id);
                    return (
                        <div key={id} className={"tile" + (a.installed ? " done" : "") + (off ? " off" : "")}>
                            {!a.installed && (
                                <label className="pick">
                                    <input type="checkbox" checked={!off} aria-label={t("profiles.include", {name: a.name})}
                                           onChange={() => { const n = new Set(skip); if (n.has(id)) n.delete(id); else n.add(id); setSkip(n); }}/>
                                </label>
                            )}
                            <button className="tilebody" onClick={() => p.onDetail(a)} aria-label={a.name}>
                                <AppIcon id={a.id} name={a.name} category={a.category} size={52}/>
                                <b>{a.name}</b>
                            </button>
                            {a.installed
                                ? <span className="chip done"><Icon name="check" size={13}/>{t("profiles.installedBadge")}</span>
                                : <button className="mini" onClick={() => p.onInstall(selectionProfile([id], a.name))}>{t("common.install")}</button>}
                        </div>
                    );
                })}
            </div>
            {current.resolvedRecipes.length > 0 && p.advanced && (
                <p className="muted small">{t("profiles.after", {list: current.resolvedRecipes.join(", ")})}</p>
            )}
        </div>
    );
}
