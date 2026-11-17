import {TaskViewer} from "../components/TaskViewer";
import {t} from "../lib/i18n";
import {EnvDoctor} from "./developer/EnvDoctor";
import {Features} from "./developer/Features";

/**
 * Tools for developers: a doctor for PATH and tool variables, and the Windows features (WSL, virtualization,
 * Sandbox) that development tools need. Changing a feature runs through the repair runner App provides.
 */
export function Developer(p: {
    admin: boolean; say: (m: string) => void; onAdmin: () => void; onRun: (ids: string[]) => void;
    /** Changes whenever a run finished, so the features are read again. */
    refreshKey?: number;
}) {
    return (
        <div className="page developerpage">
            <header className="pagehead">
                <div>
                    <h1>{t("dev.title")}</h1>
                    <p className="muted">{t("dev.intro")}</p>
                </div>
            </header>
            <TaskViewer kind="developer"/>
            <EnvDoctor say={p.say}/>
            <Features admin={p.admin} say={p.say} onAdmin={p.onAdmin} onRun={p.onRun} refreshKey={p.refreshKey}/>
        </div>
    );
}
