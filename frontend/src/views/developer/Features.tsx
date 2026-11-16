import {useCallback, useEffect, useState} from "react";
import {api, errText, type Feature, type WSLInfo} from "../../api";
import {Icon, type IconName} from "../../components/Icon";
import {ConfirmDialog} from "../../components/Modals";
import {t, tOpt} from "../../lib/i18n";

const ICON: Record<string, IconName> = {wsl: "code", vmp: "cpu", hyperv: "monitor", sandbox: "shield", netfx3: "windows"};
const featureName = (id: string) => tOpt(`dev.feat.${id}`) ?? id;

function Wsl(p: { info: WSLInfo | null; error: string }) {
    const {info} = p;
    return (
        <>
            <h3 className="subhead">{t("dev.wsl.title")}</h3>
            {p.error && <p className="muted" role="alert">{t("dev.wsl.error", {error: p.error})}</p>}
            {info && !info.installed && <p className="muted small">{t("dev.wsl.notInstalled")}</p>}
            {info?.installed && (
                <>
                    <p className="small">{t("dev.wsl.installed", {version: info.defaultVersion})}</p>
                    {info.distros.length === 0 ? <p className="muted small">{t("dev.wsl.noDistros")}</p> : (
                        <ul className="cleanlist" aria-label={t("dev.wsl.distros")}>
                            {info.distros.map((d) => (
                                <li key={d.name}>
                                    <div className="finding">
                                        <span className="grow">
                                            <span className="catname">
                                                <b>{d.name}</b>
                                                {d.isDefault && <span className="chip done">{t("dev.wsl.default")}</span>}
                                                <span className="chip">{t("dev.wsl.version", {n: d.version})}</span>
                                                <span className="chip">{tOpt(`dev.wsl.state.${d.state.toLowerCase()}`) ?? d.state}</span>
                                            </span>
                                        </span>
                                    </div>
                                </li>
                            ))}
                        </ul>
                    )}
                </>
            )}
        </>
    );
}

/**
 * The Windows features development tools need (WSL, virtualization, Sandbox, .NET 3.5) and what WSL has installed.
 * Turning a feature on or off hands its recipe id to the repair runner, which asks for administrator rights.
 */
export function Features(p: { admin: boolean; say: (m: string) => void; onAdmin: () => void; onRun: (ids: string[]) => void; refreshKey?: number }) {
    const [features, setFeatures] = useState<Feature[] | null>(null);
    const [error, setError] = useState("");
    const [wsl, setWsl] = useState<WSLInfo | null>(null);
    const [wslError, setWslError] = useState("");
    const [asking, setAsking] = useState<Feature | null>(null);

    const load = useCallback(async () => {
        setError("");
        setWslError("");
        try { setFeatures(await api.FeaturesList()); } catch (e) { setError(errText(e)); setFeatures((v) => v ?? []); }
        try { setWsl(await api.WSLInfo()); } catch (e) { setWslError(errText(e)); }
    }, []);
    useEffect(() => { void load(); }, [load, p.refreshKey]);

    const list = features ?? [];
    const adminNeeded = !p.admin && list.some((f) => f.needsAdmin);

    return (
        <section aria-labelledby="dev-feat">
            <div className="sectionhead">
                <h2 id="dev-feat">{t("dev.feat.title")}</h2>
                <button onClick={() => void load()}><Icon name="refresh" size={15}/> {t("dev.feat.refresh")}</button>
            </div>
            <p className="muted small">{t("dev.feat.intro")}</p>

            {features === null && <p className="muted" role="status">{t("dev.feat.loading")}</p>}
            {error && <p className="muted" role="alert">{t("dev.feat.error", {error})}</p>}
            {features !== null && !error && list.length === 0 && <p className="muted" role="status">{t("dev.feat.empty")}</p>}
            {adminNeeded && (
                <p className="muted small hint adminhint"><Icon name="lock" size={14}/> <span>{t("dev.feat.adminHint")}</span>
                    <button className="mini" onClick={p.onAdmin}>{t("dev.feat.restartAdmin")}</button></p>
            )}

            {list.length > 0 && (
                <ul className="optcards" aria-label={t("dev.feat.list")}>
                    {list.map((f) => {
                        const name = featureName(f.id);
                        const usable = f.state === "on" || f.state === "off";
                        const turningOff = f.state === "on";
                        return (
                            <li key={f.id} className="optcard">
                                <span className="bubble sm"><Icon name={ICON[f.id] ?? "settings"} size={18}/></span>
                                <div className="grow">
                                    <span className="catname">
                                        <b>{name}</b>
                                        <span className={"chip" + (f.state === "on" ? " done" : "")}
                                              title={f.state === "unavailable" ? t("dev.feat.unavailableTip") : f.state === "unknown" ? t("dev.feat.unknownTip") : undefined}>
                                            {tOpt(`dev.feat.state.${f.state}`) ?? f.state}
                                        </span>
                                    </span>
                                    {tOpt(`dev.feat.${f.id}.body`) && <span className="muted small">{tOpt(`dev.feat.${f.id}.body`)}</span>}
                                    {f.restartPending && <span className="small optwarn"><Icon name="alert" size={13}/> {t("dev.feat.restart")}</span>}
                                </div>
                                <button className="sm" disabled={!usable}
                                        aria-label={t(turningOff ? "dev.feat.turnOffAria" : "dev.feat.turnOnAria", {name})}
                                        onClick={() => (turningOff ? setAsking(f) : p.onRun([f.enable]))}>
                                    <Icon name={turningOff ? "x" : "play"} size={13}/> {t(turningOff ? "dev.feat.turnOff" : "dev.feat.turnOn")}
                                </button>
                            </li>
                        );
                    })}
                </ul>
            )}

            <Wsl info={wsl} error={wslError}/>

            {asking && (
                <ConfirmDialog title={t("dev.feat.confirmTitle", {name: featureName(asking.id)})} danger confirm={t("dev.feat.confirm")}
                               body={t("dev.feat.confirmBody", {name: featureName(asking.id)})}
                               onConfirm={() => p.onRun([asking.disable])} onClose={() => setAsking(null)}/>
            )}
        </section>
    );
}
