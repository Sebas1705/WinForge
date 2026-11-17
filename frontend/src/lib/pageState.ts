import {useCallback, useState} from "react";

/**
 * State that outlives the page showing it. Leaving a page unmounts its components, which used to throw away
 * what a long job had produced (the scan list, the ticked boxes, a result line). A page keeps those here
 * instead, under a key of its own, so coming back shows them as they were, and a job that finishes while the
 * page is away still fills them in. It lasts until the app closes; nothing is saved to disk.
 */
const store = new Map<string, unknown>();

type Setter<T> = (value: T | ((previous: T) => T)) => void;

/** Like useState, but the value is kept under `key` for the whole session. Use a key unique to the piece of state. */
export function usePageState<T>(key: string, initial: T | (() => T)): [T, Setter<T>] {
    const [value, setValue] = useState<T>(() => (store.has(key) ? (store.get(key) as T) : typeof initial === "function" ? (initial as () => T)() : initial));
    const set = useCallback<Setter<T>>((next) => {
        // Read the kept value, not this render's: the component may have been unmounted and mounted again meanwhile.
        const current = store.has(key) ? (store.get(key) as T) : value;
        const resolved = typeof next === "function" ? (next as (p: T) => T)(current) : next;
        store.set(key, resolved);
        setValue(resolved);
    }, [key, value]);
    return [value, set];
}

/** Forgets every kept value (used by tests, and when the person asks to start over). */
export function resetPageState(): void { store.clear(); }

/** Forgets the kept values whose key starts with `prefix`. */
export function clearPageState(prefix: string): void {
    for (const k of [...store.keys()]) if (k.startsWith(prefix)) store.delete(k);
}
