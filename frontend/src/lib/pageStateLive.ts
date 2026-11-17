import {useCallback, useEffect} from "react";
import {usePageState} from "./pageState";

type Setter<T> = (value: T | ((previous: T) => T)) => void;

/** The setter of the page instance shown now, per key. An instance that was left keeps working, but only for the kept value. */
const shown = new Map<string, Setter<never>>();

/**
 * usePageState for a value that a job fills in. The setter it returns always reaches the instance of the page
 * that is on screen now, so a job started before the page was left (and still running when it is shown again)
 * updates what the person sees, and one that ends while the page is away still lands in the kept value.
 * Call the returned setter from async code; for ordinary events use usePageState.
 */
export function usePageStateLive<T>(key: string, initial: T | (() => T)): [T, Setter<T>] {
    const [value, set] = usePageState<T>(key, initial);
    useEffect(() => { shown.set(key, set as Setter<never>); });
    const live = useCallback<Setter<T>>((next) => { ((shown.get(key) as Setter<T> | undefined) ?? set)(next); }, [key, set]);
    return [value, live];
}
