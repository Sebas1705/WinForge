import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import App from "../App";
import type {EOLFinding, EOLReport} from "../lib/eol";
import {api, resetApi} from "./apiMock";
import {app, apps, healthResult, makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const catalogApps = [
    ...apps,
    app({id: "python-3-10", name: "Python 3.10", installed: true, version: "3.10.11", category: "dev/languages"}),
    app({id: "python-3-14", name: "Python 3.14", installed: false, category: "dev/languages"}),
    app({id: "temurin-17", name: "Eclipse Temurin JDK 17", installed: true, version: "17.0.9", category: "dev/languages"}),
    app({id: "temurin-11", name: "Eclipse Temurin JDK 11", installed: true, version: "11.0.20", category: "dev/languages"}),
    app({id: "temurin-21", name: "Eclipse Temurin JDK 21", installed: false, category: "dev/languages"}),
    app({id: "powershell", name: "PowerShell", installed: true, version: "7.4.6", category: "dev/shell"}),
    app({id: "go", name: "Go", installed: true, version: "1.26.1", category: "dev/languages"}),
];

const finding = (o: Partial<EOLFinding> & {appId: string}): EOLFinding => ({
    product: "python", productName: "Python", version: "3.10.11", cycle: "3.10", status: "ended", end: "2026-10-01", days: -47,
    source: "https://endoflife.date/python", replace: [], apps: [], ...o,
});

const report = (findings: EOLFinding[], o: Partial<EOLReport> = {}): EOLReport => ({asOf: "2026-11-17", ageDays: 3, stale: false, findings, ...o});

const sample = (): EOLReport => report([
    finding({
        appId: "python-3-10", replace: [{cycle: "3.15", end: "2031-10-31"}, {cycle: "3.14", end: "2030-10-31"}], apps: ["python-3-14"],
    }),
    finding({
        appId: "powershell", product: "powershell", productName: "PowerShell 7", version: "7.4.6", cycle: "7.4", status: "ended", end: "2026-11-10", days: -7, lts: true,
        replace: [{cycle: "7.6", end: "2028-11-14", lts: true}], apps: [], selfUpdate: true, source: "https://endoflife.date/powershell",
    }),
    finding({
        appId: "temurin-11", product: "temurin", productName: "Eclipse Temurin (Java)", version: "11.0.20", cycle: "11", status: "soon", end: "2027-05-10", days: 174, lts: true,
        replace: [{cycle: "25", end: "2031-09-30", lts: true}, {cycle: "21", end: "2029-12-31", lts: true}], apps: ["temurin-21"], source: "https://endoflife.date/eclipse-temurin",
    }),
    finding({appId: "go", product: "go", productName: "Go", version: "1.26.1", cycle: "1.26", status: "ok", end: undefined, days: undefined, source: "https://endoflife.date/go"}),
    finding({appId: "temurin-17", product: "temurin", productName: "Eclipse Temurin (Java)", version: "17.0.9", cycle: "17", status: "ok", end: "2027-10-31", days: 348, lts: true, source: "https://endoflife.date/eclipse-temurin"}),
]);

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi(makeState({apps: structuredClone(catalogApps)}));
    api.HealthScan.mockResolvedValue(healthResult());
    api.EOLReport.mockResolvedValue(sample());
});

async function open(lang?: "es") {
    if (lang) localStorage.setItem("winforge.settings", JSON.stringify({language: lang}));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(lang ? /apps del catálogo/ : /catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: lang ? "Menú principal" : "Main menu"})).getByRole("button", {name: lang ? /Salud del PC/ : /PC health/}));
    await screen.findByRole("heading", {name: lang ? "Fin de soporte" : "End of support", level: 2});
    return user;
}

const section = () => within(screen.getByRole("region", {name: /End of support|Fin de soporte/}));

