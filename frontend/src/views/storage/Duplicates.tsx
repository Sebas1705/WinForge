import {useMemo, useState} from "react";
import {api, errText, type DupGroup, type StorageDefaults} from "../../api";
import {Icon} from "../../components/Icon";
import {ConfirmDialog} from "../../components/Modals";
import {formatBytes, formatDate} from "../../lib/format";
import {getLang, t} from "../../lib/i18n";
import {defaultRemovals, fileCount, groupWaste, keptIn, removalTotals, sortGroups, splitPath, toggleRemoval} from "../../lib/storage";
import {TaskStatus, type TaskRunner} from "./useTask";

export const MIN_SIZES = [1, 10, 100] as const;
const PAGE = 25;

/** The folders searched by default: the ones people keep files in. */
export const defaultRoots = (d: StorageDefaults | null): string[] => (d ? [d.downloads, d.documents, d.pictures, d.videos].filter((x) => !!x) : []);

const same = (a: string, b: string) => a.replace(/[\\/]+$/, "").toLowerCase() === b.replace(/[\\/]+$/, "").toLowerCase();

function Group(p: { g: DupGroup; removed: ReadonlySet<string>; disabled: boolean; onToggle: (path: string) => void }) {
    const {g} = p;
    const kept = keptIn(g, p.removed);
    return (
        <li className="dupgroup">
            <header>
                <b>{t("storage.dup.group", {n: g.files.length, size: formatBytes(g.sizeBytes)})}</b>
                <span className="muted small">{t("storage.dup.waste", {size: formatBytes(groupWaste(g))})}</span>
            </header>
            <ul>
                {g.files.map((f) => {
                    const marked = p.removed.has(f.path);
                    const locked = !marked && kept.length === 1; // the last copy that stays
                    const {dir, name} = splitPath(f.path);
                    return (
                        <li key={f.path}>
                            <label className={marked ? "on" : ""} title={locked ? t("storage.dup.keptLocked") : f.path}>
                                <input type="checkbox" checked={marked} disabled={p.disabled || locked} onChange={() => p.onToggle(f.path)}
                                       aria-label={t("storage.dup.check", {path: f.path})}/>
                                <span className="grow dupname">
                                    <b>{name}</b>
                                    <small className="mono muted">{dir}</small>
                                </span>
                                <span className="muted small nowrap">{formatDate(f.modified, getLang())}</span>
                                {!marked && <span className="chip done"><Icon name="check" size={12}/> {t("storage.dup.kept")}</span>}
                            </label>
                        </li>
                    );
                })}
            </ul>
        </li>
    );
}

/**
 * Groups of files with identical content. In each group one copy always stays: the oldest
 * by default, and the person can pick another, but never all of them.
 */
export function Duplicates(p: { task: TaskRunner; defaults: StorageDefaults | null; say: (m: string) => void }) {
    const [custom, setCustom] = useState<string[] | null>(null); // null: still the default folders
    const [minMB, setMinMB] = useState<number>(10);
    const [groups, setGroups] = useState<DupGroup[] | null>(null);
    const [removed, setRemoved] = useState<Set<string>>(new Set());
    const [stopped, setStopped] = useState(false);
    const [shown, setShown] = useState(PAGE);
    const [confirming, setConfirming] = useState(false);
    const [moving, setMoving] = useState(false);
    const locked = p.task.busy !== null || moving;

    const roots = custom ?? defaultRoots(p.defaults);
    const totals = useMemo(() => removalTotals(groups ?? [], removed), [groups, removed]);

    const addFolder = async () => {
        try {
            const dir = await api.PickFolder();
            if (dir && !roots.some((r) => same(r, dir))) setCustom([...roots, dir]);
        } catch (e) { p.say(errText(e)); }
    };

    const scan = async () => {
        const out = await p.task.run("dups", () => api.StorageDuplicates(roots, minMB), (l) => t("tasks.sum.items", {n: l.length}));
        if (!out) return;
        setStopped(out.stopped);
        if (out.error !== undefined) {
            if (!out.stopped) p.say(errText(out.error));
            return;
        }
        const sorted = sortGroups(out.value ?? []);
        setGroups(sorted);
        setRemoved(defaultRemovals(sorted));
        setShown(PAGE);
    };

    const recycle = async () => {
        const paths = [...removed];
        if (paths.length === 0) return;
        setMoving(true);
        try {
            const r = await api.StorageRecycle(paths);
            if (r.moved > 0) p.say(t("storage.recycle.done", {n: r.moved, size: formatBytes(r.freedBytes)}));
            if (r.failed.length > 0) p.say(t("storage.recycle.failed", {n: r.failed.length, list: r.failed.map((x) => splitPath(x).name).join(", ")}));
        } catch (e) { p.say(errText(e)); setMoving(false); return; }
        setMoving(false);
        await scan();
    };

    const list = groups ?? [];
    return (
        <section aria-labelledby="storage-dups">
            <h2 id="storage-dups">{t("storage.dup.title")}</h2>
            <p className="muted small">{t("storage.dup.intro")}</p>

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
                <label className="check">{t("storage.dup.minsize")}
                    <select value={minMB} disabled={locked} onChange={(e) => setMinMB(Number(e.target.value))}>
                        {MIN_SIZES.map((n) => <option key={n} value={n}>{t("storage.dup.mb", {n})}</option>)}
                    </select>
                </label>
                <button className="primary" disabled={locked || roots.length === 0} onClick={() => void scan()}>
                    <Icon name="search" size={15}/> {groups === null ? t("storage.dup.scan") : t("storage.dup.rescan")}
                </button>
            </div>
            <TaskStatus task={p.task} names={["dups"]}/>
            {stopped && groups !== null && <p className="muted small" role="status">{t("storage.stopped")}</p>}
            {groups !== null && list.length === 0 && <p className="muted" role="status">{t("storage.dup.none")}</p>}

            {list.length > 0 && (
                <>
                    <ul className="dupgroups">
                        {list.slice(0, shown).map((g) => (
                            <Group key={g.id} g={g} removed={removed} disabled={locked}
                                   onToggle={(path) => setRemoved((r) => toggleRemoval(g, r, path))}/>
                        ))}
                    </ul>
                    {list.length > shown && (
                        <p><button className="sm" onClick={() => setShown((n) => n + PAGE)}>{t("storage.dup.more", {n: list.length - shown})}</button></p>
                    )}
                    <div className="cleanbar">
                        <span className="grow" role="status">
                            {totals.files > 0 ? t("storage.dup.summary", {files: fileCount(totals.files), size: formatBytes(totals.bytes)}) : t("storage.dup.summaryNone")}
                        </span>
                        <button className="danger" disabled={locked || totals.files === 0} onClick={() => setConfirming(true)}>
                            <Icon name="trash" size={14}/> {t("storage.dup.recycle")}
                        </button>
                    </div>
                </>
            )}

            {confirming && (
                <ConfirmDialog title={t("storage.recycle.title")} confirm={t("storage.recycle.confirm")} danger
                               body={t("storage.dup.confirmBody", {files: fileCount(totals.files), size: formatBytes(totals.bytes)})}
                               onConfirm={() => void recycle()} onClose={() => setConfirming(false)}/>
            )}
        </section>
    );
}
