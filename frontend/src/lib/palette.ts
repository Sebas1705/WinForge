// Matching for the command palette: forgiving about case and accents, strict about order.

/** Lower case with accents removed, so "catalogo" finds "Catálogo". */
export function fold(s: string): string {
    return s.normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase();
}

/**
 * How well one word matches a text, 0 meaning not at all. A whole match beats a prefix, a prefix beats
 * the start of a later word, that beats a plain substring, and letters that merely appear in order
 * ("scn" in "scan") come last and are accepted only for words of three letters or more.
 */
export function score(word: string, text: string): number {
    const w = fold(word.trim());
    const s = fold(text);
    if (!w) return 0;
    if (s === w) return 100;
    if (s.startsWith(w)) return 90;
    if (new RegExp("(^|[^\\p{L}\\p{N}])" + w.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "u").test(s)) return 75;
    if (s.includes(w)) return 60;
    if (w.length >= 3) {
        let at = 0;
        for (const ch of s) if (ch === w[at]) at++;
        if (at === w.length) return 25;
    }
    return 0;
}

export interface Rankable { label: string; keywords?: string }

/**
 * Items that match every word of the query, best first. Ties keep their original order, so the
 * list does not shuffle as someone types. An empty query keeps everything in order.
 */
export function rank<T extends Rankable>(items: T[], query: string, limit = Infinity): T[] {
    const words = query.split(/\s+/).filter(Boolean);
    if (words.length === 0) return items.slice(0, limit);
    const scored: { item: T; at: number; value: number }[] = [];
    items.forEach((item, at) => {
        let weakest = Infinity;
        for (const w of words) {
            const best = Math.max(score(w, item.label), 0.6 * score(w, item.keywords ?? ""));
            if (best === 0) return;
            weakest = Math.min(weakest, best);
        }
        scored.push({item, at, value: weakest + (score(query, item.label) === 100 ? 5 : 0)});
    });
    scored.sort((a, b) => b.value - a.value || a.at - b.at);
    return scored.slice(0, limit).map((x) => x.item);
}