describe("End of support section", () => {
    it("lists what ended and what ends soon first, with the date, the cycle and what to do", async () => {
        await open();
        const s = section();
        await s.findByText("Python 3.10");
        const rows = s.getAllByRole("listitem").filter((li) => li.classList.contains("eol-row"));
        // Ended first (longest ago first), then soon; the fine ones are folded away.
        expect(rows.map((r) => r.querySelector("b")?.textContent)).toEqual(["Python 3.10", "PowerShell", "Eclipse Temurin JDK 11"]);
        const py = rows[0];
        expect(within(py).getByText("Support ended")).toBeInTheDocument();
        expect(within(py).getByText("Version 3.10.11")).toBeInTheDocument();
        expect(within(py).getByText("Support for Python 3.10 ended on October 1, 2026 (47 days ago).")).toBeInTheDocument();
        expect(within(py).getByText(/Its maker no longer provides regular updates for this release\. Move to a supported one\./)).toBeInTheDocument();
        expect(within(py).getByText("Supported releases: 3.15 until October 31, 2031; 3.14 until October 31, 2030.")).toBeInTheDocument();
        const jdk = rows[2];
        expect(within(jdk).getByText("Ends soon")).toBeInTheDocument();
        expect(within(jdk).getByText("Support for Eclipse Temurin (Java) 11 ends on May 10, 2027 (in 174 days).")).toBeInTheDocument();
        expect(within(jdk).getByText(/Supported releases: 25 \(long-term support\) until September 30, 2031; 21 \(long-term support\) until December 31, 2029\./)).toBeInTheDocument();
        expect(s.getByText("Support ended: 2. Ends within 180 days: 1. Supported: 2. Not matched: 0.")).toBeInTheDocument();
    });

    it("says what it is based on, what it does not cover and that nothing was sent", async () => {
        await open();
        const s = section();
        expect(await s.findByText(/Based on the data bundled with this version of WinForge, as of November 17, 2026\./)).toBeInTheDocument();
        expect(s.getByText(/It says nothing about security flaws that are not end of support\./)).toBeInTheDocument();
        expect(s.getByText("Nothing was sent anywhere: the list of your software and its versions never leaves this PC.")).toBeInTheDocument();
        expect(s.queryByText(/months old/)).toBeNull();
    });

    it("never relies on colour: every status is an icon plus words", async () => {
        await open();
        const s = section();
        await s.findByText("Python 3.10");
        for (const li of s.getAllByRole("listitem").filter((x) => x.classList.contains("eol-row"))) {
            expect(li.querySelector(".sev svg")).not.toBeNull();
            expect(li.querySelector(".eol-status")?.textContent).toMatch(/^(Support ended|Ends soon|Supported|Not matched)$/);
        }
    });

    it("opens the detail of a newer app the catalog has, and offers nothing for what it does not have", async () => {
        const user = await open();
        const s = section();
        await s.findByText("Python 3.10");
        // The PowerShell row has no app to open: updating it is the way.
        const ps = s.getByText("PowerShell").closest("li")!;
        expect(within(ps).queryByRole("button", {name: /^Open /})).toBeNull();
        expect(within(ps).getByText("Updating it from the Updates page moves it to a newer release.")).toBeInTheDocument();
        await user.click(s.getByRole("button", {name: "Open Python 3.14"}));
        const dialog = await screen.findByRole("dialog", {name: "Python 3.14"});
        expect(dialog).toBeInTheDocument();
    });

    it("hides the supported rows behind a toggle that works from the keyboard", async () => {
        const user = await open();
        const s = section();
        await s.findByText("Python 3.10");
        expect(s.queryByText("Go")).toBeNull();
        const toggle = s.getByRole("button", {name: "Show the rest (2)"});
        expect(toggle).toHaveAttribute("aria-expanded", "false");
        toggle.focus();
        await user.keyboard("{Enter}");
        expect(s.getByRole("button", {name: "Hide the rest (2)"})).toHaveAttribute("aria-expanded", "true");
        const go = s.getByText("Go").closest("li")!;
        expect(within(go).getByText("Supported")).toBeInTheDocument();
        expect(within(go).getByText("Go 1.26 is a current release; no end date has been announced.")).toBeInTheDocument();
    });

    it("opens the source page through the existing link path", async () => {
        const user = await open();
        const s = section();
        await s.findByText("Python 3.10");
        const py = s.getByText("Python 3.10").closest("li")!;
        await user.click(within(py).getByRole("button", {name: /Where this date comes from: Python/}));
        await waitFor(() => expect(api.OpenLink).toHaveBeenCalledWith("https://endoflife.date/python"));
    });

    it("warns when the data is more than a year old", async () => {
        api.EOLReport.mockResolvedValue(report(sample().findings, {asOf: "2025-06-01", ageDays: 534, stale: true}));
        await open();
        const s = section();
        expect(await s.findByText(/This data is 17 months old, so it may be missing recent changes\./)).toBeInTheDocument();
        expect(s.getByText(/as of June 1, 2025\./)).toBeInTheDocument();
    });

    it("says everything is fine when nothing needs attention, and shows the rows", async () => {
        api.EOLReport.mockResolvedValue(report(sample().findings.filter((f) => f.status === "ok")));
        await open();
        const s = section();
        expect(await s.findByText("Everything checked is still supported.")).toBeInTheDocument();
        expect(s.getByText("Go")).toBeInTheDocument();
        expect(s.getByText("Eclipse Temurin JDK 17")).toBeInTheDocument();
        expect(s.queryByRole("button", {name: /Show the rest/})).toBeNull();
    });

    it("says so when there is nothing to check", async () => {
        api.EOLReport.mockResolvedValue(report([]));
        await open();
        expect(await section().findByText("No installed software with a known end-of-support date was found on this PC.")).toBeInTheDocument();
    });

    it("explains a version it could not match, without a date", async () => {
        api.EOLReport.mockResolvedValue(report([
            finding({appId: "python-3-10", status: "unknown", cycle: "", end: undefined, days: undefined, version: "9.9.9"}),
            finding({appId: "go", product: "go", productName: "Go", status: "unknown", cycle: "", end: undefined, days: undefined, version: ""}),
        ]));
        await open();
        const s = section();
        expect(await s.findByText("WinForge could not tell which Python release version 9.9.9 is, so it cannot say whether it is supported.")).toBeInTheDocument();
        expect(s.getByText("WinForge could not read a version number for this Go, so it cannot say whether it is supported.")).toBeInTheDocument();
        expect(s.getAllByText("Not matched").length).toBe(2);
        expect(s.getByText("Version not read")).toBeInTheDocument();
        expect(s.getByText("Everything checked is still supported.")).toBeInTheDocument();
    });

    it("shows a failed read and keeps the page working", async () => {
        api.EOLReport.mockRejectedValue(new Error("table missing"));
        await open();
        expect(await section().findByRole("alert")).toHaveTextContent("The end-of-support report could not be read: table missing");
        expect(screen.getAllByRole("button", {name: /scan/i}).length).toBeGreaterThan(0);
    });

    it("survives a binding that does not exist yet", async () => {
        api.EOLReport.mockImplementation(() => { throw new TypeError("api.EOLReport is not a function"); });
        await open();
        expect(await section().findByRole("alert")).toHaveTextContent("is not a function");
    });

    it("is in Spanish with the same facts", async () => {
        await open("es");
        const s = section();
        expect(await s.findByText(/Basado en los datos incluidos en esta versión de WinForge, a fecha de 17 de noviembre de 2026\./)).toBeInTheDocument();
        expect(s.getByText("El soporte de Python 3.10 terminó el 1 de octubre de 2026 (hace 47 días).")).toBeInTheDocument();
        expect(s.getByText("El soporte de Eclipse Temurin (Java) 11 termina el 10 de mayo de 2027 (dentro de 174 días).")).toBeInTheDocument();
        expect(s.getByText("Soporte terminado: 2. Termina en menos de 180 días: 1. Con soporte: 2. Sin identificar: 0.")).toBeInTheDocument();
        expect(s.getByText("No se ha enviado nada a ningún sitio: la lista de tu software y sus versiones no sale de este PC.")).toBeInTheDocument();
        expect(s.getByRole("button", {name: "Abrir Python 3.14"})).toBeInTheDocument();
        expect(s.getByRole("button", {name: "Mostrar el resto (2)"})).toBeInTheDocument();
    });

    it("warns in Spanish when the data is old", async () => {
        api.EOLReport.mockResolvedValue(report(sample().findings, {asOf: "2025-06-01", ageDays: 534, stale: true}));
        await open("es");
        expect(await section().findByText(/Estos datos tienen 17 meses/)).toBeInTheDocument();
    });
});
