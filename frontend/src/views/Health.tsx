import {useEffect, useMemo, useRef, useState} from "react";
import {Icon, type IconName} from "../components/Icon";
import {Strip} from "../components/Strip";
import {Ring, Tip} from "../components/Visual";
import {t, type Key} from "../lib/i18n";
import {
    byGroup, filterDrivers, findingText, isThirdParty,
    type Finding, type Group, type HealthLink, type HealthResult, type Severity,
} from "../lib/health";
import type {App} from "../lib/model";

const SEV_ICON: Record<Severity, IconName> = {ok: "check", info: "info", warn: "alert", bad: "x", unknown: "help"};
const SEV_CELL: Record<Severity, string> = {ok: "", info: "info", warn: "warn", bad: "failed", unknown: "skipped"};
const GROUP_ICON: Record<Group, IconName> = {firmware: "bios", drivers: "cpu", storage: "disk", security: "shield", windows: "windows"};
const RANK: Record<Severity, number> = {bad: 0, warn: 1, info: 2, unknown: 3, ok: 4};

/** Tone of a set of findings: the worst thing in it. */
function tone(fs: Finding[]): "ok" | "warn" | "bad" | "muted" {
    if (fs.some((f) => f.severity === "bad")) return "bad";
    if (fs.some((f) => f.severity === "warn")) return "warn";
    return fs.length ? "ok" : "muted";
}

