export namespace main {
	
	export class ServiceInfo {
	    name: string;
	    proto: string;
	    port: number;
	    backendPort: number;
	    nodeIp: string;
	    fqdn: string;
	
	    static createFrom(source: any = {}) {
	        return new ServiceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.proto = source["proto"];
	        this.port = source["port"];
	        this.backendPort = source["backendPort"];
	        this.nodeIp = source["nodeIp"];
	        this.fqdn = source["fqdn"];
	    }
	}
	export class PeerInfo {
	    hostname: string;
	    publicKey: string;
	    assignedIp: string;
	    endpoint: string;
	
	    static createFrom(source: any = {}) {
	        return new PeerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostname = source["hostname"];
	        this.publicKey = source["publicKey"];
	        this.assignedIp = source["assignedIp"];
	        this.endpoint = source["endpoint"];
	    }
	}
	export class JoinResult {
	    networkId: string;
	    dnsSuffix: string;
	    overlayCidr: string;
	    assignedIp: string;
	    relayAddr: string;
	    publicKey: string;
	    deviceUuid: string;
	    peers: PeerInfo[];
	    services: ServiceInfo[];
	
	    static createFrom(source: any = {}) {
	        return new JoinResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.networkId = source["networkId"];
	        this.dnsSuffix = source["dnsSuffix"];
	        this.overlayCidr = source["overlayCidr"];
	        this.assignedIp = source["assignedIp"];
	        this.relayAddr = source["relayAddr"];
	        this.publicKey = source["publicKey"];
	        this.deviceUuid = source["deviceUuid"];
	        this.peers = this.convertValues(source["peers"], PeerInfo);
	        this.services = this.convertValues(source["services"], ServiceInfo);
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

