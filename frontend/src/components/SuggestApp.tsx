import {useId} from "react";
import {t} from "../lib/i18n";
import {cleanName, suggestUrl} from "../lib/suggest";
import "../diag.css";

/**
 * "Not here? Suggest this app": shown when a search found nothing. It opens the browser on a pre-filled
 * GitHub issue and says so; the only thing it sends is the text the person typed in the search box.
 */
export function SuggestApp(p: { query: string; openURL: (u: string) => void; className?: string; as?: "p" | "li" }) {
    const id = useId();
    const name = cleanName(p.query);
    if (!name) return null;
    const body = (
        <>
            <button type="button" className="linkbtn suggestlink" aria-describedby={id} onClick={() => p.openURL(suggestUrl(p.query))}>
                {t("diag.suggest.link")} <span aria-hidden="true">↗</span>
            </button>
            <span id={id} className="muted small suggesthint">{t("diag.suggest.hint", {q: name})}</span>
        </>
    );
    return p.as === "li" ? <li className="suggest">{body}</li> : <div className="suggest">{body}</div>;
}
