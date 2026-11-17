import {errText, on} from "../api";
import {t, type Key} from "./i18n";
import type {HealthResult} from "./health";
import {startTask} from "./taskStore";

/**
 * Runs a PC health job (the scan or the update check) as a task of the "health" kind, so the page finds it
 * running, with the stage it is at, after it was left and shown again. The result and the error go back
 * to the caller untouched.
 */
export async function trackHealth(job: "scan" | "updates", call: () => Promise<HealthResult>): Promise<HealthResult> {
    const handle = startTask("health", t(job === "scan" ? "tasks.health.scan" : "tasks.health.updates"), {tag: job});
    const off = on.scan((e) => { if (e.scan === "health") handle.update({live: t(`scan.health.${e.step}` as Key)}); });
    try {
        const r = await call();
        handle.done(job === "scan"
            ? t("tasks.health.sum.scan", {ok: r.ok, total: r.total})
            : t("tasks.health.sum.updates", {n: r.updates?.updates.length ?? 0}));
        return r;
    } catch (e) {
        handle.fail(errText(e));
        throw e;
    } finally { off(); }
}
