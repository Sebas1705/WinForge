export namespace battery {
	
	export class Battery {
	    chemistry?: string;
	    designMWh: number;
	    fullMWh: number;
	    healthPct: number;
	    cycles?: number;
	    verdict: string;
	
	    static createFrom(source: any = {}) {
	        return new Battery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chemistry = source["chemistry"];
	        this.designMWh = source["designMWh"];
	        this.fullMWh = source["fullMWh"];
	        this.healthPct = source["healthPct"];
	        this.cycles = source["cycles"];
	        this.verdict = source["verdict"];
	    }
	}
	export class Point {
	    date: string;
	    designMWh: number;
	    fullMWh: number;
	    healthPct: number;
	    cycles?: number;
	
	    static createFrom(source: any = {}) {
	        return new Point(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.designMWh = source["designMWh"];
	        this.fullMWh = source["fullMWh"];
	        this.healthPct = source["healthPct"];
	        this.cycles = source["cycles"];
	    }
	}
	export class Report {
	    present: boolean;
	    batteries: Battery[];
	    designMWh: number;
	    fullMWh: number;
	    healthPct: number;
	    cycles?: number;
	    verdict: string;
	    history: Point[];
	    goodMinPct: number;
	    replaceBelowPct: number;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.present = source["present"];
	        this.batteries = this.convertValues(source["batteries"], Battery);
	        this.designMWh = source["designMWh"];
	        this.fullMWh = source["fullMWh"];
	        this.healthPct = source["healthPct"];
	        this.cycles = source["cycles"];
	        this.verdict = source["verdict"];
	        this.history = this.convertValues(source["history"], Point);
	        this.goodMinPct = source["goodMinPct"];
	        this.replaceBelowPct = source["replaceBelowPct"];
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

export namespace catalog {
	
	export class RunSpec {
	    exes: string[];
	    hints: string[];
	    args?: string[];
	    register?: string[];
	
	    static createFrom(source: any = {}) {
	        return new RunSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exes = source["exes"];
	        this.hints = source["hints"];
	        this.args = source["args"];
	        this.register = source["register"];
	    }
	}
	export class Source {
	    id: string;
	    system: string;
	    kind: string;
	    name: string;
	    en: string;
	    es: string;
	    url: string;
	    base?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Source(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.system = source["system"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.en = source["en"];
	        this.es = source["es"];
	        this.url = source["url"];
	        this.base = source["base"];
	    }
	}
	export class System {
	    id: string;
	    en: string;
	    es: string;
	
	    static createFrom(source: any = {}) {
	        return new System(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.en = source["en"];
	        this.es = source["es"];
	    }
	}
	export class Emulation {
	    systems: System[];
	    emulators: Record<string, Array<string>>;
	    sources: Source[];
	    run: Record<string, RunSpec>;
	
	    static createFrom(source: any = {}) {
	        return new Emulation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.systems = this.convertValues(source["systems"], System);
	        this.emulators = source["emulators"];
	        this.sources = this.convertValues(source["sources"], Source);
	        this.run = this.convertValues(source["run"], RunSpec, true);
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
	export class Game {
	    id: string;
	    name: string;
	    system: string;
	    en: string;
	    es: string;
	    license: string;
	    homepage: string;
	    url: string;
	    sha256: string;
	    size: number;
	    kind: string;
	    file?: string;
	    entry?: string;
	
	    static createFrom(source: any = {}) {
	        return new Game(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.system = source["system"];
	        this.en = source["en"];
	        this.es = source["es"];
	        this.license = source["license"];
	        this.homepage = source["homepage"];
	        this.url = source["url"];
	        this.sha256 = source["sha256"];
	        this.size = source["size"];
	        this.kind = source["kind"];
	        this.file = source["file"];
	        this.entry = source["entry"];
	    }
	}
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

export namespace envdoctor {
	
	export class Backup {
	    id: string;
	    // Go type: time
	    time: any;
	    changed: string[];
	
	    static createFrom(source: any = {}) {
	        return new Backup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.time = this.convertValues(source["time"], null);
	        this.changed = source["changed"];
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
	export class Finding {
	    id: string;
	    kind: string;
	    severity: string;
	    scope: string;
	    subject: string;
	    paths: string[];
	    fixable: boolean;
	    fixKind: string;
	
	    static createFrom(source: any = {}) {
	        return new Finding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.severity = source["severity"];
	        this.scope = source["scope"];
	        this.subject = source["subject"];
	        this.paths = source["paths"];
	        this.fixable = source["fixable"];
	        this.fixKind = source["fixKind"];
	    }
	}
	export class FixResult {
	    applied: number;
	    failed: string[];
	    backupId: string;
	
	    static createFrom(source: any = {}) {
	        return new FixResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.applied = source["applied"];
	        this.failed = source["failed"];
	        this.backupId = source["backupId"];
	    }
	}
	export class Var {
	    name: string;
	    scope: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new Var(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.scope = source["scope"];
	        this.value = source["value"];
	    }
	}
	export class Tool {
	    name: string;
	    found: string[];
	
	    static createFrom(source: any = {}) {
	        return new Tool(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.found = source["found"];
	    }
	}
	export class Report {
	    findings: Finding[];
	    userPath: string[];
	    machinePath: string[];
	    tools: Tool[];
	    vars: Var[];
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.findings = this.convertValues(source["findings"], Finding);
	        this.userPath = source["userPath"];
	        this.machinePath = source["machinePath"];
	        this.tools = this.convertValues(source["tools"], Tool);
	        this.vars = this.convertValues(source["vars"], Var);
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

export namespace eol {
	
	export class Replacement {
	    cycle: string;
	    end?: string;
	    lts?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Replacement(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cycle = source["cycle"];
	        this.end = source["end"];
	        this.lts = source["lts"];
	    }
	}
	export class Finding {
	    appId: string;
	    product: string;
	    productName: string;
	    version: string;
	    cycle: string;
	    status: string;
	    end?: string;
	    days?: number;
	    lts?: boolean;
	    source: string;
	    replace: Replacement[];
	    apps: string[];
	    selfUpdate?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Finding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.appId = source["appId"];
	        this.product = source["product"];
	        this.productName = source["productName"];
	        this.version = source["version"];
	        this.cycle = source["cycle"];
	        this.status = source["status"];
	        this.end = source["end"];
	        this.days = source["days"];
	        this.lts = source["lts"];
	        this.source = source["source"];
	        this.replace = this.convertValues(source["replace"], Replacement);
	        this.apps = source["apps"];
	        this.selfUpdate = source["selfUpdate"];
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
	
	export class Report {
	    asOf: string;
	    ageDays: number;
	    stale: boolean;
	    findings: Finding[];
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.asOf = source["asOf"];
	        this.ageDays = source["ageDays"];
	        this.stale = source["stale"];
	        this.findings = this.convertValues(source["findings"], Finding);
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

export namespace games {
	
	export class Receipt {
	    id: string;
	    sha256: string;
	    url: string;
	    files: number;
	    // Go type: time
	    installedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Receipt(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sha256 = source["sha256"];
	        this.url = source["url"];
	        this.files = source["files"];
	        this.installedAt = this.convertValues(source["installedAt"], null);
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

export namespace healthhistory {
	
	export class BatteryPoint {
	    designMWh: number;
	    fullMWh: number;
	    cycles?: number;
	
	    static createFrom(source: any = {}) {
	        return new BatteryPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.designMWh = source["designMWh"];
	        this.fullMWh = source["fullMWh"];
	        this.cycles = source["cycles"];
	    }
	}
	export class Disk {
	    name: string;
	    media?: string;
	    health?: string;
	    wearPct?: number;
	    tempC?: number;
	    powerOnHours?: number;
	
	    static createFrom(source: any = {}) {
	        return new Disk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.media = source["media"];
	        this.health = source["health"];
	        this.wearPct = source["wearPct"];
	        this.tempC = source["tempC"];
	        this.powerOnHours = source["powerOnHours"];
	    }
	}
	export class Volume {
	    letter: string;
	    freeBytes: number;
	    totalBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new Volume(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.letter = source["letter"];
	        this.freeBytes = source["freeBytes"];
	        this.totalBytes = source["totalBytes"];
	    }
	}
	export class Snapshot {
	    // Go type: time
	    at: any;
	    score: number;
	    total: number;
	    areas: Record<string, number>;
	    volumes: Volume[];
	    disks: Disk[];
	    battery?: BatteryPoint;
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.at = this.convertValues(source["at"], null);
	        this.score = source["score"];
	        this.total = source["total"];
	        this.areas = source["areas"];
	        this.volumes = this.convertValues(source["volumes"], Volume);
	        this.disks = this.convertValues(source["disks"], Disk);
	        this.battery = this.convertValues(source["battery"], BatteryPoint);
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
	export class Statement {
	    code: string;
	    params: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new Statement(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.params = source["params"];
	    }
	}
	export class View {
	    snapshots: Snapshot[];
	    statements: Statement[];
	    enough: boolean;
	
	    static createFrom(source: any = {}) {
	        return new View(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.snapshots = this.convertValues(source["snapshots"], Snapshot);
	        this.statements = this.convertValues(source["statements"], Statement);
	        this.enough = source["enough"];
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

export namespace history {
	
	export class AppRef {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new AppRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class Filter {
	    app: string;
	    result: string;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new Filter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.app = source["app"];
	        this.result = source["result"];
	        this.limit = source["limit"];
	    }
	}
	export class View {
	    id: string;
	    app: string;
	    name: string;
	    action: string;
	    from: string;
	    to: string;
	    // Go type: time
	    at: any;
	    result: string;
	    source: string;
	    reason?: string;
	    canRollback: boolean;
	
	    static createFrom(source: any = {}) {
	        return new View(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.app = source["app"];
	        this.name = source["name"];
	        this.action = source["action"];
	        this.from = source["from"];
	        this.to = source["to"];
	        this.at = this.convertValues(source["at"], null);
	        this.result = source["result"];
	        this.source = source["source"];
	        this.reason = source["reason"];
	        this.canRollback = source["canRollback"];
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
	export class Page {
	    entries: View[];
	    total: number;
	    apps: AppRef[];
	
	    static createFrom(source: any = {}) {
	        return new Page(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], View);
	        this.total = source["total"];
	        this.apps = this.convertValues(source["apps"], AppRef);
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
	export class Pending {
	    title: string;
	    steps: Step[];
	
	    static createFrom(source: any = {}) {
	        return new Pending(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.steps = this.convertValues(source["steps"], Step);
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
	export class AppPreview {
	    id: string;
	    name: string;
	    installed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.installed = source["installed"];
	    }
	}
	export class AssistantModeText {
	    read: string;
	    plan: string;
	    full: string;
	
	    static createFrom(source: any = {}) {
	        return new AssistantModeText(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.read = source["read"];
	        this.plan = source["plan"];
	        this.full = source["full"];
	    }
	}
	export class AssistantCommands {
	    claudeCode: AssistantModeText;
	    claudeDesktopJson: string;
	    claudeDesktop: AssistantModeText;
	
	    static createFrom(source: any = {}) {
	        return new AssistantCommands(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.claudeCode = this.convertValues(source["claudeCode"], AssistantModeText);
	        this.claudeDesktopJson = source["claudeDesktopJson"];
	        this.claudeDesktop = this.convertValues(source["claudeDesktop"], AssistantModeText);
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
	export class AssistantInfoResult {
	    exePath: string;
	    exists: boolean;
	    installedNextToApp: boolean;
	    commands: AssistantCommands;
	    modes: string[];
	
	    static createFrom(source: any = {}) {
	        return new AssistantInfoResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exePath = source["exePath"];
	        this.exists = source["exists"];
	        this.installedNextToApp = source["installedNextToApp"];
	        this.commands = this.convertValues(source["commands"], AssistantCommands);
	        this.modes = source["modes"];
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
	
	export class AutoSettings {
	    mode: string;
	    frequency: string;
	    weekday: number;
	    time: string;
	
	    static createFrom(source: any = {}) {
	        return new AutoSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.frequency = source["frequency"];
	        this.weekday = source["weekday"];
	        this.time = source["time"];
	    }
	}
	export class AutoState {
	    on: boolean;
	    enabled: boolean;
	    foreign: boolean;
	    mode: string;
	    frequency: string;
	    weekday: number;
	    time: string;
	    nextRun?: string;
	    canEnable: boolean;
	    problem?: string;
	    runs: sched.Report[];
	
	    static createFrom(source: any = {}) {
	        return new AutoState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.on = source["on"];
	        this.enabled = source["enabled"];
	        this.foreign = source["foreign"];
	        this.mode = source["mode"];
	        this.frequency = source["frequency"];
	        this.weekday = source["weekday"];
	        this.time = source["time"];
	        this.nextRun = source["nextRun"];
	        this.canEnable = source["canEnable"];
	        this.problem = source["problem"];
	        this.runs = this.convertValues(source["runs"], sched.Report);
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
	export class SetPreview {
	    id: string;
	    files: number;
	    new: number;
	    changed: number;
	    same: number;
	
	    static createFrom(source: any = {}) {
	        return new SetPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.files = source["files"];
	        this.new = source["new"];
	        this.changed = source["changed"];
	        this.same = source["same"];
	    }
	}
	export class BackupPreview {
	    host: string;
	    app: string;
	    createdAt: string;
	    intact: boolean;
	    checked: boolean;
	    problems: string[];
	    sets: SetPreview[];
	    apps: AppPreview[];
	    missing: number;
	    unknown: string[];
	    profile?: catalog.Profile;
	    folders: settings.FolderInfo[];
	
	    static createFrom(source: any = {}) {
	        return new BackupPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.app = source["app"];
	        this.createdAt = source["createdAt"];
	        this.intact = source["intact"];
	        this.checked = source["checked"];
	        this.problems = source["problems"];
	        this.sets = this.convertValues(source["sets"], SetPreview);
	        this.apps = this.convertValues(source["apps"], AppPreview);
	        this.missing = source["missing"];
	        this.unknown = source["unknown"];
	        this.profile = this.convertValues(source["profile"], catalog.Profile);
	        this.folders = this.convertValues(source["folders"], settings.FolderInfo);
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
	export class BackupRequest {
	    sets: string[];
	    apps: boolean;
	    folders: string[];
	
	    static createFrom(source: any = {}) {
	        return new BackupRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sets = source["sets"];
	        this.apps = source["apps"];
	        this.folders = source["folders"];
	    }
	}
	export class BackupResult {
	    path: string;
	    files: number;
	    bytes: number;
	    apps: number;
	    folders: number;
	    verified: boolean;
	    problems: number;
	
	    static createFrom(source: any = {}) {
	        return new BackupResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.files = source["files"];
	        this.bytes = source["bytes"];
	        this.apps = source["apps"];
	        this.folders = source["folders"];
	        this.verified = source["verified"];
	        this.problems = source["problems"];
	    }
	}
	export class CompareApp {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new CompareApp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class CompareUnknown {
	    id: string;
	    in: string;
	
	    static createFrom(source: any = {}) {
	        return new CompareUnknown(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.in = source["in"];
	    }
	}
	export class CompareVersion {
	    id: string;
	    name: string;
	    a: string;
	    b: string;
	
	    static createFrom(source: any = {}) {
	        return new CompareVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.a = source["a"];
	        this.b = source["b"];
	    }
	}
	export class CompareSide {
	    kind: string;
	    name: string;
	    file: string;
	    apps: number;
	    versioned: boolean;
	    unknownRecipes: string[];
	    unknownProfiles: string[];
	    profile?: catalog.Profile;
	
	    static createFrom(source: any = {}) {
	        return new CompareSide(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.file = source["file"];
	        this.apps = source["apps"];
	        this.versioned = source["versioned"];
	        this.unknownRecipes = source["unknownRecipes"];
	        this.unknownProfiles = source["unknownProfiles"];
	        this.profile = this.convertValues(source["profile"], catalog.Profile);
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
	export class CompareResult {
	    a: CompareSide;
	    b: CompareSide;
	    onlyA: CompareApp[];
	    onlyB: CompareApp[];
	    both: CompareApp[];
	    differ: CompareVersion[];
	    unknown: CompareUnknown[];
	
	    static createFrom(source: any = {}) {
	        return new CompareResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.a = this.convertValues(source["a"], CompareSide);
	        this.b = this.convertValues(source["b"], CompareSide);
	        this.onlyA = this.convertValues(source["onlyA"], CompareApp);
	        this.onlyB = this.convertValues(source["onlyB"], CompareApp);
	        this.both = this.convertValues(source["both"], CompareApp);
	        this.differ = this.convertValues(source["differ"], CompareVersion);
	        this.unknown = this.convertValues(source["unknown"], CompareUnknown);
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
	
	
	
	export class DriftApp {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new DriftApp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class DriftChange {
	    id: string;
	    name: string;
	    from: string;
	    to: string;
	
	    static createFrom(source: any = {}) {
	        return new DriftChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.from = source["from"];
	        this.to = source["to"];
	    }
	}
	export class DriftInfo {
	    hasBaseline: boolean;
	    at?: string;
	    removed: DriftApp[];
	    added: DriftApp[];
	    updated: DriftChange[];
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new DriftInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hasBaseline = source["hasBaseline"];
	        this.at = source["at"];
	        this.removed = this.convertValues(source["removed"], DriftApp);
	        this.added = this.convertValues(source["added"], DriftApp);
	        this.updated = this.convertValues(source["updated"], DriftChange);
	        this.count = source["count"];
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
	export class FolderInfo {
	    path: string;
	    default: string;
	    custom: boolean;
	    count: number;
	    problem?: string;
	
	    static createFrom(source: any = {}) {
	        return new FolderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.default = source["default"];
	        this.custom = source["custom"];
	        this.count = source["count"];
	        this.problem = source["problem"];
	    }
	}
	export class FolderChange {
	    copied: number;
	    skipped: number;
	    info: FolderInfo;
	
	    static createFrom(source: any = {}) {
	        return new FolderChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.copied = source["copied"];
	        this.skipped = source["skipped"];
	        this.info = this.convertValues(source["info"], FolderInfo);
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
	
	export class GameInstalled {
	    receipt: games.Receipt;
	    registered: string[];
	    registerError?: string;
	
	    static createFrom(source: any = {}) {
	        return new GameInstalled(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.receipt = this.convertValues(source["receipt"], games.Receipt);
	        this.registered = source["registered"];
	        this.registerError = source["registerError"];
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
	export class GamesView {
	    root: string;
	    games: catalog.Game[];
	    installed: Record<string, games.Receipt>;
	    launchers: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new GamesView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.games = this.convertValues(source["games"], catalog.Game);
	        this.installed = this.convertValues(source["installed"], games.Receipt, true);
	        this.launchers = source["launchers"];
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
	    fixes?: string[];
	
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
	        this.fixes = source["fixes"];
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
	export class KitApp {
	    id: string;
	    name: string;
	    version: string;
	    type: string;
	    sizeBytes: number;
	    admin?: boolean;
	    state: string;
	    problems: offlinekit.Problem[];
	    reason?: string;
	    detail?: string;
	    installed?: boolean;
	    installedVersion?: string;
	
	    static createFrom(source: any = {}) {
	        return new KitApp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.type = source["type"];
	        this.sizeBytes = source["sizeBytes"];
	        this.admin = source["admin"];
	        this.state = source["state"];
	        this.problems = this.convertValues(source["problems"], offlinekit.Problem);
	        this.reason = source["reason"];
	        this.detail = source["detail"];
	        this.installed = source["installed"];
	        this.installedVersion = source["installedVersion"];
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
	export class KitCreateResult {
	    added: string[];
	    failed: offlinekit.NotIncluded[];
	    totalApps: number;
	    bytes: number;
	    cancelled: boolean;
	    fingerprint: string;
	    folder: string;
	
	    static createFrom(source: any = {}) {
	        return new KitCreateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.added = source["added"];
	        this.failed = this.convertValues(source["failed"], offlinekit.NotIncluded);
	        this.totalApps = source["totalApps"];
	        this.bytes = source["bytes"];
	        this.cancelled = source["cancelled"];
	        this.fingerprint = source["fingerprint"];
	        this.folder = source["folder"];
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
	export class KitListing {
	    folder: string;
	    fingerprint: string;
	    created: string;
	    updated?: string;
	    winforge: string;
	    profile?: string;
	    apps: KitApp[];
	    notIncluded: offlinekit.NotIncluded[];
	    unlisted: string[];
	    needsAdmin: boolean;
	
	    static createFrom(source: any = {}) {
	        return new KitListing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.fingerprint = source["fingerprint"];
	        this.created = source["created"];
	        this.updated = source["updated"];
	        this.winforge = source["winforge"];
	        this.profile = source["profile"];
	        this.apps = this.convertValues(source["apps"], KitApp);
	        this.notIncluded = this.convertValues(source["notIncluded"], offlinekit.NotIncluded);
	        this.unlisted = source["unlisted"];
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
	export class KitRef {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new KitRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class KitVerified {
	    installed: KitRef[];
	    missing: KitRef[];
	
	    static createFrom(source: any = {}) {
	        return new KitVerified(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = this.convertValues(source["installed"], KitRef);
	        this.missing = this.convertValues(source["missing"], KitRef);
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
	export class LeftoversRecycleResult {
	    moved: number;
	    freedBytes: number;
	    failed: uninstall.Failure[];
	
	    static createFrom(source: any = {}) {
	        return new LeftoversRecycleResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.moved = source["moved"];
	        this.freedBytes = source["freedBytes"];
	        this.failed = this.convertValues(source["failed"], uninstall.Failure);
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
	export class LeftoversResult {
	    appId: string;
	    appName: string;
	    candidates: uninstall.Candidate[];
	    searched: uninstall.Searched[];
	
	    static createFrom(source: any = {}) {
	        return new LeftoversResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.appId = source["appId"];
	        this.appName = source["appName"];
	        this.candidates = this.convertValues(source["candidates"], uninstall.Candidate);
	        this.searched = this.convertValues(source["searched"], uninstall.Searched);
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
	export class PatchResult {
	    path: string;
	    format: string;
	    checked: boolean;
	    headerless: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PatchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.format = source["format"];
	        this.checked = source["checked"];
	        this.headerless = source["headerless"];
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
	    changes: number;
	
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
	        this.changes = source["changes"];
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
	    pending?: install.Pending;
	    startupError?: string;
	
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
	        this.pending = this.convertValues(source["pending"], install.Pending);
	        this.startupError = source["startupError"];
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
	export class UninstallApp {
	    id: string;
	    name: string;
	    version?: string;
	    publisher: string;
	    admin?: boolean;
	    sizeBytes?: number;
	
	    static createFrom(source: any = {}) {
	        return new UninstallApp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.publisher = source["publisher"];
	        this.admin = source["admin"];
	        this.sizeBytes = source["sizeBytes"];
	    }
	}
	export class UninstallRemoved {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new UninstallRemoved(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class UninstallListing {
	    apps: UninstallApp[];
	    removed: UninstallRemoved[];
	
	    static createFrom(source: any = {}) {
	        return new UninstallListing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apps = this.convertValues(source["apps"], UninstallApp);
	        this.removed = this.convertValues(source["removed"], UninstallRemoved);
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
	
	export class UninstallVerified {
	    removed: UninstallRemoved[];
	    still: UninstallRemoved[];
	
	    static createFrom(source: any = {}) {
	        return new UninstallVerified(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.removed = this.convertValues(source["removed"], UninstallRemoved);
	        this.still = this.convertValues(source["still"], UninstallRemoved);
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
	    ignore?: string;
	    ignoreVersion?: string;
	    ignoreUntil?: string;
	
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
	        this.ignore = source["ignore"];
	        this.ignoreVersion = source["ignoreVersion"];
	        this.ignoreUntil = source["ignoreUntil"];
	    }
	}

}

export namespace offlinekit {
	
	export class NotIncluded {
	    id: string;
	    name: string;
	    winget?: string;
	    reason: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new NotIncluded(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.winget = source["winget"];
	        this.reason = source["reason"];
	        this.detail = source["detail"];
	    }
	}
	export class PlanApp {
	    id: string;
	    name: string;
	    winget: string;
	
	    static createFrom(source: any = {}) {
	        return new PlanApp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.winget = source["winget"];
	    }
	}
	export class CreatePlan {
	    folder: string;
	    apps: PlanApp[];
	    notIncluded: NotIncluded[];
	    needBytes: number;
	    freeBytes: number;
	    freeKnown: boolean;
	    enough: boolean;
	    existingKit: boolean;
	    problem?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreatePlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.apps = this.convertValues(source["apps"], PlanApp);
	        this.notIncluded = this.convertValues(source["notIncluded"], NotIncluded);
	        this.needBytes = source["needBytes"];
	        this.freeBytes = source["freeBytes"];
	        this.freeKnown = source["freeKnown"];
	        this.enough = source["enough"];
	        this.existingKit = source["existingKit"];
	        this.problem = source["problem"];
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
	
	
	export class Problem {
	    kind: string;
	    file?: string;
	
	    static createFrom(source: any = {}) {
	        return new Problem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.file = source["file"];
	    }
	}

}

export namespace privacy {
	
	export class Backup {
	    id: string;
	    // Go type: time
	    time: any;
	    action: string;
	    tweaks: string[];
	    // Go type: time
	    restored?: any;
	
	    static createFrom(source: any = {}) {
	        return new Backup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.time = this.convertValues(source["time"], null);
	        this.action = source["action"];
	        this.tweaks = source["tweaks"];
	        this.restored = this.convertValues(source["restored"], null);
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
	export class ChangeView {
	    path: string;
	    name: string;
	    existed: boolean;
	    oldType: string;
	    old: string;
	    type: string;
	    private: number;
	    error: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChangeView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.existed = source["existed"];
	        this.oldType = source["oldType"];
	        this.old = source["old"];
	        this.type = source["type"];
	        this.private = source["private"];
	        this.error = source["error"];
	    }
	}
	export class RestoreResult {
	    restored: string[];
	    changedSince: string[];
	    failed: string[];
	
	    static createFrom(source: any = {}) {
	        return new RestoreResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.restored = source["restored"];
	        this.changedSince = source["changedSince"];
	        this.failed = source["failed"];
	    }
	}
	export class Result {
	    applied: number;
	    skipped: string[];
	    failed: string[];
	    backupId: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.applied = source["applied"];
	        this.skipped = source["skipped"];
	        this.failed = source["failed"];
	        this.backupId = source["backupId"];
	    }
	}
	export class TweakView {
	    id: string;
	    area: string;
	    nameKey: string;
	    whyKey: string;
	    state: string;
	    changes: ChangeView[];
	
	    static createFrom(source: any = {}) {
	        return new TweakView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.area = source["area"];
	        this.nameKey = source["nameKey"];
	        this.whyKey = source["whyKey"];
	        this.state = source["state"];
	        this.changes = this.convertValues(source["changes"], ChangeView);
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

export namespace sched {
	
	export class Item {
	    id: string;
	    name: string;
	    from: string;
	    to: string;
	    result: string;
	    reason?: string;
	    reboot?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.from = source["from"];
	        this.to = source["to"];
	        this.result = source["result"];
	        this.reason = source["reason"];
	        this.reboot = source["reboot"];
	    }
	}
	export class Report {
	    id: string;
	    // Go type: time
	    started: any;
	    // Go type: time
	    finished: any;
	    mode: string;
	    scheduled: boolean;
	    error?: string;
	    items: Item[];
	    notified?: boolean;
	    fingerprint?: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.started = this.convertValues(source["started"], null);
	        this.finished = this.convertValues(source["finished"], null);
	        this.mode = source["mode"];
	        this.scheduled = source["scheduled"];
	        this.error = source["error"];
	        this.items = this.convertValues(source["items"], Item);
	        this.notified = source["notified"];
	        this.fingerprint = source["fingerprint"];
	        this.status = source["status"];
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

export namespace services {
	
	export class Item {
	    id: string;
	    name: string;
	    displayName: string;
	    publisher: string;
	    state: string;
	    startMode: string;
	    binary: string;
	    needsAdmin: boolean;
	    caution: boolean;
	    dependents: string[];
	    changed: boolean;
	    original: string;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.displayName = source["displayName"];
	        this.publisher = source["publisher"];
	        this.state = source["state"];
	        this.startMode = source["startMode"];
	        this.binary = source["binary"];
	        this.needsAdmin = source["needsAdmin"];
	        this.caution = source["caution"];
	        this.dependents = source["dependents"];
	        this.changed = source["changed"];
	        this.original = source["original"];
	    }
	}

}

export namespace settings {
	
	export class FolderInfo {
	    name: string;
	    dir: string;
	    files: number;
	    bytes: number;
	    skipped: number;
	
	    static createFrom(source: any = {}) {
	        return new FolderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.dir = source["dir"];
	        this.files = source["files"];
	        this.bytes = source["bytes"];
	        this.skipped = source["skipped"];
	    }
	}
	export class FolderResult {
	    written: number;
	    same: number;
	    kept: number;
	    skipped: string[];
	    dest: string;
	
	    static createFrom(source: any = {}) {
	        return new FolderResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.written = source["written"];
	        this.same = source["same"];
	        this.kept = source["kept"];
	        this.skipped = source["skipped"];
	        this.dest = source["dest"];
	    }
	}
	export class Found {
	    id: string;
	    files: number;
	    bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new Found(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.files = source["files"];
	        this.bytes = source["bytes"];
	    }
	}
	export class Result {
	    restored: number;
	    unchanged: number;
	    backedUp: number;
	    extensions: number;
	    skipped: string[];
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.restored = source["restored"];
	        this.unchanged = source["unchanged"];
	        this.backedUp = source["backedUp"];
	        this.extensions = source["extensions"];
	        this.skipped = source["skipped"];
	    }
	}

}

export namespace startup {
	
	export class Item {
	    id: string;
	    name: string;
	    source: string;
	    command: string;
	    target: string;
	    publisher: string;
	    enabled: boolean;
	    needsAdmin: boolean;
	    missing: boolean;
	    caution: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.source = source["source"];
	        this.command = source["command"];
	        this.target = source["target"];
	        this.publisher = source["publisher"];
	        this.enabled = source["enabled"];
	        this.needsAdmin = source["needsAdmin"];
	        this.missing = source["missing"];
	        this.caution = source["caution"];
	    }
	}

}

export namespace storage {
	
	export class BigFile {
	    path: string;
	    name: string;
	    sizeBytes: number;
	    // Go type: time
	    modified: any;
	
	    static createFrom(source: any = {}) {
	        return new BigFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.sizeBytes = source["sizeBytes"];
	        this.modified = this.convertValues(source["modified"], null);
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
	export class Category {
	    id: string;
	    risk: string;
	    needsAdmin: boolean;
	    available: boolean;
	    bytes: number;
	    files: number;
	    skipped: number;
	
	    static createFrom(source: any = {}) {
	        return new Category(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.risk = source["risk"];
	        this.needsAdmin = source["needsAdmin"];
	        this.available = source["available"];
	        this.bytes = source["bytes"];
	        this.files = source["files"];
	        this.skipped = source["skipped"];
	    }
	}
	export class Defaults {
	    home: string;
	    downloads: string;
	    documents: string;
	    pictures: string;
	    videos: string;
	
	    static createFrom(source: any = {}) {
	        return new Defaults(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.home = source["home"];
	        this.downloads = source["downloads"];
	        this.documents = source["documents"];
	        this.pictures = source["pictures"];
	        this.videos = source["videos"];
	    }
	}
	export class DupFile {
	    path: string;
	    // Go type: time
	    modified: any;
	
	    static createFrom(source: any = {}) {
	        return new DupFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.modified = this.convertValues(source["modified"], null);
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
	export class DupGroup {
	    id: string;
	    sizeBytes: number;
	    files: DupFile[];
	
	    static createFrom(source: any = {}) {
	        return new DupGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sizeBytes = source["sizeBytes"];
	        this.files = this.convertValues(source["files"], DupFile);
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
	export class FolderSize {
	    path: string;
	    name: string;
	    sizeBytes: number;
	    files: number;
	
	    static createFrom(source: any = {}) {
	        return new FolderSize(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.sizeBytes = source["sizeBytes"];
	        this.files = source["files"];
	    }
	}
	export class Outcome {
	    id: string;
	    freedBytes: number;
	    files: number;
	    failed: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Outcome(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.freedBytes = source["freedBytes"];
	        this.files = source["files"];
	        this.failed = source["failed"];
	        this.error = source["error"];
	    }
	}
	export class ProjectJunk {
	    path: string;
	    project: string;
	    projectPath: string;
	    kind: string;
	    sizeBytes: number;
	    files: number;
	    // Go type: time
	    lastTouched: any;
	
	    static createFrom(source: any = {}) {
	        return new ProjectJunk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.project = source["project"];
	        this.projectPath = source["projectPath"];
	        this.kind = source["kind"];
	        this.sizeBytes = source["sizeBytes"];
	        this.files = source["files"];
	        this.lastTouched = this.convertValues(source["lastTouched"], null);
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
	export class ProjectResult {
	    freedBytes: number;
	    removed: number;
	    files: number;
	    failed: string[];
	    reasons: string[];
	
	    static createFrom(source: any = {}) {
	        return new ProjectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.freedBytes = source["freedBytes"];
	        this.removed = source["removed"];
	        this.files = source["files"];
	        this.failed = source["failed"];
	        this.reasons = source["reasons"];
	    }
	}
	export class RecycleResult {
	    moved: number;
	    freedBytes: number;
	    failed: string[];
	
	    static createFrom(source: any = {}) {
	        return new RecycleResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.moved = source["moved"];
	        this.freedBytes = source["freedBytes"];
	        this.failed = source["failed"];
	    }
	}
	export class Result {
	    freedBytes: number;
	    files: number;
	    failed: number;
	    outcomes: Outcome[];
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.freedBytes = source["freedBytes"];
	        this.files = source["files"];
	        this.failed = source["failed"];
	        this.outcomes = this.convertValues(source["outcomes"], Outcome);
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
	export class VirtualDisk {
	    path: string;
	    name: string;
	    kind: string;
	    sizeBytes: number;
	    // Go type: time
	    modified: any;
	
	    static createFrom(source: any = {}) {
	        return new VirtualDisk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.sizeBytes = source["sizeBytes"];
	        this.modified = this.convertValues(source["modified"], null);
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
	    label: string;
	    totalBytes: number;
	    freeBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new Volume(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.letter = source["letter"];
	        this.label = source["label"];
	        this.totalBytes = source["totalBytes"];
	        this.freeBytes = source["freeBytes"];
	    }
	}

}

export namespace tasks {
	
	export class Item {
	    id: string;
	    name: string;
	    folder: string;
	    triggers: string[];
	    action: string;
	    state: string;
	    enabled: boolean;
	    lastResult?: number;
	    needsAdmin: boolean;
	    caution: boolean;
	    changed: boolean;
	    original: string;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.folder = source["folder"];
	        this.triggers = source["triggers"];
	        this.action = source["action"];
	        this.state = source["state"];
	        this.enabled = source["enabled"];
	        this.lastResult = source["lastResult"];
	        this.needsAdmin = source["needsAdmin"];
	        this.caution = source["caution"];
	        this.changed = source["changed"];
	        this.original = source["original"];
	    }
	}

}

export namespace uninstall {
	
	export class Why {
	    rule: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new Why(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rule = source["rule"];
	        this.value = source["value"];
	    }
	}
	export class Candidate {
	    path: string;
	    kind: string;
	    place: string;
	    sizeBytes: number;
	    files: number;
	    // Go type: time
	    modified: any;
	    why: Why;
	
	    static createFrom(source: any = {}) {
	        return new Candidate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.kind = source["kind"];
	        this.place = source["place"];
	        this.sizeBytes = source["sizeBytes"];
	        this.files = source["files"];
	        this.modified = this.convertValues(source["modified"], null);
	        this.why = this.convertValues(source["why"], Why);
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
	export class Failure {
	    path: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new Failure(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.reason = source["reason"];
	    }
	}
	export class Searched {
	    place: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new Searched(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.place = source["place"];
	        this.path = source["path"];
	    }
	}

}

export namespace winfeatures {
	
	export class Distro {
	    name: string;
	    state: string;
	    version: number;
	    isDefault: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Distro(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.state = source["state"];
	        this.version = source["version"];
	        this.isDefault = source["isDefault"];
	    }
	}
	export class Feature {
	    id: string;
	    state: string;
	    restartPending: boolean;
	    enable: string;
	    disable: string;
	    needsAdmin: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Feature(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.state = source["state"];
	        this.restartPending = source["restartPending"];
	        this.enable = source["enable"];
	        this.disable = source["disable"];
	        this.needsAdmin = source["needsAdmin"];
	    }
	}
	export class WSLInfo {
	    installed: boolean;
	    defaultVersion: number;
	    distros: Distro[];
	
	    static createFrom(source: any = {}) {
	        return new WSLInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.defaultVersion = source["defaultVersion"];
	        this.distros = this.convertValues(source["distros"], Distro);
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

