import {useEffect, useId, useMemo, useState} from "react";
import {api, errText} from "../api";
import {Icon, type IconName} from "../components/Icon";
import {ConfirmDialog} from "../components/Modals";
import {formatBytes} from "../lib/format";
import {
    batterySeries, dateText, driveSeries, plotX, plotY, pctText, scaleFor, scoreSeries, statementText, usable, VERDICT_KEY, whText,
    type BatteryInfo, type ChartSeries, type HHSnapshot, type HHView,
} from "../lib/hhist";
import {getLang, t} from "../lib/i18n";
import "../hhist.css";

const W = 640, H = 220, LEFT = 56, RIGHT = 52, TOP = 12, BOTTOM = 30;
const DASH = ["", "7 4", "2 3", "10 3 2 3"];

function dateTimeText(ms: number): string {
    return Number.isFinite(ms) ? new Intl.DateTimeFormat(getLang(), {dateStyle: "medium", timeStyle: "short"}).format(new Date(ms)) : "";
}

function Marker({shape, x, y}: { shape: number; x: number; y: number }) {
    const common = {className: "hh-marker", strokeWidth: 2};
    switch (shape % 4) {
        case 1: return <rect {...common} x={x - 4.5} y={y - 4.5} width={9} height={9}/>;
        case 2: return <polygon {...common} points={`${x},${y - 5.5} ${x + 5.5},${y + 4.5} ${x - 5.5},${y + 4.5}`}/>;
        case 3: return <polygon {...common} points={`${x},${y - 6} ${x + 6},${y} ${x},${y + 6} ${x - 6},${y}`}/>;
        default: return <circle {...common} cx={x} cy={y} r={4.5}/>;
    }
}

/**
 * A line chart that does not rely on colour: each series has its own marker shape and dash pattern and
 * is named at the end of its line; the same numbers are available as a table, and the figure has a
 * text summary for screen readers.
 */
