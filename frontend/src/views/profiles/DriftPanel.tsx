import {useEffect, useId, useState} from "react";
import {api, errText, type DriftInfo} from "../../api";
import {Icon} from "../../components/Icon";
import {dayOf} from "../../lib/drift";
import {getLang, t} from "../../lib/i18n";
import type {ProfileInfo} from "../../lib/model";

/**
 * What changed on this PC since the profile was last applied: apps that went away, apps installed since that
 * the profile does not list, and versions that moved. Reading is all it does until a button is pressed, and
 * the buttons change the profile's reference or the person's own profile, never the PC.
 */
export function DriftPanel(p: {
    profile: ProfileInfo;
    /** Changes whenever the PC is scanned again, so the report is read again. */
    scan: unknown;
    onAdd: (ids: string[]) => Promise<void>;
    say: (m: string) => void;
}) {
    const [info, setInfo] = useState<DriftInfo | null>(null);
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);
    const [version, setVersion] = useState(0); // bumped to read the report again after a button
    const titleId = useId();
    const id = p.profile.id;
    const name = p.profile.name;

    useEffect(() => {
        let live = true;
        setError("");
        api.ProfileDrift(id).then((r) => { if (live) setInfo(r); }).catch((e) => { if (live) { setInfo(null); setError(errText(e)); } });
        return () => { live = false; };
    }, [id, p.scan, version]);

    const accept = async (message: string) => {
        setBusy(true);
        try {
            await api.AcceptProfileState(id);
            p.say(message);
            setVersion((v) => v + 1);
        } catch (e) { p.say(errText(e)); } finally { setBusy(false); }
    };
    const add = async () => {
        if (!info) return;
        setBusy(true);
        try {
            await p.onAdd(info.added.map((a) => a.id));
            p.say(t("drift.addedToast", {n: info.added.length, name}));
            setVersion((v) => v + 1);
        } catch (e) { p.say(errText(e)); } finally { setBusy(false); }
    };

    if (error) return <p className="muted small drift-note" role="note">{t("drift.error", {e: error})}</p>;
    if (!info) return null;
    const day = dayOf(info.at, getLang());

    if (!info.hasBaseline) {
        return (
            <section className="drift" aria-labelledby={titleId}>
                <p className="muted small" id={titleId}>{t("drift.noRecord")}</p>
                <div className="actions tight">
                    <button className="sm" disabled={busy} title={t("drift.rememberHint")} onClick={() => void accept(t("drift.accepted", {name}))}>{t("drift.remember")}</button>
                </div>
            </section>
        );
    }
    if (info.count === 0) {
        return <p className="muted small drift-ok"><Icon name="check" size={14}/> {t("drift.none", {date: day})}</p>;
    }

    return (
        <section className="drift" aria-labelledby={titleId}>
            <h2 id={titleId}>{t("drift.title", {date: day})}</h2>
            {info.removed.length > 0 && (
                <div>
                    <h3>{t("drift.removed")}</h3>
                    <ul className="plain">{info.removed.map((a) => <li key={a.id}>{a.name}</li>)}</ul>
                    <p className="muted small">{t("drift.removedHint")}</p>
                </div>
            )}
            {info.added.length > 0 && (
                <div>
                    <h3>{t("drift.added")}</h3>
                    <ul className="plain">{info.added.map((a) => <li key={a.id}>{a.name}</li>)}</ul>
                    {p.profile.builtin
                        ? <p className="muted small">{t("drift.addedBuiltin")}</p>
                        : <div className="actions tight"><button className="sm" disabled={busy} onClick={() => void add()}>{t("drift.add", {n: info.added.length})}</button></div>}
                </div>
            )}
            {info.updated.length > 0 && (
                <div>
                    <h3>{t("drift.updated")}</h3>
                    <ul className="plain">{info.updated.map((c) => <li key={c.id}>{c.name} <span className="mono muted">{t("updates.from", {from: c.from, to: c.to})}</span></li>)}</ul>
                </div>
            )}
            <div className="actions tight">
                <button className="sm" disabled={busy} title={t("drift.acceptHint")} onClick={() => void accept(t("drift.accepted", {name}))}>{t("drift.accept")}</button>
            </div>
        </section>
    );
}
