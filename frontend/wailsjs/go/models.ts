export namespace audiosync {
	
	export class DetectionResult {
	    offset_ms: number;
	    confidence: number;
	    drift_ms: number;
	    method: string;
	    tempo_factor: number;
	    notes: string;
	
	    static createFrom(source: any = {}) {
	        return new DetectionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offset_ms = source["offset_ms"];
	        this.confidence = source["confidence"];
	        this.drift_ms = source["drift_ms"];
	        this.method = source["method"];
	        this.tempo_factor = source["tempo_factor"];
	        this.notes = source["notes"];
	    }
	}

}

export namespace config {
	
	export class Config {
	    tmdb_key: string;
	    serveurperso_url: string;
	    fallback_index: string;
	    hydracker_key: string;
	    unfr_key: string;
	    output_dir: string;
	    output_dir_lihdl: string;
	    output_dir_psa: string;
	    mkvmerge_path: string;
	    default_encoder: string;
	    default_team: string;
	    default_quality: string;
	    default_source: string;
	    languagetool_key: string;
	    languagetool_user: string;
	    languagetool_url: string;
	    opensubtitles_api_key: string;
	    discord_bot_token: string;
	    discord_forum_id: string;
	    discord_index_url: string;
	    github_token: string;
	    github_repo: string;
	    github_branch: string;
	    github_index_file_path: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tmdb_key = source["tmdb_key"];
	        this.serveurperso_url = source["serveurperso_url"];
	        this.fallback_index = source["fallback_index"];
	        this.hydracker_key = source["hydracker_key"];
	        this.unfr_key = source["unfr_key"];
	        this.output_dir = source["output_dir"];
	        this.output_dir_lihdl = source["output_dir_lihdl"];
	        this.output_dir_psa = source["output_dir_psa"];
	        this.mkvmerge_path = source["mkvmerge_path"];
	        this.default_encoder = source["default_encoder"];
	        this.default_team = source["default_team"];
	        this.default_quality = source["default_quality"];
	        this.default_source = source["default_source"];
	        this.languagetool_key = source["languagetool_key"];
	        this.languagetool_user = source["languagetool_user"];
	        this.languagetool_url = source["languagetool_url"];
	        this.opensubtitles_api_key = source["opensubtitles_api_key"];
	        this.discord_bot_token = source["discord_bot_token"];
	        this.discord_forum_id = source["discord_forum_id"];
	        this.discord_index_url = source["discord_index_url"];
	        this.github_token = source["github_token"];
	        this.github_repo = source["github_repo"];
	        this.github_branch = source["github_branch"];
	        this.github_index_file_path = source["github_index_file_path"];
	    }
	}

}

export namespace main {
	
