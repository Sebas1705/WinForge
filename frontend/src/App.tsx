import {useCallback, useEffect, useMemo, useRef, useState} from "react";
import * as Go from "../wailsjs/go/main/App";
import {EventsOn} from "../wailsjs/runtime/runtime";
import {
    applyEvent, categories, filterApps, selectionProfile, slugify,
    type App as AppInfo, type Filter, type InstallEvent, type Plan, type Profile,
    type ProfileInfo, type State, type StepStatus,
} from "./lib/model";
import * as prefs from "./lib/settings";
import logo from "./logo.svg";

// The generated bindings use their own model classes; the JSON shapes are
// identical, so the boundary is typed once here.
const api = Go as unknown as {
    GetState(): Promise<State>;
    Plan(p: Profile): Promise<Plan>;
    Apply(p: Profile): Promise<void>;
    Cancel(): Promise<void>;
    SaveProfile(p: Profile): Promise<void>;
    DeleteProfile(id: string): Promise<void>;
    ProfileFromPC(id: string, name: string, pin: boolean): Promise<Profile>;
    ExportProfile(p: Profile): Promise<string>;
    ExportWinget(p: Profile): Promise<string>;
    ImportProfile(): Promise<{ profile: Profile; unknownApps: string[]; unknownRecipes: string[] } | null>;
    RestartAsAdmin(): Promise<void>;
    OpenURL(u: string): Promise<void>;
    CheckUpdate(): Promise<UpdateInfo>;
    InstallUpdate(): Promise<void>;
};

interface UpdateInfo { current: string; latest: string; available: boolean; url: string; notes: string }

type Tab = "profiles" | "catalog";

interface Run {
    profile: Profile;
    plan: Plan;
    status: Record<string, StepStatus>;
    log: string[];
    running: boolean;
    failed: string[] | null;
}

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));