export function Health(p: {
    result: HealthResult | null; busy: boolean; updatesBusy: boolean; admin: boolean; advanced: boolean; byId: Map<string, App>;
    onScan: () => void; onUpdates: () => void; onExport: () => void; onAdmin: () => void;
    onLink: (l: HealthLink) => void; onInstallApp: (a: App) => void; onDetail: (a: App) => void;
}) {
    const [query, setQuery] = useState("");
    const [vendorOnly, setVendorOnly] = useState(true);
    const [old, setOld] = useState(false);
    const [showDrivers, setShowDrivers] = useState(p.advanced);
    useEffect(() => { setShowDrivers(p.advanced); }, [p.advanced]);
    const sections = useRef<Record<string, HTMLElement | null>>({});
    const r = p.result;
    const now = useMemo(() => new Date(), [r]);
    const drivers = useMemo(
        () => (r ? filterDrivers(r.report.drivers, {query, thirdPartyOnly: vendorOnly, olderThanYears: old ? 2 : null}, now) : []),
        [r, query, vendorOnly, old, now]);

    const attention = r ? r.findings.filter((f) => f.severity === "bad" || f.severity === "warn").sort((a, b) => RANK[a.severity] - RANK[b.severity]) : [];
    const unknown = r ? r.findings.filter((f) => f.severity === "unknown").length : 0;
    const overall = r ? tone(r.findings) : "muted";
    const headline = overall === "bad" ? t("health.score.bad") : attention.length > 0 ? t("health.score.attention", {n: attention.length}) : t("health.score.good");

    return (
        <div className="page health">
            <header className="pagehead">
                <div>
                    <h1>{t("health.title")} <Tip text={t("health.help")}/></h1>
                    <p className="muted">{t("health.readOnly")}</p>
                </div>
                <span className="grow"/>
                <button className="primary" disabled={p.busy} onClick={p.onScan}><Icon name="refresh" size={15}/> {p.busy ? t("health.scanning") : t("health.scan")}</button>
                <button disabled={p.busy || p.updatesBusy} onClick={p.onUpdates}>{p.updatesBusy ? t("health.checkingUpdates") : t("health.checkUpdates")}</button>
                <button className="ghost" disabled={!r} onClick={p.onExport}>{t("health.export")}</button>
            </header>

            {!r && !p.busy && <p className="muted empty">{t("health.empty")}</p>}
            {!r && p.busy && <div className="scanning"><Ring size={72} stroke={7} value={0.3} tone="accent"><Icon name="pulse" size={26}/></Ring><span>{t("health.scanning")}</span></div>}

            {r && (
                <>
                    <section className="scorecard">
                        <Ring size={132} stroke={13} value={r.total ? r.ok / r.total : 0} tone={overall}>
                            <span className="ringnum big">{r.ok}<small>/{r.total}</small></span>
                        </Ring>
                        <div className="grow">
                            <h2>{headline}</h2>
                            <p className="muted">{r.ok} {t("health.summaryOf", {total: r.total})}</p>
                            <Strip size="lg" label={`${r.ok}/${r.total}`}
                                   cells={r.findings.map((f) => ({on: f.severity === "ok", state: SEV_CELL[f.severity], name: findingText(f, "title")}))}/>
                            {unknown > 0 && !p.admin && (
                                <p className="muted small hint"><Icon name="lock" size={14}/> {t("health.unknownNote", {n: unknown})}
                                    <button className="mini" onClick={p.onAdmin}>{t("health.restartAdmin")}</button></p>
                            )}
                        </div>
                    </section>

                    <div className="areas">
                        {byGroup(r.findings).map(([g, list]) => {
                            const tn = tone(list);
                            const issues = list.filter((f) => f.severity === "bad" || f.severity === "warn").length;
                            return (
                                <button key={g} className={`area ${tn}`} onClick={() => (sections.current[g] ?? sections.current.attention)?.scrollIntoView({behavior: "smooth", block: "start"})}>
                                    <span className="bubble"><Icon name={GROUP_ICON[g]} size={22}/></span>
                                    <b>{t(`group.${g}` as Key)}</b>
                                    <span className="status"><Icon name={issues ? "alert" : "check"} size={14}/>{issues ? issues : ""}</span>
                                </button>
                            );
                        })}
                    </div>

                    {attention.length > 0 && (
                        <section ref={(el) => { sections.current.attention = el; }}>
                            <h2><Icon name="alert" size={18}/> {t("health.fix")}</h2>
                            <ul className="findings">
                                {attention.map((f, i) => <FindingCard key={f.key + i} f={f} {...p} open/>)}
                            </ul>
                        </section>
                    )}

                    <Specs r={r}/>

                    {/* What needs fixing is shown once, above; groups list the rest. */}
                    {byGroup(r.findings).map(([g, list]) => {
                        const rest = list.filter((f) => f.severity !== "bad" && f.severity !== "warn");
                        if (rest.length === 0) return null;
                        return (
                            <section key={g} ref={(el) => { sections.current[g] = el; }}>
                                <h2><Icon name={GROUP_ICON[g]} size={18}/> {t(`group.${g}` as Key)}</h2>
                                <ul className="findings compact">
                                    {[...rest].sort((a, b) => RANK[a.severity] - RANK[b.severity]).map((f, i) => (
                                        <FindingCard key={f.key + i} f={f} {...p}/>
                                    ))}
                                </ul>
                            </section>
                        );
                    })}

                    <section>
                        <h2><Icon name="download" size={18}/> {t("health.updates")}</h2>
                        {r.updates === null ? <p className="muted">{t("health.updatesNotChecked")}</p>
                            : r.updates.updates.length === 0 ? <p className="muted"><Icon name="check" size={14}/> {t("health.updatesNone")}</p>
                            : (
                                <ul className="plain">
                                    {r.updates.updates.map((u, i) => (
                                        <li key={i}><span className="pill">{u.kind === "Driver" ? t("health.driverKind") : "Windows"}</span> {u.title}
                                            {p.advanced && u.sizeMB > 0 && <span className="mono muted"> {u.sizeMB} MB</span>}</li>
                                    ))}
                                </ul>
                            )}
                    </section>

                    <section>
                        <div className="sectionhead">
                            <h2><Icon name="cpu" size={18}/> {t("health.drivers")}</h2>
                            <button className="ghost" onClick={() => setShowDrivers(!showDrivers)}>{showDrivers ? t("health.hideDrivers") : t("health.showAllDrivers")}</button>
                        </div>
                        {showDrivers && (
                            <>
                                <div className="toolbar tight">
                                    <div className="searchbox"><Icon name="search" size={16}/>
                                        <input className="search" placeholder={t("health.searchDrivers")} value={query} onChange={(e) => setQuery(e.target.value)}/></div>
                                    <label className="check"><input type="checkbox" checked={vendorOnly} onChange={(e) => setVendorOnly(e.target.checked)}/> {t("health.thirdPartyOnly")}</label>
                                    <label className="check"><input type="checkbox" checked={old} onChange={(e) => setOld(e.target.checked)}/> {t("health.oldOnly")}</label>
                                </div>
                                <p className="mono muted small">{t("health.driverCount", {shown: drivers.length, total: r.report.drivers.length})}</p>
                                <div className="tablewrap">
                                    <table>
                                        <thead><tr>
                                            <th>{t("health.col.device")}</th><th>{t("health.col.class")}</th><th>{t("health.col.maker")}</th>
                                            <th>{t("health.col.version")}</th><th>{t("health.col.date")}</th>
                                        </tr></thead>
                                        <tbody>
                                            {drivers.map((d, i) => (
                                                <tr key={d.device + d.inf + i} className={!d.signed ? "unsigned" : ""}>
                                                    <td>{d.device}</td><td className="mono">{d.class}</td><td>{d.manufacturer}</td>
                                                    <td className="mono">{d.version}</td>
                                                    <td className="mono">{d.date}{isThirdParty(d) ? "" : " ·"}</td>
                                                </tr>
                                            ))}
                                        </tbody>
                                    </table>
                                </div>
                            </>
                        )}
                    </section>
                </>
            )}
        </div>
    );
}

