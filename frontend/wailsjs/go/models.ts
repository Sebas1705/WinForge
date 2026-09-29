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

}

