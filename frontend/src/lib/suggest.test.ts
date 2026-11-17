import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {describe, expect, it} from "vitest";
import {FIELD_NAME, LABEL, MAX_NAME, REPO, TEMPLATE, cleanName, suggestBody, suggestUrl} from "./suggest";

const parse = (u: string) => new URL(u);

describe("suggestUrl", () => {
    it("opens a new issue on the WinForge repository with the title, label, template and name", () => {
        const u = parse(suggestUrl("Obsidian"));
        expect(u.origin + u.pathname).toBe(`https://github.com/${REPO}/issues/new`);
        expect(u.searchParams.get("title")).toBe("App request: Obsidian");
        expect(u.searchParams.get("labels")).toBe(LABEL);
        expect(u.searchParams.get("template")).toBe(TEMPLATE);
        expect(u.searchParams.get(FIELD_NAME)).toBe("Obsidian");
        expect(u.searchParams.get("body")).toContain("### App name\nObsidian\n");
    });

    it("always starts with https:// (the only scheme OpenURL opens)", () => {
        expect(suggestUrl("x")).toMatch(/^https:\/\/github\.com\//);
    });

    it("asks for the app name, the winget id and the reason, and holds nothing but the typed name", () => {
        const body = suggestBody("Obsidian");
        for (const q of ["App name", "winget id", "Why would it be a good fit"]) expect(body).toContain(q);
        const rest = body.replace("Obsidian", "");
        expect(rest).not.toMatch(/[A-Z]:\\|@|\/Users\/|APPDATA|%\w+%/);
    });

    it.each([
        ["spaces and plus", "c++ editor", "c++ editor"],
        ["ampersand and equals", "a&b=c", "a&b=c"],
        ["hash and question mark", "what #1? app", "what #1? app"],
        ["percent", "100% free", "100% free"],
        ["quotes and angle brackets", `"x" <y>`, `"x" <y>`],
        ["accents and emoji", "Ñandú 🦉 日本語", "Ñandú 🦉 日本語"],
        ["a URL", "https://example.com/a?b=c", "https://example.com/a?b=c"],
    ])("round-trips %s through the query string", (_n, typed, shown) => {
        const u = parse(suggestUrl(typed));
        expect(u.searchParams.get("title")).toBe(`App request: ${shown}`);
        expect(u.searchParams.get(FIELD_NAME)).toBe(shown);
        // a raw "&" or "#" from the person must never become a second parameter or a fragment
        expect(u.hash).toBe("");
        expect([...u.searchParams.keys()].sort()).toEqual(["app-name", "body", "labels", "template", "title"]);
    });

    it("encodes everything: no raw space, quote, ampersand from the text or line break in the address", () => {
        const u = suggestUrl(`a b&c="d"\nnew line`);
        expect(u).not.toMatch(/[\s"<>]/);
        expect((u.match(/&/g) ?? []).length).toBe(4); // only the five parameter separators
    });

    it("cuts a very long search at the limit without splitting an emoji, and stays a usable length", () => {
        const long = "🦉".repeat(500);
        const name = cleanName(long);
        expect(Array.from(name)).toHaveLength(MAX_NAME);
        expect(() => encodeURIComponent(name)).not.toThrow();
        const u = suggestUrl(long);
        expect(u.length).toBeLessThan(6000);
        expect(() => parse(u)).not.toThrow();
        expect(parse(u).searchParams.get("title")).toBe(`App request: ${name}`);
    });

    it("keeps lone surrogates out, since they cannot be URL-encoded", () => {
        expect(() => suggestUrl("ok \ud800 bad \udc00")).not.toThrow();
        expect(cleanName("ok \ud800 bad")).toBe("ok bad");
    });

    it("turns line breaks and control characters into single spaces and trims", () => {
        expect(cleanName("  one\r\n two\t\u0000three   ")).toBe("one two three");
        expect(cleanName("   ")).toBe("");
    });

    it("has no empty name field when nothing is typed", () => {
        const u = parse(suggestUrl("   "));
        expect(u.searchParams.get("title")).toBe("App request");
        expect(u.searchParams.has(FIELD_NAME)).toBe(false);
    });

    it("can be told apart from a script: markup in the search stays text in the query string", () => {
        const u = suggestUrl(`<script>alert(1)</script>`);
        expect(u).not.toContain("<");
        expect(parse(u).searchParams.get("title")).toBe("App request: <script>alert(1)</script>");
    });
});

describe("the issue form", () => {
    const form = readFileSync(resolve(__dirname, "../../../.github/ISSUE_TEMPLATE/app-request.yml"), "utf8");

    it("exists under the name the link uses, with the label and the field the link fills in", () => {
        expect(form).toContain(`labels: ["${LABEL}"]`);
        expect(form).toMatch(new RegExp(`id: ${FIELD_NAME}\\b`));
        expect(TEMPLATE).toBe("app-request.yml");
    });

    it("asks for the app name, the winget id and the reason", () => {
        expect(form).toContain("label: App name");
        expect(form).toContain("winget id");
        expect(form).toContain("Why would it be a good fit");
    });
});
