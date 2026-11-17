import {useEffect, useState} from "react";
import {api, errText, type BigFile, type FolderSize} from "../../api";
import {Icon} from "../../components/Icon";
import {ConfirmDialog, Segmented} from "../../components/Modals";
import {formatBytes, formatDate} from "../../lib/format";
import {getLang, t} from "../../lib/i18n";
import {fileCount, folderName, splitPath} from "../../lib/storage";
import {SpaceMap} from "./SpaceMap";
import {TaskStatus, type TaskRunner} from "./useTask";

/** How many of the biggest files are asked for. */
export const BIG_LIMIT = 50;

type Tab = "files" | "folders" | "map";
/** What is read: the Map tab draws the same folder sizes as the Folders tab. */
type Kind = "files" | "folders";
type Note = "stopped" | "error";

/**
 * The largest files and folders under a folder the person picks. A folder in the list
 * becomes the new root, with a breadcrumb back. Files can be shown in Explorer or moved
 * to the Recycle Bin after a confirmation.
 */
export function Biggest(p: { task: TaskRunner; home: string; say: (m: string) => void }) {
    const [trail, setTrail] = useState<string[]>([]);
    const [tab, setTab] = useState<Tab>("files");
    const [started, setStarted] = useState(false);
    const [files, setFiles] = useState<Record<string, BigFile[]>>({});
    const [folders, setFolders] = useState<Record<string, FolderSize[]>>({});
    const [notes, setNotes] = useState<Record<string, Note>>({});
    const [recycling, setRecycling] = useState<BigFile | null>(null);
    const [moving, setMoving] = useState(false);

    const root = trail.length > 0 ? trail[trail.length - 1] : p.home;
    const kind: Kind = tab === "map" ? "folders" : tab;
    const key = `${kind}|${root}`;
    const have = kind === "files" ? root in files : root in folders;
    const locked = p.task.busy !== null || moving;

    const load = async (which: Kind, dir: string) => {
        const k = `${which}|${dir}`;
        const out = which === "files"
            ? await p.task.run("big", () => api.StorageBigFiles(dir, BIG_LIMIT))
            : await p.task.run("folders", () => api.StorageFolders(dir));
        if (!out) return;
        if (out.error !== undefined) {
            setNotes((n) => ({...n, [k]: out.stopped ? "stopped" : "error"}));
            if (!out.stopped) p.say(errText(out.error));
            return;
        }
        if (out.stopped) setNotes((n) => ({...n, [k]: "stopped"}));
        if (which === "files") setFiles((m) => ({...m, [dir]: (out.value as BigFile[] | undefined) ?? []}));
        else setFolders((m) => ({...m, [dir]: (out.value as FolderSize[] | undefined) ?? []}));
    };

    // After the first scan, moving into a folder or changing tab reads what is not known yet.
    // A failed or stopped read leaves a note, so it is not retried until the person asks.
    useEffect(() => {
        if (started && root && !have && !(key in notes) && p.task.busy === null) void load(kind, root);
    }, [started, root, kind, have, p.task.busy]);

    const scanNow = () => {
        if (!root) return;
        setNotes((n) => { const c = {...n}; delete c[key]; return c; });
        setStarted(true);
        void load(kind, root);
    };

    const choose = async () => {
        try {
            const dir = await api.PickFolder();
            if (!dir) return;
            setTrail([dir]);
            setStarted(true);
            setFiles({}); setFolders({}); setNotes({});
        } catch (e) { p.say(errText(e)); }
    };

    const recycle = async (f: BigFile) => {
        setMoving(true);
        try {
            const r = await api.StorageRecycle([f.path]);
            const gone = new Set([f.path].filter((x) => !r.failed.includes(x)));
            setFiles((m) => Object.fromEntries(Object.entries(m).map(([dir, list]) => [dir, list.filter((x) => !gone.has(x.path))])));
            setFolders({}); // sizes of folders changed
            if (r.moved > 0) p.say(t("storage.recycle.done", {n: r.moved, size: formatBytes(r.freedBytes)}));
            if (r.failed.length > 0) p.say(t("storage.recycle.failed", {n: r.failed.length, list: r.failed.map((x) => splitPath(x).name).join(", ")}));
        } catch (e) { p.say(errText(e)); } finally { setMoving(false); }
    };

    const revealPath = (path: string) => { api.StorageReveal(path).catch((e) => p.say(errText(e))); };
    const openFolder = (path: string) => setTrail([...(trail.length > 0 ? trail : [root]), path]);
    const reveal = (f: BigFile) => { api.StorageReveal(f.path).catch((e) => p.say(errText(e))); };

    const list = kind === "files" ? files[root] : folders[root];
    const biggest = kind === "folders" ? Math.max(1, ...(folders[root] ?? []).map((f) => f.sizeBytes)) : 1;

    return (
        <section aria-labelledby="storage-big">
            <h2 id="storage-big">{t("storage.big.title")}</h2>
            <p className="muted small">{t("storage.big.intro")}</p>

            <div className="folderbar">
                <span className="muted">{t("storage.big.look")}</span>
                <span className="mono grow" title={root}>{root || "—"}</span>
                <button className="sm" disabled={locked} onClick={() => void choose()}>{t("storage.big.choose")}</button>
            </div>

            {trail.length > 1 && (
                <nav className="crumbs" aria-label={t("storage.big.crumbs")}>
                    <ol>
                        {trail.map((dir, i) => (
                            <li key={`${i}|${dir}`}>
                                {i < trail.length - 1 ? (
                                    <button className="linkbtn" disabled={locked} title={dir} onClick={() => setTrail(trail.slice(0, i + 1))}>{i === 0 ? dir : folderName(dir)}</button>
                                ) : <span aria-current="location" title={dir}>{folderName(dir)}</span>}
                                {i < trail.length - 1 && <Icon name="chevron" size={12}/>}
                            </li>
                        ))}
                    </ol>
                </nav>
            )}

            <div className="toolrow">
                <Segmented<Tab> value={tab} onChange={setTab} options={[["files", t("storage.big.files")], ["folders", t("storage.big.folders")], ["map", t("tmap.tab")]]}/>
                <button className="primary" disabled={locked || !root} onClick={scanNow}>
                    <Icon name="search" size={15}/> {have ? t("storage.big.rescan") : t("storage.big.scan")}
                </button>
            </div>
            <TaskStatus task={p.task} names={["big", "folders"]}/>
            {notes[key] === "stopped" && <p className="muted small" role="status">{t("storage.stopped")}</p>}
            {!root && <p className="muted">{t("storage.big.choose.first")}</p>}

            {list && tab === "files" && (files[root].length === 0 ? <p className="muted" role="status">{t("storage.big.emptyFiles")}</p> : (
                <div className="tablewrap bigfiles">
                    <table>
                        <thead>
                            <tr>
                                <th scope="col">{t("storage.big.colName")}</th>
                                <th scope="col" className="num">{t("storage.big.colSize")}</th>
                                <th scope="col">{t("storage.big.colModified")}</th>
                                <th scope="col">{t("storage.big.colFolder")}</th>
                                <th scope="col">{t("storage.big.colActions")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {files[root].map((f) => (
                                <tr key={f.path}>
                                    <td className="fname" title={f.path}>{f.name}</td>
                                    <td className="num mono">{formatBytes(f.sizeBytes)}</td>
                                    <td className="nowrap">{formatDate(f.modified, getLang())}</td>
                                    <td className="fdir mono" title={splitPath(f.path).dir}>{splitPath(f.path).dir}</td>
                                    <td className="factions">
                                        <button className="sm iconbtn" disabled={moving} title={t("storage.big.reveal")} aria-label={`${t("storage.big.reveal")}: ${f.name}`} onClick={() => reveal(f)}>
                                            <Icon name="external" size={15}/>
                                        </button>
                                        <button className="sm iconbtn danger" disabled={locked} title={t("storage.big.recycle")} aria-label={`${t("storage.big.recycle")}: ${f.name}`} onClick={() => setRecycling(f)}>
                                            <Icon name="trash" size={15}/>
                                        </button>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            ))}

            {list && tab === "folders" && (folders[root].length === 0 ? <p className="muted" role="status">{t("storage.big.emptyFolders")}</p> : (
                <ul className="folderlist">
                    {folders[root].map((f) => (
                        <li key={f.path}>
                            <button className="folderitem" disabled={locked} title={f.path} aria-label={t("storage.big.openFolder", {name: f.name})}
                                    onClick={() => openFolder(f.path)}>
                                <span className="fname">{f.name}</span>
                                <span className="mono fsize">{formatBytes(f.sizeBytes)}</span>
                                <span className="muted small fcount">{fileCount(f.files)}</span>
                                <span className="bar" aria-hidden="true"><i style={{width: `${Math.max(2, Math.round((f.sizeBytes / biggest) * 100))}%`}}/></span>
                            </button>
                        </li>
                    ))}
                </ul>
            ))}

            {list && tab === "map" && <SpaceMap key={root} folders={folders[root]} root={root} locked={locked} onOpen={openFolder} onReveal={revealPath}/>}

            {recycling && (
                <ConfirmDialog title={t("storage.recycle.title")} confirm={t("storage.recycle.confirm")} danger
                               body={t("storage.recycle.body", {name: recycling.name, size: formatBytes(recycling.sizeBytes)})}
                               onConfirm={() => void recycle(recycling)} onClose={() => setRecycling(null)}/>
            )}
        </section>
    );
}
