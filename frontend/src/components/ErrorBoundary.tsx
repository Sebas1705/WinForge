import {Component, type ReactNode} from "react";
import {resolveLang, setLang, t} from "../lib/i18n";
import * as prefs from "../lib/settings";

/**
 * Last line of defence: a render error must never leave a blank window. It
 * explains what happened in the person's language and offers a reload.
 */
export class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
    state = {error: null as Error | null};

    static getDerivedStateFromError(error: Error) {
        return {error};
    }

    componentDidCatch(error: Error, info: { componentStack?: string | null }) {
        console.error("WinForge render error:", error, info.componentStack);
    }

    render() {
        const {error} = this.state;
        if (!error) return this.props.children;
        // The app may have crashed before it set the language.
        setLang(resolveLang(prefs.load().language, navigator.language));
        const details = `${error.name}: ${error.message}\n${error.stack ?? ""}`;
        return (
            <div className="crash" role="alert">
                <h1>{t("error.title")}</h1>
                <p>{t("error.body")}</p>
                <div className="actions">
                    <button className="primary" onClick={() => window.location.reload()}>{t("error.reload")}</button>
                    <button onClick={() => void navigator.clipboard?.writeText(details)}>{t("error.copy")}</button>
                </div>
                <pre className="mono small muted">{error.message}</pre>
            </div>
        );
    }
}
