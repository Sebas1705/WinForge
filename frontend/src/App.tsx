import {useCallback, useEffect, useState} from "react";
import {api, errText, on, type UpdateInfo} from "./api";
import {AppDetail} from "./components/AppDetail";
import {Icon, type IconName} from "./components/Icon";
import {NameDialog, RunModal, Segmented, SettingsDialog, Tour, stepKey, type RunState} from "./components/Modals";
import {POPULAR} from "./views/Catalog";
import {IconsContext, loadIcons, type IconIndex} from "./lib/icons";
import {reportMarkdown, type HealthLink, type HealthResult} from "./lib/health";
import {getLang, resolveLang, setLang, t} from "./lib/i18n";
import {profileText} from "./lib/profileText";
import {
    applyEvent, selectionProfile, slugify,
    type App as CatalogApp, type Profile, type ProfileInfo, type State, type UpgradeInfo,
} from "./lib/model";
import * as prefs from "./lib/settings";
import logo from "./logo.svg";
import {Catalog} from "./views/Catalog";
import {Health} from "./views/Health";
import {Home} from "./views/Home";
import {Profiles} from "./views/Profiles";
import {Updates} from "./views/Updates";

type Tab = "home" | "profiles" | "catalog" | "updates" | "health";

interface Runner {
    /** Called when the person confirms the plan. */
    start: () => Promise<void>;
    profile?: Profile;
    /** Runs after the plan finishes, to refresh what the screen shows. */
    after?: () => void;
}

const NAV_ICON: Record<Tab, IconName> = {home: "home", profiles: "layers", catalog: "grid", updates: "download", health: "pulse"};