export default function App() {
    const [state, setState] = useState<State | null>(null);
    const [tab, setTab] = useState<Tab>("profiles");
    const [busy, setBusy] = useState(true);
    const [toast, setToast] = useState<string | null>(null);
    const [selection, setSelection] = useState<Set<string>>(new Set());
    const [run, setRun] = useState<Run | null>(null);
    const [settings, setSettings] = useState<prefs.Settings>(prefs.load);
    const [showSettings, setShowSettings] = useState(false);
    const [update, setUpdate] = useState<UpdateInfo | null>(null);
    const [updating, setUpdating] = useState<{ done: number; total: number } | null>(null);
    const [naming, setNaming] = useState<null | { title: string; initial: string; onOk: (n: string) => void }>(null);

    const say = useCallback((m: string) => {
        setToast(m);
        window.setTimeout(() => setToast((t) => (t === m ? null : t)), 5000);
    }, []);

    const refresh = useCallback(async () => {
        setBusy(true);
        try {
            setState(await api.GetState());
        } catch (e) {
            say(errText(e));
        } finally {
            setBusy(false);
        }
    }, [say]);

    useEffect(() => { void refresh(); }, [refresh]);

    const checkUpdate = useCallback(async (announce: boolean) => {
        try {
            const u = await api.CheckUpdate();
            setUpdate(u.available ? u : null);
            if (announce) say(u.available ? `WinForge ${u.latest} is available.` : "WinForge is up to date.");
        } catch (e) {
            if (announce) say(errText(e));
        }
    }, [say]);

    useEffect(() => { if (settings.checkUpdates) void checkUpdate(false); }, []); // once, at startup

    useEffect(() => EventsOn("update:progress", (p: { done: number; total: number }) => setUpdating(p)), []);

    const installUpdate = async () => {
        setUpdating({done: 0, total: 0});
        try { await api.InstallUpdate(); } catch (e) { setUpdating(null); say(errText(e)); }
    };

    useEffect(() => {
        const mq = window.matchMedia("(prefers-color-scheme: dark)");
        const paint = () => prefs.apply(settings, document.documentElement, mq.matches);
        paint();
        prefs.save(settings);
        mq.addEventListener("change", paint);
        return () => mq.removeEventListener("change", paint);
    }, [settings]);

    useEffect(() => {
        const offEvent = EventsOn("install", (e: InstallEvent) => {
            setRun((r) => {
                if (!r) return r;
                const key = `${e.step.kind}:${e.step.id}`;
                const log = e.status === "output" && e.line ? [...r.log.slice(-400), `${e.step.id}  ${e.line}`]
                    : e.error ? [...r.log, `${e.step.id}  ${e.status}: ${e.error}`] : r.log;
                return {...r, log, status: {...r.status, [key]: applyEvent(r.status[key] ?? "pending", e)}};
            });
        });
        const offDone = EventsOn("install:done", (failed: string[] | null) => {
            setRun((r) => (r ? {...r, running: false, failed: failed ?? []} : r));
            void refresh();
        });
        return () => { offEvent(); offDone(); };
    }, [refresh]);

    const startRun = async (profile: Profile) => {
        try {
            const plan = await api.Plan(profile);
            if (plan.steps.length === 0) return say("Nothing to install: everything in this profile is already on this PC.");
            setRun({profile, plan, status: {}, log: [], running: false, failed: null});
        } catch (e) { say(errText(e)); }
    };

    const confirmRun = async () => {
        if (!run) return;
        try {
            await api.Apply(run.profile);
            setRun({...run, running: true, log: [], status: {}, failed: null});
        } catch (e) { say(errText(e)); }
    };

    const saveSelection = () => {
        setNaming({
            title: "Save selection as a profile",
            initial: "",
            onOk: async (name) => {
                try {
                    await api.SaveProfile({...selectionProfile(selection, name), id: slugify(name)});
                    setSelection(new Set());
                    setTab("profiles");
                    await refresh();
                    say(`Saved “${name}”.`);
                } catch (e) { say(errText(e)); }
            },
        });
    };

    const saveFromPC = () => setNaming({
        title: "Save this PC as a profile",
        initial: "My PC",
        onOk: async (name) => {
            try {
                const p = await api.ProfileFromPC(slugify(name), name, false);
                await api.SaveProfile(p);
                await refresh();
                say(`Saved ${p.apps?.length ?? 0} installed apps as “${name}”.`);
            } catch (e) { say(errText(e)); }
        },
    });

    const importProfile = async () => {
        try {
            const r = await api.ImportProfile();
            if (!r) return;
            await api.SaveProfile(r.profile);
            await refresh();
            const dropped = r.unknownApps.length + r.unknownRecipes.length;
            say(`Imported “${r.profile.name}”` + (dropped ? `; ${dropped} entries are not in this catalog and were ignored.` : "."));
        } catch (e) { say(errText(e)); }
    };

    return (
        <div className="shell">
            <header>
                <div className="brand"><img src={logo} alt="" width={26} height={26}/>Win<b>Forge</b> <span className="ver">{state?.version}</span></div>
                <nav>
                    <button className={tab === "profiles" ? "on" : ""} onClick={() => setTab("profiles")}>Profiles</button>
                    <button className={tab === "catalog" ? "on" : ""} onClick={() => setTab("catalog")}>Catalog</button>
                </nav>
                <div className="grow"/>
                {state && !state.admin && <span className="pill" title="Some installers and recipes need administrator rights">standard user</span>}
                {state?.admin && <span className="pill ok">administrator</span>}
                <button onClick={() => void refresh()} disabled={busy}>{busy ? "Scanning…" : "Rescan PC"}</button>
                <button className="icon" title="Appearance" aria-label="Appearance" onClick={() => setShowSettings(true)}>⚙</button>
            </header>

            {update && (
                <div className="banner info">
                    <span>WinForge <b>{update.latest}</b> is available (you have {update.current}).</span>
                    {updating ? (
                        <progress value={updating.done} max={updating.total || undefined}/>
                    ) : (
                        <>
                            <button className="primary" onClick={() => void installUpdate()}>Update now</button>
                            <a href="#" onClick={(e) => { e.preventDefault(); void api.OpenURL(update.url); }}>Release notes</a>
                        </>
                    )}
                </div>
            )}

            {state?.wingetError && (
                <div className="banner">{state.wingetError}. Detection works, but installing needs winget.</div>
            )}

            <main>
                {!state ? <p className="muted pad">Scanning this PC…</p> : tab === "profiles" ? (
                    <ProfilesView state={state} onInstall={startRun} onChanged={refresh} say={say}
                                  onImport={importProfile} onFromPC={saveFromPC}
                                  onEdit={(p) => { setSelection(new Set(p.resolved)); setTab("catalog"); }}/>
                ) : (
                    <CatalogView apps={state.apps} selection={selection} setSelection={setSelection}
                                 onInstall={() => void startRun(selectionProfile(selection))}
                                 onSave={saveSelection} openURL={(u) => void api.OpenURL(u)}/>
                )}
            </main>

            {run && <RunModal run={run} state={state} onConfirm={() => void confirmRun()}
                              onCancel={() => void api.Cancel()} onClose={() => setRun(null)}
                              onAdmin={() => void api.RestartAsAdmin().catch((e) => say(errText(e)))}/>}
            {showSettings && <SettingsDialog value={settings} onChange={setSettings} version={state?.version ?? ""} onCheck={() => void checkUpdate(true)} onClose={() => setShowSettings(false)}/>}
            {naming && <NameDialog {...naming} onClose={() => setNaming(null)}/>}
            {toast && <div className="toast" role="status">{toast}</div>}
        </div>
    );
}

