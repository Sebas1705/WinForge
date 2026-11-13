import {Icon, type IconName} from "../../components/Icon";
import {t, type Key} from "../../lib/i18n";

interface Task { id: string; icon: IconName; title: Key; body: Key; warn?: Key }

/** The maintenance tasks. Their ids are the repair steps the Go side runs ("optimize-*"). */
export const OPTIMIZE_TASKS: Task[] = [
    {id: "optimize-trim", icon: "refresh", title: "storage.opt.trim", body: "storage.opt.trimBody"},
    {id: "optimize-defrag", icon: "disk", title: "storage.opt.defrag", body: "storage.opt.defragBody"},
    {id: "optimize-component-cleanup", icon: "archive", title: "storage.opt.componentCleanup", body: "storage.opt.componentCleanupBody"},
    {id: "optimize-hibernate-off", icon: "alert", title: "storage.opt.hibernate", body: "storage.opt.hibernateBody", warn: "storage.opt.hibernateWarn"},
];

/** One card per maintenance task. Running one hands its id to the repair runner. */
export function Optimize(p: { onOptimize: (ids: string[]) => void }) {
    return (
        <section aria-labelledby="storage-opt">
            <h2 id="storage-opt">{t("storage.opt.title")}</h2>
            <p className="muted small">{t("storage.opt.intro")}</p>
            <ul className="optcards">
                {OPTIMIZE_TASKS.map((o) => (
                    <li key={o.id} className="optcard">
                        <span className="bubble sm"><Icon name={o.icon} size={18}/></span>
                        <div className="grow">
                            <b>{t(o.title)}</b>
                            <span className="muted small">{t(o.body)}</span>
                            {o.warn && <span className="small optwarn"><Icon name="alert" size={13}/> {t(o.warn)}</span>}
                        </div>
                        <button className="sm" aria-label={`${t("storage.opt.run")}: ${t(o.title)}`} onClick={() => p.onOptimize([o.id])}>
                            <Icon name="play" size={13}/> {t("storage.opt.run")}
                        </button>
                    </li>
                ))}
            </ul>
        </section>
    );
}
