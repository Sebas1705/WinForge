import {useCallback, useEffect, useState} from "react";
import {api, errText, type StorageDefaults, type StorageVolume} from "../api";
import {t} from "../lib/i18n";
import {Biggest} from "./storage/Biggest";
import {Cleanup} from "./storage/Cleanup";
import {Duplicates} from "./storage/Duplicates";
import {Projects} from "./storage/Projects";
import {Optimize} from "./storage/Optimize";
import {Overview} from "./storage/Overview";
import {useTask} from "./storage/useTask";

/**
 * Cleanup and optimization: how full each drive is, junk that can be cleaned, the biggest
 * files and folders, duplicate files, and maintenance tasks. Long jobs run one at a time
 * and can be cancelled. Nothing is removed before the person confirms.
 */
export function Storage(p: {
    admin: boolean; say: (m: string) => void; onAdmin: () => void; onOptimize: (ids: string[]) => void;
    /** Changes whenever a maintenance run finished, so the drives are measured again. */
    refreshKey?: number;
}) {
    const [volumes, setVolumes] = useState<StorageVolume[] | null>(null);
    const [defaults, setDefaults] = useState<StorageDefaults | null>(null);
    const task = useTask(p.say);

    const loadVolumes = useCallback(async () => {
        try { setVolumes(await api.StorageVolumes()); } catch (e) { p.say(errText(e)); setVolumes((v) => v ?? []); }
    }, [p.say]);

    useEffect(() => { void loadVolumes(); }, [loadVolumes, p.refreshKey]);
    useEffect(() => {
        let live = true;
        api.StorageDefaults().then((d) => { if (live) setDefaults(d); }).catch((e) => p.say(errText(e)));
        return () => { live = false; };
    }, [p.say]);

    return (
        <div className="page storagepage">
            <header className="pagehead">
                <div>
                    <h1>{t("storage.title")}</h1>
                    <p className="muted">{t("storage.intro")}</p>
                </div>
            </header>
            <Overview volumes={volumes} onRefresh={() => void loadVolumes()}/>
            <Cleanup task={task} admin={p.admin} say={p.say} onAdmin={p.onAdmin} onCleaned={() => void loadVolumes()}/>
            <Biggest task={task} home={defaults?.home ?? ""} say={p.say}/>
            <Duplicates task={task} defaults={defaults} say={p.say}/>
            <Projects task={task} defaults={defaults} say={p.say} onCleaned={() => void loadVolumes()}/>
            <Optimize onOptimize={p.onOptimize}/>
        </div>
    );
}
