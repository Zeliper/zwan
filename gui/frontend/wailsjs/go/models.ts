export namespace engine {
	
	export class Status {
	    connected: boolean;
	    networkId: string;
	    dnsSuffix: string;
	    overlayCidr: string;
	    assignedIp: string;
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
	        this.networkId = source["networkId"];
	        this.dnsSuffix = source["dnsSuffix"];
	        this.overlayCidr = source["overlayCidr"];
	        this.assignedIp = source["assignedIp"];
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
	    networkId: string;
	    dnsSuffix: string;
	    cidr: string;
	    token: string;
	    controlAddr: string;
	    relayAddr: string;
	    peers: proto.Peer[];
	    services: proto.Service[];
	
	    static createFrom(source: any = {}) {
	        return new HostState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.networkId = source["networkId"];
	        this.dnsSuffix = source["dnsSuffix"];
	        this.cidr = source["cidr"];
	        this.token = source["token"];
	        this.controlAddr = source["controlAddr"];
	        this.relayAddr = source["relayAddr"];
	        this.peers = this.convertValues(source["peers"], proto.Peer);
	        this.services = this.convertValues(source["services"], proto.Service);
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

export namespace proto {
	
	export class Peer {
	    hostname: string;
	    public_key: string;
	    assigned_ip: string;
	    endpoint?: string;
	
	    static createFrom(source: any = {}) {
	        return new Peer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostname = source["hostname"];
	        this.public_key = source["public_key"];
	        this.assigned_ip = source["assigned_ip"];
	        this.endpoint = source["endpoint"];
	    }
	}
	export class Service {
	    name: string;
	    proto: string;
	    port: number;
	    backend_port?: number;
	    node_ip: string;
	
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
	    }
	}

}

