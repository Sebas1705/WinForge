import {useEffect, useId, useMemo, useRef, useState} from "react";
import {api, on, type KitCreatePlan, type KitCreateResult, type KitListing, type KitProgress} from "../../api";
import {Icon} from "../../components/Icon";
import {ConfirmDialog} from "../../components/Modals";
import {formatBytes, formatDate, percent} from "../../lib/format";
import {getLang, t} from "../../lib/i18n";
import {getTasks, runningOf, startTask, tasksOf, useTasks, type TaskHandle, type TrackedTask} from "../../lib/taskStore";
import {groupFingerprint, installable, kitError, kitNeedsReopen, notIncludedText, problemText, stateKey, unsupportedText} from "../../lib/kit";
import "../../kit.css";

/** The panels are plain sections: the page behind them stays reachable, and focus moves to the panel when it opens. */
function useFocusOnOpen() {
    const ref = useRef<HTMLElement>(null);
    useEffect(() => { ref.current?.focus(); }, []);
    return ref;
}

/** Listens to the kit's progress events while a task of that kind runs. */
function useKitProgress(task: KitProgress["task"], active: boolean): KitProgress | null {
    const [progress, setProgress] = useState<KitProgress | null>(null);
    useEffect(() => {
        if (!active) { setProgress(null); return; }
        return on.kitProgress((p) => { if (p.task === task) setProgress(p); });
    }, [task, active]);
    return progress;
}

/** The kit job running now, if any. The Go side runs one kit task at a time, so a second start is refused here too. */
const kitJob = (list: TrackedTask[]) => runningOf(list.filter((x) => x.tag === "kit-create" || x.tag === "kit-open"));

/**
 * Registers a kit job in the task store and mirrors the Go side's progress events into it, so the Tasks list
 * keeps showing it when the panel is left. Returns null (and tells the person why) when another kit job runs.
 */
function beginKit(tag: "kit-create" | "kit-open", title: string, say: (m: string) => void): { job: TaskHandle; off: () => void; stopped: () => boolean } | null {
    const now = kitJob(tasksOf(getTasks(), "profiles"));
    if (now) { say(t("tasks.busy", {task: now.title})); return null; }
    let stop = false;
    const job = startTask("profiles", title, {tag, cancel: () => { stop = true; api.KitCancel().catch((e) => say(kitError(e))); }});
    const task = tag === "kit-create" ? "create" : "verify";
    const off = on.kitProgress((p) => {
        if (p.task !== task) return;
        job.update({
            live: p.name ? t("tasks.pf.kitLive", {name: p.name, done: Math.min(p.done + 1, p.total), total: p.total}) : undefined,
            fraction: p.total > 0 ? Math.min(1, (p.done + (p.percent ?? 0) / 100) / p.total) : undefined,
        });
    });
    return {job, off, stopped: () => stop};
}

const appsLine = (n: number): string => t(n === 1 ? "kit.create.apps.one" : "kit.create.apps.other", {n});

/**
 * Creates an offline kit from a profile's apps: pick a folder, see what would be downloaded and whether the drive
 * has room, confirm, watch it download (and stop it if needed). Failures are reported per app and the kit is
 * usable with the rest.
 */
