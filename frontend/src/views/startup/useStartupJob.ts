import {useRef} from "react";
import {runJob, useRefreshWhenOtherEnds, type JobResult} from "../../lib/runJob";
import {runningOf, useTasks} from "../../lib/taskStore";

export type StartupTab = "programs" | "services" | "tasks";

/**
 * Changes on the Startup tabs go through the task store (kind "startup"), so they show in the Tasks list and
 * still hold after leaving the page. One change runs at a time across the three tabs: `busy` is true on all of them.
 * `refresh` reads the tab's list again when a change this tab started before it was left ends.
 */
export function useStartupJob(tab: StartupTab, say: (m: string) => void, refresh: () => void) {
    const running = runningOf(useTasks("startup"));
    const own = useRef(false);
    useRefreshWhenOtherEnds(running, own, refresh, [tab]);

    /** Runs fn as a registered change; null when another change is running (a toast says so). */
    const job = async <T,>(title: string, fn: () => Promise<T>, summarize: (v: T) => string, failure: (e: unknown) => string | null): Promise<JobResult<T> | null> => {
        own.current = true;
        const r = await runJob("startup", say, {title, tag: tab, summarize, failure}, fn);
        if (!r) own.current = false;
        return r;
    };

    return {busy: running !== undefined, job};
}
