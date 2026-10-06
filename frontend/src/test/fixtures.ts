import type {HealthResult} from "../lib/health";
import type {App, Plan, Profile, ProfileInfo, State} from "../lib/model";

export const app = (o: Partial<App> & {id: string}): App => ({
    name: o.id.toUpperCase(), category: "dev/vcs", description: `${o.id} description`, homepage: "https://example.com",
    winget: `Vendor.${o.id}`, publisher: "Vendor", installed: false, openSource: false, ...o,
});

export const apps: App[] = [
    app({id: "firefox", name: "Firefox", category: "browsers", openSource: true, license: "MPL-2.0", tagline: {en: "Fast, private web browser", es: "Navegador rápido y privado"}}),
    app({id: "vlc", name: "VLC", category: "media", installed: true, version: "3.0.21"}),
    app({id: "git", name: "Git", category: "dev/vcs", admin: true}),
    app({id: "gimp", name: "GIMP", category: "design", openSource: true}),
];

const info = (o: Partial<ProfileInfo> & {id: string; name: string}): ProfileInfo =>
    ({builtin: true, resolved: [], resolvedRecipes: [], description: "", ...o});

export const profiles: ProfileInfo[] = [
    info({id: "gaming", name: "Gaming", description: "Game launchers and chat.", kind: "base", resolved: ["firefox", "vlc"]}),
    info({id: "dev-base", name: "Developer base", kind: "dev", resolved: ["git"]}),
    info({id: "mine", name: "Mine", builtin: false, kind: "custom", resolved: ["gimp"], apps: [{id: "gimp"}]}),
];

export const makeState = (o: Partial<State> = {}): State => ({
    version: "v0.0.0-test", admin: false, apps: structuredClone(apps), profiles: structuredClone(profiles),
    featured: ["firefox", "vlc"], ...o,
});

/** A plan whose steps are the apps of the profile that are not installed. */
export const planFor = (p: Profile, all: App[] = apps): Plan => {
    const ids = [...(p.apps ?? []).map((a) => a.id), ...(p.extends ?? []).flatMap((e) => profiles.find((x) => x.id === e)?.resolved ?? [])];
    const steps = ids.map((id) => all.find((a) => a.id === id)).filter((a): a is App => !!a && !a.installed)
        .map((a) => ({kind: "app" as const, id: a.id, name: a.name, admin: a.admin}));
    return {steps, alreadyInstalled: [], needsAdmin: steps.some((s) => s.admin)};
};

export const healthResult = (o: Partial<HealthResult> = {}): HealthResult => ({
    ok: 1, total: 3, updates: null,
    findings: [
        {key: "device-problems", group: "drivers", severity: "bad", params: {count: "2", devices: "Realtek ACPI\\RTK5452\\1"},
            links: [{kind: "support", label: "ASUS", url: "https://www.asus.com/support/"}]},
        {key: "tpm", group: "firmware", severity: "unknown"},
        {key: "disk-health", group: "storage", severity: "ok"},
    ],
    report: {
        collectedAt: "2026-10-06T10:00:00Z", admin: false,
        machine: {manufacturer: "ASUS", model: "System Product Name", type: "Desktop"},
        os: {caption: "Microsoft Windows 11 Pro", version: "10.0.26200", build: "26200", arch: "64-bit", uptimeDays: 2, pendingReboot: false},
        cpu: {name: "AMD Ryzen 9 7950X", cores: 16, threads: 32}, memory: {totalGB: 63.1, modules: []},
        gpus: [{name: "RTX 4080 SUPER", vendor: "NVIDIA", driverVersion: "32.0.16.1714", driverDate: "2026-09-17"}],
        board: {manufacturer: "ASUSTeK", product: "ROG STRIX X870-A"}, bios: {vendor: "AMI", version: "2402", date: "2026-07-13", uefi: true},
        disks: [{name: "Samsung 990", media: "SSD", bus: "NVMe", sizeGB: 1863, health: "Healthy"}], volumes: [],
        drivers: [{device: "Realtek Audio", class: "MEDIA", manufacturer: "Realtek", version: "6.0.1", date: "2025-01-01", signed: true, inf: "oem1.inf"}],
        problems: [], errors: [],
    },
    ...o,
});
