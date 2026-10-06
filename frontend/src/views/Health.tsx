import {useMemo, useState} from "react";
import {Strip} from "../components/Strip";
import {t} from "../lib/i18n";
import {
    byGroup, filterDrivers, findingText, isThirdParty,
    type Finding, type HealthLink, type HealthResult, type Severity,
} from "../lib/health";
import type {App} from "../lib/model";

const SEV_LABEL: Record<Severity, string> = {ok: "✓", info: "i", warn: "!", bad: "✗", unknown: "?"};
const SEV_CELL: Record<Severity, string> = {ok: "", info: "info", warn: "warn", bad: "failed", unknown: "skipped"};

export function Health(p: {
    result: HealthResult | null; busy: boolean; updatesBusy: boolean; admin: boolean; byId: Map<string, App>;
    onScan: () => void; onUpdates: () => void; onExport: () => void; onAdmin: () => void;
    onLink: (l: HealthLink) => void; onInstallApp: (a: App) => void; onDetail: (a: App) => void;
}) {
    const [query, setQuery] = useState("");
    const [vendorOnly, setVendorOnly] = useState(true);
    const [old, setOld] = useState(false);
    const r = p.result;
    const now = useMemo(() => new Date(), [r]);
    const drivers = useMemo(
        () => (r ? filterDrivers(r.report.drivers, {query, thirdPartyOnly: vendorOnly, olderThanYears: old ? 2 : null}, now) : []),
        [r, query, vendorOnly, old, now]);
    const unknown = r ? r.findings.filter((f) => f.severity === "unknown").length : 0;

    return (
        <div className="page health">
            <header className="pagehead">
                <div>
                    <h1>{t("health.title")}</h1>
                    <p className="muted">{t("health.readOnly")}</p>
                </div>
                <span className="grow"/>
                <button className="primary" disabled={p.busy} onClick={p.onScan}>{p.busy ? t("health.scanning") : t("health.scan")}</button>
                <button disabled={p.busy || p.updatesBusy} onClick={p.onUpdates}>{p.updatesBusy ? t("health.checkingUpdates") : t("health.checkUpdates")}</button>
                <button className="ghost" disabled={!r} onClick={p.onExport}>{t("health.export")}</button>
            </header>

            {!r && !p.busy && <p className="muted empty">{t("health.empty")}</p>}

            {r && (
                <>
                    <section className="verdict">
                        <div>
                            <em>{r.ok}</em> <span>{t("health.summaryOf", {total: r.total})}</span>
                        </div>
                        <Strip size="lg" label={`${r.ok}/${r.total}`}
                               cells={r.findings.map((f) => ({on: f.severity === "ok", state: SEV_CELL[f.severity], name: findingText(f, "title")}))}/>
                        {unknown > 0 && !p.admin && (
                            <p className="muted small">{t("health.unknownNote", {n: unknown})} <button className="mini" onClick={p.onAdmin}>{t("health.restartAdmin")}</button></p>
                        )}
                    </section>

                    <Machine r={r}/>

                    {byGroup(r.findings).map(([g, list]) => (
                        <section key={g}>
                            <h2>{t(`group.${g}` as "group.firmware")}</h2>
                            <ul className="findings">
                                {[...list].sort(worstFirst).map((f, i) => (
                                    <FindingRow key={f.key + i} f={f} byId={p.byId} onLink={p.onLink} onInstallApp={p.onInstallApp} onDetail={p.onDetail}/>
                                ))}
                            </ul>
                        </section>
                    ))}

                    <section>
                        <h2>{t("health.updates")}</h2>
                        {r.updates === null ? <p className="muted">{t("health.updatesNotChecked")}</p>
                            : r.updates.updates.length === 0 ? <p className="muted">{t("health.updatesNone")}</p>
                            : (
                                <ul className="plain">
                                    {r.updates.updates.map((u, i) => (
                                        <li key={i}><span className="pill">{u.kind === "Driver" ? t("health.driverKind") : "Windows"}</span> {u.title}
                                            {u.sizeMB > 0 && <span className="mono muted"> {u.sizeMB} MB</span>}</li>
                                    ))}
                                </ul>
                            )}
                    </section>

                    <section>
                        <h2>{t("health.drivers")}</h2>
                        <div className="toolbar tight">
                            <input className="search" placeholder={t("health.searchDrivers")} value={query} onChange={(e) => setQuery(e.target.value)}/>
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
                    </section>
                </>
            )}
        </div>
    );
}

const worstRank: Record<Severity, number> = {bad: 0, warn: 1, info: 2, unknown: 3, ok: 4};
const worstFirst = (a: Finding, b: Finding) => worstRank[a.severity] - worstRank[b.severity];

function FindingRow(p: {
    f: Finding; byId: Map<string, App>;
    onLink: (l: HealthLink) => void; onInstallApp: (a: App) => void; onDetail: (a: App) => void;
}) {
    const {f} = p;
    const app = f.catalogId ? p.byId.get(f.catalogId) : undefined;
    return (
        <li className={`finding ${f.severity}`}>
            <span className="sev" aria-label={f.severity}>{SEV_LABEL[f.severity]}</span>
            <div className="grow">
                <b>{findingText(f, "title")}</b>
                <p className="muted">{findingText(f, "detail")}</p>
                {f.severity !== "ok" && ((f.links?.length ?? 0) > 0 || app) && (
                    <div className="actions tight">
                        {f.links?.map((l) => (
                            <button key={l.url} className="sm" onClick={() => p.onLink(l)}>{t(`link.${l.kind}` as "link.support", {label: l.label})} ↗</button>
                        ))}
                        {app && !app.installed && <button className="sm primary" onClick={() => p.onInstallApp(app)}>{t("health.installTool", {name: app.name})}</button>}
                        {app && app.installed && <button className="sm" onClick={() => p.onDetail(app)}>{app.name}</button>}
                    </div>
                )}
            </div>
        </li>
    );
}

function Machine({r}: { r: HealthResult }) {
    const x = r.report;
    const ram = x.memory.modules.length > 0
        ? `${x.memory.totalGB} GB · ${t("health.ramModules", {n: x.memory.modules.length})}${x.memory.modules[0].configuredMHz ? ` · ${x.memory.modules[0].configuredMHz} MHz` : ""}`
        : `${x.memory.totalGB} GB`;
    const rows: [string, string][] = [
        [t("health.machine"), `${x.machine.manufacturer} ${x.machine.model}${x.machine.type ? ` · ${x.machine.type}` : ""}`],
        [t("health.os"), `${x.os.caption} · build ${x.os.build} · ${x.os.arch}`],
        [t("health.cpu"), `${x.cpu.name} · ${x.cpu.cores}/${x.cpu.threads}`],
        [t("health.ram"), ram],
        ...x.gpus.map((g): [string, string] => [t("health.gpu"), `${g.name} · ${g.driverVersion}`]),
        [t("health.board"), `${x.board.manufacturer} ${x.board.product}`],
        [t("health.bios"), `${x.bios.vendor} ${x.bios.version} · ${x.bios.date}`],
        ...x.disks.map((d): [string, string] => [t("health.disks"), `${d.name} · ${d.sizeGB} GB${d.media ? ` · ${d.media}` : ""}`]),
    ];
    return (
        <section>
            <dl className="machine">
                {rows.map(([k, v], i) => (<div key={i}><dt>{k}</dt><dd>{v}</dd></div>))}
            </dl>
        </section>
    );
}

