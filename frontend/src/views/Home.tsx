import {useMemo} from "react";
import {Ruler, Strip} from "../components/Strip";
import {categoryLabel, t} from "../lib/i18n";
import type {App, Profile, ProfileInfo, UpgradeInfo} from "../lib/model";
import {categoryStats, closestToDone, profileTally} from "../lib/tally";

export function Home(p: {
    apps: App[]; profiles: ProfileInfo[];
    upgrades: UpgradeInfo[] | null; upgradesBusy: boolean;
    onInstall: (profile: Profile) => void; onOpenProfile: (id: string) => void;
    onCheckUpgrades: () => void; onOpenUpdates: () => void; onOpenCategory: (top: string) => void;
}) {
    const byId = useMemo(() => new Map(p.apps.map((a) => [a.id, a])), [p.apps]);
    const found = p.apps.filter((a) => a.installed).length;
    const stats = useMemo(() => categoryStats(p.apps), [p.apps]);
    const next = useMemo(() => closestToDone(p.profiles, byId, 3), [p.profiles, byId]);

    return (
        <div className="page home">
            <header className="hero">
                <span className="eyebrow">{t("home.eyebrow")}</span>
                <h1><em>{found}</em> <span>{t("home.foundOf", {total: p.apps.length})}</span></h1>
                <p className="muted">{t("home.sub")}</p>
                <Ruler apps={p.apps}/>
                <span className="caption">{t("home.ruler")}</span>
            </header>

            <div className="two">
                <section>
                    <h2>{t("home.next")}</h2>
                    {next.length === 0 && <p className="muted">{t("home.nextEmpty")}</p>}
                    <ul className="cards">
                        {next.map(({profile, missing}) => {
                            const tally = profileTally(profile.resolved, byId);
                            return (
                                <li key={profile.id} className="card">
                                    <button className="link title" onClick={() => p.onOpenProfile(profile.id)}>{profile.name}</button>
                                    <Strip cells={tally.cells.map((c) => ({on: c.on, name: c.name}))} label={`${tally.have}/${tally.total}`}/>
                                    <div className="row">
                                        <span className="mono muted">{t("home.missing", {n: missing})}</span>
                                        <span className="grow"/>
                                        <button className="primary sm" onClick={() => p.onInstall(profile)}>{t("common.install")}</button>
                                    </div>
                                </li>
                            );
                        })}
                    </ul>
                </section>

                <section>
                    <h2>{t("home.updates")}</h2>
                    <div className="card">
                        {p.upgrades === null ? (
                            <button className="primary sm" disabled={p.upgradesBusy} onClick={p.onCheckUpgrades}>
                                {p.upgradesBusy ? t("updates.checking") : t("home.updatesCheck")}
                            </button>
                        ) : p.upgrades.length === 0 ? (
                            <p>{t("home.updatesNone")}</p>
                        ) : (
                            <>
                                <p className="big"><em>{p.upgrades.length}</em> {t("home.updatesWord")}</p>
                                <button className="sm" onClick={p.onOpenUpdates}>{t("home.updatesView")}</button>
                            </>
                        )}
                    </div>
                </section>
            </div>

            <section>
                <h2>{t("home.categories")}</h2>
                <ul className="cats">
                    {stats.map((s) => (
                        <li key={s.top}>
                            <button className="link" onClick={() => p.onOpenCategory(s.top)}>{categoryLabel(s.top)}</button>
                            <span className="bar" aria-hidden><i style={{width: `${(100 * s.have) / s.total}%`}}/></span>
                            <span className="mono muted">{s.have}/{s.total}</span>
                        </li>
                    ))}
                </ul>
            </section>
        </div>
    );
}
