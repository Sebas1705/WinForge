// Types mirroring the PC health JSON, plus the pure helpers the page uses.
import {tOpt} from "./i18n";

export type Severity = "ok" | "info" | "warn" | "bad" | "unknown";
export type Group = "firmware" | "drivers" | "storage" | "security" | "windows";
export const GROUPS: Group[] = ["firmware", "drivers", "storage", "security", "windows"];

export interface HealthLink { kind: "support" | "download" | "search" | "settings"; label: string; url: string }

export interface Finding {
    key: string;
    group: Group;
    severity: Severity;
    params?: Record<string, string>;
    links?: HealthLink[];
    winget?: string;
    catalogId?: string;
}

export interface HDriver { device: string; class: string; manufacturer: string; version: string; date: string; signed: boolean; inf: string }

export interface HealthReport {
    collectedAt: string;
    admin: boolean;
    machine: { manufacturer: string; model: string; type: string };
    os: { caption: string; version: string; build: string; arch: string; uptimeDays: number; pendingReboot: boolean };
    cpu: { name: string; cores: number; threads: number };
    memory: { totalGB: number; modules: { capacityGB: number; speedMHz: number; configuredMHz: number; manufacturer: string; part: string }[] };
    gpus: { name: string; vendor: string; driverVersion: string; driverDate: string }[];
    board: { manufacturer: string; product: string };
    bios: { vendor: string; version: string; date: string; uefi: boolean | null };
    disks: { name: string; media: string; bus: string; sizeGB: number; health: string }[];
    volumes: { letter: string; sizeGB: number; freeGB: number }[];
    drivers: HDriver[];
    problems: { device: string; class: string; hardwareId: string; code: number }[];
    errors: string[];
}

export interface WUpdate { title: string; kind: string; category: string; sizeMB: number; reboot: boolean }
export interface UpdateScan { checkedAt: string; updates: WUpdate[] }

export interface HealthResult {
    report: HealthReport;
    findings: Finding[];
    updates: UpdateScan | null;
    ok: number;
    total: number;
}

/** Text for a finding in the current language, with severity-specific wording when the dictionary has it. */
export function findingText(f: Finding, part: "title" | "detail"): string {
    const vars = f.params ?? {};
    return tOpt(`finding.${f.key}.${part}.${f.severity}`, vars) ?? tOpt(`finding.${f.key}.${part}`, vars) ?? f.key;
}

export function byGroup(findings: Finding[]): [Group, Finding[]][] {
    return GROUPS.map((g) => [g, findings.filter((f) => f.group === g)] as [Group, Finding[]]).filter(([, l]) => l.length > 0);
}

/** Findings that need attention, worst first. */
export function attention(findings: Finding[]): Finding[] {
    const rank: Record<Severity, number> = {bad: 0, warn: 1, info: 2, unknown: 3, ok: 4};
    return findings.filter((f) => f.severity === "bad" || f.severity === "warn")
        .sort((a, b) => rank[a.severity] - rank[b.severity]);
}

/** A driver shipped by Windows itself rather than by a hardware vendor (mirrors the Go rule). */
export function isThirdParty(d: HDriver): boolean {
    const m = d.manufacturer.trim().toLowerCase();
    if (m.startsWith("(standard") || m === "microsoft" || m.startsWith("microsoft ")) return false;
    return !d.version.startsWith("10.0.");
}

export interface DriverFilter { query: string; thirdPartyOnly: boolean; olderThanYears: number | null }

export function ageYears(date: string, now: Date): number | null {
    const t = Date.parse(date);
    return Number.isNaN(t) ? null : (now.getTime() - t) / (365.25 * 24 * 3600 * 1000);
}

export function filterDrivers(drivers: HDriver[], f: DriverFilter, now: Date): HDriver[] {
    const q = f.query.trim().toLowerCase();
    return drivers.filter((d) => {
        if (f.thirdPartyOnly && !isThirdParty(d)) return false;
        if (f.olderThanYears !== null) {
            const a = ageYears(d.date, now);
            if (a === null || a < f.olderThanYears) return false;
        }
        return !q || `${d.device} ${d.class} ${d.manufacturer} ${d.version}`.toLowerCase().includes(q);
    }).sort((a, b) => a.date.localeCompare(b.date) || a.device.localeCompare(b.device));
}

const MARK: Record<Severity, string> = {ok: "✓", info: "ℹ", warn: "⚠", bad: "✗", unknown: "?"};

/**
 * A shareable report in Markdown. It contains hardware and driver names only:
 * no serial numbers, user names or addresses are ever collected.
 */
export function reportMarkdown(r: HealthResult, labels: { group: (g: Group) => string; title: string; checks: string; drivers: string; updates: string }): string {
    const x = r.report;
    const lines: string[] = [];
    lines.push(`# ${labels.title}`, "", `${new Date(x.collectedAt).toISOString().slice(0, 10)} · WinForge`, "");
    lines.push(`- **${x.machine.manufacturer} ${x.machine.model}**${x.machine.type ? ` (${x.machine.type})` : ""}`);
    lines.push(`- ${x.os.caption} ${x.os.version} (build ${x.os.build}, ${x.os.arch})`);
    lines.push(`- ${x.cpu.name}, ${x.cpu.cores}/${x.cpu.threads}`);
    lines.push(`- RAM ${x.memory.totalGB} GB`);
    for (const g of x.gpus) lines.push(`- GPU ${g.name}, driver ${g.driverVersion} (${g.driverDate})`);
    lines.push(`- Board ${x.board.manufacturer} ${x.board.product}`);
    lines.push(`- BIOS ${x.bios.vendor} ${x.bios.version} (${x.bios.date})`);
    for (const d of x.disks) lines.push(`- Disk ${d.name} ${d.sizeGB} GB, ${d.health}`);
    lines.push("", `## ${labels.checks}: ${r.ok}/${r.total}`, "");
    for (const [g, list] of byGroup(r.findings)) {
        lines.push(`### ${labels.group(g)}`, "");
        for (const f of list) lines.push(`- ${MARK[f.severity]} **${findingText(f, "title")}**. ${findingText(f, "detail")}`);
        lines.push("");
    }
    if (r.updates && r.updates.updates.length > 0) {
        lines.push(`## ${labels.updates}`, "");
        for (const u of r.updates.updates) lines.push(`- ${u.kind}: ${u.title}`);
        lines.push("");
    }
    const tp = x.drivers.filter(isThirdParty).sort((a, b) => a.date.localeCompare(b.date));
    if (tp.length > 0) {
        lines.push(`## ${labels.drivers}`, "", "| Device | Maker | Version | Date |", "|---|---|---|---|");
        for (const d of tp) lines.push(`| ${d.device.replace(/\|/g, "/")} | ${d.manufacturer.replace(/\|/g, "/")} | ${d.version} | ${d.date} |`);
        lines.push("");
    }
    return lines.join("\n");
}
