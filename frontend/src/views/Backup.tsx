import {useCallback, useEffect, useState} from "react";
import {api, errText, type BackupPreview, type BackupSet} from "../api";
import {Icon, type IconName} from "../components/Icon";
import {Segmented} from "../components/Modals";
import {t, type Key} from "../lib/i18n";
import type {Profile} from "../lib/model";

const SET_ICON: Record<string, IconName> = {
    vscode: "code2", "vscode-insiders": "code2", vscodium: "code2", cursor: "code2", jetbrains: "code2", sublime: "code2", vim: "code2",
    git: "code", githubcli: "code", terminal: "monitor", alacritty: "monitor", wezterm: "monitor", powershell: "monitor", shell: "monitor",
    ssh: "lock", docker: "layers", wsl: "windows", cargo: "wrench", notepadpp: "list", powertoys: "wrench", flowlauncher: "search",
    altsnap: "grid", autohotkey: "play", sharex: "monitor", obs: "play", vlc: "play", mpv: "play", winget: "download",
};

const size = (b: number) => (b >= 1 << 30 ? `${(b / (1 << 30)).toFixed(1)} GB` : b >= 1 << 20 ? `${(b / (1 << 20)).toFixed(1)} MB` : `${Math.max(1, Math.round(b / 1024))} KB`);
const flip = (set: Set<string>, id: string) => { const n = new Set(set); if (n.has(id)) n.delete(id); else n.add(id); return n; };
const filesText = (n: number) => (n === 1 ? t("backup.file1") : t("backup.files", {n}));
const setName = (id: string) => t(`backup.set.${id}` as Key);
const setHint = (id: string) => t(`backup.set.${id}.hint` as Key);

type Stage = "off" | "wait" | "run" | "ok" | "fail" | "skip";

/** A checklist row: a box, an icon, a title with a line under it, and a note at the end. */
function Row(p: { checked: boolean; onChange: () => void; icon: IconName; title: string; hint?: string; note?: string; disabled?: boolean; label?: string }) {
    return (
        <li>
            <label className={p.disabled ? "off" : ""}>
                <input type="checkbox" checked={p.checked} disabled={p.disabled} onChange={p.onChange} aria-label={p.label ?? p.title}/>
                <span className="bubble sm"><Icon name={p.icon} size={18}/></span>
                <span className="grow"><b>{p.title}</b>{p.hint && <small className="muted">{p.hint}</small>}</span>
                {p.note && <span className="mono muted small">{p.note}</span>}
            </label>
        </li>
    );
}

function Picks(p: { onAll: () => void; onNone: () => void }) {
    return (
        <span className="picks">
            <button type="button" className="linkbtn" onClick={p.onAll}>{t("backup.selectAll")}</button>
            <button type="button" className="linkbtn" onClick={p.onNone}>{t("backup.selectNone")}</button>
        </span>
    );
}

export function Backup(p: {
    say: (m: string) => void; advanced: boolean; appCount: number;
    /** Runs the install flow for a profile; `done` is called when its dialog closes. */
    onInstallProfile: (profile: Profile, done: (o: { ran: boolean; failed: string[] }) => void) => void;
}) {
    const [mode, setMode] = useState<"create" | "rebuild">("create");
    return (
        <div className="page backup">
            <header className="pagehead">
                <div>
                    <h1>{t("backup.title")}</h1>
                    <p className="muted">{t("backup.intro")}</p>
                </div>
                <span className="grow"/>
                <Segmented value={mode} onChange={setMode} options={[["create", t("backup.tab.create")], ["rebuild", t("backup.tab.rebuild")]]}/>
            </header>
            <p className="muted small hint"><Icon name="shield" size={14}/> {t("backup.safe")}</p>
            {mode === "create" ? <Create {...p}/> : <Rebuild {...p}/>}
        </div>
    );
}

