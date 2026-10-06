import {useMemo} from "react";
import {AppCard} from "../components/AppCard";
import {Icon, categoryIcon, type IconName} from "../components/Icon";
import {Ruler} from "../components/Strip";
import {IconStack, Ring} from "../components/Visual";
import {categoryLabel, getLang, t, type Key} from "../lib/i18n";
import {profileText} from "../lib/profileText";
import type {HealthResult} from "../lib/health";
import type {App, Profile, ProfileInfo, UpgradeInfo} from "../lib/model";
import {categoryStats, closestToDone, featuredApps, profileApps, profileTally} from "../lib/tally";

// Ready-made starting points, by the profile each one opens.
const QUICK: { id: string; icon: IconName }[] = [
    {id: "everyday", icon: "home"},
    {id: "gaming", icon: "gamepad"},
    {id: "dev-base", icon: "code"},
    {id: "creator", icon: "palette"},
    {id: "privacy", icon: "shield"},
    {id: "opensource", icon: "heart"},
];

export function Home(p: {
    apps: App[]; profiles: ProfileInfo[]; featured: string[];
    upgrades: UpgradeInfo[] | null; upgradesBusy: boolean; health: HealthResult | null;
    onInstall: (profile: Profile) => void; onOpenProfile: (id: string) => void;
    onCheckUpgrades: () => void; onOpenUpdates: () => void; onOpenCategory: (top: string) => void;
    onOpenHealth: () => void; onOpenPopular: () => void;
    onInstallApp: (a: App) => void; onDetail: (a: App) => void;
}) {
    const byId = useMemo(() => new Map(p.apps.map((a) => [a.id, a])), [p.apps]);
    const found = p.apps.filter((a) => a.installed).length;
    const stats = useMemo(() => categoryStats(p.apps), [p.apps]);
    const next = useMemo(() => closestToDone(p.profiles, byId, 3), [p.profiles, byId]);
    const popular = useMemo(() => featuredApps(p.apps, p.featured, 14), [p.apps, p.featured]);
    const quick = QUICK.flatMap((q) => {
        const prof = p.profiles.find((x) => x.id === q.id);
        return prof ? [{...q, prof}] : [];
    });
    const h = p.health;
    const worst = h ? (h.findings.some((f) => f.severity === "bad") ? "bad" : h.findings.some((f) => f.severity === "warn") ? "warn" : "ok") : "muted";

    return (
        <div className="page home">
            <header className="hero">
                <Ring size={110} stroke={11} value={p.apps.length ? found / p.apps.length : 0}>
                    <span className="ringnum big">{found}</span>
                </Ring>
                <div className="grow">
                    <span className="eyebrow">{t("home.eyebrow")}</span>
                    <h1>{t("home.foundOf", {total: p.apps.length})}</h1>
                    <Ruler apps={p.apps}/>
                </div>
            </header>

            <section>
                <h2>{t("home.quick")}</h2>
                <div className="quick">
                    {quick.map(({id, icon, prof}) => {
                        const tl = profileTally(prof.resolved, byId);
                        const full = tl.total > 0 && tl.have === tl.total;
                        return (
                            <button key={id} className={"quicktile" + (full ? " full" : "")} onClick={() => p.onOpenProfile(id)}>
                                <span className="bubble"><Icon name={full ? "check" : icon} size={26}/></span>
                                <b>{t(`home.tile.${id}` as Key)}</b>
                                <span className="mono muted small">{tl.have}/{tl.total}</span>
                            </button>
                        );
                    })}
                </div>
            </section>

            {popular.length > 0 && (
                <section>
                    <div className="sectionhead">
                        <h2>{t("home.popular")}</h2>
                        <button className="link seeall" onClick={p.onOpenPopular}>{t("home.seeAll")} <Icon name="chevron" size={14}/></button>
                    </div>
                    <div className="shelf">
                        {popular.map((a) => (
                            <AppCard key={a.id} app={a} compact onOpen={() => p.onDetail(a)} onInstall={() => p.onInstallApp(a)}/>
                        ))}
                    </div>
                </section>
            )}

            <div className="two">
                <button className="statuscard" onClick={p.onOpenHealth}>
                    <Ring size={72} stroke={8} value={h && h.total ? h.ok / h.total : 0} tone={worst as "ok" | "warn" | "bad" | "muted"}>
                        <Icon name={worst === "ok" ? "check" : worst === "muted" ? "pulse" : "alert"} size={26}/>
                    </Ring>
                    <div>
                        <b>{t("home.healthCard")}</b>
                        <span className="muted">{h ? t("home.healthScore", {ok: h.ok, total: h.total}) : t("home.healthHint")}</span>
                    </div>
                    <span className="go">{h ? <Icon name="chevron" size={18}/> : <span className="pill">{t("home.healthCheck")}</span>}</span>
                </button>

                {p.upgrades === null ? (
                    <button className="statuscard" onClick={p.onCheckUpgrades} disabled={p.upgradesBusy}>
                        <span className="bubble"><Icon name="download" size={26}/></span>
                        <div>
                            <b>{t("home.updatesCard")}</b>
                            <span className="muted">{p.upgradesBusy ? t("updates.checking") : t("home.updatesCheck")}</span>
                        </div>
                    </button>
                ) : (
                    <button className="statuscard" onClick={p.onOpenUpdates}>
                        <span className={"bubble" + (p.upgrades.length === 0 ? " ok" : "")}><Icon name={p.upgrades.length === 0 ? "check" : "download"} size={26}/></span>
                        <div>
                            <b>{t("home.updatesCard")}</b>
                            <span className="muted">{p.upgrades.length === 0 ? t("home.updatesNone") : `${p.upgrades.length} ${t("home.updatesWord")}`}</span>
                        </div>
                        <span className="go"><Icon name="chevron" size={18}/></span>
                    </button>
                )}
            </div>

            {next.length > 0 && (
                <section>
                    <h2>{t("home.next")}</h2>
                    <ul className="cards">
                        {next.map(({profile, missing}) => {
                            const tl = profileTally(profile.resolved, byId);
                            return (
                                <li key={profile.id} className="card nextcard">
                                    <IconStack apps={profileApps(profile.resolved, byId)} max={5} size={34}/>
                                    <button className="link title" onClick={() => p.onOpenProfile(profile.id)}>{profileText(profile, getLang()).name}</button>
                                    <span className="grow"/>
                                    <Ring size={36} stroke={4} value={tl.have / tl.total}><span className="mono tiny">{missing}</span></Ring>
                                    <button className="primary sm" onClick={() => p.onInstall(profile)}>{t("common.install")}</button>
                                </li>
                            );
                        })}
                    </ul>
                </section>
            )}

            <section>
                <h2>{t("home.categories")}</h2>
                <ul className="cats">
                    {stats.map((s) => (
                        <li key={s.top}>
                            <button className="link" onClick={() => p.onOpenCategory(s.top)}><Icon name={categoryIcon(s.top)} size={17}/>{categoryLabel(s.top)}</button>
                            <span className="bar" aria-hidden><i style={{width: `${(100 * s.have) / s.total}%`}}/></span>
                            <span className="mono muted">{s.have}/{s.total}</span>
                        </li>
                    ))}
                </ul>
            </section>
        </div>
    );
}
