<script>
  import { onMount } from 'svelte';
  import banner from './assets/images/banner.png';
  import logo from './assets/images/logo.png';
  import {
    GetVersion, GetConfig, SaveConfig, GetLihdlOptions,
    SelectMkvFile, SelectSubFiles, SelectAudioFiles, SelectOutputDir, LocateMkvmerge, OpenFolder, SearchTmdbTV,
    AnalyzeMkv, SearchTmdb, TestTmdbKey, FileSize,
    Mux, CancelMux,
    CheckUpdate, InstallUpdate,
  } from '../wailsjs/go/main/App.js';
  import { EventsOn } from '../wailsjs/runtime/runtime.js';

  let screen = 'source';
  let appVersion = '';
  let mkvmergePath = '';

  let config = {
    tmdb_key: '',
    serveurperso_url: 'https://www.serveurperso.com/stats/search.php',
    output_dir: '',
    mkvmerge_path: '',
    default_encoder: 'GANDALF',
    default_team: 'LiHDL',
    default_quality: 'HDLight',
    default_source: 'REMUX LiHDL',
  };

  let options = {
    audio_labels: [], subtitle_labels: [],
    video_qualities: [], video_encoders: [],
    video_sources: [], video_teams: [],
  };

  let sourcePath = '';
  let sourceInfo = null;   // réponse AnalyzeMkv
  let tracks = [];         // état UI enrichi de chaque piste

  let videoChoice = {
    quality: 'HDLight',
    encoder: 'GANDALF',
    sourceType: 'COMPLETE BluRay', // ex-source split
    sourceTeam: '',                 // team de la source (ex: Alkaline) — éditable
    team: 'LiHDL',                  // team de sortie (dans le filename)
  };
  let target = { title: '', year: '', resolution: '1080p', source: 'HDLight', video_codec: '', lang: 'auto' };
  // Dernière fiche TMDB sélectionnée — permet de basculer VF/VO sans re-chercher.
  let lastTmdbResult = null;

  // Dropdowns pour le filename (ordre : résolution.source).
  const RESOLUTION_OPTIONS = ['720p', '1080p', '2160p'];
  const TARGET_SOURCE_OPTIONS = ['HDLight', 'WEBLight', 'WEB-DL'];
  // Dropdown élargi pour le type de source de la piste vidéo.
  const VIDEO_SOURCE_TYPE_OPTIONS = [
    'REMUX', 'REMUX CUSTOM',
    'WEB-DL CUSTOM', 'WEB CUSTOM',
    'WEB', 'WEB-DL', 'WEBRip',
    'WEBRip PSA Audio SUPPLY', 'WEBRIP PSA Audio FW',
    'COMPLETE BluRay',
  ];
  const VIDEO_CODEC_OPTIONS = ['H264', 'x264', 'H265', 'x265', 'AV1'];
  let tmdbResults = [];
  let tmdbQuery = '';
  let tmdbMode = 'movie'; // 'movie' | 'tv'
  let filenameCopied = false;
  let filenameCopiedTimer = null;

  // Queue batch : liste de .mkv en attente de mux.
  let queue = [];

  function queueAdd(paths) {
    if (!paths || !paths.length) return;
    const existing = new Set(queue);
    for (const p of paths) {
      if (p && !existing.has(p) && p !== sourcePath) {
        queue = [...queue, p];
        existing.add(p);
      }
    }
  }

  function queueRemove(idx) {
    queue = queue.filter((_, i) => i !== idx);
  }

  function queueLoad(idx) {
    const p = queue[idx];
    if (!p) return;
    queue = queue.filter((_, i) => i !== idx);
    openMkv(p);
  }

  function queueNext() {
    if (queue.length === 0) return;
    const p = queue[0];
    queue = queue.slice(1);
    openMkv(p);
  }

  async function copyFilename() {
    if (!previewFilename) return;
    try {
      await navigator.clipboard.writeText(previewFilename);
      filenameCopied = true;
      if (filenameCopiedTimer) clearTimeout(filenameCopiedTimer);
      filenameCopiedTimer = setTimeout(() => { filenameCopied = false; }, 1500);
    } catch (e) {
      appendLog('⚠ Copie : ' + String(e));
    }
  }
  let tmdbSearching = false;

  let muxing = false;
  let muxPercent = 0;
  let logLines = [];
  let logEl;

  // Subs externes ajoutés (srt/sup/ass/ssa/sub/idx). Chaque entrée a le même
  // format qu'une piste interne pour l'UI unifiée, mais avec un flag external.
  let externalSubs = [];
  let externalAudios = [];

  // --- helpers ---
  function now() {
    return new Date().toTimeString().slice(0, 8);
  }
  function classify(msg) {
    const l = msg.toLowerCase();
    if (msg.includes('❌') || l.includes('erreur') || l.includes('error')) return 'error';
    if (msg.includes('✅') || msg.includes('✓') || msg.includes('→')) return 'ok';
    if (msg.includes('⏳') || msg.includes('⚠') || msg.includes('ℹ') || msg.includes('🔎') || msg.includes('🔧')) return 'progress';
    return 'default';
  }
  function appendLog(msg) {
    for (const line of String(msg).split('\n')) {
      if (line.trim() === '') continue;
      logLines = [...logLines, { time: now(), text: line, level: classify(line) }];
    }
    setTimeout(() => { if (logEl) logEl.scrollTop = logEl.scrollHeight; }, 0);
  }

  function mapLangCode(lbl) {
    // Règle LiHDL : VFQ = québécois → "fr-ca" (IETF BCP 47).
    // Les autres variantes FR restent en "fre" (ISO 639-2).
    if (/\bVFQ\b/.test(lbl)) return 'fr-ca';
    if (lbl.startsWith('FR')) return 'fre';
    if (lbl.startsWith('ENG')) return 'eng';
    if (lbl.startsWith('JPN')) return 'jpn';
    if (lbl.startsWith('ITA')) return 'ita';
    return '';
  }

  // Alias maintenu pour compat : renvoie le codec saisi par l'utilisateur
  // (éditable dans Cible) ou la suggestion auto si vide.
  function videoCodecLihdl() {
    return target.video_codec || suggestedVideoCodec();
  }

  function resolutionFromTracks() {
    const v = tracks.find(t => t.type === 'video');
    if (!v) return '';
    const dim = v.pixelDims || '';
    const m = /(\d+)x(\d+)/.exec(dim);
    if (!m) return '';
    const h = parseInt(m[2], 10);
    if (h >= 2100) return '2160p';
    if (h >= 1070) return '1080p';
    if (h >= 700)  return '720p';
    if (h >= 560)  return '576p';
    return h + 'p';
  }

  // Codec+canaux de la 1re piste audio FR VFx gardée (VFF/VFQ/VFi).
  // Fallback : 1re piste audio gardée tout court. Ex : "AC3.5.1".
  function firstAudioCodecForFilename() {
    const kept = tracks.filter(t => t.type === 'audio' && t.keep);
    const firstFR = kept.find(t => /\bVF[FQi]\b/.test(t.label || ''));
    const chosen = firstFR || kept[0];
    if (!chosen) return '';
    const m = /: (AC3|EAC3) (\d\.\d)/.exec(chosen.label || '');
    return m ? `${m[1]}.${m[2]}` : '';
  }

  // Vrai si au moins une piste audio gardée porte un label "FR AD".
  function hasAudioDescription() {
    return tracks.some(t => t.type === 'audio' && t.keep && /\bFR AD\b/.test(t.label || ''));
  }

  function keptAudioLabels() {
    return tracks.filter(t => t.type === 'audio' && t.keep).map(t => t.label);
  }

  // --- Calcul client-side du flag langue et du filename (évite les Promises
  //     de Wails en rendu synchrone qui cassaient les tabs). ---

  function langFlagClient(labels) {
    let hasVFF = false, hasVFQ = false, hasVFi = false, hasVO = false;
    for (const l of labels) {
      if (!l) continue;
      if (/\bVFF\b/.test(l)) hasVFF = true;
      else if (/\bVFQ\b/.test(l)) hasVFQ = true;
      else if (/\bVFi\b/.test(l)) hasVFi = true;
      else if (/\bVO\b/.test(l)) hasVO = true;
    }
    const vfCount = (hasVFF?1:0) + (hasVFQ?1:0) + (hasVFi?1:0);
    if (vfCount >= 2) return 'MULTi.VF2';
    if (hasVFF && hasVO) return 'MULTi.VFF';
    if (hasVFQ && hasVO) return 'MULTi.VFQ';
    if (hasVFi && hasVO) return 'MULTi.VFi';
    if (hasVFF) return 'VFF';
    if (hasVFQ) return 'VFQ';
    if (hasVFi) return 'VFi';
    if (hasVO)  return 'VO';
    return 'VO';
  }

  // Remplace espaces ET tirets par des points (pas de - dans le titre).
  // Puis compresse les points multiples.
  function dotify(s) {
    return String(s || '').trim()
      .replace(/[\s-]+/g, '.')
      .replace(/\.+/g, '.');
  }

  // Version FR uniquement si FRENCH / TRUEFRENCH / VOF apparaissent.
  // MULTi → le film d'origine est étranger → titre VO (anglais).
  // VFF / VFQ seuls sont des métadonnées de pistes, pas de la version du film.
  function looksFrench(path) {
    const n = String(path || '').toUpperCase();
    return /\b(FRENCH|TRUEFRENCH|VOF)\b/.test(n);
  }

  // Retire un suffixe " (YYYY)" du titre pour générer le filename proprement.
  function stripYearSuffix(title) {
    return String(title || '').replace(/\s*\(\d{4}\)\s*$/, '').trim();
  }

  // Auto-détection de la team de la source depuis le nom de fichier.
  // Pattern : ...-TeamName.mkv → "TeamName"
  function extractSourceTeam(path) {
    const name = basename(path || '').replace(/\.[^.]+$/, '');
    const m = /-([A-Za-z0-9]+)\s*$/.exec(name);
    return m ? m[1] : '';
  }

  // Suggestion auto du codec cible (ex : H264 si détecté AVC + source WEB-DL,
  // sinon x264 ; pareil pour H265/x265). L'utilisateur peut override.
  function suggestedVideoCodec() {
    const v = tracks.find(t => t.type === 'video');
    if (!v) return '';
    const c = ((v.codec || '') + ' ' + (v.codecId || '')).toUpperCase();
    if (c.includes('AV1') || c.includes('AV01')) return 'AV1';
    let base = '';
    if (c.includes('HEVC') || c.includes('H.265') || c.includes('H265') || c.includes('MPEGH')) base = '265';
    else if (c.includes('AVC') || c.includes('H.264') || c.includes('H264') || c.includes('MPEG4')) base = '264';
    if (!base) return '';
    // Règle LiHDL : H264/H265 toujours. Les x264/x265 sont pour d'autres teams.
    const prefix = videoChoice.team === 'LiHDL' ? 'H' : (target.source === 'WEB-DL' ? 'H' : 'x');
    return prefix + base;
  }

  // Construit le nom de fichier selon les normes LiHDL.
  // Format : {Title}.{Year}.{Flag}.{SourceQuality}.{Audio}[.WiTH.AD].{VideoCodec}-{Team}.mkv
  // Exemple : The.Target.2014.VFF.HDLight.1080p.AC3.5.1.WiTH.AD.H264-LiHDL.mkv
  // Choisit le titre (FR ou VO) à utiliser pour le nom de fichier selon target.lang.
  // Auto = FR si le nom de source ressemble à FRENCH/VOF, sinon VO (étranger).
  // Si pas de TMDB, fallback sur target.title (saisi manuellement).
  function filenameTitleForBuild() {
    if (!lastTmdbResult) return stripYearSuffix(target.title);
    let useFR;
    if (target.lang === 'vf')      useFR = true;
    else if (target.lang === 'vo') useFR = false;
    else                           useFR = looksFrench(sourcePath);
    return useFR
      ? (lastTmdbResult.titre_fr || lastTmdbResult.titre_vo || '')
      : (lastTmdbResult.titre_vo || lastTmdbResult.titre_fr || '');
  }

  function buildFilenameClient() {
    if (!sourceInfo) return '';
    const parts = [];
    // Titre du fichier = FR ou VO selon target.lang (≠ titre cible qui est toujours FR).
    const cleanTitle = stripYearSuffix(filenameTitleForBuild());
    if (cleanTitle) parts.push(dotify(cleanTitle));
    // Année : soit depuis le champ, soit extraite du titre si absent.
    const yearMatch = String(target.title || '').match(/\((\d{4})\)\s*$/);
    const year = target.year || (yearMatch ? yearMatch[1] : '');
    if (year) parts.push(year);
    parts.push(langFlagClient(keptAudioLabels()));
    if (target.resolution) parts.push(target.resolution);
    if (target.source) parts.push(target.source);
    const ac = firstAudioCodecForFilename();
    if (ac) parts.push(ac);
    if (hasAudioDescription()) { parts.push('WiTH'); parts.push('AD'); }
    const vc = videoCodecLihdl();
    if (vc) parts.push(vc);
    let name = parts.filter(Boolean).join('.');
    if (videoChoice.team) name += '-' + videoChoice.team;
    return name + '.mkv';
  }

  function videoTrackNameClient() {
    // Format LiHDL : "HDLight By GANDALF (Source COMPLETE BluRay Alkaline)"
    const src = videoChoice.sourceTeam
      ? `${videoChoice.sourceType} ${videoChoice.sourceTeam}`
      : videoChoice.sourceType;
    return `${videoChoice.quality} By ${videoChoice.encoder} (Source ${src})`;
  }

  // Réactivité : on référence chaque dépendance pour que Svelte détecte
  // les changements et recalcule (sinon il n'analyse pas l'intérieur des fns).
  $: previewFilename = (function() {
    const _deps = [tracks.length, videoChoice.team, target.title, target.year,
                   target.resolution, target.source, target.video_codec, target.lang,
                   ...tracks.map(t => (t.keep ? '1' : '0') + (t.label || ''))];
    void _deps;
    return buildFilenameClient();
  })();
  $: previewVideoName = (function() {
    const _deps = [videoChoice.quality, videoChoice.encoder,
                   videoChoice.sourceType, videoChoice.sourceTeam];
    void _deps;
    return videoTrackNameClient();
  })();
  // Auto-fill du codec cible quand aucun n'est explicitement choisi.
  $: if (!target.video_codec) target.video_codec = suggestedVideoCodec();

  $: suggestedCodecDisplay = (function() {
    const _deps = [target.source, tracks.length,
                   ...tracks.filter(t => t.type === 'video').map(t => t.codec + (t.codecId || ''))];
    void _deps;
    return suggestedVideoCodec();
  })();

  // --- actions ---
  function openMkv(path) {
    sourcePath = path;
    sourceInfo = null;
    tracks = [];
    // Auto-fill la team de la source depuis le nom de fichier.
    const st = extractSourceTeam(path);
    if (st) videoChoice.sourceTeam = st;
    target.video_codec = ''; // reset pour que l'auto-suggest se réapplique
    AnalyzeMkv(path); // fire-and-forget, résultat via event 'analyze:result'
  }

  function finalizeAnalyze(rawTracks) {
    appendLog('🎯 finalizeAnalyze appelé avec ' + rawTracks.length + ' pistes');
    sourceInfo = { tracks: rawTracks };
    externalSubs = []; // reset les subs externes quand on recharge un mkv
    tracks = rawTracks.map((t, i) => {
      // Les pistes viennent avec les champs aplatis (pas de .properties imbriqué).
      const base = {
        id: t.id,
        type: t.type,
        codec: t.codec,
        lang: t.language || '',
        channels: t.audio_channels || 0,
        codecId: t.codec_id || '',
        pixelDims: t.pixel_dimensions || '',
        keep: true,
        default: !!t.default_track,
        forced: !!t.forced_track,
        name: t.track_name || '',
        order: i * 10, // pas de 10 pour laisser de la place aux externes
      };
      if (t.type === 'audio')     base.label = suggestAudioLabelFlat(base);
      if (t.type === 'subtitles') base.label = suggestSubLabelFlat(base);
      if (t.type === 'video')     base.label = '';
      return base;
    });
    if (sourcePath) maybeAutoFillTitle(sourcePath);
  }

  // Auto-suggest d'un label de sous-titre à partir du nom de fichier.
  // Patterns courants : film.fr.srt, film.fr.forced.srt, film.en.sdh.srt,
  //                     film.vff.srt, film.fr-fr.srt, film.fra.srt, etc.
  function suggestSubLabelFromFilename(filename) {
    const n = filename.toLowerCase();
    const isPGS   = /\.(sup)$/.test(n);
    const isASS   = /\.(ass|ssa)$/.test(n);
    const isIDX   = /\.(idx|sub)$/.test(n);
    const fmt = isPGS ? 'PGS' : (isASS ? 'ASS' : (isIDX ? 'VobSub' : 'SRT'));
    const forced = /[._-](forced|forc[eé]s?)[._-]/.test(n) || /\bforced\b/.test(n);
    const sdh    = /[._-](sdh|cc)[._-]/.test(n) || /\bsdh\b/.test(n);
    const isFR   = /[._-](fr|fre|fra|fran[cç]ais|french)[._-]/.test(n) || /\.(fr|fre|fra)\./.test(n);
    const isVFF  = /\bvff\b/.test(n);
    const isVFQ  = /\bvfq\b/.test(n) || /qu[ée]bec/.test(n);
    const isEN   = /[._-](en|eng|english)[._-]/.test(n) || /\.(en|eng)\./.test(n);

    if (fmt === 'ASS' || fmt === 'VobSub') {
      // Pas dans la liste LiHDL — user devra choisir "— autre — ".
      return '';
    }

    let prefix = '';
    if (isFR) {
      if (isVFF) prefix = 'FR VFF';
      else if (isVFQ) prefix = 'FR VFQ';
      else prefix = 'FR';
    } else if (isEN) {
      prefix = 'ENG';
    }
    if (!prefix) return '';

    const kind = forced ? 'Forced' : (sdh ? 'SDH' : 'Full');
    const candidate = `${prefix} ${kind} : ${fmt}`;
    return options.subtitle_labels.includes(candidate) ? candidate : '';
  }

  function basename(path) {
    return String(path || '').split('/').pop().split('\\').pop();
  }

  function formatBytes(n) {
    if (n == null || n < 0) return '';
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
    return (n / (1024 * 1024)).toFixed(1) + ' MB';
  }

  // Heuristique forced vs full selon la taille du fichier.
  // SRT petit < 5KB = typiquement forced. PGS petit < 300KB = forced.
  function isLikelyForcedBySize(name, size) {
    if (size == null || size < 0) return false;
    const ext = (name.match(/\.([a-z0-9]+)$/i) || [,''])[1].toLowerCase();
    if (ext === 'srt' || ext === 'ass' || ext === 'ssa') return size < 5 * 1024;
    if (ext === 'sup') return size < 300 * 1024;
    return false;
  }

  async function addExternalSubs(paths) {
    let maxOrder = 0;
    for (const t of tracks) maxOrder = Math.max(maxOrder, t.order ?? 0);
    for (const s of externalSubs) maxOrder = Math.max(maxOrder, s.order ?? 0);

    for (const p of paths) {
      const name = basename(p);
      maxOrder += 10;
      let size = -1;
      try { size = await FileSize(p); } catch {}
      const forcedByName = /forced/i.test(name);
      const forcedBySize = isLikelyForcedBySize(name, size);
      const forced = forcedByName || forcedBySize;
      // Si auto-détecté forced mais le label suggéré dit "Full", corrige en "Forced".
      let label = suggestSubLabelFromFilename(name);
      if (forced && label && label.includes(' Full ')) {
        label = label.replace(' Full ', ' Forced ');
      }
      externalSubs = [...externalSubs, {
        path: p, name, size,
        keep: true, default: false,
        forced, label,
        order: maxOrder,
      }];
    }
    if (paths.length > 0) {
      appendLog('✓ ' + paths.length + ' sous-titre(s) ajouté(s)');
    }
  }

  async function pickSubsDialog() {
    try {
      const paths = await SelectSubFiles();
      if (paths && paths.length > 0) addExternalSubs(paths);
    } catch (e) {
      appendLog('❌ ' + String(e));
    }
  }

  function removeExternalSub(idx) {
    externalSubs = externalSubs.filter((_, i) => i !== idx);
  }

  async function addExternalAudios(paths) {
    let maxOrder = 0;
    for (const t of tracks) maxOrder = Math.max(maxOrder, t.order ?? 0);
    for (const a of externalAudios) maxOrder = Math.max(maxOrder, a.order ?? 0);
    for (const p of paths) {
      const name = basename(p);
      maxOrder += 10;
      let size = -1;
      try { size = await FileSize(p); } catch {}
      externalAudios = [...externalAudios, {
        path: p, name, size,
        keep: true, default: false, forced: false,
        label: '', order: maxOrder,
      }];
    }
    if (paths.length) appendLog('✓ ' + paths.length + ' audio(s) ajouté(s)');
  }

  async function pickAudioDialog() {
    try {
      const paths = await SelectAudioFiles();
      if (paths && paths.length > 0) addExternalAudios(paths);
    } catch (e) {
      appendLog('❌ ' + String(e));
    }
  }

  function removeExternalAudio(idx) {
    externalAudios = externalAudios.filter((_, i) => i !== idx);
  }

  function removeInternalTrack(id) {
    tracks = tracks.filter(t => t.id !== id);
  }

  function moveAudioTrack(kind, idx, dir) {
    const all = [
      ...tracks.filter(t => t.type === 'audio').map((t, i) => ({ kind: 'internal', idx: i, ref: t })),
      ...externalAudios.map((a, i) => ({ kind: 'external', idx: i, ref: a })),
    ].sort((a, b) => (a.ref.order ?? 0) - (b.ref.order ?? 0));
    const pos = all.findIndex(x => x.kind === kind && x.idx === idx);
    const newPos = pos + dir;
    if (newPos < 0 || newPos >= all.length) return;
    const cur = all[pos].ref, oth = all[newPos].ref;
    const tmp = cur.order ?? 0;
    cur.order = oth.order ?? 0;
    oth.order = tmp;
    tracks = [...tracks];
    externalAudios = [...externalAudios];
  }

  // --- Ordre global des pistes (internes + externes) ---
  // Chaque piste a un champ `order` (initialisé à son index d'arrivée).
  // L'UI affiche les sous-titres triés par order avec des flèches ↑↓.

  // Fait monter/descendre une piste audio (idx dans le tableau trié par order).
  function moveAudio(idx, dir) {
    const audio = tracks.filter(t => t.type === 'audio').slice().sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
    const newIdx = idx + dir;
    if (newIdx < 0 || newIdx >= audio.length) return;
    const cur = audio[idx], oth = audio[newIdx];
    const tmp = cur.order ?? 0;
    cur.order = oth.order ?? 0;
    oth.order = tmp;
    tracks = [...tracks];
  }

  function moveTrack(kind, idx, dir) {
    // kind = 'internal' | 'external' ; dir = -1 (up) / +1 (down)
    const subs = [...tracks.filter(t => t.type === 'subtitles'), ...externalSubs];
    subs.sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
    // Trouve la piste concernée dans le tableau trié.
    let target;
    if (kind === 'internal') {
      target = tracks.filter(t => t.type === 'subtitles')[idx];
    } else {
      target = externalSubs[idx];
    }
    const pos = subs.indexOf(target);
    const newPos = pos + dir;
    if (newPos < 0 || newPos >= subs.length) return;
    // Swap orders.
    const other = subs[newPos];
    const tmp = target.order ?? 0;
    target.order = other.order ?? 0;
    other.order = tmp;
    // Force reactivity.
    tracks = [...tracks];
    externalSubs = [...externalSubs];
  }

  // Versions des suggest adaptées à la structure aplatie.
  function suggestAudioLabelFlat(t) {
    const lang = (t.lang || '').toLowerCase();
    const codec = ((t.codec || '') + ' ' + (t.codecId || '')).toUpperCase();
    const ch = t.channels || 2;
    const ac3 = codec.includes('AC-3') && !codec.includes('E-AC');
    const eac3 = codec.includes('E-AC-3') || codec.includes('EAC3');
    const c = eac3 ? 'EAC3' : (ac3 ? 'AC3' : '');
    const chStr = ch >= 6 ? '5.1' : (ch >= 2 ? '2.0' : '1.0');
    if (!c) return '';
    let prefix = '';
    if (lang === 'fre' || lang === 'fra' || lang === 'fr') prefix = 'FR VFF';
    else if (lang === 'eng' || lang === 'en') prefix = 'ENG VO';
    else if (lang === 'jpn' || lang === 'ja') prefix = 'JPN VO';
    else if (lang === 'ita' || lang === 'it') prefix = 'ITA VO';
    if (!prefix) return '';
    const candidate = `${prefix} : ${c} ${chStr}`;
    return options.audio_labels.includes(candidate) ? candidate : '';
  }
  function suggestSubLabelFlat(t) {
    const lang = (t.lang || '').toLowerCase();
    const codec = ((t.codec || '') + ' ' + (t.codecId || '')).toUpperCase();
    const isPGS = codec.includes('PGS') || codec.includes('HDMV');
    const fmt = isPGS ? 'PGS' : 'SRT';
    const forced = !!t.forced;
    if (lang === 'fre' || lang === 'fra' || lang === 'fr') {
      const kind = forced ? 'Forced' : 'Full';
      return `FR ${kind} : ${fmt}`;
    }
    if (lang === 'eng' || lang === 'en') {
      const kind = forced ? 'Forced' : 'Full';
      return `ENG ${kind} : ${fmt}`;
    }
    return '';
  }

  // Construit le titre "Titre FR (Année)" — TOUJOURS le titre français TMDB
  // avec l'année entre parenthèses (norme LiHDL). Fallback VO si FR vide.
  function composeTmdbTitle(r) {
    const base = r.titre_fr || r.titre_vo || '';
    const year = r.annee_fr || '';
    return year ? (base + ' (' + year + ')') : base;
  }

  // Rejoue le calcul du titre quand on bascule VF/VO dans le dropdown.
  // Reassign complet pour forcer la réactivité Svelte (sinon la preview
  // filename ne se mettait pas à jour).
  function refreshTitleFromLang() {
    if (!lastTmdbResult) return;
    const newTitle = composeTmdbTitle(lastTmdbResult);
    const newYear = lastTmdbResult.annee_fr || '';
    target = { ...target, title: newTitle, year: newYear };
    appendLog('✓ Titre ' + (target.lang || 'auto').toUpperCase() + ' : ' + newTitle);
  }

  async function maybeAutoFillTitle(path) {
    const name = path.split('/').pop().replace(/\.[^.]+$/, '');
    tmdbQuery = name;
    try {
      tmdbSearching = true;
      const r = await SearchTmdb(name);
      tmdbResults = r || [];
      if (r && r.length === 1) {
        lastTmdbResult = r[0];
        target.title = composeTmdbTitle(r[0]);
        target.year  = r[0].annee_fr || '';
        appendLog('✓ TMDB : ' + target.title);
      } else if (r && r.length > 1) {
        appendLog('ℹ ' + r.length + ' résultats TMDB — choisis dans Cible');
      }
    } catch (e) {
      appendLog('⚠ TMDB : ' + String(e));
    } finally {
      tmdbSearching = false;
    }
  }

  async function searchTmdb() {
    if (!tmdbQuery.trim()) return;
    tmdbSearching = true;
    tmdbResults = [];
    try {
      const r = tmdbMode === 'tv'
        ? await SearchTmdbTV(tmdbQuery)
        : await SearchTmdb(tmdbQuery);
      tmdbResults = r || [];
    } catch (e) {
      appendLog('❌ TMDB : ' + String(e));
    } finally {
      tmdbSearching = false;
    }
  }

  function pickTmdb(r) {
    lastTmdbResult = r;
    target.title = composeTmdbTitle(r);
    target.year  = r.annee_fr || '';
    tmdbResults = [];
    screen = 'cible';
  }

  async function pickMkvDialog() {
    const p = await SelectMkvFile();
    if (p) openMkv(p);
  }

  async function pickOutputDir() {
    const d = await SelectOutputDir();
    if (d) config.output_dir = d;
  }

  async function openOutputDir() {
    if (!config.output_dir) { appendLog('⚠ Dossier de sortie non défini'); return; }
    try {
      await OpenFolder(config.output_dir);
    } catch (e) {
      appendLog('❌ Ouvrir dossier : ' + String(e));
    }
  }

  async function saveSettings() {
    await SaveConfig(config);
    appendLog('✓ Réglages enregistrés');
    mkvmergePath = await LocateMkvmerge();
  }

  let tmdbTest = { running: false, ok: null, message: '' };

  // Auto-update state.
  let updateInfo = null;       // {version, url, notes} si dispo
  let checkingUpdate = false;
  let installingUpdate = false;

  async function checkForUpdate() {
    if (checkingUpdate) return;
    checkingUpdate = true;
    updateInfo = null;
    appendLog('🔍 Recherche de mise à jour…');
    try {
      const info = await CheckUpdate();
      if (info && info.available) {
        updateInfo = info;
        appendLog('🎉 Mise à jour disponible : ' + info.version);
      } else {
        appendLog('✓ Déjà à jour (v' + appVersion + ')');
      }
    } catch (e) {
      appendLog('❌ Check MAJ : ' + String(e));
    } finally {
      checkingUpdate = false;
    }
  }

  async function doInstallUpdate() {
    if (installingUpdate) return;
    installingUpdate = true;
    try {
      await InstallUpdate();
      // L'app va quitter et relancer la nouvelle version.
    } catch (e) {
      appendLog('❌ Install MAJ : ' + String(e));
      installingUpdate = false;
    }
  }
  async function doTestTmdbKey() {
    tmdbTest = { running: true, ok: null, message: '' };
    try {
      const r = await TestTmdbKey(config.tmdb_key || '');
      tmdbTest = { running: false, ok: !!r.ok, message: r.message || '' };
    } catch (e) {
      tmdbTest = { running: false, ok: false, message: String(e) };
    }
  }

  async function doMux() {
    if (!sourcePath)        { appendLog('⚠ Aucun .mkv source'); return; }
    if (!config.output_dir) { appendLog('⚠ Dossier de sortie non défini — ouvre Réglages'); return; }
    if (!previewFilename)   { appendLog('⚠ Nom de fichier incomplet'); return; }

    // Construit le nom de piste vidéo LiHDL et rattache aux tracks.
    const videoName = videoTrackNameClient();
    const specs = tracks.map(t => ({
      ID: t.id,
      Type: t.type,
      Keep: t.keep,
      Name: t.type === 'video' ? videoName : (t.label || ''),
      Language: t.type === 'video' ? '' : mapLangCode(t.label || ''),
      Default: !!t.default,
      Forced: !!t.forced,
      Order: t.order ?? 0,
    }));
    const extSubs = externalSubs.map(s => ({
      Path: s.path,
      Name: s.label || '',
      Language: mapLangCode(s.label || ''),
      Default: !!s.default,
      Forced: !!s.forced,
      Order: s.order ?? 0,
    }));
    const extAudios = externalAudios.map(a => ({
      Path: a.path,
      Name: a.label || '',
      Language: mapLangCode(a.label || ''),
      Default: !!a.default,
      Forced: !!a.forced,
      Order: a.order ?? 0,
    }));

    const outputPath = config.output_dir.replace(/\/$/, '') + '/' + previewFilename;

    muxing = true;
    muxPercent = 0;
    try {
      await Mux({
        input_path: sourcePath,
        output_path: outputPath,
        title: target.title || '',
        tracks: specs,
        external_audios: extAudios,
        external_subs: extSubs,
      });
    } catch (e) {
      appendLog('❌ ' + String(e));
    } finally {
      muxing = false;
    }
  }

  function stopMux() { CancelMux(); }

  onMount(async () => {
    try { appVersion = await GetVersion(); } catch {}
    try { config = await GetConfig(); } catch {}
    try { options = await GetLihdlOptions(); } catch {}
    try { mkvmergePath = await LocateMkvmerge(); } catch {}

    EventsOn('log', (msg) => appendLog(msg));
    EventsOn('mux:progress', (p) => { muxPercent = p.Percent || p.percent || 0; });
    EventsOn('mux:done', () => {
      muxing = false;
      muxPercent = 0;
      if (queue.length > 0) {
        appendLog('✓ Mux terminé — ' + queue.length + ' en file. Clique "Charger le suivant" pour continuer.');
      }
    });
    EventsOn('file:dropped', (path) => { openMkv(path); });
    EventsOn('subs:dropped', (paths) => { addExternalSubs(paths || []); });
    EventsOn('audios:dropped', (paths) => { addExternalAudios(paths || []); });
    EventsOn('files:dropped', (paths) => {
      if (!paths || !paths.length) return;
      queueAdd(paths);
      appendLog('✓ ' + paths.length + ' .mkv ajoutés à la file');
    });
    // Pattern chunked : analyze:start (n attendu) + analyze:track (x N).
    // On auto-finalise dès qu'on atteint n (l'event analyze:end de Wails
    // ne fire pas côté JS pour une raison obscure).
    let pendingTracks = [];
    let pendingExpected = 0;
    EventsOn('analyze:start', (n) => {
      pendingExpected = Number(n) || 0;
      pendingTracks = [];
    });
    EventsOn('analyze:track', (raw) => {
      try { pendingTracks.push(JSON.parse(raw)); }
      catch (e) { appendLog('❌ parse track : ' + String(e)); }
      if (pendingExpected > 0 && pendingTracks.length >= pendingExpected) {
        finalizeAnalyze(pendingTracks);
        pendingTracks = [];
        pendingExpected = 0;
      }
    });

    if (!mkvmergePath) {
      appendLog('⚠ mkvmerge introuvable — installe MKVToolNix ou configure le chemin dans Réglages');
    } else {
      appendLog('ℹ mkvmerge : ' + mkvmergePath);
    }

    // Check silencieux au démarrage : si MAJ dispo, le bouton devient vert.
    try {
      const info = await CheckUpdate();
      if (info && info.available) {
        updateInfo = info;
        appendLog('🎉 Mise à jour disponible : ' + info.version);
      }
    } catch {}
  });