function FindingCard(p: {
    f: Finding; open?: boolean; advanced: boolean; byId: Map<string, App>;
    onLink: (l: HealthLink) => void; onInstallApp: (a: App) => void; onDetail: (a: App) => void;
}) {
    const {f} = p;
    const [open, setOpen] = useState(!!p.open);
    const app = f.catalogId ? p.byId.get(f.catalogId) : undefined;
    const hasMore = f.severity !== "ok" && ((f.links?.length ?? 0) > 0 || !!app);
    return (
        <li className={`finding ${f.severity}`}>
            <span className="sev"><Icon name={SEV_ICON[f.severity]} size={16}/></span>
            <div className="grow">
                <button className="link fhead" onClick={() => setOpen(!open)} aria-expanded={open}>
                    <b>{findingText(f, "title")}</b>
                    <Icon name="chevron" size={14} className={open ? "turn" : ""}/>
                </button>
                {open && <p className="muted">{findingText(f, "detail")}</p>}
                {open && hasMore && (
                    <div className="actions tight">
                        {f.links?.map((l) => (
                            <button key={l.url} className="sm" onClick={() => p.onLink(l)}>{t(`link.${l.kind}` as "link.support", {label: l.label})} <Icon name="external" size={13}/></button>
                        ))}
                        {app && !app.installed && <button className="sm primary" onClick={() => p.onInstallApp(app)}>{t("health.installTool", {name: app.name})}</button>}
                        {app && app.installed && <button className="sm" onClick={() => p.onDetail(app)}>{app.name}</button>}
                    </div>
                )}
            </div>
        </li>
    );
}

function Specs({r}: { r: HealthResult }) {
    const x = r.report;
    const ram = x.memory.modules.length > 0
        ? `${x.memory.totalGB} GB · ${t("health.ramModules", {n: x.memory.modules.length})}${x.memory.modules[0].configuredMHz ? ` · ${x.memory.modules[0].configuredMHz} MHz` : ""}`
        : `${x.memory.totalGB} GB`;
    const rows: [IconName, string, string][] = [
        ["monitor", t("health.machine"), `${x.machine.manufacturer} ${x.machine.model}${x.machine.type ? ` · ${x.machine.type}` : ""}`],
        ["windows", t("health.os"), `${x.os.caption} · build ${x.os.build}`],
        ["cpu", t("health.cpu"), `${x.cpu.name} · ${x.cpu.cores}/${x.cpu.threads}`],
        ["memory", t("health.ram"), ram],
        ...x.gpus.map((g): [IconName, string, string] => ["monitor", t("health.gpu"), `${g.name} · ${g.driverVersion}`]),
        ["board", t("health.board"), `${x.board.manufacturer} ${x.board.product}`],
        ["bios", t("health.bios"), `${x.bios.vendor} ${x.bios.version} · ${x.bios.date}`],
        ...x.disks.map((d): [IconName, string, string] => ["disk", t("health.disks"), `${d.name} · ${d.sizeGB} GB${d.media ? ` · ${d.media}` : ""}`]),
    ];
    return (
        <section>
            <div className="specs">
                {rows.map(([icon, k, v], i) => (
                    <div key={i} className="spec">
                        <span className="bubble sm"><Icon name={icon} size={18}/></span>
                        <div><small>{k}</small><b>{v}</b></div>
                    </div>
                ))}
            </div>
        </section>
    );
}
