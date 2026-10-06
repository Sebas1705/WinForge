import "@testing-library/jest-dom/vitest";
import {cleanup} from "@testing-library/react";
import {afterEach, vi} from "vitest";

// jsdom lacks a few browser APIs the app uses; stub them the way a browser answers.
Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
        matches: false, media: query, onchange: null,
        addEventListener: () => {}, removeEventListener: () => {}, addListener: () => {}, removeListener: () => {}, dispatchEvent: () => false,
    }),
});
Element.prototype.scrollIntoView = vi.fn();
Element.prototype.scrollTo = vi.fn() as unknown as typeof Element.prototype.scrollTo;
// offsetParent is always null in jsdom (no layout); report "visible" so focus handling is testable.
Object.defineProperty(HTMLElement.prototype, "offsetParent", {get() { return this.parentElement; }});

afterEach(() => {
    cleanup();
    try { localStorage.clear(); } catch { /* ignore */ }
    vi.clearAllMocks();
});
