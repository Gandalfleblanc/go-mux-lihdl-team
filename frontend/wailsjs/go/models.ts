export namespace config {
	
	export class Config {
	    tmdb_key: string;
	    serveurperso_url: string;
	    output_dir: string;
	    mkvmerge_path: string;
	    default_encoder: string;
	    default_team: string;
	    default_quality: string;
	    default_source: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tmdb_key = source["tmdb_key"];
	        this.serveurperso_url = source["serveurperso_url"];
	        this.output_dir = source["output_dir"];
	        this.mkvmerge_path = source["mkvmerge_path"];
	        this.default_encoder = source["default_encoder"];
	        this.default_team = source["default_team"];
	        this.default_quality = source["default_quality"];
	        this.default_source = source["default_source"];
	    }
	}

}

export namespace main {
	
	export class LihdlOptions {
	    audio_labels: string[];
	    subtitle_labels: string[];
	    video_qualities: string[];
	    video_encoders: string[];
	    video_sources: string[];
	    video_teams: string[];
	
	    static createFrom(source: any = {}) {
	        return new LihdlOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.audio_labels = source["audio_labels"];
	        this.subtitle_labels = source["subtitle_labels"];
	        this.video_qualities = source["video_qualities"];
	        this.video_encoders = source["video_encoders"];
	        this.video_sources = source["video_sources"];
	        this.video_teams = source["video_teams"];
	    }
	}
	export class MuxRequest {
	    input_path: string;
	    output_path: string;
	    title: string;
	    tracks: mkvtool.TrackSpec[];
	    external_audios: mkvtool.ExternalAudio[];
	    external_subs: mkvtool.ExternalSub[];
	
	    static createFrom(source: any = {}) {
	        return new MuxRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.input_path = source["input_path"];
	        this.output_path = source["output_path"];
	        this.title = source["title"];
	        this.tracks = this.convertValues(source["tracks"], mkvtool.TrackSpec);
	        this.external_audios = this.convertValues(source["external_audios"], mkvtool.ExternalAudio);
	        this.external_subs = this.convertValues(source["external_subs"], mkvtool.ExternalSub);
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
	export class TmdbTestResult {
	    ok: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new TmdbTestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	    }
	}
	export class UpdateInfo {
	    available: boolean;
	    version: string;
	    url: string;
	    notes: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.version = source["version"];
	        this.url = source["url"];
	        this.notes = source["notes"];
	    }
	}

}

export namespace mkvtool {
	
	export class ExternalAudio {
	    Path: string;
	    Name: string;
	    Language: string;
	    Default: boolean;
	    Forced: boolean;
	    Order: number;
	
	    static createFrom(source: any = {}) {
	        return new ExternalAudio(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.Name = source["Name"];
	        this.Language = source["Language"];
	        this.Default = source["Default"];
	        this.Forced = source["Forced"];
	        this.Order = source["Order"];
	    }
	}
	export class ExternalSub {
	    Path: string;
	    Name: string;
	    Language: string;
	    Default: boolean;
	    Forced: boolean;
	    Order: number;
	
	    static createFrom(source: any = {}) {
	        return new ExternalSub(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.Name = source["Name"];
	        this.Language = source["Language"];
	        this.Default = source["Default"];
	        this.Forced = source["Forced"];
	        this.Order = source["Order"];
	    }
	}
	export class TrackSpec {
	    ID: number;
	    Type: string;
	    Keep: boolean;
	    Name: string;
	    Language: string;
	    Default: boolean;
	    Forced: boolean;
	    Order: number;
	
	    static createFrom(source: any = {}) {
	        return new TrackSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Type = source["Type"];
	        this.Keep = source["Keep"];
	        this.Name = source["Name"];
	        this.Language = source["Language"];
	        this.Default = source["Default"];
	        this.Forced = source["Forced"];
	        this.Order = source["Order"];
	    }
	}

}

export namespace naming {
	
	export class FilenameParams {
	    Title: string;
	    Year: string;
	    AudioLabels: string[];
	    Resolution: string;
	    Source: string;
	    Format: string;
	    AudioCodecs: string[];
	    VideoCodec: string;
	    Team: string;
	    CustomSource: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FilenameParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Title = source["Title"];
	        this.Year = source["Year"];
	        this.AudioLabels = source["AudioLabels"];
	        this.Resolution = source["Resolution"];
	        this.Source = source["Source"];
	        this.Format = source["Format"];
	        this.AudioCodecs = source["AudioCodecs"];
	        this.VideoCodec = source["VideoCodec"];
	        this.Team = source["Team"];
	        this.CustomSource = source["CustomSource"];
	    }
	}

}

export namespace tmdb {
	
	export class Result {
	    tmdb_id: string;
	    note: number;
	    titre_fr: string;
	    annee_fr: string;
	    titre_vo: string;
	    duree: string;
	    url: string;
	    poster_url: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tmdb_id = source["tmdb_id"];
	        this.note = source["note"];
	        this.titre_fr = source["titre_fr"];
	        this.annee_fr = source["annee_fr"];
	        this.titre_vo = source["titre_vo"];
	        this.duree = source["duree"];
	        this.url = source["url"];
	        this.poster_url = source["poster_url"];
	    }
	}

}

