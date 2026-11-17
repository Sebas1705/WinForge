import {useEffect, useId, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent} from "react";
import type {FolderSize} from "../../api";
import {Icon} from "../../components/Icon";
import {formatBytes} from "../../lib/format";
import {t} from "../../lib/i18n";
import {fileCount, folderName} from "../../lib/storage";
import {compareEntries, labelFits, layoutTreemap, readingOrder, tintOf, type TreeCell, type TreeEntry} from "../../lib/treemap";
import "../../tmap.css";

/** Blocks drawn at most (the merged "other" block included), and rows of the table. */
export const MAP_MAX_BLOCKS = 60;
const TABLE_ROWS = 200;
/** Width used when the browser cannot report sizes (no ResizeObserver). */
const FALLBACK_WIDTH = 640;

/** Height for a width: roughly 5:3, never under 240 px (about 4:3 on a phone-sized window) nor over 520. */
export const mapHeight = (width: number): number => Math.min(520, Math.max(240, Math.round(width * 0.6)));

/** "31%", or "less than 1%" for a block too small to round to a number. */
export function pctText(share: number): string {
    const n = Math.round(share * 100);
    return n < 1 ? t("tmap.pct.small") : t("tmap.pct", {n});
}

/** The width of an element, kept up to date with a ResizeObserver. */
function useWidth(ref: { current: HTMLElement | null }): number {
    const [width, setWidth] = useState(0);
    useLayoutEffect(() => {
        const el = ref.current;
        if (!el) return;
        if (typeof ResizeObserver === "undefined") { setWidth(Math.floor(el.clientWidth) || FALLBACK_WIDTH); return; }
        const first = Math.floor(el.clientWidth);
        if (first > 0) setWidth(first);
        const ro = new ResizeObserver((entries) => {
            const w = entries[entries.length - 1]?.contentRect.width ?? el.clientWidth;
            setWidth(Math.max(0, Math.floor(w)));
        });
        ro.observe(el);
        return () => ro.disconnect();
    }, [ref]);
    return width;
}

/**
 * The folders of the folder you are in as a treemap: every block is a folder and its area is its size.
 * Size is also written in each block's label (when it fits), its tooltip and its accessible name, and
 * in a table; colour only orders the blocks by size rank, it carries no meaning of its own.
 * A block is a real button: Enter, Space or a click goes into the folder; the arrow keys move between
 * blocks in reading order, and only one block is in the Tab order.
 */
