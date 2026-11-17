import {useCallback, useEffect, useRef, useState} from "react";
import {api, errText} from "../api";
import {t} from "../lib/i18n";
import "../diag.css";
import {useModal} from "./useModal";

/** What the Go side says when the chosen file is not a .json/.txt path. */
const PATH_REFUSED = "diag:path";

function saveProblem(e: unknown): string {
    const text = errText(e);
    return text.includes(PATH_REFUSED) ? t("diag.err.path") : t("diag.saveError", {error: text});
}

/**
 * The diagnostic report: what it holds and what it never holds, the exact text that would be saved, and
 * a button to save it. Nothing is written until the person presses Save and picks a file, and nothing is
 * ever sent: the dialog says so.
 */
export function DiagDialog(p: { onClose: () => void }) {
    const dialog = useRef<HTMLDivElement>(null);
    useModal(dialog, p.onClose);
    const [text, setText] = useState<string | null>(null);
    const [error, setError] = useState("");
    const [saving, setSaving] = useState(false);
    const [note, setNote] = useState<{ kind: "ok" | "error"; text: string } | null>(null);

    const load = useCallback(() => {
        setText(null);
        setError("");
        let live = true;
        api.DiagPreview().then((s) => { if (live) setText(s); }).catch((e) => { if (live) setError(t("diag.error", {error: errText(e)})); });
        return () => { live = false; };
    }, []);
    useEffect(() => load(), [load]);

    const save = async () => {
        setSaving(true);
        setNote(null);
        try {
            const path = await api.DiagSave("");
            if (path) setNote({kind: "ok", text: t("diag.saved", {path})});
        } catch (e) {
            setNote({kind: "error", text: saveProblem(e)});
        } finally {
            setSaving(false);
        }
    };

    return (
        <div className="overlay" onClick={p.onClose}>
            <div className="modal diagdlg" ref={dialog} role="dialog" aria-modal="true" aria-label={t("diag.title")} onClick={(e) => e.stopPropagation()}>
                <h3>{t("diag.title")}</h3>
                <p className="diagpromise" role="note">{t("diag.promise")}</p>
                <p className="muted small nogap">{t("diag.includes")}</p>
                <p className="muted small nogap">{t("diag.excludes")}</p>
                <label className="diaglabel" htmlFor="diag-preview">{t("diag.previewLabel")}</label>
                {error ? (
                    <div className="diagerror" role="alert">
                        <span>{error}</span>
                        <button type="button" onClick={() => void load()}>{t("diag.retry")}</button>
                    </div>
                ) : (
                    <textarea id="diag-preview" className="diagpreview mono" readOnly spellCheck={false} wrap="off" rows={12}
                              value={text ?? t("diag.loading")} aria-busy={text === null}/>
                )}
                <div className="diagnote small" role="status" aria-live="polite" data-kind={note?.kind}>{note?.text ?? ""}</div>
                <div className="actions end">
                    <button onClick={p.onClose}>{t("common.close")}</button>
                    <button className="primary" disabled={text === null || saving} onClick={() => void save()}>{saving ? t("diag.saving") : t("diag.save")}</button>
                </div>
            </div>
        </div>
    );
}
