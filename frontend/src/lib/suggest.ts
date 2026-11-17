// "Not here? Suggest this app": builds the pre-filled GitHub new-issue address that opens in the
// browser. Pure text work, no network. Only what the person typed in the search box goes in, cleaned
// and cut short; nothing about this PC, the catalog or the person.

/** The repository whose issues receive app requests. */
export const REPO = "Sebas1705/WinForge";
/** The issue form in .github/ISSUE_TEMPLATE. */
export const TEMPLATE = "app-request.yml";
export const LABEL = "app-request";
/** The longest app name sent, in characters. A search box can hold a pasted paragraph. */
export const MAX_NAME = 80;

/** What the issue form calls its first field (its `id` in app-request.yml). */
export const FIELD_NAME = "app-name";

/** The typed text, made safe to put in a title: control characters and line breaks become spaces, runs of
 *  spaces collapse, lone surrogates go (they cannot be URL-encoded), and it is cut at MAX_NAME characters. */
export function cleanName(q: string): string {
    let out = "";
    for (let i = 0; i < q.length; i++) {
        const c = q.charCodeAt(i);
        if (c >= 0xd800 && c <= 0xdbff) { // a high surrogate is kept only when its low half follows
            const next = q.charCodeAt(i + 1);
            if (next >= 0xdc00 && next <= 0xdfff) { out += q[i] + q[i + 1]; i++; }
            continue;
        }
        if (c >= 0xdc00 && c <= 0xdfff) continue; // a lone low surrogate
        const control = c < 0x20 || (c >= 0x7f && c <= 0x9f) || c === 0x2028 || c === 0x2029;
        out += control ? " " : q[i];
    }
    const chars = Array.from(out.replace(/\s+/g, " ").trim());
    return chars.slice(0, MAX_NAME).join("").trim();
}

/** The issue body: it asks the questions, and holds the typed name. It never holds anything else. */
export function suggestBody(name: string): string {
    return [
        "### App name",
        name,
        "",
        "### winget id, if you know it",
        "(for example Publisher.AppName; `winget search <name>` finds it)",
        "",
        "### Why would it be a good fit for the catalog?",
        "(what it does, and why you would like WinForge to offer it)",
        "",
        "Please do not add personal information to this request.",
    ].join("\n");
}

/** The address that opens a new issue on the repository with the title, labels and body filled in. */
export function suggestUrl(query: string): string {
    const name = cleanName(query);
    const params: [string, string][] = [
        ["template", TEMPLATE],
        ["title", name ? `App request: ${name}` : "App request"],
        ["labels", LABEL],
        [FIELD_NAME, name],
        ["body", suggestBody(name)],
    ];
    const qs = params.filter(([, v]) => v !== "").map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`).join("&");
    return `https://github.com/${REPO}/issues/new?${qs}`;
}