export function KitCreate(p: { profileName: string; appIds: string[]; say: (m: string) => void; onClose: () => void; onOpenKit: (folder: string) => void }) {
    const panel = useFocusOnOpen();
    const titleId = useId();
    const [folder, setFolder] = useState("");
    const [plan, setPlan] = useState<KitCreatePlan | null>(null);
    const [checking, setChecking] = useState(false);
    const [asking, setAsking] = useState(false);
    const kit = kitJob(useTasks("profiles"));
    const running = kit?.tag === "kit-create";
    const [result, setResult] = useState<KitCreateResult | null>(null);
    const progress = useKitProgress("create", running);

    const choose = async () => {
        try {
            const dir = await api.PickFolder();
            if (!dir) return;
            setFolder(dir);
            setResult(null);
            setChecking(true);
            setPlan(await api.KitPrepare(p.appIds, dir));
        } catch (e) { p.say(kitError(e)); } finally { setChecking(false); }
    };

    const create = async () => {
        if (!plan) return;
        const started = beginKit("kit-create", t("tasks.pf.kitCreate", {name: p.profileName}), p.say);
        if (!started) return;
        setResult(null);
        try {
            const r = await api.KitCreate(p.appIds, folder, p.profileName);
            if (r.cancelled) started.job.cancelled();
            else started.job.done(r.added.length > 0 ? t("kit.create.done", {n: r.totalApps, size: formatBytes(r.bytes)}) : t("kit.create.doneNone"));
            setResult(r);
            setPlan(await api.KitPrepare(p.appIds, folder).catch(() => plan));
        } catch (e) { if (started.stopped()) started.job.cancelled(); else started.job.fail(kitError(e)); p.say(kitError(e)); } finally { started.off(); }
    };

    const canStart = !!plan && !plan.problem && plan.apps.length > 0 && !kit && !checking;
    const pct = progress && progress.total > 0 ? percent(progress.done * 100 + (progress.percent ?? 0), progress.total * 100) : 0;
    const label = !progress ? t("kit.create.preparing")
        : progress.percent ? t("kit.create.runningPct", {name: progress.name ?? "", done: Math.min(progress.done + 1, progress.total), total: progress.total, pct: progress.percent})
        : t("kit.create.running", {name: progress.name ?? "", done: Math.min(progress.done + 1, progress.total), total: progress.total});

    return (
        <section className="kitcard" ref={panel} tabIndex={-1} aria-labelledby={titleId}>
            <div className="kithead">
                <div>
                    <h2 id={titleId}>{t("kit.create.title")}</h2>
                    <p className="muted">{t("kit.create.intro")}</p>
                </div>
                <button className="ghost" disabled={running} onClick={p.onClose}>{t("kit.create.close")}</button>
            </div>

            <div className="kitfolder">
                <span className="mono"><span className="sr-only">{t("kit.create.folder")}: </span>{folder || t("kit.create.noFolder")}</span>
                <button disabled={!!kit || checking} onClick={() => void choose()}>
                    <Icon name="archive" size={15}/> {folder ? t("kit.create.change") : t("kit.create.pick")}
                </button>
            </div>

            {checking && <p className="muted" role="status">{t("kit.create.checking")}</p>}

            {plan && !checking && (
                <div className="kitstack" role="status" aria-live="polite">
                    {plan.problem ? (
                        <p className="kitnote bad"><Icon name="alert" size={15}/> <span>{plan.problem === "badkit" ? t("kit.create.problem.badkit") : t("kit.create.problem.folder")}</span></p>
                    ) : (
                        <>
                            <p>{appsLine(plan.apps.length)}</p>
                            <p className="muted small">
                                {plan.freeKnown ? t("kit.create.space", {need: formatBytes(plan.needBytes), free: formatBytes(plan.freeBytes)})
                                    : t("kit.create.spaceUnknown", {need: formatBytes(plan.needBytes)})}
                            </p>
                            {!plan.enough && <p className="kitnote warn"><Icon name="alert" size={15}/> <span>{t("kit.create.notEnough")}</span></p>}
                            {plan.existingKit && <p className="kitnote"><Icon name="info" size={15}/> <span>{t("kit.create.existing")}</span></p>}
                        </>
                    )}
                    {plan.notIncluded.length > 0 && !result && (
                        <NotIncludedList title={t("kit.create.notIncluded")} items={plan.notIncluded}/>
                    )}
                </div>
            )}

            {running && (
                <div className="kitprogress" role="status" aria-live="polite">
                    <b>{label}</b>
                    <progress value={pct} max={100} aria-label={label}/>
                    <div className="kitbar">
                        <button className="sm" aria-label={t("kit.create.stopLabel")} onClick={() => { api.KitCancel().catch((e) => p.say(kitError(e))); }}>{t("kit.create.stop")}</button>
                    </div>
                </div>
            )}

            {result && !running && (
                <div className="kitstack" role="status" aria-live="polite">
                    <p><Icon name="check" size={15}/> {result.cancelled ? t("kit.create.stopped") : result.added.length > 0 ? t("kit.create.done", {n: result.totalApps, size: formatBytes(result.bytes)}) : t("kit.create.doneNone")}</p>
                    {result.failed.length > 0 && (
                        <>
                            <NotIncludedList title={t("kit.create.failed")} items={result.failed}/>
                            {result.totalApps > 0 && <p className="muted small">{t("kit.create.usable")}</p>}
                        </>
                    )}
                    <div>
                        <h3>{t("kit.create.fingerprint")}</h3>
                        <p className="kitprint">{groupFingerprint(result.fingerprint)}</p>
                        <p className="muted small">{t("kit.create.fingerprintHint")}</p>
                    </div>
                    {result.totalApps > 0 && <div className="kitbar"><button className="sm" onClick={() => p.onOpenKit(result.folder)}>{t("kit.create.open")}</button></div>}
                </div>
            )}

            {!running && (
                <div className="kitbar">
                    <button className="primary" disabled={!canStart} onClick={() => setAsking(true)}><Icon name="download" size={15}/> {t("kit.create.start")}</button>
                </div>
            )}

            {asking && plan && (
                <ConfirmDialog title={t("kit.create.confirmTitle")} confirm={t("kit.create.start")} onClose={() => setAsking(false)} onConfirm={() => void create()}
                               body={t("kit.create.confirmBody", {n: plan.apps.length, folder, need: formatBytes(plan.needBytes)})}/>
            )}
        </section>
    );
}

