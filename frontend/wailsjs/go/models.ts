export namespace lid {
	
	export class ClipResult {
	    folder: string;
	    file: string;
	    path: string;
	    duration_s?: number;
	    speech_s?: number;
	    language: string;
	    confidence?: number;
	    second_language: string;
	    second_confidence?: number;
	    status: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ClipResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.file = source["file"];
	        this.path = source["path"];
	        this.duration_s = source["duration_s"];
	        this.speech_s = source["speech_s"];
	        this.language = source["language"];
	        this.confidence = source["confidence"];
	        this.second_language = source["second_language"];
	        this.second_confidence = source["second_confidence"];
	        this.status = source["status"];
	        this.error = source["error"];
	    }
	}
	export class ExportRequest {
	    format: string;
	    paths: string[];
	    fields: string[];
	    bom: boolean;
	    out: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = source["format"];
	        this.paths = source["paths"];
	        this.fields = source["fields"];
	        this.bom = source["bom"];
	        this.out = source["out"];
	    }
	}
	export class FolderProgress {
	    path: string;
	    name: string;
	    done: number;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new FolderProgress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.done = source["done"];
	        this.total = source["total"];
	    }
	}
	export class FolderSummary {
	    path: string;
	    name: string;
	    clips: number;
	    skipped: Record<string, number>;
	    total_duration_s?: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new FolderSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.clips = source["clips"];
	        this.skipped = source["skipped"];
	        this.total_duration_s = source["total_duration_s"];
	        this.error = source["error"];
	    }
	}
	export class Summary {
	    total: number;
	    ok: number;
	    review: number;
	    no_speech: number;
	    error: number;
	
	    static createFrom(source: any = {}) {
	        return new Summary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.ok = source["ok"];
	        this.review = source["review"];
	        this.no_speech = source["no_speech"];
	        this.error = source["error"];
	    }
	}
	export class Progress {
	    state: string;
	    done: number;
	    total: number;
	    elapsed_s: number;
	    eta_s?: number;
	    current: string;
	    folders: FolderProgress[];
	    recent: ClipResult[];
	    summary: Summary;
	
	    static createFrom(source: any = {}) {
	        return new Progress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.done = source["done"];
	        this.total = source["total"];
	        this.elapsed_s = source["elapsed_s"];
	        this.eta_s = source["eta_s"];
	        this.current = source["current"];
	        this.folders = this.convertValues(source["folders"], FolderProgress);
	        this.recent = this.convertValues(source["recent"], ClipResult);
	        this.summary = this.convertValues(source["summary"], Summary);
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
	export class Settings {
	    extensions: string[];
	    recursive: boolean;
	    min_confidence: number;
	    workers: number;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.extensions = source["extensions"];
	        this.recursive = source["recursive"];
	        this.min_confidence = source["min_confidence"];
	        this.workers = source["workers"];
	    }
	}

}

