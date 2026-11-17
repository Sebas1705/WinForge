import {useCallback, useEffect, useId, useRef, useState} from "react";
import {api, errText, type AssistantInfo, type AssistantMode} from "../api";
import {t, type Key} from "../lib/i18n";
import "../assistant.css";
import {useModal} from "./useModal";

const MODES: AssistantMode[] = ["read", "plan", "full"];

type Block = "code" | "desktop";
type CopyState = "" | "ok" | "fail";

/** Copies text; a missing or refusing clipboard is reported, never thrown. */
async function copyText(text: string): Promise<boolean> {
    try {
        if (!navigator.clipboard?.writeText) return false;
        await navigator.clipboard.writeText(text);
        return true;
    } catch {
        return false;
    }
}

function CodeBlock(p: { label: string; text: string; copy: CopyState; onCopy: () => void }) {
    return (
        <div className="assistblock">
            <pre className="assistcode mono" tabIndex={0} role="group" aria-label={p.label}>{p.text}</pre>
            <div className="assistcopyrow">
                <button type="button" onClick={p.onCopy} aria-label={t("assistant.copyAria", {what: p.label})}>{t("assistant.copy")}</button>
                <span className="small assistcopied" role="status" aria-live="polite" data-kind={p.copy}>
                    {p.copy === "ok" ? t("assistant.copied") : p.copy === "fail" ? t("assistant.copyFailed") : ""}
                </span>
            </div>
        </div>
    );
}

/**
 * Connecting an assistant (an MCP client such as Claude) to winforge-mcp.exe. WinForge only shows the exact
 * text to copy and where the program is: it never runs, installs or writes anything for this. The privacy
 * box says what the assistant's model receives and what each mode can change.
 */
export function AssistantDialog(p: { onClose: () => void }) {
    const dialog = useRef<HTMLDivElement>(null);
    useModal(dialog, p.onClose);
    const [info, setInfo] = useState<AssistantInfo | null>(null);
    const [error, setError] = useState("");
    const [mode, setMode] = useState<AssistantMode>("read");
    const [copy, setCopy] = useState<Record<Block, CopyState>>({code: "", desktop: ""});
    const [revealError, setRevealError] = useState("");
    const name = useId();

    const load = useCallback(() => {
        setInfo(null);
        setError("");
        let live = true;
        api.AssistantInfo().then((i) => { if (live) setInfo(i); }).catch((e) => { if (live) setError(t("assistant.exe.error", {error: errText(e)})); });
        return () => { live = false; };
    }, []);
    useEffect(() => load(), [load]);

    const pick = (m: AssistantMode) => {
        setMode(m);
        setCopy({code: "", desktop: ""});
    };
    const doCopy = async (block: Block, text: string) => {
        const ok = await copyText(text);
        setCopy((c) => ({...c, [block]: ok ? "ok" : "fail"}));
    };
    const reveal = async () => {
        setRevealError("");
        try { await api.AssistantRevealExe(); } catch (e) { setRevealError(t("assistant.exe.revealError", {error: errText(e)})); }
    };

    const codeLabel = t("assistant.cmd.code.label");
    const desktopLabel = t("assistant.cmd.desktop.label");
    const modeName = (m: AssistantMode) => t(`assistant.mode.${m}` as Key);

    return (
        <div className="overlay" onClick={p.onClose}>
            <div className="modal assistdlg" ref={dialog} role="dialog" aria-modal="true" aria-label={t("assistant.title")} onClick={(e) => e.stopPropagation()}>
                <h3>{t("assistant.title")}</h3>
                <p className="nogap">{t("assistant.intro")}</p>

                <fieldset className="assistmodes">
                    <legend>{t("assistant.modeLegend")}</legend>
                    {MODES.map((m) => (
                        <label key={m} className={"assistmode" + (mode === m ? " on" : "")}>
                            <input type="radio" name={name} value={m} checked={mode === m} onChange={() => pick(m)} aria-labelledby={`${name}-${m}-n`} aria-describedby={`${name}-${m}`}/>
                            <span className="assistmodetext">
                                <b id={`${name}-${m}-n`}>{modeName(m)}</b>
                                <span className="muted small" id={`${name}-${m}`}>{t(`assistant.mode.${m}.desc` as Key)}</span>
                            </span>
                        </label>
                    ))}
                </fieldset>

                <section className="assistsection" aria-labelledby={`${name}-exe`}>
                    <h4 id={`${name}-exe`}>{t("assistant.exe.title")}</h4>
                    {error ? (
                        <div className="assisterror" role="alert">
                            <span>{error}</span>
                            <button type="button" onClick={() => void load()}>{t("assistant.exe.recheck")}</button>
                        </div>
                    ) : info === null ? (
                        <p className="muted small nogap" role="status">{t("assistant.exe.loading")}</p>
                    ) : info.exists ? (
                        <>
                            <p className="small nogap">{t("assistant.exe.where")}</p>
                            <code className="mono small assistpath">{info.exePath}</code>
                            <div className="actions tight"><button type="button" onClick={() => void reveal()}>{t("assistant.exe.show")}</button></div>
                            {revealError && <p className="small assistnote" role="alert" data-kind="error">{revealError}</p>}
                        </>
                    ) : (
                        <div className="assistmissing" role="note">
                            <p className="small nogap">{t("assistant.exe.missing")}</p>
                            <div className="actions tight"><button type="button" onClick={() => void load()}>{t("assistant.exe.recheck")}</button></div>
                        </div>
                    )}
                </section>

                {info?.exists && (
                    <section className="assistsection" aria-labelledby={`${name}-cmd`}>
                        <h4 id={`${name}-cmd`}>{t("assistant.cmd.heading", {mode: modeName(mode)})}</h4>
                        <h5>{t("assistant.cmd.code.title")}</h5>
                        <p className="muted small nogap">{t("assistant.cmd.code.hint")}</p>
                        <CodeBlock label={codeLabel} text={info.commands.claudeCode[mode]} copy={copy.code} onCopy={() => void doCopy("code", info.commands.claudeCode[mode])}/>
                        <h5>{t("assistant.cmd.desktop.title")}</h5>
                        <p className="muted small nogap">{t("assistant.cmd.desktop.hint")}</p>
                        <CodeBlock label={desktopLabel} text={info.commands.claudeDesktop[mode]} copy={copy.desktop} onCopy={() => void doCopy("desktop", info.commands.claudeDesktop[mode])}/>
                    </section>
                )}

                <section className="assistprivacy" aria-labelledby={`${name}-priv`}>
                    <h4 id={`${name}-priv`}>{t("assistant.privacy.title")}</h4>
                    <ul>
                        <li>{t("assistant.privacy.sent")}</li>
                        <li>{t("assistant.privacy.none")}</li>
                        <li>{t("assistant.privacy.read")}</li>
                        <li>{t("assistant.privacy.full")}</li>
                        <li>{t("assistant.privacy.approval")}</li>
                    </ul>
                </section>

                <div className="actions end"><button className="primary" onClick={p.onClose}>{t("common.close")}</button></div>
            </div>
        </div>
    );
}
