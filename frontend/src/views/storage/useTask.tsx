import {useCallback, useEffect, useRef, useState} from "react";
import {api, errText, on, type StorageProgress} from "../../api";
import {formatBytes} from "../../lib/format";
import {t, type Key} from "../../lib/i18n";
import {fileCount} from "../../lib/storage";

export type TaskName = StorageProgress["task"];

export interface Outcome<T> { value?: T; error?: unknown; stopped: boolean }

export interface TaskRunner {
    /** The task running now, or null. Only one runs at a time: the Go side has a single cancelable job. */
    busy: TaskName | null;
    progress: StorageProgress | null;
    /** Runs fn as `task`. Returns null (and does nothing) when another task is already running. */
    run: <T>(task: TaskName, fn: () => Promise<T>) => Promise<Outcome<T> | null>;
    cancel: () => void;
}

/**
 * One long job at a time for the whole Storage page, with the live counters the Go side
 * reports and a way to stop it. `stopped` tells the caller the person pressed Cancel,
 * so a cancellation error is not shown as a failure.
 */
export function useTask(say: (m: string) => void): TaskRunner {
    const [busy, setBusy] = useState<TaskName | null>(null);
    const [progress, setProgress] = useState<StorageProgress | null>(null);
    const running = useRef<TaskName | null>(null);
    const stopped = useRef(false);

    useEffect(() => on.storageProgress((e) => { if (running.current === e.task) setProgress(e); }), []);

    const run = useCallback(async <T,>(task: TaskName, fn: () => Promise<T>): Promise<Outcome<T> | null> => {
        if (running.current) return null;
        running.current = task;
        stopped.current = false;
        setProgress(null);
        setBusy(task);
        try {
            return {value: await fn(), stopped: stopped.current};
        } catch (error) {
            return {error, stopped: stopped.current};
        } finally {
            running.current = null;
            setBusy(null);
            setProgress(null);
        }
    }, []);

    const cancel = useCallback(() => {
        stopped.current = true;
        api.StorageCancel().catch((e) => say(errText(e)));
    }, [say]);

    return {busy, progress, run, cancel};
}

/** Live status of the running task: counters, the item being looked at, and Cancel. */
export function TaskStatus(p: { task: TaskRunner; names: TaskName[] }) {
    const {busy, progress} = p.task;
    if (!busy || !p.names.includes(busy)) return null;
    const label = t(`storage.progress.${busy}` as Key);
    return (
        <div className="taskstatus" role="status" aria-live="polite">
            <div className="grow">
                <b>{label}</b>
                <progress aria-label={label}/>
                <span className="muted small">
                    {progress ? t("storage.progress.counts", {files: fileCount(progress.files), size: formatBytes(progress.bytes)}) : " "}
                </span>
                {progress?.current && <span className="mono muted small current" title={progress.current}>{progress.current}</span>}
            </div>
            <button className="sm" aria-label={t("storage.progress.cancelLabel", {task: label})} onClick={p.task.cancel}>{t("storage.progress.cancel")}</button>
        </div>
    );
}