export function SpaceMap(p: {
    folders: FolderSize[]; root: string; locked: boolean;
    onOpen: (path: string) => void; onReveal: (path: string) => void;
}) {
    const id = useId();
    const wrap = useRef<HTMLDivElement>(null);
    const width = useWidth(wrap);
    const height = mapHeight(width);
    const [table, setTable] = useState(false);
    const [sel, setSel] = useState<string | null>(null);
    const [active, setActive] = useState<string | null>(null);
    const refs = useRef(new Map<string, HTMLButtonElement>());

    const byId = useMemo(() => new Map(p.folders.map((f) => [f.path, f])), [p.folders]);
    const entries = useMemo<TreeEntry[]>(() => p.folders.map((f) => ({id: f.path, name: f.name, size: f.sizeBytes})), [p.folders]);
    const layout = useMemo(() => layoutTreemap(entries, width, height, {maxBlocks: MAP_MAX_BLOCKS}), [entries, width, height]);
    // Blocks that are too thin to see are not drawn (they stay in the table); the rest are in reading order.
    const cells = useMemo(() => readingOrder(layout.cells.filter((c) => c.rect.w >= 4 && c.rect.h >= 4)), [layout]);
    const itemCount = layout.cells.filter((c) => c.kind === "item").length;

    const first = cells[0]?.id ?? null;
    const tabStop = active !== null && cells.some((c) => c.id === active) ? active : first;
    const selected = sel !== null ? cells.find((c) => c.id === sel) ?? null : null;

    const focusAt = (i: number) => {
        const c = cells[Math.max(0, Math.min(cells.length - 1, i))];
        if (c) refs.current.get(c.id)?.focus();
    };
    const onKey = (e: KeyboardEvent, i: number) => {
        if (e.altKey || e.ctrlKey || e.metaKey) return;
        const step = e.key === "ArrowRight" || e.key === "ArrowDown" ? 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? -1 : 0;
        if (step !== 0) { e.preventDefault(); focusAt(i + step); }
        else if (e.key === "Home") { e.preventDefault(); focusAt(0); }
        else if (e.key === "End") { e.preventDefault(); focusAt(cells.length - 1); }
    };

    // The selection and the tab stop follow the data: a block that is gone cannot stay selected.
    useEffect(() => {
        if (sel !== null && !cells.some((c) => c.id === sel)) setSel(null);
    }, [cells, sel]);

    const label = (c: TreeCell): string => c.kind === "other"
        ? t("tmap.other", {n: c.merged.length})
        : c.name;
    const accessible = (c: TreeCell): string => c.kind === "other"
        ? t("tmap.other.block", {n: c.merged.length, size: formatBytes(c.size), pct: pctText(c.share)})
        : t("tmap.block", {name: c.name, size: formatBytes(c.size), pct: pctText(c.share)});
    const tip = (c: TreeCell): string => c.kind === "other"
        ? t("tmap.other.tip", {n: c.merged.length, size: formatBytes(c.size)})
        : t("tmap.tip", {path: c.id, size: formatBytes(c.size)});

    const sorted = useMemo(() => [...entries].filter((e) => Number.isFinite(e.size) && e.size > 0).sort(compareEntries), [entries]);
    const rows = sorted.slice(0, TABLE_ROWS);

    if (sorted.length === 0) return <p className="muted" role="status">{t("tmap.empty")}</p>;

    const biggest = layout.cells.find((c) => c.kind === "item");
    const summary = biggest
        ? t("tmap.summary", {n: cells.length, name: biggest.name, size: formatBytes(biggest.size), pct: pctText(biggest.share)})
        : "";

    return (
        <figure className="tmap">
            <p className="muted small">{t("tmap.intro")}</p>
            <p className="tm-stats mono small">{t("tmap.stats", {n: sorted.length, size: formatBytes(layout.total)})}</p>
            <div className="tm-wrap" ref={wrap}>
                {width > 0 && (
                    <div className="tm-grid" role="group" aria-label={t("tmap.label", {folder: folderName(p.root)})} aria-describedby={`${id}-d`} style={{height}}>
                        <p id={`${id}-d`} className="sr-only">{summary} {t("tmap.keys")}</p>
                        {cells.map((c, i) => {
                            const r = c.rect;
                            const two = labelFits(r, 2), one = !two && labelFits(r, 1, 64, 16, 8);
                            const other = c.kind === "other";
                            return (
                                <button
                                    key={c.id}
                                    ref={(el) => { if (el) refs.current.set(c.id, el); else refs.current.delete(c.id); }}
                                    type="button"
                                    className={`tm-block${other ? " tm-other" : ""}${sel === c.id ? " tm-sel" : ""}`}
                                    data-tint={other ? undefined : tintOf(c.rank, itemCount)}
                                    style={{left: r.x + 1, top: r.y + 1, width: r.w - 2, height: r.h - 2}}
                                    tabIndex={c.id === tabStop ? 0 : -1}
                                    aria-label={accessible(c)}
                                    aria-disabled={other || undefined}
                                    disabled={!other && p.locked}
                                    title={tip(c)}
                                    onFocus={() => { setActive(c.id); setSel(c.id); }}
                                    onKeyDown={(e) => onKey(e, i)}
                                    onClick={() => { if (!other && !p.locked) p.onOpen(c.id); }}
                                >
                                    {(two || one) && <span className="tm-name" aria-hidden="true">{label(c)}</span>}
                                    {two && <span className="tm-size mono" aria-hidden="true">{formatBytes(c.size)}</span>}
                                </button>
                            );
                        })}
                    </div>
                )}
            </div>

            <div className="tm-actions" role="group" aria-label={t("tmap.actions")}>
                {selected && selected.kind === "item" && byId.get(selected.id) && (
                    <>
                        <span className="tm-selected">{t("tmap.selected", {name: selected.name, size: formatBytes(selected.size)})}</span>
                        <button className="sm" disabled={p.locked} onClick={() => p.onOpen(selected.id)}><Icon name="chevron" size={14}/> {t("tmap.open")}</button>
                        <button className="sm" onClick={() => p.onReveal(selected.id)}><Icon name="external" size={14}/> {t("tmap.reveal")}</button>
                        <span className="muted small">{t("tmap.noRecycle")}</span>
                    </>
                )}
                {selected && selected.kind === "other" && <span className="muted small">{t("tmap.other.tip", {n: selected.merged.length, size: formatBytes(selected.size)})}</span>}
                {!selected && <span className="muted small">{t("tmap.keys")}</span>}
            </div>

            <button className="sm ghost tm-toggle" aria-expanded={table} aria-controls={`${id}-table`} onClick={() => setTable(!table)}>
                {table ? t("tmap.table.hide") : t("tmap.table.show")}
            </button>
            {table && (
                <div className="tablewrap tm-table" id={`${id}-table`}>
                    <table>
                        <caption className="sr-only">{t("tmap.table.caption", {folder: folderName(p.root)})}</caption>
                        <thead>
                            <tr>
                                <th scope="col">{t("tmap.table.name")}</th>
                                <th scope="col" className="num">{t("tmap.table.size")}</th>
                                <th scope="col" className="num">{t("tmap.table.share")}</th>
                                <th scope="col" className="num">{t("tmap.table.files")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((e) => {
                                const f = byId.get(e.id);
                                return (
                                    <tr key={e.id}>
                                        <th scope="row" className="fname" title={e.id}>{e.name}</th>
                                        <td className="num mono">{formatBytes(e.size)}</td>
                                        <td className="num mono">{pctText(e.size / layout.total)}</td>
                                        <td className="num">{f ? fileCount(f.files) : ""}</td>
                                    </tr>
                                );
                            })}
                            {sorted.length > rows.length && (
                                <tr><td colSpan={4} className="muted">{t("tmap.table.more", {n: sorted.length - rows.length})}</td></tr>
                            )}
                        </tbody>
                    </table>
                </div>
            )}
        </figure>
    );
}
