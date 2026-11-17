import {useEffect, useState} from "react";
import {Icon} from "./Icon";
import {formatElapsed} from "../lib/format";
import {t} from "../lib/i18n";
import {clearFinished, useTasks, type TaskKind, type TrackedTask} from "../lib/taskStore";

const STATE_KEY = {done: "tasks.done", failed: "tasks.failed", cancelled: "tasks.cancelled", running: "tasks.running"} as const;

/** Re-renders every second while something runs, so the elapsed time moves. */
function useNow(active: boolean): number {
    const [now, setNow] = useState(() => Date.now());
    useEffect(() => {
        if (!active) return;
        const id = window.setInterval(() => setNow(Date.now()), 1000);
        return () => window.clearInterval(id);
    }, [active]);
    return now;
}

function Row(p: { task: TrackedTask; now: number }) {
    const {task: x} = p;
    const running = x.state === "running";
    const elapsed = formatElapsed((x.endedAt ?? p.now) - x.startedAt);
    return (
        <li className={"taskrow " + x.state}>
            <div className="taskmain">
                <b>{x.title}</b>
                {running && (x.fraction !== undefined ? <progress aria-label={x.title} value={x.fraction} max={1}/> : <progress aria-label={x.title}/>)}
                {running && x.live && <span className="mono muted small tasklive" title={x.live}>{x.live}</span>}
                {!running && x.summary && <span className="muted small">{x.summary}</span>}
                {x.state === "failed" && x.error && <span className="small taskerr">{x.error}</span>}
            </div>
            <span className={"chip taskchip " + x.state}>{t(STATE_KEY[x.state])}</span>
            <span className="muted small mono taskdur">{elapsed}</span>
            {running && x.cancel && (
                <button className="sm" aria-label={t("tasks.cancelLabel", {task: x.title})} onClick={x.cancel}>{t("tasks.cancel")}</button>
            )}
        </li>
    );
}

/**
 * The jobs of one kind: the ones running now (with live progress and a way to stop them) and the
 * last few that finished, with what they did. It lives outside the page's own state, so leaving a
 * page and coming back shows the same job still running, or how it ended.
 */
export function TaskViewer(p: { kind: TaskKind; heading?: "h2" | "h3" }) {
    const list = useTasks(p.kind);
    const running = list.filter((x) => x.state === "running");
    const now = useNow(running.length > 0);
    const finished = list.filter((x) => x.state !== "running").reverse();
    const H = p.heading ?? "h2";
    return (
        <section className="taskviewer" aria-labelledby={`tasks-${p.kind}`}>
            <div className="taskhead">
                <H id={`tasks-${p.kind}`}><Icon name="list" size={15}/> {t("tasks.title")}</H>
                {finished.length > 0 && <button className="mini" onClick={() => clearFinished(p.kind)}>{t("tasks.clear")}</button>}
            </div>
            {list.length === 0 && <p className="muted small">{t("tasks.empty")}</p>}
            {running.length > 0 && (
                <>
                    <ul className="tasklist" role="status" aria-live="polite">{running.map((x) => <Row key={x.id} task={x} now={now}/>)}</ul>
                    <p className="muted small">{t("tasks.runningHint")}</p>
                </>
            )}
            {finished.length > 0 && <ul className="tasklist">{finished.map((x) => <Row key={x.id} task={x} now={now}/>)}</ul>}
        </section>
    );
}
