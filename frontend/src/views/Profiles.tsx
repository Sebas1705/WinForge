import {useEffect, useMemo, useState} from "react";
import {api, errText} from "../api";
import {AppIcon} from "../components/AppIcon";
import {Strip} from "../components/Strip";
import {t} from "../lib/i18n";
import type {App, Profile, ProfileInfo, State} from "../lib/model";
import {selectionProfile} from "../lib/model";
import {profileTally} from "../lib/tally";

export function Profiles(p: {
    state: State; focus: string | null;
    onInstall: (p: Profile) => void; onChanged: () => Promise<void>; say: (m: string) => void;
    onImport: () => void; onFromPC: () => void; onEdit: (p: ProfileInfo) => void;
    onDetail: (a: App) => void;
}) {
    const [sel, setSel] = useState<string>(p.focus ?? p.state.profiles[0]?.id ?? "");
    const [skip, setSkip] = useState<Set<string>>(new Set());
    const [q, setQ] = useState("");
    useEffect(() => { if (p.focus) { setSel(p.focus); setSkip(new Set()); } }, [p.focus]);

    const apps = useMemo(() => new Map(p.state.apps.map((a) => [a.id, a])), [p.state.apps]);
    const current = p.state.profiles.find((x) => x.id === sel) ?? p.state.profiles[0];
    const needle = q.trim().toLowerCase();
    const match = (x: ProfileInfo) => !needle || `${x.name} ${x.description ?? ""}`.toLowerCase().includes(needle);
    const groups: [string, ProfileInfo[]][] = [
        [t("profiles.dev"), p.state.profiles.filter((x) => x.builtin && x.kind === "dev" && match(x))],
        [t("profiles.general"), p.state.profiles.filter((x) => x.builtin && x.kind !== "dev" && match(x))],
        [t("profiles.mine"), p.state.profiles.filter((x) => !x.builtin && match(x))],
    ];
    const choose = (id: string) => { setSel(id); setSkip(new Set()); };

    const tally = current ? profileTally(current.resolved, apps) : null;
    const toInstall = current ? current.resolved.filter((id) => !skip.has(id) && !apps.get(id)?.installed) : [];
    const missingAll = tally ? tally.total - tally.have : 0;
    const installSelected = () => {
        if (!current) return;
        // Untouched selection = the whole profile, its own setup steps included.
        if (skip.size === 0) p.onInstall(current);
        else p.onInstall(selectionProfile(toInstall, current.name));
    };
    const run = (fn: () => Promise<string | void>, ok?: string) => fn().then((r) => {
        if (r) p.say(t("toast.savedTo", {path: r})); else if (ok) p.say(ok);
    }).catch((e) => p.say(errText(e)));

    return (
        <div className="split">
            <aside>
                <input className="search" placeholder={t("profiles.search")} value={q} onChange={(e) => setQ(e.target.value)}/>
                {groups.every(([, l]) => l.length === 0) && <p className="muted pad">{t("profiles.none")}</p>}
                {groups.map(([title, list]) => list.length > 0 && (
                    <section key={title}>
                        <h4>{title}</h4>
                        {list.map((x) => {
                            const tl = profileTally(x.resolved, apps);
                            return (
                                <button key={x.id} className={"row" + (x.id === current?.id ? " on" : "")} onClick={() => choose(x.id)}>
                                    <span className="name">{x.name}</span>
                                    <em className="mono">{tl.have}/{tl.total}</em>
                                    <Strip size="sm" cells={tl.cells.map((c) => ({on: c.on}))} label={`${tl.have}/${tl.total}`}/>
                                </button>
                            );
                        })}
                    </section>
                ))}
                <div className="stack">
                    <button onClick={p.onFromPC}>{t("profiles.fromPC")}</button>
                    <button onClick={p.onImport}>{t("profiles.import")}</button>
                </div>
            </aside>

            {current && tally && (
                <section className="detail">
                    <h2>{current.name}</h2>
                    <p className="muted">{current.description}</p>
                    <Strip size="lg" cells={tally.cells.map((c) => ({on: c.on, name: c.name}))} label={`${tally.have}/${tally.total}`}/>
                    <p className="mono muted small">{t("profiles.progress", {have: tally.have, total: tally.total})}</p>

                    <div className="actions">
                        <button className="primary" disabled={toInstall.length === 0} onClick={installSelected}>
                            {skip.size === 0 ? t("profiles.installMissing") : t("profiles.installN", {n: toInstall.length})}
                        </button>
                        <button onClick={() => run(() => api.ExportProfile(current))}>{t("profiles.export")}</button>
                        <button title={t("profiles.exportWingetHint")} onClick={() => run(() => api.ExportWinget(current))}>{t("profiles.exportWinget")}</button>
                        <button title={t("profiles.exportScriptHint")} disabled={missingAll === 0} onClick={() => run(() => api.ExportScript(current))}>{t("profiles.exportScript")}</button>
                        <button onClick={() => p.onEdit(current)}>{t("profiles.edit")}</button>
                        {!current.builtin && (
                            <button className="danger" onClick={() => run(async () => { await api.DeleteProfile(current.id); await p.onChanged(); }, t("toast.deleted"))}>{t("profiles.delete")}</button>
                        )}
                    </div>

                    <ul className="apps">
                        {current.resolved.map((id) => {
                            const a = apps.get(id);
                            if (!a) return null;
                            return (
                                <li key={id} className={skip.has(id) ? "off" : ""}>
                                    {!a.installed ? (
                                        <input type="checkbox" checked={!skip.has(id)} aria-label={t("profiles.include", {name: a.name})}
                                               onChange={() => { const n = new Set(skip); if (n.has(id)) n.delete(id); else n.add(id); setSkip(n); }}/>
                                    ) : <span className="dot ok" title={`${a.version ?? ""}`}/>}
                                    <AppIcon id={a.id} name={a.name} category={a.category} size={28}/>
                                    <span className="grow">
                                        <button className="link name" onClick={() => p.onDetail(a)}>{a.name}</button> <span className="muted">{a.publisher}</span>
                                        {a.admin && <span className="tag">{t("badge.admin")}</span>}
                                        {a.installed && a.version && <span className="mono muted"> {a.version}</span>}
                                    </span>
                                    {!a.installed && <button className="mini" onClick={() => p.onInstall(selectionProfile([id], a.name))}>{t("common.install")}</button>}
                                </li>
                            );
                        })}
                    </ul>
                    {current.resolvedRecipes.length > 0 && (
                        <p className="muted small">{t("profiles.after", {list: current.resolvedRecipes.join(", ")})}</p>
                    )}
                </section>
            )}
        </div>
    );
}
