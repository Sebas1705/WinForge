import {Icon} from "../../components/Icon";
import type {StorageVolume} from "../../api";
import {formatBytes, percent} from "../../lib/format";
import {t} from "../../lib/i18n";

/** Below this share of free space a drive is flagged. */
export const LOW_FREE_PCT = 10;

export const isLow = (v: StorageVolume): boolean => v.totalBytes > 0 && percent(v.freeBytes, v.totalBytes) < LOW_FREE_PCT;

function Volume({v}: { v: StorageVolume }) {
    const known = v.totalBytes > 0;
    const free = Math.min(Math.max(v.freeBytes, 0), v.totalBytes);
    const used = known ? v.totalBytes - free : 0;
    const usedPct = known ? percent(used, v.totalBytes) : 0;
    const freePct = known ? percent(free, v.totalBytes) : 0;
    const low = isLow(v);
    const name = v.label ? `${v.letter} ${v.label}` : v.letter;
    return (
        <li className={"volume" + (low ? " low" : "")}>
            <div className="volhead">
                <b>{name}</b>
                <span className="muted small">
                    {known ? t("storage.volume.line", {used: formatBytes(used), total: formatBytes(v.totalBytes), free: formatBytes(free)}) : t("storage.volume.unknown")}
                </span>
                {known && <span className="grow volpct small"><span className="sr-only">. </span>{t("storage.volume.freePct", {pct: freePct})}</span>}
            </div>
            <div className="volbar" role="meter" aria-label={t("storage.volume.usedLabel", {drive: name, pct: usedPct})}
                 aria-valuemin={0} aria-valuemax={100} aria-valuenow={usedPct}>
                <i style={{width: `${usedPct}%`}}/>
            </div>
            {low && <p className="volwarn small" role="note"><Icon name="alert" size={14}/> {t("storage.volume.low", {free: formatBytes(free)})}</p>}
        </li>
    );
}

/** One bar per drive: how full it is, with a warning when it is nearly full. */
export function Overview(p: { volumes: StorageVolume[] | null; onRefresh: () => void }) {
    return (
        <section aria-labelledby="storage-volumes">
            <div className="sectionhead">
                <h2 id="storage-volumes">{t("storage.volumes.title")}</h2>
                <button className="sm" onClick={p.onRefresh}><Icon name="refresh" size={13}/> {t("storage.volumes.refresh")}</button>
            </div>
            {p.volumes === null ? <p className="muted">{t("rail.scanning")}</p> : p.volumes.length === 0 ? (
                <p className="muted">{t("storage.volumes.none")}</p>
            ) : (
                <ul className="volumes">{p.volumes.map((v) => <Volume key={v.letter} v={v}/>)}</ul>
            )}
        </section>
    );
}
