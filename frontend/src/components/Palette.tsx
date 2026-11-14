import {useEffect, useId, useMemo, useRef, useState} from "react";
import {t} from "../lib/i18n";
import {useModal} from "./useModal";

export type PaletteKind = "go" | "action" | "app" | "profile" | "search";

export interface PaletteItem {
    id: string;
    label: string;
    kind: PaletteKind;
    run: () => void;
}

const KIND_KEY = {
    go: "palette.kind.go", action: "palette.kind.action", app: "palette.kind.app", profile: "palette.kind.profile", search: "palette.kind.search",
} as const;

/**
 * The command palette: type, move with the arrow keys, press Enter. Focus stays in the text box and
 * the highlighted result is exposed through aria-activedescendant, which is how a list that
 * follows a text field is meant to work with a screen reader.
 */
export function Palette(p: { build: (query: string) => PaletteItem[]; onClose: () => void }) {
    const [query, setQuery] = useState("");
    const [at, setAt] = useState(0);
    const dialog = useRef<HTMLDivElement>(null);
    const uid = useId();
    const listId = `${uid}-list`;
    useModal(dialog, p.onClose);
    const items = useMemo(() => p.build(query), [p.build, query]);
    useEffect(() => { setAt(0); }, [query]);
    useEffect(() => { document.getElementById(`${uid}-${at}`)?.scrollIntoView?.({block: "nearest"}); }, [at, uid]);

    // Closed first, so focus is back where it was before the thing the person chose opens its own dialog.
    const run = (item: PaletteItem | undefined) => {
        if (!item) return;
        p.onClose();
        item.run();
    };
    const onKey = (e: React.KeyboardEvent<HTMLInputElement>) => {
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault();
            if (items.length > 0) setAt((at + (e.key === "ArrowDown" ? 1 : -1) + items.length) % items.length);
        } else if (e.key === "Enter" && !e.nativeEvent.isComposing) {
            e.preventDefault();
            run(items[at]);
        }
    };

    return (
        <div className="overlay palette-overlay" onClick={p.onClose}>
            <div className="modal palette" ref={dialog} role="dialog" aria-modal="true" aria-label={t("palette.title")} onClick={(e) => e.stopPropagation()}>
                <input data-autofocus type="text" role="combobox" aria-expanded="true" aria-controls={listId} aria-autocomplete="list"
                       aria-activedescendant={items.length > 0 ? `${uid}-${at}` : undefined} aria-label={t("palette.title")}
                       placeholder={t("palette.placeholder")} value={query} autoComplete="off" spellCheck={false}
                       onChange={(e) => setQuery(e.target.value)} onKeyDown={onKey}/>
                <ul id={listId} role="listbox" aria-label={t("palette.results")}>
                    {items.map((it, i) => (
                        <li key={it.id} id={`${uid}-${i}`} role="option" aria-selected={i === at} onMouseMove={() => { if (i !== at) setAt(i); }}
                            onMouseDown={(e) => e.preventDefault()} onClick={() => run(it)}>
                            <span className="grow">{it.label}</span>
                            <span className="kind">{t(KIND_KEY[it.kind])}</span>
                        </li>
                    ))}
                </ul>
                {items.length === 0 && <p className="muted palette-empty">{t("palette.none")}</p>}
                <p className="muted small palette-hint" aria-hidden="true">{t("palette.hint")}</p>
                <p className="sr-only" aria-live="polite" aria-atomic="true">{items.length === 0 ? t("palette.none") : t("palette.count", {n: items.length})}</p>
            </div>
        </div>
    );
}
