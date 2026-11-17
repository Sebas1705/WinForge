import {useEffect, useId, useMemo, useState, type ReactNode} from "react";
import {api, type CompareApp, type CompareResult, type CompareSide} from "../../api";
import {AppIcon} from "../../components/AppIcon";
import {Icon, type IconName} from "../../components/Icon";
import {TaskViewer} from "../../components/TaskViewer";
import {compareError, kindOf, LIST_LIMIT, saveName, sideLabel, summary, type CompareKind} from "../../lib/compare";
import {withApps} from "../../lib/drift";
import {clearPageState, usePageState} from "../../lib/pageState";
import {usePageStateLive} from "../../lib/pageStateLive";
import {t} from "../../lib/i18n";
import type {App, Profile, ProfileInfo} from "../../lib/model";
import {selectionProfile, uniqueProfile} from "../../lib/model";
import {runningOf, startTask, useTasks} from "../../lib/taskStore";
import "../../compare.css";

type Mode = "file" | "code" | "files";
type Request = { mode: "file"; path: string } | { mode: "code"; code: string } | { mode: "files"; a: string; b: string };

/** Which comparison the screen waits for; an older answer is ignored. Module level, so a page left and shown again keeps counting. */
let compareSeq = 0;

const fileName =(path: string) => path.split(/[\\/]/).pop() || path;

/**
 * Sets this PC against a profile file or a share code, or two profile files against each other. Reading is
 * all the comparison does; the buttons under the result go through the paths the app already has (the
 * install plan, saving a profile) and each says what it will do.
 */
