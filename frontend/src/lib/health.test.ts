import {beforeEach, describe, expect, it} from "vitest";
import {
    GROUPS, attention, byGroup, filterDrivers, findingText, isThirdParty, reportMarkdown,
    type Finding, type HDriver, type HealthResult,
} from "./health";
import {KEYS, setLang} from "./i18n";
import {readFileSync} from "node:fs";
import {resolve} from "node:path";

const f = (key: string, severity: Finding["severity"], group: Finding["group"] = "drivers", params: Record<string, string> = {}): Finding =>
    ({key, severity, group, params});

describe("finding text", () => {
    beforeEach(() => setLang("en"));

    it("prefers severity-specific wording and falls back to the base text", () => {
        expect(findingText(f("device-problems", "ok"), "title")).toBe("No devices with driver problems");
        expect(findingText(f("device-problems", "bad", "drivers", {count: "2", devices: "A; B"}), "title")).toBe("2 devices need a driver");
        expect(findingText(f("uefi-mode", "info"), "detail")).toContain("Legacy BIOS");
        expect(findingText(f("uefi-mode", "ok"), "detail")).toContain("UEFI");
        expect(findingText(f("bios-age", "unknown"), "title")).toBe("BIOS date unknown");
    });
    it("works in Spanish and never leaks a raw placeholder for missing params", () => {
        setLang("es");
        expect(findingText(f("pending-reboot", "warn"), "title")).toBe("Hay un reinicio pendiente");
        expect(findingText(f("gpu-driver", "info", "drivers", {gpu: "RTX", version: "617.14"}), "title")).toBe("RTX: driver 617.14");
        expect(findingText(f("uptime", "info"), "title")).not.toContain("{");
    });
    it("falls back to the key for something the interface does not know", () => {
        expect(findingText(f("brand-new-rule", "warn"), "title")).toBe("brand-new-rule");
    });
});

describe("every finding the backend can emit is worded", () => {
    // The Go side lists the keys it can emit; the interface must word each one.
    const go = readFileSync(resolve(__dirname, "../../../internal/health/analyze.go"), "utf8");
    const block = go.slice(go.indexOf("var Keys = []string{"), go.indexOf("}", go.indexOf("var Keys = []string{")));
    const emitted = [...block.matchAll(/"([a-z-]+)"/g)].map((m) => m[1]);

    it("reads the key list from the Go source", () => {
        expect(emitted.length).toBeGreaterThanOrEqual(15);
        expect(emitted).toContain("bios-age");
    });
    for (const lang of ["en", "es"] as const) {
        it(`has a title and a detail for each key in ${lang}`, () => {
            setLang(lang);
            for (const k of emitted) {
                expect(KEYS as string[], `finding.${k}.title`).toContain(`finding.${k}.title`);
                expect(KEYS as string[], `finding.${k}.detail`).toContain(`finding.${k}.detail`);
            }
        });
    }
});

describe("grouping", () => {
    const fs = [f("a", "ok", "security"), f("b", "bad", "drivers"), f("c", "warn", "firmware"), f("d", "info", "drivers"), f("e", "unknown", "windows")];
    it("keeps the group order and drops empty groups", () => {
        expect(byGroup(fs).map(([g]) => g)).toEqual(["firmware", "drivers", "security", "windows"]);
        expect(GROUPS).toHaveLength(5);
    });
    it("lists only warnings and problems, worst first", () => {
        expect(attention(fs).map((x) => x.key)).toEqual(["b", "c"]);
    });
});

