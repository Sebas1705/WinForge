import type {KitApp, KitNotIncluded, KitProblem} from "../api";
import {errText} from "../api";
import {tOpt, t, type Key} from "./i18n";

/**
 * The Go side's refusals carry a stable code ("kit:nokit ..."); this says them in the person's language.
 * Anything else is shown as it came.
 */
export function kitError(e: unknown): string {
    const m = errText(e);
    const code = /kit:([a-z]+)/.exec(m)?.[1];
    return (code && tOpt(`kit.error.${code}`)) || m;
}

/** Whether a refusal means the opened kit is gone and must be opened again. */
export function kitNeedsReopen(e: unknown): boolean {
    const code = /kit:([a-z]+)/.exec(errText(e))?.[1];
    return code === "notopen" || code === "notlisted";
}

/** Why an app is not in the kit, as a sentence. */
export function notIncludedText(n: KitNotIncluded): string {
    return tOpt(`kit.not.${n.reason}`, {type: n.detail ?? "", detail: n.detail ?? ""}) ?? n.reason;
}

/** What is wrong with a file of an app, as a sentence. */
export function problemText(p: KitProblem): string {
    return tOpt(`kit.problem.${p.kind}`, {file: p.file ?? ""}) ?? p.kind;
}

/** Why an intact app cannot be installed offline. */
export function unsupportedText(a: Pick<KitApp, "reason" | "detail">): string {
    if (a.reason === "type") return t("kit.unsupported.type", {type: a.detail || "?"});
    return t("kit.unsupported.switches");
}

/** What the person can tick: verified and not already installed. */
export const installable = (a: KitApp): boolean => a.state === "ready" && !a.installed;

/** Which label an app's state gets. */
export function stateKey(a: KitApp): Key {
    if (a.state === "problem") return "kit.state.problem";
    if (a.state === "unsupported") return "kit.state.unsupported";
    return a.installed ? "kit.state.installed" : "kit.state.ready";
}

/** The fingerprint in groups of 8 characters, so two can be compared by eye. */
export const groupFingerprint = (fp: string): string => fp.replace(/(.{8})(?=.)/g, "$1 ");
