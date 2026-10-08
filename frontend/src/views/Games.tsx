import {useEffect, useMemo, useState} from "react";
import {api, errText, type EmuSource, type Emulation} from "../api";
import {AppIcon} from "../components/AppIcon";
import {Icon} from "../components/Icon";
import {getLang, t, type Key} from "../lib/i18n";
import type {App} from "../lib/model";

const text = (o: { en: string; es: string }) => (getLang() === "es" ? o.es : o.en);

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

/**
 * The emulators installed on this PC, the systems they play, and where to find
 * free games, homebrew and patches for each. Nothing is downloaded here.
 */
export function Games(p: {
    apps: App[]; say: (m: string) => void; onOpen: (url: string) => void; onBrowse: () => void; onDetail: (a: App) => void;
}) {
    const [data, setData] = useState<Emulation | null>(null);
    const [busy, setBusy] = useState(false);
    const [note, setNote] = useState<string | null>(null);

    useEffect(() => {
        let live = true;
        api.Emulation().then((d) => { if (live) setData(d); }).catch((e) => { if (live) { p.say(errText(e)); setData({systems: [], emulators: {}, sources: []}); } });
        return () => { live = false; };
    }, [p.say]);

    const mine = useMemo(() => (data ? p.apps.filter((a) => a.installed && data.emulators[a.id]) : []), [p.apps, data]);
    const sections = useMemo(() => {
        if (!data) return [];
        return data.systems.flatMap((sys) => {
            const players = mine.filter((a) => data.emulators[a.id].includes(sys.id));
            return players.length > 0 ? [{sys, players, sources: data.sources.filter((s) => s.system === sys.id)}] : [];
        });
    }, [data, mine]);
    const general = data?.sources.filter((s) => s.system === "any") ?? [];

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
        <div className="page games">
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
            </section>

            <section className="patchcard">
                <h2>{t("games.patch.title")}</h2>
                <p className="muted">{t("games.patch.body")}</p>
                <button disabled={busy} onClick={() => void patch()}><Icon name="wrench" size={15}/> {t("games.patch.button")}</button>
                {note && <p className="resultline" role="status">{note}</p>}
            </section>

            {sections.map(({sys, players, sources}) => (
                <section key={sys.id} aria-label={text(sys)}>
                    <h2>{text(sys)}</h2>
                    <p className="muted small">{t("games.playsOn", {emulators: players.map((a) => a.name).join(", ")})}</p>
                    {sources.length > 0 && (
                        <ul className="sources">{sources.map((s) => <Source key={s.id} s={s} onOpen={p.onOpen}/>)}</ul>
                    )}
                </section>
            ))}
            {data !== null && mine.length > 0 && sections.length === 0 && <p className="muted">{t("games.noSystems")}</p>}

            {general.length > 0 && (
                <section>
                    <h2>{t("games.any")}</h2>
                    <p className="muted small">{t("games.anyHint")}</p>
                    <ul className="sources">{general.map((s) => <Source key={s.id} s={s} onOpen={p.onOpen}/>)}</ul>
                </section>
            )}
        </div>
    );
}
