import {useMemo, useState} from "react";
import {api, errText, type ProjectJunk, type ProjectResult, type StorageDefaults, type VirtualDisk} from "../../api";
import {Icon} from "../../components/Icon";
import {ConfirmDialog} from "../../components/Modals";
import {formatBytes, formatDate} from "../../lib/format";
import {getLang, t, tOpt} from "../../lib/i18n";
import {usePageState} from "../../lib/pageState";
import {splitPath} from "../../lib/storage";
import {TaskStatus, type TaskRunner} from "./useTask";

/** How long a project must have been left alone before its build folders are offered. */
export const AGES = [30, 60, 90, 180, 365] as const;
const PAGE = 25;

const same = (a: string, b: string) => a.replace(/[\\/]+$/, "").toLowerCase() === b.replace(/[\\/]+$/, "").toLowerCase();

/** The folders searched by default: where people usually keep code. */
export const projectRoots = (d: StorageDefaults | null): string[] => (d?.home ? [d.home] : []);

const folderCount = (n: number): string => t(n === 1 ? "storage.proj.folders.one" : "storage.proj.folders.other", {n});
const kindName = (kind: string): string => tOpt(`storage.proj.kind.${kind}`) ?? kind;

/** Sum of the sizes of the ticked folders. */
export const markedBytes = (list: ProjectJunk[], marked: ReadonlySet<string>): number =>
    list.reduce((n, j) => (marked.has(j.path) ? n + j.sizeBytes : n), 0);

function VirtualDisks(p: { busy: boolean; say: (m: string) => void }) {
    const [disks, setDisks] = usePageState<VirtualDisk[] | null>("storage:proj:disks", null);
    const find = async () => {
        try { setDisks(await api.StorageVirtualDisks()); } catch (e) { p.say(errText(e)); }
    };
    const reveal = (d: VirtualDisk) => { api.StorageReveal(d.path).catch((e) => p.say(errText(e))); };
    return (
        <section aria-labelledby="storage-vdisks">
            <h3 id="storage-vdisks" className="subhead">{t("storage.vdisk.title")}</h3>
            <p className="muted small">{t("storage.vdisk.intro")}</p>
            <div className="toolrow">
                <button disabled={p.busy} onClick={() => void find()}><Icon name="search" size={15}/> {t("storage.vdisk.find")}</button>
            </div>
            {disks !== null && disks.length === 0 && <p className="muted" role="status">{t("storage.vdisk.none")}</p>}
            {disks !== null && disks.length > 0 && (
                <>
                    <ul className="cleanlist">
                        {disks.map((d) => (
                            <li key={d.path}>
                                <div className="vdisk">
                                    <span className="grow">
                                        <b>{t("storage.vdisk.item", {name: d.name, size: formatBytes(d.sizeBytes)})}</b>
                                        <small className="mono muted" title={d.path}>{d.path}</small>
                                        <small className="muted">{t("storage.vdisk.modified", {date: formatDate(d.modified, getLang())})}</small>
                                    </span>
                                    <button className="sm iconbtn" title={t("storage.vdisk.reveal")} aria-label={`${t("storage.vdisk.reveal")}: ${d.name}`} onClick={() => reveal(d)}>
                                        <Icon name="external" size={15}/>
                                    </button>
                                </div>
                            </li>
                        ))}
                    </ul>
                    <p className="muted small">{t("storage.vdisk.compact")}</p>
                </>
            )}
        </section>
    );
}

/**
 * Dependency and build folders of projects nobody has touched for a while, plus the WSL and Docker disk
 * images. Deleting a folder is final, so nothing starts ticked, the question says so, and the Go side proves
 * each folder again at the moment it deletes it.
 */
