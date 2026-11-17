import {useEffect, useRef, type MutableRefObject} from "react";
import {errText} from "../api";
import {t} from "./i18n";
import {getTasks, runningOf, startTask, tasksOf, type TaskKind, type TrackedTask} from "./taskStore";

export type JobResult<T> = { ok: true; value: T } | { ok: false; error: unknown };

export interface JobOptions<T> {
    /** Already-localized title shown in the Tasks list. */
    title: string;
    /** Which job of the kind this is, so a page can recognise it after coming back. */
    tag: string;
    cancel?: () => void;
    /** The line the Tasks list keeps once the job is done. */
    summarize?: (value: T) => string | undefined;
    /** Turns an error into the failure text; return null when it is not a failure (the job is then listed as stopped). */
    failure?: (error: unknown) => string | null;
    /** Called with the handle so the caller can report progress. Returns what to run when the job ends. */
    onStart?: (update: (live: string) => void) => (() => void) | void;
}

/**
 * Runs one job of `kind` registered in the task store, so it survives leaving the page. Only one job
 * of a kind runs at a time: when another is running it says so (toast) and returns null without calling fn.
 */
export async function runJob<T>(kind: TaskKind, say: (m: string) => void, o: JobOptions<T>, fn: () => Promise<T>): Promise<JobResult<T> | null> {
    const running = runningOf(tasksOf(getTasks(), kind));
    if (running) { say(t("tasks.busy", {task: running.title})); return null; }
    const handle = startTask(kind, o.title, {tag: o.tag, cancel: o.cancel});
    const off = o.onStart?.((live) => handle.update({live}));
    try {
        const value = await fn();
        handle.done(o.summarize?.(value));
        return {ok: true, value};
    } catch (error) {
        const text = o.failure ? o.failure(error) : errText(error);
        if (text === null) handle.cancelled(); else handle.fail(text);
        return {ok: false, error};
    } finally {
        off?.();
    }
}

/**
 * Calls `refresh` (with the tag that ended) when a job, started by an earlier instance of the page (one that
 * was left while it ran), ends: the page that started it reads its own result, but a page shown again has not.
 * `own` is set by the page right before it starts a job itself, so its own result is not read twice.
 * `tags` limits it to the jobs of those tags (default: every job of the kind).
 */
export function useRefreshWhenOtherEnds(running: TrackedTask | undefined, own: MutableRefObject<boolean>, refresh: (tag: string | undefined) => void, tags?: string[]): void {
    const was = useRef<TrackedTask | undefined>(running);
    const cb = useRef(refresh);
    cb.current = refresh;
    const key = tags?.join("|");
    useEffect(() => {
        const prev = was.current;
        was.current = running;
        if (prev && !running && (!key || (prev.tag !== undefined && key.split("|").includes(prev.tag)))) {
            if (own.current) own.current = false;
            else cb.current(prev.tag);
        }
    }, [running, key, own]);
}
