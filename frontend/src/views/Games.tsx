import {useCallback, useEffect, useMemo, useState} from "react";
import {api, errText, on, type EmuSource, type Emulation, type GamesView, type InstallableGame} from "../api";
import {AppIcon} from "../components/AppIcon";
import {Icon} from "../components/Icon";
import {ConfirmDialog} from "../components/Modals";
import {getLang, t, type Key} from "../lib/i18n";
import type {App} from "../lib/model";

const text = (o: { en: string; es: string }) => (getLang() === "es" ? o.es : o.en);

const size = (b: number) => (b >= 1 << 30 ? `${(b / (1 << 30)).toFixed(1)} GB` : b >= 1 << 20 ? `${Math.round(b / (1 << 20))} MB` : `${Math.max(1, Math.round(b / 1024))} KB`);

function Source(p: { s: EmuSource; onOpen: (url: string) => void }) {
    const {s} = p;
    return (
        <li className="source">
            <span className={`kind ${s.kind}`}>{t(`games.kind.${s.kind}` as Key)}</span>
            <div className="grow">
                <b>{s.name}</b>
                <span className="muted small">{text(s)}</span>
                {s.base && <span className="needs small"><Icon name="lock" size={12}/> {t("games.needsBase")}</span>}
            </div>
            <button className="sm" aria-label={`${t("games.open")}: ${s.name}`} onClick={() => p.onOpen(s.url)}>
                {t("games.open")} <Icon name="external" size={13}/>
            </button>
        </li>
    );
}

/** One game that can be installed here: its state and what can be done with it. */
function GameRow(p: {
    g: InstallableGame; root: string; installed: boolean; pct: number | null; players: App[]; openFrom: string[];
    onInstall: () => void; onPlay: (emulator: App) => void; onFolder: () => void; onRemove: () => void; onOpen: (url: string) => void;
}) {
    const {g} = p;
    const folder = `${p.root}\\${g.system}\\${g.id}`;
    return (
        <li className="game">
            <div className="grow">
                <b>{g.name}</b>
                <span className="muted small">{text(g)}</span>
                <span className="muted small">
                    {t("games.license", {license: g.license})} · {size(g.size)} ·{" "}
                    <button type="button" className="linkbtn" onClick={() => p.onOpen(g.homepage)}>{t("games.homepage")}</button>
                </span>
                {p.installed && p.players.length === 0 && p.openFrom.length > 0 && (
                    <span className="small hint">{t("games.openFrom", {emulators: p.openFrom.join(", "), folder})}</span>
                )}
            </div>
            <div className="gameactions">
                {p.pct !== null ? (
                    <span className="installing" role="status">
                        <progress value={p.pct} max={100} aria-label={g.name}/> {t("games.installing", {pct: p.pct})}
                    </span>
                ) : !p.installed ? (
                    <button className="primary sm" aria-label={`${t("games.install")}: ${g.name}`} onClick={p.onInstall}>
                        <Icon name="download" size={13}/> {t("games.install")}
                    </button>
                ) : (
                    <>
                        <span className="chip done"><Icon name="check" size={13}/>{t("games.installed")}</span>
                        {p.players.map((a) => (
                            <button key={a.id} className="primary sm" aria-label={`${t("games.play", {emulator: a.name})}: ${g.name}`} onClick={() => p.onPlay(a)}>
                                <Icon name="play" size={13}/> {t("games.play", {emulator: a.name})}
                            </button>
                        ))}
                        <button className="sm" aria-label={`${t("games.openFolder")}: ${g.name}`} onClick={p.onFolder}>{t("games.openFolder")}</button>
                        <button className="sm danger" aria-label={`${t("games.remove")}: ${g.name}`} onClick={p.onRemove}>{t("games.remove")}</button>
                    </>
                )}
            </div>
        </li>
    );
}

/**
 * The emulators installed on this PC, the systems they play, the free games that
 * can be installed for them, and where to find more. Installs are checked
 * against a pinned checksum; nothing else is downloaded.
 */