export function LineChart(p: {
    name: string; series: ChartSeries[]; format: (v: number) => string; fixed?: { y0: number; y1: number };
}) {
    const id = useId();
    const [table, setTable] = useState(false);
    const series = p.series.map((s) => ({...s, points: usable(s.points)})).filter((s) => s.points.length > 0);
    const scale = scaleFor(series, p.fixed);
    if (!scale) return null;
    const pw = W - LEFT - RIGHT, ph = H - TOP - BOTTOM;
    const ticks = [0, 0.5, 1].map((f) => scale.y0 + (scale.y1 - scale.y0) * f);
    const all = series.flatMap((s) => s.points);
    const first = Math.min(...all.map((q) => q.x)), last = Math.max(...all.map((q) => q.x));
    const ends = (s: ChartSeries) => ({first: p.format(s.points[0].y), last: p.format(s.points[s.points.length - 1].y)});
    const summary = series.length === 1
        ? (series[0].points.length === 1
            ? t("hh.chart.summary1", {name: p.name, from: dateText(first), last: ends(series[0]).last})
            : t("hh.chart.summary", {name: p.name, n: series[0].points.length, from: dateText(first), to: dateText(last), ...ends(series[0])}))
        : `${p.name}. ${series.map((s) => t("hh.chart.series", {name: s.name, ...ends(s)})).join(". ")}.`;
    const rows = [...new Set(all.map((q) => q.x))].sort((a, b) => a - b);
    const at = (s: ChartSeries, x: number) => s.points.find((q) => q.x === x);

    return (
        <figure className="hh-chart">
            <figcaption><b>{p.name}</b></figcaption>
            <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={p.name} aria-describedby={`${id}-d`} className="hh-svg">
                <desc id={`${id}-d`}>{summary}</desc>
                {ticks.map((v, i) => {
                    const y = plotY(v, scale, TOP, ph);
                    return (
                        <g key={i}>
                            <line className="hh-grid" x1={LEFT} x2={LEFT + pw} y1={y} y2={y}/>
                            <text className="hh-tick" x={LEFT - 6} y={y + 4} textAnchor="end">{p.format(v)}</text>
                        </g>
                    );
                })}
                <line className="hh-axis" x1={LEFT} x2={LEFT + pw} y1={TOP + ph} y2={TOP + ph}/>
                <text className="hh-tick" x={LEFT} y={H - 8} textAnchor="start">{dateText(first)}</text>
                {last > first && <text className="hh-tick" x={LEFT + pw} y={H - 8} textAnchor="end">{dateText(last)}</text>}
                {series.map((s, i) => {
                    const pts = s.points.map((q) => [plotX(q.x, scale, LEFT, pw), plotY(q.y, scale, TOP, ph)] as const);
                    const end = pts[pts.length - 1];
                    return (
                        <g key={s.id} className={i % 2 ? "hh-s hh-alt" : "hh-s"}>
                            {pts.length > 1 && <polyline className="hh-line" fill="none" strokeDasharray={DASH[i % 4]} points={pts.map(([x, y]) => `${x},${y}`).join(" ")}/>}
                            {pts.map(([x, y], k) => <Marker key={k} shape={i} x={x} y={y}/>)}
                            {series.length > 1 && <text className="hh-endlabel" x={end[0] + 9} y={end[1] + 4}>{s.name}</text>}
                        </g>
                    );
                })}
            </svg>
            {series.length > 1 && (
                <ul className="hh-legend">
                    {series.map((s, i) => (
                        <li key={s.id}>
                            <svg width="34" height="14" aria-hidden className={i % 2 ? "hh-s hh-alt" : "hh-s"}>
                                <line className="hh-line" x1="2" x2="32" y1="7" y2="7" strokeDasharray={DASH[i % 4]}/>
                                <Marker shape={i} x={17} y={7}/>
                            </svg>
                            {t("hh.chart.series", {name: s.name, ...ends(s)})}
                        </li>
                    ))}
                </ul>
            )}
            <button className="sm ghost" aria-expanded={table} aria-controls={`${id}-table`} onClick={() => setTable(!table)}>
                {table ? t("hh.table.hide") : t("hh.table.show")}
            </button>
            {table && (
                <div className="tablewrap hh-table" id={`${id}-table`}>
                    <table>
                        <caption className="sr-only">{t("hh.table.caption", {name: p.name})}</caption>
                        <thead><tr><th scope="col">{t("hh.table.date")}</th>{series.map((s) => <th scope="col" key={s.id}>{s.name}</th>)}</tr></thead>
                        <tbody>
                            {rows.map((x) => (
                                <tr key={x}>
                                    <th scope="row">{dateTimeText(x)}</th>
                                    {series.map((s) => { const q = at(s, x); return <td key={s.id} className="mono">{q ? p.format(q.y) : t("hh.table.none")}</td>; })}
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
        </figure>
    );
}

const gbFormat = (gb: number) => formatBytes(gb * 2 ** 30);

function DriveReadings({snap}: { snap: HHSnapshot }) {
    const disks = snap.disks ?? [];
    if (disks.length === 0) return null;
    const val = (v: number | null | undefined, unit: string) => (v === null || v === undefined ? t("hh.drives.notReported") : `${v}${unit}`);
    return (
        <div className="hh-drives">
            <h3>{t("hh.drives.title")}</h3>
            <div className="tablewrap">
                <table>
                    <thead><tr>
                        <th scope="col">{t("hh.drives.drive")}</th><th scope="col">{t("hh.drives.health")}</th><th scope="col">{t("hh.drives.wear")}</th>
                        <th scope="col">{t("hh.drives.temp")}</th><th scope="col">{t("hh.drives.hours")}</th>
                    </tr></thead>
                    <tbody>
                        {disks.map((d) => (
                            <tr key={d.name}>
                                <th scope="row">{d.name}</th><td>{d.health || t("hh.drives.notReported")}</td>
                                <td className="mono">{val(d.wearPct, "%")}</td><td className="mono">{val(d.tempC, " °C")}</td><td className="mono">{val(d.powerOnHours, "")}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
            {disks.some((d) => d.wearPct === null || d.tempC === null) && <p className="muted small">{t("hh.drives.adminNote")}</p>}
        </div>
    );
}

/** The History section: score over time, free space per drive, battery capacity and plain statements, with a way to clear it. */
export function HistorySection(p: { refreshKey: string }) {
    const [view, setView] = useState<HHView | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [ask, setAsk] = useState(false);
    const [version, setVersion] = useState(0);
    const [notice, setNotice] = useState("");

    useEffect(() => {
        let live = true;
        api.HealthHistory().then((v) => { if (live) { setView(v); setError(null); } }, (e) => { if (live) setError(errText(e)); });
        return () => { live = false; };
    }, [p.refreshKey, version]);

    const snaps = view?.snapshots ?? [];
    const score = useMemo(() => scoreSeries(snaps), [view]);
    const drives = useMemo(() => driveSeries(snaps), [view]);
    const bat = useMemo(() => batterySeries(snaps), [view]);
    const maxTotal = Math.max(1, ...snaps.map((s) => (Number.isFinite(s.total) ? s.total : 0)));
    const statements = (view?.statements ?? []).map(statementText).filter(Boolean);

    const clear = async () => {
        try { await api.HealthHistoryClear(); setNotice(t("hh.cleared")); setVersion((v) => v + 1); } catch (e) { setNotice(errText(e)); }
    };

    return (
        <section className="hh" aria-labelledby="hh-title">
            <div className="sectionhead">
                <h2 id="hh-title"><Icon name="pulse" size={18}/> {t("hh.title")}</h2>
                {snaps.length > 0 && <button className="ghost" onClick={() => setAsk(true)}><Icon name="trash" size={14}/> {t("hh.clear")}</button>}
            </div>
            <p className="muted">{t("hh.intro")}</p>
            {notice && <p className="muted" role="status">{notice}</p>}
            {error && <p className="muted" role="alert">{t("hh.error", {error})}</p>}
            {!view && !error && <p className="muted" role="status">{t("hh.loading")}</p>}
            {view && snaps.length === 0 && <p className="muted">{t("hh.empty")}</p>}
            {snaps.length > 0 && (
                <>
                    <p className="muted small mono">{t("hh.count", {n: snaps.length})}</p>
                    <h3>{t("hh.trends")}</h3>
                    {view?.enough && statements.length > 0
                        ? <ul className="hh-statements">{statements.map((s, i) => <li key={i}>{s}</li>)}</ul>
                        : <p className="muted">{t("hh.notEnough")}</p>}
                    <div className="hh-charts">
                        <LineChart name={t("hh.chart.score")} series={[score]} format={(v) => String(Math.round(v))} fixed={{y0: 0, y1: maxTotal}}/>
                        <LineChart name={t("hh.chart.free")} series={drives} format={gbFormat}/>
                        <LineChart name={t("hh.chart.battery")} series={[bat]} format={(v) => pctText(Math.round(v * 10) / 10)} fixed={{y0: 0, y1: 100}}/>
                    </div>
                    <DriveReadings snap={snaps[snaps.length - 1]}/>
                </>
            )}
            {ask && <ConfirmDialog title={t("hh.clear.title")} body={t("hh.clear.body")} confirm={t("hh.clear.confirm")} danger onConfirm={() => void clear()} onClose={() => setAsk(false)}/>}
        </section>
    );
}

const VERDICT_ICON: Record<BatteryInfo["verdict"], IconName> = {good: "check", worn: "alert", replace: "x", unknown: "help"};

/** The battery card: nothing at all while loading, a short note on a desktop, a plain verdict on a laptop. */
export function BatteryCard(p: { refreshKey: string }) {
    const [info, setInfo] = useState<BatteryInfo | null>(null);
    const [error, setError] = useState<string | null>(null);
    useEffect(() => {
        let live = true;
        setInfo(null); setError(null);
        api.Battery().then((b) => { if (live) setInfo(b); }, (e) => { if (live) setError(errText(e)); });
        return () => { live = false; };
    }, [p.refreshKey]);

    if (error) return <section className="hh-battery"><h2><Icon name="heart" size={18}/> {t("hh.battery.title")}</h2><p className="muted">{t("hh.battery.error", {error})}</p></section>;
    if (!info) return null;
    if (!info.present) return <section className="hh-battery"><h2><Icon name="heart" size={18}/> {t("hh.battery.title")}</h2><p className="muted">{t("hh.battery.none")}</p></section>;

    const known = info.verdict !== "unknown";
    const none = t("hh.battery.notReported");
    const stat = (label: string, value: string) => <div className="spec"><div><small>{label}</small><b>{value || none}</b></div></div>;
    return (
        <section className="hh-battery" aria-labelledby="hh-battery-title">
            <h2 id="hh-battery-title"><Icon name="heart" size={18}/> {t("hh.battery.title")}</h2>
            <div className={`hh-verdict ${info.verdict}`}>
                <span className="sev"><Icon name={VERDICT_ICON[info.verdict]} size={18}/></span>
                <p>{t(VERDICT_KEY[info.verdict], {pct: pctText(info.healthPct)})}</p>
            </div>
            <div className="specs">
                {stat(t("hh.battery.design"), known ? whText(info.designMWh) : "")}
                {stat(t("hh.battery.full"), known ? whText(info.fullMWh) : "")}
                {stat(t("hh.battery.health"), known ? pctText(info.healthPct) : "")}
                {stat(t("hh.battery.cycles"), info.cycles ? String(info.cycles) : "")}
            </div>
            {info.batteries.length > 1 && (
                <ul className="plain">
                    {info.batteries.map((b, i) => (
                        <li key={i}>{t("hh.battery.pack", {n: i + 1})}: {b.verdict === "unknown" ? none : `${pctText(b.healthPct)} · ${whText(b.fullMWh)} / ${whText(b.designMWh)}`}</li>
                    ))}
                </ul>
            )}
            <p className="muted small">{t("hh.battery.how", {good: info.goodMinPct, replace: info.replaceBelowPct})}</p>
            {info.history.length > 0 && (
                <>
                    <h3>{t("hh.battery.history")}</h3>
                    <div className="tablewrap">
                        <table>
                            <thead><tr><th scope="col">{t("hh.battery.col.date")}</th><th scope="col">{t("hh.battery.col.full")}</th><th scope="col">{t("hh.battery.col.health")}</th></tr></thead>
                            <tbody>
                                {info.history.map((h) => (
                                    <tr key={h.date}><th scope="row">{dateText(Date.parse(h.date))}</th><td className="mono">{whText(h.fullMWh)}</td><td className="mono">{pctText(h.healthPct)}</td></tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                </>
            )}
        </section>
    );
}
