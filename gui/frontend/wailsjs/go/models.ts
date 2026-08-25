export namespace acl {
	
	export class Rule {
	    src: string[];
	    dst: string[];
	
	    static createFrom(source: any = {}) {
	        return new Rule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.src = source["src"];
	        this.dst = source["dst"];
	    }
	}

}

export namespace config {
	
	export class Config {
	    networkId: string;
	    dnsSuffix: string;
	    cidr: string;
	    token: string;
	    controlAddr: string;
	    relayAddr: string;
	    relayPublic: string;
	    tlsMode: string;
	    domains: string[];
	    publicHost: string;
	    groupTokens?: Record<string, string>;
	    acl?: acl.Rule[];
	    autoStart: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.networkId = source["networkId"];
	        this.dnsSuffix = source["dnsSuffix"];
	        this.cidr = source["cidr"];
	        this.token = source["token"];
	        this.controlAddr = source["controlAddr"];
	        this.relayAddr = source["relayAddr"];
	        this.relayPublic = source["relayPublic"];
	        this.tlsMode = source["tlsMode"];
	        this.domains = source["domains"];
	        this.publicHost = source["publicHost"];
	        this.groupTokens = source["groupTokens"];
	        this.acl = this.convertValues(source["acl"], acl.Rule);
	        this.autoStart = source["autoStart"];
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

export namespace engine {
	
	export class Status {
	    connected: boolean;
	    server: string;
	    pinned: boolean;
	    networkId: string;
	    dnsSuffix: string;
	    overlayCidr: string;
	    assignedIp: string;
	    overlayIp: string;
	    localCidr: string;
	    relayAddr: string;
	    publicKey: string;
	    via: string;
	    peers: proto.Peer[];
	    services: proto.Service[];
	    handshakes: Record<string, boolean>;
	    lastError: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.server = source["server"];
	        this.pinned = source["pinned"];
	        this.networkId = source["networkId"];
	        this.dnsSuffix = source["dnsSuffix"];
	        this.overlayCidr = source["overlayCidr"];
	        this.assignedIp = source["assignedIp"];
	        this.overlayIp = source["overlayIp"];
	        this.localCidr = source["localCidr"];
	        this.relayAddr = source["relayAddr"];
	        this.publicKey = source["publicKey"];
	        this.via = source["via"];
	        this.peers = this.convertValues(source["peers"], proto.Peer);
	        this.services = this.convertValues(source["services"], proto.Service);
	        this.handshakes = source["handshakes"];
	        this.lastError = source["lastError"];
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
	
	export class HostState {
	    running: boolean;
	    config: config.Config;
	    tlsMode: string;
	    pin: string;
	    joinUrl: string;
	    peers: proto.Peer[];
	    services: proto.Service[];
	    lastError: string;
	    managedByService: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HostState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.config = this.convertValues(source["config"], config.Config);
	        this.tlsMode = source["tlsMode"];
	        this.pin = source["pin"];
	        this.joinUrl = source["joinUrl"];
	        this.peers = this.convertValues(source["peers"], proto.Peer);
	        this.services = this.convertValues(source["services"], proto.Service);
	        this.lastError = source["lastError"];
	        this.managedByService = source["managedByService"];
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

export namespace manager {
	
	export class Status {
	    network: profile.Network;
	    engine: engine.Status;
	    warning?: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.network = this.convertValues(source["network"], profile.Network);
	        this.engine = this.convertValues(source["engine"], engine.Status);
	        this.warning = source["warning"];
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

export namespace profile {
	
	export class Network {
	    alias: string;
	    server: string;
	    pin?: string;
	    token: string;
	    name?: string;
	    useRelay: boolean;
	    autoConnect: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Network(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.alias = source["alias"];
	        this.server = source["server"];
	        this.pin = source["pin"];
	        this.token = source["token"];
	        this.name = source["name"];
	        this.useRelay = source["useRelay"];
	        this.autoConnect = source["autoConnect"];
	    }
	}

}

export namespace proto {
	
	export class Peer {
	    hostname: string;
	    public_key: string;
	    assigned_ip: string;
	    endpoint?: string;
	    group?: string;
	
	    static createFrom(source: any = {}) {
	        return new Peer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostname = source["hostname"];
	        this.public_key = source["public_key"];
	        this.assigned_ip = source["assigned_ip"];
	        this.endpoint = source["endpoint"];
	        this.group = source["group"];
	    }
	}
	export class Service {
	    name: string;
	    proto: string;
	    port: number;
	    backend_port?: number;
	    node_ip: string;
	    allow_groups?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Service(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.proto = source["proto"];
	        this.port = source["port"];
	        this.backend_port = source["backend_port"];
	        this.node_ip = source["node_ip"];
	        this.allow_groups = source["allow_groups"];
	    }
	}

}

export namespace update {
	
	export class Release {
	    tag: string;
	    version: string;
	    installerUrl: string;
	    notes: string;
	
	    static createFrom(source: any = {}) {
	        return new Release(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tag = source["tag"];
	        this.version = source["version"];
	        this.installerUrl = source["installerUrl"];
	        this.notes = source["notes"];
	    }
	}

}