</script>

<main>
  <header class="topbar">
    <div class="brand">
      <img class="logo" src={logo} alt="LiHDL" />
      <div class="brand-text">
        <div class="app-title">GO MUX <span class="brand-lihdl">LiHDL</span> TEAM</div>
        <div class="app-subtitle">BY GANDALF</div>
      </div>
    </div>
    <div class="topbar-right">
      {#if updateInfo}
        <button class="update-pill available" on:click={doInstallUpdate} disabled={installingUpdate}
                title="Installer {updateInfo.version}">
          {installingUpdate ? '⟳ Installation…' : '⬇ Installer ' + updateInfo.version}
        </button>
      {:else}
        <button class="update-pill" on:click={checkForUpdate} disabled={checkingUpdate}
                title="Rechercher une mise à jour">
          <span class="version-icon" class:spin={checkingUpdate}>⟳</span>
          <span class="version-label">{appVersion}</span>
        </button>
      {/if}
      <button class="settings-btn" on:click={() => screen = 'reglages'}>
        <span class="gear">⚙</span>
        <span class="settings-label">SETTINGS</span>
      </button>
    </div>
  </header>

  <nav class="tabs">
    <button class:active={screen === 'source'}   on:click={() => screen = 'source'}>Source</button>
    <button class:active={screen === 'cible'}    on:click={() => screen = 'cible'}>Cible</button>
    <button class:active={screen === 'reglages'} on:click={() => screen = 'reglages'}>Réglages</button>
  </nav>

  <section class="content">
    {#if screen === 'source'}
      <!-- Drop zone / pick file -->
      <div class="card drop-target" style:--wails-drop-target="drop">
        <div class="drop-icon">🎬</div>
        {#if !sourcePath}
          <div class="drop-title">Glisse un fichier .mkv ici</div>
          <div class="drop-sub">ou plusieurs pour batcher · puis audios/subs externes</div>
          <button class="btn-primary" on:click={pickMkvDialog}>Choisir un fichier</button>
        {:else}
          <div class="drop-title">{sourcePath.split('/').pop()}</div>
          <div class="drop-sub">{sourcePath}</div>
          <button class="btn-ghost" on:click={pickMkvDialog}>Changer</button>
        {/if}
      </div>

      <!-- Queue batch -->
      {#if queue.length > 0}
        <div class="card">
          <div class="section-title-row">
            <div class="section-title">File d'attente ({queue.length})</div>
            <div class="section-actions">
              <button class="btn-ghost" on:click={queueNext} disabled={muxing}>Charger le suivant</button>
              <button class="btn-ghost" on:click={() => queue = []}>Vider</button>
            </div>
          </div>
          <ul class="queue-list">
            {#each queue as p, i}
              <li class="queue-row">
                <span class="queue-idx">{i + 1}</span>
                <span class="queue-name mono">{p.split('/').pop()}</span>
                <button class="btn-ghost btn-xs" on:click={() => queueLoad(i)} disabled={muxing}>Charger</button>
                <button class="btn-delete" on:click={() => queueRemove(i)} title="Retirer de la file">✕</button>
              </li>
            {/each}
          </ul>
        </div>
      {/if}

      <!-- Tracks -->
      {#if tracks.length > 0}
        <!-- Video -->
        <div class="card">
          <div class="section-title">Piste vidéo</div>
          {#each tracks.filter(t => t.type === 'video') as t}
            <div class="track-row video">
              <div class="track-meta">
                <span class="badge video">VIDEO</span>
                <span class="mono">#{t.id} · {t.codec} · {t.pixelDims || ''}</span>
              </div>
              <div class="video-dropdowns">
                <label>Qualité
                  <select bind:value={videoChoice.quality}>
                    {#each options.video_qualities as q}<option>{q}</option>{/each}
                  </select>
                </label>
                <label>Encodeur
                  <select bind:value={videoChoice.encoder}>
                    {#each options.video_encoders as e}<option>{e}</option>{/each}
                  </select>
                </label>
                <label>Type source
                  <select bind:value={videoChoice.sourceType}>
                    {#each VIDEO_SOURCE_TYPE_OPTIONS as s}<option>{s}</option>{/each}
                  </select>
                </label>
                <label>Team de la source
                  <input type="text" bind:value={videoChoice.sourceTeam} placeholder="ex: Alkaline" />
                </label>
              </div>
              <div class="track-preview mono">→ {previewVideoName}</div>
            </div>
          {/each}
        </div>

        <!-- Audio -->
        <div class="card">
          <div class="section-title-row">
            <div class="section-title">Pistes audio</div>
            <button class="btn-tiny" on:click={pickAudioDialog}>+ Ajouter un fichier</button>
          </div>
          {#if tracks.some(t => t.type === 'audio') || externalAudios.length > 0}
            {@const internalAudios = tracks.filter(t => t.type === 'audio')}
            {@const mergedAudios = [
              ...internalAudios.map((t, i) => ({ kind: 'internal', idx: i, ref: t, order: t.order ?? 0 })),
              ...externalAudios.map((a, i) => ({ kind: 'external', idx: i, ref: a, order: a.order ?? 0 })),
            ].sort((a, b) => a.order - b.order)}
            {#each mergedAudios as item (item.kind + '-' + item.idx)}
              <div class="track-row" class:dropped={!item.ref.keep}>
                <div class="track-meta">
                  {#if item.kind === 'internal'}
                    <span class="badge audio">AUDIO</span>
                    <span class="mono">#{item.ref.id} · {item.ref.codec} · {item.ref.lang || '??'} · {item.ref.channels || '?'}ch</span>
                    {#if item.ref.name}<span class="track-current">« {item.ref.name} »</span>{/if}
                  {:else}
                    <span class="badge audio-ext">AUDIO EXT</span>
                    <span class="mono">{basename(item.ref.path)}</span>
                    {#if item.ref.size != null && item.ref.size >= 0}
                      <span class="sub-size mono">{formatBytes(item.ref.size)}</span>
                    {/if}
                  {/if}
                  <div class="order-ctrls">
                    <button class="btn-arrow" title="Monter" on:click={() => moveAudioTrack(item.kind, item.idx, -1)}>↑</button>
                    <button class="btn-arrow" title="Descendre" on:click={() => moveAudioTrack(item.kind, item.idx, +1)}>↓</button>
                    {#if item.kind === 'internal'}
                      <button class="btn-arrow danger" title="Supprimer" on:click={() => removeInternalTrack(item.ref.id)}>✕</button>
                    {:else}
                      <button class="btn-arrow danger" title="Supprimer" on:click={() => removeExternalAudio(item.idx)}>✕</button>
                    {/if}
                  </div>
                </div>
                <div class="track-controls">
                  <select bind:value={item.ref.label}>
                    <option value="">— choisir —</option>
                    {#each options.audio_labels as lbl}<option>{lbl}</option>{/each}
                  </select>
                  <label class="chk"><input type="checkbox" bind:checked={item.ref.keep}/> Garder</label>
                  <label class="chk"><input type="checkbox" bind:checked={item.ref.default}/> Default</label>
                  <label class="chk"><input type="checkbox" bind:checked={item.ref.forced}/> Forced</label>
                </div>
              </div>
            {/each}
          {:else}
            <div class="empty-hint">Aucune piste audio détectée. Clique "+ Ajouter un fichier" pour ajouter un audio externe.</div>
          {/if}
        </div>

        <!-- Subtitles (internes + externes triés par ordre) -->
        <div class="card">
          <div class="section-title-row">
            <div class="section-title">Sous-titres</div>
            <button class="btn-tiny" on:click={pickSubsDialog}>+ Ajouter un fichier</button>
          </div>
          {#if tracks.some(t => t.type === 'subtitles') || externalSubs.length > 0}
            {@const internalSubs = tracks.filter(t => t.type === 'subtitles')}
            {@const mergedSubs = [
              ...internalSubs.map((t, i) => ({ kind: 'internal', idx: i, ref: t, order: t.order ?? 0 })),
              ...externalSubs.map((s, i) => ({ kind: 'external', idx: i, ref: s, order: s.order ?? 0 })),
            ].sort((a, b) => a.order - b.order)}
            {#each mergedSubs as item (item.kind + '-' + item.idx)}
              <div class="track-row">
                <div class="track-meta">
                  {#if item.kind === 'internal'}
                    <span class="badge subs">SUBS</span>
                    <span class="mono">#{item.ref.id} · {item.ref.codec} · {item.ref.lang || '??'}</span>
                    {#if item.ref.name}<span class="track-current">« {item.ref.name} »</span>{/if}
                  {:else}
                    <span class="badge subs-ext">SUBS EXT</span>
                    <span class="mono">{basename(item.ref.path)}</span>
                    {#if item.ref.size != null && item.ref.size >= 0}
                      <span class="sub-size mono">{formatBytes(item.ref.size)}</span>
                    {/if}
                  {/if}
                  <div class="order-ctrls">
                    <button class="btn-arrow" title="Monter" on:click={() => moveTrack(item.kind, item.idx, -1)}>↑</button>
                    <button class="btn-arrow" title="Descendre" on:click={() => moveTrack(item.kind, item.idx, +1)}>↓</button>
                    {#if item.kind === 'internal'}
                      <button class="btn-arrow danger" title="Supprimer" on:click={() => removeInternalTrack(item.ref.id)}>✕</button>
                    {:else}
                      <button class="btn-arrow danger" title="Supprimer" on:click={() => removeExternalSub(item.idx)}>✕</button>
                    {/if}
                  </div>
                </div>
                <div class="track-controls">
                  <select bind:value={item.ref.label}>
                    <option value="">— choisir —</option>
                    {#each options.subtitle_labels as lbl}<option>{lbl}</option>{/each}
                  </select>
                  <label class="chk"><input type="checkbox" bind:checked={item.ref.keep}/> Garder</label>
                  <label class="chk"><input type="checkbox" bind:checked={item.ref.default}/> Default</label>
                  <label class="chk"><input type="checkbox" bind:checked={item.ref.forced}/> Forced</label>
                </div>
              </div>
            {/each}
          {:else}
            <div class="empty-hint">Aucun sous-titre détecté. Drop un .srt/.sup/.ass/.sub ici ou clique "+ Ajouter un fichier".</div>
          {/if}
        </div>

        <div class="actions-row">
          <button class="btn-primary" on:click={() => screen = 'cible'}>Suivant → Cible</button>
        </div>
      {/if}

    {:else if screen === 'cible'}
      <div class="card">
        <div class="section-title">Recherche TMDB</div>
        <div class="lang-toggle" style:margin-bottom="8px">
          <button class:active={tmdbMode === 'movie'} on:click={() => tmdbMode = 'movie'}>🎬 Film</button>
          <button class:active={tmdbMode === 'tv'}    on:click={() => tmdbMode = 'tv'}>📺 Série</button>
        </div>
        <div class="field-row">
          <input type="text" bind:value={tmdbQuery} placeholder={tmdbMode === 'tv' ? 'Titre de série ou ID TMDB…' : 'Titre du film ou ID TMDB…'} on:keydown={(e) => e.key === 'Enter' && searchTmdb()} />
          <button class="btn-primary" on:click={searchTmdb} disabled={tmdbSearching}>{tmdbSearching ? '…' : 'Chercher'}</button>
        </div>
        {#if tmdbMode === 'tv' && !config.tmdb_key}
          <div class="field-hint">⚠ Clé API TMDB requise pour chercher des séries — Réglages.</div>
        {/if}
        {#if tmdbResults.length > 0}
          <ul class="tmdb-list">
            {#each tmdbResults as r}
              <li>
                <button class="tmdb-item" on:click={() => pickTmdb(r)}>
                  {#if r.poster_url}<img class="tmdb-poster" src={r.poster_url} alt=""/>{/if}
                  <div class="tmdb-body">
                    <div class="tmdb-title">{r.titre_fr || r.titre_vo} <span class="tmdb-year">({r.annee_fr})</span></div>
                    <div class="tmdb-meta">{r.duree || ''} · ⭐ {r.note || '?'}</div>
                  </div>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>

      <div class="card">
        <div class="section-title">Titre cible</div>
        {#if target.title}
          <div class="tmdb-preview mono">{target.title}</div>
        {/if}
        <div class="field-row">
          <div class="field" style:flex="3"><label>Titre</label>
            <input type="text" bind:value={target.title} placeholder="Titre du film" />
          </div>
          <div class="field" style:flex="1"><label>Année</label>
            <input type="text" bind:value={target.year} placeholder="2025" maxlength="4" />
          </div>
        </div>
        <div class="field-row">
          <div class="field" style:flex="1"><label>Résolution</label>
            <select bind:value={target.resolution}>
              {#each RESOLUTION_OPTIONS as r}<option>{r}</option>{/each}
            </select>
          </div>
          <div class="field" style:flex="1"><label>Source</label>
            <select bind:value={target.source}>
              {#each TARGET_SOURCE_OPTIONS as s}<option>{s}</option>{/each}
            </select>
          </div>
          <div class="field" style:flex="1"><label>Codec vidéo</label>
            <input type="text" bind:value={target.video_codec} list="codec-suggestions" placeholder="H264" />
            <datalist id="codec-suggestions">
              {#each VIDEO_CODEC_OPTIONS as c}<option value={c}/>{/each}
            </datalist>
          </div>
        </div>
        <div class="field-hint">Codec auto-suggéré : <b class="mono">{suggestedCodecDisplay || '—'}</b> — modifie au besoin.</div>
        <div class="preview-box">
          <div class="preview-label">Nom de fichier final</div>
          {#if lastTmdbResult}
            <div class="lang-toggle">
              <button class:active={target.lang === 'auto'} on:click={() => target.lang = 'auto'}>Auto</button>
              <button class:active={target.lang === 'vf'}   on:click={() => target.lang = 'vf'}>VF</button>
              <button class:active={target.lang === 'vo'}   on:click={() => target.lang = 'vo'}>VO</button>
            </div>
          {/if}
          <div class="preview-filename-row">
            <div class="preview-value mono">{previewFilename || '—'}</div>
            {#if previewFilename}
              <button class="btn-copy" on:click={copyFilename} title="Copier">{filenameCopied ? '✓ Copié' : '📋 Copier'}</button>
              <button class="btn-copy" on:click={openOutputDir} disabled={!config.output_dir} title="Ouvrir dossier de sortie">📂 Dossier</button>
            {/if}
          </div>
        </div>
      </div>

      <div class="actions-row">
        {#if muxing}
          <button class="btn-cancel" on:click={stopMux}>Stop</button>
          <div class="progress-bar"><div class="progress-fill" style:width="{muxPercent}%"></div></div>
          <span class="mono">{muxPercent}%</span>
        {:else}
          <button class="btn-primary" on:click={doMux} disabled={!sourcePath || !previewFilename}>Muxer</button>
        {/if}
      </div>

    {:else if screen === 'reglages'}
      <div class="card">
        <div class="section-title">TMDB</div>
        <div class="field"><label>Clé API TMDB (optionnelle)</label>
          <div class="field-row">
            <input type="password" bind:value={config.tmdb_key} placeholder="laisse vide si tu utilises juste serveurperso" />
            <button class="btn-test" on:click={doTestTmdbKey} disabled={tmdbTest.running}>
              {tmdbTest.running ? '…' : 'Test'}
            </button>
          </div>
          {#if tmdbTest.ok !== null}
            <div class="result-badge {tmdbTest.ok ? 'ok' : 'err'}">{tmdbTest.message}</div>
          {/if}
        </div>
        <div class="field"><label>URL de l'index</label>
          <input type="text" bind:value={config.serveurperso_url} />
        </div>
      </div>

      <div class="card">
        <div class="section-title">Dossier de sortie</div>
        <div class="field-row">
          <input type="text" bind:value={config.output_dir} placeholder="/Users/…/Mux" readonly />
          <button class="btn-test" on:click={pickOutputDir}>Choisir…</button>
          <button class="btn-test" on:click={openOutputDir} disabled={!config.output_dir} title="Ouvrir dans le Finder">📂 Ouvrir</button>
        </div>
      </div>

      <div class="card">
        <div class="section-title">MKVToolNix</div>
        <div class="field"><label>Chemin mkvmerge (optionnel — auto-détecté sinon)</label>
          <input type="text" bind:value={config.mkvmerge_path} placeholder="/opt/homebrew/bin/mkvmerge" />
        </div>
        <div class="field-hint">Détecté actuellement : <b class="mono">{mkvmergePath || 'introuvable'}</b></div>
      </div>

      <div class="card">
        <div class="section-title">Valeurs par défaut</div>
        <div class="field"><label>Qualité</label>
          <select bind:value={config.default_quality}>
            {#each options.video_qualities as q}<option>{q}</option>{/each}
          </select>
        </div>
        <div class="field"><label>Encodeur</label>
          <select bind:value={config.default_encoder}>
            {#each options.video_encoders as e}<option>{e}</option>{/each}
          </select>
        </div>
        <div class="field"><label>Type source piste vidéo</label>
          <select bind:value={config.default_source}>
            {#each VIDEO_SOURCE_TYPE_OPTIONS as s}<option>{s}</option>{/each}
          </select>
        </div>
        <div class="field"><label>Team de sortie (dans le filename)</label>
          <input type="text" bind:value={config.default_team} placeholder="LiHDL" />
        </div>
      </div>

      <div class="actions-row">
        <button class="btn-primary" on:click={saveSettings}>Enregistrer</button>
      </div>
    {/if}
  </section>

  <section class="log-panel" bind:this={logEl}>
    {#each logLines as l}
      <div class="log-line">
        <span class="log-time">{l.time}</span>
        <span class="log-msg lvl-{l.level}">{l.text}</span>
      </div>
    {/each}
  </section>
</main>

<style>
  :root {
    --bg:         #0d0a10;
    --bg-tint:    #14101a;
    --bg2:        #1a1420;
    --border:     rgba(255, 255, 255, 0.08);
    --border-strong: rgba(255, 255, 255, 0.14);
    --red:        #e63946;
    --red-hot:    #ff5a4a;
    --blue:       #00b4d8;
    --blue-hot:   #48cae4;
    --yellow:     #ffd60a;
    --green:      #7ef0c0;
    --text:       #f5efe7;
    --text2:      #b5a9a1;
    --text3:      #7a6e68;
  }

  :global(body) {
    color: var(--text);
    background:
      radial-gradient(1400px 900px at 50% 30%, rgba(230, 57, 70, 0.07), transparent 70%),
      var(--bg);
  }

  main {
    display: flex;
    flex-direction: column;
    min-height: 100vh;
    text-align: left;
    position: relative;
    isolation: isolate;
  }

  /* Banner en watermark de fond, centré, opacité réduite */
  main::before {
    content: '';
    position: fixed;
    inset: 100px 0 180px 0;
    background-image: url(./assets/images/banner.png);
    background-repeat: no-repeat;
    background-position: center center;
    background-size: clamp(500px, 70%, 1100px) auto;
    opacity: 0.10;
    pointer-events: none;
    z-index: -1;
  }

  /* ---- Topbar (header style LiHDL Post Discord v3) ---- */
  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 20px;
    border-bottom: 1px solid var(--border);
    background: rgba(13, 10, 16, 0.65);
    backdrop-filter: blur(12px) saturate(140%);
    -webkit-backdrop-filter: blur(12px) saturate(140%);
  }
  .brand {
    display: flex; align-items: center; gap: 14px;
  }
  .logo {
    width: 48px; height: 48px; border-radius: 10px;
    object-fit: contain;
    box-shadow: 0 0 24px rgba(230, 57, 70, 0.35);
  }
  .brand-text { display: flex; flex-direction: column; gap: 3px; line-height: 1; }
  .app-title {
    font-size: 15px; font-weight: 800;
    letter-spacing: 2.5px;
    background: linear-gradient(90deg, #ff5a4a 0%, #ffd60a 35%, #7ef0c0 70%, #48cae4 100%);
    -webkit-background-clip: text;
    background-clip: text;
    color: transparent;
  }
  /* LiHDL garde sa casse d'origine (L majuscule, i minuscule, HDL majuscule). */
  .brand-lihdl { letter-spacing: 1.5px; }
  .app-subtitle {
    display: inline-block; align-self: flex-start;
    padding: 2px 8px; border-radius: 10px;
    font-size: 10px; font-weight: 700; letter-spacing: 1.8px;
    background: rgba(255,255,255,0.06);
    border: 1px solid var(--border);
    color: var(--text2);
  }

  .topbar-right { display: flex; align-items: center; gap: 10px; }

  .update-pill {
    display: inline-flex; align-items: center; gap: 7px;
    padding: 7px 13px; border-radius: 10px;
    background: rgba(0, 180, 216, 0.08);
    border: 1px solid rgba(0, 180, 216, 0.35);
    color: var(--blue-hot);
    font: inherit; font-size: 12px; font-weight: 700;
    letter-spacing: 0.5px;
    cursor: pointer; transition: all 150ms;
  }
  .update-pill:hover:not(:disabled) {
    background: rgba(0, 180, 216, 0.18);
    border-color: rgba(0, 180, 216, 0.55);
  }
  .update-pill:disabled { cursor: wait; opacity: 0.7; }
  .update-pill.available {
    background: rgba(126, 240, 192, 0.12);
    border-color: rgba(126, 240, 192, 0.5);
    color: var(--green);
    animation: pulse-available 2s infinite;
  }
  .update-pill.available:hover:not(:disabled) {
    background: rgba(126, 240, 192, 0.22);
  }
  @keyframes pulse-available {
    0%,100% { box-shadow: 0 0 0 0 rgba(126, 240, 192, 0.3); }
    50%     { box-shadow: 0 0 0 6px rgba(126, 240, 192, 0); }
  }
  .version-icon { opacity: 0.7; display: inline-block; }
  .version-icon.spin { animation: spin-icon 1s linear infinite; }
  @keyframes spin-icon { to { transform: rotate(360deg); } }
  .version-label { font-variant-numeric: tabular-nums; }

  .settings-btn {
    display: inline-flex; align-items: center; gap: 8px;
    padding: 7px 13px; border-radius: 10px;
    border: 1px solid var(--border);
    background: rgba(255,255,255,0.04);
    color: var(--text2);
    font: inherit; font-size: 11px; font-weight: 700;
    letter-spacing: 1.5px;
    cursor: pointer; transition: all 150ms;
  }
  .settings-btn:hover {
    background: rgba(255,255,255,0.08);
    color: var(--text); border-color: var(--border-strong);
  }
  .gear { font-size: 14px; }

  .tabs {
    display: flex; gap: 2px; padding: 0 20px;
    border-bottom: 1px solid var(--border);
    background: var(--bg-tint);
  }
  .tabs button {
    padding: 12px 18px; border: none; border-bottom: 2px solid transparent;
    background: transparent; color: var(--text2);
    font: inherit; font-size: 13px; font-weight: 600; cursor: pointer;
    transition: all 150ms;
  }
  .tabs button:hover { color: var(--text); }
  .tabs button.active { color: var(--red-hot); border-bottom-color: var(--red); }

  .content { flex: 1; padding: 20px; overflow-y: auto; }

  .card {
    background: linear-gradient(180deg, rgba(26, 20, 32, 0.75), rgba(18, 14, 24, 0.75));
    border: 1px solid var(--border);
    border-radius: 14px;
    padding: 18px 20px;
    margin-bottom: 14px;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.04), 0 10px 30px -12px rgba(0,0,0,0.5);
  }
  .section-title {
    font-size: 11px; font-weight: 700;
    color: var(--text2);
    text-transform: uppercase; letter-spacing: 1.2px;
    margin-bottom: 12px;
  }

  .drop-target {
    border: 2px dashed rgba(230, 57, 70, 0.3);
    text-align: center;
    padding: 48px 28px;
    max-width: 640px;
    margin-left: auto;
    margin-right: auto;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 6px;
  }
  .drop-icon { font-size: 42px; margin-bottom: 10px; }
  .drop-title { font-size: 15px; font-weight: 700; color: var(--text); }
  .drop-sub { font-size: 12px; color: var(--text2); margin: 4px 0 10px; word-break: break-all; }

  .track-row {
    padding: 10px 0;
    border-top: 1px dashed var(--border);
    transition: opacity 150ms;
  }
  .track-row.dropped { opacity: 0.38; }
  .track-row.dropped .track-meta { text-decoration: line-through; }
  .track-row:first-child { border-top: none; }
  .track-row.video .video-dropdowns {
    display: grid; grid-template-columns: repeat(4, 1fr);
    gap: 10px; margin-top: 8px;
  }
  .video-dropdowns label { font-size: 11px; color: var(--text3); display: flex; flex-direction: column; gap: 4px; }
  .video-dropdowns select, .video-dropdowns input[type="text"] {
    width: 100%; box-sizing: border-box;
    height: 34px; line-height: 1.2;
    padding: 0 11px;
    margin: 0;
    appearance: none; -webkit-appearance: none;
  }
  .track-preview { font-size: 12px; color: var(--green); margin-top: 8px; }

  .track-meta { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  .track-current { color: var(--text3); font-size: 11px; font-style: italic; }
  .badge {
    padding: 2px 7px; border-radius: 4px; font-size: 10px; font-weight: 700;
    letter-spacing: 1px;
  }
  .badge.audio { background: rgba(0,180,216,0.15); color: var(--blue-hot); }
  .badge.subs  { background: rgba(255,214,10,0.15); color: var(--yellow); }
  .badge.subs-ext { background: rgba(255,149,133,0.18); color: #ff9585; }

  .section-title-row {
    display: flex; justify-content: space-between; align-items: center;
    margin-bottom: 12px;
  }
  .section-title-row .section-title { margin-bottom: 0; }

  .btn-tiny {
    padding: 5px 10px; border-radius: 6px;
    border: 1px solid var(--border); background: rgba(255,255,255,0.03);
    color: var(--text2); font: inherit; font-size: 11px; font-weight: 600;
    cursor: pointer; transition: all 150ms;
  }
  .btn-tiny:hover { background: rgba(255,255,255,0.08); color: var(--text); border-color: var(--border-strong); }

  .order-ctrls {
    display: flex; gap: 4px; margin-left: auto;
  }
  .btn-arrow {
    width: 24px; height: 24px; border-radius: 5px;
    border: 1px solid var(--border); background: rgba(255,255,255,0.03);
    color: var(--text2); font: inherit; font-size: 12px; cursor: pointer;
    display: inline-flex; align-items: center; justify-content: center;
    transition: all 120ms;
  }
  .btn-arrow:hover { background: rgba(255,255,255,0.1); color: var(--text); }
  .btn-arrow.danger:hover { background: rgba(239,68,68,0.15); color: #ff9585; border-color: rgba(239,68,68,0.4); }

  .sub-size {
    padding: 2px 6px; border-radius: 4px; font-size: 10px;
    background: rgba(255,255,255,0.04); color: var(--text3);
    border: 1px solid var(--border);
  }

  .empty-hint {
    padding: 16px; text-align: center;
    font-size: 12px; color: var(--text3); font-style: italic;
    border: 1px dashed var(--border); border-radius: 8px;
  }
  .badge.video { background: rgba(126,240,192,0.15); color: var(--green); }

  .track-controls {
    display: flex; gap: 10px; align-items: center; margin-top: 8px; flex-wrap: wrap;
  }
  .chk {
    display: flex; align-items: center; gap: 5px;
    font-size: 11px; color: var(--text2); cursor: pointer;
  }

  select, input[type="text"], input[type="password"] {
    padding: 8px 11px;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: rgba(0,0,0,0.3);
    color: var(--text);
    font: inherit; font-size: 12px;
    transition: all 150ms;
  }
  select:hover, input:hover { border-color: var(--border-strong); }
  select:focus, input:focus { outline: none; border-color: rgba(230,57,70,0.5); box-shadow: 0 0 0 3px rgba(230,57,70,0.15); }

  .field { display: flex; flex-direction: column; gap: 5px; margin-bottom: 10px; }
  .field label { font-size: 11px; color: var(--text2); font-weight: 600; }
  .field-hint { font-size: 11px; color: var(--text3); margin-top: 4px; }
  .field-hint b { color: var(--text); }
  .field-row { display: flex; gap: 8px; }
  .field-row input, .field-row select { flex: 1; }

  .actions-row {
    display: flex; gap: 10px; align-items: center;
    padding: 14px 0 0; justify-content: flex-end;
  }
  .btn-primary {
    padding: 10px 18px; border-radius: 8px; border: 1px solid rgba(230,57,70,0.5);
    background: linear-gradient(180deg, rgba(230,57,70,0.25), rgba(230,57,70,0.18));
    color: #fff; font: inherit; font-size: 13px; font-weight: 700; cursor: pointer;
    transition: all 150ms;
  }
  .btn-primary:hover:not(:disabled) { background: rgba(230,57,70,0.35); border-color: var(--red); }
  .btn-primary:disabled { opacity: 0.4; cursor: not-allowed; }
  .btn-ghost {
    padding: 8px 14px; border-radius: 8px; border: 1px solid var(--border);
    background: transparent; color: var(--text2);
    font: inherit; font-size: 12px; cursor: pointer;
  }
  .btn-ghost:hover { color: var(--text); border-color: var(--border-strong); }
  .btn-test {
    padding: 9px 14px; border-radius: 8px; border: 1px solid var(--border);
    background: rgba(0, 180, 216, 0.08); color: var(--blue-hot);
    font: inherit; font-size: 12px; font-weight: 600; cursor: pointer;
    white-space: nowrap;
  }
  .btn-test:hover { background: rgba(0,180,216,0.18); border-color: rgba(0,180,216,0.5); }
  .btn-cancel {
    padding: 8px 14px; border-radius: 8px; border: 1px solid rgba(239,68,68,0.4);
    background: rgba(239,68,68,0.12); color: #ff9585;
    font: inherit; font-size: 12px; font-weight: 600; cursor: pointer;
  }
  .btn-cancel:hover { background: rgba(239,68,68,0.22); border-color: rgba(239,68,68,0.7); }

  .progress-bar {
    flex: 1; height: 8px; background: rgba(255,255,255,0.06);
    border-radius: 4px; overflow: hidden;
  }
  .progress-fill {
    height: 100%; background: linear-gradient(90deg, var(--red), var(--red-hot));
    transition: width 200ms ease-out;
  }

  .tmdb-preview {
    padding: 8px 12px; margin: 6px 0 12px;
    background: rgba(126, 240, 192, 0.06);
    border: 1px solid rgba(126, 240, 192, 0.2);
    border-radius: 8px; font-size: 13px; color: var(--green);
  }

  .lang-toggle {
    display: inline-flex; gap: 2px; margin: 0 0 12px;
    padding: 3px; border-radius: 8px; border: 1px solid var(--border);
    background: rgba(0,0,0,0.25);
  }
  .lang-toggle button {
    padding: 5px 12px; border: none; border-radius: 6px;
    background: transparent; color: var(--text2);
    font: inherit; font-size: 11px; font-weight: 700; letter-spacing: 0.5px;
    cursor: pointer; transition: all 120ms;
  }
  .lang-toggle button:hover { color: var(--text); }
  .lang-toggle button.active {
    background: rgba(230, 57, 70, 0.2);
    color: var(--red-hot);
  }

  .result-badge {
    margin-top: 6px; padding: 6px 10px; border-radius: 6px;
    font-size: 11px;
  }
  .result-badge.ok  { background: rgba(126,240,192,0.1); color: var(--green); border: 1px solid rgba(126,240,192,0.3); }
  .result-badge.err { background: rgba(255,149,133,0.1); color: #ff9585; border: 1px solid rgba(255,149,133,0.3); }

  .preview-box {
    margin-top: 10px; padding: 12px 14px;
    background: rgba(0,0,0,0.35); border: 1px solid var(--border);
    border-radius: 8px;
  }
  .preview-label {
    font-size: 10px; color: var(--text3); text-transform: uppercase;
    letter-spacing: 1px; margin-bottom: 6px;
  }
  .preview-value { color: var(--green); font-size: 13px; word-break: break-all; flex: 1; }
  .preview-filename-row { display: flex; gap: 10px; align-items: flex-start; }
  .btn-copy {
    flex-shrink: 0; padding: 4px 10px; font-size: 11px;
    background: var(--panel2); color: var(--text); border: 1px solid var(--border);
    border-radius: 6px; cursor: pointer; white-space: nowrap;
  }
  .btn-copy:hover { background: var(--panel3); }

  .queue-list { list-style: none; padding: 0; margin: 8px 0 0; display: flex; flex-direction: column; gap: 4px; }
  .queue-row {
    display: flex; align-items: center; gap: 8px;
    padding: 6px 10px; background: rgba(0,0,0,0.25); border: 1px solid var(--border); border-radius: 6px;
  }
  .queue-idx { color: var(--text3); font-size: 11px; min-width: 16px; }
  .queue-name { flex: 1; font-size: 12px; color: var(--text); word-break: break-all; }
  .btn-xs { padding: 2px 8px; font-size: 11px; }

  .tmdb-list { list-style: none; padding: 0; margin: 10px 0 0; display: flex; flex-direction: column; gap: 6px; }
  .tmdb-item {
    display: flex; gap: 10px; align-items: flex-start; width: 100%; text-align: left;
    padding: 8px; background: rgba(0,0,0,0.25); border: 1px solid var(--border);
    border-radius: 8px; cursor: pointer; font: inherit; color: inherit;
  }
  .tmdb-item:hover { border-color: rgba(230,57,70,0.5); background: rgba(230,57,70,0.06); }
  .tmdb-poster { width: 40px; height: 60px; object-fit: cover; border-radius: 4px; }
  .tmdb-body { flex: 1; min-width: 0; }
  .tmdb-title { font-weight: 600; font-size: 13px; }
  .tmdb-year { color: var(--text3); font-weight: 400; }
  .tmdb-meta { font-size: 11px; color: var(--text2); margin-top: 2px; }

  .mono { font-family: "JetBrains Mono", "SF Mono", ui-monospace, monospace; font-size: 11px; }

  .log-panel {
    height: 170px; overflow-y: auto; padding: 10px 14px;
    background: rgba(0,0,0,0.35); border-top: 1px solid var(--border);
    font-family: "JetBrains Mono", "SF Mono", ui-monospace, monospace;
    font-size: 11px; line-height: 1.6; color: var(--text2);
  }
  .log-line { display: flex; gap: 10px; padding: 1px 0; }
  .log-time { color: rgba(245,239,231,0.25); font-variant-numeric: tabular-nums; }
  .log-msg { word-break: break-word; white-space: pre-wrap; }
  :global(.log-msg.lvl-default)  { color: #d7d0c8 !important; }
  :global(.log-msg.lvl-progress) { color: #ffd60a !important; }
  :global(.log-msg.lvl-ok)       { color: #7ef0c0 !important; }
  :global(.log-msg.lvl-error)    { color: #ff9585 !important; }
</style>
