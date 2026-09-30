import {useEffect, useState} from "react";
import {t} from "../lib/i18n";
import type {UpgradeInfo} from "../lib/model";

export function Updates(p: {
    upgrades: UpgradeInfo[] | null; busy: boolean;
    onCheck: () => void; onUpdate: (ids: string[]) => void;
}) {
    const [picked, setPicked] = useState<Set<string>>(new Set());
    // Everything is ticked by default: the usual answer is "update it all".
    useEffect(() => { setPicked(new Set((p.upgrades ?? []).map((u) => u.id))); }, [p.upgrades]);
    const list = p.upgrades ?? [];
    const toggle = (id: string) => {
        const n = new Set(picked);
        if (n.has(id)) n.delete(id); else n.add(id);
        setPicked(n);
    };

    return (
        <div className="page">
            <header className="pagehead">
                <div>
                    <h1>{t("updates.title")}</h1>
                    <p className="muted">{t("updates.hint")}</p>
                </div>
                <span className="grow"/>
                <button disabled={p.busy} onClick={p.onCheck}>{p.busy ? t("updates.checking") : t("updates.check")}</button>
                <button className="primary" disabled={picked.size === 0}
                        onClick={() => p.onUpdate(picked.size === list.length ? [] : [...picked])}>
                    {picked.size === list.length && list.length > 0 ? t("updates.all") : t("updates.updateN", {n: picked.size})}
                </button>
            </header>

            {p.upgrades === null && !p.busy && <p className="muted empty">{t("updates.notChecked")}</p>}
            {p.upgrades !== null && list.length === 0 && <p className="muted empty">{t("updates.none")}</p>}
            <ul className="list">
                {list.map((u) => (
                    <li key={u.id} className={picked.has(u.id) ? "sel" : ""}>
                        <label>
                            <input type="checkbox" checked={picked.has(u.id)} onChange={() => toggle(u.id)} aria-label={u.name}/>
                            <span className="grow"><b>{u.name}</b> <span className="muted small">{u.publisher}</span></span>
                        </label>
                        <span className="mono muted">{t("updates.from", {from: u.current, to: u.available})}</span>
                    </li>
                ))}
            </ul>
        </div>
    );
}
