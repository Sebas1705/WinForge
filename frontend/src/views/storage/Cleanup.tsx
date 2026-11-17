import {useEffect, useMemo, useRef, useState, type Ref} from "react";
import {api, errText, type CleanCategory, type CleanResult} from "../../api";
import {Icon} from "../../components/Icon";
import {ConfirmDialog} from "../../components/Modals";
import {formatBytes} from "../../lib/format";
import {usePageState} from "../../lib/pageState";
import {t} from "../../lib/i18n";
import {categoryHint, categoryName, fileCount, initialSelection, showsFileCount, visibleCategories} from "../../lib/storage";
import {TaskStatus, type TaskRunner} from "./useTask";

// The backend reports short codes for what it did not do; anything else is shown as it came.
const reason = (error: string): string =>
    error === "needs-admin" ? t("storage.clean.error.needsAdmin") : error === "unknown category" ? t("storage.clean.error.unknown") : error;

function Result(p: { r: CleanResult; ref?: Ref<HTMLDivElement> }) {
    const {r} = p;
    const problems = r.outcomes.filter((o) => o.error);
    return (
        <div className="resultline" role="status" tabIndex={-1} ref={p.ref}>
            <p>{r.freedBytes > 0 ? t("storage.clean.result", {size: formatBytes(r.freedBytes), files: fileCount(r.files)}) : t("storage.clean.resultNone")}</p>
            {r.failed > 0 && <p className="muted small">{t("storage.clean.failed", {n: r.failed})}</p>}
            {problems.length > 0 && (
                <ul className="plain small">
                    {problems.map((o) => <li key={o.id} className="muted">{t("storage.clean.errorLine", {name: categoryName(o.id), error: reason(o.error ?? "")})}</li>)}
                </ul>
            )}
        </div>
    );
}

/**
 * Scans for files that can be rebuilt, lets the person tick what to clean, asks once,
 * cleans and shows what was freed. Safe categories start ticked, "review" ones do not.
 */