describe("drivers", () => {
    const d = (device: string, manufacturer: string, version: string, date: string): HDriver =>
        ({device, class: "NET", manufacturer, version, date, signed: true, inf: "x.inf"});
    const list = [
        d("Realtek NIC", "Realtek", "10.1.2.3", "2020-01-01"),
        d("ACPI Button", "(Standard system devices)", "10.0.26100.1", "2006-06-21"),
        d("NVIDIA GPU", "NVIDIA", "32.0.16.1714", "2026-09-17"),
        d("AMD Processor", "Advanced Micro Devices", "10.0.26100.9278", "2009-04-21"),
    ];
    const now = new Date("2026-10-06");
    it("recognises in-box Windows drivers", () => {
        expect(list.map(isThirdParty)).toEqual([true, false, true, false]);
    });
    it("filters by vendor-only, age and text, oldest first", () => {
        expect(filterDrivers(list, {query: "", thirdPartyOnly: true, olderThanYears: null}, now).map((x) => x.device)).toEqual(["Realtek NIC", "NVIDIA GPU"]);
        expect(filterDrivers(list, {query: "", thirdPartyOnly: true, olderThanYears: 2}, now).map((x) => x.device)).toEqual(["Realtek NIC"]);
        expect(filterDrivers(list, {query: "nvidia", thirdPartyOnly: false, olderThanYears: null}, now)).toHaveLength(1);
        expect(filterDrivers(list, {query: "", thirdPartyOnly: false, olderThanYears: null}, now)[0].device).toBe("ACPI Button");
    });
});

describe("report", () => {
    beforeEach(() => setLang("en"));
    const result: HealthResult = {
        ok: 1, total: 2, updates: {checkedAt: "2026-10-06T10:00:00Z", updates: [{title: "Realtek - Audio", kind: "Driver", category: "", sizeMB: 1, reboot: false}]},
        findings: [f("pending-reboot", "warn", "windows"), f("uptime", "ok", "windows", {days: "2"})],
        report: {
            collectedAt: "2026-10-06T10:00:00Z", admin: false,
            machine: {manufacturer: "ASUS", model: "System Product Name", type: "Desktop"},
            os: {caption: "Microsoft Windows 11 Pro", version: "10.0.26100", build: "26100", arch: "64-bit", uptimeDays: 2, pendingReboot: true},
            cpu: {name: "AMD Ryzen 9 7950X", cores: 16, threads: 32}, memory: {totalGB: 63.1, modules: []},
            gpus: [{name: "RTX 4080 SUPER", vendor: "NVIDIA", driverVersion: "32.0.16.1714", driverDate: "2026-09-17"}],
            board: {manufacturer: "ASUSTeK", product: "ROG STRIX X870-A"}, bios: {vendor: "AMI", version: "2402", date: "2026-07-13", uefi: true},
            disks: [{name: "Samsung 990", media: "SSD", bus: "NVMe", sizeGB: 1863, health: "Healthy"}], volumes: [],
            drivers: [{device: "Pipe | Device", class: "NET", manufacturer: "Acme", version: "1.0", date: "2024-01-01", signed: true, inf: "a.inf"},
                {device: "In-box", class: "SYSTEM", manufacturer: "(Standard system devices)", version: "10.0.1", date: "2006-06-21", signed: true, inf: "b.inf"}],
            problems: [], errors: [],
        },
    };
    const labels = {group: (g: string) => g.toUpperCase(), title: "PC health report", checks: "Checks", drivers: "Drivers", updates: "Windows Update"};

    it("renders a shareable Markdown report without in-box drivers", () => {
        const md = reportMarkdown(result, labels);
        expect(md).toContain("# PC health report");
        expect(md).toContain("ASUS System Product Name");
        expect(md).toContain("## Checks: 1/2");
        expect(md).toContain("⚠ **A restart is pending**");
        expect(md).toContain("- Driver: Realtek - Audio");
        expect(md).toContain("| Pipe / Device | Acme | 1.0 | 2024-01-01 |");
        expect(md).not.toContain("In-box");
    });
    it("contains no serial numbers or user names by construction", () => {
        const md = reportMarkdown(result, labels).toLowerCase();
        expect(md).not.toContain("serial");
        expect(md).not.toContain("c:\\users");
    });
});