function Create(p: { say: (m: string) => void; advanced: boolean; appCount: number }) {
    const [sets, setSets] = useState<BackupSet[] | null>(null);
    const [picked, setPicked] = useState<Set<string>>(new Set());
    const [withApps, setWithApps] = useState(true);
    const [folders, setFolders] = useState<string[]>([]);
    const [busy, setBusy] = useState(false);
    const [result, setResult] = useState<string | null>(null);

    const load = useCallback(async () => {
        try {
            const found = await api.BackupSets();
            setSets(found);
            setPicked(new Set(found.map((s) => s.id)));
        } catch (e) { p.say(errText(e)); setSets([]); }
    }, [p.say]);
    useEffect(() => { void load(); }, [load]);

    const addFolder = async () => {
        try {
            const dir = await api.PickFolder();
            if (dir && !folders.includes(dir)) setFolders([...folders, dir]);
        } catch (e) { p.say(errText(e)); }
    };

    const create = async () => {
        const apps = withApps && p.appCount > 0;
        if (picked.size === 0 && !apps && folders.length === 0) return p.say(t("backup.nothing"));
        setBusy(true);
        setResult(null);
        try {
            const r = await api.CreateBackup({sets: [...picked], apps, folders});
            if (r.path) {
                const verdict = r.verified ? t("backup.verified") : t("backup.notVerified", {n: r.problems});
                setResult(`${t("backup.made", {path: r.path, files: r.files, size: size(r.bytes)})} ${verdict}`);
                p.say(t("backup.made", {path: r.path, files: r.files, size: size(r.bytes)}));
            }
        } catch (e) { p.say(errText(e)); } finally { setBusy(false); }
    };

    const folderName = (d: string) => d.split(/[\\/]/).filter(Boolean).pop() ?? d;

    return (
        <>
            <section>
                <h2>{t("backup.create.title")}</h2>
                <ul className="setlist">
                    <Row checked={withApps && p.appCount > 0} disabled={p.appCount === 0} onChange={() => setWithApps(!withApps)} icon="layers"
                         title={t("backup.apps")} hint={p.appCount > 0 ? t("backup.apps.hint", {n: p.appCount}) : t("backup.apps.none")}/>
                </ul>
            </section>

            <section>
                <div className="sectionhead">
                    <h2>{t("backup.settings")}</h2>
                    {sets && sets.length > 0 && <Picks onAll={() => setPicked(new Set(sets.map((s) => s.id)))} onNone={() => setPicked(new Set())}/>}
                </div>
                {sets === null ? <p className="muted">{t("rail.scanning")}</p>
                    : sets.length === 0 ? <p className="muted empty">{t("backup.none")}</p>
                    : (
                        <ul className="setlist">
                            {sets.map((s) => (
                                <Row key={s.id} checked={picked.has(s.id)} onChange={() => setPicked(flip(picked, s.id))} icon={SET_ICON[s.id] ?? "settings"}
                                     title={setName(s.id)} hint={setHint(s.id)} note={filesText(s.files) + (p.advanced && s.bytes > 0 ? ` · ${size(s.bytes)}` : "")}/>
                            ))}
                        </ul>
                    )}
            </section>

            <section>
                <h2>{t("backup.folders")}</h2>
                <p className="muted small">{t("backup.folders.hint")}</p>
                {folders.length > 0 && (
                    <ul className="setlist">
                        {folders.map((d) => (
                            <li key={d}>
                                <div className="folderrow">
                                    <span className="bubble sm"><Icon name="archive" size={18}/></span>
                                    <span className="grow"><b>{folderName(d)}</b><small className="muted mono">{d}</small></span>
                                    <button className="icon" aria-label={t("backup.removeFolder", {name: folderName(d)})} onClick={() => setFolders(folders.filter((x) => x !== d))}>
                                        <Icon name="x" size={16}/>
                                    </button>
                                </div>
                            </li>
                        ))}
                    </ul>
                )}
                <button onClick={() => void addFolder()}><Icon name="plus" size={14}/> {t("backup.addFolder")}</button>
            </section>

            <div className="actions">
                <button className="primary big" disabled={busy} onClick={() => void create()}><Icon name="archive" size={16}/> {t("backup.create")}</button>
            </div>
            {result && <p className="resultline" role="status">{result}</p>}
        </>
    );
}

