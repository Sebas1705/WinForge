import {useCallback, useEffect, useState, type ReactNode} from "react";
import {api, errText, on, type UpdateInfo} from "./api";
import {NameDialog, RunModal, SettingsDialog, stepKey, type RunState} from "./components/Modals";
import {resolveLang, setLang, t} from "./lib/i18n";
import {
    applyEvent, selectionProfile, slugify,
    type Profile, type ProfileInfo, type State, type UpgradeInfo,
} from "./lib/model";
import * as prefs from "./lib/settings";
import logo from "./logo.svg";
import {Catalog} from "./views/Catalog";
import {Home} from "./views/Home";
import {Profiles} from "./views/Profiles";
import {Updates} from "./views/Updates";

type Tab = "home" | "profiles" | "catalog" | "updates";

interface Runner {
    /** Called when the person confirms the plan. */
    start: () => Promise<void>;
    profile?: Profile;
    /** Runs after the plan finishes, to refresh what the screen shows. */
    after?: () => void;
}

const ICONS: Record<Tab, ReactNode> = {
    home: <path d="M3 11.5 12 4l9 7.5M6 10v10h12V10"/>,
    profiles: <path d="M4 6h16M4 12h16M4 18h10"/>,
    catalog: <path d="M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z"/>,
    updates: <path d="M12 4v11m0 0-4-4m4 4 4-4M5 20h14"/>,
};