export function CompareView(p: {
    apps: Map<string, App>;
    profiles: ProfileInfo[];
    /** Changes whenever the PC is scanned again, so the comparison is read again. */
    scan: unknown;
    onBack: () => void;
    onInstall: (p: Profile) => void;
    onChanged: () => Promise<void>;
    say: (m: string) => void;
}) {
    // What was compared and what came out is kept while the app runs, so leaving the page does not lose it.
    const [mode, setMode] = usePageState<Mode>("compare:mode", "file");
    const [pathA, setPathA] = usePageState("compare:pathA", "");
    const [pathB, setPathB] = usePageState("compare:pathB", "");
    const [code, setCode] = usePageState("compare:code", "");
    const [req, setReq] = usePageState<Request | null>("compare:req", null);
    const [ran, setRan] = usePageState<{ req: Request; scan: unknown } | null>("compare:ran", null);
    const [result, setResult] = usePageStateLive<CompareResult | null>("compare:result", null);
    const [error, setError] = usePageStateLive("compare:error", "");
    const [reading, setBusy] = useState(false);
    // A comparison running in the background (started before leaving this page) keeps the controls locked too.
    const compareJob = runningOf(useTasks("profiles").filter((x) => x.tag === "compare"));
    const busy = reading || !!compareJob;
    const modeId = useId();
    const codeId = useId();

    useEffect(() => {
        if (!req) return;
        // Shown again with the answer for this very request and scan already kept (or still on its way): do not ask again.
        if (ran && ran.req === req && ran.scan === p.scan) return;
        const id = ++compareSeq;
        setRan({req, scan: p.scan});
        setBusy(true);
        const what = req.mode === "file" ? fileName(req.path) : req.mode === "code" ? t("tasks.pf.compare.code") : t("tasks.pf.compare.files", {a: fileName(req.a), b: fileName(req.b)});
        const job = startTask("profiles", t("tasks.pf.compare", {what}), {tag: "compare"});
        const call = req.mode === "file" ? api.CompareFile(req.path) : req.mode === "code" ? api.CompareCode(req.code) : api.CompareFiles(req.a, req.b);
        call.then((r) => { job.done(summary(r)); if (id === compareSeq) { setResult(r); setError(""); } })
            .catch((e) => { job.fail(compareError(e)); if (id === compareSeq) { setResult(null); setError(compareError(e)); } })
            .finally(() => { if (id === compareSeq) setBusy(false); });
    }, [req, p.scan]);

    /** Drops the comparison on screen, and any answer still on its way. */
    const forget = () => { compareSeq++; setReq(null); setRan(null); setResult(null); setError(""); };
    const choose = (mode: Mode) => { setMode(mode); forget(); };
    const leave = () => { clearPageState("compare:"); p.onBack(); };
    const pick = async (then: (path: string) => void) => {
        try {
            const path = await api.PickProfileFile();
            if (path) then(path);
        } catch (e) { setResult(null); setError(compareError(e)); }
    };

    return (
        <div className="page detail-page cmp">
            <button className="ghost back" onClick={leave}><Icon name="back" size={16}/> {t("cmp.back")}</button>
            <header>
                <h1>{t("cmp.title")}</h1>
                <p className="muted">{t("cmp.intro")}</p>
                <p className="muted small">{t("cmp.savedLink")}</p>
            </header>

            <TaskViewer kind="profiles"/>

            <fieldset className="cmp-modes">
                <legend id={modeId}>{t("cmp.mode")}</legend>
                {([["file", "cmp.mode.file"], ["code", "cmp.mode.code"], ["files", "cmp.mode.files"]] as const).map(([m, label]) => (
                    <label key={m} className="cmp-mode">
                        <input type="radio" name="cmp-mode" checked={mode === m} onChange={() => choose(m)}/>
                        <span>{t(label)}</span>
                    </label>
                ))}
            </fieldset>

            <div className="cmp-source">
                {mode === "file" && (
                    <>
                        <p className="muted small">{t("cmp.fileHint")}</p>
                        <div className="actions tight">
                            <button disabled={busy} onClick={() => void pick((path) => setReq({mode: "file", path}))}><Icon name="archive" size={14}/> {t("cmp.choose")}</button>
                            {req?.mode === "file" && <span className="mono small cmp-file">{fileName(req.path)}</span>}
                        </div>
                    </>
                )}
                {mode === "code" && (
                    <>
                        <p className="muted small">{t("cmp.codeHint")}</p>
                        <label htmlFor={codeId} className="small">{t("cmp.codeLabel")}</label>
                        <textarea id={codeId} className="codebox mono cmp-code" rows={3} value={code} spellCheck={false} placeholder="WF1.…" onChange={(e) => setCode(e.target.value)}/>
                        <div className="actions tight">
                            <button className="primary" disabled={busy || code.trim() === ""} onClick={() => setReq({mode: "code", code})}>{busy ? t("cmp.comparing") : t("cmp.compare")}</button>
                        </div>
                    </>
                )}
                {mode === "files" && (
                    <>
                        <p className="muted small">{t("cmp.filesHint")}</p>
                        <div className="actions tight">
                            <button disabled={busy} onClick={() => void pick((path) => { setPathA(path); forget(); })}>{t("cmp.chooseA")}</button>
                            <span className="mono small cmp-file">{pathA ? t("cmp.chosenA", {file: fileName(pathA)}) : t("cmp.noneA")}</span>
                        </div>
                        <div className="actions tight">
                            <button disabled={busy} onClick={() => void pick((path) => { setPathB(path); forget(); })}>{t("cmp.chooseB")}</button>
                            <span className="mono small cmp-file">{pathB ? t("cmp.chosenB", {file: fileName(pathB)}) : t("cmp.noneB")}</span>
                        </div>
                        <div className="actions tight">
                            <button className="primary" disabled={busy || !pathA || !pathB} onClick={() => setReq({mode: "files", a: pathA, b: pathB})}>{busy ? t("cmp.comparing") : t("cmp.compare")}</button>
                        </div>
                    </>
                )}
            </div>

            {error && <p className="cmp-error" role="alert"><Icon name="alert" size={16}/> <span>{error}</span></p>}
            {result && <Result r={result} apps={p.apps} profiles={p.profiles} busy={busy} onInstall={p.onInstall} onChanged={p.onChanged} say={p.say}/>}
        </div>
    );
}

