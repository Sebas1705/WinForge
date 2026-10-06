import {useEffect, useRef, type RefObject} from "react";

// Dialogs can stack (a detail panel opened over a run, the guide over anything).
// Only the topmost reacts to Escape and Tab, so one key press never closes two
// layers and focus never escapes into the page behind.
const stack: symbol[] = [];

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function focusables(root: HTMLElement): HTMLElement[] {
    return [...root.querySelectorAll<HTMLElement>(FOCUSABLE)].filter((el) => el.offsetParent !== null || el === document.activeElement);
}

/**
 * Makes an element behave as a modal: focus moves into it, Tab cycles inside it,
 * Escape calls onEscape (when given), and focus returns to where it was on close.
 */
export function useModal(ref: RefObject<HTMLElement | null>, onEscape?: () => void): void {
    const escape = useRef(onEscape);
    escape.current = onEscape;

    useEffect(() => {
        const id = Symbol("modal");
        stack.push(id);
        const before = document.activeElement as HTMLElement | null;
        const root = ref.current;
        if (root && !root.contains(document.activeElement)) {
            // Prefer a control that asked for focus, then the primary action, then the first control.
            const target = root.querySelector<HTMLElement>("[autofocus], [data-autofocus]")
                ?? root.querySelector<HTMLElement>("button.primary:not([disabled])")
                ?? focusables(root)[0] ?? root;
            if (!target.hasAttribute("tabindex") && target === root) root.tabIndex = -1;
            target.focus({preventScroll: true});
        }

        const onKey = (e: KeyboardEvent) => {
            if (stack[stack.length - 1] !== id) return;
            if (e.key === "Escape" && escape.current) {
                e.preventDefault();
                e.stopPropagation();
                escape.current();
            } else if (e.key === "Tab" && ref.current) {
                const items = focusables(ref.current);
                if (items.length === 0) { e.preventDefault(); return; }
                const first = items[0], last = items[items.length - 1];
                const active = document.activeElement;
                if (e.shiftKey && (active === first || !ref.current.contains(active))) { e.preventDefault(); last.focus(); }
                else if (!e.shiftKey && (active === last || !ref.current.contains(active))) { e.preventDefault(); first.focus(); }
            }
        };
        document.addEventListener("keydown", onKey, true);
        return () => {
            document.removeEventListener("keydown", onKey, true);
            const i = stack.indexOf(id);
            if (i >= 0) stack.splice(i, 1);
            if (before && document.contains(before)) before.focus({preventScroll: true});
        };
    }, [ref]);
}

/** Closes a popover when the pointer goes down outside it or Escape is pressed. */
export function useDismiss(ref: RefObject<HTMLElement | null>, open: boolean, close: () => void): void {
    const cb = useRef(close);
    cb.current = close;
    useEffect(() => {
        if (!open) return;
        const down = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) cb.current(); };
        const key = (e: KeyboardEvent) => { if (e.key === "Escape" && stack.length === 0) cb.current(); };
        document.addEventListener("mousedown", down);
        document.addEventListener("keydown", key);
        return () => { document.removeEventListener("mousedown", down); document.removeEventListener("keydown", key); };
    }, [ref, open]);
}
