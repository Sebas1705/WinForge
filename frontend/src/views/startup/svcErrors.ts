import {errText} from "../../api";
import {t} from "../../lib/i18n";

export interface SvcError {
    text: string;
    /** The list should be read again: the item is gone, protected or not as it was. */
    refresh: boolean;
    /** Go wants the person to confirm first (the page normally asks before it gets here). */
    ask: boolean;
}

/**
 * Go answers the Services and Scheduled tasks tabs with codes ("svc:needs-admin", "task:protected"...):
 * this turns them into sentences. Anything else is shown as it came.
 */
export function svcError(e: unknown): SvcError {
    const m = errText(e);
    const has = (code: string) => m.includes(`:${code}`);
    if (has("needs-admin")) return {text: t("svc.error.needsAdmin"), refresh: false, ask: false};
    if (has("not-found") || has("not-listed")) return {text: t("svc.error.gone"), refresh: true, ask: false};
    if (has("protected")) return {text: t("svc.error.protected"), refresh: true, ask: false};
    if (has("no-original")) return {text: t("svc.error.noOriginal"), refresh: true, ask: false};
    if (has("store-failed")) return {text: t("svc.error.store"), refresh: false, ask: false};
    if (has("not-applied")) return {text: t("svc.error.notApplied"), refresh: true, ask: false};
    if (has("bad-mode")) return {text: t("svc.error.badMode"), refresh: false, ask: false};
    if (has("needs-confirm") || has("has-dependents")) return {text: "", refresh: false, ask: true};
    const failed = m.indexOf(":failed:");
    if (failed >= 0) return {text: t("svc.error.failed", {detail: m.slice(failed + ":failed:".length).trim() || "?"}), refresh: false, ask: false};
    return {text: m, refresh: false, ask: false};
}

/** The id of a state element, safe for aria-describedby. */
export const domId = (prefix: string, id: string) => `${prefix}-${id.replace(/[^a-zA-Z0-9]/g, "_")}`;