function Rebuild(p: {
    say: (m: string) => void; advanced: boolean;
    onInstallProfile: (profile: Profile, done: (o: { ran: boolean; failed: string[] }) => void) => void;
}) {
    const [prev, setPrev] = useState<BackupPreview | null>(null);
    const [doApps, setDoApps] = useState(false);
    const [pickedSets, setPickedSets] = useState<Set<string>>(new Set());
    const [pickedFolders, setPickedFolders] = useState<Set<string>>(new Set());
    const [dest, setDest] = useState("");
    const [busy, setBusy] = useState(false);
    const [stages, setStages] = useState<Record<"apps" | "settings" | "folders", Stage>>({apps: "off", settings: "off", folders: "off"});
    const [notes, setNotes] = useState<string[]>([]);

    const choose = async () => {
        try {
            const r = await api.PickBackup();
            if (!r) return;
            setPrev(r);
            setDoApps(r.missing > 0);
            setPickedSets(new Set(r.sets.filter((s) => s.new + s.changed > 0 || r.sets.length === 1).map((s) => s.id)));
            setPickedFolders(new Set(r.folders.map((f) => f.name)));
            setDest("");
            setStages({apps: "off", settings: "off", folders: "off"});
            setNotes([]);
        } catch (e) { p.say(errText(e)); }
    };

    const pickDest = async () => {
        try { const d = await api.PickFolder(); if (d) setDest(d); } catch (e) { p.say(errText(e)); }
    };

    const rebuild = async () => {
        if (!prev) return;
        const wantFolders = pickedFolders.size > 0;
        if (wantFolders && !dest) return p.say(t("backup.needDest"));
        setBusy(true);
        setNotes([]);
        const log = (m: string) => setNotes((n) => [...n, m]);
        const stage = (k: "apps" | "settings" | "folders", v: Stage) => setStages((s) => ({...s, [k]: v}));
        setStages({apps: doApps ? "wait" : "off", settings: pickedSets.size > 0 ? "wait" : "off", folders: wantFolders ? "wait" : "off"});
        try {
            // 1. Apps first: settings only make sense once the apps are there.
            if (doApps && prev.profile) {
                stage("apps", "run");
                const out = await new Promise<{ ran: boolean; failed: string[] }>((res) => p.onInstallProfile(prev.profile as Profile, res));
                stage("apps", !out.ran ? "skip" : out.failed.length > 0 ? "fail" : "ok");
                log(out.ran ? t("backup.appsDone") : t("backup.appsCancelled"));
            }
            if (pickedSets.size > 0) {
                stage("settings", "run");
                try {
                    const r = await api.RestoreSettings([...pickedSets]);
                    const extra = (r.extensions > 0 ? t("backup.restoredExt", {n: r.extensions}) : "") + (r.backedUp > 0 ? t("backup.restoredKept", {n: r.backedUp}) : "");
                    log(t("backup.setsDone", {n: r.restored, extra}));
                    stage("settings", r.skipped.length > 0 ? "fail" : "ok");
                } catch (e) { stage("settings", "fail"); log(errText(e)); }
            }
            if (wantFolders) {
                stage("folders", "run");
                try {
                    const r = await api.RestoreFolders([...pickedFolders], dest);
                    log(t("backup.foldersDone", {written: r.written, same: r.same, kept: r.kept}));
                    stage("folders", r.skipped.length > 0 ? "fail" : "ok");
                } catch (e) { stage("folders", "fail"); log(errText(e)); }
            }
            p.say(t("backup.rebuilt"));
        } finally { setBusy(false); }
    };

    const stageText = (s: Stage) => (s === "off" ? "" : t(`backup.stage.${s}` as Key));
    const total = pickedFolders.size + pickedSets.size + (doApps ? 1 : 0);

    return (
        <>
            <div className="actions">
                <button className="primary" disabled={busy} onClick={() => void choose()}><Icon name="archive" size={15}/> {t("backup.pick")}</button>
            </div>

            {prev && (
                <>
                    <p className="muted">{prev.host ? t("backup.from", {host: prev.host, date: prev.createdAt, app: prev.app || "?"}) : t("backup.fromUnknown", {date: prev.createdAt})}</p>
                    {prev.intact && prev.checked && <p className="badge ok" role="status"><Icon name="check" size={14}/> {t("backup.intact")}</p>}
                    {prev.intact && !prev.checked && <p className="badge warn" role="status"><Icon name="alert" size={14}/> {t("backup.noSums")}</p>}
                    {!prev.intact && <p className="badge bad" role="alert"><Icon name="alert" size={14}/> {t("backup.damaged", {n: prev.problems.length})}</p>}

                    <section>
                        <h2>{t("backup.stage.apps")} {stages.apps !== "off" && <span className={`stagebadge ${stages.apps}`}>{stageText(stages.apps)}</span>}</h2>
                        {prev.apps.length === 0 ? <p className="muted">{t("backup.stage.apps.none")}</p> : (
                            <ul className="setlist">
                                <Row checked={doApps} disabled={prev.missing === 0} onChange={() => setDoApps(!doApps)} icon="layers"
                                     title={t("backup.apps")} hint={t("backup.stage.apps.body", {missing: prev.missing, have: prev.apps.length - prev.missing})}
                                     note={p.advanced ? prev.apps.filter((a) => !a.installed).map((a) => a.name).join(", ") : undefined}/>
                            </ul>
                        )}
                        {prev.unknown.length > 0 && <p className="muted small">{t("backup.unknownApps", {n: prev.unknown.length})}</p>}
                    </section>

                    <section>
                        <h2>{t("backup.stage.settings")} {stages.settings !== "off" && <span className={`stagebadge ${stages.settings}`}>{stageText(stages.settings)}</span>}</h2>
                        {prev.sets.length === 0 ? <p className="muted">{t("backup.stage.settings.none")}</p> : (
                            <ul className="setlist">
                                {prev.sets.map((s) => (
                                    <Row key={s.id} checked={pickedSets.has(s.id)} onChange={() => setPickedSets(flip(pickedSets, s.id))} icon={SET_ICON[s.id] ?? "settings"}
                                         title={setName(s.id)} hint={setHint(s.id)} note={t("backup.counts", {new: s.new, changed: s.changed, same: s.same})}/>
                                ))}
                            </ul>
                        )}
                        {prev.sets.length > 0 && <p className="muted small">{t("backup.restoreHint")}</p>}
                    </section>

                    <section>
                        <h2>{t("backup.stage.folders")} {stages.folders !== "off" && <span className={`stagebadge ${stages.folders}`}>{stageText(stages.folders)}</span>}</h2>
                        {prev.folders.length === 0 ? <p className="muted">{t("backup.stage.folders.none")}</p> : (
                            <>
                                <ul className="setlist">
                                    {prev.folders.map((f) => (
                                        <Row key={f.dir} checked={pickedFolders.has(f.name)} onChange={() => setPickedFolders(flip(pickedFolders, f.name))} icon="archive"
                                             title={f.name} note={`${filesText(f.files)} · ${size(f.bytes)}`}/>
                                    ))}
                                </ul>
                                <div className="destrow">
                                    <span className="muted">{t("backup.dest")}</span>
                                    <span className="mono grow">{dest || "—"}</span>
                                    <button onClick={() => void pickDest()}>{t("backup.pickDest")}</button>
                                </div>
                            </>
                        )}
                    </section>

                    <div className="actions">
                        <button className="primary big" disabled={busy || total === 0} onClick={() => void rebuild()}>
                            <Icon name="download" size={16}/> {busy ? t("backup.rebuilding") : t("backup.rebuild")}
                        </button>
                    </div>
                    {notes.length > 0 && <ul className="plain notes" role="status">{notes.map((n, i) => <li key={i}>{n}</li>)}</ul>}
                </>
            )}
        </>
    );
}
