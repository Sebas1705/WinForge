import {useEffect, useState} from "react";

/**
 * A small store of the long jobs started from the app's pages, kept outside React so it survives
 * leaving a page and coming back: the job keeps running in the background, and the page finds it
 * (live progress, then its result) when it is shown again. It is the data behind every TaskViewer.
 */
export type TaskKind = "storage" | "uninstall" | "health" | "privacy" | "developer" | "startup" | "profiles" | "backup" | "updates" | "install" | "games";

export type TaskState = "running" | "done" | "failed" | "cancelled";

export interface TrackedTask {
    id: number;
    kind: TaskKind;
    title: string;
    state: TaskState;
    startedAt: number;
    endedAt?: number;
    /** What is happening now (the item being looked at), or short counters. Only while running. */
    live?: string;
    /** Overall progress from 0 to 1 when known; absent means indeterminate. */
    fraction?: number;
    /** How it ended: what was freed, removed, found. Set when it is done. */
    summary?: string;
    error?: string;
    /** Which job of the kind this is (for example "scan"), for the page that started it to recognise it again. */
    tag?: string;
    /** Stops the job, if it can be stopped. */
    cancel?: () => void;
}

export interface TaskHandle {
    readonly id: number;
    update: (patch: Partial<Pick<TrackedTask, "live" | "fraction" | "title">>) => void;
    done: (summary?: string) => void;
    fail: (error: string) => void;
    cancelled: () => void;
}

/** Finished tasks kept per kind. */
export const HISTORY_LIMIT = 8;

let tasks: TrackedTask[] = [];
let nextId = 1;
const listeners = new Set<() => void>();
const emit = () => { tasks = [...tasks]; listeners.forEach((l) => l()); };

const patchTask = (id: number, f: (t: TrackedTask) => TrackedTask) => {
    const i = tasks.findIndex((x) => x.id === id);
    if (i < 0 || tasks[i].state !== "running") return;
    tasks[i] = f(tasks[i]);
    emit();
};

const trim = (kind: TaskKind) => {
    const finished = tasks.filter((x) => x.kind === kind && x.state !== "running");
    const drop = new Set(finished.slice(0, Math.max(0, finished.length - HISTORY_LIMIT)).map((x) => x.id));
    if (drop.size) tasks = tasks.filter((x) => !drop.has(x.id));
};

/** Registers a running job. Call done, fail or cancelled exactly once when it ends. */
export function startTask(kind: TaskKind, title: string, opts: { cancel?: () => void; tag?: string } = {}): TaskHandle {
    const id = nextId++;
    tasks = [...tasks, {id, kind, title, state: "running", startedAt: Date.now(), cancel: opts.cancel, tag: opts.tag}];
    emit();
    const end = (state: TaskState, extra: Partial<TrackedTask>) => patchTask(id, (x) => {
        const next = {...x, ...extra, state, endedAt: Date.now(), live: undefined, fraction: undefined, cancel: undefined};
        queueMicrotask(() => { trim(kind); emit(); });
        return next;
    });
    return {
        id,
        update: (patch) => patchTask(id, (x) => ({...x, ...patch})),
        done: (summary) => end("done", {summary}),
        fail: (error) => end("failed", {error}),
        cancelled: () => end("cancelled", {}),
    };
}

export const subscribeTasks = (l: () => void): (() => void) => { listeners.add(l); return () => { listeners.delete(l); }; };
export const getTasks = (): TrackedTask[] => tasks;

/** Every task of a kind, oldest first. */
export const tasksOf = (all: TrackedTask[], kind: TaskKind): TrackedTask[] => all.filter((x) => x.kind === kind);

/**
 * The tasks of a kind. It subscribes with an effect and plain state, not useSyncExternalStore: that one
 * re-renders synchronously, so a page that reacts to "no job running" (to read what it has not loaded)
 * would run before the state its own await continuation sets, and ask for the same thing twice.
 */
export function useTasks(kind: TaskKind): TrackedTask[] {
    const [all, setAll] = useState(getTasks);
    useEffect(() => {
        setAll(getTasks());
        return subscribeTasks(() => setAll(getTasks()));
    }, []);
    return all.filter((x) => x.kind === kind);
}

/** The running task of a kind, if any. */
export const runningOf = (list: TrackedTask[]): TrackedTask | undefined => list.find((x) => x.state === "running");

/** Forgets the finished tasks of a kind (the Clear button of the viewer). */
export function clearFinished(kind: TaskKind): void {
    tasks = tasks.filter((x) => x.kind !== kind || x.state === "running");
    emit();
}

/** Test helper: empties the store. */
export function resetTasks(): void { tasks = []; nextId = 1; emit(); }
