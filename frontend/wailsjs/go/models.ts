export namespace main {
	
	export class Bookmark {
	    id: string;
	    page: number;
	    label: string;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Bookmark(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.page = source["page"];
	        this.label = source["label"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class DocInfo {
	    id: string;
	    name: string;
	    path: string;
	    pageCount: number;
	    lastPage: number;
	    offset: number;
	    zoom: number;
	    furthestPage: number;
	    updatedAt: number;
	    exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DocInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.pageCount = source["pageCount"];
	        this.lastPage = source["lastPage"];
	        this.offset = source["offset"];
	        this.zoom = source["zoom"];
	        this.furthestPage = source["furthestPage"];
	        this.updatedAt = source["updatedAt"];
	        this.exists = source["exists"];
	    }
	}
	export class Point {
	    x: number;
	    y: number;
	
	    static createFrom(source: any = {}) {
	        return new Point(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.y = source["y"];
	    }
	}
	export class ReadingState {
	    docId: string;
	    page: number;
	    offset: number;
	    zoom: number;
	    furthestPage: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new ReadingState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.docId = source["docId"];
	        this.page = source["page"];
	        this.offset = source["offset"];
	        this.zoom = source["zoom"];
	        this.furthestPage = source["furthestPage"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class Stroke {
	    id: string;
	    page: number;
	    tool: string;
	    color: string;
	    width: number;
	    points: Point[];
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Stroke(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.page = source["page"];
	        this.tool = source["tool"];
	        this.color = source["color"];
	        this.width = source["width"];
	        this.points = this.convertValues(source["points"], Point);
	        this.createdAt = source["createdAt"];
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
	export class TextNote {
	    id: string;
	    page: number;
	    x: number;
	    y: number;
	    width: number;
	    size: number;
	    color: string;
	    text: string;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new TextNote(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.page = source["page"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.width = source["width"];
	        this.size = source["size"];
	        this.color = source["color"];
	        this.text = source["text"];
	        this.createdAt = source["createdAt"];
	    }
	}

}