function ProfilesView(p: {
    state: State; onInstall: (p: Profile) => void; onChanged: () => Promise<void>; say: (m: string) => void;
    onImport: () => void; onFromPC: () => void; onEdit: (p: ProfileInfo) => void;
}) {
    const [sel, setSel] = useState<string>(p.state.profiles[0]?.id ?? "");
    const current = p.state.profiles.find((x) => x.id === sel) ?? p.state.profiles[0];
    const apps = useMemo(() => new Map(p.state.apps.map((a) => [a.id, a])), [p.state.apps]);
    const groups: [string, ProfileInfo[]][] = [
        ["Developer", p.state.profiles.filter((x) => x.builtin && x.kind === "dev")],
        ["General", p.state.profiles.filter((x) => x.builtin && x.kind !== "dev")],
        ["Mine", p.state.profiles.filter((x) => !x.builtin)],
    ];
    const run = (fn: () => Promise<string | void>, ok?: string) => fn().then((r) => {
        if (r) p.say(`Saved to ${r}`); else if (ok) p.say(ok);
    }).catch((e) => p.say(errText(e)));

    return (
        <div className="split">
            <aside>
                {groups.map(([title, list]) => list.length > 0 && (
                    <section key={title}>
                        <h4>{title}</h4>
                        {list.map((x) => {
                            const have = x.resolved.filter((id) => apps.get(id)?.installed).length;
                            return (
                                <button key={x.id} className={"row" + (x.id === current?.id ? " on" : "")} onClick={() => setSel(x.id)}>
                                    <span>{x.name}</span><em>{have}/{x.resolved.length}</em>
                                </button>
                            );
                        })}
                    </section>
                ))}
                <div className="stack">
                    <button onClick={p.onFromPC}>Save this PC as profile…</button>
                    <button onClick={p.onImport}>Import profile…</button>
                </div>
            </aside>
            {current && (
                <section className="detail">
                    <h2>{current.name}</h2>
                    <p className="muted">{current.description}</p>
                    <div className="actions">
                        <button className="primary" onClick={() => p.onInstall(current)}>Install what’s missing…</button>
                        <button onClick={() => run(() => api.ExportProfile(current))}>Export</button>
                        <button onClick={() => run(() => api.ExportWinget(current))} title="A file `winget import` understands">Export for winget</button>
                        <button onClick={() => p.onEdit(current)}>Edit in catalog</button>
                        {!current.builtin && <button className="danger" onClick={() => run(async () => { await api.DeleteProfile(current.id); await p.onChanged(); }, "Deleted.")}>Delete</button>}
                    </div>
                    <ul className="apps">
                        {current.resolved.map((id) => {
                            const a = apps.get(id);
                            return a && (
                                <li key={id}>
                                    <span className={"dot " + (a.installed ? "ok" : "miss")} title={a.installed ? `installed ${a.version ?? ""}` : "not installed"}/>
                                    <b>{a.name}</b><span className="muted"> {a.publisher}</span>
                                    {a.admin && <span className="tag">admin</span>}
                                </li>
                            );
                        })}
                    </ul>
                    {current.resolvedRecipes.length > 0 && <p className="muted">After installing: {current.resolvedRecipes.join(", ")}</p>}
                </section>
            )}
        </div>
    );
}