function NotIncludedList(p: { title: string; items: KitCreatePlan["notIncluded"] }) {
    return (
        <div>
            <h3>{p.title}</h3>
            <ul className="plain" aria-label={p.title}>
                {p.items.map((n) => (
                    <li key={n.id} className="kitnote warn">
                        <Icon name="alert" size={15}/>
                        <span><b>{n.name}</b>: {notIncludedText(n)}</span>
                    </li>
                ))}
            </ul>
        </div>
    );
}

type Phase = "idle" | "checking" | "open";

/**
 * Installs from an offline kit: pick the folder, WinForge verifies every file and shows the list with what is
 * wrong with each app (nothing runs yet), then asks before installing the ticked ones through the usual run window.
 */
export function KitInstall(p: {
    admin: boolean; say: (m: string) => void; onClose: () => void; onAdmin: () => void;
    /** Starts the run window for these app ids. */
    onInstall: (ids: string[]) => void;
    /** Bumped when a run from a kit finished, so the "already installed" marks are read again. */
    refreshKey: number;
    /** A folder to open at once (the kit that was just created). */
    initialFolder?: string;
}) {
    const panel = useFocusOnOpen();
    const titleId = useId();
    const [phase, setPhase] = useState<Phase>("idle");
    const kit = kitJob(useTasks("profiles"));
    const verifying = phase === "checking" || kit?.tag === "kit-open";
    const [listing, setListing] = useState<KitListing | null>(null);
    const [marked, setMarked] = useState<Set<string>>(new Set());
    const [asking, setAsking] = useState(false);
    const progress = useKitProgress("verify", verifying);
    const opening = useRef(0); // which open the screen waits for; an older answer is ignored

    const open = async (folder: string) => {
        const started = beginKit("kit-open", t("tasks.pf.kitOpen", {folder}), p.say);
        if (!started) return;
        const mine = ++opening.current;
        setPhase("checking");
        setListing(null);
        setMarked(new Set());
        try {
            const l = await api.KitOpen(folder);
            started.job.done(t("tasks.pf.kitOpen.done", {n: l.apps.length}));
            if (mine !== opening.current) return;
            setListing(l);
            setPhase("open");
        } catch (e) {
            if (started.stopped()) started.job.cancelled(); else started.job.fail(kitError(e));
            if (mine !== opening.current) return;
            p.say(kitError(e));
            setPhase("idle");
        } finally { started.off(); }
    };
    const choose = async () => {
        try {
            const dir = await api.PickFolder();
            if (dir) await open(dir);
        } catch (e) { p.say(kitError(e)); }
    };
    useEffect(() => { if (p.initialFolder) void open(p.initialFolder); }, [p.initialFolder]);

    // After a run: which apps are installed now.
    useEffect(() => {
        if (p.refreshKey === 0 || !listing) return;
        const ids = listing.apps.filter((a) => a.state === "ready").map((a) => a.id);
        if (ids.length === 0) return;
        let live = true;
        api.KitVerify(ids).then((v) => {
            if (!live) return;
            const have = new Set(v.installed.map((x) => x.id));
            setListing((l) => (l ? {...l, apps: l.apps.map((a) => (a.state === "ready" ? {...a, installed: have.has(a.id)} : a))} : l));
            setMarked((m) => new Set([...m].filter((id) => !have.has(id))));
        }).catch((e) => { if (kitNeedsReopen(e)) setListing(null); });
        return () => { live = false; };
    }, [p.refreshKey]);

    const apps = listing?.apps ?? [];
    const canTick = useMemo(() => apps.filter(installable), [apps]);
    const chosen = canTick.filter((a) => marked.has(a.id));
    const needAdmin = chosen.filter((a) => a.admin).length;
    const toggle = (id: string) => setMarked((m) => { const n = new Set(m); if (n.has(id)) n.delete(id); else n.add(id); return n; });

    const pct = progress && progress.total > 0 ? percent(progress.done, progress.total) : 0;
    const checkingLabel = progress && progress.name ? t("kit.open.checking", {name: progress.name, done: Math.min(progress.done + 1, progress.total), total: progress.total}) : t("kit.open.checkingStart");

    return (
        <section className="kitcard" ref={panel} tabIndex={-1} aria-labelledby={titleId}>
            <div className="kithead">
                <div>
                    <h2 id={titleId}>{t("kit.open.title")}</h2>
                    <p className="muted">{t("kit.open.intro")}</p>
                </div>
                <button className="ghost" disabled={verifying} onClick={p.onClose}>{t("kit.open.close")}</button>
            </div>

            <div className="kitbar">
                <button disabled={verifying || !!kit} onClick={() => void choose()}>
                    <Icon name="archive" size={15}/> {listing ? t("kit.open.again") : t("kit.open.pick")}
                </button>
            </div>

            {verifying && (
                <div className="kitprogress" role="status" aria-live="polite">
                    <b>{checkingLabel}</b>
                    <progress value={pct} max={100} aria-label={checkingLabel}/>
                    <div className="kitbar">
                        <button className="sm" aria-label={t("kit.open.stopLabel")} onClick={() => { api.KitCancel().catch((e) => p.say(kitError(e))); }}>{t("kit.open.stop")}</button>
                    </div>
                </div>
            )}

            {listing && phase === "open" && (
                <>
                    <div className="kitstack tight">
                        <p className="muted small">
                            {t("kit.open.created", {
                                date: formatDate(listing.created, getLang()) || listing.created, version: listing.winforge || "?",
                                profile: listing.profile ? t("kit.open.profile", {name: listing.profile}) : "",
                            })}
                        </p>
                        <p className="muted small">{t("kit.open.fingerprint")}: <span className="kitprint">{groupFingerprint(listing.fingerprint)}</span></p>
                        <p className="muted small">{t("kit.open.verified")}</p>
                    </div>
                    {listing.unlisted.length > 0 && <p className="kitnote warn"><Icon name="alert" size={15}/> <span>{t("kit.open.unlisted", {names: listing.unlisted.join(", ")})}</span></p>}

                    {apps.length === 0 && <p className="muted" role="status">{t("kit.open.empty")}</p>}
                    {apps.length > 0 && canTick.length === 0 && <p className="muted" role="status">{t("kit.open.nothingToInstall")}</p>}

                    {apps.length > 0 && (
                        <>
                            <div className="kitbar">
                                <span className="muted" role="status">{t("kit.open.summary", {n: chosen.length, total: canTick.length})}</span>
                                <span className="grow"/>
                                <button className="sm" disabled={canTick.length === 0 || chosen.length === canTick.length} onClick={() => setMarked(new Set(canTick.map((a) => a.id)))}>{t("kit.open.selectAll")}</button>
                                <button className="sm" disabled={chosen.length === 0} onClick={() => setMarked(new Set())}>{t("kit.open.clear")}</button>
                            </div>
                            {!p.admin && needAdmin > 0 && (
                                <p className="muted small hint adminhint"><Icon name="lock" size={14}/> <span>{t("kit.adminHint", {n: needAdmin})}</span>
                                    <button className="mini" onClick={p.onAdmin}>{t("kit.restartAdmin")}</button></p>
                            )}
                            <ul className="kitlist" aria-label={t("kit.open.list")}>
                                {apps.map((a) => {
                                    const can = installable(a);
                                    const meta = [a.version ? t("kit.open.version", {v: a.version}) : "", a.type ? t("kit.open.type", {type: a.type}) : "", formatBytes(a.sizeBytes),
                                        a.installed && a.installedVersion ? t("kit.open.installedVersion", {v: a.installedVersion}) : ""].filter(Boolean).join(" · ");
                                    const body = (
                                        <>
                                            {can && <input type="checkbox" checked={marked.has(a.id)} onChange={() => toggle(a.id)} aria-label={t("kit.open.select", {name: a.name})}/>}
                                            <span className="body">
                                                <span className="name">
                                                    <b>{a.name}</b>
                                                    <span className={"chip kitstate " + (a.state === "problem" ? "bad" : a.state === "ready" && !a.installed ? "ok" : "")}>
                                                        {a.state === "problem" ? <Icon name="alert" size={12}/> : a.state === "ready" && !a.installed ? <Icon name="check" size={12}/> : null} {t(stateKey(a))}
                                                    </span>
                                                    {a.admin && a.state === "ready" && <span className="chip admin" title={t("kit.badge.adminTip")}><Icon name="lock" size={12}/> {t("kit.badge.admin")}</span>}
                                                </span>
                                                <small className="muted">{meta}</small>
                                                {a.state === "unsupported" && <small>{unsupportedText(a)}</small>}
                                                {a.state === "problem" && (
                                                    <ul className="kitproblems" aria-label={a.name}>
                                                        {a.problems.map((pr, i) => <li key={i}>{problemText(pr)}</li>)}
                                                    </ul>
                                                )}
                                            </span>
                                        </>
                                    );
                                    return <li key={a.id}>{can ? <label className={"kitrow" + (marked.has(a.id) ? " on" : "")}>{body}</label> : <div className="kitrow">{body}</div>}</li>;
                                })}
                            </ul>
                            <div className="kitbar">
                                <span className="grow" aria-hidden="true"/>
                                <button className="primary" disabled={chosen.length === 0} onClick={() => setAsking(true)}>
                                    <Icon name="download" size={15}/> {chosen.length > 0 ? t("kit.install.button.n", {n: chosen.length}) : t("kit.install.button")}
                                </button>
                            </div>
                        </>
                    )}

                    {listing.notIncluded.length > 0 && (
                        <div>
                            <h3>{t("kit.open.notIncluded")}</h3>
                            <p className="muted small">{t("kit.open.notIncludedHint")}</p>
                            <ul className="plain" aria-label={t("kit.open.notIncluded")}>
                                {listing.notIncluded.map((n) => (
                                    <li key={n.id} className="kitnote"><Icon name="info" size={15}/><span><b>{n.name}</b>: {notIncludedText(n)}</span></li>
                                ))}
                            </ul>
                        </div>
                    )}
                </>
            )}

            {asking && listing && (
                <ConfirmDialog title={t("kit.install.confirmTitle")} confirm={t("kit.install.confirm")} onClose={() => setAsking(false)}
                               body={t("kit.install.confirmBody", {n: chosen.length, folder: listing.folder})}
                               onConfirm={() => p.onInstall(chosen.map((a) => a.id))}>
                    {!p.admin && needAdmin > 0 && <p className="muted small">{t("kit.install.confirmAdmin", {n: needAdmin})}</p>}
                </ConfirmDialog>
            )}
        </section>
    );
}