export function Games(p: {
    apps: App[]; say: (m: string) => void; onOpen: (url: string) => void; onBrowse: () => void; onDetail: (a: App) => void;
}) {
    const [data, setData] = useState<Emulation | null>(null);
    const [view, setView] = useState<GamesView | null>(null);
    const [busy, setBusy] = useState(false);
    const [note, setNote] = useState<string | null>(null);
    const [pct, setPct] = useState<Record<string, number>>({});
    const [removing, setRemoving] = useState<InstallableGame | null>(null);

    const refresh = useCallback(async () => {
        try { setView(await api.GamesState()); } catch (e) { p.say(errText(e)); }
    }, [p.say]);

    useEffect(() => {
        let live = true;
        api.Emulation().then((d) => { if (live) setData(d); }).catch((e) => { if (live) { p.say(errText(e)); setData({systems: [], emulators: {}, sources: []}); } });
        void refresh();
        return () => { live = false; };
    }, [p.say, refresh]);
    useEffect(() => on.gameProgress((e) => setPct((x) => (e.id in x ? {...x, [e.id]: e.total ? Math.floor((e.done * 100) / e.total) : 0} : x))), []);

    const mine = useMemo(() => (data ? p.apps.filter((a) => a.installed && data.emulators[a.id]) : []), [p.apps, data]);
    const byId = useMemo(() => new Map(p.apps.map((a) => [a.id, a])), [p.apps]);
    const sections = useMemo(() => {
        if (!data) return [];
        return data.systems.flatMap((sys) => {
            const players = mine.filter((a) => data.emulators[a.id].includes(sys.id));
            return players.length > 0 ? [{sys, players, sources: data.sources.filter((s) => s.system === sys.id)}] : [];
        });
    }, [data, mine]);
    const general = data?.sources.filter((s) => s.system === "any") ?? [];

    // Games for systems none of my emulators plays: they wait for one.
    const waiting = useMemo(() => {
        if (!data || !view) return null;
        const playable = new Set(mine.flatMap((a) => data.emulators[a.id]));
        const list = view.games.filter((g) => !playable.has(g.system));
        if (list.length === 0) return null;
        const names = [...new Set(list.map((g) => data.systems.find((s) => s.id === g.system)).filter((s) => !!s).map((s) => text(s!)))];
        return {n: list.length, systems: names.join(", ")};
    }, [data, view, mine]);

    const startable = (g: InstallableGame) =>
        !g.entry || !data || !view ? [] : mine.filter((a) => data.emulators[a.id].includes(g.system) && (data.run?.[a.id]?.args?.length ?? 0) > 0 && !!view.launchers[a.id]);

    const install = async (g: InstallableGame) => {
        setPct((x) => ({...x, [g.id]: 0}));
        try {
            const r = await api.InstallGame(g.id);
            const folder = `${view?.root ?? ""}\\${g.system}\\${g.id}`;
            const names = (ids: string[]) => ids.map((id) => byId.get(id)?.name ?? id).join(", ");
            let m = t("games.done", {name: g.name, folder});
            if (r.registered.length > 0) m += t("games.doneRegistered", {emulators: names(r.registered)});
            if (r.registerError) m += t("games.registerFailed", {emulators: names([r.registerError.split(":")[0]]), reason: r.registerError.split(":").slice(1).join(":").trim()});
            p.say(m);
            await refresh();
        } catch (e) { p.say(errText(e)); } finally {
            setPct((x) => { const n = {...x}; delete n[g.id]; return n; });
        }
    };

    const play = (g: InstallableGame, a: App) => { api.PlayGame(g.id, a.id).catch((e) => p.say(errText(e))); };

    const changeRoot = async () => {
        try { await api.SetGamesRoot(); await refresh(); } catch (e) { p.say(errText(e)); }
    };

    const patch = async () => {
        setBusy(true);
        setNote(null);
        try {
            const r = await api.PatchROM();
            if (r.path) {
                const how = r.headerless ? ` ${t("games.patch.headerless")}` : "";
                setNote(`${t("games.patch.done", {path: r.path})} ${r.checked ? t("games.patch.checked") : t("games.patch.unchecked")}${how}`);
            }
        } catch (e) { p.say(errText(e)); } finally { setBusy(false); }
    };

    return (
        <div className="page gamespage">
            <header className="pagehead">
                <div>
                    <h1>{t("games.title")}</h1>
                    <p className="muted">{t("games.intro")}</p>
                </div>
            </header>
            <p className="notice" role="note"><Icon name="shield" size={15}/> {t("games.legal")}</p>

            <section>
                <h2>{t("games.yours")}</h2>
                {data === null ? <p className="muted">{t("rail.scanning")}</p> : mine.length === 0 ? (
                    <div className="emptybox">
                        <p className="muted">{t("games.none")}</p>
                        <button className="primary" onClick={p.onBrowse}><Icon name="grid" size={15}/> {t("games.browse")}</button>
                    </div>
                ) : (
                    <ul className="emus">
                        {mine.map((a) => (
                            <li key={a.id}>
                                <button className="emu" onClick={() => p.onDetail(a)} aria-label={a.name}>
                                    <AppIcon id={a.id} name={a.name} category={a.category} size={36}/>
                                    <span>{a.name}</span>
                                </button>
                            </li>
                        ))}
                    </ul>
                )}
                {waiting && <p className="muted small">{t("games.waiting", waiting)}</p>}
            </section>

            {view && (
                <div className="folderbar">
                    <span className="muted">{t("games.folder")}</span>
                    <span className="mono grow" title={view.root}>{view.root}</span>
                    <button className="sm" onClick={() => void changeRoot()}>{t("games.folder.change")}</button>
                </div>
            )}

            <section className="patchcard">
                <h2>{t("games.patch.title")}</h2>
                <p className="muted">{t("games.patch.body")}</p>
                <button disabled={busy} onClick={() => void patch()}><Icon name="wrench" size={15}/> {t("games.patch.button")}</button>
                {note && <p className="resultline" role="status">{note}</p>}
            </section>

            {sections.map(({sys, players, sources}) => {
                const mineGames = view?.games.filter((g) => g.system === sys.id) ?? [];
                return (
                    <section key={sys.id} aria-label={text(sys)}>
                        <h2>{text(sys)}</h2>
                        <p className="muted small">{t("games.playsOn", {emulators: players.map((a) => a.name).join(", ")})}</p>
                        {mineGames.length > 0 && view && (
                            <>
                                <h3 className="subhead">{t("games.install.title")}</h3>
                                <ul className="gamelist">
                                    {mineGames.map((g) => (
                                        <GameRow key={g.id} g={g} root={view.root} installed={g.id in view.installed} pct={g.id in pct ? pct[g.id] : null}
                                                 players={startable(g)} openFrom={players.map((a) => a.name)} onOpen={p.onOpen}
                                                 onInstall={() => void install(g)} onPlay={(a) => play(g, a)}
                                                 onFolder={() => { api.OpenGameFolder(g.id).catch((e) => p.say(errText(e))); }}
                                                 onRemove={() => setRemoving(g)}/>
                                    ))}
                                </ul>
                                <p className="muted small">{t("games.install.hint")}</p>
                            </>
                        )}
                        {sources.length > 0 && (
                            <ul className="sources">{sources.map((s) => <Source key={s.id} s={s} onOpen={p.onOpen}/>)}</ul>
                        )}
                    </section>
                );
            })}
            {data !== null && mine.length > 0 && sections.length === 0 && <p className="muted">{t("games.noSystems")}</p>}

            {general.length > 0 && (
                <section>
                    <h2>{t("games.any")}</h2>
                    <p className="muted small">{t("games.anyHint")}</p>
                    <ul className="sources">{general.map((s) => <Source key={s.id} s={s} onOpen={p.onOpen}/>)}</ul>
                </section>
            )}

            {removing && (
                <ConfirmDialog title={t("games.removeTitle", {name: removing.name})} body={t("games.removeBody")} confirm={t("games.remove")} danger
                               onConfirm={() => { const g = removing; api.RemoveGame(g.id).then(() => refresh()).catch((e) => p.say(errText(e))); }}
                               onClose={() => setRemoving(null)}/>
            )}
        </div>
    );
}