function CatalogView(p: {
    apps: AppInfo[]; selection: Set<string>; setSelection: (s: Set<string>) => void;
    onInstall: () => void; onSave: () => void; openURL: (u: string) => void;
}) {
    const [f, setF] = useState<Filter>({query: "", category: "", installed: "all", openSourceOnly: false});
    const cats = useMemo(() => categories(p.apps), [p.apps]);
    const list = useMemo(() => filterApps(p.apps, f), [p.apps, f]);
    const toggle = (id: string) => {
        const n = new Set(p.selection);
        if (n.has(id)) n.delete(id); else n.add(id);
        p.setSelection(n);
    };
    const missing = [...p.selection].filter((id) => !p.apps.find((a) => a.id === id)?.installed).length;
    return (
        <div className="catalog">
            <div className="toolbar">
                <input placeholder="Search apps, publishers, winget ids…" value={f.query} autoFocus
                       onChange={(e) => setF({...f, query: e.target.value})}/>
                <select value={f.category} onChange={(e) => setF({...f, category: e.target.value})}>
                    <option value="">All categories</option>
                    {cats.map((c) => <option key={c}>{c}</option>)}
                </select>
                <select value={f.installed} onChange={(e) => setF({...f, installed: e.target.value as Filter["installed"]})}>
                    <option value="all">Installed or not</option>
                    <option value="installed">Installed</option>
                    <option value="missing">Not installed</option>
                </select>
                <label className="check"><input type="checkbox" checked={f.openSourceOnly}
                       onChange={(e) => setF({...f, openSourceOnly: e.target.checked})}/> Open source</label>
            </div>
            <ul className="list">
                {list.map((a) => (
                    <li key={a.id} className={p.selection.has(a.id) ? "sel" : ""}>
                        <label>
                            <input type="checkbox" checked={p.selection.has(a.id)} onChange={() => toggle(a.id)}/>
                            <div className="grow">
                                <b>{a.name}</b> <span className="muted">{a.category}</span>
                                {a.admin && <span className="tag">admin</span>}
                                {a.openSource && <span className="tag oss" title={a.license}>open source</span>}
                                <div className="muted">{a.description}</div>
                            </div>
                        </label>
                        <div className="meta">
                            {a.installed ? <span className="pill ok">installed {a.version}</span> : <span className="pill">not installed</span>}
                            <a href="#" onClick={(e) => { e.preventDefault(); p.openURL(a.homepage); }}>{a.publisher}</a>
                        </div>
                    </li>
                ))}
                {list.length === 0 && <li className="muted pad">No app matches.</li>}
            </ul>
            <footer>
                <span>{list.length} of {p.apps.length} shown · {p.selection.size} selected, {missing} to install</span>
                <div className="grow"/>
                <button disabled={!p.selection.size} onClick={() => p.setSelection(new Set())}>Clear</button>
                <button disabled={!p.selection.size} onClick={p.onSave}>Save as profile…</button>
                <button className="primary" disabled={!missing} onClick={p.onInstall}>Install {missing || ""}…</button>
            </footer>
        </div>
    );
}