	export class ApiKeyTestResult {
	    ok: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ApiKeyTestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	    }
	}
	export class AudioSyncOffset {
	    track_id: number;
	    delay_ms: number;
	    tempo_factor: number;
	
	    static createFrom(source: any = {}) {
	        return new AudioSyncOffset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.track_id = source["track_id"];
	        this.delay_ms = source["delay_ms"];
	        this.tempo_factor = source["tempo_factor"];
	    }
	}
	export class AudioSyncRequest {
	    input_path: string;
	    output_path: string;
	    offsets: AudioSyncOffset[];
	
	    static createFrom(source: any = {}) {
	        return new AudioSyncRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.input_path = source["input_path"];
	        this.output_path = source["output_path"];
	        this.offsets = this.convertValues(source["offsets"], AudioSyncOffset);
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
	export class FRAudioExtraction {
	    path: string;
	    variant: string;
	    codec: string;
	    codec_id: string;
	    channels: number;
	    track_name: string;
	    language: string;
	    delay_ms: number;
	    tempo_factor: number;
	    confidence: number;
	    method: string;
	    notes: string;
	    was_converted: boolean;
	    bitrate_kbps: number;
	    mi_title: string;
	    mi_format: string;
	    mi_format_profile: string;
	    mi_format_commercial: string;
	    mi_format_commercial_if_any: string;
	    mi_format_features: string;
	    mi_channels: string;
	    mi_service_kind: string;
	    mi_service_kind_name: string;
	
	    static createFrom(source: any = {}) {
	        return new FRAudioExtraction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.variant = source["variant"];
	        this.codec = source["codec"];
	        this.codec_id = source["codec_id"];
	        this.channels = source["channels"];
	        this.track_name = source["track_name"];
	        this.language = source["language"];
	        this.delay_ms = source["delay_ms"];
	        this.tempo_factor = source["tempo_factor"];
	        this.confidence = source["confidence"];
	        this.method = source["method"];
	        this.notes = source["notes"];
	        this.was_converted = source["was_converted"];
	        this.bitrate_kbps = source["bitrate_kbps"];
	        this.mi_title = source["mi_title"];
	        this.mi_format = source["mi_format"];
	        this.mi_format_profile = source["mi_format_profile"];
	        this.mi_format_commercial = source["mi_format_commercial"];
	        this.mi_format_commercial_if_any = source["mi_format_commercial_if_any"];
	        this.mi_format_features = source["mi_format_features"];
	        this.mi_channels = source["mi_channels"];
	        this.mi_service_kind = source["mi_service_kind"];
	        this.mi_service_kind_name = source["mi_service_kind_name"];
	    }
	}
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
	export class MkvBasicInfo {
	    duration_seconds: number;
	    framerate: number;
	    width: number;
	    height: number;
	    has_vfq_audio: boolean;
	    vfq_track_info: string;
	
	    static createFrom(source: any = {}) {
	        return new MkvBasicInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.duration_seconds = source["duration_seconds"];
	        this.framerate = source["framerate"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.has_vfq_audio = source["has_vfq_audio"];
	        this.vfq_track_info = source["vfq_track_info"];
	    }
	}
	export class MuxRequest {
	    input_path: string;
	    output_path: string;
	    title: string;
	    tracks: mkvtool.TrackSpec[];
	    external_audios: mkvtool.ExternalAudio[];
	    external_subs: mkvtool.ExternalSub[];
	    secondary_path: string;
	    secondary_audios: mkvtool.SecondaryTrack[];
	    secondary_subs: mkvtool.SecondaryTrack[];
	    no_chapters: boolean;
	
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
	        this.secondary_path = source["secondary_path"];
	        this.secondary_audios = this.convertValues(source["secondary_audios"], mkvtool.SecondaryTrack);
	        this.secondary_subs = this.convertValues(source["secondary_subs"], mkvtool.SecondaryTrack);
	        this.no_chapters = source["no_chapters"];
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
	export class PSASyncResult {
	    offset_ms: number;
	    confidence: number;
	    method: string;
	    fps_ref_mkv: number;
	    fps_cand_mkv: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new PSASyncResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offset_ms = source["offset_ms"];
	        this.confidence = source["confidence"];
	        this.method = source["method"];
	        this.fps_ref_mkv = source["fps_ref_mkv"];
	        this.fps_cand_mkv = source["fps_cand_mkv"];
	        this.error = source["error"];
	    }
	}
	export class RefSubResult {
	    path: string;
	    language: string;
	    forced: boolean;
	    sdh: boolean;
	    label: string;
	    delay_ms: number;
	    tempo_factor: number;
	    confidence: number;
	    method: string;
	
	    static createFrom(source: any = {}) {
	        return new RefSubResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.language = source["language"];
	        this.forced = source["forced"];
	        this.sdh = source["sdh"];
	        this.label = source["label"];
	        this.delay_ms = source["delay_ms"];
	        this.tempo_factor = source["tempo_factor"];
	        this.confidence = source["confidence"];
	        this.method = source["method"];
	    }
	}
	export class SubSyncCheck {
	    path: string;
	    synced_path: string;
	    offset_ms: number;
	    fps_ratio: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new SubSyncCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.synced_path = source["synced_path"];
	        this.offset_ms = source["offset_ms"];
	        this.fps_ratio = source["fps_ratio"];
	        this.error = source["error"];
	    }
	}
	export class SubSyncRequest {
	    path: string;
	    from_reference: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SubSyncRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.from_reference = source["from_reference"];
	    }
	}
	export class SyncAudioTrack {
	    id: number;
	    codec: string;
	    language: string;
	    name: string;
	    channels: number;
	
	    static createFrom(source: any = {}) {
	        return new SyncAudioTrack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.codec = source["codec"];
	        this.language = source["language"];
	        this.name = source["name"];
	        this.channels = source["channels"];
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
	    VisualImpaired: boolean;
	    DelayMs: number;
	    TempoFactor: number;
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
	        this.VisualImpaired = source["VisualImpaired"];
	        this.DelayMs = source["DelayMs"];
	        this.TempoFactor = source["TempoFactor"];
	        this.Order = source["Order"];
	    }
	}
	export class ExternalSub {
	    Path: string;
	    Name: string;
	    Language: string;
	    Default: boolean;
	    Forced: boolean;
	    DelayMs: number;
	    TempoFactor: number;
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
	        this.DelayMs = source["DelayMs"];
	        this.TempoFactor = source["TempoFactor"];
	        this.Order = source["Order"];
	    }
	}
	export class SecondaryTrack {
	    ID: number;
	    Name: string;
	    Language: string;
	    Default: boolean;
	    Forced: boolean;
	    VisualImpaired: boolean;
	    DelayMs: number;
	    Order: number;
	
	    static createFrom(source: any = {}) {
	        return new SecondaryTrack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Language = source["Language"];
	        this.Default = source["Default"];
	        this.Forced = source["Forced"];
	        this.VisualImpaired = source["VisualImpaired"];
	        this.DelayMs = source["DelayMs"];
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
	    VisualImpaired: boolean;
	    DelayMs: number;
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
	        this.VisualImpaired = source["VisualImpaired"];
	        this.DelayMs = source["DelayMs"];
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

export namespace ocrsubs {
	
	export class CustomDictEntry {
	    wrong: string;
	    right: string;
	    added_at: string;
	    auto: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CustomDictEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.wrong = source["wrong"];
	        this.right = source["right"];
	        this.added_at = source["added_at"];
	        this.auto = source["auto"];
	    }
	}

}

export namespace opensubtitles {
	
	export class OSSearchResult {
	    id: string;
	    subtitle_id: string;
	    title: string;
	    year: number;
	    language: string;
	    download_count: number;
	    rating: number;
	    filename: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new OSSearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.subtitle_id = source["subtitle_id"];
	        this.title = source["title"];
	        this.year = source["year"];
	        this.language = source["language"];
	        this.download_count = source["download_count"];
	        this.rating = source["rating"];
	        this.filename = source["filename"];
	        this.url = source["url"];
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
	    overview: string;
	    original_language: string;
	
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
	        this.overview = source["overview"];
	        this.original_language = source["original_language"];
	    }
	}

}

