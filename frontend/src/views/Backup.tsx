import {useCallback, useEffect, useState} from "react";
import {api, errText, type BackupSet} from "../api";
import {Icon, type IconName} from "../components/Icon";
import {t, type Key} from "../lib/i18n";

const SET_ICON: Record<string, IconName> = {
    vscode: "code2", git: "code", terminal: "monitor", powershell: "code2", ssh: "lock", notepadpp: "list", vlc: "play", winget: "download",
};

const size = (b: number) => (b >= 1 << 20 ? `${(b / (1 << 20)).toFixed(1)} MB` : `${Math.max(1, Math.round(b / 1024))} KB`);

/** A checklist of what a backup holds, with one action under it. */
function SetList(p: { sets: BackupSet[]; picked: Set<string>; onToggle: (id: string) => void; advanced: boolean }) {
    return (
        <ul className="setlist">
            {p.sets.map((s) => (
                <li key={s.id}>
                    <label>
                        <input type="checkbox" checked={p.picked.has(s.id)} onChange={() => p.onToggle(s.id)}/>
                        <span className="bubble sm"><Icon name={SET_ICON[s.id] ?? "settings"} size={18}/></span>
                        <span className="grow">
                            <b>{t(`backup.set.${s.id}` as Key)}</b>
                            <small className="muted">{t(`backup.set.${s.id}.hint` as Key)}</small>
                        </span>
                        <span className="mono muted small">{s.files === 1 ? t("backup.file1") : t("backup.files", {n: s.files})}{p.advanced && s.bytes > 0 ? ` · ${size(s.bytes)}` : ""}</span>
                    </label>
                </li>
            ))}
        </ul>
    );
}

export function Backup(p: { say: (m: string) => void; advanced: boolean }) {
    const [sets, setSets] = useState<BackupSet[] | null>(null);
    const [picked, setPicked] = useState<Set<string>>(new Set());
    // A chosen backup file: what it holds and what the person keeps ticked.
    const [restore, setRestore] = useState<{ sets: BackupSet[]; picked: Set<string> } | null>(null);
    const [busy, setBusy] = useState(false);

    const load = useCallback(async () => {
        try {
            const found = await api.BackupSets();
            setSets(found);
            setPicked(new Set(found.map((s) => s.id)));
        } catch (e) { p.say(errText(e)); setSets([]); }
    }, [p.say]);
    useEffect(() => { void load(); }, [load]);

    const flip = (set: Set<string>, id: string) => { const n = new Set(set); if (n.has(id)) n.delete(id); else n.add(id); return n; };

    const save = async () => {
        if (picked.size === 0) return p.say(t("backup.nothingSelected"));
        setBusy(true);
        try {
            const path = await api.BackupSettings([...picked]);
            if (path) p.say(t("backup.saved", {path}));
        } catch (e) { p.say(errText(e)); } finally { setBusy(false); }
    };

    const choose = async () => {
        try {
            const r = await api.PickRestore();
            if (r) setRestore({sets: r.sets, picked: new Set(r.sets.map((s) => s.id))});
        } catch (e) { p.say(errText(e)); }
    };

    const doRestore = async () => {
        if (!restore || restore.picked.size === 0) return p.say(t("backup.nothingSelected"));
        setBusy(true);
        try {
            const r = await api.RestoreSettings([...restore.picked]);
            const extra = (r.extensions > 0 ? t("backup.restoredExt", {n: r.extensions}) : "") + (r.backedUp > 0 ? t("backup.restoredKept", {n: r.backedUp}) : "");
            p.say(t("backup.restored", {n: r.restored, extra}));
            setRestore(null);
            void load();
        } catch (e) { p.say(errText(e)); } finally { setBusy(false); }
    };

    return (
        <div className="page backup">
            <header className="pagehead">
                <div>
                    <h1>{t("backup.title")}</h1>
                    <p className="muted">{t("backup.intro")}</p>
                </div>
            </header>
            <p className="muted small hint"><Icon name="shield" size={14}/> {t("backup.safe")}</p>

            {restore ? (
                <section>
                    <h2>{t("backup.restoreTitle")}</h2>
                    <SetList sets={restore.sets} picked={restore.picked} advanced={p.advanced} onToggle={(id) => setRestore({...restore, picked: flip(restore.picked, id)})}/>
                    <p className="muted small">{t("backup.restoreHint")}</p>
                    <div className="actions">
                        <button className="primary" disabled={busy || restore.picked.size === 0} onClick={() => void doRestore()}>
                            <Icon name="download" size={15}/> {t("backup.restoreNow")}
                        </button>
                        <button onClick={() => setRestore(null)}>{t("common.cancel")}</button>
                    </div>
                </section>
            ) : (
                <>
                    <section>
                        {sets === null ? <p className="muted">{t("rail.scanning")}</p>
                            : sets.length === 0 ? <p className="muted empty">{t("backup.none")}</p>
                            : <SetList sets={sets} picked={picked} advanced={p.advanced} onToggle={(id) => setPicked(flip(picked, id))}/>}
                    </section>
                    <div className="actions">
                        <button className="primary" disabled={busy || !sets || sets.length === 0 || picked.size === 0} onClick={() => void save()}>
                            <Icon name="archive" size={15}/> {t("backup.save")}
                        </button>
                        <button disabled={busy} onClick={() => void choose()}>{t("backup.restore")}</button>
                    </div>
                </>
            )}
        </div>
    );
}