export default function App() {
    const [state, setState] = useState<State | null>(null);
    const [tab, setTab] = useState<Tab>("home");
    const [busy, setBusy] = useState(true);
    const [toast, setToast] = useState<string | null>(null);
    const [selection, setSelection] = useState<Set<string>>(new Set());
    const [focusProfile, setFocusProfile] = useState<string | null>(null);
    const [category, setCategory] = useState("");
    const [run, setRun] = useState<RunState | null>(null);
    const [runner, setRunner] = useState<Runner | null>(null);
    const [naming, setNaming] = useState<null | { title: string; initial: string; onOk: (n: string) => void }>(null);
    const [settings, setSettings] = useState<prefs.Settings>(prefs.load);
    const [showSettings, setShowSettings] = useState(false);
    const [update, setUpdate] = useState<UpdateInfo | null>(null);
    const [updating, setUpdating] = useState<{ done: number; total: number } | null>(null);
    const [upgrades, setUpgrades] = useState<UpgradeInfo[] | null>(null);
    const [upgradesBusy, setUpgradesBusy] = useState(false);

    // The language is applied during render so every child translates with it.
    setLang(resolveLang(settings.language, navigator.language));
    useEffect(() => { document.documentElement.lang = resolveLang(settings.language, navigator.language); }, [settings.language]);

    const say = useCallback((m: string) => {
        setToast(m);
        window.setTimeout(() => setToast((x) => (x === m ? null : x)), 5000);
    }, []);

    const refresh = useCallback(async () => {
        setBusy(true);
        try { setState(await api.GetState()); } catch (e) { say(errText(e)); } finally { setBusy(false); }
    }, [say]);
    useEffect(() => { void refresh(); }, [refresh]);

    const checkUpgrades = useCallback(async () => {
        setUpgradesBusy(true);
        try { setUpgrades(await api.Upgrades()); } catch (e) { say(errText(e)); } finally { setUpgradesBusy(false); }
    }, [say]);

    const checkUpdate = useCallback(async (announce: boolean) => {
        try {
            const u = await api.CheckUpdate();
            setUpdate(u.available ? u : null);
            if (announce) say(u.available ? t("banner.available", {v: u.latest, c: u.current}) : t("toast.upToDate"));
        } catch (e) { if (announce) say(errText(e)); }
    }, [say]);
    useEffect(() => { if (settings.checkUpdates) void checkUpdate(false); }, []); // once, at startup
    useEffect(() => on.progress(setUpdating), []);

    useEffect(() => {
        const mq = window.matchMedia("(prefers-color-scheme: dark)");
        const paint = () => prefs.apply(settings, document.documentElement, mq.matches);
        paint();
        prefs.save(settings);
        mq.addEventListener("change", paint);
        return () => mq.removeEventListener("change", paint);
    }, [settings]);

    // Installation events feed the open run.
    useEffect(() => {
        const offEvent = on.install((e) => {
            setRun((r) => {
                if (!r) return r;
                const key = stepKey(e.step);
                const log = e.status === "output" && e.line ? [...r.log.slice(-400), `${e.step.id}  ${e.line}`]
                    : e.error ? [...r.log, `${e.step.id}  ${e.status}: ${e.error}`] : r.log;
                return {...r, log, status: {...r.status, [key]: applyEvent(r.status[key] ?? "pending", e)}};
            });
        });
        const offDone = on.done((failed) => {
            setRun((r) => (r ? {...r, running: false, failed: failed ?? []} : r));
            void refresh();
            setRunner((rn) => { rn?.after?.(); return rn; });
        });
        return () => { offEvent(); offDone(); };
    }, [refresh]);

    const openRun = (title: string, plan: RunState["plan"], rn: Runner) => {
        if (plan.steps.length === 0) return say(t("toast.nothing"));
        setRunner(rn);
        setRun({title, plan, status: {}, log: [], running: false, failed: null, startedAt: null, canScript: !!rn.profile});
    };

    const startProfile = async (profile: Profile) => {
        try {
            const plan = await api.Plan(profile);
            openRun(t("run.install", {name: profile.name}), plan, {profile, start: () => api.Apply(profile)});
        } catch (e) { say(errText(e)); }
    };

    const startUpgrades = async (ids: string[]) => {
        try {
            const plan = await api.PlanUpgrades(ids);
            openRun(t("run.updateTitle"), plan, {start: () => api.ApplyUpgrades(ids), after: () => void checkUpgrades()});
        } catch (e) { say(errText(e)); }
    };

    const confirmRun = async () => {
        if (!run || !runner) return;
        try {
            await runner.start();
            setRun({...run, running: true, log: [], status: {}, failed: null, startedAt: Date.now()});
        } catch (e) { say(errText(e)); }
    };

    const closeRun = () => { setRun(null); setRunner(null); };

    const copyLog = async () => {
        try { await navigator.clipboard.writeText(run?.log.join("\n") ?? ""); say(t("run.copied")); } catch (e) { say(errText(e)); }
    };

    const saveSelection = () => setNaming({
        title: t("dlg.saveSelection"), initial: "",
        onOk: async (name) => {
            try {
                await api.SaveProfile({...selectionProfile(selection, name), id: slugify(name)});
                setSelection(new Set());
                setTab("profiles");
                await refresh();
                say(t("toast.saved", {name}));
            } catch (e) { say(errText(e)); }
        },
    });

    const saveFromPC = () => setNaming({
        title: t("dlg.saveFromPC"), initial: "",
        onOk: async (name) => {
            try {
                const p = await api.ProfileFromPC(slugify(name), name, false);
                await api.SaveProfile(p);
                await refresh();
                say(t("toast.savedPC", {n: p.apps?.length ?? 0, name}));
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
            say(dropped ? t("toast.importedDropped", {name: r.profile.name, n: dropped}) : t("toast.imported", {name: r.profile.name}));
        } catch (e) { say(errText(e)); }
    };

    const go = (next: Tab) => {
        setTab(next);
        if (next === "updates" && upgrades === null && !upgradesBusy) void checkUpgrades();
    };
    const editProfile = (p: ProfileInfo) => { setSelection(new Set(p.resolved)); setCategory(""); setTab("catalog"); };

    const nav: [Tab, string, string | number | null][] = [
        ["home", t("nav.home"), null],
        ["profiles", t("nav.profiles"), state?.profiles.length ?? null],
        ["catalog", t("nav.catalog"), state?.apps.length ?? null],
        ["updates", t("nav.updates"), upgrades && upgrades.length > 0 ? upgrades.length : null],
    ];

    return (
        <div className="shell">
            <nav className="rail" aria-label="Main">
                <div className="brand"><img src={logo} alt="" width={28} height={28}/><span>Win<b>Forge</b></span></div>
                {nav.map(([id, label, count]) => (
                    <button key={id} className={"navitem" + (tab === id ? " on" : "")} aria-current={tab === id ? "page" : undefined} onClick={() => go(id)}>
                        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden>{ICONS[id]}</svg>
                        <span>{label}</span>
                        {count !== null && <em className="mono">{count}</em>}
                    </button>
                ))}
                <span className="grow"/>
                {state && (
                    <span className={"pill" + (state.admin ? " ok" : "")} title={state.admin ? "" : t("status.userHint")}>
                        {state.admin ? t("status.admin") : t("status.user")}
                    </span>
                )}
                <button className="ghost" disabled={busy} onClick={() => void refresh()}>{busy ? t("rail.scanning") : t("rail.rescan")}</button>
                <div className="railfoot">
                    <span className="mono muted">{state?.version}</span>
                    <button className="icon" title={t("rail.appearance")} aria-label={t("rail.appearance")} onClick={() => setShowSettings(true)}>⚙</button>
                </div>
            </nav>

            <div className="content">
                {update && (
                    <div className="banner info">
                        <span>{t("banner.available", {v: update.latest, c: update.current})}</span>
                        {updating ? <progress value={updating.done} max={updating.total || undefined}/> : (
                            <>
                                <button className="primary sm" onClick={() => { setUpdating({done: 0, total: 0}); api.InstallUpdate().catch((e) => { setUpdating(null); say(errText(e)); }); }}>{t("banner.now")}</button>
                                <a href="#" onClick={(e) => { e.preventDefault(); void api.OpenURL(update.url); }}>{t("banner.notes")}</a>
                            </>
                        )}
                    </div>
                )}
                {state?.wingetError && <div className="banner warn">{state.wingetError}. {t("wingetOnlyDetect")}</div>}

                <main>
                    {!state ? <p className="muted pad">{t("rail.scanning")}</p> : tab === "home" ? (
                        <Home apps={state.apps} profiles={state.profiles} upgrades={upgrades} upgradesBusy={upgradesBusy}
                              onInstall={(p) => void startProfile(p)}
                              onOpenProfile={(id) => { setFocusProfile(id); setTab("profiles"); }}
                              onCheckUpgrades={() => void checkUpgrades()} onOpenUpdates={() => go("updates")}
                              onOpenCategory={(top) => { setCategory(top); setTab("catalog"); }}/>
                    ) : tab === "profiles" ? (
                        <Profiles state={state} focus={focusProfile} onInstall={(p) => void startProfile(p)} onChanged={refresh} say={say}
                                  onImport={() => void importProfile()} onFromPC={saveFromPC} onEdit={editProfile}/>
                    ) : tab === "catalog" ? (
                        <Catalog apps={state.apps} selection={selection} setSelection={setSelection} initialCategory={category}
                                 onInstall={() => void startProfile(selectionProfile(selection))}
                                 onInstallOne={(a) => void startProfile(selectionProfile([a.id], a.name))}
                                 onSave={saveSelection} openURL={(u) => void api.OpenURL(u)}/>
                    ) : (
                        <Updates upgrades={upgrades} busy={upgradesBusy} onCheck={() => void checkUpgrades()} onUpdate={(ids) => void startUpgrades(ids)}/>
                    )}
                </main>
            </div>

            {run && <RunModal run={run} admin={!!state?.admin} onConfirm={() => void confirmRun()} onCancel={() => void api.Cancel()}
                              onClose={closeRun} onAdmin={() => void api.RestartAsAdmin().catch((e) => say(errText(e)))}
                              onScript={() => runner?.profile && api.ExportScript(runner.profile).then((p) => p && say(t("toast.savedTo", {path: p}))).catch((e) => say(errText(e)))}
                              onCopy={() => void copyLog()}/>}
            {showSettings && <SettingsDialog value={settings} onChange={setSettings} version={state?.version ?? ""}
                                             onCheck={() => void checkUpdate(true)} onClose={() => setShowSettings(false)}/>}
            {naming && <NameDialog {...naming} onClose={() => setNaming(null)}/>}
            {toast && <div className="toast" role="status">{toast}</div>}
        </div>
    );
}