function tourSeen(): boolean {
    try { return localStorage.getItem("winforge.tour") === "1"; } catch { return true; }
}

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
    const [health, setHealth] = useState<HealthResult | null>(null);
    const [healthBusy, setHealthBusy] = useState(false);
    const [healthUpdBusy, setHealthUpdBusy] = useState(false);
    const [icons, setIcons] = useState<IconIndex>({});
    const [detail, setDetail] = useState<CatalogApp | null>(null);
    const [showTour, setShowTour] = useState(() => !tourSeen());
    const advanced = settings.detail === "advanced";
    const closeTour = () => {
        setShowTour(false);
        try { localStorage.setItem("winforge.tour", "1"); } catch { /* shown again next time */ }
    };
    useEffect(() => { void loadIcons().then(setIcons); }, []);

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

    const scanHealth = useCallback(async () => {
        setHealthBusy(true);
        try { setHealth(await api.HealthScan()); } catch (e) { say(errText(e)); } finally { setHealthBusy(false); }
    }, [say]);

    const healthUpdates = useCallback(async () => {
        setHealthUpdBusy(true);
        try { setHealth(await api.HealthUpdates()); } catch (e) { say(errText(e)); } finally { setHealthUpdBusy(false); }
    }, [say]);

    const exportHealth = async () => {
        if (!health) return;
        const md = reportMarkdown(health, {
            title: t("health.title"), checks: t("health.checks"), drivers: t("health.drivers"), updates: t("health.updates"),
            group: (g) => t(`group.${g}` as "group.firmware"),
        });
        try {
            const path = await api.SaveTextFile("winforge-pc-health.md", md);
            if (path) say(t("health.exported", {path}));
        } catch (e) { say(errText(e)); }
    };

    const openLink = (l: HealthLink) => { api.OpenLink(l.url).catch((e) => say(errText(e))); };

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
            const info = state?.profiles.find((x) => x.id === profile.id);
            const name = info?.builtin && info.name === profile.name ? profileText(info, getLang()).name : profile.name;
            openRun(t("run.install", {name}), plan, {profile, start: () => api.Apply(profile)});
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

    const copyText = async (text: string, done: string) => {
        try { await navigator.clipboard.writeText(text); say(done); } catch (e) { say(errText(e)); }
    };

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
        if (next === "health" && health === null && !healthBusy) void scanHealth();
    };
    const editProfile = (p: ProfileInfo) => { setSelection(new Set(p.resolved)); setCategory(""); setTab("catalog"); };

    const nav: [Tab, string, string | number | null][] = [
        ["home", t("nav.home"), null],
        ["profiles", t("nav.profiles"), state?.profiles.length ?? null],
        ["catalog", t("nav.catalog"), state?.apps.length ?? null],
        ["updates", t("nav.updates"), upgrades && upgrades.length > 0 ? upgrades.length : null],
        ["health", t("nav.health"), health && health.ok < health.total ? health.total - health.ok : null],
    ];

    const byId = new Map((state?.apps ?? []).map((a) => [a.id, a]));
    // A detail panel shows the live state: it follows rescans and installs.
    const shown = detail ? byId.get(detail.id) ?? detail : null;

    return (
        <IconsContext.Provider value={icons}>
        <div className="shell">
            <nav className="rail" aria-label="Main">
                <div className="brand"><img src={logo} alt="" width={28} height={28}/><span>Win<b>Forge</b></span></div>
                {nav.map(([id, label, count]) => (
                    <button key={id} className={"navitem" + (tab === id ? " on" : "")} aria-current={tab === id ? "page" : undefined} onClick={() => go(id)}>
                        <Icon name={NAV_ICON[id]} size={19}/>
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
                <button className="ghost" disabled={busy} onClick={() => void refresh()}><Icon name="refresh" size={15}/> {busy ? t("rail.scanning") : t("rail.rescan")}</button>
                <div title={t("mode.hint")}>
                    <Segmented value={settings.detail} onChange={(detail) => setSettings({...settings, detail})}
                               options={[["simple", t("mode.simple")], ["advanced", t("mode.advanced")]]}/>
                </div>
                <div className="railfoot">
                    <span className="mono muted">{state?.version}</span>
                    <span>
                        <button className="icon" title={t("tour.help")} aria-label={t("tour.help")} onClick={() => setShowTour(true)}><Icon name="help" size={17}/></button>
                        <button className="icon" title={t("rail.appearance")} aria-label={t("rail.appearance")} onClick={() => setShowSettings(true)}><Icon name="settings" size={17}/></button>
                    </span>
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
                        <Home apps={state.apps} profiles={state.profiles} featured={state.featured ?? []} upgrades={upgrades} upgradesBusy={upgradesBusy}
                              health={health} onOpenHealth={() => go("health")} onOpenPopular={() => { setCategory(POPULAR); setTab("catalog"); }}
                              onInstallApp={(a) => void startProfile(selectionProfile([a.id], a.name))} onDetail={setDetail}
                              onInstall={(p) => void startProfile(p)}
                              onOpenProfile={(id) => { setFocusProfile(id); setTab("profiles"); }}
                              onCheckUpgrades={() => void checkUpgrades()} onOpenUpdates={() => go("updates")}
                              onOpenCategory={(top) => { setCategory(top); setTab("catalog"); }}/>
                    ) : tab === "profiles" ? (
                        <Profiles state={state} focus={focusProfile} advanced={advanced} onFocusDone={() => setFocusProfile(null)} onInstall={(p) => void startProfile(p)} onChanged={refresh} say={say}
                                  onImport={() => void importProfile()} onFromPC={saveFromPC} onEdit={editProfile} onDetail={setDetail}/>
                    ) : tab === "catalog" ? (
                        <Catalog apps={state.apps} featured={state.featured ?? []} advanced={advanced} selection={selection} setSelection={setSelection} initialCategory={category}
                                 onInstall={() => void startProfile(selectionProfile(selection))}
                                 onInstallOne={(a) => void startProfile(selectionProfile([a.id], a.name))}
                                 onSave={saveSelection} openURL={(u) => void api.OpenURL(u)} onDetail={setDetail}/>
                    ) : tab === "health" ? (
                        <Health advanced={advanced} result={health} busy={healthBusy} updatesBusy={healthUpdBusy} admin={state.admin} byId={byId}
                                onScan={() => void scanHealth()} onUpdates={() => void healthUpdates()} onExport={() => void exportHealth()}
                                onAdmin={() => void api.RestartAsAdmin().catch((e) => say(errText(e)))} onLink={openLink}
                                onInstallApp={(a) => void startProfile(selectionProfile([a.id], a.name))} onDetail={setDetail}/>
                    ) : (
                        <Updates upgrades={upgrades} busy={upgradesBusy} onCheck={() => void checkUpgrades()} onUpdate={(ids) => void startUpgrades(ids)} byId={byId} onDetail={setDetail}/>
                    )}
                </main>
            </div>

            {run && <RunModal run={run} advanced={advanced} byId={byId} admin={!!state?.admin} onConfirm={() => void confirmRun()} onCancel={() => void api.Cancel()}
                              onClose={closeRun} onAdmin={() => void api.RestartAsAdmin().catch((e) => say(errText(e)))}
                              onScript={() => runner?.profile && api.ExportScript(runner.profile).then((p) => p && say(t("toast.savedTo", {path: p}))).catch((e) => say(errText(e)))}
                              onCopy={() => void copyLog()}/>}
            {showSettings && <SettingsDialog value={settings} onChange={setSettings} version={state?.version ?? ""}
                                             onCheck={() => void checkUpdate(true)} onClose={() => setShowSettings(false)}/>}
            {naming && <NameDialog {...naming} onClose={() => setNaming(null)}/>}
            {showTour && <Tour onClose={closeTour}/>}
            {shown && <AppDetail advanced={advanced} app={shown} byId={byId} onClose={() => setDetail(null)} openURL={(u) => void api.OpenURL(u)}
                                 onInstall={(a) => { setDetail(null); void startProfile(selectionProfile([a.id], a.name)); }}
                                 onCopy={(c) => void copyText(c, t("detail.copied"))}/>}
            {toast && <div className="toast" role="status">{toast}</div>}
        </div>
        </IconsContext.Provider>
    );
}