export function Cleanup(p: { task: TaskRunner; admin: boolean; say: (m: string) => void; onAdmin: () => void; onCleaned: () => void }) {
    // Kept while the page is away, so the list, the ticks and the result survive leaving and coming back.
    const [cats, setCats] = usePageState<CleanCategory[] | null>("storage:cleanup:cats", null);
    const [selected, setSelected] = usePageState<Set<string>>("storage:cleanup:selected", () => new Set());
    const [result, setResult] = usePageState<CleanResult | null>("storage:cleanup:result", null);
    const [stopped, setStopped] = usePageState("storage:cleanup:stopped", false);
    const [confirming, setConfirming] = useState(false);
    const locked = p.task.busy !== null;
    // The button that opened the confirmation is disabled while cleaning, so focus would fall to the page
    // top. When it is lost, it goes to the result (or the section heading) once the work ends.
    const resultRef = useRef<HTMLDivElement>(null);
    const headingRef = useRef<HTMLHeadingElement>(null);
    const refocus = useRef(false);
    useEffect(() => {
        if (locked || !refocus.current) return;
        refocus.current = false;
        const a = document.activeElement;
        if (!a || a === document.body) (resultRef.current ?? headingRef.current)?.focus();
    }, [locked, result]);

    const shown = useMemo(() => visibleCategories(cats ?? []), [cats]);
    const chosen = shown.filter((c) => selected.has(c.id));
    const total = chosen.reduce((n, c) => n + c.bytes, 0);
    const found = shown.reduce((n, c) => n + c.bytes, 0);
    const adminOnly = chosen.filter((c) => c.needsAdmin);

    const scan = async (): Promise<void> => {
        const out = await p.task.run("scan", () => api.StorageScan(), (l) => t("storage.clean.found", {size: formatBytes(visibleCategories(l).reduce((n, c) => n + c.bytes, 0))}));
        if (!out) return;
        setStopped(out.stopped);
        if (out.error !== undefined) {
            if (!out.stopped) p.say(errText(out.error));
            return;
        }
        const list = out.value ?? [];
        setCats(list);
        setSelected(initialSelection(list));
    };

    const startScan = () => { setResult(null); void scan(); };

    const clean = async () => {
        const ids = chosen.map((c) => c.id);
        if (ids.length === 0) return;
        refocus.current = true;
        const out = await p.task.run("clean", () => api.StorageClean(ids), (r) => r.freedBytes > 0 ? t("storage.clean.result", {size: formatBytes(r.freedBytes), files: fileCount(r.files)}) : t("storage.clean.resultNone"));
        if (!out) return;
        if (out.error !== undefined) {
            if (!out.stopped) p.say(errText(out.error));
            return;
        }
        if (out.value) setResult(out.value);
        p.onCleaned();
        await scan(); // what is left, with the sizes as they are now
    };

    const toggle = (id: string) => setSelected((s) => { const n = new Set(s); if (n.has(id)) n.delete(id); else n.add(id); return n; });

    return (
        <section aria-labelledby="storage-clean">
            <h2 id="storage-clean" tabIndex={-1} ref={headingRef}>{t("storage.clean.title")}</h2>
            <p className="muted small">{t("storage.clean.intro")}</p>
            <div className="toolrow">
                <button className="primary" disabled={locked} onClick={startScan}>
                    <Icon name="search" size={15}/> {cats === null ? t("storage.clean.scan") : t("storage.clean.rescan")}
                </button>
            </div>
            <TaskStatus task={p.task} names={["scan", "clean"]}/>

            {stopped && cats !== null && <p className="muted small" role="status">{t("storage.stopped")}</p>}
            {result && <Result r={result} ref={resultRef}/>}

            {cats !== null && shown.length === 0 && <p className="muted" role="status">{t("storage.clean.nothing")}</p>}
            {shown.length > 0 && (
                <>
                    <p className="muted small">{t("storage.clean.found", {size: formatBytes(found)})}</p>
                    <ul className="cleanlist">
                        {shown.map((c) => (
                            <li key={c.id}>
                                <label className={selected.has(c.id) ? "on" : ""}>
                                    <input type="checkbox" checked={selected.has(c.id)} disabled={locked} onChange={() => toggle(c.id)}
                                           aria-label={t("storage.clean.check", {name: categoryName(c.id)})}/>
                                    <span className="grow">
                                        <span className="catname">
                                            <b>{categoryName(c.id)}</b>
                                            {c.needsAdmin && <span className="chip admin" title={t("storage.badge.adminTip")}><Icon name="lock" size={12}/> {t("storage.badge.admin")}</span>}
                                            {c.risk === "review" && <span className="chip review" title={t("storage.badge.reviewTip")}><Icon name="alert" size={12}/> {t("storage.badge.review")}</span>}
                                        </span>
                                        {categoryHint(c.id) && <small className="muted">{categoryHint(c.id)}</small>}
                                        {c.skipped > 0 && <small className="muted">{t("storage.clean.skipped", {n: c.skipped})}</small>}
                                    </span>
                                    <span className="cleansize">
                                        <b className="mono">{formatBytes(c.bytes)}</b>
                                        {showsFileCount(c.id) && <small className="muted">{fileCount(c.files)}</small>}
                                    </span>
                                </label>
                            </li>
                        ))}
                    </ul>
                    {!p.admin && adminOnly.length > 0 && (
                        <p className="muted small hint adminhint"><Icon name="lock" size={14}/> <span>{t("storage.clean.adminHint", {names: adminOnly.map((c) => categoryName(c.id)).join(", ")})}</span>
                            <button className="mini" onClick={p.onAdmin}>{t("health.restartAdmin")}</button></p>
                    )}
                    <div className="cleanbar">
                        <span className="grow" role="status">
                            {chosen.length > 0 ? t("storage.clean.selected", {n: chosen.length, size: formatBytes(total)}) : t("storage.clean.none")}
                        </span>
                        <button className="danger" disabled={locked || chosen.length === 0} onClick={() => setConfirming(true)}>
                            <Icon name="trash" size={14}/> {t("storage.clean.button")}
                        </button>
                    </div>
                </>
            )}

            {confirming && (
                <ConfirmDialog title={t("storage.clean.confirmTitle")} danger confirm={t("storage.clean.confirm")}
                               body={`${t("storage.clean.confirmBody")}${chosen.some((c) => c.id === "recycle-bin") ? ` ${t("storage.clean.confirmRecycle")}` : ""}`}
                               onConfirm={() => void clean()} onClose={() => setConfirming(false)}>
                    <ul className="confirmlist">
                        {chosen.map((c) => <li key={c.id}>{t("storage.clean.confirmItem", {name: categoryName(c.id), size: formatBytes(c.bytes)})}</li>)}
                    </ul>
                </ConfirmDialog>
            )}
        </section>
    );
}