export function Projects(p: { task: TaskRunner; defaults: StorageDefaults | null; say: (m: string) => void; onCleaned: () => void }) {
    // Kept while the page is away: the folders, the age, the list and which folders are ticked.
    const [custom, setCustom] = usePageState<string[] | null>("storage:proj:custom", null); // null: still the default folder
    const [days, setDays] = usePageState<number>("storage:proj:days", 90);
    const [list, setList] = usePageState<ProjectJunk[] | null>("storage:proj:list", null);
    const [ticked, setMarked] = usePageState<Set<string>>("storage:proj:marked", () => new Set());
    const [stopped, setStopped] = usePageState("storage:proj:stopped", false);
    const [shown, setShown] = usePageState("storage:proj:shown", PAGE);
    const [confirming, setConfirming] = useState(false);
    const [result, setResult] = usePageState<ProjectResult | null>("storage:proj:result", null);
    const locked = p.task.busy !== null;

    const roots = custom ?? projectRoots(p.defaults);
    const items = list ?? [];
    // A kept tick that points at a folder no longer in the kept list is dropped, never acted on.
    const marked = useMemo(() => {
        const known = new Set(items.map((j) => j.path));
        return new Set([...ticked].filter((x) => known.has(x)));
    }, [items, ticked]);
    const bytes = useMemo(() => markedBytes(items, marked), [items, marked]);

    const addFolder = async () => {
        try {
            const dir = await api.PickFolder();
            if (dir && !roots.some((r) => same(r, dir))) setCustom([...roots, dir]);
        } catch (e) { p.say(errText(e)); }
    };

    const scan = async () => {
        const out = await p.task.run("projects", () => api.StorageProjects(roots, days), (l) => t("tasks.sum.items", {n: l.length}));
        if (!out) return;
        setStopped(out.stopped);
        if (out.error !== undefined) {
            if (!out.stopped) p.say(errText(out.error));
            return;
        }
        setList(out.value ?? []);
        setMarked(new Set());
        setShown(PAGE);
    };

    const remove = async () => {
        const paths = items.filter((j) => marked.has(j.path)).map((j) => j.path);
        if (paths.length === 0) return;
        const out = await p.task.run("projects-clean", () => api.StorageProjectsClean(paths), (r) => t("tasks.sum.items", {n: r.removed}));
        if (!out) return;
        if (out.error !== undefined) {
            if (!out.stopped) p.say(errText(out.error));
            return;
        }
        if (out.value) setResult(out.value);
        p.onCleaned();
        await scan(); // what is left, as it is now
    };

    const toggle = (path: string) => setMarked((m) => { const n = new Set(m); if (n.has(path)) n.delete(path); else n.add(path); return n; });
    const reveal = (j: ProjectJunk) => { api.StorageReveal(j.path).catch((e) => p.say(errText(e))); };
    const chosen = marked.size;

    return (
        <section aria-labelledby="storage-proj">
            <h2 id="storage-proj">{t("storage.proj.title")}</h2>
            <p className="muted small">{t("storage.proj.intro")}</p>

            <h3 className="subhead">{t("storage.dup.folders")}</h3>
            <ul className="rootchips">
                {roots.map((r) => (
                    <li key={r} className="rootchip">
                        <span className="mono" title={r}>{r}</span>
                        <button className="icon" disabled={locked} aria-label={t("storage.dup.remove", {path: r})} onClick={() => setCustom(roots.filter((x) => x !== r))}>
                            <Icon name="x" size={13}/>
                        </button>
                    </li>
                ))}
                <li><button className="sm" disabled={locked} onClick={() => void addFolder()}><Icon name="plus" size={13}/> {t("storage.dup.add")}</button></li>
            </ul>
            {roots.length === 0 && <p className="muted small">{t("storage.dup.nofolders")}</p>}

            <div className="toolrow">
                <label className="check">{t("storage.proj.age")}
                    <select value={days} disabled={locked} onChange={(e) => setDays(Number(e.target.value))}>
                        {AGES.map((n) => <option key={n} value={n}>{t("storage.proj.days", {n})}</option>)}
                    </select>
                </label>
                <button className="primary" disabled={locked || roots.length === 0} onClick={() => { setResult(null); void scan(); }}>
                    <Icon name="search" size={15}/> {list === null ? t("storage.proj.scan") : t("storage.proj.rescan")}
                </button>
            </div>
            <TaskStatus task={p.task} names={["projects", "projects-clean"]}/>
            {stopped && list !== null && <p className="muted small" role="status">{t("storage.stopped")}</p>}

            {result && (
                <div className="resultline" role="status">
                    <p>{t("storage.proj.result", {n: result.removed, size: formatBytes(result.freedBytes)})}</p>
                    {result.failed.length > 0 && (
                        <>
                            <p className="muted small">{t("storage.proj.failed", {n: result.failed.length})}</p>
                            <ul className="plain small">
                                {result.failed.map((f, i) => <li key={f} className="muted">{t("storage.proj.failedLine", {name: f, reason: result.reasons[i] ?? ""})}</li>)}
                            </ul>
                        </>
                    )}
                </div>
            )}

            {list !== null && items.length === 0 && <p className="muted" role="status">{t("storage.proj.none")}</p>}
            {items.length > 0 && (
                <>
                    <div className="toolrow">
                        <button className="sm" disabled={locked} onClick={() => setMarked(new Set(items.map((j) => j.path)))}>{t("storage.proj.all")}</button>
                        <button className="sm" disabled={locked || chosen === 0} onClick={() => setMarked(new Set())}>{t("storage.proj.clear")}</button>
                    </div>
                    <ul className="cleanlist">
                        {items.slice(0, shown).map((j) => (
                            <li key={j.path}>
                                <label className={marked.has(j.path) ? "on" : ""}>
                                    <input type="checkbox" checked={marked.has(j.path)} disabled={locked} onChange={() => toggle(j.path)}
                                           aria-label={t("storage.proj.check", {path: j.path})}/>
                                    <span className="grow dupname">
                                        <span className="catname"><b>{j.project}</b> <span className="chip">{kindName(j.kind)}</span></span>
                                        <small className="mono muted" title={j.path}>{splitPath(j.path).dir}</small>
                                        <small className="muted">{t("storage.proj.touched", {date: formatDate(j.lastTouched, getLang())})}</small>
                                    </span>
                                    <span className="cleansize"><b className="mono">{formatBytes(j.sizeBytes)}</b></span>
                                </label>
                                <button className="sm iconbtn" title={t("storage.proj.reveal")} aria-label={`${t("storage.proj.reveal")}: ${j.project} (${kindName(j.kind)})`} onClick={() => reveal(j)}>
                                    <Icon name="external" size={15}/>
                                </button>
                            </li>
                        ))}
                    </ul>
                    {items.length > shown && (
                        <p><button className="sm" onClick={() => setShown((n) => n + PAGE)}>{t("storage.proj.more", {n: items.length - shown})}</button></p>
                    )}
                    <div className="cleanbar">
                        <span className="grow" role="status">
                            {chosen > 0 ? t("storage.proj.summary", {n: folderCount(chosen), size: formatBytes(bytes)}) : t("storage.proj.summaryNone")}
                        </span>
                        <button className="danger" disabled={locked || chosen === 0} onClick={() => setConfirming(true)}>
                            <Icon name="trash" size={14}/> {t("storage.proj.button")}
                        </button>
                    </div>
                </>
            )}

            <VirtualDisks busy={locked} say={p.say}/>

            {confirming && (
                <ConfirmDialog title={t("storage.proj.confirmTitle")} confirm={t("storage.proj.confirm")} danger
                               body={t("storage.proj.confirmBody", {n: folderCount(chosen), size: formatBytes(bytes)})}
                               onConfirm={() => void remove()} onClose={() => setConfirming(false)}>
                    <ul className="confirmlist">
                        {items.filter((j) => marked.has(j.path)).map((j) => <li key={j.path} className="mono">{j.path}</li>)}
                    </ul>
                </ConfirmDialog>
            )}
        </section>
    );
}
