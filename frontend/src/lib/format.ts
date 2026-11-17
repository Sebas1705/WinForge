import {getLang, type Lang} from "./i18n";

const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];

/**
 * A byte count for people: 0 B, 1023 B, 1 KB, 1.5 GB, 512 GB. Binary steps (1024),
 * one decimal below 100 and none above. Negative, NaN and infinite values read as 0 B.
 * The decimal mark follows the interface language.
 */
export function formatBytes(bytes: number, lang: Lang = getLang()): string {
    const n = Number.isFinite(bytes) && bytes > 0 ? bytes : 0;
    if (n < 1024) return `${Math.floor(n)} B`;
    let i = 0;
    let value = n;
    while (value >= 1024 && i < UNITS.length - 1) { value /= 1024; i++; }
    const digits = (v: number) => (v < 100 ? 1 : 0);
    // 1023.96 KB rounds to "1024 KB": show it as 1 MB instead.
    if (Number(value.toFixed(digits(value))) >= 1024 && i < UNITS.length - 1) { value /= 1024; i++; }
    const fmt = new Intl.NumberFormat(lang, {minimumFractionDigits: 0, maximumFractionDigits: digits(value)});
    return `${fmt.format(Number(value.toFixed(digits(value))))} ${UNITS[i]}`;
}

/** A date such as "5 Mar 2024" / "5 mar 2024" from an ISO 8601 string; "" when it cannot be read. */
export function formatDate(iso: string, lang: Lang = getLang()): string {
    const d = new Date(iso);
    // Go's zero time (year 1) means "unknown".
    if (!iso || Number.isNaN(d.getTime()) || d.getUTCFullYear() < 1970) return "";
    return new Intl.DateTimeFormat(lang, {dateStyle: "medium"}).format(d);
}

/** Whole percent, clamped to 0-100; 0 when the whole is not positive. */
export function percent(part: number, whole: number): number {
    if (!(whole > 0) || !Number.isFinite(part)) return 0;
    return Math.min(100, Math.max(0, Math.round((part / whole) * 100)));
}

/** "12 s", "3 min 05 s": how long a job ran, short enough for a list row. */
export function formatElapsed(ms: number): string {
    const s = Math.max(0, Math.round(ms / 1000));
    if (s < 60) return `${s} s`;
    const m = Math.floor(s / 60);
    return `${m} min ${String(s % 60).padStart(2, "0")} s`;
}
