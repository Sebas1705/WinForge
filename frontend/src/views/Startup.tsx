import {useState} from "react";
import {TaskViewer} from "../components/TaskViewer";
import {t} from "../lib/i18n";
import {Programs} from "./startup/Programs";
import {Services} from "./startup/Services";
import {Panel, TabList} from "./startup/Tabs";
import {Tasks} from "./startup/Tasks";

export {startupError, visibleItems} from "./startup/Programs";

type TabId = "programs" | "services" | "tasks";

/**
 * What starts with Windows, in three tabs: programs (Run keys and Startup folders), third-party services
 * and third-party scheduled tasks. Nothing is ever deleted; everything shown can be switched back.
 * A tab is read the first time it is opened and then kept, so switching between tabs is instant.
 */
export function Startup(p: { admin: boolean; say: (m: string) => void; onAdmin: () => void }) {
    const [tab, setTab] = useState<TabId>("programs");
    const [opened, setOpened] = useState<TabId[]>(["programs"]);
    const open = (id: TabId) => {
        setTab(id);
        setOpened((o) => (o.includes(id) ? o : [...o, id]));
    };
    const tabs: { id: TabId; label: string }[] = [
        {id: "programs", label: t("svc.tab.programs")},
        {id: "services", label: t("svc.tab.services")},
        {id: "tasks", label: t("svc.tab.tasks")},
    ];
    return (
        <div className="page startuppage">
            <header className="pagehead">
                <h1>{t("startup.title")}</h1>
            </header>
            <TaskViewer kind="startup"/>
            <TabList label={t("svc.tabs")} tabs={tabs} value={tab} onChange={open}/>
            <Panel id="programs" active={tab === "programs"}><Programs admin={p.admin} say={p.say} onAdmin={p.onAdmin}/></Panel>
            {opened.includes("services") && <Panel id="services" active={tab === "services"}><Services admin={p.admin} say={p.say} onAdmin={p.onAdmin}/></Panel>}
            {opened.includes("tasks") && <Panel id="tasks" active={tab === "tasks"}><Tasks admin={p.admin} say={p.say} onAdmin={p.onAdmin}/></Panel>}
        </div>
    );
}
