export namespace catalog {
	
	export class ProfileApp {
	    id: string;
	    version?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProfileApp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.version = source["version"];
	    }
	}
	export class Profile {
	    id: string;
	    name: string;
	    description?: string;
	    kind?: string;
	    extends?: string[];
	    apps?: ProfileApp[];
	    recipes?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.kind = source["kind"];
	        this.extends = source["extends"];
	        this.apps = this.convertValues(source["apps"], ProfileApp);
	        this.recipes = source["recipes"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace health {
	
	export class AV {
	    name: string;
	    enabled: boolean;
	    upToDate: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AV(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.upToDate = source["upToDate"];
	    }
	}
	export class BIOS {
	    vendor: string;
	    version: string;
	    date: string;
	    uefi?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BIOS(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vendor = source["vendor"];
	        this.version = source["version"];
	        this.date = source["date"];
	        this.uefi = source["uefi"];
	    }
	}
	export class Board {
	    manufacturer: string;
	    product: string;
	
	    static createFrom(source: any = {}) {
	        return new Board(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.manufacturer = source["manufacturer"];
	        this.product = source["product"];
	    }
	}
	export class CPU {
	    name: string;
	    cores: number;
	    threads: number;
	    virtualizationOn?: boolean;
	    hypervisorPresent: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CPU(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.cores = source["cores"];
	        this.threads = source["threads"];
	        this.virtualizationOn = source["virtualizationOn"];
	        this.hypervisorPresent = source["hypervisorPresent"];
	    }
	}
	export class Defender {
	    enabled: boolean;
	    realTime: boolean;
	    signatureAgeDays: number;
	
	    static createFrom(source: any = {}) {
	        return new Defender(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.realTime = source["realTime"];
	        this.signatureAgeDays = source["signatureAgeDays"];
	    }
	}
	export class Disk {
	    name: string;
	    media: string;
	    bus: string;
	    sizeGB: number;
	    health: string;
	    operational: string;
	
	    static createFrom(source: any = {}) {
	        return new Disk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.media = source["media"];
	        this.bus = source["bus"];
	        this.sizeGB = source["sizeGB"];
	        this.health = source["health"];
	        this.operational = source["operational"];
	    }
	}
	export class Driver {
	    device: string;
	    class: string;
	    manufacturer: string;
	    version: string;
	    date: string;
	    signed: boolean;
	    inf: string;
	
	    static createFrom(source: any = {}) {
	        return new Driver(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.device = source["device"];
	        this.class = source["class"];
	        this.manufacturer = source["manufacturer"];
	        this.version = source["version"];
	        this.date = source["date"];
	        this.signed = source["signed"];
	        this.inf = source["inf"];
	    }
	}
	export class Firewall {
	    profile: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Firewall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = source["profile"];
	        this.enabled = source["enabled"];
	    }
	}
	export class GPU {
	    name: string;
	    vendor: string;
	    driverVersion: string;
	    driverDate: string;
	
	    static createFrom(source: any = {}) {
	        return new GPU(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.vendor = source["vendor"];
	        this.driverVersion = source["driverVersion"];
	        this.driverDate = source["driverDate"];
	    }
	}
	export class Link {
	    kind: string;
	    label: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new Link(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.label = source["label"];
	        this.url = source["url"];
	    }
	}
	export class Machine {
	    manufacturer: string;
	    model: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new Machine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.manufacturer = source["manufacturer"];
	        this.model = source["model"];
	        this.type = source["type"];
	    }
	}
	export class Module {
	    capacityGB: number;
	    speedMHz: number;
	    configuredMHz: number;
	    manufacturer: string;
	    part: string;
	
	    static createFrom(source: any = {}) {
	        return new Module(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.capacityGB = source["capacityGB"];
	        this.speedMHz = source["speedMHz"];
	        this.configuredMHz = source["configuredMHz"];
	        this.manufacturer = source["manufacturer"];
	        this.part = source["part"];
	    }
	}
	export class Memory {
	    totalGB: number;
	    modules: Module[];
	
	    static createFrom(source: any = {}) {
	        return new Memory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalGB = source["totalGB"];
	        this.modules = this.convertValues(source["modules"], Module);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class OS {
	    caption: string;
	    version: string;
	    build: string;
	    arch: string;
	    uptimeDays: number;
	    pendingReboot: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OS(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.caption = source["caption"];
	        this.version = source["version"];
	        this.build = source["build"];
	        this.arch = source["arch"];
	        this.uptimeDays = source["uptimeDays"];
	        this.pendingReboot = source["pendingReboot"];
	    }
	}
	export class Problem {
	    device: string;
	    class: string;
	    hardwareId: string;
	    code: number;
	
	    static createFrom(source: any = {}) {
	        return new Problem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.device = source["device"];
	        this.class = source["class"];
	        this.hardwareId = source["hardwareId"];
	        this.code = source["code"];
	    }
	}
	export class TPM {
	    present: boolean;
	    ready: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TPM(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.present = source["present"];
	        this.ready = source["ready"];
	    }
	}
	export class Security {
	    antivirus: AV[];
	    defender?: Defender;
	    firewall: Firewall[];
	    secureBoot?: boolean;
	    tpm?: TPM;
	    bitlockerSystem?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Security(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.antivirus = this.convertValues(source["antivirus"], AV);
	        this.defender = this.convertValues(source["defender"], Defender);
	        this.firewall = this.convertValues(source["firewall"], Firewall);
	        this.secureBoot = source["secureBoot"];
	        this.tpm = this.convertValues(source["tpm"], TPM);
	        this.bitlockerSystem = source["bitlockerSystem"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Volume {
	    letter: string;
	    sizeGB: number;
	    freeGB: number;
	    fileSystem: string;
	
	    static createFrom(source: any = {}) {
	        return new Volume(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.letter = source["letter"];
	        this.sizeGB = source["sizeGB"];
	        this.freeGB = source["freeGB"];
	        this.fileSystem = source["fileSystem"];
	    }
	}
	export class Report {
	    // Go type: time
	    collectedAt: any;
	    admin: boolean;
	    machine: Machine;
	    os: OS;
	    cpu: CPU;
	    memory: Memory;
	    gpus: GPU[];
	    board: Board;
	    bios: BIOS;
	    disks: Disk[];
	    volumes: Volume[];
	    security: Security;
	    drivers: Driver[];
	    problems: Problem[];
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.collectedAt = this.convertValues(source["collectedAt"], null);
	        this.admin = source["admin"];
	        this.machine = this.convertValues(source["machine"], Machine);
	        this.os = this.convertValues(source["os"], OS);
	        this.cpu = this.convertValues(source["cpu"], CPU);
	        this.memory = this.convertValues(source["memory"], Memory);
	        this.gpus = this.convertValues(source["gpus"], GPU);
	        this.board = this.convertValues(source["board"], Board);
	        this.bios = this.convertValues(source["bios"], BIOS);
	        this.disks = this.convertValues(source["disks"], Disk);
	        this.volumes = this.convertValues(source["volumes"], Volume);
	        this.security = this.convertValues(source["security"], Security);
	        this.drivers = this.convertValues(source["drivers"], Driver);
	        this.problems = this.convertValues(source["problems"], Problem);
	        this.errors = source["errors"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class Update {
	    title: string;
	    kind: string;
	    category: string;
	    sizeMB: number;
	    reboot: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Update(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.kind = source["kind"];
	        this.category = source["category"];
	        this.sizeMB = source["sizeMB"];
	        this.reboot = source["reboot"];
	    }
	}
	export class UpdateScan {
	    // Go type: time
	    checkedAt: any;
	    updates: Update[];
	
	    static createFrom(source: any = {}) {
	        return new UpdateScan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.checkedAt = this.convertValues(source["checkedAt"], null);
	        this.updates = this.convertValues(source["updates"], Update);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace install {
	
	export class Step {
	    kind: string;
	    id: string;
	    name: string;
	    version?: string;
	    admin?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Step(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.id = source["id"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.admin = source["admin"];
	    }
	}
	export class Plan {
	    steps: Step[];
	    alreadyInstalled: string[];
	    skippedRecipes?: string[];
	    needsAdmin: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Plan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.steps = this.convertValues(source["steps"], Step);
	        this.alreadyInstalled = source["alreadyInstalled"];
	        this.skippedRecipes = source["skippedRecipes"];
	        this.needsAdmin = source["needsAdmin"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class AppInfo {
	    id: string;
	    name: string;
	    category: string;
	    description: string;
	    homepage: string;
	    license?: string;
	    tags?: string[];
	    winget: string;
	    publisher: string;
	    trust?: string;
	    scope?: string;
	    override?: string;
	    admin?: boolean;
	    // Go type: catalog
	    detect?: any;
	    requires?: string[];
	    recipes?: string[];
	    notes?: string;
	    // Go type: catalog
	    tagline?: any;
	    openSource: boolean;
	    installed: boolean;
	    version?: string;
	    sources?: string[];
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.category = source["category"];
	        this.description = source["description"];
	        this.homepage = source["homepage"];
	        this.license = source["license"];
	        this.tags = source["tags"];
	        this.winget = source["winget"];
	        this.publisher = source["publisher"];
	        this.trust = source["trust"];
	        this.scope = source["scope"];
	        this.override = source["override"];
	        this.admin = source["admin"];
	        this.detect = this.convertValues(source["detect"], null);
	        this.requires = source["requires"];
	        this.recipes = source["recipes"];
	        this.notes = source["notes"];
	        this.tagline = this.convertValues(source["tagline"], null);
	        this.openSource = source["openSource"];
	        this.installed = source["installed"];
	        this.version = source["version"];
	        this.sources = source["sources"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HealthFinding {
	    key: string;
	    group: string;
	    severity: string;
	    params?: Record<string, string>;
	    links?: health.Link[];
	    winget?: string;
	    catalogId?: string;
	
	    static createFrom(source: any = {}) {
	        return new HealthFinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.group = source["group"];
	        this.severity = source["severity"];
	        this.params = source["params"];
	        this.links = this.convertValues(source["links"], health.Link);
	        this.winget = source["winget"];
	        this.catalogId = source["catalogId"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HealthResult {
	    report?: health.Report;
	    findings: HealthFinding[];
	    updates?: health.UpdateScan;
	    ok: number;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new HealthResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.report = this.convertValues(source["report"], health.Report);
	        this.findings = this.convertValues(source["findings"], HealthFinding);
	        this.updates = this.convertValues(source["updates"], health.UpdateScan);
	        this.ok = source["ok"];
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportResult {
	    profile: catalog.Profile;
	    unknownApps: string[];
	    unknownRecipes: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = this.convertValues(source["profile"], catalog.Profile);
	        this.unknownApps = source["unknownApps"];
	        this.unknownRecipes = source["unknownRecipes"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProfileInfo {
	    id: string;
	    name: string;
	    description?: string;
	    kind?: string;
	    extends?: string[];
	    apps?: catalog.ProfileApp[];
	    recipes?: string[];
	    builtin: boolean;
	    resolved: string[];
	    resolvedRecipes: string[];
	
	    static createFrom(source: any = {}) {
	        return new ProfileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.kind = source["kind"];
	        this.extends = source["extends"];
	        this.apps = this.convertValues(source["apps"], catalog.ProfileApp);
	        this.recipes = source["recipes"];
	        this.builtin = source["builtin"];
	        this.resolved = source["resolved"];
	        this.resolvedRecipes = source["resolvedRecipes"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class State {
	    version: string;
	    admin: boolean;
	    apps: AppInfo[];
	    profiles: ProfileInfo[];
	    featured: string[];
	    wingetError?: string;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.admin = source["admin"];
	        this.apps = this.convertValues(source["apps"], AppInfo);
	        this.profiles = this.convertValues(source["profiles"], ProfileInfo);
	        this.featured = source["featured"];
	        this.wingetError = source["wingetError"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UpdateInfo {
	    current: string;
	    latest: string;
	    available: boolean;
	    url: string;
	    notes: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.available = source["available"];
	        this.url = source["url"];
	        this.notes = source["notes"];
	    }
	}
	export class UpgradeInfo {
	    id: string;
	    name: string;
	    publisher: string;
	    current: string;
	    available: string;
	
	    static createFrom(source: any = {}) {
	        return new UpgradeInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.publisher = source["publisher"];
	        this.current = source["current"];
	        this.available = source["available"];
	    }
	}

}

