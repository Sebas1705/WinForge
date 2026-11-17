import {useCallback} from "react";
import {api, errText, on, type StorageProgress} from "../../api";
import {formatBytes} from "../../lib/format";
import {t, type Key} from "../../lib/i18n";
import {fileCount} from "../../lib/storage";
import {getTasks, runningOf, startTask, tasksOf, useTasks, type TrackedTask} from "../../lib/taskStore";

export type TaskName = StorageProgress["task"];

export interface Outcome<T> { value?: T; error?: unknown; stopped: boolean }

/** What the status shows while a task runs: the counters and the item being looked at. */
export interface TaskProgress { files: number; bytes: number; current?: string }

export interface TaskRunner {
    /** The task running now, or null. Only one runs at a time: the Go side has a single cancelable job. */
    busy: TaskName | null;
    progress: TaskProgress | null;
    /**
     * Runs fn as `task`. Returns null (and does nothing) when another task is already running.
     * `summarize` turns the value into the line the Tasks list keeps once it is done.
     */
    run: <T>(task: TaskName, fn: () => Promise<T>, summarize?: (value: T) => string | undefined) => Promise<Outcome<T> | null>;
    cancel: () => void;
}

/** Only one storage job runs at a time, whichever page started it, so this is global, not per mount. */
let stopRequested = false;
const live = new Map<number, TaskProgress>();

/**
 * One long job at a time for the whole Storage page, with the live counters the Go side
 * reports and a way to stop it. The job is registered in the task store, so leaving the page
 * and coming back still shows it running (and then how it ended). `stopped` tells the caller
 * the person pressed Cancel, so a cancellation error is not shown as a failure.
 */
export function useTask(say: (m: string) => void): TaskRunner {
    const list = useTasks("storage");
    const running: TrackedTask | undefined = runningOf(list);
    const busy = (running?.tag as TaskName | undefined) ?? null;
    const progress = running ? live.get(running.id) ?? null : null;

    const run = useCallback(async <T,>(task: TaskName, fn: () => Promise<T>, summarize?: (value: T) => string | undefined): Promise<Outcome<T> | null> => {
        if (runningOf(tasksOf(getTasks(), "storage"))) return null;
        stopRequested = false;
        const handle = startTask("storage", t(`storage.progress.${task}` as Key), {
            tag: task,
            cancel: () => { stopRequested = true; api.StorageCancel().catch((e) => say(errText(e))); },
        });
        const off = on.storageProgress((e) => {
            if (e.task !== task) return;
            live.set(handle.id, e);
            handle.update({live: e.current ?? t("storage.progress.counts", {files: fileCount(e.files), size: formatBytes(e.bytes)})});
        });
        try {
            const value = await fn();
            if (stopRequested) handle.cancelled(); else handle.done(summarize?.(value));
            return {value, stopped: stopRequested};
        } catch (error) {
            if (stopRequested) handle.cancelled(); else handle.fail(errText(error));
            return {error, stopped: stopRequested};
        } finally {
            off();
            live.delete(handle.id);
        }
    }, [say]);

    const cancel = useCallback(() => running?.cancel?.(), [running]);

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
