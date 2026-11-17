import {useEffect, useId, useMemo, useState} from "react";
import {api, errText} from "../api";
import {Icon, type IconName} from "../components/Icon";
import {
    adviceText, counts, dayText, lineText, monthsOld, replacementText,
    type EOLFinding, type EOLReport as Report, type EOLStatus,
} from "../lib/eol";
import {t} from "../lib/i18n";
import {usePageState} from "../lib/pageState";
import {usePageStateLive} from "../lib/pageStateLive";
import type {HealthLink} from "../lib/health";
import type {App} from "../lib/model";
import "../eol.css";

// The status is always an icon plus words, never a colour alone. The row classes reuse the PC health
// findings' (ok / warn / bad / unknown), so every style and theme keeps its contrast.
const ICON: Record<EOLStatus, IconName> = {ended: "x", soon: "alert", ok: "check", unknown: "help"};
const ROW: Record<EOLStatus, string> = {ended: "bad", soon: "warn", ok: "ok", unknown: "unknown"};

function Row(p: { f: EOLFinding; byId: Map<string, App>; onDetail: (a: App) => void; onLink: (l: HealthLink) => void }) {
    const {f} = p;
    const app = p.byId.get(f.appId);
    const name = app?.name ?? f.productName;
    const advice = adviceText(f);
    const offered = f.apps.map((id) => p.byId.get(id)).filter((a): a is App => !!a);
    const attention = f.status === "ended" || f.status === "soon";
    return (
        <li className={`finding ${ROW[f.status]} eol-row`}>
            <span className="sev"><Icon name={ICON[f.status]} size={16}/></span>
            <div className="grow eol-body">
                <div className="eol-head">
                    <b>{name}</b>
                    <span className="mono muted">{f.version ? t("eol.version", {version: f.version}) : t("eol.noVersion")}</span>
                    <span className="pill eol-status">{t(`eol.status.${f.status}` as "eol.status.ok")}</span>
                </div>
                <p>{lineText(f)}</p>
                {advice && <p className="muted">{advice}</p>}
                {attention && f.replace.length > 0 && (
                    <p className="muted">{t("eol.do.supported", {list: f.replace.map(replacementText).join("; ")})}</p>
                )}
                {attention && f.selfUpdate && <p className="muted">{t("eol.do.self")}</p>}
                <div className="actions tight">
                    {attention && offered.map((a) => (
                        <button key={a.id} className="sm primary" onClick={() => p.onDetail(a)}>{t("eol.open", {name: a.name})}</button>
                    ))}
                    {f.source && (
                        <button className="sm ghost" onClick={() => p.onLink({kind: "support", label: f.productName, url: f.source})}>
                            {t("eol.source", {name: f.productName})} <Icon name="external" size={13}/>
                        </button>
                    )}
                </div>
            </div>
        </li>
    );
}

/**
 * The "End of support" section of the PC health page: which installed software is on a release its
 * maker no longer supports (or soon will not), from a table bundled with this version of WinForge.
 * It reads that table and the last scan; nothing about the PC is sent anywhere, and the section says so.
 */
export function EOLSection(p: {
    byId: Map<string, App>; onDetail: (a: App) => void; onLink: (l: HealthLink) => void; refreshKey: string;
}) {
    // Kept while the app runs, with the key it was read for, so coming back shows it at once.
    const [report, setReport] = usePageStateLive<Report | null>("eol:report", null);
    const [error, setError] = usePageStateLive<string | null>("eol:error", null);
    const [readFor, setReadFor] = usePageState<string | null>("eol:readFor", null);
    const [showRest, setShowRest] = useState(false);
    const restId = useId();

    useEffect(() => {
        if (readFor === p.refreshKey && (report || error)) return;
        setReadFor(p.refreshKey);
        let live = true;
        Promise.resolve().then(() => api.EOLReport()).then((r) => { if (live) { setReport(r); setError(null); } }, (e) => { if (live) setError(errText(e)); });
        return () => { live = false; };
    }, [p.refreshKey]);

    const findings = report?.findings ?? [];
    const attention = useMemo(() => findings.filter((f) => f.status === "ended" || f.status === "soon"), [report]);
    const rest = useMemo(() => findings.filter((f) => f.status !== "ended" && f.status !== "soon"), [report]);
    const c = counts(findings);
    // With nothing to act on, the rows that are fine are the whole answer; otherwise they fold away.
    const restOpen = attention.length === 0 || showRest;
    const row = (f: EOLFinding) => <Row key={f.appId} f={f} byId={p.byId} onDetail={p.onDetail} onLink={p.onLink}/>;

    return (
        <section className="eol" aria-labelledby="eol-title">
            <div className="sectionhead">
                <h2 id="eol-title"><Icon name="archive" size={18}/> {t("eol.title")}</h2>
            </div>
            <p className="muted eol-note-text">{t("eol.basis", {date: report ? dayText(report.asOf) : "…"})}</p>
            <p className="muted eol-note-text">{t("eol.scope")}</p>
            <p className="muted eol-note-text">{t("eol.private")}</p>
            {error && <p className="muted" role="alert">{t("eol.error", {error})}</p>}
            {!report && !error && <p className="muted" role="status">{t("eol.loading")}</p>}
            {report?.stale && (
                <p className="eol-stale" role="status"><Icon name="alert" size={16}/> <span>{t("eol.stale", {months: monthsOld(report.ageDays)})}</span></p>
            )}
            {report && findings.length === 0 && <p className="muted"><Icon name="info" size={14}/> {t("eol.none")}</p>}
            {report && findings.length > 0 && (
                <>
                    <p className="mono small eol-summary">{t("eol.summary", c)}</p>
                    {attention.length === 0 && <p role="status"><Icon name="check" size={14}/> {t("eol.allFine")}</p>}
                    {attention.length > 0 && <ul className="findings eol-list">{attention.map(row)}</ul>}
                    {rest.length > 0 && (
                        <>
                            {attention.length > 0 && (
                                <button className="ghost eol-toggle" aria-expanded={showRest} aria-controls={restId} onClick={() => setShowRest(!showRest)}>
                                    <Icon name="chevron" size={14} className={showRest ? "turn" : ""}/> {t(showRest ? "eol.rest.hide" : "eol.rest.show", {n: rest.length})}
                                </button>
                            )}
                            {restOpen && <ul id={restId} className="findings eol-list">{rest.map(row)}</ul>}
                        </>
                    )}
                </>
            )}
        </section>
    );
}