function RunModal(p: {
    run: Run; state: State | null; onConfirm: () => void; onCancel: () => void; onClose: () => void; onAdmin: () => void;
}) {
    const {run} = p;
    const started = run.running || run.failed !== null;
    const logRef = useRef<HTMLPreElement>(null);
    useEffect(() => { logRef.current?.scrollTo(0, logRef.current.scrollHeight); }, [run.log]);
    const icon = (s: StepStatus) => ({pending: "○", running: "◔", ok: "✓", failed: "✗", skipped: "–"}[s]);
    return (
        <div className="overlay">
            <div className="modal">
                <h3>{started ? (run.running ? "Installing…" : run.failed?.length ? "Finished with errors" : "Done") : `Install “${run.profile.name}”`}</h3>
                {!started && run.plan.needsAdmin && !p.state?.admin && (
                    <div className="banner">Some steps need administrator rights. <button onClick={p.onAdmin}>Restart as administrator</button></div>
                )}
                <ul className="steps">
                    {run.plan.steps.map((s) => {
                        const st = run.status[`${s.kind}:${s.id}`] ?? "pending";
                        return <li key={s.kind + s.id} className={st}><span>{icon(st)}</span> {s.kind === "recipe" ? "Setup: " : ""}{s.name}{s.admin && <span className="tag">admin</span>}</li>;
                    })}
                </ul>
                {started && <pre ref={logRef} className="log">{run.log.join("\n")}</pre>}
                <div className="actions end">
                    {!started && <><button onClick={p.onClose}>Cancel</button><button className="primary" onClick={p.onConfirm}>Install {run.plan.steps.length} steps</button></>}
                    {run.running && <button className="danger" onClick={p.onCancel}>Stop</button>}
                    {started && !run.running && <button className="primary" onClick={p.onClose}>Close</button>}
                </div>
            </div>
        </div>
    );
}

function NameDialog(p: { title: string; initial: string; onOk: (n: string) => void; onClose: () => void }) {
    const [v, setV] = useState(p.initial);
    const submit = () => { if (v.trim()) { p.onOk(v.trim()); p.onClose(); } };
    return (
        <div className="overlay" onClick={p.onClose}>
            <div className="modal small" onClick={(e) => e.stopPropagation()}>
                <h3>{p.title}</h3>
                <input autoFocus value={v} placeholder="Profile name" onChange={(e) => setV(e.target.value)}
                       onKeyDown={(e) => { if (e.key === "Enter") submit(); if (e.key === "Escape") p.onClose(); }}/>
                <div className="actions end"><button onClick={p.onClose}>Cancel</button><button className="primary" disabled={!v.trim()} onClick={submit}>Save</button></div>
            </div>
        </div>
    );
}

function SettingsDialog(p: { version: string; onCheck: () => void; value: prefs.Settings; onChange: (s: prefs.Settings) => void; onClose: () => void }) {
    const set = (patch: Partial<prefs.Settings>) => p.onChange({...p.value, ...patch});
    return (
        <div className="overlay" onClick={p.onClose}>
            <div className="modal small" onClick={(e) => e.stopPropagation()} onKeyDown={(e) => e.key === "Escape" && p.onClose()}>
                <h3>Appearance</h3>
                <div className="field"><span>Theme</span>
                    <div className="seg">
                        {(["system", "dark", "light"] as const).map((t) => (
                            <button key={t} className={p.value.theme === t ? "on" : ""} onClick={() => set({theme: t})}>{t}</button>
                        ))}
                    </div>
                </div>
                <div className="field"><span>Accent</span>
                    <div className="swatches">
                        {prefs.ACCENTS.map((c) => (
                            <button key={c.value} title={c.name} aria-label={c.name} aria-pressed={p.value.accent === c.value}
                                    className={"swatch" + (p.value.accent === c.value ? " on" : "")} style={{background: c.value}}
                                    onClick={() => set({accent: c.value})}/>
                        ))}
                    </div>
                </div>
                <div className="field"><span>Density</span>
                    <div className="seg">
                        {(["comfortable", "compact"] as const).map((d) => (
                            <button key={d} className={p.value.density === d ? "on" : ""} onClick={() => set({density: d})}>{d}</button>
                        ))}
                    </div>
                </div>
                <div className="field"><span>Updates</span>
                    <label><input type="checkbox" checked={p.value.checkUpdates} onChange={(e) => set({checkUpdates: e.target.checked})}/> Check at startup</label>
                </div>
                <div className="field"><span className="muted">Version {p.version}</span><button onClick={p.onCheck}>Check now</button></div>
                <div className="actions end"><button className="primary" onClick={p.onClose}>Done</button></div>
            </div>
        </div>
    );
}