function Result(p: {
    r: CompareResult; apps: Map<string, App>; profiles: ProfileInfo[]; busy: boolean;
    onInstall: (p: Profile) => void; onChanged: () => Promise<void>; say: (m: string) => void;
}) {
    const {r} = p;
    const kind = kindOf(r);
    const [working, setWorking] = useState(false);
    const own = useMemo(() => p.profiles.filter((x) => !x.builtin), [p.profiles]);
    const [target, setTarget] = useState("");
    const targetId = own.some((x) => x.id === target) ? target : own[0]?.id ?? "";
    const targetId2 = useId();

    const titleA = kind === "files" ? t("cmp.onlyA.files") : t("cmp.onlyA.pc");
    const titleB = kind === "files" ? t("cmp.onlyB.files") : kind === "code" ? t("cmp.onlyB.code") : t("cmp.onlyB.file");
    const differText = kind === "files" ? "cmp.differ.files" : kind === "code" ? "cmp.differ.code" : "cmp.differ.pc";
    const showDiffer = r.a.versioned && r.b.versioned;
    const inLabel = (w: "a" | "b" | "both") => t(w === "both" ? "cmp.in.both" : kind === "files" ? (w === "a" ? "cmp.in.a" : "cmp.in.b") : kind === "code" ? "cmp.in.code" : "cmp.in.file");

    const guard = async (work: () => Promise<void>) => {
        setWorking(true);
        try { await work(); } catch (e) { p.say(compareError(e)); } finally { setWorking(false); }
    };
    const add = () => {
        const dest = own.find((x) => x.id === targetId);
        if (!dest) return Promise.resolve();
        return guard(async () => {
            await api.SaveProfile(withApps(dest, r.onlyA.map((a) => a.id)));
            await p.onChanged();
            p.say(t("cmp.addedToast", {n: r.onlyA.length, name: dest.name}));
        });
    };
    const save = (side: CompareSide) => guard(async () => {
        if (!side.profile) return;
        const u = uniqueProfile(saveName(side), p.profiles.map((x) => x.id), p.profiles.map((x) => x.name));
        await api.SaveProfile({...side.profile, id: u.id, name: u.name});
        await p.onChanged();
        p.say(t("cmp.savedToast", {name: u.name}));
    });

    const noVersions = !r.a.versioned || !r.b.versioned;
    const files = ([[r.a, "a"], [r.b, "b"]] as const).filter(([s]) => s.kind !== "pc" && s.profile);
    const saveLabel = (role: "a" | "b") => kind === "files" ? t(role === "a" ? "cmp.saveA" : "cmp.saveB") : kind === "code" ? t("cmp.saveCode") : t("cmp.saveFile");
    const dropped = (side: CompareSide) => [...side.unknownRecipes];

    return (
        <section className="cmp-result" aria-busy={p.busy}>
            <p className="muted">{t("cmp.vs", {a: sideLabel(r.a, "a", kind), b: sideLabel(r.b, "b", kind)})}</p>
            <p className="cmp-summary" role="status"><Icon name="info" size={16}/> <span>{summary(r)}</span></p>

            <Group icon="monitor" title={titleA} items={r.onlyA} apps={p.apps}/>
            <Group icon="archive" title={titleB} items={r.onlyB} apps={p.apps}/>
            {showDiffer && (
                <Group icon="refresh" title={t("cmp.differ")} count={r.differ.length} wide empty={r.differ.length === 0}>
                    {r.differ.map((d) => (
                        <Row key={d.id} id={d.id} name={d.name} apps={p.apps}>
                            <span className="mono small muted cmp-extra">{t(differText, {a: d.a, b: d.b})}</span>
                        </Row>
                    ))}
                </Group>
            )}
            <Group icon="check" title={t("cmp.both")} items={r.both} apps={p.apps} collapsible/>
            {r.unknown.length > 0 && (
                <Group icon="help" title={t("cmp.unknown")} count={r.unknown.length} wide hint={t("cmp.unknownHint")}>
                    {r.unknown.map((u) => (
                        <li key={u.id} className="cmp-row">
                            <span className="cmp-unknown-icon" aria-hidden><Icon name="help" size={16}/></span>
                            <span className="cmp-name mono">{u.id}</span>
                            <span className="muted small cmp-extra">{inLabel(u.in)}</span>
                        </li>
                    ))}
                </Group>
            )}

            {noVersions && <p className="muted small">{t("cmp.noVersions")}</p>}
            {[r.a, r.b].map((s, i) => s.unknownRecipes.length > 0 && <p key={"r" + i} className="muted small">{t("cmp.droppedRecipes", {list: dropped(s).join(", ")})}</p>)}
            {[r.a, r.b].map((s, i) => s.unknownProfiles.length > 0 && <p key={"p" + i} className="muted small">{t("cmp.droppedProfiles", {list: s.unknownProfiles.join(", ")})}</p>)}

            <section className="cmp-actions" aria-labelledby={targetId2 + "-h"}>
                <h2 id={targetId2 + "-h"}>{t("cmp.actions")}</h2>
                <div className="actions tight">
                    {r.a.kind === "pc" && r.onlyB.length > 0 && (
                        <button className="primary" disabled={working} title={t("cmp.installHint")}
                                onClick={() => p.onInstall(selectionProfile(r.onlyB.map((a) => a.id), r.b.name || saveName(r.b)))}>
                            <Icon name="download" size={16}/> {t("cmp.install", {n: r.onlyB.length})}
                        </button>
                    )}
                    {files.map(([s, role]) => (
                        <button key={role} disabled={working} onClick={() => void save(s)}>{saveLabel(role)}</button>
                    ))}
                </div>
                {r.a.kind === "pc" && r.onlyA.length > 0 && (
                    own.length === 0 ? <p className="muted small">{t("cmp.addNone")}</p> : (
                        <div className="cmp-add">
                            <label htmlFor={targetId2} className="small">{t("cmp.addLabel")}</label>
                            <div className="actions tight">
                                <select id={targetId2} aria-label={t("cmp.addProfile")} value={targetId} onChange={(e) => setTarget(e.target.value)}>
                                    {own.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
                                </select>
                                <button disabled={working} onClick={() => void add()}>{t("cmp.addButton", {n: r.onlyA.length})}</button>
                            </div>
                        </div>
                    )
                )}
            </section>
        </section>
    );
}

function Row(p: { id: string; name: string; apps: Map<string, App>; children?: ReactNode }) {
    const a = p.apps.get(p.id);
    return (
        <li className="cmp-row">
            <AppIcon id={p.id} name={p.name} category={a?.category ?? ""} size={30}/>
            <span className="cmp-name">{p.name}</span>
            {p.children}
        </li>
    );
}

/** One group of the result: an icon and a title with its count (never colour alone), then a list. */
function Group(p: {
    icon: IconName; title: string; apps?: Map<string, App>; items?: CompareApp[]; count?: number;
    children?: ReactNode; collapsible?: boolean; wide?: boolean; hint?: string; empty?: boolean;
}) {
    const id = useId();
    const listId = id + "-list";
    const [open, setOpen] = useState(!p.collapsible);
    const [all, setAll] = useState(false);
    const count = p.count ?? p.items?.length ?? 0;
    const items = p.items ?? [];
    const shown = all ? items : items.slice(0, LIST_LIMIT);
    const rest = items.length - shown.length;
    const empty = p.empty ?? (p.items ? items.length === 0 : false);

    return (
        <section className="cmp-group" aria-labelledby={id}>
            <h2 id={id}><Icon name={p.icon} size={18}/> {t("cmp.count", {title: p.title, n: count})}</h2>
            {p.hint && <p className="muted small">{p.hint}</p>}
            {empty ? <p className="muted small">{t("cmp.empty")}</p> : (
                <>
                    {p.collapsible && (
                        <button className="sm ghost" aria-expanded={open} aria-controls={listId} onClick={() => setOpen(!open)}>
                            {open ? t("cmp.hide") : t("cmp.show")}
                        </button>
                    )}
                    {open && (
                        <ul id={listId} className={"cmp-list" + (p.wide ? " wide" : "")} aria-label={p.title}>
                            {p.items ? shown.map((a) => <Row key={a.id} id={a.id} name={a.name} apps={p.apps ?? new Map()}/>) : p.children}
                        </ul>
                    )}
                    {open && rest > 0 && <button className="sm" onClick={() => setAll(true)}>{t("cmp.showMore", {n: rest})}</button>}
                    {open && all && items.length > LIST_LIMIT && <button className="sm ghost" onClick={() => setAll(false)}>{t("cmp.showLess")}</button>}
                </>
            )}
        </section>
    );
}
