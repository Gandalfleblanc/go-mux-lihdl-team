<script>
  import { onMount } from 'svelte';
  import banner from './assets/images/banner.png';
  import logo from './assets/images/logo.png';
  import {
    GetVersion, GetConfig, SaveConfig, GetLihdlOptions,
    SelectMkvFile, SelectMkvFiles, SelectSubFiles, SelectSupFiles, SelectAudioFiles, SelectOutputDir, LocateMkvmerge, OpenFolder, SearchTmdbTV, SearchTmdbMovie, AnalyzeMkvSecondary, MoveToTrash, MoveDirContentsToTrash, LookupHydrackerURL, TestHydrackerKey, TestUnfrKey, OpenURL, GetMkvBasicInfo, ExtractRefSubs, ExtractFRAudios, CheckSubsSync, CheckPSASync,
    AnalyzeMkv, SearchTmdb, TestTmdbKey, FileSize,
    Mux, CancelMux,
    CheckUpdate, InstallUpdate,
    ListAudioTracksForSync, MuxAudioSync, DetectAudioOffset,
    OCRPGSTrack, OCRSupFile,
    TestLanguageToolKey,
    ApplyOCRFix,
    SearchOpenSubtitles, DownloadOpenSubtitle,
    OCRCustomDictList, OCRCustomDictAdd, OCRCustomDictRemove,
    DiscordIndexScan, DiscordIndexRead, DiscordIndexLookup, DiscordIndexRefreshRemote, DiscordIndexPushGitHub,
  } from '../wailsjs/go/main/App.js';
  import { EventsOn } from '../wailsjs/runtime/runtime.js';

  let screen = 'source';
  let muxMode = 'lihdl';   // 'lihdl' | 'psa'

  function switchMuxMode(mode) {
    if (muxMode === mode) return;
    muxMode = mode;
    if (mode === 'psa') {
      // Défauts mode PSA : série WEBRip Team GANDALF
      videoChoice.team = 'GANDALF';
      if (!target.source || target.source === 'HDLight') target.source = 'WEBRip';
      if (sourcePath && !target.episode) {
        target.episode = detectEpisode(sourcePath.split('/').pop()) || 'S01E01';
      }
      // Si une source est déjà chargée, re-jouer l'auto-config PSA :
      // - Codec H265/H264 → x265/x264 (norme GANDALF)
      // - Audios PSA décochés (remplacés par SUPPLY)
      // - Subs non-FR décochés
      if (sourcePath) {
        const psaName = sourcePath.split('/').pop() || '';
        const psa = parsePsaSourceInfo(psaName);
        if (psa.isPSA) {
          videoChoice.quality = 'Custom PSA';
          videoChoice.encoder = 'GANDALF';
          videoChoice.team = 'GANDALF';
          videoChoice.sourceTeam = '';
          if (psa.source) {
            target.source = psa.source;
            videoChoice.sourceType = psa.source + ' PSA';
          }
          if (psa.videoCodec) {
            let vc = psa.videoCodec;
            if (vc === 'H265') vc = 'x265';
            else if (vc === 'H264') vc = 'x264';
            target.video_codec = vc;
          }
          appendLog(`✓ PSA re-détecté au switch : ${psa.source || '?'} ${target.video_codec || '?'} → Custom PSA / GANDALF`);
        }
        // Auto-décoche audios + subs non-FR sur les tracks déjà chargés.
        if (tracks.length > 0) {
          let droppedAud = 0, droppedSub = 0;
          tracks = tracks.map(t => {
            if (t.type === 'audio') {
              if (t.keep) droppedAud++;
              return { ...t, keep: false };
            }
            if (t.type === 'subtitles') {
              const isFR = /^fr/i.test(t.lang || '') || /^FR /.test(t.label || '');
              if (t.keep && !isFR) droppedSub++;
              return { ...t, keep: isFR };
            }
            return t;
          });
          if (droppedAud || droppedSub) {
            appendLog(`🎬 Mode PSA : ${droppedAud} audio(s) PSA décoché(s), ${droppedSub} sub(s) non-FR décoché(s)`);
          }
        }
      }
    } else {
      // Défauts mode LiHDL : film/release Team LiHDL
      videoChoice.team = 'LiHDL';
      if (target.source === 'WEBRip') target.source = 'HDLight';
      // Pas de SUPPLY en mode LiHDL — on nettoie.
      clearSecondary();
    }
    appendLog('Mode mux : ' + mode.toUpperCase());
  }
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
    languagetool_key: '',
    languagetool_user: '',
    languagetool_url: '',
    opensubtitles_api_key: '',
    discord_bot_token: '',
    discord_forum_id: '',
    discord_index_url: '',
    github_token: '',
    github_repo: '',
    github_branch: 'main',
    github_index_file_path: 'discord_index.json',
  };

  // Index Discord — état UI :
  // discordIndexEntry  = URL Discord du film actuel ("" si pas trouvé)
  // discordScanRunning = scan admin en cours
  // discordScanProgress = { scanned, total, message }
  let discordIndexEntry = '';
  let discordScanRunning = false;
  let discordScanProgress = { scanned: 0, total: 0, message: '' };
  let discordCopyOk = false;
  let githubPushing = false;
  let githubPushOk = false;

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
  let target = {
    title: '', year: '', resolution: '1080p', source: 'HDLight',
    video_codec: '', lang: 'auto',
    episode: '',          // SxxExx auto-détecté (vide = film)
    flagOverride: 'auto', // override du langflag : auto | VFF | VFQ | VFi | VO | MULTi.VFF | MULTi.VFQ | MULTi.VF2 | MULTi.VFi | FASTSUB.VOSTFR | FASTSUB.FR
  };
  const FLAG_OVERRIDE_OPTIONS = [
    'auto',
    'VFF', 'VFQ', 'VFi', 'VO',
    'MULTi.VFF', 'MULTi.VFQ', 'MULTi.VF2', 'MULTi.VFi',
    'FASTSUB.VOSTFR', 'FASTSUB.FR',
  ];

  function detectEpisode(filename) {
    const m = /\bS(\d{1,2})E(\d{1,3})\b/i.exec(String(filename || ''));
    if (!m) return '';
    const s = m[1].padStart(2, '0');
    const e = m[2].padStart(2, '0');
    return `S${s}E${e}`;
  }

  // Détecte le type de source depuis le nom de fichier selon les normes LiHDL.
  // Priorité : REMUX CUSTOM > WEB-DL CUSTOM > WEB CUSTOM > REMUX > WEB-DL > WEBRip > WEB > COMPLETE BluRay
  // "COMPLETE BluRay" exige les deux mots présents — sinon pas d'auto-detect.
  // WEB-DL accepte aussi la forme dotée "WEB.DL" (fréquente dans les filenames).
  function detectSourceType(filename) {
    const n = String(filename || '').toUpperCase();
    const hasCustom = /\bCUSTOM\b/.test(n);
    const hasWebDL  = /\bWEB[-.]DL\b/.test(n);
    if (/\bREMUX\b/.test(n) && hasCustom)  return 'REMUX CUSTOM';
    if (hasWebDL && hasCustom)             return 'WEB-DL CUSTOM';
    if (/\bWEB\b/.test(n) && hasCustom && !hasWebDL) return 'WEB CUSTOM';
    if (/\bREMUX\b/.test(n))   return 'REMUX';
    if (hasWebDL)              return 'WEB-DL';
    if (/\bWEBRIP\b/.test(n))  return 'WEBRip';
    if (/\bWEB\b/.test(n))     return 'WEB';
    if (/\bCOMPLETE\b/.test(n) && /\bBLURAY\b/.test(n)) return 'COMPLETE BluRay';
    return '';
  }

  // Parse infos d'un nom de fichier PSA (vidéo principale).
  // Retourne : { isPSA, source, videoCodec, team }.
  function parsePsaSourceInfo(filename) {
    const n = String(filename || '');
    const stripped = n.replace(/\.[^.]+$/, '');
    const teamMatch = /-([A-Za-z0-9]+)\s*$/.exec(stripped);
    const team = teamMatch ? teamMatch[1].toUpperCase() : '';
    const isPSA = team === 'PSA';
    let source = '';
    if (/\bWEBRip\b/i.test(n)) source = 'WEBRip';
    else if (/\bWEB-DL\b/i.test(n)) source = 'WEB-DL';
    else if (/\bBluRay\b/i.test(n)) source = 'BluRay';
    else if (/\bWEB\b/i.test(n)) source = 'WEB';
    let videoCodec = '';
    if (/\bx265\b/i.test(n)) videoCodec = 'x265';
    else if (/\bx264\b/i.test(n)) videoCodec = 'x264';
    else if (/\b[Hh]\.?265\b|\bHEVC\b/.test(n)) videoCodec = 'H265';
    else if (/\b[Hh]\.?264\b|\bAVC\b/.test(n)) videoCodec = 'H264';
    return { isPSA, source, videoCodec, team };
  }

  // Parse infos d'un nom de fichier SUPPLY/FW (audios+subs source).
  // Retourne : { team, langFlag }.
  function parseSupplyInfo(filename) {
    const n = String(filename || '');
    const stripped = n.replace(/\.[^.]+$/, '');
    const teamMatch = /-([A-Za-z0-9]+)\s*$/.exec(stripped);
    const team = teamMatch ? teamMatch[1] : '';
    let langFlag = '';
    if (/\bFASTSUB\.VOSTFR\b/i.test(n)) langFlag = 'FASTSUB.VOSTFR';
    else if (/\bFASTSUB\.FR\b/i.test(n)) langFlag = 'FASTSUB.FR';
    else if (/\bMULTi?\.VF2\b/i.test(n)) langFlag = 'MULTi.VF2';
    else if (/\bMULTi?\.VFF\b/i.test(n)) langFlag = 'MULTi.VFF';
    else if (/\bMULTi?\.VFQ\b/i.test(n)) langFlag = 'MULTi.VFQ';
    else if (/\bMULTi?\.VFi\b/i.test(n)) langFlag = 'MULTi.VFi';
    else if (/\bVFF\b/i.test(n)) langFlag = 'VFF';
    else if (/\bVFQ\b/i.test(n)) langFlag = 'VFQ';
    else if (/\bVFi\b/i.test(n)) langFlag = 'VFi';
    else if (/\bVOSTFR\b/i.test(n)) langFlag = 'FASTSUB.VOSTFR';
    return { team, langFlag };
  }
  // Dernière fiche TMDB sélectionnée — permet de basculer VF/VO sans re-chercher.
  let lastTmdbResult = null;
  // Flag de validation TMDB : tant qu'il est false ET qu'on a une fiche TMDB,
  // on bloque l'affichage des pistes/réglages pour forcer une confirmation visuelle.
  let tmdbValidated = false;

  // Lookup Discord à chaque changement de TMDB ID : on demande au backend
  // (qui regarde l'index local d'abord, puis le remote si configuré). Best-
  // effort silencieux ; aucune erreur ne remonte côté UI.
  $: if (lastTmdbResult && lastTmdbResult.tmdb_id) {
    const id = String(lastTmdbResult.tmdb_id);
    DiscordIndexLookup(id).then((u) => {
      // Vérifie que le TMDB ID n'a pas changé entre temps (anti-race).
      if (lastTmdbResult && String(lastTmdbResult.tmdb_id) === id) {
        discordIndexEntry = u || '';
      }
    }).catch(() => { /* silencieux */ });
  } else {
    discordIndexEntry = '';
  }

  // Dropdowns pour le filename (ordre : résolution.source).
  const RESOLUTION_OPTIONS = ['720p', '1080p', '2160p'];
  const TARGET_SOURCE_OPTIONS = ['HDLight', 'WEBLight', 'WEB-DL', 'WEBRip'];
  // Dropdown élargi pour le type de source de la piste vidéo.
  const VIDEO_SOURCE_TYPE_OPTIONS = [
    'REMUX', 'REMUX CUSTOM',
    'WEB-DL CUSTOM', 'WEB CUSTOM',
    'WEB', 'WEB-DL', 'WEBRip',
    'WEBRip PSA Audio SUPPLY', 'WEBRIP PSA Audio FW', 'WEBRip PSA Audio Super U',
    'COMPLETE BluRay',
  ];
  const VIDEO_CODEC_OPTIONS = ['H264', 'x264', 'H265', 'x265', 'AV1'];
  let tmdbResults = [];
  let tmdbResultIndex = 0; // index du résultat actuel dans tmdbResults (cycle via "Autre résultat")

  // Cycle vers le prochain résultat TMDB (utile en cas d'ambiguïté).
  async function cycleNextTmdbResult() {
    if (!tmdbResults || tmdbResults.length < 2) return;
    tmdbResultIndex = (tmdbResultIndex + 1) % tmdbResults.length;
    let picked = tmdbResults[tmdbResultIndex];
    // Si la fiche n'a pas d'overview, fetch détaillé via la clé TMDB.
    if (config.tmdb_key && picked.tmdb_id && !picked.overview) {
      try {
        let detail;
        if (muxMode === 'lihdl') detail = await SearchTmdbMovie(picked.tmdb_id);
        else detail = await SearchTmdb(picked.tmdb_id);
        if (detail && detail.length > 0) picked = detail[0];
      } catch {}
    }
    lastTmdbResult = picked;
    target.title = composeTmdbTitle(picked);
    target.year = picked.annee_fr || '';
    appendLog(`↻ TMDB : passage au résultat ${tmdbResultIndex + 1}/${tmdbResults.length} — ${target.title}`);
    if (muxMode === 'lihdl' && picked && picked.original_language === 'fr') applyVOFSwap();
  }
  let tmdbQuery = '';
  let tmdbIdQuery = ''; // recherche par ID TMDB (séparée du nom)
  let tmdbMode = 'movie'; // 'movie' | 'tv'
  // Toggle VFi par défaut ON : les pistes audio FR sont labellisées FR VFi.
  // Si l'utilisateur décoche → FR VFF.
  let useVFi = true;
  // Règle universelle pour les subs : un label "FR Forced" ou "FR VFF Forced"
  // a default=true ET forced=true. Toutes les autres subs ont les 2 à false.
  function isForcedFRLabel(label) {
    return /^FR( VFF)? Forced\b/.test(label || '');
  }
  // Applique la règle Forced à tous les subs (internes + externes + secondaires).
  // À appeler après tout changement de label sub.
  function applySubForcedRule() {
    tracks = tracks.map(t => {
      if (t.type !== 'subtitles') return t;
      const f = isForcedFRLabel(t.label);
      return { ...t, default: f, forced: f };
    });
    externalSubs = externalSubs.map(s => {
      const f = isForcedFRLabel(s.label);
      return { ...s, default: f, forced: f };
    });
    secondarySelected = secondarySelected.map(t => {
      if (t.type !== 'subtitles') return t;
      const f = isForcedFRLabel(t.label);
      return { ...t, default: f, forced: f };
    });
  }
  // Appelé sur changement de dropdown sub. Mute uniquement la piste concernée.
  function onSubLabelChange() {
    applySubForcedRule();
  }

  // Swap immédiat des labels audio FR VFF ↔ FR VFQ quand le toggle change.
  // Couvre aussi externalAudios + secondarySelected pour mode PSA.
  // Si TMDB indique que le film est originellement français (original_language=fr),
  // toutes les pistes audio FR (VFF/VFi/VFQ) sont en réalité la VO du film → FR VOF.
  function applyVOFSwap() {
    const swap = (lbl) => {
      if (/^FR (VFF|VFi|VFQ) /.test(lbl || '')) return lbl.replace(/^FR (VFF|VFi|VFQ) /, 'FR VOF ');
      return lbl;
    };
    tracks = tracks.map(t => t.type === 'audio'
      ? { ...t, label: swap(t.label || '') }
      : t);
    externalAudios = externalAudios.map(a => ({ ...a, label: swap(a.label || '') }));
    secondarySelected = secondarySelected.map(t => t.type === 'audio'
      ? { ...t, label: swap(t.label || '') }
      : t);
    appendLog('🇫🇷 Film français (TMDB) → labels FR VFF/VFi/VFQ convertis en FR VOF');

    // Sur un film français : 2+ pistes FR + une en 2.0 → c'est l'audiodescription (FR AD).
    // Le flag malvoyant + l'insertion de WiTH.AD dans le nom de fichier sont gérés en aval.
    const toAD = (lbl) => /^FR (VFF|VFQ|VFi|VOF) /.test(lbl || '') && / 2\.0/.test(lbl || '')
      ? lbl.replace(/^FR (VFF|VFQ|VFi|VOF) /, 'FR AD ')
      : lbl;
    const frInternal = tracks.filter(t => t.type === 'audio' && /^FR /.test(t.label || ''));
    if (frInternal.length >= 2) {
      tracks = tracks.map(t => t.type === 'audio' ? { ...t, label: toAD(t.label || '') } : t);
    }
    const frSecondary = secondarySelected.filter(t => t.type === 'audio' && /^FR /.test(t.label || ''));
    if (frSecondary.length >= 2) {
      secondarySelected = secondarySelected.map(t => t.type === 'audio' ? { ...t, label: toAD(t.label || '') } : t);
    }
    if (externalAudios.filter(a => /^FR /.test(a.label || '')).length >= 2) {
      externalAudios = externalAudios.map(a => ({ ...a, label: toAD(a.label || '') }));
    }
    if (tracks.some(t => /^FR AD /.test(t.label || '')) ||
        secondarySelected.some(t => /^FR AD /.test(t.label || '')) ||
        externalAudios.some(a => /^FR AD /.test(a.label || ''))) {
      appendLog('🦮 2ᵉ piste FR 2.0 détectée → labellisée FR AD (audiodescription)');
    }
  }

  // Toggle VFi : applique le swap immédiat sur toutes les pistes audio FR.
  //   useVFi = true  → tout FR VFF devient FR VFi
  //   useVFi = false → tout FR VFi devient FR VFF
  // Note : ne touche PAS les FR VOF (films originellement français).
  function applyVFiSwap() {
    const swapVFFtoVFi = (lbl) => /^FR VFF\b/.test(lbl) ? lbl.replace(/^FR VFF/, 'FR VFi') : lbl;
    const swapVFitoVFF = (lbl) => /^FR VFi\b/.test(lbl) ? lbl.replace(/^FR VFi/, 'FR VFF') : lbl;
    const fn = useVFi ? swapVFFtoVFi : swapVFitoVFF;
    tracks = tracks.map(t => t.type === 'audio'
      ? { ...t, label: fn(t.label || '') }
      : t);
    externalAudios = externalAudios.map(a => ({ ...a, label: fn(a.label || '') }));
    secondarySelected = secondarySelected.map(t => t.type === 'audio'
      ? { ...t, label: fn(t.label || '') }
      : t);
    appendLog(useVFi ? '↻ FR VFF → FR VFi' : '↻ FR VFi → FR VFF');
  }

  // Re-label les sous-titres FR Forced/Full/SDH selon la présence d'une piste
  // audio FR VFQ. Norme LiHDL : sans VFQ, les SRT FR sont nommés "FR Forced",
  // "FR Full". Dès qu'une VFQ est ajoutée, ils deviennent "FR VFF Forced",
  // "FR VFF Full" pour disambiguïser. Si la VFQ est retirée, on revert.
  // Ne touche PAS aux "FR VFQ Forced/Full" (déjà préfixés VFQ) ni aux ENG/etc.
  function applyFRSubVFFLabels() {
    const hasVFQ = tracks.some(t => t.type === 'audio' && /^FR VFQ/.test(t.label || ''))
      || externalAudios.some(a => /^FR VFQ/.test(a.label || ''))
      || secondarySelected.some(t => t.type === 'audio' && /^FR VFQ/.test(t.label || ''));
    const swap = (lbl) => {
      if (!lbl) return lbl;
      if (hasVFQ) {
        const m = lbl.match(/^FR (Forced|Full|SDH)\b(.*)$/);
        if (m) return `FR VFF ${m[1]}${m[2]}`;
      } else {
        const m = lbl.match(/^FR VFF (Forced|Full|SDH)\b(.*)$/);
        if (m) return `FR ${m[1]}${m[2]}`;
      }
      return lbl;
    };
    // Guard idempotent : ne déclenche pas de reactive update si rien ne change.
    let needsUpdate = false;
    const checkSub = (lbl) => { if (swap(lbl) !== lbl) needsUpdate = true; };
    for (const t of tracks) if (t.type === 'subtitles' && !needsUpdate) checkSub(t.label);
    for (const s of externalSubs) if (!needsUpdate) checkSub(s.label);
    for (const t of secondarySelected) if (t.type === 'subtitles' && !needsUpdate) checkSub(t.label);
    if (!needsUpdate) return;
    tracks = tracks.map(t => t.type === 'subtitles' ? { ...t, label: swap(t.label) } : t);
    externalSubs = externalSubs.map(s => ({ ...s, label: swap(s.label) }));
    secondarySelected = secondarySelected.map(t => t.type === 'subtitles' ? { ...t, label: swap(t.label) } : t);
  }

  // Réactif Svelte : à chaque changement audio (interne, externe, secondary),
  // recalcule les labels SRT FR ↔ FR VFF selon la présence VFQ. Le guard
  // dans applyFRSubVFFLabels évite les boucles infinies.
  $: {
    void tracks; void externalAudios; void secondarySelected;
    applyFRSubVFFLabels();
  }

  // Réactif Svelte : si une VFQ apparaît dans les pistes audio, l'autre piste
  // FR (typiquement VFi) doit devenir FR VFF (norme LiHDL — quand y'a 2 pistes
  // FR, l'une est VFF/internationale, l'autre est VFQ québécoise).
  $: {
    void tracks; void externalAudios; void secondarySelected;
    const hasVFQ = tracks.some(t => t.type === 'audio' && /^FR VFQ/.test(t.label || ''))
      || externalAudios.some(a => /^FR VFQ/.test(a.label || ''))
      || secondarySelected.some(t => t.type === 'audio' && /^FR VFQ/.test(t.label || ''));
    if (hasVFQ && useVFi) {
      useVFi = false;
      applyVFiSwap(); // FR VFi → FR VFF (useVFi=false)
    }
  }

  // URL fiche Hydracker (résolue via API à partir de l'ID TMDB ; vide si pas de clé)
  let hydrackerURL = '';

  // Source de référence (3ᵉ barre, optionnelle) : pour vérifier compatibilité
  // durée + FPS avant de récupérer les sous-titres dessus.
  let referencePath = '';
  let sourceMkvInfo = null;     // { duration_seconds, framerate, width, height }
  let referenceMkvInfo = null;
  let showReferenceBar = false;

  function formatDuration(secs) {
    if (!secs || secs <= 0) return '?';
    const s = Math.round(secs);
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const r = s % 60;
    return h > 0 ? `${h}h${String(m).padStart(2,'0')}m${String(r).padStart(2,'0')}s` : `${m}m${String(r).padStart(2,'0')}s`;
  }

  async function pickReferenceDialog() {
    const p = await SelectMkvFile();
    if (!p) return;
    referencePath = p;
    referenceMkvInfo = await GetMkvBasicInfo(p).catch(() => null);
    appendLog('🔍 Référence : ' + p.split('/').pop() + (referenceMkvInfo ? ` (${formatDuration(referenceMkvInfo.duration_seconds)}, ${referenceMkvInfo.framerate} fps)` : ''));
  }

  async function runRefSubsExtraction() {
    if (!referencePath) { appendLog('⚠ Choisis d\'abord une source de référence'); return; }
    srtExtracting = true;
    srtExtractionResult = '';
    srtPhase = '';
    srtAssConverted = false;
    srtPercent = 0;
    try {
      appendLog('⏳ Extraction des sous-titres FR/ENG compatibles…');
      const subs = await ExtractRefSubs(referencePath, sourcePath || '');
      if (subs && subs.length > 0) {
        // Anti-doublon INVERSE : on garde les pistes existantes (internes ou
        // externes) et on SKIP l'extraction des labels déjà présents. Idée :
        // les pistes internes (FR Full, FR Forced) du source LiHDL sont déjà
        // parfaitement synchros — pas la peine de les remplacer par les
        // extracts de la ref qui nécessitent un resync. On n'ajoute que ce
        // qui MANQUE.
        const norm = (s) => (s || '').trim().toLowerCase().replace(/\s+/g, ' ');
        const existingLabels = new Set([
          ...tracks.filter(t => t.type === 'subtitles' && t.keep).map(t => norm(t.label)),
          ...externalSubs.map(s => norm(s.label)),
        ].filter(Boolean));
        const initialCount = subs.length;
        const filteredSubs = subs.filter(s => {
          const lbl = norm(s.label);
          if (lbl && existingLabels.has(lbl)) {
            appendLog(`↳ ${s.label} déjà présent → skip extraction (on garde l'existant)`);
            return false;
          }
          return true;
        });
        if (filteredSubs.length === 0) {
          appendLog(`ℹ Tous les sous-titres extraits étaient déjà présents (${initialCount} skippés) — rien à ajouter`);
          srtExtractionResult = 'success';
          srtExtracting = false;
          srtPhase = '';
          srtPercent = 0;
          return;
        }
        subs.length = 0;
        subs.push(...filteredSubs);
        let maxOrder = 0;
        for (const t of tracks) maxOrder = Math.max(maxOrder, t.order ?? 0);
        for (const s of externalSubs) maxOrder = Math.max(maxOrder, s.order ?? 0);
        for (const s of subs) {
          maxOrder += 10;
          let size = -1;
          try { size = await FileSize(s.path); } catch {}
          // Nom lisible dérivé du label LiHDL (ex: "FR Forced.srt", "ENG Full.srt")
          // au lieu de "submux-extract-XXX.srt".
          const friendlyName = (s.label || '').replace(/\s*:\s*SRT/i, '').replace(/\s+/g, '.') + '.srt';
          externalSubs = [...externalSubs, {
            path: s.path,
            name: friendlyName || basename(s.path),
            size,
            keep: true,
            default: false,
            forced: !!s.forced,
            label: s.label,
            delayMs: s.delay_ms || 0,
            tempoFactor: s.tempo_factor || 1.0,
            order: maxOrder,
            fromReference: true, // extrait de la ref → tempo+offset si FPS différents
          }];
        }
        const tempoInfo = subs[0] && subs[0].tempo_factor && subs[0].tempo_factor !== 1.0 ? ` + tempo ${subs[0].tempo_factor.toFixed(6)}` : '';
        const delayInfo = subs[0] && subs[0].delay_ms ? ` (sync : ${subs[0].delay_ms} ms${tempoInfo} appliqué)` : '';
        appendLog(`✓ ${subs.length} sous-titre(s) FR/ENG extrait(s) et ajouté(s)${delayInfo}`);
        applySubForcedRule();
        // Tri LiHDL des subs (FR avant ENG ; Forced avant Full avant SDH).
        applyLihdlTrackOrder();
        srtExtractionResult = 'success';
        // Trigger auto-sync alass après extraction SRT.
        setTimeout(maybeAutoSubSyncCheck, 100);
      } else {
        appendLog('ℹ Aucun sous-titre FR/ENG texte trouvé dans la référence.');
        srtExtractionResult = 'success'; // Pas d'erreur, juste rien à extraire.
      }
    } catch (e) {
      appendLog('❌ Extraction subs : ' + String(e));
      srtExtractionResult = 'error';
    } finally {
      srtExtracting = false;
      srtPhase = '';
      srtPercent = 0;
    }
  }

  function clearReference() {
    referencePath = '';
    referenceMkvInfo = null;
    srtExtractionResult = '';
    frAudioExtractionResult = '';
    frAudioConvertedSummary = '';
    extractFRVFF = false;
    extractFRVFQ = false;
    extractENG = false;
  }

  // ---- Extraction FR Audio (VFF/VFQ depuis la source de référence) ----
  // Utilise referencePath comme unique source — pas de fichier séparé.
  let extractFRVFF = false;
  // Référence audio pour la sync (Chromaprint + cross-corr) : "fr" (default,
  // matche la VFF/VFi de la source LiHDL) ou "eng" (pour les cas où la VFF
  // de la source n'est pas fiable, ex: source LiHDL re-encodée avec un drift
  // sur l'audio FR mais ENG VO préservée).
  let syncRefLang = 'fr';
  let extractFRVFQ = false;
  let extractENG = false;
  let frAudioExtracting = false;
  let frAudioExtractionResult = ''; // '' | 'success' | 'error'
  let frAudioPhase = ''; // 'sync' | 'convert:CODEC,VARIANT' — phase courante pour le label de la progress bar
  let frAudioConvertPercent = 0; // 0-100 pendant la conversion AC3 (event ac3convert:progress)
  let frAudioConvertedSummary = ''; // "448 kb/s 5.1" ou "192 kb/s 2.0" ou "448 kb/s 5.1 / 192 kb/s 2.0" selon les conversions réelles
  let srtExtracting = false; // true pendant l'extraction SRT auto au pick d'une référence
  let srtExtractionResult = ''; // '' | 'success' | 'error'
  let srtPhase = ''; // '' | 'convert_ass' — phase courante pour le label de la SRT progress bar
  let srtAssConverted = false; // true si au moins un sub ASS a été converti en SRT pendant cette extraction
  let srtPercent = 0; // 0-100 progression de l'extraction SRT (event srtprogress)

  // ---- Vérification sync subs externes vs audio source LiHDL ----
  let subSyncChecking = false;
  let subSyncResults = []; // [{path, offset_ms, raw_offset_ms, confidence, method, error}]
  let subSyncPercent = 0;
  let subSyncCurrentName = '';
  let subSyncAppliedMsg = ''; // "✓ N SRT resyncronisé(s)" persistant après apply

  // Hash des paths SRT externes — pour éviter de relancer alass plusieurs fois
  // sur le même état après auto-trigger.
  let lastSyncedSrtPathsHash = '';
  // Set des paths SRT déjà syncés (un alass terminé = path mémorisé). Évite
  // de re-syncer un SRT qui n'a pas changé quand on en ajoute un nouveau.
  let alreadySyncedPaths = new Set();

  // Déclenchement auto : appelé quand source LiHDL ET au moins un SRT externe
  // sont chargés. Ne refait pas l'analyse si elle a déjà été faite pour les
  // mêmes paths (state hash).
  async function maybeAutoSubSyncCheck() {
    if (!sourcePath) return;
    const srtSubs = externalSubs.filter(s => s.path && /\.(srt|ass|ssa)$/i.test(s.path));
    if (srtSubs.length === 0) return;
    const hash = srtSubs.map(s => s.path).sort().join('|');
    if (hash === lastSyncedSrtPathsHash) return; // déjà fait pour ce set
    if (subSyncChecking) return;
    lastSyncedSrtPathsHash = hash;
    appendLog('🤖 Vérification sync SRT automatique…');
    await runSubSyncCheck();
  }

  async function runSubSyncCheck() {
    if (!sourcePath) { appendLog('⚠ Charge la source LiHDL d\'abord'); return; }
    const srtSubs = externalSubs.filter(s => s.path && /\.(srt|ass|ssa)$/i.test(s.path));
    if (srtSubs.length === 0) { appendLog('ℹ Aucun SRT/ASS/SSA externe à vérifier'); return; }
    // Filtre les SRT déjà syncés (cache des paths déjà passés par alass).
    const newSubs = srtSubs.filter(s => !alreadySyncedPaths.has(s.path));
    const skippedCount = srtSubs.length - newSubs.length;
    if (newSubs.length === 0) {
      appendLog(`ℹ Tous les sous-titres (${srtSubs.length}) ont déjà été vérifiés — skip`);
      return;
    }
    if (skippedCount > 0) {
      appendLog(`↳ ${skippedCount} sous-titre(s) déjà syncé(s) → skip`);
    }
    subSyncChecking = true;
    subSyncResults = [];
    subSyncPercent = 0;
    subSyncCurrentName = '';
    subSyncAppliedMsg = '';
    try {
      appendLog(`🔎 Sync alass de ${newSubs.length} sous-titre(s) vs source…`);
      const reqs = newSubs.map(s => ({ path: s.path, from_reference: !!s.fromReference }));
      subSyncResults = await CheckSubsSync(reqs, sourcePath, referencePath || '', syncRefLang);
      // Marque les paths comme syncés (succès ou échec : on ne re-tentera pas).
      for (const r of (subSyncResults || [])) {
        if (r && r.path) alreadySyncedPaths.add(r.path);
        if (r && r.synced_path) alreadySyncedPaths.add(r.synced_path);
      }
    } catch (e) {
      appendLog('❌ alass sync : ' + String(e));
    } finally {
      subSyncChecking = false;
      subSyncPercent = 0;
      subSyncCurrentName = '';
    }
  }

  function applySubSyncResults() {
    let applied = 0;
    for (const r of subSyncResults) {
      if (!r || r.error) continue;
      if (!r.synced_path) continue;
      const idx = externalSubs.findIndex(s => s.path === r.path);
      if (idx >= 0) {
        // Le SRT corrigé par alass remplace le SRT original. Reset delayMs/tempoFactor
        // car les corrections sont déjà BAKED dans le fichier corrigé.
        externalSubs[idx].path = r.synced_path;
        externalSubs[idx].name = basename(r.synced_path);
        externalSubs[idx].delayMs = 0;
        externalSubs[idx].tempoFactor = 1.0;
        applied++;
      }
    }
    externalSubs = [...externalSubs];
    appendLog(`✓ ${applied} sous-titre(s) remplacé(s) par version corrigée alass`);
    subSyncAppliedMsg = `✓ ${applied} sous-titre(s) resynchronisé(s) via alass`;
    subSyncResults = [];
  }

  function dismissSubSyncResults() {
    subSyncResults = [];
    subSyncAppliedMsg = '';
  }

  async function runFRAudioExtraction() {
    if (!referencePath) { appendLog('⚠ Choisis d\'abord une source de référence'); return; }
    if (!extractFRVFF && !extractFRVFQ && !extractENG) { appendLog('⚠ Coche au moins VFF, VFQ ou ENG'); return; }
    if (!sourcePath) { appendLog('⚠ Charge la source LiHDL d\'abord (pour la sync)'); return; }
    frAudioExtracting = true;
    frAudioExtractionResult = '';
    frAudioConvertPercent = 0;
    frAudioConvertedSummary = '';
    try {
      const variants = [extractFRVFF && 'VFF', extractFRVFQ && 'VFQ', extractENG && 'ENG'].filter(Boolean).join(' + ');
      appendLog(`⏳ Extraction ${variants} + détection sync…`);
      const extractionsRaw = await ExtractFRAudios(referencePath, !!extractFRVFF, !!extractFRVFQ, !!extractENG, sourcePath, syncRefLang);
      if (!extractionsRaw || extractionsRaw.length === 0) {
        appendLog('ℹ Aucune piste correspondante trouvée dans la source FR audio.');
        return;
      }
      // Tri norme LiHDL : FR VFF (ou VFi/VOF) en 1er, puis FR VFQ, puis ENG VO.
      const extractions = [...extractionsRaw].sort((a, b) => {
        const rank = (v) => {
          if (v === 'VFF' || v === 'VFi' || v === 'VOF') return 0;
          if (v === 'VFQ') return 1;
          if (v === 'ENG') return 4;
          return 9;
        };
        return rank(a.variant) - rank(b.variant);
      });
      // Drop des pistes FR/ENG/autres VO internes correspondant aux variantes extraites.
      // Si l'utilisateur extrait ENG VO, on supprime aussi toutes les autres pistes VO
      // (JPN VO, ITA VO, etc.) qui deviennent superflues — on garde uniquement la ENG VO extraite.
      tracks = tracks.map(t => {
        if (t.type !== 'audio') return t;
        const isVFForVFi = /^FR (VFF|VFi|VOF) /.test(t.label || '');
        const isVFQ = /^FR VFQ /.test(t.label || '');
        const isFR = /^FR /.test(t.label || '');
        if (extractFRVFF && isVFForVFi) return { ...t, keep: false };
        if (extractFRVFQ && isVFQ) return { ...t, keep: false };
        if (extractENG && !isFR) return { ...t, keep: false }; // drop tous les non-FR (ENG, JPN, ITA, etc.)
        return { ...t, default: false };
      });
      // Ajout brut des extractions ; l'ordre + default seront recomputés
      // globalement après la boucle pour garantir : FR VFF → FR VFQ → FR AD → reste.
      let isFirstExtracted = true;
      for (const ex of extractions) {
        // Construction directe du label LiHDL depuis variant + codec + channels +
        // détection Atmos (mediainfo). On ne passe PAS par inferAudioLabel car
        // sa logique de hints (track_name) n'est pas fiable sur une piste
        // extraite (elle perd son contexte mkv).
        const ch = formatChannels(ex.channels);
        const atmosFields = [
          String(ex.track_name || ''),
          String(ex.mi_title || ''),
          String(ex.mi_format_profile || ''),
          String(ex.mi_format_commercial || ''),
          String(ex.mi_format_commercial_if_any || ''),
          String(ex.mi_format_features || ''),
        ].join(' ').toUpperCase();
        const isAtmos = (atmosFields.includes('ATMOS') || atmosFields.includes('JOC')) && ex.codec === 'EAC3' && ch === '5.1';
        const atmosSuffix = isAtmos ? ' ATMOS' : '';
        // Norme LiHDL : "FR <variant>" pour FR (VFF/VFQ/VFi/VOF), "ENG VO" pour anglais.
        const labelPrefix = ex.variant === 'ENG' ? 'ENG VO' : `FR ${ex.variant}`;
        const label = `${labelPrefix} : ${ex.codec} ${ch}${atmosSuffix}`;
        // Nom lisible (ex: "FR.VFF.AC3.5.1.ac3", "ENG.VO.AC3.5.1.ac3")
        const namePrefix = ex.variant === 'ENG' ? 'ENG.VO' : `FR.${ex.variant}`;
        const friendlyName = `${namePrefix}${atmosSuffix ? '.ATMOS' : ''}.${ex.codec}.${ch}.${ex.codec.toLowerCase()}`.replace(/\s+/g, '');
        externalAudios = [...externalAudios, {
          path: ex.path,
          name: friendlyName,
          label,
          keep: true,
          default: false, // sera recalculé en bloc
          forced: false,
          delayMs: ex.delay_ms || 0,
          tempoFactor: ex.tempo_factor || 1.0,
          order: 0, // sera recalculé
        }];
        isFirstExtracted = false;
        const tempoStr = ex.tempo_factor && ex.tempo_factor !== 1.0 ? `, tempo ${ex.tempo_factor.toFixed(6)}` : '';
        const syncMsg = (ex.delay_ms !== 0 || (ex.tempo_factor && ex.tempo_factor !== 1.0))
          ? ` (offset ${ex.delay_ms} ms${tempoStr}, conf ${(ex.confidence || 0).toFixed(2)}, ${ex.method})`
          : ' (parfaitement synchro)';
        appendLog(`✓ FR ${ex.variant} ajoutée → ${label}${syncMsg}`);
      }
      // applyLihdlTrackOrder gère maintenant internes + externes ensemble
      // selon la priorité LiHDL (FR VFF → FR VFQ → FR AD → reste).
      applyLihdlTrackOrder();

      // Résumé des conversions effectuées (uniquement les pistes réellement
      // converties par ffmpeg ; les pistes AC3 source extraites lossless
      // n'apparaissent pas dans le résumé).
      const convertedBitrates = new Set();
      for (const ex of extractions) {
        if (!ex.was_converted) continue;
        const ch = formatChannels(ex.channels);
        const kbps = ex.bitrate_kbps;
        if (kbps && ch) convertedBitrates.add(`${kbps} kb/s ${ch}`);
      }
      frAudioConvertedSummary = [...convertedBitrates].join(' / ');

      frAudioExtractionResult = 'success';
    } catch (e) {
      appendLog('❌ Extraction FR audio : ' + String(e));
      frAudioExtractionResult = 'error';
    } finally {
      frAudioExtracting = false;
      frAudioPhase = '';
      frAudioConvertPercent = 0;
    }
  }

  // Fallback de formatage canaux si inferAudioLabel ne sait pas (ex: extracted file).
  function formatChannels(ch) {
    if (ch === 1) return '1.0';
    if (ch === 2) return '2.0';
    if (ch === 6) return '5.1';
    if (ch === 8) return '7.1';
    return ch ? `${ch}ch` : '';
  }

  // Compare 2 MkvBasicInfo et retourne {durationOK, fpsOK} (tolérance 1s + 0.05fps)
  function checkCompat(a, b) {
    if (!a || !b) return { durationOK: null, fpsOK: null };
    const durationOK = Math.abs((a.duration_seconds || 0) - (b.duration_seconds || 0)) <= 2;
    const fpsOK = Math.abs((a.framerate || 0) - (b.framerate || 0)) <= 0.05;
    return { durationOK, fpsOK };
  }

  // Réinitialiser tous les états de la session courante (source, secondaire,
  // pistes, sous-titres externes, TMDB, status). Ne supprime pas les fichiers.
  function resetAll() {
    sourcePath = '';
    sourceInfo = null;
    tracks = [];
    secondaryPath = '';
    secondaryTracks = [];
    secondarySelected = [];
    externalSubs = [];
    externalAudios = [];
    lastTmdbResult = null;
    tmdbValidated = false;
    tmdbResults = [];
    tmdbQuery = '';
    tmdbIdQuery = '';
    target = { ...target, title: '', year: '', episode: '', flagOverride: 'auto' };
    filenameOverride = false;
    manualFilename = '';
    useVFi = true;
    referencePath = '';
    sourceMkvInfo = null;
    referenceMkvInfo = null;
    showReferenceBar = false;
    extractFRVFF = false;
    extractFRVFQ = false;
    extractENG = false;
    frAudioExtracting = false;
    frAudioExtractionResult = '';
    frAudioConvertedSummary = '';
    srtExtracting = false;
    srtExtractionResult = '';
    srtAssConverted = false;
    lastSyncedSrtPathsHash = '';
    subSyncResults = [];
    subSyncAppliedMsg = '';
    hydrackerURL = '';
    autoMuxStatus = '';
    if (autoMuxStatusTimer) { clearTimeout(autoMuxStatusTimer); autoMuxStatusTimer = null; }
    // Reset état OCR (barre + résultat + à vérifier).
    ocrRunning = false;
    ocrTrackId = -1;
    ocrProgress = { status: '', percent: 0, message: '',
      total_lines: 0, corrected_lines: 0, suspicious_lines: 0, quality_score: 0, subtitles: 0,
      lt_total_issues: 0, lt_auto_fixed: 0, lt_needs_review: 0, lt_review_list: [] };
    appendLog('↻ Réinitialisé');
  }

  // Auto-reset après mux réussi : envoie source + sous-titres + secondaire à
  // la corbeille (réversible), puis reset l'état.
  async function autoResetAfterMux(preserveAutoMuxStatus = false) {
    // Vide tout le contenu du dossier "LiHDL en cours" (norme : tous les fichiers
    // source/référence/SUPPLY/subs externes y vivent, et après un mux réussi on
    // peut tout dégager). Inclut sous-dossiers, exclut .DS_Store.
    const lihdlEnCoursDir = '/Users/gandalf/Downloads/LiHDL en cours';
    try {
      const n = await MoveDirContentsToTrash(lihdlEnCoursDir);
      if (n > 0) {
        appendLog(`🗑 ${n} élément(s) du dossier "LiHDL en cours" envoyé(s) à la corbeille`);
      }
    } catch (e) {
      appendLog('⚠ vidage "LiHDL en cours" : ' + String(e));
    }
    // Reset UI désactivé après mux : on garde l'état + le log visibles pour
    // que l'utilisateur puisse copier les logs et inspecter le résultat.
    // Il cliquera ↻ RESET manuellement quand il aura fini.
    appendLog('ℹ État conservé après mux — clique ↻ RESET en haut quand tu veux continuer');
    void preserveAutoMuxStatus; // arg gardé pour compat caller
  }
  let filenameCopied = false;
  let filenameCopiedTimer = null;
  let filenameOverride = false;   // true = l'utilisateur a pris la main sur le nom
  let manualFilename = '';        // valeur saisie manuellement quand override actif

  // Le nom utilisé pour le mux : manuel si override, sinon généré.
  $: effectiveFilename = filenameOverride ? manualFilename : previewFilename;

  function startFilenameOverride() {
    manualFilename = previewFilename;
    filenameOverride = true;
  }

  function resetFilenameOverride() {
    filenameOverride = false;
    manualFilename = '';
  }

  // Queue batch : liste de .mkv en attente de mux.
  let queue = [];
  let bottomPaneTab = 'journal'; // 'journal' | 'queue' — tab actif dans la card secondaire en bas droite

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
    if (!effectiveFilename) return;
    try {
      await navigator.clipboard.writeText(effectiveFilename);
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

  // ─── Onglet Synchro Audios ────────────────────────────────────────────────
  // État indépendant du workflow LiHDL/PSA. Charge un .mkv, affiche ses
  // pistes audio, permet de désigner une piste de référence et d'appliquer
  // un décalage (ms) aux autres. Le mux résultant copie l'audio bit-à-bit
  // (mkvmerge --sync TID:offset, pas de réencodage).
  let syncSourcePath = '';
  let syncAudioTracks = [];   // [{id, codec, language, name, channels}]
  let syncRefId = null;       // track ID désigné comme référence
  let syncOffsets = {};       // { trackId: delayMs }
  let syncRunning = false;
  let syncPercent = 0;
  let syncDetecting = {};   // { trackId: true } pendant la détection
  let syncResults = {};     // { trackId: {offset_ms, confidence, drift_ms, method, notes} }

  async function syncDetectOne(trackId) {
    if (!syncSourcePath || syncRefId === null || trackId === syncRefId) return;
    syncDetecting = { ...syncDetecting, [trackId]: true };
    try {
      const r = await DetectAudioOffset(syncSourcePath, syncRefId, trackId);
      syncResults = { ...syncResults, [trackId]: r };
      syncOffsets = { ...syncOffsets, [trackId]: r.offset_ms };
    } catch (e) {
      appendLog('❌ Détection piste ' + trackId + ' : ' + String(e));
    } finally {
      syncDetecting = { ...syncDetecting, [trackId]: false };
    }
  }

  async function syncDetectAll() {
    for (const t of syncAudioTracks) {
      if (t.id === syncRefId) continue;
      await syncDetectOne(t.id);
    }
  }

  async function syncPickFile() {
    try {
      const p = await SelectMkvFile();
      if (!p) return;
      syncSourcePath = p;
      syncAudioTracks = [];
      syncRefId = null;
      syncOffsets = {};
      const list = await ListAudioTracksForSync(p);
      syncAudioTracks = list || [];
      // sélectionne par défaut la 1ère piste comme référence
      if (syncAudioTracks.length > 0) syncRefId = syncAudioTracks[0].id;
      appendLog(`📂 Synchro : ${syncAudioTracks.length} piste(s) audio chargée(s)`);
    } catch (e) {
      appendLog('❌ Synchro : ' + String(e));
    }
  }

  function syncSetRef(id) {
    syncRefId = id;
    // Une piste référence n'a jamais d'offset (c'est elle qu'on cale les autres dessus).
    if (syncOffsets[id]) { delete syncOffsets[id]; syncOffsets = { ...syncOffsets }; }
  }

  function syncDefaultOutput() {
    if (!syncSourcePath) return '';
    const i = syncSourcePath.lastIndexOf('.');
    const base = i > 0 ? syncSourcePath.slice(0, i) : syncSourcePath;
    return base + '.sync.mkv';
  }

  async function syncApply() {
    if (!syncSourcePath || syncRefId === null) return;
    const offsets = [];
    for (const t of syncAudioTracks) {
      if (t.id === syncRefId) continue;
      const d = parseInt(syncOffsets[t.id] || 0, 10);
      const r = syncResults[t.id];
      const tempo = (r && r.tempo_factor && r.tempo_factor !== 1.0) ? r.tempo_factor : 1.0;
      // On envoie la piste si offset OU tempo nécessite une action.
      if ((Number.isFinite(d) && d !== 0) || tempo !== 1.0) {
        offsets.push({ track_id: t.id, delay_ms: d || 0, tempo_factor: tempo });
      }
    }
    if (offsets.length === 0) {
      appendLog('ℹ Aucun décalage saisi — rien à recaler');
      return;
    }
    syncRunning = true;
    syncPercent = 0;
    try {
      await MuxAudioSync({
        input_path: syncSourcePath,
        output_path: syncDefaultOutput(),
        offsets,
      });
    } catch (e) {
      appendLog('❌ Recalage : ' + String(e));
    }
  }
  let logEl;

  // Subs externes ajoutés (srt/sup/ass/ssa/sub/idx). Chaque entrée a le même
  // format qu'une piste interne pour l'UI unifiée, mais avec un flag external.
  let externalSubs = [];
  let externalAudios = [];

  // OCR PGS → SRT : état réactif partagé entre toutes les pistes (un seul OCR
  // à la fois pour ne pas saturer Tesseract). ocrTrackId = id de la piste en
  // cours (-1 si aucun). ocrProgress = { status, percent, message }.
  let ocrRunning = false;
  let ocrTrackId = -1;
  let ocrProgress = {
    status: '', percent: 0, message: '',
    total_lines: 0, corrected_lines: 0, suspicious_lines: 0, quality_score: 0, subtitles: 0,
    lt_total_issues: 0, lt_auto_fixed: 0, lt_needs_review: 0, lt_review_list: [],
  };
  // Garde la barre OCR visible quelques secondes après "done" pour afficher le score.
  let ocrLastResultAt = 0;
  // Modal "lignes à vérifier" (LanguageTool) — toggle simple.
  let showLTReview = false;
  // États par-match : pour chaque match (index dans lt_review_list) on stocke
  // `resolved` (✓ après patch ou ignore) et `customText` (champ d'édition libre).
  let ltReviewState = []; // [{resolved, ignored, customText, busy, error}]
  // Path du SRT actuellement affiché dans le modal review (pour ApplyOCRFix).
  let ocrCurrentSRT = '';

  // OpenSubtitles modal de recherche.
  let showOSModal = false;
  let osQuery = '';
  let osYear = '';
  let osLang = 'fr,en';
  let osResults = [];
  let osSearching = false;
  let osError = '';
  // Quand le modal a été ouvert depuis la vue post-source : on ajoute le SRT
  // résultant aux externalSubs. Sinon (page d'accueil), on demande un dossier
  // de destination.
  let osContext = 'standalone'; // 'standalone' | 'post-source'
  let osDownloading = '';

  // Dictionnaire custom OCR — modal Settings.
  let customDictEntries = [];
  let showAddDictModal = false;
  let newDictWrong = '';
  let newDictRight = '';
  let dictBusy = false;

  // Recharge la liste du dico OCR quand on entre dans l'écran Réglages.
  $: if (screen === 'reglages' && typeof window !== 'undefined') { loadCustomDictReactive(); }
  let _lastDictLoadAt = 0;
  function loadCustomDictReactive() {
    if (Date.now() - _lastDictLoadAt < 500) return;
    _lastDictLoadAt = Date.now();
    loadCustomDict();
  }

  // Détecte si une piste sub interne est en codec PGS (Blu-ray image-based).
  // Utilisé pour afficher le bouton "🔠 OCR" uniquement sur ces pistes.
  function isPGSTrack(t) {
    if (!t || t.type !== 'subtitles') return false;
    const c = ((t.codecId || '') + ' ' + (t.codec || '')).toUpperCase();
    return c.includes('PGS') || c.includes('HDMV');
  }

  // Vrai si le sub externe est un PGS (.sup) — détectable par extension.
  function isPGSExternal(s) {
    return !!(s && s.path && /\.sup$/i.test(s.path));
  }
  // Devine la lang Tesseract à partir du nom de fichier d'un .sup externe.
  function ocrLangFromExternal(s) {
    const n = String((s && s.name) || (s && s.path) || '').toLowerCase();
    if (/\b(fre|fra|fr|french)\b/.test(n)) return 'fra';
    if (/\b(eng|en|english)\b/.test(n)) return 'eng';
    if (/\b(deu|ger|de|german)\b/.test(n)) return 'deu';
    if (/\b(spa|es|spanish)\b/.test(n)) return 'spa';
    if (/\b(ita|it|italian)\b/.test(n)) return 'ita';
    return 'fra';
  }

  // Mappe un code langue piste (fre/fra/fr/eng/en) vers le code Tesseract
  // attendu par pgsrip (fra/eng/deu/spa/ita…).
  function ocrLangFromTrack(t) {
    const l = String(t.lang || '').toLowerCase();
    if (l === 'fre' || l === 'fra' || l === 'fr') return 'fra';
    if (l === 'eng' || l === 'en') return 'eng';
    if (l === 'deu' || l === 'ger' || l === 'de') return 'deu';
    if (l === 'spa' || l === 'es') return 'spa';
    if (l === 'ita' || l === 'it') return 'ita';
    if (l === 'por' || l === 'pt') return 'por';
    if (l === 'nld' || l === 'dut' || l === 'nl') return 'nld';
    return 'fra'; // défaut FR (cas LiHDL principal)
  }

  // Lance l'OCR PGS → SRT sur une piste interne. Le SRT final est ajouté
  // automatiquement aux externalSubs (avec label suggéré FR/ENG Full ou Forced).
  async function runOCR(track) {
    if (ocrRunning) return;
    if (!sourcePath) {
      appendLog('❌ OCR : aucun .mkv source chargé');
      return;
    }
    const lang = ocrLangFromTrack(track);
    ocrRunning = true;
    ocrTrackId = track.id;
    ocrProgress = { status: 'init', percent: 0, message: '' };
    appendLog(`🔠 OCR PGS → SRT (piste #${track.id}, lang=${lang}) — peut prendre plusieurs minutes…`);
    try {
      const srtPath = await OCRPGSTrack(sourcePath, track.id, lang);
      if (srtPath) {
        // Suggère un label cohérent : forced si la piste source était forced,
        // sinon Full. Préfixe FR/ENG selon la lang.
        const prefix = (lang === 'fra') ? 'FR' : (lang === 'eng' ? 'ENG' : '');
        const kind = track.forced ? 'Forced' : 'Full';
        const label = prefix ? `${prefix} ${kind} : SRT` : '';
        let size = -1;
        try { size = await FileSize(srtPath); } catch {}
        let maxOrder = 0;
        for (const t of tracks) maxOrder = Math.max(maxOrder, t.order ?? 0);
        for (const s of externalSubs) maxOrder = Math.max(maxOrder, s.order ?? 0);
        externalSubs = [...externalSubs, {
          path: srtPath,
          name: basename(srtPath),
          size,
          keep: true,
          default: false,
          forced: !!track.forced,
          label,
          order: maxOrder + 10,
        }];
        appendLog('✓ OCR PGS → SRT : ' + srtPath);
        applySubForcedRule();
      }
    } catch (e) {
      const msg = String(e && e.message ? e.message : e);
      appendLog('❌ OCR : ' + msg);
      // Hint d'install si binaire absent.
      if (/tesseract/i.test(msg) || /pgsrip/i.test(msg)) {
        appendLog('ℹ Installer : `brew install tesseract tesseract-lang` puis `pip3 install pgsrip`');
      }
    } finally {
      ocrRunning = false;
      // Garde le score visible 8s après done (ou erreur).
      ocrLastResultAt = Date.now();
      const myStamp = ocrLastResultAt;
      setTimeout(() => {
        if (ocrLastResultAt === myStamp && !ocrRunning) {
          ocrTrackId = -1;
          ocrProgress = { status: '', percent: 0, message: '',
            total_lines: 0, corrected_lines: 0, suspicious_lines: 0, quality_score: 0, subtitles: 0,
            lt_total_issues: 0, lt_auto_fixed: 0, lt_needs_review: 0, lt_review_list: [] };
        }
      }, 8000);
    }
  }

  // Outil autonome : ouvre un picker .sup et lance l'OCR sur le ou les fichiers
  // sélectionnés. Sortie : .srt à côté du .sup. N'ajoute rien aux subs externes
  // (utilisé hors workflow mux normal).
  async function pickAndOCRStandaloneSup() {
    if (ocrRunning) return;
    let paths;
    try { paths = await SelectSupFiles(); } catch { paths = null; }
    if (!paths || !paths.length) return;
    for (const p of paths) {
      const lang = ocrLangFromExternal({ name: p, path: p });
      ocrRunning = true;
      ocrTrackId = -2000; // marqueur "outil autonome"
      ocrProgress = { status: 'init', percent: 0, message: '',
        total_lines: 0, corrected_lines: 0, suspicious_lines: 0, quality_score: 0, subtitles: 0 };
      appendLog(`🔠 OCR PGS .sup → SRT (autonome, ${p.split('/').pop()}, lang=${lang})…`);
      try {
        const srtPath = await OCRSupFile(p, lang);
        if (srtPath) appendLog('✓ OCR PGS .sup → SRT : ' + srtPath);
      } catch (e) {
        const msg = String(e && e.message ? e.message : e);
        appendLog('❌ OCR : ' + msg);
        if (/tesseract/i.test(msg) || /pgsrip/i.test(msg)) {
          appendLog('ℹ Installer : `brew install tesseract tesseract-lang` puis `pip3 install pgsrip`');
        }
      } finally {
        ocrRunning = false;
        // Sur la page d'accueil (outil autonome), on GARDE le résultat affiché
        // jusqu'au prochain OCR — pas de timer auto-reset.
      }
    }
  }

  // Lance l'OCR sur un sub PGS externe (fichier .sup ajouté manuellement).
  // Remplace le .sup par le .srt généré dans la liste externalSubs.
  async function runOCRExternalSup(idx) {
    if (ocrRunning) return;
    const ext = externalSubs[idx];
    if (!ext || !ext.path) { appendLog('❌ OCR : sub externe introuvable'); return; }
    const lang = ocrLangFromExternal(ext);
    ocrRunning = true;
    ocrTrackId = -1000 - idx; // marqueur pour le markup (id négatif unique)
    ocrProgress = { status: 'init', percent: 0, message: '' };
    appendLog(`🔠 OCR PGS .sup → SRT (${ext.name}, lang=${lang}) — peut prendre plusieurs minutes…`);
    try {
      const srtPath = await OCRSupFile(ext.path, lang);
      if (srtPath) {
        const prefix = (lang === 'fra') ? 'FR' : (lang === 'eng' ? 'ENG' : '');
        const kind = ext.forced ? 'Forced' : 'Full';
        const label = prefix ? `${prefix} ${kind} : SRT` : ext.label;
        let size = -1;
        try { size = await FileSize(srtPath); } catch {}
        // Remplace l'entrée .sup par le .srt généré.
        externalSubs = externalSubs.map((s, i) => i === idx ? {
          ...s,
          path: srtPath,
          name: basename(srtPath),
          size,
          label: label || s.label,
        } : s);
        appendLog('✓ OCR PGS .sup → SRT : ' + srtPath);
        applySubForcedRule();
      }
    } catch (e) {
      const msg = String(e && e.message ? e.message : e);
      appendLog('❌ OCR : ' + msg);
      if (/tesseract/i.test(msg) || /pgsrip/i.test(msg)) {
        appendLog('ℹ Installer : `brew install tesseract tesseract-lang` puis `pip3 install pgsrip`');
      }
    } finally {
      ocrRunning = false;
      // Garde le score visible 8s après done (ou erreur).
      ocrLastResultAt = Date.now();
      const myStamp = ocrLastResultAt;
      setTimeout(() => {
        if (ocrLastResultAt === myStamp && !ocrRunning) {
          ocrTrackId = -1;
          ocrProgress = { status: '', percent: 0, message: '',
            total_lines: 0, corrected_lines: 0, suspicious_lines: 0, quality_score: 0, subtitles: 0,
            lt_total_issues: 0, lt_auto_fixed: 0, lt_needs_review: 0, lt_review_list: [] };
        }
      }, 8000);
    }
  }

  // Source secondaire (SUPPLY/FW) — récupère uniquement audios + subs.
  let secondaryPath = '';
  let secondaryTracks = [];      // tracks audio + subs analysées
  let secondaryMkvInfo = null;   // { duration_seconds, framerate, … }
  let psaSyncStatus = '';         // '' | 'checking' | 'ok' | 'corrected' | 'error'
  let psaSyncMessage = '';        // texte à afficher (offset, conf, etc.)
  let secondarySelected = [];    // tracks sélectionnées avec label LiHDL

  function inferAudioLabel(track) {
    const lang = String(track.language || '').toLowerCase();
    // On combine track_name (mkvmerge) + mi_title + mi_service_kind_name (mediainfo)
    const hints = [
      String(track.track_name || ''),
      String(track.mi_title || ''),
      String(track.mi_service_kind_name || ''),
    ].join(' ').toLowerCase();
    // Codec : SEUL mkvmerge codec_id + mediainfo Format sont fiables. JAMAIS le
    // track_name (= titre LiHDL/release) qui peut contenir "E-AC3" alors que la
    // vraie piste est AC3 (rebadging suite à rémux).
    const miFormat = String(track.mi_format || '').toUpperCase();
    const codecId = String(track.codec_id || track.codec || '').toUpperCase();
    const miFormatProfile = String(track.mi_format_profile || '').toUpperCase();
    // Channels : priorité mediainfo, fallback mkvmerge.
    const miCh = parseInt(track.mi_channels, 10);
    const mkvCh = Number(track.audio_channels || 0);
    const ch = (isFinite(miCh) && miCh > 0) ? miCh : mkvCh;
    let codec = 'AC3';
    // 1) codec_id mkvmerge est toujours définitif (A_AC3, A_EAC3, etc.).
    if (codecId.includes('A_EAC3')) codec = 'EAC3';
    else if (codecId.includes('A_AC3')) codec = 'AC3';
    else if (codecId.includes('A_DTS')) codec = 'DTS';
    else if (codecId.includes('A_TRUEHD') || codecId.includes('MLP FBA')) codec = 'TrueHD';
    else if (codecId.includes('A_FLAC')) codec = 'FLAC';
    else if (codecId.includes('A_OPUS')) codec = 'Opus';
    else if (codecId.includes('A_AAC')) codec = 'AAC';
    // 2) Sinon fallback mediainfo Format.
    else if (miFormat.includes('E-AC') || miFormat.includes('EAC3')) codec = 'EAC3';
    else if (miFormat.includes('AC-3') || miFormat.includes('AC3')) codec = 'AC3';
    else if (miFormat.includes('DTS')) codec = 'DTS';
    else if (miFormat.includes('TRUEHD') || miFormat.includes('MLP FBA')) codec = 'TrueHD';
    else if (miFormat.includes('FLAC')) codec = 'FLAC';
    else if (miFormat.includes('OPUS')) codec = 'Opus';
    else if (miFormat.includes('AAC')) codec = 'AAC';
    // Channels
    let chans = '5.1';
    if (ch === 1) chans = '1.0';
    else if (ch === 2) chans = '2.0';
    else if (ch === 6) chans = '5.1';
    else if (ch === 8) chans = '7.1';
    // Atmos : on cherche "atmos" ou "JOC" dans tous les champs mediainfo dispo
    // (Format_Profile, Format_Commercial, Format_AdditionalFeatures, etc.)
    const atmosFields = [
      String(track.track_name || ''),
      String(track.mi_title || ''),
      String(track.mi_format_profile || ''),
      String(track.mi_format_commercial || ''),
      String(track.mi_format_commercial_if_any || ''),
      String(track.mi_format_features || ''),
    ].join(' ').toUpperCase();
    const isAtmos = atmosFields.includes('ATMOS') || atmosFields.includes('JOC');
    // Service kind mediainfo : VI = Visual Impaired (audiodescription), HI = Hearing Impaired
    const isAD = /^vi$/i.test(String(track.mi_service_kind || '')) ||
                 /audio.?descrip|\bad\b|vmal|malvoyant|visual.?impair/i.test(hints);
    // Préfixe langue + variante FR
    let prefix = 'ENG VO';
    if (lang === 'fre' || lang === 'fra' || lang === 'fr') {
      if (isAD)                                            prefix = 'FR AD';
      else if (/canad|québ|quebec|vfq/i.test(hints))       prefix = 'FR VFQ';
      else if (/internat|vfi/i.test(hints))                 prefix = 'FR VFi';
      else                                                  prefix = 'FR VFF';
    }
    else if (lang === 'eng' || lang === 'en') prefix = 'ENG VO';
    else if (lang === 'jpn' || lang === 'ja') prefix = 'JPN VO';
    else if (lang === 'ita' || lang === 'it') prefix = 'ITA VO';
    else if (lang === 'spa' || lang === 'es') prefix = 'SPA VO';
    else if (lang === 'ger' || lang === 'de') prefix = 'GER VO';
    else if (lang === 'chi' || lang === 'zho' || lang === 'zh') prefix = 'CHI VO';
    else if (lang === 'rus' || lang === 'ru') prefix = 'RUS VO';
    else if (lang === 'dut' || lang === 'nld' || lang === 'nl') prefix = 'DUT VO';
    // ATMOS : suffixe ajouté pour toute langue dès lors qu'on est en EAC3 5.1
    // ET que mediainfo signale JOC (Atmos) ou que track_name contient "atmos".
    const atmosSuffix = (isAtmos && codec === 'EAC3' && chans === '5.1') ? ' ATMOS' : '';
    return `${prefix} : ${codec} ${chans}${atmosSuffix}`;
  }

  function inferSubLabel(track, _indexAmongSameLang) {
    const lang = String(track.language || '').toLowerCase();
    const hints = [
      String(track.track_name || ''),
      String(track.mi_title || ''),
      String(track.mi_service_kind_name || ''),
    ].join(' ').toLowerCase();
    const codecId = String(track.codec_id || track.codec || '').toUpperCase();
    const forced = !!track.forced_track;
    const isHI = /^hi$/i.test(String(track.mi_service_kind || ''));
    let fmt = 'SRT';
    if (codecId.includes('PGS') || codecId.includes('HDMV')) fmt = 'PGS';
    let prefix = 'ENG';
    if (lang === 'fre' || lang === 'fra' || lang === 'fr') {
      if (/canad|québ|quebec|vfq/i.test(hints)) prefix = 'FR VFQ';
      else if (/vff|france/i.test(hints))       prefix = 'FR VFF';
      else                                       prefix = 'FR';
    } else if (lang === 'eng' || lang === 'en') prefix = 'ENG';
    // Règle simplifiée : Forced > SDH (explicite uniquement) > Full (défaut).
    // SDH ne se déclenche QUE si mediainfo dit ServiceKind=HI OU si le nom
    // de piste contient un mot-clé Sourds/Malentendants explicite.
    let kind = 'Full';
    if (forced || /forc(é|e)/i.test(hints)) {
      kind = 'Forced';
    } else if (isHI || /\bsdh\b|sourds|hearing|malentend/i.test(hints)) {
      kind = 'SDH';
    }
    return `${prefix} ${kind} : ${fmt}`;
  }

  async function pickSecondaryDialog() {
    const p = await SelectMkvFile();
    if (!p) return;
    secondaryPath = p;
    secondaryTracks = [];
    secondarySelected = [];
    secondaryMkvInfo = null;
    psaSyncStatus = ''; // reset
    const filename = p.split('/').pop() || '';
    AnalyzeMkvSecondary(p);
    // FPS + durée pour la card de comparaison.
    GetMkvBasicInfo(p).then(info => { secondaryMkvInfo = info; }).catch(() => {});
    appendLog('🔍 Analyse secondaire : ' + filename);

    // Auto-fill depuis le nom de fichier SUPPLY/FW.
    const supply = parseSupplyInfo(filename);
    const psaName = (sourcePath || '').split('/').pop() || '';
    const psa = parsePsaSourceInfo(psaName);
    // sourceType combiné PSA + SUPPLY (ex: "WEBRip PSA Audio Supply").
    // Whitelist des teams "officielles" affichées dans le nom : SUPPLY, FW,
    // Super U. Pour les autres teams (ex: UNFR), on affiche juste "PSA"
    // sans mentionner la team source.
    if (psa.isPSA && psa.source && supply.team) {
      const validTeams = ['supply', 'fw', 'super u', 'super-u', 'superu'];
      const teamLow = supply.team.toLowerCase();
      if (validTeams.includes(teamLow)) {
        videoChoice.sourceType = `${psa.source} PSA Audio ${supply.team}`;
      } else {
        videoChoice.sourceType = `${psa.source} PSA`;
      }
      appendLog(`✓ sourceType : ${videoChoice.sourceType}`);
    } else if (psa.isPSA && psa.source) {
      videoChoice.sourceType = `${psa.source} PSA`;
    }
    // Lang flag depuis SUPPLY → override automatique.
    if (supply.langFlag) {
      target.flagOverride = supply.langFlag;
      appendLog(`✓ Flag langue : ${supply.langFlag}`);
    }
    // S/E : si pas déjà détecté sur PSA, prends celui de SUPPLY ; sinon valide.
    const supplyEp = detectEpisode(filename);
    if (!target.episode && supplyEp) {
      target.episode = supplyEp;
    } else if (supplyEp && target.episode && supplyEp !== target.episode) {
      appendLog(`⚠ S/E PSA (${target.episode}) ≠ SUPPLY (${supplyEp})`);
    }
  }

  // Vérifie la sync audio entre PSA et SUPPLY/Super U via Chromaprint.
  // Si offset détecté avec confiance > 0.5 et |offset| > 50ms → applique
  // DelayMs sur secondarySelected automatiquement.
  async function checkPSASync() {
    if (!sourcePath || !secondaryPath) return;
    psaSyncStatus = 'checking';
    psaSyncMessage = 'Vérification sync PSA ↔ SUPPLY…';
    try {
      const res = await CheckPSASync(sourcePath, secondaryPath, syncRefLang || 'fr');
      if (res.error) {
        psaSyncStatus = 'error';
        psaSyncMessage = res.error;
        appendLog('⚠ Sync PSA↔SUPPLY : ' + res.error);
        return;
      }
      const off = Math.abs(res.offset_ms || 0);
      const conf = res.confidence || 0;
      if (conf < 0.5) {
        psaSyncStatus = 'error';
        psaSyncMessage = `confiance ${conf.toFixed(2)} trop faible — vérif manuelle requise`;
        appendLog(`⚠ Sync PSA↔SUPPLY : confiance ${conf.toFixed(2)} trop faible`);
        return;
      }
      if (off < 50) {
        psaSyncStatus = 'ok';
        psaSyncMessage = `Synchros (offset ${res.offset_ms} ms · conf ${conf.toFixed(2)})`;
        appendLog(`✓ Sync PSA↔SUPPLY : ${res.offset_ms} ms (synchro, conf ${conf.toFixed(2)})`);
        return;
      }
      // Offset significatif → applique sur les pistes audio secondarySelected.
      // mkvmerge --sync TID:offset (positif retarde, négatif avance).
      secondarySelected = secondarySelected.map(t => {
        if (t.type !== 'audio') return t;
        return { ...t, delayMs: res.offset_ms };
      });
      psaSyncStatus = 'corrected';
      psaSyncMessage = `Décalage ${res.offset_ms} ms corrigé auto (conf ${conf.toFixed(2)})`;
      appendLog(`↻ Sync PSA↔SUPPLY : décalage ${res.offset_ms} ms appliqué sur audios SUPPLY (conf ${conf.toFixed(2)})`);
    } catch (e) {
      psaSyncStatus = 'error';
      psaSyncMessage = String(e);
      appendLog('⚠ Sync PSA↔SUPPLY : ' + String(e));
    }
  }

  function clearSecondary() {
    secondaryPath = '';
    secondaryTracks = [];
    secondarySelected = [];
  }

  function removeSecondaryTrack(idx, type) {
    // idx = position dans la sous-liste filtrée par type (audio ou subtitles)
    const filtered = secondarySelected.filter(t => t.type === type);
    if (idx < 0 || idx >= filtered.length) return;
    const target = filtered[idx];
    secondarySelected = secondarySelected.filter(t => t !== target);
  }

  // ⚡ Automatiser : drop tous audios/subs internes du PSA, prend ceux du SUPPLY,
  // applique les labels LiHDL automatiques, et règle Team=GANDALF + mode série.
  function automate() {
    if (!sourcePath) { appendLog('⚠ Charge d\'abord le fichier PSA'); return; }
    if (!secondaryPath || secondaryTracks.length === 0) {
      appendLog('⚠ Charge un fichier SUPPLY/FW d\'abord');
      return;
    }
    // 1. Drop audios/subs internes du PSA (keep=false).
    tracks = tracks.map(t => (t.type === 'audio' || t.type === 'subtitles') ? { ...t, keep: false } : t);
    // 2. Construit secondarySelected depuis SUPPLY tracks avec auto-labels.
    const seenLangSubs = {};
    let order = 100; // après la vidéo (qui sera order=0..99)
    secondarySelected = secondaryTracks.map(t => {
      let label, language, defaultFlag = false, forcedFlag = !!t.forced_track;
      let keepFlag = true;
      if (t.type === 'audio') {
        label = inferAudioLabel(t);
        // Code langue : VFQ → fr-ca, VFF/VFi/AD → fre, sinon ISO ou langue d'origine.
        if (/\bFR VFQ\b/.test(label))           language = 'fr-ca';
        else if (/^FR /.test(label))             language = 'fre';
        else if (/^ENG /.test(label))            language = 'eng';
        else if (/^JPN /.test(label))            language = 'jpn';
        else if (/^ITA /.test(label))            language = 'ita';
        else if (/^SPA /.test(label))            language = 'spa';
        else if (/^GER /.test(label))            language = 'ger';
        else if (/^CHI /.test(label))            language = 'zho';
        else if (/^RUS /.test(label))            language = 'rus';
        else if (/^DUT /.test(label))            language = 'dut';
        else                                      language = t.language || 'und';
        defaultFlag = false; // 1ère piste FR sera default plus tard
      } else {
        const lang = String(t.language || '').toLowerCase();
        const key = lang || 'und';
        seenLangSubs[key] = (seenLangSubs[key] ?? -1) + 1;
        label = inferSubLabel(t, seenLangSubs[key]);
        if (/\bFR VFQ\b/.test(label))            language = 'fr-ca';
        else if (/^FR /.test(label))             language = 'fre';
        else if (/^ENG /.test(label))            language = 'eng';
        else                                      language = lang || 'und';
        // Norme PSA : on ne garde que les subs FR. Les non-FR sont
        // filtrés (return null) → pas affichés du tout dans la liste.
        const isFRsub = /^FR /.test(label);
        if (!isFRsub) return null;
        const isForcedFR = /^FR( VFF)? Forced\b/.test(label);
        if (isForcedFR) {
          defaultFlag = true;
          forcedFlag = true;
        } else {
          defaultFlag = false;
          forcedFlag = false;
        }
      }
      return {
        id: t.id,
        type: t.type,
        codec: t.codec,
        codec_id: t.codec_id,
        language,
        label,
        keep: keepFlag,
        default: defaultFlag,
        forced: forcedFlag,
        order: order++,
      };
    }).filter(Boolean);
    // Marque la 1ère piste audio FR comme default.
    const firstFr = secondarySelected.find(t => t.type === 'audio' && t.language === 'fre');
    if (firstFr) firstFr.default = true;
    // Cas FASTSUB / VOSTFR : pas de doublage FR, on a la VO + sub FR. Donc :
    // - 1ère audio (typiquement ENG VO) → default
    // - 1er sub FR → default + forced (affiché auto sur la VO)
    const langFlag = String(target.flagOverride || '').toUpperCase();
    const isFastsubVO = /FASTSUB|VOSTFR/.test(langFlag);
    if (isFastsubVO && !firstFr) {
      const firstAudio = secondarySelected.find(t => t.type === 'audio');
      if (firstAudio) firstAudio.default = true;
      const firstFRSub = secondarySelected.find(t => t.type === 'subtitles' && /^FR /.test(t.label || ''));
      if (firstFRSub) {
        firstFRSub.default = true;
        firstFRSub.forced = true;
        appendLog(`🇫🇷 ${langFlag} : audio VO + sub FR marqués default (sub aussi forced)`);
      }
    }

    // Heuristique : si on a 2+ pistes FR audio et qu'une est en 2.0,
    // on la marque automatiquement comme AD (FR AD : <codec> 2.0).
    const frAudios = secondarySelected.filter(t => t.type === 'audio' && /^FR /.test(t.label || ''));
    if (frAudios.length >= 2) {
      for (const t of frAudios) {
        if (/ 2\.0/.test(t.label || '') && !/^FR AD/.test(t.label)) {
          t.label = t.label.replace(/^FR (VFF|VFQ|VFi|VOF) /, 'FR AD ');
        }
      }
    }

    // Norme LiHDL : la piste FR Forced est placée EN PREMIER parmi les subs.
    const subEntries = secondarySelected.filter(t => t.type === 'subtitles');
    if (subEntries.length > 0) {
      const minOrder = Math.min(...subEntries.map(t => t.order ?? 0));
      for (const t of secondarySelected) {
        if (t.type === 'subtitles' && /^FR( VFF)? Forced\b/.test(t.label || '')) {
          t.order = minOrder - 1;
          break;
        }
      }
    }

    // Debug : log de chaque piste audio avec ses indicateurs Atmos depuis mediainfo
    for (const raw of secondaryTracks) {
      if (raw.type !== 'audio') continue;
      appendLog(`[audio] id=${raw.id} lang=${raw.language} name="${raw.track_name||''}" format_profile="${raw.mi_format_profile||''}" features="${raw.mi_format_features||''}" commercial="${raw.mi_format_commercial||''}"`);
    }
    // Plus de 2ᵉ passe sur les subs : par défaut Full, SDH uniquement si
    // mediainfo (ServiceKind=HI) ou track_name explicite (sourds/hearing/SDH).
    secondarySelected = [...secondarySelected];
    // 3. Réglages série + GANDALF.
    videoChoice.team = 'GANDALF';
    if (!target.episode) {
      target.episode = detectEpisode((sourcePath || '').split('/').pop()) || 'S01E01';
    }
    appendLog('⚡ Automatisé : ' + secondarySelected.filter(t=>t.type==='audio').length + ' audio(s) + ' + secondarySelected.filter(t=>t.type==='subtitles').length + ' sub(s) depuis SUPPLY/FW');
  }

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
    if (lbl.startsWith('SPA')) return 'spa';
    if (lbl.startsWith('GER')) return 'ger';
    if (lbl.startsWith('CHI')) return 'zho';
    if (lbl.startsWith('RUS')) return 'rus';
    if (lbl.startsWith('DUT')) return 'dut';
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
    // Combine internes + secondaire — secondaire prioritaire (workflow PSA+SUPPLY)
    const internal = tracks.filter(t => t.type === 'audio' && t.keep);
    const secondary = secondarySelected.filter(t => t.type === 'audio' && t.keep);
    const all = [...secondary, ...internal];
    const firstFR = all.find(t => /\bVF[FQi]\b/.test(t.label || ''));
    const chosen = firstFR || all[0];
    if (!chosen) return '';
    // Match: "FR VFF : EAC3 5.1" ou "ENG VO : EAC3 5.1 ATMOS"
    const m = /: (AC3|EAC3|DTS|TrueHD) (\d\.\d)(?: (ATMOS))?/.exec(chosen.label || '');
    if (!m) return '';
    return m[3] ? `${m[1]}.${m[2]}.${m[3]}` : `${m[1]}.${m[2]}`;
  }

  // Vrai si au moins une piste audio gardée porte un label "FR AD".
  function hasAudioDescription() {
    const internalAD = tracks.some(t => t.type === 'audio' && t.keep && /\bFR AD\b/.test(t.label || ''));
    const secondaryAD = secondarySelected.some(t => t.type === 'audio' && t.keep && /\bFR AD\b/.test(t.label || ''));
    return internalAD || secondaryAD;
  }

  function keptAudioLabels() {
    const internal = tracks.filter(t => t.type === 'audio' && t.keep).map(t => t.label);
    const external = externalAudios.map(a => a.label);
    const secondary = secondarySelected.filter(t => t.type === 'audio' && t.keep).map(t => t.label);
    return [...internal, ...external, ...secondary];
  }

  // --- Calcul client-side du flag langue et du filename (évite les Promises
  //     de Wails en rendu synchrone qui cassaient les tabs). ---

  // Vrai si le mux contient au moins un sous-titre FR (interne kept, externe ou secondaire).
  function hasFrSub() {
    const re = /^FR\b/;
    if (tracks.some(t => t.type === 'subtitles' && t.keep && re.test(t.label || ''))) return true;
    if (externalSubs.some(s => re.test(s.label || ''))) return true;
    if (secondarySelected.some(t => t.type === 'subtitles' && t.keep && re.test(t.label || ''))) return true;
    return false;
  }

  function langFlagClient(labels) {
    let hasVFF = false, hasVFQ = false, hasVFi = false, hasVOF = false, hasVO = false;
    for (const l of labels) {
      if (!l) continue;
      if (/\bVOF\b/.test(l)) hasVOF = true;       // film original français
      else if (/\bVFF\b/.test(l)) hasVFF = true;
      else if (/\bVFQ\b/.test(l)) hasVFQ = true;
      else if (/\bVFi\b/.test(l)) hasVFi = true;
      else if (/\bVO\b/.test(l)) hasVO = true;
    }
    // Film français → flag dédié
    if (hasVOF) return hasVO ? 'MULTi.FRENCH' : 'FRENCH.VOF';
    const vfCount = (hasVFF?1:0) + (hasVFQ?1:0) + (hasVFi?1:0);
    // Si pas de sous-titre FR : préfixe DUAL au lieu de MULTi (norme LiHDL).
    const prefix = hasFrSub() ? 'MULTi' : 'DUAL';
    if (vfCount >= 2) return prefix + '.VF2';
    if (hasVFF && hasVO) return prefix + '.VFF';
    if (hasVFQ && hasVO) return prefix + '.VFQ';
    if (hasVFi && hasVO) return prefix + '.VFi';
    if (hasVFF) return 'VFF';
    if (hasVFQ) return 'VFQ';
    if (hasVFi) return 'VFi';
    if (hasVO)  return 'VO';
    return 'VO';
  }

  // Remplace espaces ET tirets par des points (pas de - dans le titre).
  // Puis compresse les points multiples.
  function dotify(s) {
    // Convertit espaces/tirets en points, capitalise chaque segment.
    // ex: "jurassic world: fallen kingdom" → "Jurassic.World.Fallen.Kingdom"
    const cleaned = String(s || '').trim()
      .replace(/[\s\-]+/g, '.')      // espaces et tirets → points
      .replace(/[^\w.À-ſ]/g, '')  // retire les autres caractères (sauf accents)
      .replace(/\.+/g, '.');           // points consécutifs → un seul
    // Capitalise la 1ère lettre de chaque segment entre points
    return cleaned.split('.').map(seg => {
      if (!seg) return seg;
      return seg.charAt(0).toUpperCase() + seg.slice(1);
    }).join('.');
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
    return m ? normalizeLihdl(m[1]) : '';
  }

  // Normalise un nom de team selon les conventions LiHDL :
  //   - "lihdl" (n'importe quelle casse) → "LiHDL" exactement
  //   - autres teams → MAJUSCULES sauf les "i" qui restent minuscules
  // Ex : "psa"→"PSA", "4Fr"→"4FR", "Alkaline"→"ALKALiNE", "lihdl"→"LiHDL"
  function normalizeLihdl(s) {
    if (!s) return s;
    return String(s).replace(/\b\w+\b/g, (word) => {
      if (!/[A-Za-z]/.test(word)) return word;       // chiffres seuls (années) → laisse
      if (/^lihdl$/i.test(word)) return 'LiHDL';
      return word.toUpperCase().replace(/I/g, 'i');
    });
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
  // Format : {Title}.{Year}.{Flag}[.WiTH.AD].{Resolution}.{Source}.{Audio}.{VideoCodec}-{Team}.mkv
  // Exemple : Le.Comte.De.Monte.Cristo.2024.FRENCH.VOF.WiTH.AD.1080p.WEBRip.AC3.5.1.H264-LiHDL.mkv
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
    // Mode série : SxxExx remplace l'année. Mode film : année.
    if (target.episode) {
      parts.push(target.episode);
    } else {
      const yearMatch = String(target.title || '').match(/\((\d{4})\)\s*$/);
      const year = target.year || (yearMatch ? yearMatch[1] : '');
      if (year) parts.push(year);
    }
    // Flag langue : override manuel (incl. FASTSUB) ou auto-calculé via LangFlag.
    const flag = (target.flagOverride && target.flagOverride !== 'auto')
      ? target.flagOverride
      : langFlagClient(keptAudioLabels());
    if (flag) parts.push(flag);
    // WiTH.AD inséré entre le flag (VOF/FRENCH.VOF/etc.) et la résolution.
    if (hasAudioDescription()) { parts.push('WiTH'); parts.push('AD'); }
    if (target.resolution) parts.push(target.resolution);
    if (target.source) parts.push(target.source);
    const ac = firstAudioCodecForFilename();
    if (ac) parts.push(ac);
    const vc = videoCodecLihdl();
    if (vc) parts.push(vc);
    let name = parts.filter(Boolean).join('.');
    // Team de sortie (LiHDL/GANDALF) : déjà canonique via dropdown.
    if (videoChoice.team) name += '-' + videoChoice.team;
    return name + '.mkv';
  }

  function videoTrackNameClient() {
    // Format LiHDL : "HDLight LiHDL By GANDALF (Source COMPLETE BluRay Alkaline)"
    // Team de l'encodeur (LiHDL/GANDALF) insérée entre la qualité et "By <encoder>".
    const sourceTeam = normalizeLihdl(videoChoice.sourceTeam || '');
    const src = sourceTeam
      ? `${videoChoice.sourceType} ${sourceTeam}`
      : videoChoice.sourceType;
    const team = videoChoice.team || '';
    return `${videoChoice.quality}${team ? ' ' + team : ''} By ${videoChoice.encoder} (Source ${src})`;
  }

  // Réactivité : on référence chaque dépendance pour que Svelte détecte
  // les changements et recalcule (sinon il n'analyse pas l'intérieur des fns).
  $: previewFilename = (function() {
    const _deps = [tracks.length, videoChoice.team, target.title, target.year,
                   target.resolution, target.source, target.video_codec, target.lang,
                   target.episode, target.flagOverride,
                   secondarySelected.length,
                   ...tracks.map(t => (t.keep ? '1' : '0') + (t.label || '')),
                   ...secondarySelected.map(t => (t.keep ? '1' : '0') + (t.label || ''))];
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
    // Reset validation TMDB : nouvelle source = nouvelle confirmation à demander.
    tmdbValidated = false;
    // Reset le hash sync : nouvelle source = nouveau check à faire.
    lastSyncedSrtPathsHash = '';
    // Si des SRT externes sont déjà chargés, déclenche la vérif après init.
    setTimeout(maybeAutoSubSyncCheck, 1500);
    const filename = path.split('/').pop() || '';
    // Auto-fill la team de la source depuis le nom de fichier.
    const st = extractSourceTeam(path);
    if (st) videoChoice.sourceTeam = st;
    target.video_codec = ''; // reset pour que l'auto-suggest se réapplique
    // Auto-detect SxxExx → mode série
    target.episode = detectEpisode(filename);
    // Mode PSA : auto-fill complet depuis le nom du fichier PSA.
    if (muxMode === 'psa') {
      const psa = parsePsaSourceInfo(filename);
      if (psa.isPSA) {
        videoChoice.quality = 'Custom PSA';
        videoChoice.encoder = 'GANDALF';
        videoChoice.team = 'GANDALF';
        videoChoice.sourceTeam = ''; // norme : pas de team de source pour PSA
        if (psa.source) {
          target.source = psa.source;
          videoChoice.sourceType = psa.source + ' PSA';
        }
        if (psa.videoCodec) {
          // Norme PSA team GANDALF : codec en x264/x265 (jamais H264/H265).
          let vc = psa.videoCodec;
          if (vc === 'H265') vc = 'x265';
          else if (vc === 'H264') vc = 'x264';
          target.video_codec = vc;
        }
        appendLog(`✓ PSA détecté : ${psa.source || '?'} ${target.video_codec || '?'} → Custom PSA / GANDALF`);
      }
    } else if (muxMode === 'lihdl') {
      // Mode LiHDL : auto-detect du sourceType depuis le nom (REMUX CUSTOM > REMUX > BluRay…)
      const detected = detectSourceType(filename);
      if (detected) {
        videoChoice.sourceType = detected;
        // Map sourceType → target.source + videoChoice.quality (norme LiHDL) :
        //   WEB / WEB-DL / WEBRip / WEB CUSTOM → WEBRip
        //   BluRay / REMUX / REMUX CUSTOM / COMPLETE BluRay → HDLight
        const q = qualityFromSourceType(detected);
        if (q) {
          target.source = q;
          videoChoice.quality = q;
        }
        appendLog(`✓ Source détectée : ${detected} → Qualité = ${videoChoice.quality}`);
      }
    }
    AnalyzeMkv(path); // fire-and-forget, résultat via event 'analyze:result'
    // Récupère duration + FPS pour la card de comparaison (mode LiHDL)
    sourceMkvInfo = null;
    GetMkvBasicInfo(path).then(info => { sourceMkvInfo = info; }).catch(() => {});
  }

  // Mapping norme LiHDL : sourceType → quality (qui sert aussi pour target.source).
  //   WEB / WEB-DL / WEBRip / WEB CUSTOM / WEB-DL CUSTOM → WEBRip
  //   BluRay / REMUX / REMUX CUSTOM / COMPLETE BluRay   → HDLight
  function qualityFromSourceType(sourceType) {
    const u = (sourceType || '').toUpperCase();
    if (u.includes('WEB')) return 'WEBRip';
    if (u.includes('REMUX') || u.includes('BLURAY')) return 'HDLight';
    return '';
  }

  // Handler : à chaque changement manuel du dropdown "Type source", aligne
  // automatiquement la Qualité + target.source selon la norme LiHDL.
  function onSourceTypeChange() {
    if (muxMode !== 'lihdl') return;
    const q = qualityFromSourceType(videoChoice.sourceType);
    if (q) {
      videoChoice.quality = q;
      target.source = q;
    }
  }

  // Priorité LiHDL pour l'ordre des pistes audio. Plus bas = plus haut dans le mux.
  //   FR VFi/VFF/VOF : 0 (en tête)
  //   FR VFQ          : 1
  //   FR AD           : 2
  //   FR (autre)      : 3
  //   ENG VO          : 4
  //   autres langues  : 10
  function audioLabelPriority(label) {
    const l = (label || '').toUpperCase();
    if (l.startsWith('FR VFI') || l.startsWith('FR VFF') || l.startsWith('FR VOF')) return 0;
    if (l.startsWith('FR VFQ')) return 1;
    if (l.startsWith('FR AD')) return 2;
    if (l.startsWith('FR ')) return 3;
    if (l.startsWith('ENG ')) return 4;
    return 10;
  }

  // Priorité LiHDL pour l'ordre des sous-titres. Plus bas = plus haut dans le mux.
  // Ordre : Langue (FR < ENG < autre) → Variante (sans < VFF/VFi < VFQ) → Type (Forced < Full < SDH).
  // Format SRT prioritaire sur PGS au sein d'un même bucket.
  function subLabelPriority(label) {
    const l = (label || '').toUpperCase();
    let langScore = 10000;
    if (l.startsWith('FR ')) langScore = 0;
    else if (l.startsWith('ENG ')) langScore = 1000;
    let variantScore = 0;
    if (l.startsWith('FR VFF ') || l.startsWith('FR VFI ')) variantScore = 100;
    else if (l.startsWith('FR VFQ ')) variantScore = 200;
    let kindScore = 50;
    if (/\bFORCED\b/.test(l)) kindScore = 0;
    else if (/\bFULL\b/.test(l)) kindScore = 10;
    else if (/\bSDH\b/.test(l)) kindScore = 20;
    let formatScore = 0;
    if (/PGS/.test(l)) formatScore = 1;
    return langScore + variantScore + kindScore + formatScore;
  }

  // Réassigne tous les `order` (internes + externes) selon la norme LiHDL :
  //   1. Vidéo (toujours en tête)
  //   2. Audios mélangés par priorité LiHDL (FR VFF/VFi/VOF → FR VFQ → FR AD →
  //      FR autre → ENG → autres). Les externes (extractions VFF/VFQ) sont
  //      interleavés avec les internes kept selon leur label.
  //   3. Subs internes kept (ordre source) — les externes gardent leurs propres orders.
  // Pose aussi default=true sur la 1ère piste audio (toutes sources confondues),
  // false sur les autres (internes kept ET externes).
  function applyLihdlTrackOrder() {
    let order = 0;
    // 1. Vidéo (interne)
    for (const t of tracks) {
      if (t.type === 'video') {
        t.order = order;
        order += 10;
      }
    }
    // 2. Audios : interne kept + externes, triés par priorité LiHDL.
    const allAudios = [
      ...tracks.filter(t => t.type === 'audio' && t.keep),
      ...externalAudios,
    ];
    allAudios.sort((a, b) => audioLabelPriority(a.label) - audioLabelPriority(b.label));
    let firstAudioFound = false;
    for (const a of allAudios) {
      a.order = order;
      a.default = !firstAudioFound;
      firstAudioFound = true;
      order += 10;
    }
    // 2bis. Audios internes non-kept : reset default flag (ne sera pas dans le mux).
    for (const t of tracks) {
      if (t.type === 'audio' && !t.keep) t.default = false;
    }
    // 3. Subs : interne kept + externes, triés par priorité LiHDL
    //    (FR sans variant → FR VFF/VFi → FR VFQ → ENG → autres ; Forced → Full → SDH).
    const allSubs = [
      ...tracks.filter(t => t.type === 'subtitles' && t.keep),
      ...externalSubs,
    ];
    allSubs.sort((a, b) => subLabelPriority(a.label) - subLabelPriority(b.label));
    for (const s of allSubs) {
      s.order = order;
      order += 10;
    }
    // Trigger Svelte reactivity sur toutes les listes.
    tracks = [...tracks];
    externalAudios = [...externalAudios];
    externalSubs = [...externalSubs];
  }

  function finalizeAnalyze(rawTracks) {
    appendLog('🎯 finalizeAnalyze appelé avec ' + rawTracks.length + ' pistes');
    sourceInfo = { tracks: rawTracks };
    externalSubs = []; // reset les subs externes quand on recharge un mkv
    tracks = rawTracks.map((t, i) => {
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
        order: i * 10,
        // Champs mediainfo (peuvent être absents) — utilisés par automateLihdl
        mi_title: t.mi_title || '',
        mi_format: t.mi_format || '',
        mi_format_profile: t.mi_format_profile || '',
        mi_format_commercial: t.mi_format_commercial || '',
        mi_format_commercial_if_any: t.mi_format_commercial_if_any || '',
        mi_format_features: t.mi_format_features || '',
        mi_channels: t.mi_channels || '',
        mi_service_kind: t.mi_service_kind || '',
        mi_service_kind_name: t.mi_service_kind_name || '',
      };
      if (t.type === 'audio')     base.label = suggestAudioLabelFlat(base);
      if (t.type === 'subtitles') base.label = suggestSubLabelFlat(base);
      if (t.type === 'video')     base.label = '';
      return base;
    });
    // Auto-config mode PSA : la PSA fournit la vidéo, le SUPPLY/FW/Super U
    // fournit les audios+subs. Donc on décoche d'office les audios PSA
    // (seront remplacés) et on ne garde que les subs FR.
    if (muxMode === 'psa') {
      let droppedAud = 0, droppedSub = 0;
      tracks = tracks.map(t => {
        if (t.type === 'audio') {
          if (t.keep) droppedAud++;
          return { ...t, keep: false };
        }
        if (t.type === 'subtitles') {
          const isFR = /^fr/i.test(t.lang || '') || /^FR /.test(t.label || '');
          if (t.keep && !isFR) droppedSub++;
          return { ...t, keep: isFR };
        }
        return t;
      });
      if (droppedAud || droppedSub) {
        appendLog(`🎬 Mode PSA : ${droppedAud} audio(s) PSA décoché(s) (remplacés par SUPPLY), ${droppedSub} sub(s) non-FR décoché(s)`);
      }
    }
    // Norme LiHDL : si une piste FR VFQ est présente parmi les audios (internes,
    // externes ou secondary), la 2e piste FR doit rester FR VFF (pas FR VFi).
    // On désactive donc le toggle VFi auto.
    const hasVFQ = tracks.some(t => t.type === 'audio' && /^FR VFQ/.test(t.label || ''))
      || externalAudios.some(a => /^FR VFQ/.test(a.label || ''))
      || secondarySelected.some(t => t.type === 'audio' && /^FR VFQ/.test(t.label || ''));
    if (hasVFQ && useVFi) {
      useVFi = false;
      appendLog('🇫🇷 FR VFQ détecté → l\'autre piste FR reste FR VFF (norme LiHDL)');
      // Convertit immédiatement les FR VFi déjà labellisés en FR VFF.
      tracks = tracks.map(t => {
        if (t.type !== 'audio') return t;
        if (/^FR VFi\b/.test(t.label || '')) {
          return { ...t, label: t.label.replace(/^FR VFi/, 'FR VFF') };
        }
        return t;
      });
    }
    applyLihdlTrackOrder();
    applySubForcedRule();
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
    applySubForcedRule();
    // Trigger auto-sync alass si la source LiHDL est chargée.
    setTimeout(maybeAutoSubSyncCheck, 100);
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
      ...secondarySelected.filter(t => t.type === 'audio').map((s, i) => ({ kind: 'secondary', idx: i, ref: s })),
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
    secondarySelected = [...secondarySelected];
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
    // kind = 'internal' | 'external' | 'secondary' ; dir = -1 (up) / +1 (down)
    const secondarySubs = secondarySelected.filter(t => t.type === 'subtitles');
    const subs = [...tracks.filter(t => t.type === 'subtitles'), ...externalSubs, ...secondarySubs];
    subs.sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
    let target;
    if (kind === 'internal') {
      target = tracks.filter(t => t.type === 'subtitles')[idx];
    } else if (kind === 'secondary') {
      target = secondarySubs[idx];
    } else {
      target = externalSubs[idx];
    }
    const pos = subs.indexOf(target);
    const newPos = pos + dir;
    if (newPos < 0 || newPos >= subs.length) return;
    const other = subs[newPos];
    const tmp = target.order ?? 0;
    target.order = other.order ?? 0;
    other.order = tmp;
    tracks = [...tracks];
    externalSubs = [...externalSubs];
    secondarySelected = [...secondarySelected];
  }

  // Versions des suggest adaptées à la structure aplatie.
  function suggestAudioLabelFlat(t) {
    // Adapte la track interne (camelCase) au format attendu par inferAudioLabel
    // (snake_case + champs mediainfo) — comme ça le label tient compte de
    // mediainfo dès le chargement, pas seulement à automate.
    const raw = {
      language: t.lang,
      track_name: t.name,
      audio_channels: t.channels,
      codec_id: t.codecId,
      codec: t.codec,
      forced_track: t.forced,
      mi_title: t.mi_title,
      mi_format: t.mi_format,
      mi_format_profile: t.mi_format_profile,
      mi_format_commercial: t.mi_format_commercial,
      mi_format_commercial_if_any: t.mi_format_commercial_if_any,
      mi_format_features: t.mi_format_features,
      mi_channels: t.mi_channels,
      mi_service_kind: t.mi_service_kind,
      mi_service_kind_name: t.mi_service_kind_name,
    };
    const candidate = inferAudioLabel(raw);
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

  // Nettoie un nom de fichier pour en faire une requête TMDB indexée
  // (forme dotée, comme attendue par serveurperso/uklm.xyz) :
  //   Boston.Blue.S01E15.1080p.WEBRip.x265-PSA → "Boston.Blue"
  //   The.Boys.S04E08.WEB-DL... → "The.Boys"
  //   In.the.Name.of.Ben.Hur.2016... → "In.the.Name.of.Ben.Hur"
  function cleanQueryFromFilename(filename) {
    let name = String(filename || '').replace(/\.[^.]+$/, '');
    // Série : tout avant SxxExx (séparateur point OU espace).
    const ms = /^(.+?)[\s.]S\d{1,2}E\d{1,3}\b/i.exec(name);
    if (ms) name = ms[1];
    else {
      // Film : tout avant l'année 4 chiffres 19xx/20xx (séparateur point OU espace).
      const my = /^(.+?)[\s.](?:19|20)\d{2}\b/.exec(name);
      if (my) name = my[1];
    }
    // Strip un éventuel suffixe " 2024" / ".2024" (cas: Title.2024.S01E15...)
    name = name.replace(/[\s.](?:19|20)\d{2}$/, '');
    return name.trim();
  }

  async function maybeAutoFillTitle(path) {
    const filename = path.split('/').pop() || '';
    const cleanedDotted = cleanQueryFromFilename(filename);          // "The.Boys" — pour l'index
    const cleanedSpaces = cleanedDotted.replace(/\./g, ' ').trim();  // "The Boys" — pour l'API TMDB
    const isSeries = /\bS\d{1,2}E\d{1,3}\b/i.test(filename);
    const forceTV = isSeries || muxMode === 'psa';
    tmdbQuery = cleanedSpaces;
    if (forceTV) tmdbMode = 'tv';
    try {
      tmdbSearching = true;
      let r;
      if (forceTV) {
        // Mode PSA / série
        if (!config.tmdb_key) {
          appendLog('⚠ Clé API TMDB requise pour la recherche série — Réglages');
          r = await SearchTmdb(cleanedDotted);
        } else {
          try {
            r = await SearchTmdbTV(cleanedSpaces);
          } catch (e) {
            appendLog('⚠ Recherche TV : ' + String(e) + ' — fallback index');
            r = await SearchTmdb(cleanedDotted);
          }
        }
      } else if (muxMode === 'lihdl' && config.tmdb_key) {
        // Mode LiHDL avec clé API : recherche film officielle (homogène avec PSA TV)
        try {
          r = await SearchTmdbMovie(cleanedSpaces);
        } catch (e) {
          appendLog('⚠ Recherche film API : ' + String(e) + ' — fallback index');
          r = await SearchTmdb(cleanedDotted);
        }
      } else {
        // Pas de clé : index générique (films + séries mélangés)
        r = await SearchTmdb(cleanedDotted);
      }
      tmdbResults = r || []; tmdbResultIndex = 0;
      if (r && r.length >= 1) {
        // Auto-pick TOUJOURS le 1er résultat (top match TMDB).
        // L'utilisateur stoppe le mux via Stop si le match est faux.
        let picked = r[0];
        if (config.tmdb_key && picked.tmdb_id && !picked.overview) {
          try {
            let detail;
            if (forceTV) detail = await SearchTmdbTV(picked.tmdb_id);
            else if (muxMode === 'lihdl') detail = await SearchTmdbMovie(picked.tmdb_id);
            else detail = await SearchTmdb(picked.tmdb_id);
            if (detail && detail.length > 0) picked = detail[0];
          } catch (_) { /* fallback sur résultat de base */ }
        }
        lastTmdbResult = picked;
        target.title = composeTmdbTitle(picked);
        target.year  = picked.annee_fr || '';
        const suffix = r.length > 1 ? ` (${r.length} résultats — top auto)` : '';
        appendLog('✓ TMDB' + (forceTV ? ' (série)' : '') + ' : ' + target.title + suffix);
        // Mode LiHDL : si film français → VOF, sinon applique le toggle VFi.
        if (muxMode === 'lihdl') {
          if (picked && picked.original_language === 'fr') applyVOFSwap();
          else applyVFiSwap();
        }
        // Mode LiHDL : résolution Hydracker en arrière-plan (cache 12h)
        hydrackerURL = '';
        if (muxMode === 'lihdl' && picked.tmdb_id && config.hydracker_key) {
          try {
            const id = parseInt(picked.tmdb_id, 10);
            if (Number.isFinite(id)) {
              const url = await LookupHydrackerURL(id);
              if (url) {
                hydrackerURL = url;
                appendLog('🔗 Hydracker : fiche trouvée → ' + url);
              } else {
                appendLog('ℹ Hydracker : pas de fiche pour ce TMDB ID');
              }
            }
          } catch (_) { /* silencieux, fallback search */ }
        }
      } else {
        appendLog('ℹ Aucun résultat TMDB pour « ' + cleanedDotted + ' »');
      }
    } catch (e) {
      appendLog('⚠ TMDB : ' + String(e));
    } finally {
      tmdbSearching = false;
    }
  }

  // Recherche TMDB par ID numérique uniquement (champ séparé).
  async function searchTmdbById() {
    const id = String(tmdbIdQuery || '').trim();
    if (!id || !/^\d+$/.test(id)) {
      appendLog('⚠ ID TMDB doit être numérique');
      return;
    }
    tmdbSearching = true;
    tmdbResults = [];
    try {
      const r = tmdbMode === 'tv'
        ? await SearchTmdbTV(id)
        : (muxMode === 'lihdl' && config.tmdb_key)
          ? await SearchTmdbMovie(id)
          : await SearchTmdb(id);
      tmdbResults = r || []; tmdbResultIndex = 0;
      if (r && r.length >= 1) {
        lastTmdbResult = r[0];
        target.title = composeTmdbTitle(r[0]);
        target.year  = r[0].annee_fr || '';
        appendLog(`✓ TMDB (par ID ${id}) : ${target.title}`);
        if (muxMode === 'lihdl') {
          if (r[0].original_language === 'fr') applyVOFSwap();
          else applyVFiSwap();
        }
      } else {
        appendLog(`ℹ Aucun résultat pour ID ${id}`);
      }
    } catch (e) {
      appendLog('❌ TMDB par ID : ' + String(e));
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
      tmdbResults = r || []; tmdbResultIndex = 0;
    } catch (e) {
      appendLog('❌ TMDB : ' + String(e));
    } finally {
      tmdbSearching = false;
    }
  }

  // Recherche TMDB manuelle depuis la card "Aucune fiche trouvée" sur l'accueil.
  // Set lastTmdbResult comme la recherche auto, pour que le user reste sur l'accueil
  // et puisse continuer le workflow normalement.
  async function manualTmdbSearchFromCard() {
    if (!tmdbQuery.trim()) return;
    tmdbSearching = true;
    try {
      let r;
      if (tmdbMode === 'tv') r = await SearchTmdbTV(tmdbQuery);
      else if (muxMode === 'lihdl' && config.tmdb_key) r = await SearchTmdbMovie(tmdbQuery);
      else r = await SearchTmdb(tmdbQuery);
      if (r && r.length > 0) {
        let picked = r[0];
        if (muxMode === 'lihdl' && picked && picked.tmdb_id) {
          try {
            const detail = await SearchTmdbMovie(picked.tmdb_id);
            if (detail && detail.length > 0) picked = detail[0];
          } catch (_) {}
        }
        lastTmdbResult = picked;
        target.title = composeTmdbTitle(picked);
        target.year = picked.annee_fr || '';
        tmdbResults = r;
        tmdbResultIndex = 0;
        const suffix = r.length > 1 ? ` (${r.length} résultats)` : '';
        appendLog('✓ TMDB : ' + target.title + suffix);
      } else {
        appendLog('ℹ Aucun résultat pour « ' + tmdbQuery + ' »');
      }
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

  // Multi-sélection : ouvre un dialog pour choisir plusieurs MKV → ajoutés à la queue.
  // Si aucune source actuelle, charge le 1er. Bascule l'onglet sur Queue.
  async function pickMultipleMkvDialog() {
    const paths = await SelectMkvFiles();
    if (!paths || !paths.length) return;
    queueAdd(paths);
    appendLog('✓ ' + paths.length + ' .mkv ajoutés à la file');
    if (!sourcePath && queue.length > 0) queueLoad(0);
    bottomPaneTab = 'queue';
  }

  // Renvoie le dossier de sortie selon le mode courant (mode-specific puis fallback).
  function effectiveOutputDir() {
    if (muxMode === 'lihdl') return config.output_dir_lihdl || config.output_dir || '';
    if (muxMode === 'psa')   return config.output_dir_psa   || config.output_dir || '';
    return config.output_dir || '';
  }

  async function pickOutputDir() {
    const d = await SelectOutputDir();
    if (d) config.output_dir = d;
  }
  async function pickOutputDirLihdl() {
    const d = await SelectOutputDir();
    if (d) config.output_dir_lihdl = d;
  }
  async function pickOutputDirPSA() {
    const d = await SelectOutputDir();
    if (d) config.output_dir_psa = d;
  }

  async function openOutputDir() {
    const dir = effectiveOutputDir();
    if (!dir) { appendLog('⚠ Dossier de sortie non défini'); return; }
    try {
      await OpenFolder(dir);
    } catch (e) {
      appendLog('❌ Ouvrir dossier : ' + String(e));
    }
  }

  async function saveSettings() {
    await SaveConfig(config);
    appendLog('✓ Réglages enregistrés');
    mkvmergePath = await LocateMkvmerge();
  }

  // === Index Discord (admin) ===
  async function doDiscordScan() {
    if (discordScanRunning) return;
    if (!config.discord_bot_token || !config.discord_forum_id) {
      appendLog('⚠ Configure le token bot Discord ET l\'ID forum channel avant de scanner.');
      return;
    }
    // Sauve la config avant de scanner pour que le backend lise les valeurs à jour.
    try { await SaveConfig(config); } catch (e) { appendLog('⚠ Sauvegarde config : ' + String(e)); return; }
    discordScanRunning = true;
    discordScanProgress = { scanned: 0, total: 0, message: 'Démarrage du scan…' };
    try {
      const path = await DiscordIndexScan();
      appendLog('✓ Index Discord généré : ' + path);
    } catch (e) {
      appendLog('❌ Scan Discord : ' + String(e));
    } finally {
      discordScanRunning = false;
    }
  }

  async function doDiscordCopy() {
    try {
      const json = await DiscordIndexRead();
      if (!json) { appendLog('⚠ Aucun index local. Lance d\'abord un scan.'); return; }
      await navigator.clipboard.writeText(json);
      discordCopyOk = true;
      setTimeout(() => { discordCopyOk = false; }, 2000);
      appendLog('✓ JSON copié dans le presse-papier (' + json.length + ' octets)');
    } catch (e) {
      appendLog('❌ Copier JSON : ' + String(e));
    }
  }

  // Push direct du JSON sur GitHub via l'API Contents (admin).
  async function doDiscordPushGitHub() {
    if (githubPushing) return;
    if (!config.github_token || !config.github_repo) {
      appendLog('⚠ Configure le token GitHub et le repo avant de pusher.');
      return;
    }
    // Sauvegarde d'abord la config (au cas où l'utilisateur n'a pas cliqué Enregistrer).
    try { await SaveConfig(config); } catch {}
    githubPushing = true;
    githubPushOk = false;
    appendLog('📤 Push de l\'index Discord sur GitHub…');
    try {
      const sha = await DiscordIndexPushGitHub();
      githubPushOk = true;
      setTimeout(() => { githubPushOk = false; }, 4000);
      appendLog('✓ Index pushé sur GitHub (SHA ' + String(sha).substring(0, 8) + ')');
    } catch (e) {
      appendLog('❌ Push GitHub : ' + String(e?.message || e));
    } finally {
      githubPushing = false;
    }
  }

  let tmdbTest = { running: false, ok: null, message: '' };
  let hydrackerTest = { running: false, ok: null, message: '' };
  let unfrTest = { running: false, ok: null, message: '' };
  let ltTest = { running: false, ok: null, message: '' };

  async function doTestLanguageToolKey() {
    ltTest = { running: true, ok: null, message: '' };
    try {
      const r = await TestLanguageToolKey(config.languagetool_url || '', config.languagetool_key || '', config.languagetool_user || '');
      ltTest = { running: false, ok: !!r.ok, message: r.message || '' };
    } catch (e) {
      ltTest = { running: false, ok: false, message: String(e) };
    }
  }

  // === Review modal éditable : applique un fix LT puis note resolved ===
  async function applyReviewFix(idx, correction) {
    if (!ocrCurrentSRT) {
      appendLog('⚠ Pas de SRT actif pour patcher.');
      return;
    }
    const m = ocrProgress.lt_review_list[idx];
    if (!m) return;
    if (!correction || !correction.trim()) return;
    ltReviewState[idx].busy = true;
    ltReviewState[idx].error = '';
    ltReviewState = ltReviewState;
    try {
      await ApplyOCRFix(ocrCurrentSRT, m.line_number || 0, m.snippet || '', correction);
      ltReviewState[idx].resolved = true;
      ltReviewState[idx].busy = false;
      ltReviewState = ltReviewState;
      appendLog(`✓ SRT patché ligne ${m.line_number} : « ${correction} »`);
      // Auto-ajout au dictionnaire custom (auto=true), sans bloquer.
      const cleanSnippet = String(m.snippet || '').replace(/^…|…$/g, '').trim();
      if (cleanSnippet && cleanSnippet !== correction) {
        try {
          await OCRCustomDictAdd(cleanSnippet, correction, true);
        } catch (e) {
          appendLog('⚠ Dico custom : ' + String(e));
        }
      }
      // Si tous les matches sont résolus / ignorés → ferme le modal et recompute.
      maybeCloseReviewModal();
    } catch (e) {
      ltReviewState[idx].busy = false;
      ltReviewState[idx].error = String(e && e.message ? e.message : e);
      ltReviewState = ltReviewState;
      appendLog('❌ Patch SRT : ' + ltReviewState[idx].error);
    }
  }

  function ignoreReviewMatch(idx) {
    if (!ltReviewState[idx]) return;
    ltReviewState[idx].ignored = true;
    ltReviewState[idx].resolved = true;
    ltReviewState = ltReviewState;
    maybeCloseReviewModal();
  }

  function maybeCloseReviewModal() {
    const list = ocrProgress.lt_review_list || [];
    if (!list.length) return;
    const allDone = ltReviewState.every(s => s && s.resolved);
    if (allDone) {
      // Recompute approximatif du quality_score (corrections ≈ amélioration).
      // On ne relance pas tout le pipeline — juste un retrait des suspicious.
      const fixed = ltReviewState.filter(s => s && !s.ignored).length;
      ocrProgress = {
        ...ocrProgress,
        lt_needs_review: 0,
        // Le quality_score reste basé sur les patterns regex regex côté Go.
        // On ne le recalcule pas localement pour rester cohérent.
      };
      appendLog(`✓ Toutes les lignes traitées (${fixed} corrigées, ${ltReviewState.length - fixed} ignorées). Modal fermé.`);
      showLTReview = false;
    }
  }

  // === OpenSubtitles modal ===
  function openOSModal(context) {
    osContext = context || 'standalone';
    osError = '';
    osResults = [];
    // Pré-remplit depuis TMDB si dispo.
    if (lastTmdbResult) {
      osQuery = (lastTmdbResult.titre_fr || lastTmdbResult.titre_vo || '').trim();
      osYear = String(lastTmdbResult.annee_fr || '').trim();
    } else if (target.title) {
      osQuery = target.title;
      osYear = target.year || '';
    }
    showOSModal = true;
  }

  async function searchOpenSubtitles() {
    if (!osQuery.trim()) {
      osError = 'Saisis un titre.';
      return;
    }
    osSearching = true;
    osError = '';
    osResults = [];
    try {
      const yr = parseInt(osYear, 10) || 0;
      const r = await SearchOpenSubtitles(osQuery.trim(), yr, osLang || 'fr,en');
      osResults = r || [];
      if (!osResults.length) {
        osError = 'Aucun résultat.';
      }
    } catch (e) {
      const msg = String(e && e.message ? e.message : e);
      osError = msg;
      if (/clé API/.test(msg)) {
        appendLog('ℹ Configure ta clé OpenSubtitles dans Settings (clic sur "Réglages").');
      }
    } finally {
      osSearching = false;
    }
  }

  async function downloadOSResult(r) {
    if (!r || !r.id) return;
    osDownloading = r.id;
    try {
      let dst;
      if (osContext === 'post-source' && sourcePath) {
        // Sauve à côté du .mkv source pour cohérence avec OCRPGSTrack.
        const dir = sourcePath.substring(0, sourcePath.lastIndexOf('/'));
        const base = sourcePath.split('/').pop().replace(/\.mkv$/i, '');
        dst = `${dir}/${base}.opensubtitles.${r.language || 'fr'}.srt`;
      } else {
        // Standalone : demande un dossier de sortie.
        const out = await SelectOutputDir();
        if (!out) { osDownloading = ''; return; }
        dst = `${out}/${(r.filename || ('opensubtitles-' + r.id + '.srt'))}`;
      }
      const finalPath = await DownloadOpenSubtitle(r.id, dst);
      appendLog('✓ OpenSubtitles : ' + finalPath);
      // Si vue post-source, ajoute aux externalSubs.
      if (osContext === 'post-source' && sourcePath && finalPath) {
        let size = -1;
        try { size = await FileSize(finalPath); } catch {}
        let maxOrder = 0;
        for (const t of tracks) maxOrder = Math.max(maxOrder, t.order ?? 0);
        for (const s of externalSubs) maxOrder = Math.max(maxOrder, s.order ?? 0);
        externalSubs = [...externalSubs, {
          path: finalPath,
          name: finalPath.split('/').pop(),
          size,
          keep: true,
          default: false,
          forced: false,
          label: (r.language === 'fr') ? 'FR Full : SRT' : (r.language === 'en' ? 'ENG Full : SRT' : ''),
          order: maxOrder + 10,
        }];
      }
      showOSModal = false;
    } catch (e) {
      const msg = String(e && e.message ? e.message : e);
      appendLog('❌ OpenSubtitles : ' + msg);
      osError = msg;
    } finally {
      osDownloading = '';
    }
  }

  // === Custom dict OCR (Settings) ===
  async function loadCustomDict() {
    try {
      const r = await OCRCustomDictList();
      customDictEntries = r || [];
    } catch (e) {
      appendLog('⚠ Dico custom : ' + String(e));
      customDictEntries = [];
    }
  }

  async function addCustomDictEntry() {
    const w = (newDictWrong || '').trim();
    const r = (newDictRight || '').trim();
    if (!w || !r) return;
    dictBusy = true;
    try {
      await OCRCustomDictAdd(w, r, false);
      newDictWrong = '';
      newDictRight = '';
      showAddDictModal = false;
      await loadCustomDict();
      appendLog(`✓ Dico custom : « ${w} » → « ${r} »`);
    } catch (e) {
      appendLog('❌ Dico custom : ' + String(e));
    } finally {
      dictBusy = false;
    }
  }

  async function removeCustomDictEntry(wrong) {
    if (!wrong) return;
    try {
      await OCRCustomDictRemove(wrong);
      await loadCustomDict();
    } catch (e) {
      appendLog('❌ Dico custom : ' + String(e));
    }
  }

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

  async function doTestHydrackerKey() {
    hydrackerTest = { running: true, ok: null, message: '' };
    try {
      const r = await TestHydrackerKey(config.hydracker_key || '');
      hydrackerTest = { running: false, ok: !!r.ok, message: r.message || '' };
    } catch (e) {
      hydrackerTest = { running: false, ok: false, message: String(e) };
    }
  }

  async function doTestUnfrKey() {
    unfrTest = { running: true, ok: null, message: '' };
    try {
      const r = await TestUnfrKey(config.unfr_key || '');
      unfrTest = { running: false, ok: !!r.ok, message: r.message || '' };
    } catch (e) {
      unfrTest = { running: false, ok: false, message: String(e) };
    }
  }

  async function doMux() {
    if (!sourcePath)        { appendLog('⚠ Aucun .mkv source'); return; }
    const outDir = effectiveOutputDir();
    if (!outDir) { appendLog('⚠ Dossier de sortie non défini — ouvre Réglages'); return; }
    if (!effectiveFilename) { appendLog('⚠ Nom de fichier incomplet'); return; }

    // Construit le nom de piste vidéo LiHDL et rattache aux tracks.
    const videoName = videoTrackNameClient();
    // Helper : true si le label commence par "FR AD" → flag malvoyant
    const isAD = (lbl) => /^FR AD\b/.test(lbl || '');
    const specs = tracks.map(t => ({
      ID: t.id,
      Type: t.type,
      Keep: t.keep,
      Name: t.type === 'video' ? videoName : (t.label || ''),
      Language: t.type === 'video' ? '' : mapLangCode(t.label || ''),
      Default: !!t.default,
      Forced: !!t.forced,
      VisualImpaired: t.type === 'audio' && isAD(t.label),
      Order: t.order ?? 0,
    }));
    const extSubs = externalSubs.map(s => ({
      Path: s.path,
      Name: s.label || '',
      Language: mapLangCode(s.label || ''),
      Default: !!s.default,
      Forced: !!s.forced,
      DelayMs: s.delayMs || 0,
      TempoFactor: s.tempoFactor || 1.0,
      Order: s.order ?? 0,
    }));
    const extAudios = externalAudios.map(a => ({
      Path: a.path,
      Name: a.label || '',
      Language: mapLangCode(a.label || ''),
      Default: !!a.default,
      Forced: !!a.forced,
      VisualImpaired: isAD(a.label),
      DelayMs: a.delayMs || 0,
      TempoFactor: a.tempoFactor || 1.0,
      Order: a.order ?? 0,
    }));

    // Pistes secondaires (SUPPLY/FW) — audios et subs séparés.
    const secAudios = secondarySelected.filter(t => t.type === 'audio' && t.keep).map(t => ({
      ID: t.id,
      Name: t.label || '',
      Language: t.language || mapLangCode(t.label || ''),
      Default: !!t.default,
      Forced: !!t.forced,
      VisualImpaired: isAD(t.label),
      Order: t.order ?? 0,
    }));
    const secSubs = secondarySelected.filter(t => t.type === 'subtitles' && t.keep).map(t => ({
      ID: t.id,
      Name: t.label || '',
      Language: t.language || mapLangCode(t.label || ''),
      Default: !!t.default,
      Forced: !!t.forced,
      Order: t.order ?? 0,
    }));

    const outputPath = outDir.replace(/\/$/, '') + '/' + effectiveFilename;

    muxing = true;
    muxPercent = 0;
    let success = false;
    try {
      await Mux({
        input_path: sourcePath,
        output_path: outputPath,
        title: target.title || '',
        tracks: specs,
        external_audios: extAudios,
        external_subs: extSubs,
        secondary_path: secondaryPath,
        secondary_audios: secAudios,
        secondary_subs: secSubs,
        no_chapters: muxMode === 'lihdl', // LiHDL : on retire les chapitres ; PSA : on les garde
      });
      success = true;
    } catch (e) {
      appendLog('❌ ' + String(e));
    } finally {
      muxing = false;
    }
    return success;
  }

  function stopMux() { CancelMux(); }

  // ⚡ MUX MANUEL LiHDL : auto-label de toutes les pistes internes + réglages film.
  // Navigue automatiquement vers Cible une fois les labels appliqués.
  function automateLihdl(navigateToCible = true) {
    if (!sourcePath) { appendLog('⚠ Charge un fichier d\'abord'); return; }
    if (tracks.length === 0) { appendLog('⚠ Pistes pas encore analysées — patiente'); return; }
    let firstFRdone = false;
    let nbAudio = 0, nbSubs = 0;
    // Si des audios FR externes (extraits) sont présents, le default ira dessus —
    // pas sur un audio interne, même s'il est FR.
    const hasExternalFRAudio = externalAudios.some(a => /^FR (VFF|VFQ|VFi|VOF) /.test(a.label || ''));
    tracks = tracks.map(t => {
      if (t.type === 'video') return t;
      const raw = {
        language: t.lang,
        track_name: t.name,
        audio_channels: t.channels,
        codec_id: t.codecId,
        codec: t.codec,
        forced_track: t.forced,
        mi_title: t.mi_title,
        mi_format: t.mi_format,
        mi_format_profile: t.mi_format_profile,
        mi_format_commercial: t.mi_format_commercial,
        mi_format_commercial_if_any: t.mi_format_commercial_if_any,
        mi_format_features: t.mi_format_features,
        mi_channels: t.mi_channels,
        mi_service_kind: t.mi_service_kind,
        mi_service_kind_name: t.mi_service_kind_name,
      };
      if (t.type === 'audio') {
        const newLabel = inferAudioLabel(raw);
        const isFR = /^FR /.test(newLabel);
        // Préserve keep: false (piste FR remplacée par extraction) ; sinon true.
        const newKeep = t.keep === false ? false : true;
        // Default : si externes FR présentes, jamais de default sur interne.
        const isDefault = !hasExternalFRAudio && newKeep && isFR && !firstFRdone;
        if (isDefault) firstFRdone = true;
        if (newKeep) nbAudio++;
        return { ...t, label: newLabel, keep: newKeep, default: isDefault, forced: false };
      } else { // subtitles
        const newLabel = inferSubLabel(raw, 0);
        const isForcedFR = /^FR( VFF)? Forced\b/.test(newLabel);
        nbSubs++;
        return {
          ...t,
          label: newLabel,
          keep: true,
          default: isForcedFR,
          forced: isForcedFR,
        };
      }
    });
    // Si le film est originellement français (TMDB original_language = "fr"),
    // les pistes FR ne sont pas des doublages → flag "FRENCH.VOF" + label "FR VOF".
    const isOrigFrench = lastTmdbResult && lastTmdbResult.original_language === 'fr';
    if (isOrigFrench) {
      tracks = tracks.map(t => {
        if (t.type !== 'audio') return t;
        if (/^FR (VFF|VFi|VFQ) /.test(t.label || '')) {
          return { ...t, label: t.label.replace(/^FR (VFF|VFi|VFQ) /, 'FR VOF ') };
        }
        return t;
      });
      appendLog('🇫🇷 Film français (TMDB) → labels FR VOF');
    } else {
      // Norme LiHDL : si une FR VFQ existe (interne, externe ou secondary), la
      // 2e piste FR doit rester FR VFF (pas FR VFi). Désactive le toggle avant
      // le swap pour éviter le rename FR VFF → FR VFi.
      const hasVFQ = tracks.some(t => t.type === 'audio' && /^FR VFQ/.test(t.label || ''))
        || externalAudios.some(a => /^FR VFQ/.test(a.label || ''))
        || secondarySelected.some(t => t.type === 'audio' && /^FR VFQ/.test(t.label || ''));
      if (hasVFQ && useVFi) {
        useVFi = false;
        appendLog('🇫🇷 FR VFQ détecté → l\'autre piste FR reste FR VFF (norme LiHDL)');
      }
      // Toggle VFi : applique le swap FR VFF ↔ FR VFi selon le state useVFi.
      applyVFiSwap();
    }

    // Heuristique : 2+ pistes FR audio + une en 2.0 → AD (audiodescription)
    const frAudios = tracks.filter(t => t.type === 'audio' && /^FR /.test(t.label || ''));
    if (frAudios.length >= 2) {
      tracks = tracks.map(t => {
        if (t.type !== 'audio') return t;
        if (/^FR (VFF|VFQ|VFi|VOF) /.test(t.label || '') && / 2\.0/.test(t.label)) {
          return { ...t, label: t.label.replace(/^FR (VFF|VFQ|VFi|VOF) /, 'FR AD ') };
        }
        return t;
      });
    }

    // Subs externes : applique aussi la règle FR Forced → keep + default + forced
    // (les externes ne passent pas par automateLihdl ci-dessus, on les traite ici).
    externalSubs = externalSubs.map(s => {
      const isForcedFR = /^FR( VFF)? Forced\b/.test(s.label || '');
      return { ...s, keep: true, default: isForcedFR, forced: isForcedFR };
    });

    // Norme LiHDL : la piste FR Forced est toujours placée EN PREMIER parmi
    // les sous-titres — qu'elle soit interne (tracks) OU externe (externalSubs).
    const allSubs = [
      ...tracks.filter(t => t.type === 'subtitles').map(t => t.order ?? 0),
      ...externalSubs.map(s => s.order ?? 0),
    ];
    const minSubOrder = allSubs.length > 0 ? Math.min(...allSubs) : 0;
    const fwdOrder = minSubOrder - 1;
    let forcedFRDone = false;
    tracks = tracks.map(t => {
      if (!forcedFRDone && t.type === 'subtitles' && /^FR( VFF)? Forced\b/.test(t.label || '')) {
        forcedFRDone = true;
        return { ...t, order: fwdOrder };
      }
      return t;
    });
    if (!forcedFRDone) {
      externalSubs = externalSubs.map(s => {
        if (!forcedFRDone && /^FR( VFF)? Forced\b/.test(s.label || '')) {
          forcedFRDone = true;
          return { ...s, order: fwdOrder };
        }
        return s;
      });
    }
    if (forcedFRDone) appendLog('↑ FR Forced placé en 1er dans les sous-titres');

    videoChoice.team = 'LiHDL';
    target.episode = '';

    // Norme LiHDL : ré-applique l'ordre des pistes (FR VFi/VFF avant FR VFQ)
    // ET réassigne le flag default sur la 1ère piste audio en ordre LiHDL.
    // Sinon, la boucle map ci-dessus a mis default=true sur la 1ère FR rencontrée
    // dans l'ordre SOURCE (souvent FR VFQ si le mkv a VFQ avant VFF).
    applyLihdlTrackOrder();

    appendLog(`⚡ LiHDL automatisé : ${nbAudio} audio(s) + ${nbSubs} sub(s) labellisés → Cible`);
    if (navigateToCible) screen = 'cible';
  }

  // 🚀 MUX AUTO LiHDL : automateLihdl puis doMux direct (sans nav vers Cible).
  async function muxAutoLihdl() {
    if (!sourcePath) { appendLog('⚠ Charge un fichier d\'abord'); return; }
    autoMuxStatus = '';
    if (autoMuxStatusTimer) { clearTimeout(autoMuxStatusTimer); autoMuxStatusTimer = null; }
    automateLihdl(false); // pas de navigation, on reste sur source pour la barre de progress
    await new Promise(r => setTimeout(r, 200));
    const ok = await doMux();
    autoMuxStatus = ok ? 'success' : 'error';
    // Pas d'auto-clear timer : la barre verte/rouge reste visible jusqu'au prochain mux ou reset manuel.
    if (ok) await autoResetAfterMux(true);
  }

  // MUX AUTO : automate puis lance le mux directement, sans passer par Cible.
  let autoMuxStatus = '';   // '' | 'success' | 'error'
  let autoMuxStatusTimer = null;
  async function muxAuto() {
    if (!sourcePath) { appendLog('⚠ Charge le fichier PSA d\'abord'); return; }
    if (!secondaryPath || secondaryTracks.length === 0) {
      appendLog('⚠ Charge le fichier SUPPLY/FW d\'abord');
      return;
    }
    autoMuxStatus = '';
    if (autoMuxStatusTimer) { clearTimeout(autoMuxStatusTimer); autoMuxStatusTimer = null; }
    automate();
    // Petit délai pour que Svelte propage les updates de state avant doMux.
    await new Promise(r => setTimeout(r, 200));
    const ok = await doMux();
    autoMuxStatus = ok ? 'success' : 'error';
    // Pas d'auto-clear timer : la barre verte/rouge reste visible jusqu'au prochain mux ou reset manuel.
    if (ok) await autoResetAfterMux(true);
  }

  onMount(async () => {
    try { appVersion = await GetVersion(); } catch {}
    try { config = await GetConfig(); } catch {}
    try { options = await GetLihdlOptions(); } catch {}
    try { mkvmergePath = await LocateMkvmerge(); } catch {}

    // Best-effort : fetch l'index Discord remote (cache 24 h, silencieux si pas configuré).
    DiscordIndexRefreshRemote().catch((e) => {
      // Pas bloquant — l'app fonctionne sans Discord index.
      console.warn('Discord index remote fetch failed:', e);
    });

    EventsOn('log', (msg) => {
      appendLog(msg);
      // Détection des phases d'extraction audio pour la barre de progression :
      // 🔎 Détection sync → phase "sync"
      // 🔄 Conversion XXX → AC3 → phase "convert" avec codec + variant
      // ✓ ... → reset phase si fin de conversion
      if (typeof msg === 'string') {
        if (msg.startsWith('🔎 Détection sync')) {
          frAudioPhase = 'sync';
          frAudioConvertPercent = 0;
        } else if (msg.startsWith('🔄 Conversion ASS → SRT')) {
          // Phase conversion sub ASS→SRT (rapide, pas de pourcentage)
          srtPhase = 'convert_ass';
          srtAssConverted = true; // mémorise pour le message de succès
        } else if (msg.startsWith('✓ ASS converti en SRT')) {
          srtPhase = '';
        } else if (msg.startsWith('🔄 Conversion')) {
          // Format : "🔄 Conversion EAC3 → AC3 (piste #1, VFF, 6 ch)…"
          const m = msg.match(/🔄 Conversion (\S+) → AC3.*?, (\S+),/);
          if (m) frAudioPhase = `convert:${m[1]},${m[2]}`;
          else frAudioPhase = 'convert';
          frAudioConvertPercent = 0;
        } else if (msg.startsWith('✓ ') && msg.includes('converti en AC3')) {
          frAudioPhase = ''; // conversion finie pour cette piste
          frAudioConvertPercent = 0;
        }
      }
    });
    EventsOn('discordindex:progress', (p) => {
      discordScanProgress = {
        scanned: p.scanned ?? 0,
        total: p.total ?? 0,
        message: p.message ?? '',
      };
    });
    EventsOn('mux:progress', (p) => { muxPercent = p.Percent || p.percent || 0; });
    EventsOn('mux:done', () => {
      muxing = false;
      muxPercent = 0;
      if (queue.length > 0) {
        appendLog('✓ Mux terminé — ' + queue.length + ' en file. Clique "Charger le suivant" pour continuer.');
      }
    });
    EventsOn('audiosync:progress', (p) => { syncPercent = p.Percent || p.percent || 0; });
    EventsOn('ac3convert:progress', (p) => { frAudioConvertPercent = p.percent ?? p.Percent ?? 0; });
    EventsOn('srtprogress', (p) => { srtPercent = p.percent ?? p.Percent ?? 0; });
    EventsOn('ocr:progress', (p) => {
      ocrProgress = {
        status: p.status || '',
        percent: p.percent ?? 0,
        message: p.message || '',
        total_lines: p.total_lines ?? ocrProgress.total_lines ?? 0,
        corrected_lines: p.corrected_lines ?? ocrProgress.corrected_lines ?? 0,
        suspicious_lines: p.suspicious_lines ?? ocrProgress.suspicious_lines ?? 0,
        quality_score: p.quality_score ?? ocrProgress.quality_score ?? 0,
        subtitles: p.subtitles ?? ocrProgress.subtitles ?? 0,
        lt_total_issues: p.lt_total_issues ?? ocrProgress.lt_total_issues ?? 0,
        lt_auto_fixed: p.lt_auto_fixed ?? ocrProgress.lt_auto_fixed ?? 0,
        lt_needs_review: p.lt_needs_review ?? ocrProgress.lt_needs_review ?? 0,
        lt_review_list: p.lt_review_list ?? ocrProgress.lt_review_list ?? [],
      };
      // Quand l'OCR vient de finir (status 'done'), capture le path du SRT
      // pour permettre le patch via ApplyOCRFix, et reset l'état du modal.
      if (p.status === 'done' && p.message) {
        ocrCurrentSRT = p.message;
        const list = ocrProgress.lt_review_list || [];
        ltReviewState = list.map(() => ({ resolved: false, ignored: false, customText: '', busy: false, error: '' }));
      }
    });
    EventsOn('subsync:progress', (p) => {
      subSyncPercent = p.percent ?? p.Percent ?? 0;
      subSyncCurrentName = p.current ?? p.Current ?? '';
    });
    EventsOn('audiosync:done', () => { syncRunning = false; syncPercent = 0; });
    EventsOn('file:dropped', (path) => { openMkv(path); });
    EventsOn('subs:dropped', (paths) => { addExternalSubs(paths || []); });
    EventsOn('audios:dropped', (paths) => { addExternalAudios(paths || []); });
    EventsOn('secondary:tracks', (raw) => {
      try {
        secondaryTracks = JSON.parse(String(raw || '[]'));
        // Mode PSA : applique automate() tout de suite pour que les pistes
        // audio+subs SUPPLY apparaissent direct dans les panneaux PISTES,
        // puis lance la vérif sync audio PSA ↔ SUPPLY.
        if (muxMode === 'psa' && sourcePath && secondaryTracks.length > 0) {
          setTimeout(() => {
            automate();
            checkPSASync();
          }, 50);
        }
      } catch (e) {
        appendLog('❌ secondary:tracks parse : ' + String(e));
      }
    });
    EventsOn('files:dropped', (paths) => {
      if (!paths || !paths.length) return;
      // Drop multi-MKV : autorisé uniquement sur la page d'accueil (pas de source chargée).
      if (sourcePath) {
        appendLog('ℹ Drop multi-MKV ignoré : termine d\'abord le mux en cours');
        return;
      }
      queueAdd(paths);
      appendLog('✓ ' + paths.length + ' .mkv ajoutés à la file');
      if (!sourcePath && queue.length > 0) queueLoad(0);
      bottomPaneTab = 'queue';
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

<div class="app" class:no-source={!sourcePath && tracks.length === 0 && screen === 'source'} class:tmdb-pending={sourcePath && !tmdbValidated && screen === 'source'} style:--banner-url="url({banner})">
  <!-- Liquid glass background layers : banner + dark overlay + accent glow -->
  <div class="bg-banner" aria-hidden="true"></div>
  <div class="bg-overlay" aria-hidden="true"></div>
  <div class="bg-glow bg-glow-1" aria-hidden="true"></div>
  <div class="bg-glow bg-glow-2" aria-hidden="true"></div>

  <!-- ─────── HEADER ─────── -->
  <header class="header">
    <div class="brand">
      <img class="brand-logo-img" src={logo} alt="LiHDL" />
      <div>
        <div class="brand-name">GO Mux LiHDL Team</div>
        <div class="brand-version">{appVersion || 'v?'} · BY GANDALF</div>
      </div>
    </div>

    <div class="mode-switch">
      <button class="mode-btn" class:active={muxMode === 'lihdl'} on:click={() => switchMuxMode('lihdl')}>⚡ MUX LiHDL</button>
      <button class="mode-btn" class:active={muxMode === 'psa'}   on:click={() => switchMuxMode('psa')}>🎬 Custom PSA</button>
    </div>

    <!-- Film-bar : si TMDB sélectionné, on affiche poster + identité ; sinon placeholder -->
    {#if lastTmdbResult}
      <div class="film-bar">
        {#if lastTmdbResult.poster_url}
          <img src={lastTmdbResult.poster_url} alt="" class="film-poster" loading="lazy" />
        {:else}
          <div class="film-poster placeholder"></div>
        {/if}
        <div class="film-info">
          <div class="film-title">{lastTmdbResult.titre_fr || lastTmdbResult.titre_vo || '—'}{lastTmdbResult.annee_fr ? ` (${lastTmdbResult.annee_fr})` : ''}</div>
          <div class="film-meta">
            {#if lastTmdbResult.tmdb_id}<button type="button" class="film-id film-id-link" on:click={() => OpenURL(lastTmdbResult.url || `https://www.themoviedb.org/movie/${lastTmdbResult.tmdb_id}`)} title="Ouvrir la fiche TMDB">↗ TMDB</button>{/if}
            {#if discordIndexEntry}<button type="button" class="film-id film-id-link" on:click={() => OpenURL(discordIndexEntry)} title="Ouvrir le post Discord de la Team">↗ Discord</button>{/if}
            {#if lastTmdbResult.duree}<span>⏱ {lastTmdbResult.duree}</span>{/if}
            {#if lastTmdbResult.note > 0}<span>★ {lastTmdbResult.note}</span>{/if}
          </div>
        </div>
      </div>
    {:else}
      <div class="film-bar empty">
        <div class="film-poster placeholder"></div>
        <div class="film-info">
          <div class="film-title">— Aucun film identifié —</div>
          <div class="film-meta"><span>Charge une source MKV pour démarrer</span></div>
        </div>
      </div>
    {/if}

    <div class="header-actions">
      {#if updateInfo}
        <button class="btn update-pill available" on:click={doInstallUpdate} disabled={installingUpdate}
                title="Installer {updateInfo.version}">
          {installingUpdate ? '⟳ Installation…' : '⬇ ' + updateInfo.version}
        </button>
      {:else}
        <button class="btn btn-ghost btn-icon" on:click={checkForUpdate} disabled={checkingUpdate}
                title="Rechercher une mise à jour">
          <span class:spin={checkingUpdate}>⟳</span>
        </button>
      {/if}
      <button class="btn btn-ghost" on:click={resetAll} disabled={muxing} title="Réinitialiser la session">↻ RESET</button>
      <button class="btn" on:click={() => screen = (screen === 'reglages' ? 'source' : 'reglages')}>⚙ SETTINGS</button>
      {#if muxMode === 'psa' && secondaryPath && secondaryTracks.length > 0}
        <button class="btn btn-accent" on:click={automate} disabled={muxing || !sourcePath}>⚡ MUX MANUEL</button>
        <button class="btn btn-danger" on:click={muxAuto} disabled={muxing || !sourcePath}>🚀 MUX AUTO</button>
      {:else}
        <button class="btn btn-accent" on:click={() => automateLihdl()} disabled={muxing || !sourcePath || tracks.length === 0}>⚡ MUX MANUEL</button>
        <button class="btn btn-danger" on:click={muxAutoLihdl} disabled={muxing || !sourcePath || tracks.length === 0}>🚀 MUX AUTO</button>
      {/if}
    </div>
  </header>

  <!-- Sub-nav : Source / Cible / Sync / Réglages (utilisée seulement si on quitte 'source') -->
  {#if screen !== 'source'}
    <nav class="subnav">
      <button class="btn btn-ghost" on:click={() => screen = 'source'}>← Retour</button>
      <button class="subnav-btn" class:active={screen === 'source'}   on:click={() => screen = 'source'}>Source</button>
      <button class="subnav-btn" class:active={screen === 'cible'}    on:click={() => screen = 'cible'}>Cible</button>
      <button class="subnav-btn" class:active={screen === 'sync'}     on:click={() => screen = 'sync'}>Synchro Audios</button>
      <button class="subnav-btn" class:active={screen === 'reglages'} on:click={() => screen = 'reglages'}>Réglages</button>
    </nav>
  {/if}

  <!-- Mux progress bar : si en cours, on affiche au-dessus du main -->
  {#if muxing}
    <div class="mux-progress-bar">
      <button class="btn btn-danger btn-stop-small" on:click={stopMux}>⏹ Stop</button>
      <div class="progress-bar"><div class="progress-fill" style:width="{muxPercent}%"></div></div>
      <span class="mono">{muxPercent}% · Mux en cours…</span>
    </div>
  {/if}

  <!-- ─────── MAIN ─────── -->
  <main class="main">
    {#if screen === 'source'}

    <!-- COLONNE GAUCHE -->
    <div class="col">

      <!-- Bandeau persistant : statut du dernier MUX AUTO (succès / erreur). -->
      {#if autoMuxStatus === 'success' && !muxing}
        <div class="card mux-status-banner success">
          <div class="auto-status done">✅ Mux terminé avec succès — fichier prêt dans ton dossier de sortie</div>
        </div>
      {:else if autoMuxStatus === 'error' && !muxing}
        <div class="card mux-status-banner error">
          <div class="auto-status error">❌ Mux échoué — vérifie les logs en bas</div>
        </div>
      {/if}

      <!-- Card unifiée : PSA + SUPPLY/FW (en mode PSA) ou seul (en mode LiHDL).
           Cachée tant que TMDB pas validé pour ne pas polluer la page d'accueil. -->
      {#if tmdbValidated || !sourcePath}
      <div class="card drop-target" style:--wails-drop-target="drop">
        <div class="card-title">📁 Sources</div>

        <div class="source-list">

        <!-- Ligne ① Source principale -->
        <div class="source-row" class:filled={sourcePath}>
          <div class="source-num">1</div>
          <div class="source-info">
            <div class="source-label">{muxMode === 'psa' ? 'Source PSA (vidéo gardée)' : 'Source encodée (vidéo gardée)'}</div>
            {#if sourcePath}
              <div class="source-value">{sourcePath.split('/').pop()}</div>
              {#if sourceMkvInfo}
                <div class="source-value" style="opacity:0.7;font-size:11px;">⏱ {formatDuration(sourceMkvInfo.duration_seconds)} · 🎞 {sourceMkvInfo.framerate || '?'} fps</div>
              {/if}
            {:else}
              <div class="source-value empty">— Aucun fichier sélectionné —</div>
            {/if}
          </div>
          <button class="btn btn-ghost" on:click={pickMkvDialog}>
            {sourcePath ? 'Changer' : (muxMode === 'psa' ? 'Choisir PSA' : 'Choisir')}
          </button>
        </div>

        <!-- Ligne ② SUPPLY/FW (mode PSA uniquement) -->
        {#if muxMode === 'psa'}
          <div class="source-row" class:filled={secondaryPath}>
            <div class="source-num">2</div>
            <div class="source-info">
              <div class="source-label">Source SUPPLY / FW / Super U (audios + subs)</div>
              {#if secondaryPath}
                <div class="source-value">{secondaryPath.split('/').pop()} · {secondaryTracks.length} piste(s)</div>
                {#if secondaryMkvInfo}
                  <div class="source-value" style="opacity:0.7;font-size:11px;">⏱ {formatDuration(secondaryMkvInfo.duration_seconds)} · 🎞 {secondaryMkvInfo.framerate || '?'} fps</div>
                {/if}
                {#if psaSyncStatus}
                  <div class="source-value" style="font-size:11px;font-weight:600;color:{psaSyncStatus === 'ok' ? '#7ad17a' : psaSyncStatus === 'corrected' ? '#e8b94a' : psaSyncStatus === 'error' ? '#e87a7a' : '#aaa'};">
                    {psaSyncStatus === 'checking' ? '🔎' : psaSyncStatus === 'ok' ? '✓' : psaSyncStatus === 'corrected' ? '↻' : '⚠'} {psaSyncMessage}
                  </div>
                {/if}
              {:else}
                <div class="source-value empty">— Aucun fichier sélectionné —</div>
              {/if}
            </div>
            <button class="btn btn-ghost" on:click={pickSecondaryDialog}>
              {secondaryPath ? 'Changer' : 'Choisir SUPPLY/FW/Super U'}
            </button>
          </div>
        {/if}

        <!-- Ligne ② Sous-titres externes (mode LiHDL uniquement) -->
        {#if muxMode === 'lihdl'}
          <div class="source-row source-row-stacked" class:filled={externalSubs.length > 0}>
            <div class="source-num">2</div>
            <div class="source-info">
              <div class="source-label">Sous-titres (SRT/PGS/ASS · optionnel)</div>
              {#if externalSubs.length > 0}
                <div class="source-value">
                  {externalSubs.length} fichier(s) : {basename(externalSubs[0].path)}{externalSubs.length > 1 ? ` +${externalSubs.length - 1} autre(s)` : ''}
                </div>
              {:else}
                <div class="source-value empty">— Aucun sous-titre chargé —</div>
              {/if}
              {#if srtExtracting || srtExtractionResult}
                <div class="extract-progress" class:done={srtExtractionResult === 'success' && !srtExtracting} class:err={srtExtractionResult === 'error' && !srtExtracting}>
                  <span class="extract-progress-label">
                    {#if srtExtracting}
                      {#if srtPhase === 'convert_ass'}
                        🔄 Conversion ASS → SRT (via ffmpeg) — {srtPercent}%
                      {:else}
                        ⏳ Extraction SRT + détection sync — {srtPercent}%
                      {/if}
                    {:else if srtExtractionResult === 'success'}
                      {#if srtAssConverted}
                        ✓ SRT extraits (ASS converti en SRT via ffmpeg) + sync appliquée
                      {:else}
                        ✓ SRT extraits + sync appliquée
                      {/if}
                    {:else if srtExtractionResult === 'error'}
                      ✕ Erreur lors de l'extraction SRT
                    {/if}
                  </span>
                  {#if srtExtracting}
                    <progress max="100" value={srtPercent}></progress>
                  {:else}
                    <progress max="100" value="100"></progress>
                  {/if}
                </div>
              {/if}
              <!-- Module : vérif sync SRT externes vs audio source LiHDL -->
              {#if externalSubs.some(s => s.path && /\.srt$/i.test(s.path)) && sourcePath}
                <div class="sub-sync-module">
                  {#if subSyncChecking}
                    <!-- Phase analyse : barre de progression % -->
                    <div class="extract-progress">
                      <span class="extract-progress-label">
                        🔎 Analyse sync SRT — {subSyncPercent}% {subSyncCurrentName ? `(${subSyncCurrentName})` : ''}
                      </span>
                      <progress max="100" value={subSyncPercent}></progress>
                    </div>
                  {:else if subSyncResults.length === 0 && subSyncAppliedMsg}
                    <!-- Message succès persistant après apply -->
                    <div class="extract-progress done">
                      <span class="extract-progress-label">{subSyncAppliedMsg}</span>
                      <progress max="100" value="100"></progress>
                    </div>
                    <div class="sync-actions" style="margin-top:6px;">
                      <button class="btn btn-tiny" on:click={runSubSyncCheck}>🔎 Re-vérifier</button>
                      <button class="btn btn-ghost btn-tiny" on:click={dismissSubSyncResults}>Masquer</button>
                    </div>
                  {:else if subSyncResults.length === 0}
                    <!-- État initial : bouton de lancement -->
                    <button class="btn btn-tiny" on:click={runSubSyncCheck}>
                      🔎 Vérifier sync SRT vs audio source
                    </button>
                  {:else}
                    <!-- Résultats alass : SRT corrigé prêt à remplacer l'original -->
                    <div class="sync-results-header">Synchronisation alass (SRT corrigé prêt) :</div>
                    <ul class="sync-results-list">
                      {#each subSyncResults as r}
                        <li class:has-offset={!r.error && r.synced_path} class:err={!!r.error}>
                          <span class="sync-result-name mono">{basename(r.path)}</span>
                          {#if r.error}
                            <span class="sync-result-status">✕ {r.error}</span>
                          {:else}
                            <span class="sync-result-status">
                              ✓ décalage {r.offset_ms > 0 ? '+' : ''}{r.offset_ms} ms{r.fps_ratio ? ` · FPS ${r.fps_ratio}` : ''} appliqué
                            </span>
                          {/if}
                        </li>
                      {/each}
                    </ul>
                    <div class="sync-actions">
                      {#if subSyncResults.some(r => !r.error && r.synced_path)}
                        <button class="btn btn-primary btn-tiny" on:click={applySubSyncResults}>✓ Utiliser les SRT corrigés</button>
                      {/if}
                      <button class="btn btn-ghost btn-tiny" on:click={dismissSubSyncResults}>Annuler</button>
                    </div>
                  {/if}
                </div>
              {/if}
            </div>
            <div class="source-row-actions">
              <button class="btn btn-ghost" on:click={pickSubsDialog} disabled={srtExtracting}>
                {externalSubs.length > 0 ? '+ Ajouter' : 'Choisir'}
              </button>
              {#if externalSubs.length > 0}
                <button class="btn btn-ghost btn-icon" on:click={() => externalSubs = []} title="Vider la liste" disabled={srtExtracting}>✕</button>
              {/if}
            </div>
          </div>

          <!-- Ligne ③ Source de référence : compat durée/FPS + extraction SRT (auto) + extraction FR audio (toggles) avec sync auto -->
          {#if showReferenceBar}
            {@const compat = checkCompat(sourceMkvInfo, referenceMkvInfo)}
            <div class="source-row source-row-stacked" class:filled={referencePath}>
              <div class="source-num">3</div>
              <div class="source-info">
                <div class="source-label">Source de référence (extraction sous-titres + FR audio sur demande)</div>
                {#if referencePath}
                  <div class="source-value">{basename(referencePath)}</div>
                  {#if referenceMkvInfo && sourceMkvInfo}
                    <div class="compat-grid">
                      <span>Durée : {formatDuration(sourceMkvInfo.duration_seconds)} vs {formatDuration(referenceMkvInfo.duration_seconds)} <span class:compat-ok={compat.durationOK} class:compat-bad={compat.durationOK === false}>{compat.durationOK ? '✓' : '✗'}</span></span>
                      <span>FPS : {sourceMkvInfo.framerate || '?'} vs {referenceMkvInfo.framerate || '?'} <span class:compat-ok={compat.fpsOK} class:compat-bad={compat.fpsOK === false}>{compat.fpsOK ? '✓' : '✗'}</span></span>
                      <span>VFQ : <span class:compat-ok={referenceMkvInfo.has_vfq_audio} class:compat-bad={!referenceMkvInfo.has_vfq_audio}>{referenceMkvInfo.has_vfq_audio ? '✓ ' + (referenceMkvInfo.vfq_track_info || 'piste détectée') : '✗ aucune piste FR Canada'}</span></span>
                    </div>
                  {/if}
                  <div class="fr-audio-options">
                    <label class="vfq-toggle">
                      <input type="checkbox" bind:checked={extractFRVFF} disabled={frAudioExtracting} />
                      <span>Extraire FR VFF</span>
                    </label>
                    <label class="vfq-toggle">
                      <input type="checkbox" bind:checked={extractFRVFQ} disabled={frAudioExtracting} />
                      <span>Extraire FR VFQ</span>
                    </label>
                    <label class="vfq-toggle">
                      <input type="checkbox" bind:checked={extractENG} disabled={frAudioExtracting} />
                      <span>Extraire ENG VO</span>
                    </label>
                    {#if extractFRVFF || extractFRVFQ || extractENG}
                      <button class="btn-primary btn-auto btn-tiny" on:click={runFRAudioExtraction} disabled={frAudioExtracting || !sourcePath}>
                        {frAudioExtracting ? '⏳ Extraction…' : '⚡ Extraire + sync'}
                      </button>
                    {/if}
                  </div>
                  <div class="fr-audio-options" style="margin-top:6px;">
                    <span style="opacity:0.7;font-size:12px;">🎯 Référence sync :</span>
                    <label class="vfq-toggle">
                      <input type="radio" name="syncRefLang" bind:group={syncRefLang} value="fr" disabled={frAudioExtracting} />
                      <span>FR (VFF/VFi)</span>
                    </label>
                    <label class="vfq-toggle">
                      <input type="radio" name="syncRefLang" bind:group={syncRefLang} value="eng" disabled={frAudioExtracting} />
                      <span>ENG VO</span>
                    </label>
                  </div>
                  {#if frAudioExtracting || frAudioExtractionResult}
                    <div class="extract-progress" class:done={frAudioExtractionResult === 'success' && !frAudioExtracting} class:err={frAudioExtractionResult === 'error' && !frAudioExtracting}>
                      <span class="extract-progress-label">
                        {#if frAudioExtracting}
                          {#if frAudioPhase === 'sync'}
                            🔎 Détection sync audio…
                          {:else if frAudioPhase.startsWith('convert:')}
                            {@const parts = frAudioPhase.slice(8).split(',')}
                            🔄 Conversion {parts[0]} → AC3 ({parts[1] || ''}) — {frAudioConvertPercent}%
                          {:else if frAudioPhase === 'convert'}
                            🔄 Conversion vers AC3 — {frAudioConvertPercent}%
                          {:else}
                            ⏳ Extraction FR audio + détection sync…
                          {/if}
                        {:else if frAudioExtractionResult === 'success'}
                          {#if frAudioConvertedSummary}
                            ✓ Pistes audio extraites + converties AC3 ({frAudioConvertedSummary} via ffmpeg) + sync appliquée
                          {:else}
                            ✓ Pistes audio extraites (AC3 source, lossless) + sync appliquée
                          {/if}
                        {:else if frAudioExtractionResult === 'error'}
                          ✕ Erreur lors de l'extraction FR audio
                        {/if}
                      </span>
                      {#if frAudioExtracting && frAudioPhase.startsWith('convert')}
                        <progress max="100" value={frAudioConvertPercent}></progress>
                      {:else if frAudioExtracting}
                        <progress></progress>
                      {:else}
                        <progress max="100" value="100"></progress>
                      {/if}
                    </div>
                  {/if}
                {:else}
                  <div class="source-empty">— Aucun fichier de référence —</div>
                {/if}
              </div>
              <div class="source-row-actions">
                <button class="btn-primary btn-tiny" on:click={runRefSubsExtraction} disabled={!referencePath || srtExtracting || frAudioExtracting} title="Extraire les sous-titres FR/ENG (texte) de la référence et les ajouter au mux (les doublons sont remplacés)">
                  {srtExtracting ? '…' : 'Extraire sous-titres'}
                </button>
                <button class="btn btn-ghost" on:click={pickReferenceDialog} disabled={frAudioExtracting || srtExtracting}>
                  {referencePath ? 'Changer' : 'Choisir'}
                </button>
                <button class="btn btn-ghost btn-icon" on:click={() => { clearReference(); showReferenceBar = false; }} title="Retirer la source de référence" disabled={frAudioExtracting || srtExtracting}>✕</button>
              </div>
            </div>
          {:else}
            <div class="source-row">
              <div class="source-num">3</div>
              <div class="source-info">
                <div class="source-label">Source de référence (extraction sous-titres + FR audio sur demande)</div>
                <div class="source-value empty">— Aucun fichier de référence —</div>
              </div>
              <button class="btn btn-ghost" on:click={() => { showReferenceBar = true; pickReferenceDialog(); }}>Choisir</button>
            </div>
          {/if}

        {/if}
        </div><!-- /source-list -->
      </div><!-- /card sources -->
      {/if}

      <!-- Queue déplacée dans la card secondaire en bas droite (tab "Queue"). -->


      <!-- Tracks (vidéo + audio + subs) — vue compacte mockup. Cachées tant que TMDB pas validé pour qu'on reste visuellement sur l'accueil. -->
      {#if tracks.length > 0 && tmdbValidated}
        {@const audioCount = tracks.filter(t=>t.type==='audio').length + externalAudios.length + secondarySelected.filter(t=>t.type==='audio').length}
        {@const subCount   = tracks.filter(t=>t.type==='subtitles').length + externalSubs.length + secondarySelected.filter(t=>t.type==='subtitles').length}
        {@const videoCount = tracks.filter(t=>t.type==='video').length}
        <div class="card card-grow">
          <div class="card-title-row">
            <div class="card-title">🎞️ Pistes ({videoCount} vidéo · {audioCount} audio · {subCount} subs)</div>
            <div class="card-title-actions">
              <button class="btn btn-ghost btn-tiny" on:click={pickAudioDialog}>+ audio</button>
              <button class="btn btn-ghost btn-tiny" on:click={pickSubsDialog}>+ sub</button>
            </div>
          </div>

          <!-- Vidéo -->
          {#if videoCount > 0}
            <div class="tracks-section">
              <div class="tracks-section-header video"><span class="tracks-section-dot"></span><span class="tracks-section-label">▶ Piste vidéo</span><span class="tracks-section-count">{videoCount}</span></div>
              {#each tracks.filter(t => t.type === 'video') as t}
                <div class="track">
                  <div class="track-icon video">▶</div>
                  <div class="track-label">#{t.id} · {t.codec} · {t.pixelDims || ''} → {previewVideoName}</div>
                  <div class="track-flag">{videoChoice.quality}</div>
                </div>
              {/each}
            </div>
          {/if}

          <!-- Audio (merge internal/external/secondary triés par ordre) -->
          {#if tracks.some(t => t.type === 'audio') || externalAudios.length > 0 || secondarySelected.some(t => t.type === 'audio')}
            {@const internalAudios = tracks.filter(t => t.type === 'audio')}
            {@const secondaryAudios = secondarySelected.filter(t => t.type === 'audio')}
            {@const mergedAudios = [
              ...internalAudios.map((t, i) => ({ kind: 'internal', idx: i, ref: t, order: t.order ?? 0 })),
              ...externalAudios.map((a, i) => ({ kind: 'external', idx: i, ref: a, order: a.order ?? 0 })),
              ...secondaryAudios.map((s, i) => ({ kind: 'secondary', idx: i, ref: s, order: s.order ?? 0 })),
            ].sort((a, b) => a.order - b.order)}
            <div class="tracks-section">
              <div class="tracks-section-header audio"><span class="tracks-section-dot"></span><span class="tracks-section-label">♪ Pistes audio</span><span class="tracks-section-count">{audioCount}</span></div>
            {#each mergedAudios as item (item.kind + '-' + item.idx)}
              <div class="track track-editable" class:dropped={!item.ref.keep}>
                <div class="track-icon audio">♪</div>
                <div class="track-body">
                  <div class="track-label">
                    {#if item.kind === 'internal'}
                      #{item.ref.id} · {item.ref.codec} · {item.ref.lang || '??'} · {item.ref.channels || '?'}ch
                    {:else if item.kind === 'secondary'}
                      SUPPLY · #{item.ref.id} · {item.ref.codec || ''} · {item.ref.language || '??'}
                    {:else}
                      EXT · {basename(item.ref.path)}{item.ref.size != null && item.ref.size >= 0 ? ' · ' + formatBytes(item.ref.size) : ''}
                    {/if}
                  </div>
                  <div class="track-controls">
                    <select bind:value={item.ref.label}>
                      <option value="">— label —</option>
                      {#each options.audio_labels as lbl}<option>{lbl}</option>{/each}
                    </select>
                    <label class="chk"><input type="checkbox" bind:checked={item.ref.keep}/>Keep</label>
                    <label class="chk"><input type="checkbox" bind:checked={item.ref.default}/>Default</label>
                    <label class="chk"><input type="checkbox" bind:checked={item.ref.forced}/>Forced</label>
                    <button class="btn-arrow" title="Monter" on:click={() => moveAudioTrack(item.kind, item.idx, -1)}>↑</button>
                    <button class="btn-arrow" title="Descendre" on:click={() => moveAudioTrack(item.kind, item.idx, +1)}>↓</button>
                    {#if item.kind === 'internal'}
                      <button class="btn-arrow danger" title="Supprimer" on:click={() => removeInternalTrack(item.ref.id)}>✕</button>
                    {:else if item.kind === 'secondary'}
                      <button class="btn-arrow danger" title="Retirer du SUPPLY" on:click={() => removeSecondaryTrack(item.idx, 'audio')}>✕</button>
                    {:else}
                      <button class="btn-arrow danger" title="Supprimer" on:click={() => removeExternalAudio(item.idx)}>✕</button>
                    {/if}
                  </div>
                </div>
                {#if item.ref.default}
                  <div class="track-flag success">Default</div>
                {:else if item.ref.forced}
                  <div class="track-flag warn">Forced</div>
                {:else if !item.ref.keep}
                  <div class="track-flag err">Drop</div>
                {:else}
                  <div class="track-flag">Keep</div>
                {/if}
              </div>
            {/each}
            </div>
          {/if}

          <!-- Subtitles -->
          {#if tracks.some(t => t.type === 'subtitles') || externalSubs.length > 0 || secondarySelected.some(t => t.type === 'subtitles')}
            {@const internalSubs = tracks.filter(t => t.type === 'subtitles')}
            {@const secondarySubs = secondarySelected.filter(t => t.type === 'subtitles')}
            {@const mergedSubs = [
              ...internalSubs.map((t, i) => ({ kind: 'internal', idx: i, ref: t, order: t.order ?? 0 })),
              ...externalSubs.map((s, i) => ({ kind: 'external', idx: i, ref: s, order: s.order ?? 0 })),
              ...secondarySubs.map((s, i) => ({ kind: 'secondary', idx: i, ref: s, order: s.order ?? 0 })),
            ].sort((a, b) => a.order - b.order)}
            <div class="tracks-section">
              <div class="tracks-section-header sub"><span class="tracks-section-dot"></span><span class="tracks-section-label">A Sous-titres</span><span class="tracks-section-count">{subCount}</span></div>
            {#each mergedSubs as item (item.kind + '-' + item.idx)}
              <div class="track track-editable">
                <div class="track-icon sub">A</div>
                <div class="track-body">
                  <div class="track-label">
                    {#if item.kind === 'internal'}
                      #{item.ref.id} · {item.ref.codec} · {item.ref.lang || '??'}
                    {:else if item.kind === 'secondary'}
                      SUPPLY · #{item.ref.id} · {item.ref.codec || ''} · {item.ref.language || '??'}
                    {:else}
                      EXT · {basename(item.ref.path)}{item.ref.size != null && item.ref.size >= 0 ? ' · ' + formatBytes(item.ref.size) : ''}
                    {/if}
                  </div>
                  <div class="track-controls">
                    <select bind:value={item.ref.label} on:change={onSubLabelChange}>
                      <option value="">— label —</option>
                      {#each options.subtitle_labels as lbl}<option>{lbl}</option>{/each}
                    </select>
                    <label class="chk"><input type="checkbox" bind:checked={item.ref.keep}/>Keep</label>
                    <label class="chk"><input type="checkbox" bind:checked={item.ref.default}/>Default</label>
                    <label class="chk"><input type="checkbox" bind:checked={item.ref.forced}/>Forced</label>
                    {#if item.kind === 'internal' && isPGSTrack(item.ref)}
                      <button
                        class="btn-arrow"
                        title={ocrRunning ? `OCR en cours…` : 'OCR PGS → SRT (Tesseract)'}
                        disabled={ocrRunning}
                        on:click={() => runOCR(item.ref)}
                      >🔠</button>
                    {:else if item.kind === 'external' && isPGSExternal(item.ref)}
                      <button
                        class="btn-arrow"
                        title={ocrRunning ? `OCR en cours…` : 'OCR PGS .sup → SRT (Tesseract)'}
                        disabled={ocrRunning}
                        on:click={() => runOCRExternalSup(item.idx)}
                      >🔠</button>
                    {/if}
                    <button class="btn-arrow" title="Monter" on:click={() => moveTrack(item.kind, item.idx, -1)}>↑</button>
                    <button class="btn-arrow" title="Descendre" on:click={() => moveTrack(item.kind, item.idx, +1)}>↓</button>
                    {#if item.kind === 'internal'}
                      <button class="btn-arrow danger" title="Supprimer" on:click={() => removeInternalTrack(item.ref.id)}>✕</button>
                    {:else if item.kind === 'secondary'}
                      <button class="btn-arrow danger" title="Retirer du SUPPLY" on:click={() => removeSecondaryTrack(item.idx, 'subtitles')}>✕</button>
                    {:else}
                      <button class="btn-arrow danger" title="Supprimer" on:click={() => removeExternalSub(item.idx)}>✕</button>
                    {/if}
                  </div>
                  {#if (item.kind === 'internal' && ocrTrackId === item.ref.id && ocrProgress.status) || (item.kind === 'external' && ocrTrackId === -1000 - item.idx && ocrProgress.status)}
                    <div class="ocr-progress"
                         class:ocr-running={ocrRunning}
                         class:ocr-done={!ocrRunning && ocrProgress.status === 'done'}
                         class:ocr-error={!ocrRunning && ocrProgress.status === 'error'}
                         title={ocrProgress.message}>
                      <div class="ocr-progress-bar" style="width: {ocrProgress.percent}%"></div>
                      <div class="ocr-progress-label">
                        {#if ocrRunning}
                          🔠 OCR · {ocrProgress.status || '…'} · <b>{ocrProgress.percent}%</b>
                          {#if ocrProgress.message}— {ocrProgress.message}{/if}
                        {:else if ocrProgress.status === 'done'}
                          ✅ <b>OCR terminé</b> — qualité estimée <b>{ocrProgress.quality_score.toFixed(1)}%</b>
                          ({ocrProgress.subtitles} sous-titres · {ocrProgress.corrected_lines} lignes nettoyées · {ocrProgress.lt_auto_fixed} corrections auto · {ocrProgress.lt_needs_review} à vérifier)
                          {#if ocrProgress.lt_needs_review > 0}
                            <button class="btn btn-ghost btn-tiny" style="margin-left:8px" on:click={() => showLTReview = true}>Voir les {ocrProgress.lt_needs_review} lignes à vérifier</button>
                          {/if}
                        {:else if ocrProgress.status === 'error'}
                          ❌ OCR échoué — {ocrProgress.message}
                        {/if}
                      </div>
                    </div>
                  {/if}
                </div>
                {#if item.ref.forced}
                  <div class="track-flag warn">Forced</div>
                {:else if item.ref.default}
                  <div class="track-flag success">Default</div>
                {:else if !item.ref.keep}
                  <div class="track-flag err">Drop</div>
                {:else}
                  <div class="track-flag">Keep</div>
                {/if}
              </div>
            {/each}
            </div>
          {/if}

          {#if !tracks.some(t => t.type === 'audio') && externalAudios.length === 0 && secondarySelected.length === 0}
            <div class="empty-hint">Aucune piste audio détectée. Clique "+ audio" pour ajouter un audio externe.</div>
          {/if}
        </div>
      {/if}

    </div><!-- /col gauche -->

    <!-- COLONNE DROITE -->
    <div class="col">

      <!-- Empty state : visible quand aucune source OU quand source chargée mais TMDB pas encore validé (trouvé OU pas trouvé) -->
      {#if (!sourcePath && tracks.length === 0) || (sourcePath && !tmdbValidated)}
        <div class="card empty-hero" class:compact={sourcePath}>
          <div class="empty-hero-aurora" aria-hidden="true"></div>
          <div class="empty-hero-content">
            <div class="empty-hero-badge">{muxMode === 'psa' ? 'CUSTOM PSA · SUPPLY/FW MUX' : 'LiHDL · FILM MUX PIPELINE'}</div>
            <div class="empty-hero-icon">
              <span class="empty-hero-icon-bg" aria-hidden="true"></span>
              <span class="empty-hero-icon-glyph">{sourcePath ? '✓' : '▶'}</span>
            </div>
            <div class="empty-hero-title">{sourcePath ? 'Source chargée.' : 'Prêt à muxer.'}</div>
            <div class="empty-hero-sub">
              {#if sourcePath}
                <b class="mono">{sourcePath.split('/').pop()}</b><br>
                Confirme la fiche TMDB ci-dessous pour passer aux pistes & réglages.
              {:else}
                Charge un fichier <b class="mono">.mkv</b> source pour démarrer.<br>
                Pistes, réglages TMDB, validation et boutons MUX apparaîtront ici.
              {/if}
            </div>
            {#if !sourcePath}
              <div class="empty-hero-cta-row">
                <button class="btn btn-accent empty-hero-cta" on:click={pickMkvDialog}>
                  <span>📁</span> Choisir un MKV
                </button>
                <button class="btn empty-hero-cta-secondary" on:click={pickMultipleMkvDialog}>
                  <span>📋</span> Choisir plusieurs (queue)
                </button>
              </div>
              <div class="empty-hero-droptip">↓ Ou glisse plusieurs MKV ici pour les mettre en file ↓</div>
              <div class="empty-hero-hints">
                <span class="empty-hero-hint">⚡ MUX rapide bit-à-bit</span>
                <span class="empty-hero-hint-dot">·</span>
                <span class="empty-hero-hint">🎬 Fiche TMDB auto</span>
                <span class="empty-hero-hint-dot">·</span>
                <span class="empty-hero-hint">🔊 Sync audio AC3/EAC3</span>
              </div>

              <!-- Section "Outils additionnels" intégrée DANS la card "Prêt à muxer", pleine largeur. -->
              <div class="empty-hero-tools">
                <div class="empty-hero-tools-title">🔊 Outils additionnels</div>
                <div class="empty-hero-tools-row">
                  <button class="btn btn-ghost" on:click={pickAndOCRStandaloneSup} disabled={ocrRunning} title="OCR PGS .sup → SRT (Tesseract + regex + LanguageTool)">🔠 OCR PGS → SRT</button>
                  <button class="btn btn-ghost" on:click={() => openOSModal('standalone')} title="Chercher un SRT existant sur OpenSubtitles (gain de minutes vs OCR)">🔍 OpenSubtitles</button>
                  <button class="btn btn-ghost" on:click={() => screen = 'sync'} title="Synchroniser des pistes audio (.ac3/.mka/.mkv)">🔊 SYNCHRO AUDIOS</button>
                </div>
                {#if ocrTrackId === -2000 && ocrProgress.status}
                  <div class="ocr-progress"
                       class:ocr-running={ocrRunning}
                       class:ocr-done={!ocrRunning && ocrProgress.status === 'done'}
                       class:ocr-error={!ocrRunning && ocrProgress.status === 'error'}
                       title={ocrProgress.message}
                       style="margin-top:8px">
                    <div class="ocr-progress-bar" style="width: {ocrProgress.percent}%"></div>
                    <div class="ocr-progress-label">
                      {#if ocrRunning}
                        🔠 OCR · {ocrProgress.status || '…'} · <b>{ocrProgress.percent}%</b>
                        {#if ocrProgress.message}— {ocrProgress.message}{/if}
                      {:else if ocrProgress.status === 'done'}
                        ✅ <b>OCR terminé</b> — qualité estimée <b>{ocrProgress.quality_score.toFixed(1)}%</b>
                        ({ocrProgress.subtitles} sous-titres
                        {#if ocrProgress.corrected_lines}· {ocrProgress.corrected_lines} regex{/if}
                        {#if ocrProgress.lt_auto_fixed}· {ocrProgress.lt_auto_fixed} LT auto{/if}
                        {#if ocrProgress.lt_needs_review}· {ocrProgress.lt_needs_review} à vérifier{/if})
                      {:else if ocrProgress.status === 'error'}
                        ❌ OCR échoué — {ocrProgress.message}
                      {/if}
                    </div>
                  </div>
                {/if}
              </div>
            {/if}
          </div>
        </div>
      {/if}

      <!-- Card "aucune fiche TMDB" : même layout que la card validation, mais avec
           une input de recherche au lieu de la fiche identifiée. -->
      {#if sourcePath && !tmdbValidated && !lastTmdbResult && tmdbQuery}
        <div class="card tmdb-validate-card">
          <div class="tmdb-validate-header">
            <div class="tmdb-validate-badge">⚠ AUCUNE FICHE TMDB — RECHERCHE MANUELLE</div>
            <div class="tmdb-validate-force-mini" title="Forcer un ID TMDB">
              <span class="tmdb-validate-force-mini-hash">#</span>
              <input
                type="text"
                class="tmdb-validate-force-mini-input"
                bind:value={tmdbIdQuery}
                placeholder="ID TMDB"
                inputmode="numeric"
                pattern="[0-9]*"
                on:keydown={(e) => e.key === 'Enter' && searchTmdbById()}
              />
              <button class="tmdb-validate-force-mini-btn" on:click={searchTmdbById} disabled={tmdbSearching || !tmdbIdQuery} title="Forcer cet ID">
                {tmdbSearching ? '⏳' : '↻'}
              </button>
            </div>
          </div>
          <div class="tmdb-validate-body">
            <div class="tmdb-validate-poster placeholder">🎞️</div>
            <div class="tmdb-validate-info">
              <div class="tmdb-validate-title" style="opacity:0.7;font-style:italic;">Aucun film identifié automatiquement</div>
              <div class="tmdb-validate-desc" style="margin-top:8px;">
                Édite le titre ci-dessous puis Entrée (ou clique 🔍 Rechercher).<br>
                Ou colle un ID TMDB dans le champ <b class="mono">#</b> en haut à droite.
              </div>
              <div style="display:flex;gap:8px;align-items:center;margin-top:14px;">
                <input
                  type="text"
                  bind:value={tmdbQuery}
                  placeholder="Titre du film…"
                  style="flex:1;padding:10px 14px;font-size:14px;background:rgba(255,255,255,0.05);border:1px solid rgba(255,255,255,0.15);border-radius:6px;color:#fff;"
                  on:keydown={(e) => e.key === 'Enter' && manualTmdbSearchFromCard()}
                />
              </div>
            </div>
          </div>
          <div class="tmdb-validate-actions">
            <button class="btn btn-accent tmdb-validate-cta" on:click={manualTmdbSearchFromCard} disabled={tmdbSearching || !tmdbQuery.trim()}>
              {tmdbSearching ? '⏳ Recherche…' : '🔍 Rechercher sur TMDB'}
            </button>
          </div>
        </div>
      {/if}

      <!-- Card validation TMDB : confirmation visuelle du film identifié avant de passer à l'étape pistes/réglages -->
      {#if sourcePath && !tmdbValidated && lastTmdbResult}
        <div class="card tmdb-validate-card">
          <div class="tmdb-validate-header">
            <div class="tmdb-validate-badge">🎬 FILM IDENTIFIÉ — CONFIRME POUR CONTINUER</div>
            <div class="tmdb-validate-force-mini" title="Pas le bon film ? Forcer l'ID TMDB">
              <span class="tmdb-validate-force-mini-hash">#</span>
              <input
                type="text"
                class="tmdb-validate-force-mini-input"
                bind:value={tmdbIdQuery}
                placeholder="ID TMDB"
                inputmode="numeric"
                pattern="[0-9]*"
                on:keydown={(e) => e.key === 'Enter' && searchTmdbById()}
              />
              <button class="tmdb-validate-force-mini-btn" on:click={searchTmdbById} disabled={tmdbSearching || !tmdbIdQuery} title="Forcer cet ID">
                {tmdbSearching ? '⏳' : '↻'}
              </button>
            </div>
          </div>
          <div class="tmdb-validate-body">
            {#if lastTmdbResult.poster_url}
              <img src={lastTmdbResult.poster_url} alt="Affiche du film" class="tmdb-validate-poster" />
            {:else}
              <div class="tmdb-validate-poster placeholder">🎞️</div>
            {/if}
            <div class="tmdb-validate-info">
              <div class="tmdb-validate-title">{lastTmdbResult.titre_fr || lastTmdbResult.titre_vo || '—'}{lastTmdbResult.annee_fr ? ` (${lastTmdbResult.annee_fr})` : ''}</div>
              {#if lastTmdbResult.titre_vo && lastTmdbResult.titre_fr && lastTmdbResult.titre_vo !== lastTmdbResult.titre_fr}
                <div class="tmdb-validate-original">VO : {lastTmdbResult.titre_vo}</div>
              {/if}
              <div class="tmdb-validate-meta">
                {#if lastTmdbResult.tmdb_id}<span class="tmdb-validate-pill">TMDB {lastTmdbResult.tmdb_id}</span>{/if}
                {#if lastTmdbResult.duree}<span class="tmdb-validate-pill">⏱ {lastTmdbResult.duree}</span>{/if}
                {#if lastTmdbResult.note > 0}<span class="tmdb-validate-pill rating">★ {lastTmdbResult.note}</span>{/if}
                {#if lastTmdbResult.original_language}<span class="tmdb-validate-pill">🌐 {String(lastTmdbResult.original_language).toUpperCase()}</span>{/if}
              </div>
              {#if lastTmdbResult.description || lastTmdbResult.overview || lastTmdbResult.synopsis}
                <div class="tmdb-validate-desc">{lastTmdbResult.description || lastTmdbResult.overview || lastTmdbResult.synopsis}</div>
              {/if}
            </div>
          </div>
          <div class="tmdb-validate-actions">
            {#if tmdbResults && tmdbResults.length > 1}
              <button class="btn btn-ghost" on:click={cycleNextTmdbResult} title="Cycler vers le résultat suivant si la fiche affichée n'est pas la bonne">
                ↻ Autre résultat ({tmdbResultIndex + 1}/{tmdbResults.length})
              </button>
            {/if}
            <button class="btn btn-accent tmdb-validate-cta" on:click={() => { tmdbValidated = true; }}>✓ C'est bien ce film — continuer</button>
          </div>
        </div>
      {/if}

      <!-- Card FICHE FILM supprimée — infos déjà visibles dans la card validation et le header film-bar. -->

      <!-- Réglages piste vidéo -->
      {#if tracks.length > 0 && tracks.some(t => t.type === 'video') && tmdbValidated}
        <div class="card">
          <div class="card-title">🎛️ Réglages piste vidéo</div>
          <div class="field-grid">
            <div class="field">
              <span class="field-label">Qualité</span>
              <select bind:value={videoChoice.quality}>
                {#each options.video_qualities as q}<option>{q}</option>{/each}
              </select>
            </div>
            <div class="field">
              <span class="field-label">Encodeur</span>
              <select bind:value={videoChoice.encoder}>
                {#each options.video_encoders as e}<option>{e}</option>{/each}
              </select>
            </div>
            <div class="field">
              <span class="field-label">Type source</span>
              <select bind:value={videoChoice.sourceType} on:change={onSourceTypeChange}>
                {#each VIDEO_SOURCE_TYPE_OPTIONS as s}<option>{s}</option>{/each}
              </select>
            </div>
            <div class="field">
              <span class="field-label">Team source</span>
              <input type="text" bind:value={videoChoice.sourceTeam} placeholder="ex: Alkaline" />
            </div>
            <div class="field">
              <span class="field-label">Résolution</span>
              <select bind:value={target.resolution}>
                {#each RESOLUTION_OPTIONS as r}<option>{r}</option>{/each}
              </select>
            </div>
            <div class="field">
              <span class="field-label">Source (sortie)</span>
              <select bind:value={target.source}>
                {#each TARGET_SOURCE_OPTIONS as s}<option>{s}</option>{/each}
              </select>
            </div>
            <div class="field">
              <span class="field-label">Codec vidéo</span>
              <select bind:value={target.video_codec}>
                {#each VIDEO_CODEC_OPTIONS as c}<option>{c}</option>{/each}
              </select>
            </div>
            <div class="field">
              <span class="field-label">Team (sortie)</span>
              <select bind:value={videoChoice.team}>
                {#each options.video_teams as t}<option>{t}</option>{/each}
              </select>
            </div>
            <div class="field">
              <span class="field-label">Flag langue</span>
              <select bind:value={target.flagOverride}>
                {#each FLAG_OVERRIDE_OPTIONS as f}
                  <option value={f}>{f === 'auto' ? 'Auto' : f}</option>
                {/each}
              </select>
            </div>
            <div class="field">
              <span class="field-label">Titre</span>
              <input type="text" bind:value={target.title} placeholder="Titre" />
            </div>
            <div class="field">
              <span class="field-label">{target.episode ? 'Épisode' : 'Année'}</span>
              {#if target.episode}
                <input type="text" bind:value={target.episode} placeholder="S01E01" maxlength="10" />
              {:else}
                <input type="text" bind:value={target.year} placeholder="2025" maxlength="4" />
              {/if}
            </div>
            <div class="field">
              <span class="field-label">Codec auto-suggéré</span>
              <input type="text" value={suggestedCodecDisplay || '—'} readonly />
            </div>
          </div>
        </div>
      {/if}

      <!-- Filename preview -->
      {#if tracks.length > 0 && tmdbValidated}
        <div class="card">
          <div class="card-title">📝 Nom de fichier final</div>
          {#if lastTmdbResult}
            <div class="lang-toggle" style:margin-bottom="6px">
              <button class:active={target.lang === 'auto'} on:click={() => target.lang = 'auto'}>Auto</button>
              <button class:active={target.lang === 'vf'}   on:click={() => target.lang = 'vf'}>VF</button>
              <button class:active={target.lang === 'vo'}   on:click={() => target.lang = 'vo'}>VO</button>
            </div>
          {/if}
          {#if filenameOverride}
            <input class="filename-input mono" type="text" bind:value={manualFilename} placeholder="nom-de-fichier.mkv" />
            <div class="filename-actions">
              <button class="btn btn-ghost btn-tiny" on:click={resetFilenameOverride} title="Revenir à l'auto">↺ Auto</button>
              {#if effectiveFilename}
                <button class="btn btn-ghost btn-tiny" on:click={copyFilename}>{filenameCopied ? '✓ Copié' : '📋 Copier'}</button>
                <button class="btn btn-ghost btn-tiny" on:click={openOutputDir} disabled={!effectiveOutputDir()}>📂 Dossier</button>
              {/if}
            </div>
          {:else}
            <div class="filename mono">{previewFilename || '—'}</div>
            <div class="filename-actions">
              {#if previewFilename}
                <button class="btn btn-ghost btn-tiny" on:click={startFilenameOverride} title="Modifier manuellement">✏ Modifier</button>
              {/if}
              {#if effectiveFilename}
                <button class="btn btn-ghost btn-tiny" on:click={copyFilename}>{filenameCopied ? '✓ Copié' : '📋 Copier'}</button>
                <button class="btn btn-ghost btn-tiny" on:click={openOutputDir} disabled={!effectiveOutputDir()}>📂 Dossier</button>
              {/if}
            </div>
          {/if}
        </div>
      {/if}

      <!-- Toggle FR VFi + liens externes (TMDB / Wikipédia / Hydra / UNFR) -->
      {#if lastTmdbResult}
        <div class="card vfq-links-card">
          <div class="card-title">🌐 Langues, Sous-Titres & Liens</div>
          {#if muxMode === 'lihdl'}
            <label class="vfq-toggle">
              <input type="checkbox" bind:checked={useVFi} on:change={applyVFiSwap} />
              <span class:vfq-yes={useVFi} class:vfq-no={!useVFi}>
                {useVFi ? '✓ FR VFi (international)' : '☐ FR VFF (France)'}
              </span>
              {#if lastTmdbResult.tmdb_id}
                <button class="vfq-link" type="button" on:click={() => OpenURL(`https://www.themoviedb.org/movie/${lastTmdbResult.tmdb_id}/translations`)}>TMDB ↗</button>
                <button class="vfq-link" type="button" on:click={() => OpenURL(`https://fr.wikipedia.org/w/index.php?title=Special:Search&go=Go&search=${encodeURIComponent(lastTmdbResult.titre_fr || lastTmdbResult.titre_vo || '')}`)}>Wikipédia ↗</button>
              {/if}
            </label>
          {/if}
          {#if lastTmdbResult.tmdb_id}
            <div class="vfq-toggle" style:margin-top="6px">
              <span class="srt-label">🔍 Sous-titres SRT :</span>
              <button class="vfq-link" type="button" on:click={() => OpenURL(hydrackerURL || `https://hydracker.com/titles?search=${encodeURIComponent(lastTmdbResult.titre_fr || lastTmdbResult.titre_vo || '')}`)}>Hydra ↗</button>
              <button class="vfq-link" type="button" on:click={() => OpenURL(`https://unfr.pw/?d=fiche&movieid=${lastTmdbResult.tmdb_id}`)}>UNFR ↗</button>
            </div>
          {/if}
        </div>
      {/if}

      <!-- Output dir : géré dans les Réglages, plus affiché ici. -->

      <!-- PSA secondary status (mode PSA) -->
      {#if muxMode === 'psa' && secondarySelected.length > 0}
        <div class="card">
          <div class="card-title">✓ SUPPLY/FW prêt</div>
          <div class="validation">
            <span class="pill ok">✓ {secondarySelected.filter(t=>t.type==='audio').length} audio(s)</span>
            <span class="pill ok">✓ {secondarySelected.filter(t=>t.type==='subtitles').length} sub(s)</span>
          </div>
        </div>
      {/if}

      <!-- Audio sync screen access -->
      {#if tracks.length > 0 && tmdbValidated}
        <div class="card">
          <div class="card-title">🔊 Outils additionnels</div>
          <div class="tools-row">
            <button class="btn btn-ghost btn-tiny" on:click={() => screen = 'sync'}>🔊 Synchro audios</button>
            <button class="btn btn-ghost btn-tiny" on:click={() => screen = 'cible'}>🎯 Vue Cible détaillée</button>
            <button class="btn btn-ghost btn-tiny" on:click={pickAndOCRStandaloneSup} disabled={ocrRunning} title="OCR PGS .sup → SRT (Tesseract + cleanup regex FR)">🔠 OCR PGS → SRT</button>
            <button class="btn btn-ghost btn-tiny" on:click={() => openOSModal('post-source')} title="Chercher un SRT existant sur OpenSubtitles">🔍 OpenSubtitles</button>
          </div>
          {#if ocrTrackId === -2000 && ocrProgress.status}
            <div class="ocr-progress"
                 class:ocr-running={ocrRunning}
                 class:ocr-done={!ocrRunning && ocrProgress.status === 'done'}
                 class:ocr-error={!ocrRunning && ocrProgress.status === 'error'}
                 title={ocrProgress.message}
                 style="margin-top:8px">
              <div class="ocr-progress-bar" style="width: {ocrProgress.percent}%"></div>
              <div class="ocr-progress-label">
                {#if ocrRunning}
                  🔠 OCR · {ocrProgress.status || '…'} · <b>{ocrProgress.percent}%</b>
                  {#if ocrProgress.message}— {ocrProgress.message}{/if}
                {:else if ocrProgress.status === 'done'}
                  ✅ <b>OCR terminé</b> — qualité estimée <b>{ocrProgress.quality_score.toFixed(1)}%</b>
                  ({ocrProgress.subtitles} sous-titres · {ocrProgress.corrected_lines} lignes nettoyées · {ocrProgress.lt_auto_fixed} corrections auto · {ocrProgress.lt_needs_review} à vérifier)
                  {#if ocrProgress.lt_needs_review > 0}
                    <button class="btn btn-ghost btn-tiny" style="margin-left:8px" on:click={() => showLTReview = true}>Voir les {ocrProgress.lt_needs_review} lignes à vérifier</button>
                  {/if}
                {:else if ocrProgress.status === 'error'}
                  ❌ OCR échoué — {ocrProgress.message}
                {/if}
              </div>
            </div>
          {/if}
        </div>
      {/if}

      <!-- Card secondaire en bas droite : tabs Journal | Queue -->
      {#if sourcePath && tmdbValidated}
        <div class="card journal-card journal-inline">
          <div class="bottom-pane-tabs">
            <button class="bottom-pane-tab" class:active={bottomPaneTab === 'journal'} on:click={() => bottomPaneTab = 'journal'}>
              📜 Journal{logLines.length > 0 ? ` (${logLines.length})` : ''}
            </button>
            <button class="bottom-pane-tab" class:active={bottomPaneTab === 'queue'} on:click={() => bottomPaneTab = 'queue'}>
              📋 Queue{queue.length > 0 ? ` (${queue.length})` : ''}
            </button>
          </div>

          {#if bottomPaneTab === 'journal'}
            <div class="journal-scroll" bind:this={logEl}>
              {#if logLines.length === 0}
                <div class="journal-line"><span class="log-time">--:--:--</span><span class="log-msg lvl-info">Journal vide.</span></div>
              {:else}
                {#each logLines as l}
                  <div class="journal-line">
                    <span class="log-time">{l.time}</span>
                    <span class="log-msg lvl-{l.level}">{l.text}</span>
                  </div>
                {/each}
              {/if}
            </div>
          {:else}
            <!-- bind:this conservé caché pour ne pas casser appendLog/logEl scroll -->
            <div hidden bind:this={logEl}>{#each logLines as l}<span>{l.text}</span>{/each}</div>
            <div class="queue-pane">
              {#if queue.length === 0}
                <div class="queue-empty">
                  <div class="queue-empty-icon">📋</div>
                  <div class="queue-empty-title">File vide</div>
                </div>
              {:else}
                <ul class="queue-list">
                  {#each queue as p, i}
                    <li class="queue-row" class:current={p === sourcePath}>
                      <span class="queue-idx">{i + 1}</span>
                      <span class="queue-name mono" title={p}>{p.split('/').pop()}</span>
                      <button class="btn btn-ghost btn-tiny" on:click={() => queueLoad(i)} disabled={muxing}>Charger</button>
                      <button class="btn btn-ghost btn-icon" on:click={() => queueRemove(i)} title="Retirer">✕</button>
                    </li>
                  {/each}
                </ul>
                <div class="queue-actions">
                  <button class="btn btn-ghost btn-tiny" on:click={queueNext} disabled={muxing}>↓ Charger le suivant</button>
                  <button class="btn btn-ghost btn-tiny" on:click={() => queue = []}>🗑 Vider</button>
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {/if}

    </div><!-- /col droite -->

    {:else if screen === 'cible'}
    <div class="fullscreen">
      <div class="card">
        <div class="card-title">Recherche TMDB</div>
        <div class="lang-toggle" style:margin-bottom="8px">
          <button class:active={tmdbMode === 'movie'} on:click={() => tmdbMode = 'movie'}>🎬 Film</button>
          <button class:active={tmdbMode === 'tv'}    on:click={() => tmdbMode = 'tv'}>📺 Série</button>
        </div>
        <div class="field-row">
          <input type="text" bind:value={tmdbQuery} placeholder={tmdbMode === 'tv' ? 'Titre de série…' : 'Titre du film…'} on:keydown={(e) => e.key === 'Enter' && searchTmdb()} />
          <button class="btn btn-accent" on:click={searchTmdb} disabled={tmdbSearching}>{tmdbSearching ? '…' : 'Chercher par titre'}</button>
        </div>
        <div class="field-row" style:margin-top="6px">
          <input type="text" bind:value={tmdbIdQuery} placeholder="ID TMDB numérique (ex: 12345)…" inputmode="numeric" pattern="[0-9]*" on:keydown={(e) => e.key === 'Enter' && searchTmdbById()} />
          <button class="btn btn-accent" on:click={searchTmdbById} disabled={tmdbSearching}>{tmdbSearching ? '…' : 'Chercher par ID'}</button>
        </div>
        {#if tmdbMode === 'tv' && !config.tmdb_key}
          <div class="field-hint">⚠ Clé API TMDB requise pour chercher des séries — Réglages.</div>
        {/if}
        <!-- Card de vérification : fiche TMDB sélectionnée -->
        {#if lastTmdbResult}
          <div class="tmdb-picked">
            {#if lastTmdbResult.poster_url}
              <img class="tmdb-picked-poster" src={lastTmdbResult.poster_url} alt="" />
            {/if}
            <div class="tmdb-picked-body">
              <div class="tmdb-picked-title">
                {lastTmdbResult.titre_fr || lastTmdbResult.titre_vo}
                {#if lastTmdbResult.annee_fr}<span class="tmdb-year">({lastTmdbResult.annee_fr})</span>{/if}
              </div>
              {#if lastTmdbResult.titre_vo && lastTmdbResult.titre_vo !== lastTmdbResult.titre_fr}
                <div class="tmdb-picked-vo">VO : {lastTmdbResult.titre_vo}</div>
              {/if}
              <div class="tmdb-picked-meta">
                {#if lastTmdbResult.duree}{lastTmdbResult.duree} · {/if}⭐ {lastTmdbResult.note || '?'} · ID {lastTmdbResult.tmdb_id}
              </div>
              {#if muxMode === 'lihdl'}
                <label class="vfq-toggle">
                  <input type="checkbox" bind:checked={useVFi} on:change={applyVFiSwap} />
                  <span class:vfq-yes={useVFi} class:vfq-no={!useVFi}>
                    {useVFi ? '✓ FR VFi (doublage international)' : '☐ FR VFF (doublage France métropolitaine)'}
                  </span>
                </label>
              {/if}
              {#if lastTmdbResult.overview}
                <div class="tmdb-picked-overview">{lastTmdbResult.overview}</div>
              {:else if !config.tmdb_key}
                <div class="field-hint">💡 Renseigne ta clé API TMDB dans Réglages pour afficher la description.</div>
              {/if}
            </div>
          </div>
        {/if}
        <!-- Liste affichée seulement si pas encore picked OU si l'utilisateur
             veut explicitement voir les alternatives. Limitée à 3 résultats. -->
        {#if tmdbResults.length > 0 && !lastTmdbResult}
          <ul class="tmdb-list">
            {#each tmdbResults.slice(0, 3) as r}
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
        {:else if tmdbResults.length > 1 && lastTmdbResult}
          <details class="tmdb-alts">
            <summary>Voir les autres résultats ({Math.min(tmdbResults.length - 1, 2)})</summary>
            <ul class="tmdb-list">
              {#each tmdbResults.filter(r => r.tmdb_id !== lastTmdbResult.tmdb_id).slice(0, 2) as r}
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
          </details>
        {/if}
      </div>

      <div class="card">
        <div class="card-title">Titre cible</div>
        {#if target.title}
          <div class="tmdb-preview mono">{target.title}</div>
        {/if}
        <div class="field-row">
          <div class="field" style:flex="3"><label>Titre</label>
            <input type="text" bind:value={target.title} placeholder="Titre du film" />
          </div>
          <div class="field" style:flex="1"><label>{target.episode ? 'Épisode' : 'Année'}</label>
            {#if target.episode}
              <input type="text" bind:value={target.episode} placeholder="S01E01" maxlength="10" />
            {:else}
              <input type="text" bind:value={target.year} placeholder="2025" maxlength="4" />
            {/if}
          </div>
          <div class="field" style:flex="1"><label>Mode</label>
            <select value={target.episode ? 'tv' : 'movie'} on:change={(e) => {
              if (e.currentTarget.value === 'tv' && !target.episode) {
                target.episode = detectEpisode((sourcePath || '').split('/').pop()) || 'S01E01';
              } else if (e.currentTarget.value === 'movie') {
                target.episode = '';
              }
            }}>
              <option value="movie">🎬 Film</option>
              <option value="tv">📺 Série</option>
            </select>
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
            <select bind:value={target.video_codec}>
              {#each VIDEO_CODEC_OPTIONS as c}<option>{c}</option>{/each}
            </select>
          </div>
        </div>
        <div class="field-row">
          <div class="field" style:flex="2"><label>Flag langue</label>
            <select bind:value={target.flagOverride}>
              {#each FLAG_OVERRIDE_OPTIONS as f}
                <option value={f}>{f === 'auto' ? 'Auto (calculé selon audios)' : f}</option>
              {/each}
            </select>
          </div>
          <div class="field" style:flex="1"><label>Team (sortie)</label>
            <select bind:value={videoChoice.team}>
              {#each options.video_teams as t}<option>{t}</option>{/each}
            </select>
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
            {#if filenameOverride}
              <input class="filename-input mono" type="text" bind:value={manualFilename} placeholder="nom-de-fichier.mkv" />
              <button class="btn-copy" on:click={resetFilenameOverride} title="Revenir à l'auto">↺ Auto</button>
            {:else}
              <div class="preview-value mono">{previewFilename || '—'}</div>
              {#if previewFilename}
                <button class="btn-copy" on:click={startFilenameOverride} title="Modifier manuellement">✏ Modifier</button>
              {/if}
            {/if}
            {#if effectiveFilename}
              <button class="btn-copy" on:click={copyFilename} title="Copier">{filenameCopied ? '✓ Copié' : '📋 Copier'}</button>
              <button class="btn-copy" on:click={openOutputDir} disabled={!effectiveOutputDir()} title="Ouvrir dossier de sortie">📂 Dossier</button>
            {/if}
          </div>
        </div>
      </div>

      <div class="actions-row">
        {#if muxing}
          <button class="btn btn-danger" on:click={stopMux}>⏹ Stop</button>
          <div class="progress-bar"><div class="progress-fill" style:width="{muxPercent}%"></div></div>
          <span class="mono">{muxPercent}%</span>
        {:else}
          <button class="btn btn-accent" on:click={doMux} disabled={!sourcePath || !effectiveFilename}>Muxer</button>
        {/if}
      </div>
    </div><!-- /fullscreen cible -->

    {:else if screen === 'sync'}
    <div class="fullscreen">
      <div class="card">
        <div class="card-title">🔊 Synchro des Pistes Audios</div>
        <p class="hint">
          Charge un .mkv contenant plusieurs pistes audio (ex : VFF + VFQ).
          Choisis la piste de <strong>référence</strong> et indique pour chaque autre
          piste son décalage en ms (positif = retarder, négatif = avancer).
          L'audio est copié <strong>bit-à-bit</strong> via <code>mkvmerge --sync</code> —
          aucun réencodage, AC3/EAC3 préservés.
        </p>

        <div class="field">
          <label>Fichier .mkv</label>
          <div style="display:flex; gap:8px; align-items:center;">
            <button class="btn-secondary" on:click={syncPickFile}>
              {syncSourcePath ? 'Changer' : 'Choisir un fichier'}
            </button>
            {#if syncSourcePath}
              <span class="mono" style="opacity:.8; font-size:.85em;">{syncSourcePath}</span>
            {/if}
          </div>
        </div>

        {#if syncAudioTracks.length > 0}
          <div class="section-title" style="margin-top:14px;">Pistes audio</div>
          <table class="tracks-table">
            <thead>
              <tr>
                <th style="width:60px;">Réf</th>
                <th style="width:50px;">ID</th>
                <th>Codec</th>
                <th>Langue</th>
                <th>Nom</th>
                <th style="width:70px;">Canaux</th>
                <th style="width:140px;">Décalage (ms)</th>
                <th style="width:200px;">Détection auto</th>
              </tr>
            </thead>
            <tbody>
              {#each syncAudioTracks as t (t.id)}
                <tr class:ref={t.id === syncRefId}>
                  <td>
                    <input type="radio" name="syncRef" value={t.id}
                           checked={t.id === syncRefId}
                           on:change={() => syncSetRef(t.id)} />
                  </td>
                  <td class="mono">{t.id}</td>
                  <td class="mono">{t.codec || ''}</td>
                  <td class="mono">{t.language || ''}</td>
                  <td>{t.name || ''}</td>
                  <td class="mono">{t.channels || ''}</td>
                  <td>
                    {#if t.id === syncRefId}
                      <span style="opacity:.5;">— référence —</span>
                    {:else}
                      <input type="number" step="1"
                             placeholder="0"
                             bind:value={syncOffsets[t.id]}
                             style="width:110px;" />
                    {/if}
                  </td>
                  <td>
                    {#if t.id === syncRefId}
                      <span style="opacity:.4;">—</span>
                    {:else if syncDetecting[t.id]}
                      <span style="opacity:.7;">⏳ analyse…</span>
                    {:else}
                      <button class="btn-secondary"
                              on:click={() => syncDetectOne(t.id)}
                              disabled={!syncSourcePath}>🎯 Détecter</button>
                      {#if syncResults[t.id]}
                        {@const r = syncResults[t.id]}
                        <div style="font-size:.78em; opacity:.85; margin-top:4px;">
                          conf {r.confidence.toFixed(2)}
                          {#if r.drift_ms > 0} · drift {r.drift_ms} ms{/if}
                        </div>
                        {#if r.method === 'drift_linear'}
                          <div style="margin-top:4px; padding:4px 6px; background:#5a3a00; color:#ffd166; border-radius:4px; font-size:.78em;">
                            ⚠️ FPS différent — resample atempo {r.tempo_factor.toFixed(6)} ({((r.tempo_factor - 1) * 100).toFixed(3)}%) sera appliqué au mux (1 génération AC3/EAC3 perdue).
                          </div>
                        {:else if r.method === 'drift_unstable'}
                          <div style="margin-top:4px; padding:4px 6px; background:#5a1f1f; color:#ffb3b3; border-radius:4px; font-size:.78em;">
                            ⚠️ Drift instable — recalage non fiable, vérifier manuellement.
                          </div>
                        {:else if r.method === 'low_confidence'}
                          <div style="margin-top:4px; padding:4px 6px; background:#3a3a3a; color:#bbb; border-radius:4px; font-size:.78em;">
                            ⚠️ Corrélation faible — résultat peu fiable.
                          </div>
                        {/if}
                      {/if}
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>

          <p class="hint" style="margin-top:14px; opacity:.75;">
            🎯 « Détecter » mesure l'offset par cross-correlation (extraction PCM mono
            8 kHz via ffmpeg, ~3 min en début de film + vérif drift à 85% sur films
            &gt;20 min). « Appliquer » mux un nouveau .mkv en copiant l'audio bit-à-bit
            (AC3/EAC3 préservés).
          </p>

          <div class="actions-row" style="gap:10px;">
            <button class="btn-secondary"
                    on:click={syncDetectAll}
                    disabled={!syncSourcePath || syncRefId === null}>
              🎯 Détecter toutes les pistes
            </button>
            {#if syncRunning}
              <div class="progress-bar"><div class="progress-fill" style:width="{syncPercent}%"></div></div>
              <span class="mono">{syncPercent}%</span>
            {:else}
              <button class="btn-primary"
                      on:click={syncApply}
                      disabled={!syncSourcePath || syncRefId === null}>
                ✅ Appliquer le recalage
              </button>
              {#if syncSourcePath}
                <span class="mono" style="opacity:.7; font-size:.85em;">
                  → {syncDefaultOutput()}
                </span>
              {/if}
            {/if}
          </div>
        {/if}
      </div>
    </div><!-- /fullscreen sync -->

    {:else if screen === 'reglages'}
    <div class="fullscreen">
      <div class="card">
        <div class="card-title">TMDB</div>
        <div class="field"><label>Clé API TMDB</label>
          <div class="field-row">
            <input type="password" bind:value={config.tmdb_key} placeholder="ex: a1b2c3d4e5f6… (themoviedb.org → Settings → API)" />
            <button class="btn-test" on:click={doTestTmdbKey} disabled={tmdbTest.running}>
              {tmdbTest.running ? '…' : 'Test'}
            </button>
          </div>
          {#if tmdbTest.ok !== null}
            <div class="result-badge {tmdbTest.ok ? 'ok' : 'err'}">{tmdbTest.message}</div>
          {/if}
          <div class="field-hint">
            Requise pour : <b>recherche par ID numérique</b>, <b>recherche série TV</b>.
            Sans clé, fallback sur l'index ci-dessous (films seulement).
          </div>
        </div>
        <div class="field"><label>Index de recherche</label>
          <input type="password" value={config.serveurperso_url} disabled readonly style="opacity:0.6;cursor:not-allowed;" />
          <div class="field-hint">Verrouillé (configuration interne LiHDL).</div>
        </div>
        <div class="field"><label>Index de recherche fallback</label>
          <input type="password" value={config.fallback_index} disabled readonly style="opacity:0.6;cursor:not-allowed;" />
          <div class="field-hint">Verrouillé (configuration interne LiHDL).</div>
        </div>
      </div>

      <div class="card">
        <div class="card-title">Recherche sous-titres SRT</div>
        <div class="field"><label>Clé API Hydracker</label>
          <div class="field-row">
            <input type="password" bind:value={config.hydracker_key} placeholder="ex: a1b2c3d4… (hydracker.com/account-settings)" />
            <button class="btn-test" on:click={doTestHydrackerKey} disabled={hydrackerTest.running}>
              {hydrackerTest.running ? '…' : 'Test'}
            </button>
          </div>
          {#if hydrackerTest.ok !== null}
            <div class="result-badge {hydrackerTest.ok ? 'ok' : 'err'}">{hydrackerTest.message}</div>
          {/if}
          <div class="field-hint">
            Utilisée pour récupérer la fiche Hydracker à partir d'un ID TMDB (lien direct vers /titles/&lt;id&gt;/&lt;slug&gt;).
          </div>
        </div>
        <div class="field"><label>Clé API UNFR.pw</label>
          <div class="field-row">
            <input type="password" bind:value={config.unfr_key} placeholder="ex: token-unfr…" />
            <button class="btn-test" on:click={doTestUnfrKey} disabled={unfrTest.running}>
              {unfrTest.running ? '…' : 'Test'}
            </button>
          </div>
          {#if unfrTest.ok !== null}
            <div class="result-badge {unfrTest.ok ? 'ok' : 'err'}">{unfrTest.message}</div>
          {/if}
          <div class="field-hint">
            Utilisée pour authentifier les requêtes vers l'API UNFR.
          </div>
        </div>
      </div>

      <div class="card">
        <div class="card-title">Dossiers de sortie</div>
        <div class="field"><label>⚡ MUX LiHDL (films)</label>
          <div class="field-row">
            <input type="text" bind:value={config.output_dir_lihdl} placeholder="/Users/…/Mux/LiHDL" readonly />
            <button class="btn-test" on:click={pickOutputDirLihdl}>Choisir…</button>
            <button class="btn-test" on:click={() => config.output_dir_lihdl && OpenFolder(config.output_dir_lihdl)} disabled={!config.output_dir_lihdl} title="Ouvrir dans le Finder">📂</button>
          </div>
        </div>
        <div class="field"><label>🎬 MUX CUSTOM PSA SERIES</label>
          <div class="field-row">
            <input type="text" bind:value={config.output_dir_psa} placeholder="/Users/…/Mux/PSA" readonly />
            <button class="btn-test" on:click={pickOutputDirPSA}>Choisir…</button>
            <button class="btn-test" on:click={() => config.output_dir_psa && OpenFolder(config.output_dir_psa)} disabled={!config.output_dir_psa} title="Ouvrir dans le Finder">📂</button>
          </div>
        </div>
        <div class="field"><label>Dossier fallback (si l'un des 2 ci-dessus est vide)</label>
          <div class="field-row">
            <input type="text" bind:value={config.output_dir} placeholder="/Users/…/Mux" readonly />
            <button class="btn-test" on:click={pickOutputDir}>Choisir…</button>
            <button class="btn-test" on:click={() => config.output_dir && OpenFolder(config.output_dir)} disabled={!config.output_dir} title="Ouvrir dans le Finder">📂</button>
          </div>
        </div>
      </div>

      <div class="card">
        <div class="card-title">LanguageTool (post-OCR)</div>
        <div class="field"><label>Username LanguageTool (optionnel — Premium)</label>
          <input type="text" bind:value={config.languagetool_user} placeholder="ex: davidfernandez06@gmail.com" />
        </div>
        <div class="field"><label>Clé API LanguageTool (optionnel — Premium)</label>
          <div class="field-row">
            <input type="password" bind:value={config.languagetool_key} placeholder="ex: lt-xxxxxx (laisse vide pour API publique gratuite)" />
            <button class="btn-test" on:click={doTestLanguageToolKey} disabled={ltTest.running}>
              {ltTest.running ? '…' : 'Test'}
            </button>
          </div>
          {#if ltTest.ok !== null}
            <div class="result-badge {ltTest.ok ? 'ok' : 'err'}">{ltTest.message}</div>
          {/if}
          <div class="field-hint">
            Sans clé : API publique gratuite (20 req/min, 20 KB/req). Avec clé Premium : pas de rate-limit pratique. Endpoint Premium auto-détecté.
          </div>
        </div>
        <div class="field"><label>URL endpoint (optionnel — override)</label>
          <input type="text" bind:value={config.languagetool_url} placeholder="https://api.languagetool.org/v2/check" />
        </div>
      </div>

      <div class="card">
        <div class="card-title">OpenSubtitles (recherche SRT existant avant OCR)</div>
        <div class="field"><label>Clé API OpenSubtitles</label>
          <input type="password" bind:value={config.opensubtitles_api_key} placeholder="ex: abc123… (opensubtitles.com → Profile → Consumers)" />
          <div class="field-hint">
            Crée une clé gratuite sur <b>opensubtitles.com</b> → Profile → Consumers (compte requis).
            Le User-Agent par défaut <b class="mono">GoMuxLiHDL v5.x</b> doit être enregistré sur ce compte.
          </div>
        </div>
      </div>

      <div class="card">
        <div class="card-title">Dictionnaire OCR custom</div>
        <div class="field-hint">
          Mappings « texte fautif → correction » appliqués automatiquement à la fin du cleanup OCR.
          Les entrées <b>auto</b> sont ajoutées quand tu valides une correction dans le modal "Lignes à vérifier".
        </div>
        <div class="custom-dict-list">
          {#if !customDictEntries || customDictEntries.length === 0}
            <div class="empty-hint">Aucune entrée — clique « + Ajouter » ou valide une correction dans le modal LT.</div>
          {:else}
            {#each customDictEntries as e}
              <div class="custom-dict-row">
                <span class="custom-dict-wrong mono">{e.wrong}</span>
                <span class="custom-dict-arrow">→</span>
                <span class="custom-dict-right mono">{e.right}</span>
                {#if e.auto}<span class="custom-dict-auto">auto</span>{/if}
                <button class="btn btn-ghost btn-tiny" on:click={() => removeCustomDictEntry(e.wrong)} title="Supprimer">✕</button>
              </div>
            {/each}
          {/if}
        </div>
        <div style="margin-top:8px;display:flex;gap:8px;">
          <button class="btn btn-ghost btn-tiny" on:click={() => { newDictWrong=''; newDictRight=''; showAddDictModal = true; }}>+ Ajouter</button>
          <button class="btn btn-ghost btn-tiny" on:click={loadCustomDict}>↻ Recharger</button>
        </div>
      </div>

      <div class="card">
        <div class="card-title">Index Discord (admin)</div>
        <div class="field-hint" style="margin-bottom:8px;">
          Permet aux users de l'app d'ouvrir le post Discord d'un film de la Team via un bouton <b>↗ Discord</b> dans le header film-bar.
          Seul l'admin renseigne le bot et lance le scan ; les users récupèrent l'index public via l'URL JSON.
        </div>
        <div class="field"><label>Token bot Discord</label>
          <input type="password" value={config.discord_bot_token} disabled readonly style="opacity:0.6;cursor:not-allowed;" autocomplete="off" />
          <div class="field-hint">Verrouillé (configuration interne LiHDL).</div>
        </div>
        <div class="field"><label>ID(s) forum channel(s) Discord</label>
          <input type="password" value={config.discord_forum_id} disabled readonly style="opacity:0.6;cursor:not-allowed;" autocomplete="off" />
          <div class="field-hint">Verrouillé (configuration interne LiHDL).</div>
        </div>
        <div class="field"><label>URL JSON remote (pour les users)</label>
          <input type="password" value={config.discord_index_url} disabled readonly style="opacity:0.6;cursor:not-allowed;" />
          <div class="field-hint">Verrouillé (configuration interne LiHDL).</div>
        </div>

        <!-- Push GitHub direct (admin) — verrouillé : config interne LiHDL -->
        <div style="margin-top:14px;padding:12px;border:1px solid rgba(255,255,255,0.06);border-radius:10px;background:rgba(255,255,255,0.02);">
          <div style="font-size:11.5px;font-weight:700;color:var(--text2);text-transform:uppercase;letter-spacing:0.6px;margin-bottom:8px;">📤 Push GitHub direct (admin) — verrouillé</div>
          <div class="field"><label>Token GitHub</label>
            <input type="password" value={config.github_token} disabled readonly style="opacity:0.6;cursor:not-allowed;" autocomplete="off" />
            <div class="field-hint">Verrouillé (configuration interne LiHDL).</div>
          </div>
          <div class="field"><label>Repo (owner/name)</label>
            <input type="password" value={config.github_repo} disabled readonly style="opacity:0.6;cursor:not-allowed;" autocomplete="off" />
            <div class="field-hint">Verrouillé.</div>
          </div>
          <div style="display:grid;grid-template-columns:1fr 2fr;gap:8px;">
            <div class="field"><label>Branche</label>
              <input type="password" value={config.github_branch} disabled readonly style="opacity:0.6;cursor:not-allowed;" autocomplete="off" />
            </div>
            <div class="field"><label>Path du fichier dans le repo</label>
              <input type="password" value={config.github_index_file_path} disabled readonly style="opacity:0.6;cursor:not-allowed;" autocomplete="off" />
            </div>
          </div>
        </div>

        <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin-top:8px;">
          <button class="btn btn-accent" on:click={doDiscordScan} disabled={discordScanRunning || !config.discord_bot_token || !config.discord_forum_id}>
            {discordScanRunning ? '⟳ Scan en cours…' : '🔄 Mettre à jour l\'index'}
          </button>
          <button class="btn btn-ghost" on:click={doDiscordCopy} disabled={discordScanRunning}>
            {discordCopyOk ? '✓ Copié' : '📋 Copier le JSON'}
          </button>
          <button class="btn btn-accent" on:click={doDiscordPushGitHub} disabled={discordScanRunning || githubPushing || !config.github_token || !config.github_repo}>
            {githubPushing ? '⟳ Push en cours…' : (githubPushOk ? '✓ Pushé sur GitHub' : '📤 Pusher sur GitHub')}
          </button>
        </div>
        {#if discordScanRunning || discordScanProgress.message}
          <div class="field-hint" style="margin-top:8px;">
            {#if discordScanProgress.total > 0}
              <div style="height:6px;background:#222;border-radius:3px;overflow:hidden;margin-bottom:6px;">
                <div style="height:100%;background:#7ad17a;width:{Math.min(100, Math.round((discordScanProgress.scanned / Math.max(1, discordScanProgress.total)) * 100))}%;transition:width .2s;"></div>
              </div>
              <div>{discordScanProgress.scanned} / {discordScanProgress.total} — {discordScanProgress.message}</div>
            {:else}
              <div>{discordScanProgress.message}</div>
            {/if}
          </div>
        {/if}
        <div class="field-hint" style="margin-top:10px;">
          Une fois l'index généré, push manuellement le fichier <b class="mono">~/Library/Application Support/go-mux-lihdl-team/discord_index.json</b> sur GitHub (raw) à l'URL ci-dessus. Les users de l'app le téléchargeront automatiquement au démarrage.
        </div>
      </div>

      <div class="card">
        <div class="card-title">MKVToolNix</div>
        <div class="field-hint">
          <b>mkvmerge</b> et <b>mkvextract</b> sont embarqués dans l'app — aucune installation requise.
        </div>
      </div>

      <div class="actions-row">
        <button class="btn btn-accent" on:click={saveSettings}>Enregistrer</button>
      </div>
    </div><!-- /fullscreen reglages -->
    {/if}

  </main>

  {#if showLTReview}
    <div class="lt-review-overlay" on:click={() => showLTReview = false} on:keydown={(e) => e.key === 'Escape' && (showLTReview = false)} role="presentation">
      <div class="lt-review-modal" on:click|stopPropagation role="dialog" aria-modal="true" aria-label="Lignes à vérifier (LanguageTool)">
        <div class="lt-review-header">
          <h3>🔍 Lignes à vérifier — LanguageTool ({ocrProgress.lt_needs_review})</h3>
          <button class="btn btn-ghost btn-tiny" on:click={() => showLTReview = false}>✕</button>
        </div>
        <div class="lt-review-body">
          {#if !ocrProgress.lt_review_list || ocrProgress.lt_review_list.length === 0}
            <div class="empty-hint">Aucune ligne à vérifier (top 5 max retournées par le backend).</div>
          {:else}
            {#each ocrProgress.lt_review_list as m, idx}
              <div class="lt-review-item" class:resolved={ltReviewState[idx] && ltReviewState[idx].resolved}>
                <div class="lt-review-line">
                  <b>Ligne {m.line_number}</b>
                  {#if ltReviewState[idx] && ltReviewState[idx].resolved}
                    <span class="lt-resolved-pill">{ltReviewState[idx].ignored ? '⊘ Ignoré' : '✓ Corrigé'}</span>
                  {/if}
                </div>
                <div class="lt-review-snippet mono">{m.snippet}</div>
                <div class="lt-review-msg">💬 {m.message}</div>
                {#if !(ltReviewState[idx] && ltReviewState[idx].resolved)}
                  {#if m.suggestions && m.suggestions.length > 0}
                    <div class="lt-review-sugg">
                      Suggestions cliquables :
                      {#each m.suggestions as s}
                        <button class="lt-sugg-pill clickable"
                                disabled={ltReviewState[idx] && ltReviewState[idx].busy}
                                on:click={() => applyReviewFix(idx, s)}>
                          {s}
                        </button>
                      {/each}
                    </div>
                  {/if}
                  <div class="lt-review-custom">
                    <input type="text" class="lt-custom-input" placeholder="Correction libre…"
                           bind:value={ltReviewState[idx].customText}
                           on:keydown={(e) => e.key === 'Enter' && applyReviewFix(idx, ltReviewState[idx].customText)}
                           disabled={ltReviewState[idx] && ltReviewState[idx].busy} />
                    <button class="btn btn-accent btn-tiny"
                            disabled={!ltReviewState[idx] || !ltReviewState[idx].customText || !ltReviewState[idx].customText.trim() || ltReviewState[idx].busy}
                            on:click={() => applyReviewFix(idx, ltReviewState[idx].customText)}>
                      {ltReviewState[idx] && ltReviewState[idx].busy ? '…' : 'Appliquer'}
                    </button>
                    <button class="btn btn-ghost btn-tiny"
                            on:click={() => ignoreReviewMatch(idx)}
                            title="Ignorer ce match (pas une vraie erreur)">
                      Ignorer
                    </button>
                  </div>
                  {#if ltReviewState[idx] && ltReviewState[idx].error}
                    <div class="lt-review-err">⚠ {ltReviewState[idx].error}</div>
                  {/if}
                {/if}
              </div>
            {/each}
            <div class="field-hint" style="margin-top:8px">
              ℹ Click sur une suggestion ou tape une correction libre → patche le SRT en live et alimente le dictionnaire custom.
            </div>
          {/if}
        </div>
        <div class="lt-review-footer">
          <button class="btn btn-accent" on:click={() => showLTReview = false}>Fermer</button>
        </div>
      </div>
    </div>
  {/if}

  {#if showOSModal}
    <div class="lt-review-overlay" on:click={() => showOSModal = false} on:keydown={(e) => e.key === 'Escape' && (showOSModal = false)} role="presentation">
      <div class="lt-review-modal" on:click|stopPropagation role="dialog" aria-modal="true" aria-label="OpenSubtitles">
        <div class="lt-review-header">
          <h3>🔍 OpenSubtitles — recherche SRT</h3>
          <button class="btn btn-ghost btn-tiny" on:click={() => showOSModal = false}>✕</button>
        </div>
        <div class="lt-review-body">
          {#if !config.opensubtitles_api_key}
            <div class="empty-hint" style="padding:12px;border:1px solid var(--orange);border-radius:8px;background:rgba(255,181,71,0.08);">
              ⚠ Clé API OpenSubtitles manquante.
              <button class="btn btn-ghost btn-tiny" style="margin-left:8px" on:click={() => { showOSModal = false; screen = 'reglages'; }}>
                Configurer dans Settings
              </button>
            </div>
          {/if}
          <div class="os-search-row">
            <div class="field" style="flex:2">
              <label>Titre</label>
              <input type="text" bind:value={osQuery} placeholder="ex: Inception" on:keydown={(e) => e.key === 'Enter' && searchOpenSubtitles()} />
            </div>
            <div class="field" style="flex:1">
              <label>Année</label>
              <input type="text" bind:value={osYear} placeholder="ex: 2010" inputmode="numeric" />
            </div>
            <div class="field" style="flex:1">
              <label>Langues</label>
              <input type="text" bind:value={osLang} placeholder="fr,en" />
            </div>
            <button class="btn btn-accent" on:click={searchOpenSubtitles} disabled={osSearching || !config.opensubtitles_api_key}>
              {osSearching ? '⏳' : 'Chercher'}
            </button>
          </div>
          {#if osError}
            <div class="lt-review-err">⚠ {osError}</div>
          {/if}
          {#if osResults && osResults.length > 0}
            <div class="os-results-list">
              {#each osResults as r}
                <div class="os-result-row">
                  <div class="os-result-title">
                    <b>{r.title || '—'}</b>
                    {#if r.year}<span class="os-pill">{r.year}</span>{/if}
                    <span class="os-pill">{(r.language || '').toUpperCase()}</span>
                    <span class="os-pill">↓ {r.download_count || 0}</span>
                    {#if r.rating}<span class="os-pill">★ {r.rating.toFixed(1)}</span>{/if}
                  </div>
                  <div class="os-result-filename mono">{r.filename || ''}</div>
                  <div class="os-result-actions">
                    {#if r.url}
                      <button class="btn btn-ghost btn-tiny" on:click={() => OpenURL(r.url)}>↗ Voir</button>
                    {/if}
                    <button class="btn btn-accent btn-tiny" on:click={() => downloadOSResult(r)} disabled={osDownloading === r.id}>
                      {osDownloading === r.id ? '⏳ Téléchargement…' : '⬇ Utiliser'}
                    </button>
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
        <div class="lt-review-footer">
          <button class="btn btn-ghost" on:click={() => showOSModal = false}>Fermer</button>
        </div>
      </div>
    </div>
  {/if}

  {#if showAddDictModal}
    <div class="lt-review-overlay" on:click={() => showAddDictModal = false} on:keydown={(e) => e.key === 'Escape' && (showAddDictModal = false)} role="presentation">
      <div class="lt-review-modal" on:click|stopPropagation role="dialog" aria-modal="true" aria-label="Ajouter une entrée au dictionnaire OCR">
        <div class="lt-review-header">
          <h3>+ Ajouter une entrée au dictionnaire OCR</h3>
          <button class="btn btn-ghost btn-tiny" on:click={() => showAddDictModal = false}>✕</button>
        </div>
        <div class="lt-review-body">
          <div class="field">
            <label>Texte OCR fautif (wrong)</label>
            <input type="text" bind:value={newDictWrong} placeholder="ex: Charli xex" />
          </div>
          <div class="field">
            <label>Correction (right)</label>
            <input type="text" bind:value={newDictRight} placeholder="ex: Charli XCX" on:keydown={(e) => e.key === 'Enter' && addCustomDictEntry()} />
          </div>
        </div>
        <div class="lt-review-footer">
          <button class="btn btn-ghost" on:click={() => showAddDictModal = false}>Annuler</button>
          <button class="btn btn-accent" on:click={addCustomDictEntry} disabled={dictBusy || !newDictWrong.trim() || !newDictRight.trim()}>
            {dictBusy ? '…' : 'Ajouter'}
          </button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Le journal est rendu dans la col droite quand sourcePath && tmdbValidated && screen === 'source'.
       Sinon on garde un placeholder vide pour ne pas casser appendLog (logEl est lié dans la card). -->
</div><!-- /app -->

<style>
  :root {
    color-scheme: dark;
    --bg:           #0a0712;
    --bg-deep:      #06040c;
    --card:         rgba(26, 22, 34, 0.55);
    --card-solid:   rgba(26, 22, 34, 0.75);
    --card-hi:      rgba(40, 33, 54, 0.65);
    --border:       rgba(255, 255, 255, 0.08);
    --border-hi:    rgba(255, 255, 255, 0.18);
    --border-glow:  rgba(255, 255, 255, 0.06);
    --text:         #f3eff5;
    --text2:        #a8a0b0;
    --text3:        #6f677a;
    --accent:       #7c5cff;
    --accent-hot:   #9b80ff;
    --accent-soft:  rgba(124, 92, 255, 0.55);
    --red:          #ff3d5e;
    --red-hot:      #ff6680;
    --red-soft:     rgba(255, 61, 94, 0.55);
    --green:        #5cc999;
    --orange:       #ffb547;
    --pink:         #ff5cb3;
    --blue:         #5ec5ff;
    --glass-blur:   blur(22px) saturate(180%);
    --glass-blur-strong: blur(32px) saturate(200%);
    --shadow-card:  0 8px 32px rgba(0, 0, 0, 0.35), inset 0 1px 0 rgba(255, 255, 255, 0.05);
    --shadow-card-hi: 0 12px 40px rgba(0, 0, 0, 0.45), inset 0 1px 0 rgba(255, 255, 255, 0.08);
  }
  :global(*) { box-sizing: border-box; }
  :global(html), :global(body) {
    margin: 0; padding: 0;
    font-family: -apple-system, "SF Pro Text", "Segoe UI", system-ui, sans-serif;
    color: var(--text);
    background: var(--bg-deep);
    font-size: 13px; line-height: 1.4;
    height: 100vh; overflow: hidden;
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
  }
  .mono { font-family: "JetBrains Mono", "SF Mono", "Consolas", ui-monospace, monospace; }

  /* Page = grille header / (subnav?) / (mux-progress?) / main / footer */
  .app {
    display: grid;
    grid-template-rows: auto auto auto 1fr auto;
    height: 100vh;
    overflow: hidden;
    position: relative;
    isolation: isolate;
    background: var(--bg-deep);
  }
  /* Journal d'activité inline : placé dans la col droite après "Outils additionnels",
     prend tout l'espace restant pour combler le vide en bas à droite. */
  .journal-card.journal-inline {
    flex: 1 1 auto;
    min-height: 160px;
    display: flex; flex-direction: column;
    background: linear-gradient(180deg, rgba(20, 16, 28, 0.62), rgba(16, 12, 22, 0.5));
    backdrop-filter: blur(24px) saturate(170%);
    -webkit-backdrop-filter: blur(24px) saturate(170%);
    border: 1px solid rgba(255, 255, 255, 0.08);
    box-shadow: 0 8px 28px rgba(0, 0, 0, 0.35), inset 0 1px 0 rgba(255, 255, 255, 0.04);
    border-radius: 14px;
    padding: 12px 14px;
  }
  .journal-card.journal-inline .card-title {
    margin-bottom: 8px;
    flex-shrink: 0;
  }
  .bottom-pane-tabs {
    display: flex; gap: 4px;
    margin-bottom: 10px;
    flex-shrink: 0;
    background: rgba(0, 0, 0, 0.25);
    border: 1px solid rgba(255, 255, 255, 0.05);
    border-radius: 8px;
    padding: 3px;
  }
  .bottom-pane-tab {
    flex: 1;
    background: transparent; border: none;
    color: var(--text2);
    font: inherit; font-size: 11px; font-weight: 600;
    padding: 5px 10px; border-radius: 6px;
    cursor: pointer;
    transition: all 150ms;
    text-transform: uppercase; letter-spacing: 0.4px;
  }
  .bottom-pane-tab:hover { color: var(--text); }
  .bottom-pane-tab.active {
    background: linear-gradient(180deg, rgba(124, 92, 255, 0.85), rgba(124, 92, 255, 0.65));
    color: #fff;
    box-shadow: 0 2px 8px rgba(124, 92, 255, 0.35), inset 0 1px 0 rgba(255, 255, 255, 0.15);
  }
  .queue-pane {
    flex: 1 1 auto; min-height: 0;
    display: flex; flex-direction: column;
    overflow: hidden;
  }
  .queue-empty {
    flex: 1; display: flex; flex-direction: column;
    align-items: center; justify-content: flex-start;
    gap: 4px; text-align: center; padding: 14px 16px 12px;
    color: var(--text2);
  }
  .queue-empty-icon { font-size: 26px; opacity: 0.6; line-height: 1; }
  .queue-empty-title { font-size: 12px; font-weight: 700; color: var(--text); }
  .queue-empty-sub { font-size: 10.5px; line-height: 1.35; max-width: 220px; }
  .queue-list {
    list-style: none; margin: 0; padding: 0;
    flex: 1; overflow-y: auto; min-height: 0;
    display: flex; flex-direction: column; gap: 4px;
  }
  .queue-row {
    display: grid;
    grid-template-columns: 22px 1fr auto auto;
    gap: 6px; align-items: center;
    padding: 6px 8px;
    background: rgba(255, 255, 255, 0.03);
    border: 1px solid rgba(255, 255, 255, 0.05);
    border-radius: 7px;
    transition: all 150ms;
  }
  .queue-row:hover {
    background: rgba(255, 255, 255, 0.06);
    border-color: rgba(255, 255, 255, 0.1);
  }
  .queue-row.current {
    background: rgba(92, 201, 153, 0.08);
    border-color: rgba(92, 201, 153, 0.3);
  }
  .queue-idx {
    width: 22px; height: 22px; border-radius: 50%;
    display: flex; align-items: center; justify-content: center;
    background: rgba(124, 92, 255, 0.18);
    color: var(--accent-hot, #9b80ff);
    font-size: 10px; font-weight: 700;
  }
  .queue-row.current .queue-idx { background: var(--green); color: #0e0c14; }
  .queue-name {
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    font-size: 11px; color: var(--text);
  }
  .queue-actions {
    display: flex; gap: 6px; padding-top: 8px;
    border-top: 1px solid rgba(255, 255, 255, 0.05);
    margin-top: 6px;
    flex-shrink: 0;
  }
  .journal-scroll {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    overflow-x: hidden;
    font-family: "SF Mono", "Consolas", monospace;
    font-size: 10.5px;
    padding-right: 4px;
  }
  .journal-scroll .journal-line {
    display: flex; gap: 8px; padding: 2px 0;
    border-bottom: 1px dashed rgba(255, 255, 255, 0.025);
    line-height: 1.4;
  }
  .journal-scroll .journal-line:last-child { border-bottom: none; }
  .journal-scroll .log-time { color: var(--text3); flex-shrink: 0; opacity: 0.7; }
  .journal-scroll .log-msg { color: var(--text); word-break: break-word; }
  .journal-scroll .log-msg.lvl-info { color: var(--text2); }
  .journal-scroll .log-msg.lvl-ok,
  .journal-scroll .log-msg.lvl-success { color: var(--green); }
  .journal-scroll .log-msg.lvl-warn { color: var(--orange); }
  .journal-scroll .log-msg.lvl-err,
  .journal-scroll .log-msg.lvl-error { color: var(--red); }

  /* ─── BACKGROUND LAYERS (banner + overlay + glow) ─── */
  .bg-banner {
    position: absolute; inset: 0;
    background-image: var(--banner-url);
    background-size: cover;
    background-position: center;
    background-repeat: no-repeat;
    opacity: 0.7;
    filter: saturate(125%) contrast(112%);
    z-index: -3;
    pointer-events: none;
  }
  .bg-overlay {
    position: absolute; inset: 0;
    background:
      radial-gradient(ellipse 120% 80% at 50% 0%, rgba(124, 92, 255, 0.12), transparent 60%),
      radial-gradient(ellipse 100% 60% at 50% 100%, rgba(255, 61, 94, 0.08), transparent 70%),
      linear-gradient(180deg, rgba(10, 7, 18, 0.45) 0%, rgba(10, 7, 18, 0.55) 50%, rgba(6, 4, 12, 0.7) 100%);
    z-index: -2;
    pointer-events: none;
  }
  .bg-glow {
    position: absolute;
    border-radius: 50%;
    filter: blur(120px);
    opacity: 0.55;
    z-index: -2;
    pointer-events: none;
    will-change: transform;
  }
  .bg-glow-1 {
    width: 520px; height: 520px;
    top: -180px; left: -120px;
    background: radial-gradient(circle, rgba(124, 92, 255, 0.55), transparent 70%);
    animation: float-glow 18s ease-in-out infinite;
  }
  .bg-glow-2 {
    width: 480px; height: 480px;
    bottom: -160px; right: -100px;
    background: radial-gradient(circle, rgba(255, 61, 94, 0.45), transparent 70%);
    animation: float-glow 22s ease-in-out infinite reverse;
  }
  @keyframes float-glow {
    0%, 100% { transform: translate(0, 0) scale(1); }
    50% { transform: translate(40px, -30px) scale(1.08); }
  }

  /* ─── HEADER (sticky liquid glass) ─── */
  .header {
    display: grid;
    grid-template-columns: auto auto 1fr auto;
    gap: 14px;
    align-items: center;
    padding: 10px 16px;
    background: linear-gradient(180deg, rgba(21, 17, 29, 0.28), rgba(17, 13, 24, 0.22));
    backdrop-filter: var(--glass-blur-strong);
    -webkit-backdrop-filter: var(--glass-blur-strong);
    border-bottom: 1px solid var(--border);
    box-shadow:
      0 1px 0 rgba(255, 255, 255, 0.04) inset,
      0 8px 24px rgba(0, 0, 0, 0.25);
    position: relative;
    z-index: 10;
  }
  .header::after {
    content: "";
    position: absolute; left: 0; right: 0; bottom: -1px; height: 1px;
    background: linear-gradient(90deg, transparent, rgba(124, 92, 255, 0.4), rgba(255, 61, 94, 0.3), transparent);
    pointer-events: none;
  }
  .brand { display: flex; align-items: center; gap: 10px; }
  .brand-logo-img {
    width: 38px; height: 38px; border-radius: 9px;
    object-fit: contain;
    box-shadow:
      0 4px 14px rgba(124, 92, 255, 0.35),
      0 0 0 1px rgba(255, 255, 255, 0.08);
  }
  .brand-name {
    font-size: 13px; font-weight: 700; line-height: 1.1;
    letter-spacing: -0.1px;
    background: linear-gradient(135deg, #fff 0%, #c8b5ff 100%);
    -webkit-background-clip: text; background-clip: text;
    -webkit-text-fill-color: transparent;
  }
  .brand-version {
    font-size: 10px; color: var(--text3);
    font-weight: 500;
    letter-spacing: 0.3px;
  }

  /* Mode switch — premium glass pill */
  .mode-switch {
    display: flex;
    background: rgba(8, 6, 14, 0.55);
    backdrop-filter: blur(14px) saturate(160%);
    -webkit-backdrop-filter: blur(14px) saturate(160%);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 3px;
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.03), 0 2px 8px rgba(0, 0, 0, 0.25);
  }
  .mode-btn {
    background: transparent; border: none; color: var(--text2);
    font: inherit; font-size: 11.5px; font-weight: 500;
    padding: 7px 14px; border-radius: 7px; cursor: pointer;
    transition: all 180ms cubic-bezier(0.2, 0.8, 0.2, 1);
    display: inline-flex; align-items: center; gap: 5px;
    white-space: nowrap;
    position: relative;
  }
  .mode-btn.active {
    background: linear-gradient(135deg, var(--accent), #6845e8);
    color: #fff; font-weight: 600;
    box-shadow:
      0 4px 14px rgba(124, 92, 255, 0.45),
      inset 0 1px 0 rgba(255, 255, 255, 0.18);
  }
  .mode-btn:not(.active):hover { color: var(--text); background: rgba(255, 255, 255, 0.04); }

  /* Film identity — glass pill */
  .film-bar {
    display: flex; align-items: center; gap: 10px;
    background: rgba(26, 22, 34, 0.55);
    backdrop-filter: blur(18px) saturate(160%);
    -webkit-backdrop-filter: blur(18px) saturate(160%);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 4px 12px 4px 4px;
    min-width: 0;
    box-shadow: var(--shadow-card);
    transition: all 220ms ease;
  }
  .film-bar:hover { border-color: var(--border-hi); }
  .film-bar.empty { opacity: 0.7; }
  .film-poster {
    width: 40px; height: 60px;
    border-radius: 6px;
    object-fit: cover;
    background: #2a1f3a;
    flex-shrink: 0;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.4);
  }
  .film-poster.placeholder {
    background: linear-gradient(135deg, var(--accent), var(--red));
    position: relative;
    overflow: hidden;
  }
  .film-poster.placeholder::after {
    content: "🎬"; position: absolute; inset: 0;
    display: flex; align-items: center; justify-content: center;
    font-size: 18px; opacity: 0.55;
  }
  .film-info { min-width: 0; flex: 1; }
  .film-title {
    font-size: 13px; font-weight: 700;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    letter-spacing: -0.1px;
  }
  .film-meta {
    font-size: 11px; color: var(--text2);
    display: flex; gap: 8px; align-items: center;
    flex-wrap: wrap;
  }
  .film-id {
    background: rgba(124, 92, 255, 0.18);
    color: var(--accent-hot);
    padding: 1px 7px; border-radius: 4px;
    font-size: 10px; font-weight: 600;
    border: 1px solid rgba(124, 92, 255, 0.25);
  }
  button.film-id-link {
    font: inherit;
    cursor: pointer;
    transition: all 150ms;
  }
  button.film-id-link:hover {
    background: rgba(124, 92, 255, 0.32);
    border-color: rgba(124, 92, 255, 0.5);
    color: #fff;
    box-shadow: 0 0 0 3px rgba(124, 92, 255, 0.1);
  }

  /* Header actions */
  .header-actions { display: flex; gap: 6px; flex-wrap: wrap; justify-content: flex-end; }

  .btn {
    border: 1px solid var(--border);
    background: rgba(40, 33, 54, 0.5);
    backdrop-filter: blur(14px) saturate(160%);
    -webkit-backdrop-filter: blur(14px) saturate(160%);
    color: var(--text);
    font: inherit; font-size: 12px; font-weight: 500;
    padding: 7px 13px; border-radius: 8px;
    cursor: pointer;
    display: inline-flex; align-items: center; gap: 5px;
    transition: all 180ms cubic-bezier(0.2, 0.8, 0.2, 1);
    white-space: nowrap;
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04), 0 1px 2px rgba(0, 0, 0, 0.2);
  }
  .btn:hover:not(:disabled) {
    background: rgba(60, 50, 80, 0.6);
    border-color: var(--border-hi);
    transform: translateY(-1px);
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.08), 0 4px 14px rgba(0, 0, 0, 0.3);
  }
  .btn:active:not(:disabled) { transform: translateY(0); }
  .btn:disabled { opacity: 0.4; cursor: not-allowed; }
  .btn-ghost {
    background: transparent;
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
    border-color: transparent;
    color: var(--text2);
    box-shadow: none;
  }
  .btn-ghost:hover:not(:disabled) {
    background: rgba(255,255,255,0.06);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
    color: var(--text);
    border-color: var(--border);
  }
  .btn-accent {
    background: linear-gradient(135deg, var(--accent) 0%, #6845e8 100%);
    border-color: rgba(124, 92, 255, 0.6);
    color: #fff; font-weight: 600;
    box-shadow:
      0 4px 14px rgba(124, 92, 255, 0.4),
      inset 0 1px 0 rgba(255, 255, 255, 0.2);
  }
  .btn-accent:hover:not(:disabled) {
    background: linear-gradient(135deg, var(--accent-hot) 0%, #7c5cff 100%);
    border-color: rgba(155, 128, 255, 0.7);
    box-shadow:
      0 6px 20px rgba(124, 92, 255, 0.55),
      inset 0 1px 0 rgba(255, 255, 255, 0.25);
  }
  .btn-danger {
    background: linear-gradient(135deg, var(--red) 0%, #e02744 100%);
    border-color: rgba(255, 61, 94, 0.6);
    color: #fff; font-weight: 700;
    box-shadow:
      0 4px 14px rgba(255, 61, 94, 0.4),
      inset 0 1px 0 rgba(255, 255, 255, 0.2);
  }
  .btn-danger:hover:not(:disabled) {
    background: linear-gradient(135deg, var(--red-hot) 0%, var(--red) 100%);
    border-color: rgba(255, 102, 128, 0.7);
    box-shadow:
      0 6px 20px rgba(255, 61, 94, 0.55),
      inset 0 1px 0 rgba(255, 255, 255, 0.25);
  }
  .btn-tiny { font-size: 11px; padding: 4px 10px; border-radius: 6px; }
  .btn-icon { padding: 4px 9px; }
  .btn-stop-small { padding: 5px 11px; font-size: 11px; }

  .update-pill {
    font-size: 11px; padding: 6px 11px;
  }
  .update-pill.available {
    background: linear-gradient(135deg, rgba(92, 201, 153, 0.22), rgba(92, 201, 153, 0.10));
    border-color: rgba(92, 201, 153, 0.45);
    color: var(--green); font-weight: 700;
    box-shadow: 0 0 14px rgba(92, 201, 153, 0.25), inset 0 1px 0 rgba(255, 255, 255, 0.06);
  }
  .spin { animation: spin-icon 1s linear infinite; display: inline-block; }
  @keyframes spin-icon { to { transform: rotate(360deg); } }

  /* Subnav (visible si screen != source) */
  .subnav {
    display: flex; gap: 6px; align-items: center;
    padding: 8px 16px;
    background: rgba(17, 13, 24, 0.55);
    backdrop-filter: blur(20px) saturate(170%);
    -webkit-backdrop-filter: blur(20px) saturate(170%);
    border-bottom: 1px solid var(--border);
    position: relative;
    z-index: 9;
  }
  .subnav-btn {
    background: transparent; border: 1px solid transparent;
    color: var(--text2);
    font: inherit; font-size: 12px; font-weight: 500;
    padding: 6px 12px; border-radius: 7px; cursor: pointer;
    transition: all 180ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .subnav-btn:hover { color: var(--text); background: rgba(255,255,255,0.06); }
  .subnav-btn.active {
    color: #fff;
    background: linear-gradient(135deg, var(--accent), #6845e8);
    border-color: rgba(124, 92, 255, 0.5);
    box-shadow: 0 3px 10px rgba(124, 92, 255, 0.35), inset 0 1px 0 rgba(255, 255, 255, 0.15);
  }

  /* Mux progress bar (top floating, glass) */
  .mux-progress-bar {
    display: flex; align-items: center; gap: 12px;
    padding: 10px 16px;
    background: linear-gradient(90deg, rgba(124,92,255,0.18), rgba(255,61,94,0.12));
    backdrop-filter: blur(20px) saturate(180%);
    -webkit-backdrop-filter: blur(20px) saturate(180%);
    border-bottom: 1px solid rgba(124, 92, 255, 0.25);
    box-shadow: 0 2px 12px rgba(124, 92, 255, 0.2);
    position: relative;
    z-index: 9;
  }
  .progress-bar {
    flex: 1; height: 9px; border-radius: 5px;
    background: rgba(0, 0, 0, 0.4);
    overflow: hidden;
    border: 1px solid rgba(255, 255, 255, 0.05);
    box-shadow: inset 0 1px 2px rgba(0, 0, 0, 0.3);
  }
  .progress-fill {
    height: 100%;
    background: linear-gradient(90deg, var(--accent), var(--red));
    transition: width 200ms ease-out;
    box-shadow: 0 0 12px rgba(124, 92, 255, 0.5);
    position: relative;
  }
  .progress-fill::after {
    content: "";
    position: absolute; inset: 0;
    background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.3), transparent);
    animation: progress-shine 1.6s linear infinite;
  }
  @keyframes progress-shine {
    from { transform: translateX(-100%); }
    to { transform: translateX(100%); }
  }
  .progress-bar.done .progress-fill { background: linear-gradient(90deg, #2f9e44, var(--green)); }
  .progress-bar.error .progress-fill { background: linear-gradient(90deg, #c92a2a, var(--red)); }

  /* ─── MAIN ─── */
  .main {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
    padding: 12px;
    overflow: hidden;
    min-height: 0;
    position: relative;
    z-index: 1;
  }
  /* Quand pas de source : on masque la colonne gauche (Sources redondant avec le bouton du hero),
     et la colonne droite (empty-hero) est parfaitement centrée au milieu de la fenêtre. */
  .app.no-source .main {
    grid-template-columns: 1fr;
    grid-template-rows: 1fr;
    place-items: center;
    overflow: hidden;
  }
  .app.no-source .col:first-child { display: none; }
  .app.no-source .col:last-child {
    grid-column: 1; grid-row: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    width: min(900px, 92%);
    height: 100%;
    overflow: visible;
    gap: 0;
  }
  .app.no-source .empty-hero {
    margin: 0;
    width: 100%;
    flex: 0 0 auto;
  }

  /* TMDB pending : on a une source + une fiche TMDB mais pas encore validée → on affiche
     "Source chargée" (empty-hero compact) PUIS la card de validation TMDB en dessous. */
  .app.tmdb-pending .main {
    grid-template-columns: 1fr;
    grid-template-rows: 1fr;
    overflow: hidden;
  }
  .app.tmdb-pending .col:first-child { display: none; }
  .app.tmdb-pending .col:last-child {
    grid-column: 1; grid-row: 1;
    display: flex;
    flex-direction: column;
    align-items: stretch;
    justify-content: center;
    width: min(1040px, 96%);
    margin: 0 auto;
    height: 100%;
    overflow: hidden;
    padding: 8px 0;
    gap: 10px;
  }
  /* Cache toutes les cards de la col droite SAUF empty-hero et tmdb-validate-card */
  .app.tmdb-pending .col:last-child > .card:not(.tmdb-validate-card):not(.empty-hero) { display: none; }
  .app.tmdb-pending .tmdb-validate-card { flex: 1 1 auto; min-height: 0; }
  .app.tmdb-pending .empty-hero { flex: 0 0 auto; }

  /* Section "Outils additionnels" intégrée dans la card "Prêt à muxer" — pleine largeur. */
  .empty-hero-tools {
    margin-top: 22px;
    width: 100%;
    padding: 14px 18px;
    background: rgba(255, 255, 255, 0.03);
    border: 1px solid rgba(255, 255, 255, 0.06);
    border-radius: 12px;
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    text-align: left;
  }
  .empty-hero-tools-title {
    font-size: 10.5px; font-weight: 700; letter-spacing: 0.6px;
    color: var(--text2);
    text-transform: uppercase;
    margin-bottom: 10px;
    text-align: center;
  }
  .empty-hero-tools-row {
    display: grid;
    grid-auto-flow: column;
    grid-auto-columns: 1fr;
    gap: 8px;
  }
  .empty-hero-tools-row .btn {
    width: 100%;
    padding: 10px 14px;
    font-size: 12.5px;
    font-weight: 600;
    justify-content: center;
    text-align: center;
  }
  @media (max-width: 600px) {
    .empty-hero-tools-row {
      grid-auto-flow: row;
      grid-auto-columns: unset;
      grid-template-columns: 1fr;
    }
  }

  /* Empty-hero "compact" : un mini-bandeau horizontal au lieu d'un hero plein écran */
  .empty-hero.compact {
    padding: 10px 18px;
    text-align: left;
  }
  .empty-hero.compact .empty-hero-aurora { display: none; }
  .empty-hero.compact .empty-hero-content {
    flex-direction: row;
    align-items: center;
    gap: 14px;
    text-align: left;
  }
  .empty-hero.compact .empty-hero-badge { display: none; }
  .empty-hero.compact .empty-hero-icon {
    width: 44px; height: 44px;
    flex-shrink: 0;
  }
  .empty-hero.compact .empty-hero-icon-glyph { font-size: 22px; }
  .empty-hero.compact .empty-hero-title {
    font-size: 16px;
    margin: 0;
    line-height: 1.1;
  }
  .empty-hero.compact .empty-hero-sub {
    font-size: 11.5px;
    line-height: 1.35;
    margin: 0;
    color: var(--text2);
  }
  .empty-hero.compact .empty-hero-sub b { color: var(--text); }

  /* Card TMDB : densifie pour tout faire tenir sans scroll */
  .app.tmdb-pending .tmdb-validate-card {
    padding: 18px 22px 16px;
    gap: 12px;
    display: flex;
    flex-direction: column;
  }
  .app.tmdb-pending .tmdb-validate-body {
    grid-template-columns: 200px 1fr;
    gap: 18px;
    flex: 1 1 auto;
    min-height: 0;
  }
  .app.tmdb-pending .tmdb-validate-poster {
    width: 200px; height: 300px;
  }
  .app.tmdb-pending .tmdb-validate-info {
    height: 100%;
    overflow: hidden;
    gap: 10px;
  }
  .app.tmdb-pending .tmdb-validate-desc {
    flex: 1 1 auto;
    min-height: 0;
    max-height: none;
  }
  .app.tmdb-pending .tmdb-validate-actions { padding-top: 2px; }

  /* Card validation TMDB */
  .tmdb-validate-card {
    width: 100%;
    padding: 26px 28px 22px;
    display: flex; flex-direction: column; gap: 18px;
    background: linear-gradient(180deg, rgba(26, 22, 34, 0.72), rgba(20, 16, 28, 0.65));
    border: 1px solid rgba(255, 255, 255, 0.10);
    box-shadow:
      0 24px 60px rgba(0, 0, 0, 0.55),
      0 0 0 1px rgba(124, 92, 255, 0.18) inset,
      0 0 0 4px rgba(124, 92, 255, 0.04);
    border-radius: 18px;
    backdrop-filter: blur(28px) saturate(180%);
    -webkit-backdrop-filter: blur(28px) saturate(180%);
    position: relative;
    overflow: hidden;
  }
  .tmdb-validate-card::before {
    content: "";
    position: absolute; top: 0; left: 0; right: 0; height: 3px;
    background: linear-gradient(90deg, var(--accent), var(--red), var(--orange));
    opacity: 0.85;
  }
  .tmdb-validate-header {
    display: flex; align-items: center; justify-content: center;
    position: relative;
    gap: 8px;
  }

  /* Mini-input "Forcer l'ID TMDB" : discret en coin top-right de la card validation. */
  .tmdb-validate-force-mini {
    position: absolute;
    top: 50%; right: 0;
    transform: translateY(-50%);
    display: inline-flex; align-items: center;
    background: rgba(0, 0, 0, 0.28);
    border: 1px solid rgba(255, 255, 255, 0.06);
    border-radius: 999px;
    padding: 2px 4px 2px 10px;
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
    transition: all 200ms ease;
    opacity: 0.55;
  }
  .tmdb-validate-force-mini:hover,
  .tmdb-validate-force-mini:focus-within {
    opacity: 1;
    border-color: rgba(124, 92, 255, 0.4);
    background: rgba(124, 92, 255, 0.08);
    box-shadow: 0 0 0 3px rgba(124, 92, 255, 0.10);
  }
  .tmdb-validate-force-mini-hash {
    font-size: 11px; font-weight: 700;
    color: var(--text3);
    margin-right: 2px;
    font-family: "SF Mono", "Consolas", monospace;
  }
  .tmdb-validate-force-mini:focus-within .tmdb-validate-force-mini-hash { color: var(--accent-hot, #9b80ff); }
  .tmdb-validate-force-mini-input {
    width: 80px;
    background: transparent;
    border: none;
    color: var(--text);
    font-size: 11.5px;
    font-family: "SF Mono", "Consolas", monospace;
    padding: 4px 2px;
    outline: none;
  }
  .tmdb-validate-force-mini-input::placeholder { color: var(--text3); font-family: inherit; }
  .tmdb-validate-force-mini-btn {
    background: transparent;
    border: none;
    color: var(--text2);
    width: 22px; height: 22px;
    border-radius: 50%;
    cursor: pointer;
    font-size: 12px;
    display: inline-flex; align-items: center; justify-content: center;
    transition: all 150ms;
  }
  .tmdb-validate-force-mini-btn:hover:not(:disabled) {
    background: var(--accent);
    color: #fff;
  }
  .tmdb-validate-force-mini-btn:disabled { opacity: 0.3; cursor: not-allowed; }
  .tmdb-validate-badge {
    font-size: 10.5px; font-weight: 700; letter-spacing: 1.2px;
    color: var(--accent-hot, #9b80ff);
    background: rgba(124, 92, 255, 0.10);
    border: 1px solid rgba(124, 92, 255, 0.25);
    padding: 5px 12px; border-radius: 999px;
    text-transform: uppercase;
  }
  .tmdb-validate-body {
    display: grid;
    grid-template-columns: 180px 1fr;
    gap: 22px;
    align-items: start;
  }
  .tmdb-validate-poster {
    width: 180px; height: 270px;
    border-radius: 12px;
    object-fit: cover;
    background: #2a1f3a;
    border: 1px solid rgba(255, 255, 255, 0.08);
    box-shadow: 0 14px 40px rgba(0, 0, 0, 0.5);
  }
  .tmdb-validate-poster.placeholder {
    display: flex; align-items: center; justify-content: center;
    font-size: 56px;
    background: linear-gradient(135deg, var(--accent), var(--red));
    color: #fff;
  }
  .tmdb-validate-info { min-width: 0; display: flex; flex-direction: column; gap: 12px; }
  .tmdb-validate-title {
    font-size: 22px; font-weight: 800; line-height: 1.2;
    background: linear-gradient(135deg, #fff, var(--accent-hot, #9b80ff));
    -webkit-background-clip: text; background-clip: text;
    -webkit-text-fill-color: transparent;
  }
  .tmdb-validate-original { font-size: 12px; color: var(--text2); font-style: italic; }
  .tmdb-validate-meta { display: flex; flex-wrap: wrap; gap: 6px; }
  .tmdb-validate-pill {
    font-size: 11px; font-weight: 600;
    padding: 4px 10px; border-radius: 999px;
    background: rgba(124, 92, 255, 0.12);
    color: var(--accent-hot, #9b80ff);
    border: 1px solid rgba(124, 92, 255, 0.25);
    backdrop-filter: blur(8px);
  }
  .tmdb-validate-pill.rating {
    background: rgba(255, 181, 71, 0.12);
    color: var(--orange);
    border-color: rgba(255, 181, 71, 0.25);
  }
  .tmdb-validate-desc {
    font-size: 13px; line-height: 1.55;
    color: var(--text);
    background: rgba(0, 0, 0, 0.18);
    padding: 12px 14px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.04);
    max-height: 180px;
    overflow-y: auto;
  }
  .tmdb-validate-force {
    display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
    padding: 8px 12px;
    background: rgba(0, 0, 0, 0.22);
    border: 1px solid rgba(255, 255, 255, 0.05);
    border-radius: 10px;
  }
  .tmdb-validate-force-label {
    font-size: 11px; color: var(--text2); font-weight: 500;
  }
  .tmdb-validate-force-input {
    flex: 1 1 100px;
    min-width: 90px;
    background: rgba(0, 0, 0, 0.35);
    border: 1px solid rgba(255, 255, 255, 0.08);
    border-radius: 7px;
    padding: 6px 10px;
    color: var(--text);
    font-size: 12px;
    font-family: "SF Mono", "Consolas", monospace;
    backdrop-filter: blur(8px);
    transition: border-color 150ms;
  }
  .tmdb-validate-force-input:focus {
    outline: none; border-color: var(--accent);
    box-shadow: 0 0 0 3px rgba(124, 92, 255, 0.18);
  }
  .tmdb-validate-actions {
    display: flex; gap: 10px; justify-content: flex-end; align-items: center;
    padding-top: 4px;
    flex-wrap: wrap;
  }
  .tmdb-validate-cta {
    padding: 10px 22px;
    font-size: 13px; font-weight: 700;
    box-shadow: 0 8px 24px rgba(124, 92, 255, 0.4);
  }
  @media (max-width: 760px) {
    .tmdb-validate-body { grid-template-columns: 1fr; }
    .tmdb-validate-poster { width: 140px; height: 210px; margin: 0 auto; }
  }

  /* Fiche TMDB persistante (après validation) */
  .tmdb-fiche-card {
    position: relative;
    overflow: hidden;
    padding: 24px 26px;
    border-radius: 18px;
    background:
      radial-gradient(1200px 200px at 0% 0%, rgba(124, 92, 255, 0.18), transparent 60%),
      linear-gradient(180deg, rgba(28, 22, 40, 0.85), rgba(18, 14, 26, 0.92));
    border: 1px solid rgba(124, 92, 255, 0.18);
    box-shadow:
      0 28px 70px rgba(0, 0, 0, 0.55),
      0 0 0 1px rgba(124, 92, 255, 0.10) inset;
    backdrop-filter: blur(28px) saturate(180%);
    -webkit-backdrop-filter: blur(28px) saturate(180%);
  }
  .tmdb-fiche-card::before {
    content: "";
    position: absolute; top: 0; left: 0; right: 0; height: 3px;
    background: linear-gradient(90deg, var(--accent), var(--red), var(--orange));
    opacity: 0.9;
  }
  .tmdb-fiche-glow {
    position: absolute;
    top: -80px; right: -100px;
    width: 360px; height: 360px;
    background: radial-gradient(closest-side, rgba(255, 100, 130, 0.18), transparent 70%);
    filter: blur(20px);
    pointer-events: none;
  }
  .tmdb-fiche-body {
    position: relative;
    display: grid;
    grid-template-columns: 200px 1fr;
    gap: 24px;
    align-items: start;
  }
  .tmdb-fiche-poster {
    width: 200px; height: 300px;
    border-radius: 14px;
    object-fit: cover;
    background: #2a1f3a;
    border: 1px solid rgba(255, 255, 255, 0.10);
    box-shadow:
      0 22px 50px rgba(0, 0, 0, 0.6),
      0 0 0 1px rgba(124, 92, 255, 0.12);
    transition: transform 0.3s ease;
  }
  .tmdb-fiche-poster:hover { transform: scale(1.02); }
  .tmdb-fiche-poster.placeholder {
    display: flex; align-items: center; justify-content: center;
    font-size: 64px;
    background: linear-gradient(135deg, var(--accent), var(--red));
    color: #fff;
  }
  .tmdb-fiche-info {
    min-width: 0;
    display: flex; flex-direction: column; gap: 14px;
  }
  .tmdb-fiche-badge {
    align-self: flex-start;
    font-size: 10.5px; font-weight: 700; letter-spacing: 1.2px;
    color: var(--accent-hot, #9b80ff);
    background: rgba(124, 92, 255, 0.12);
    border: 1px solid rgba(124, 92, 255, 0.30);
    padding: 5px 12px; border-radius: 999px;
    text-transform: uppercase;
  }
  .tmdb-fiche-title {
    margin: 0;
    font-size: 26px; font-weight: 800; line-height: 1.15;
    background: linear-gradient(135deg, #fff, var(--accent-hot, #9b80ff));
    -webkit-background-clip: text; background-clip: text;
    -webkit-text-fill-color: transparent;
  }
  .tmdb-fiche-year {
    font-weight: 600;
    color: var(--text2);
    -webkit-text-fill-color: var(--text2);
    margin-left: 6px;
  }
  .tmdb-fiche-original {
    font-size: 13px; color: var(--text2);
  }
  .tmdb-fiche-pills {
    display: flex; flex-wrap: wrap; gap: 7px;
  }
  .tmdb-fiche-pill {
    font-size: 11.5px; font-weight: 600;
    padding: 5px 11px; border-radius: 999px;
    background: rgba(124, 92, 255, 0.12);
    color: var(--accent-hot, #9b80ff);
    border: 1px solid rgba(124, 92, 255, 0.30);
    text-decoration: none;
    transition: all 0.18s ease;
  }
  .tmdb-pill-link {
    cursor: pointer;
  }
  .tmdb-pill-link:hover {
    background: rgba(124, 92, 255, 0.22);
    transform: translateY(-1px);
  }
  .tmdb-fiche-pill.rating {
    background: rgba(255, 181, 71, 0.13);
    color: var(--orange);
    border-color: rgba(255, 181, 71, 0.30);
  }
  .tmdb-fiche-overview {
    margin: 0;
    font-size: 13.5px; line-height: 1.6;
    color: var(--text);
    background: rgba(0, 0, 0, 0.22);
    padding: 13px 15px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.05);
    max-height: 200px;
    overflow-y: auto;
  }
  .tmdb-fiche-actions {
    display: flex; justify-content: flex-end;
    margin-top: 4px;
  }
  @media (max-width: 760px) {
    .tmdb-fiche-body { grid-template-columns: 1fr; }
    .tmdb-fiche-poster { width: 160px; height: 240px; margin: 0 auto; }
  }

  /* Sur petit écran, 1 colonne */
  @media (max-width: 1100px) {
    .main { grid-template-columns: 1fr; overflow: auto; }
  }

  .col {
    display: flex; flex-direction: column; gap: 12px;
    min-height: 0; overflow: auto;
    animation: fade-up 280ms cubic-bezier(0.2, 0.8, 0.2, 1) both;
  }
  @keyframes fade-up {
    from { opacity: 0; transform: translateY(6px); }
    to   { opacity: 1; transform: translateY(0); }
  }

  /* Fullscreen view (cible/sync/reglages prennent toute la largeur) */
  .fullscreen {
    grid-column: 1 / -1;
    display: flex; flex-direction: column; gap: 12px;
    overflow: auto;
    min-height: 0;
    padding-right: 4px;
    animation: fade-up 280ms cubic-bezier(0.2, 0.8, 0.2, 1) both;
  }

  /* ─── CARDS (liquid glass) ─── */
  .card {
    background: var(--card);
    backdrop-filter: var(--glass-blur);
    -webkit-backdrop-filter: var(--glass-blur);
    border: 1px solid var(--border);
    border-radius: 14px;
    padding: 14px 16px;
    box-shadow: var(--shadow-card);
    position: relative;
    transition: border-color 220ms ease, transform 220ms ease;
  }
  .card::before {
    content: "";
    position: absolute; inset: 0;
    border-radius: 14px;
    background: linear-gradient(180deg, rgba(255, 255, 255, 0.04) 0%, transparent 30%);
    pointer-events: none;
  }
  .card:hover { border-color: var(--border-hi); }
  .card-grow { flex: 1; min-height: 0; overflow: auto; }

  /* ─── EMPTY HERO (premium fullscreen invite) ─── */
  .empty-hero {
    flex: 1;
    display: flex; align-items: center; justify-content: center;
    text-align: center;
    padding: 0;
    background: linear-gradient(135deg, rgba(124, 92, 255, 0.08), rgba(255, 61, 94, 0.05));
    border: 1px solid rgba(124, 92, 255, 0.25);
    backdrop-filter: var(--glass-blur);
    -webkit-backdrop-filter: var(--glass-blur);
    overflow: hidden;
    min-height: 380px;
    position: relative;
  }
  .empty-hero-aurora {
    position: absolute; inset: -40%;
    background:
      radial-gradient(circle at 20% 30%, rgba(124, 92, 255, 0.35), transparent 35%),
      radial-gradient(circle at 80% 70%, rgba(255, 61, 94, 0.25), transparent 35%),
      radial-gradient(circle at 60% 20%, rgba(255, 181, 71, 0.15), transparent 30%);
    filter: blur(60px);
    animation: aurora-pulse 12s ease-in-out infinite;
    pointer-events: none;
  }
  @keyframes aurora-pulse {
    0%, 100% { transform: rotate(0deg) scale(1); opacity: 0.8; }
    50% { transform: rotate(180deg) scale(1.2); opacity: 1; }
  }
  .empty-hero-content {
    position: relative; z-index: 1;
    display: flex; flex-direction: column; align-items: center;
    gap: 14px;
    padding: 48px 32px;
    max-width: 820px;
    width: 100%;
  }
  .empty-hero-badge {
    font-size: 10px; font-weight: 700;
    color: var(--accent-hot);
    background: rgba(124, 92, 255, 0.12);
    border: 1px solid rgba(124, 92, 255, 0.3);
    padding: 5px 11px; border-radius: 999px;
    letter-spacing: 1.2px;
    text-transform: uppercase;
    box-shadow: 0 0 14px rgba(124, 92, 255, 0.2);
  }
  .empty-hero-icon {
    position: relative;
    width: 110px; height: 110px;
    display: flex; align-items: center; justify-content: center;
    margin: 8px 0;
  }
  .empty-hero-icon-bg {
    position: absolute; inset: 0;
    background: linear-gradient(135deg, var(--accent) 0%, var(--red) 100%);
    border-radius: 28px;
    box-shadow:
      0 12px 40px rgba(124, 92, 255, 0.5),
      0 0 60px rgba(255, 61, 94, 0.3),
      inset 0 1px 0 rgba(255, 255, 255, 0.25);
    animation: hero-pulse 3s ease-in-out infinite;
  }
  .empty-hero-icon-glyph {
    position: relative; z-index: 1;
    font-size: 44px; color: #fff;
    line-height: 1;
    text-shadow: 0 4px 14px rgba(0, 0, 0, 0.4);
    transform: translateX(2px);
  }
  @keyframes hero-pulse {
    0%, 100% { transform: scale(1); box-shadow: 0 12px 40px rgba(124, 92, 255, 0.5), 0 0 60px rgba(255, 61, 94, 0.3), inset 0 1px 0 rgba(255, 255, 255, 0.25); }
    50% { transform: scale(1.04); box-shadow: 0 16px 50px rgba(124, 92, 255, 0.65), 0 0 80px rgba(255, 61, 94, 0.45), inset 0 1px 0 rgba(255, 255, 255, 0.3); }
  }
  .empty-hero-title {
    font-size: 28px; font-weight: 700;
    background: linear-gradient(135deg, #fff 0%, #c8b5ff 100%);
    -webkit-background-clip: text; background-clip: text;
    -webkit-text-fill-color: transparent;
    letter-spacing: -0.4px;
    margin: 4px 0 0;
  }
  .empty-hero-sub {
    font-size: 13px; color: var(--text2); line-height: 1.6;
    max-width: 420px;
  }
  .empty-hero-cta {
    margin-top: 8px;
    padding: 12px 24px;
    font-size: 13px; font-weight: 600;
  }
  .empty-hero-cta-row {
    display: flex; gap: 10px; flex-wrap: wrap; justify-content: center;
    margin-top: 8px;
  }
  .empty-hero-cta-row .empty-hero-cta { margin-top: 0; }
  .empty-hero-cta-secondary {
    padding: 12px 22px;
    font-size: 13px; font-weight: 600;
    background: rgba(255, 255, 255, 0.04);
    border: 1px solid rgba(124, 92, 255, 0.3);
    color: var(--accent-hot, #9b80ff);
    backdrop-filter: blur(10px);
  }
  .empty-hero-cta-secondary:hover:not(:disabled) {
    background: rgba(124, 92, 255, 0.15);
    border-color: rgba(124, 92, 255, 0.55);
    color: #fff;
  }
  .empty-hero-droptip {
    margin-top: 14px;
    font-size: 11px; font-weight: 600; letter-spacing: 0.4px;
    color: var(--text3);
    padding: 8px 16px;
    border: 1.5px dashed rgba(124, 92, 255, 0.28);
    border-radius: 999px;
    background: rgba(124, 92, 255, 0.04);
    text-transform: uppercase;
  }
  .empty-hero-hints {
    display: flex; align-items: center; gap: 10px;
    margin-top: 12px;
    flex-wrap: wrap; justify-content: center;
  }
  .empty-hero-hint {
    font-size: 11px; color: var(--text2);
    font-weight: 500;
  }
  .empty-hero-hint-dot { color: var(--text3); font-size: 11px; }

  .card-title {
    display: flex; align-items: center; gap: 6px;
    font-size: 10.5px; font-weight: 600; color: var(--text2);
    text-transform: uppercase; letter-spacing: 0.7px;
    margin-bottom: 10px;
  }
  .card-title-row {
    display: flex; align-items: center; justify-content: space-between;
    gap: 8px; margin-bottom: 10px;
  }
  .card-title-row .card-title { margin-bottom: 0; }
  .card-title-actions { display: flex; gap: 4px; }

  /* Drop target visual hint */
  .drop-target { position: relative; }
  .drop-target::before {
    content: "";
    position: absolute;
    inset: 0;
    border-radius: 10px;
    border: 1px dashed transparent;
    pointer-events: none;
    transition: border-color 150ms;
  }

  /* Source rows — glass tiles */
  .source-list { display: flex; flex-direction: column; gap: 6px; position: relative; z-index: 1; }
  .source-row {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: 12px;
    padding: 10px 12px;
    border-radius: 10px;
    background: rgba(255, 255, 255, 0.03);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
    border: 1px solid var(--border);
    transition: all 200ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .source-row:hover {
    background: rgba(255, 255, 255, 0.06);
    border-color: var(--border-hi);
  }
  .source-row.filled {
    background: linear-gradient(135deg, rgba(92, 201, 153, 0.10), rgba(92, 201, 153, 0.04));
    border-color: rgba(92, 201, 153, 0.35);
    box-shadow: 0 0 0 1px rgba(92, 201, 153, 0.06), inset 0 1px 0 rgba(255, 255, 255, 0.04);
  }
  .source-row-stacked { grid-template-columns: auto 1fr auto; align-items: start; }
  .source-row-stacked > .source-num { margin-top: 1px; }
  .source-row-placeholder {
    width: 100%;
    text-align: center;
    background: transparent;
    border: 1px dashed rgba(255, 255, 255, 0.12);
    border-radius: 10px;
    padding: 11px 12px;
    color: var(--text3);
    font: inherit; font-size: 12px;
    cursor: pointer;
    transition: all 200ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .source-row-placeholder:hover {
    color: var(--accent-hot); border-color: rgba(124, 92, 255, 0.5);
    background: rgba(124, 92, 255, 0.06);
    box-shadow: 0 0 18px rgba(124, 92, 255, 0.15);
  }
  .source-num {
    width: 26px; height: 26px; border-radius: 50%;
    display: flex; align-items: center; justify-content: center;
    background: rgba(124, 92, 255, 0.18);
    color: var(--accent-hot);
    font-size: 11px; font-weight: 700;
    flex-shrink: 0;
    border: 1px solid rgba(124, 92, 255, 0.3);
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.08);
  }
  .source-row.filled .source-num {
    background: linear-gradient(135deg, var(--green), #4ab685);
    color: #0a0712;
    border-color: rgba(92, 201, 153, 0.5);
    box-shadow: 0 0 12px rgba(92, 201, 153, 0.4), inset 0 1px 0 rgba(255, 255, 255, 0.25);
  }
  .source-info { min-width: 0; }
  .source-label {
    font-size: 11px; font-weight: 600; color: var(--text2);
  }
  .source-value {
    font-size: 12px; color: var(--text);
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    font-family: "SF Mono", "Consolas", monospace;
    margin-top: 1px;
  }
  .source-value.empty { color: var(--text3); font-style: italic; font-family: inherit; }

  .source-row-actions { display: flex; gap: 4px; align-items: center; flex-shrink: 0; }

  /* Compat grid (durée/FPS/VFQ) */
  .compat-grid {
    display: flex; flex-wrap: wrap; gap: 4px 14px; margin-top: 5px;
    font-size: 11px; color: var(--text2);
  }
  .compat-ok { color: var(--green); font-weight: 600; }
  .compat-bad { color: var(--red); font-weight: 600; }

  /* FR audio options (toggle row) */
  .fr-audio-options {
    display: flex; flex-wrap: wrap; gap: 10px; align-items: center;
    margin-top: 6px;
  }

  /* Extract progress bars */
  .extract-progress {
    display: flex; flex-direction: column; gap: 4px;
    margin-top: 6px;
  }
  .extract-progress-label { font-size: 11px; color: var(--text2); }
  .extract-progress progress {
    width: 100%; height: 5px; border: none; border-radius: 3px;
    background: rgba(0,0,0,0.4); overflow: hidden;
    appearance: none; -webkit-appearance: none;
  }
  .extract-progress progress::-webkit-progress-bar { background: rgba(0,0,0,0.4); border-radius: 3px; }
  .extract-progress progress::-webkit-progress-value {
    background: linear-gradient(90deg, var(--accent), var(--accent-hot));
    border-radius: 3px;
  }
  .extract-progress progress:not([value]) {
    background:
      linear-gradient(90deg, transparent 0%, var(--accent-hot) 50%, transparent 100%) 0/40% 100% no-repeat,
      rgba(0,0,0,0.4);
    animation: ind-slide 1.4s linear infinite;
  }
  @keyframes ind-slide { 0% { background-position: -40% 0; } 100% { background-position: 140% 0; } }
  .extract-progress.done .extract-progress-label { color: var(--green); font-weight: 600; }
  .extract-progress.done progress[value]::-webkit-progress-value {
    background: linear-gradient(90deg, #2f9e44, var(--green));
  }
  .extract-progress.err .extract-progress-label { color: var(--red); font-weight: 600; }
  .extract-progress.err progress[value]::-webkit-progress-value {
    background: linear-gradient(90deg, #c92a2a, var(--red));
  }

  /* Module sync subs externes */
  .sub-sync-module {
    margin-top: 7px;
    padding: 6px 8px;
    border-radius: 6px;
    background: rgba(255, 255, 255, 0.025);
    border: 1px solid var(--border);
  }
  .sync-results-header {
    font-size: 10px; font-weight: 600; color: var(--text2);
    text-transform: uppercase; letter-spacing: 0.6px;
    margin-bottom: 4px;
  }
  .sync-results-list {
    list-style: none; padding: 0; margin: 0;
    display: flex; flex-direction: column; gap: 2px;
    font-size: 11px;
  }
  .sync-results-list li {
    display: flex; gap: 10px; justify-content: space-between; align-items: center;
    padding: 2px 5px; border-radius: 4px;
  }
  .sync-results-list li.has-offset { background: rgba(255, 181, 71, 0.10); }
  .sync-results-list li.has-offset .sync-result-status { color: var(--orange); font-weight: 600; }
  .sync-results-list li.err .sync-result-status { color: var(--red); }
  .sync-result-name { color: var(--text2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sync-actions { display: flex; gap: 5px; margin-top: 5px; }

  /* Réglages (field-grid 4 cols) */
  .field-grid {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 8px;
  }
  @media (max-width: 800px) { .field-grid { grid-template-columns: repeat(2, 1fr); } }
  .field { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
  .field-label, .field label {
    font-size: 10px; color: var(--text2); font-weight: 500;
    text-transform: uppercase; letter-spacing: 0.4px;
  }
  .field input, .field select, .field-row input, .field-row select {
    background: rgba(0, 0, 0, 0.35);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1px solid var(--border);
    border-radius: 7px;
    padding: 6px 9px;
    color: var(--text);
    font-size: 11.5px; font-family: inherit;
    min-width: 0;
    transition: border-color 180ms ease, box-shadow 180ms ease;
  }
  .field input:focus, .field select:focus, .field-row input:focus, .field-row select:focus {
    outline: none;
    border-color: var(--accent-soft);
    box-shadow: 0 0 0 3px rgba(124, 92, 255, 0.15);
  }
  .field input[readonly] { color: var(--text2); background: rgba(0,0,0,0.45); }
  .field-row { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; }
  .field-row > input, .field-row > select { flex: 1; min-width: 0; }
  .field-hint { font-size: 11px; color: var(--text3); margin-top: 4px; line-height: 1.5; }

  /* Tracks — glass rows */
  .tracks-section {
    margin-bottom: 14px;
    padding: 10px 12px 8px;
    background: rgba(255, 255, 255, 0.015);
    border: 1px solid rgba(255, 255, 255, 0.04);
    border-radius: 12px;
  }
  .tracks-section:last-child { margin-bottom: 0; }
  .tracks-section-header {
    display: flex; align-items: center; gap: 8px;
    margin-bottom: 8px; padding: 0 2px 8px;
    font-size: 10.5px; font-weight: 700; letter-spacing: 0.6px;
    text-transform: uppercase;
    border-bottom: 1px dashed rgba(255, 255, 255, 0.06);
  }
  .tracks-section-dot {
    width: 8px; height: 8px; border-radius: 50%;
    flex-shrink: 0;
    box-shadow: 0 0 8px currentColor;
  }
  .tracks-section-label { flex: 1; }
  .tracks-section-count {
    background: rgba(255, 255, 255, 0.06);
    border: 1px solid rgba(255, 255, 255, 0.08);
    color: var(--text);
    padding: 2px 7px; border-radius: 999px;
    font-size: 10px; font-weight: 700;
    min-width: 22px; text-align: center;
  }
  .tracks-section-header.video { color: var(--accent-hot); }
  .tracks-section-header.video .tracks-section-dot { background: var(--accent-hot); }
  .tracks-section-header.video { border-bottom-color: rgba(124, 92, 255, 0.20); }
  .tracks-section-header.audio { color: var(--orange); }
  .tracks-section-header.audio .tracks-section-dot { background: var(--orange); }
  .tracks-section-header.audio { border-bottom-color: rgba(255, 181, 71, 0.20); }
  .tracks-section-header.sub { color: var(--pink); }
  .tracks-section-header.sub .tracks-section-dot { background: var(--pink); }
  .tracks-section-header.sub { border-bottom-color: rgba(255, 92, 179, 0.20); }

  .track {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: 10px;
    padding: 7px 10px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.03);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
    border: 1px solid var(--border);
    font-size: 11.5px;
    margin-bottom: 4px;
    transition: all 180ms ease;
  }
  .track:hover { background: rgba(255, 255, 255, 0.05); border-color: var(--border-hi); }
  .track.track-editable { align-items: flex-start; grid-template-columns: auto 1fr 84px; }
  .track.dropped { opacity: 0.45; }
  .track-icon {
    width: 20px; height: 20px; border-radius: 5px;
    display: flex; align-items: center; justify-content: center;
    font-size: 10px; font-weight: 700;
    flex-shrink: 0;
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.08);
  }
  .track-icon.video { background: rgba(124, 92, 255, 0.22); color: var(--accent-hot); border: 1px solid rgba(124, 92, 255, 0.3); }
  .track-icon.audio { background: rgba(255, 181, 71, 0.22); color: var(--orange); border: 1px solid rgba(255, 181, 71, 0.3); }
  .track-icon.sub { background: rgba(255, 92, 179, 0.22); color: var(--pink); border: 1px solid rgba(255, 92, 179, 0.3); }
  .track-label {
    color: var(--text); font-weight: 500;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .track-body { min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  .track-controls {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 78px 90px 82px 28px 28px 28px;
    gap: 5px; align-items: center;
    font-size: 11px;
  }
  .track-controls select {
    background: rgba(0, 0, 0, 0.4);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1px solid var(--border);
    border-radius: 5px;
    padding: 3px 7px;
    color: var(--text);
    font-size: 11px; font-family: inherit;
    width: 100%;
    min-width: 0;
    transition: border-color 180ms ease, box-shadow 180ms ease;
  }
  .track-controls .chk { justify-content: center; width: 100%; }
  .track-controls .btn-arrow { width: 100%; padding-left: 0; padding-right: 0; }
  .track-controls .btn-arrow:disabled { opacity: 0.4; cursor: not-allowed; }
  .ocr-progress {
    margin-top: 6px;
    position: relative;
    height: 22px;
    border-radius: 5px;
    background: rgba(255,255,255,0.04);
    border: 1px solid var(--border);
    overflow: hidden;
    transition: border-color 200ms;
  }
  .ocr-progress.ocr-running { border-color: rgba(124,92,255,0.4); }
  .ocr-progress.ocr-done {
    border-color: rgba(34, 197, 94, 0.85);
    background: rgba(22, 163, 74, 0.32);
    box-shadow: 0 0 0 2px rgba(34, 197, 94, 0.25), 0 0 18px rgba(34, 197, 94, 0.35);
  }
  .ocr-progress.ocr-error {
    border-color: rgba(255, 61, 94, 0.55);
    background: rgba(255, 61, 94, 0.10);
  }
  .ocr-progress-bar {
    position: absolute; top: 0; left: 0; bottom: 0;
    background: linear-gradient(90deg, rgba(124,92,255,0.5), rgba(124,92,255,0.8));
    transition: width 250ms ease-out, background 250ms ease;
  }
  .ocr-progress.ocr-done .ocr-progress-bar {
    background: linear-gradient(90deg, rgba(22, 163, 74, 0.85), rgba(34, 197, 94, 1));
    width: 100% !important;
  }
  .ocr-progress.ocr-error .ocr-progress-bar {
    background: linear-gradient(90deg, rgba(255, 61, 94, 0.55), rgba(255, 61, 94, 0.85));
  }
  .ocr-progress-label {
    position: relative;
    font-size: 0.74rem;
    line-height: 22px;
    padding: 0 10px;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-weight: 500;
  }
  .ocr-progress.ocr-done .ocr-progress-label { color: #fff; font-weight: 600; }
  .ocr-progress.ocr-done .ocr-progress-label b { color: #d1fae5; }
  @media (max-width: 1280px) {
    .track-controls { grid-template-columns: minmax(0, 1fr) 66px 78px 70px 26px 26px 26px; }
  }
  .track-controls select:hover { border-color: var(--border-hi); }
  .track-controls select:focus {
    outline: none;
    border-color: var(--accent-soft);
    box-shadow: 0 0 0 2px rgba(124, 92, 255, 0.15);
  }
  .track-controls .chk {
    display: inline-flex; align-items: center; gap: 4px;
    color: var(--text2); cursor: pointer; user-select: none;
    padding: 2px 7px; border-radius: 5px;
    background: rgba(255, 255, 255, 0.03);
    border: 1px solid transparent;
    transition: all 150ms ease;
  }
  .track-controls .chk:hover {
    color: var(--text); background: rgba(255, 255, 255, 0.06);
    border-color: var(--border);
  }
  .track-controls .chk input { margin: 0; accent-color: var(--accent); }
  .track-flag {
    font-size: 9.5px; padding: 2px 8px; border-radius: 5px;
    background: rgba(124, 92, 255, 0.18); color: var(--accent-hot);
    font-weight: 700; letter-spacing: 0.3px;
    text-transform: uppercase;
    flex-shrink: 0;
    border: 1px solid rgba(124, 92, 255, 0.25);
    text-align: center;
    min-width: 64px;
    justify-self: end;
  }
  .track-flag.success { background: rgba(92, 201, 153, 0.18); color: var(--green); border-color: rgba(92, 201, 153, 0.3); }
  .track-flag.warn { background: rgba(255, 181, 71, 0.18); color: var(--orange); border-color: rgba(255, 181, 71, 0.3); }
  .track-flag.err { background: rgba(255, 61, 94, 0.18); color: var(--red); border-color: rgba(255, 61, 94, 0.3); }

  .btn-arrow {
    background: transparent; border: 1px solid var(--border);
    color: var(--text2); cursor: pointer;
    padding: 1px 5px; border-radius: 4px;
    font: inherit; font-size: 10px;
    transition: all 120ms;
  }
  .btn-arrow:hover { color: var(--text); background: rgba(255,255,255,0.04); border-color: var(--border-hi); }
  .btn-arrow.danger:hover { color: var(--red-hot); border-color: rgba(255,61,94,0.4); background: rgba(255,61,94,0.05); }

  .empty-hint { font-size: 11px; color: var(--text3); padding: 8px 4px; font-style: italic; }

  /* Filename preview — premium gradient pill */
  .filename {
    background: linear-gradient(135deg, rgba(124, 92, 255, 0.14), rgba(255, 61, 94, 0.08));
    border: 1px solid rgba(124, 92, 255, 0.35);
    border-radius: 10px;
    padding: 11px 14px;
    font-family: "SF Mono", "Consolas", monospace;
    font-size: 12px;
    word-break: break-all;
    line-height: 1.55;
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.05), 0 0 18px rgba(124, 92, 255, 0.08);
  }
  .filename-input {
    width: 100%;
    background: rgba(0, 0, 0, 0.4);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 9px 12px;
    color: var(--text);
    font-family: "SF Mono", "Consolas", monospace;
    font-size: 12px;
    transition: border-color 180ms ease, box-shadow 180ms ease;
  }
  .filename-input:focus {
    outline: none; border-color: var(--accent-soft);
    box-shadow: 0 0 0 3px rgba(124, 92, 255, 0.18);
  }
  .filename-actions { display: flex; gap: 5px; margin-top: 8px; flex-wrap: wrap; }

  /* Validation pills */
  .validation { display: flex; flex-wrap: wrap; gap: 6px; }
  .pill {
    display: inline-flex; align-items: center; gap: 4px;
    padding: 4px 10px; border-radius: 999px;
    font-size: 10.5px; font-weight: 700;
    border: 1px solid;
    letter-spacing: 0.3px;
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
  }
  .pill.ok { background: rgba(92, 201, 153, 0.14); color: var(--green); border-color: rgba(92, 201, 153, 0.35); box-shadow: 0 0 12px rgba(92, 201, 153, 0.12); }
  .pill.warn { background: rgba(255, 181, 71, 0.14); color: var(--orange); border-color: rgba(255, 181, 71, 0.35); box-shadow: 0 0 12px rgba(255, 181, 71, 0.12); }
  .pill.err { background: rgba(255, 61, 94, 0.14); color: var(--red); border-color: rgba(255, 61, 94, 0.35); box-shadow: 0 0 12px rgba(255, 61, 94, 0.12); }

  /* TMDB preview */
  .tmdb-preview {
    display: flex; gap: 12px;
    margin-bottom: 6px;
  }
  .tmdb-preview-poster {
    width: 78px; height: 117px; flex-shrink: 0;
    object-fit: cover; border-radius: 8px;
    border: 1px solid var(--border-hi);
    box-shadow: 0 6px 18px rgba(0, 0, 0, 0.4);
  }
  .tmdb-preview-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
  .tmdb-preview-title {
    font-size: 13px; font-weight: 700; color: var(--text); line-height: 1.2;
    overflow: hidden; text-overflow: ellipsis;
  }
  .tmdb-preview-vo { font-size: 11px; color: var(--text2); font-style: italic; }
  .tmdb-preview-meta { display: flex; flex-wrap: wrap; gap: 8px; font-size: 11px; color: var(--text2); }
  .tmdb-preview-override {
    display: flex; align-items: center; gap: 5px;
    margin-top: 6px; padding-top: 6px;
    border-top: 1px dashed var(--border);
    font-size: 11px; flex-wrap: wrap;
  }
  .tmdb-preview-override-label { color: var(--text3); }
  .tmdb-preview-override-input {
    flex: 1; max-width: 130px; min-width: 80px;
    padding: 4px 8px; font-size: 11px;
    background: rgba(0,0,0,0.3); border: 1px solid var(--border); border-radius: 5px;
    color: var(--text);
    font-family: "SF Mono", monospace;
  }
  .tmdb-preview-override-input:focus { outline: none; border-color: var(--accent); }

  /* TMDB list (recherche inline) */
  .tmdb-list {
    list-style: none; padding: 0; margin: 6px 0 0;
    display: flex; flex-direction: column; gap: 4px;
  }
  .tmdb-item {
    display: flex; gap: 8px; align-items: flex-start;
    padding: 6px;
    background: rgba(255,255,255,0.025);
    border: 1px solid var(--border);
    border-radius: 6px;
    cursor: pointer; width: 100%;
    color: var(--text); font: inherit;
    text-align: left;
    transition: all 120ms;
  }
  .tmdb-item:hover { background: rgba(124,92,255,0.08); border-color: rgba(124,92,255,0.3); }
  .tmdb-poster { width: 36px; height: 54px; object-fit: cover; border-radius: 4px; flex-shrink: 0; }
  .tmdb-body { min-width: 0; flex: 1; }
  .tmdb-title { font-size: 12px; font-weight: 600; }
  .tmdb-year { color: var(--text3); font-weight: 500; }
  .tmdb-meta { font-size: 11px; color: var(--text2); margin-top: 2px; }

  /* TMDB picked (pour cible) */
  .tmdb-picked {
    display: flex; gap: 12px;
    padding: 10px;
    background: rgba(124,92,255,0.05);
    border: 1px solid rgba(124,92,255,0.25);
    border-radius: 8px;
    margin-top: 8px;
  }
  .tmdb-picked-poster { width: 90px; height: 135px; object-fit: cover; border-radius: 6px; flex-shrink: 0; }
  .tmdb-picked-body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  .tmdb-picked-title { font-size: 14px; font-weight: 700; }
  .tmdb-picked-vo { font-size: 11px; color: var(--text2); font-style: italic; }
  .tmdb-picked-meta { font-size: 11px; color: var(--text2); }
  .tmdb-picked-overview { font-size: 11px; color: var(--text2); margin-top: 4px; line-height: 1.4; }
  .tmdb-alts { margin-top: 8px; }
  .tmdb-alts summary { font-size: 11px; color: var(--text2); cursor: pointer; padding: 4px 0; }

  /* Lang toggle — glass pill */
  .lang-toggle {
    display: inline-flex; gap: 0;
    background: rgba(0, 0, 0, 0.4);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 2px;
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.03);
  }
  .lang-toggle button {
    background: transparent; border: none; color: var(--text2);
    font: inherit; font-size: 11px; font-weight: 500;
    padding: 5px 11px; border-radius: 6px; cursor: pointer;
    transition: all 180ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .lang-toggle button.active {
    background: linear-gradient(135deg, var(--accent), #6845e8);
    color: #fff; font-weight: 600;
    box-shadow: 0 2px 8px rgba(124, 92, 255, 0.35), inset 0 1px 0 rgba(255, 255, 255, 0.15);
  }
  .lang-toggle button:not(.active):hover { color: var(--text); background: rgba(255, 255, 255, 0.04); }

  /* VFi toggle (label) */
  .vfq-toggle {
    display: flex; align-items: center; gap: 6px;
    font-size: 11px; flex-wrap: wrap;
  }
  .vfq-toggle input[type="checkbox"] { margin: 0; }
  .vfq-yes { color: var(--green); font-weight: 600; }
  .vfq-no  { color: var(--text3); }
  .vfq-link {
    background: rgba(124, 92, 255, 0.14);
    border: 1px solid rgba(124, 92, 255, 0.35);
    color: var(--accent-hot);
    font: inherit; font-size: 10.5px; font-weight: 600;
    padding: 3px 9px; border-radius: 5px;
    cursor: pointer;
    transition: all 180ms cubic-bezier(0.2, 0.8, 0.2, 1);
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
  }
  .vfq-link:hover {
    background: rgba(124, 92, 255, 0.28);
    border-color: rgba(124, 92, 255, 0.55);
    box-shadow: 0 0 12px rgba(124, 92, 255, 0.25);
  }
  .srt-label { color: var(--text2); font-weight: 600; }

  /* Mux status banner */
  .mux-status-banner.success {
    border-color: rgba(92, 201, 153, 0.45);
    background: linear-gradient(135deg, rgba(92, 201, 153, 0.16), rgba(47, 158, 68, 0.08));
    box-shadow: 0 0 22px rgba(92, 201, 153, 0.18), var(--shadow-card);
  }
  .mux-status-banner.error {
    border-color: rgba(255, 61, 94, 0.45);
    background: linear-gradient(135deg, rgba(255, 61, 94, 0.16), rgba(201, 42, 42, 0.08));
    box-shadow: 0 0 22px rgba(255, 61, 94, 0.18), var(--shadow-card);
  }
  .auto-status { font-size: 13px; padding: 2px 0; font-weight: 600; }
  .auto-status.done { color: var(--green); }
  .auto-status.error { color: var(--red); }

  /* Queue */
  .queue-list {
    list-style: none; padding: 0; margin: 0;
    display: flex; flex-direction: column; gap: 4px;
    max-height: 200px; overflow: auto;
  }
  .queue-row {
    display: grid;
    grid-template-columns: auto 1fr auto auto;
    align-items: center; gap: 8px;
    padding: 6px 10px;
    background: rgba(255, 255, 255, 0.03);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1px solid var(--border);
    border-radius: 8px;
    font-size: 11.5px;
    transition: all 180ms ease;
  }
  .queue-row:hover { background: rgba(255, 255, 255, 0.06); border-color: var(--border-hi); }
  .queue-idx {
    width: 20px; height: 20px; border-radius: 50%;
    background: rgba(124,92,255,0.15); color: var(--accent-hot);
    display: flex; align-items: center; justify-content: center;
    font-size: 10px; font-weight: 700;
  }
  .queue-name { color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .queue-actions { display: flex; gap: 4px; margin-top: 6px; }

  /* Tools row */
  .tools-row { display: flex; gap: 6px; flex-wrap: wrap; }

  /* Sync (audio sync) tracks-table */
  .tracks-table {
    width: 100%; border-collapse: collapse;
    font-size: 11.5px;
  }
  .tracks-table th {
    text-align: left; padding: 6px 8px;
    font-size: 10px; font-weight: 600; color: var(--text2);
    text-transform: uppercase; letter-spacing: 0.5px;
    border-bottom: 1px solid var(--border);
  }
  .tracks-table td {
    padding: 6px 8px;
    border-bottom: 1px solid var(--border);
    color: var(--text);
  }
  .tracks-table tr.ref td { background: rgba(124,92,255,0.06); }
  .tracks-table input[type="number"] {
    background: rgba(0,0,0,0.3); border: 1px solid var(--border);
    border-radius: 5px; padding: 3px 6px; color: var(--text);
    font: inherit; font-size: 11px;
  }

  /* Result badge */
  .result-badge {
    display: inline-block;
    padding: 3px 8px; border-radius: 5px;
    font-size: 11px; font-weight: 600;
    margin-top: 4px;
  }
  .result-badge.ok { background: rgba(92,201,153,0.12); color: var(--green); }
  .result-badge.err { background: rgba(255,61,94,0.12); color: var(--red); }

  /* Hint paragraph */
  .hint { font-size: 12px; color: var(--text2); line-height: 1.5; }
  .hint code { background: rgba(0,0,0,0.4); padding: 1px 5px; border-radius: 3px; font-family: "SF Mono", monospace; }

  /* Actions row */
  .actions-row {
    display: flex; gap: 8px; align-items: center; flex-wrap: wrap;
    margin-top: 6px;
  }

  /* Btn-test, btn-secondary, btn-primary, btn-copy (legacy classes) */
  .btn-test, .btn-secondary, .btn-primary, .btn-copy {
    border: 1px solid var(--border);
    background: rgba(40, 33, 54, 0.5);
    backdrop-filter: blur(12px) saturate(160%);
    -webkit-backdrop-filter: blur(12px) saturate(160%);
    color: var(--text);
    font: inherit; font-size: 11.5px; font-weight: 500;
    padding: 6px 12px; border-radius: 7px;
    cursor: pointer;
    transition: all 180ms cubic-bezier(0.2, 0.8, 0.2, 1);
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
  }
  .btn-test:hover:not(:disabled),
  .btn-secondary:hover:not(:disabled),
  .btn-copy:hover:not(:disabled) {
    background: rgba(60, 50, 80, 0.6);
    border-color: var(--border-hi);
    transform: translateY(-1px);
  }
  .btn-primary {
    background: linear-gradient(135deg, var(--accent), #6845e8);
    border-color: rgba(124, 92, 255, 0.55);
    color: #fff; font-weight: 600;
    box-shadow: 0 4px 14px rgba(124, 92, 255, 0.4), inset 0 1px 0 rgba(255, 255, 255, 0.18);
  }
  .btn-primary:hover:not(:disabled) {
    background: linear-gradient(135deg, var(--accent-hot), var(--accent));
    transform: translateY(-1px);
    box-shadow: 0 6px 20px rgba(124, 92, 255, 0.55), inset 0 1px 0 rgba(255, 255, 255, 0.22);
  }
  .btn-test:disabled, .btn-secondary:disabled, .btn-primary:disabled, .btn-copy:disabled { opacity: 0.4; cursor: not-allowed; }
  .btn-auto { /* used as alias near "+ Extraire + sync" */
    padding: 4px 10px; font-size: 11px;
  }

  /* ─── FOOTER (Journal) — glass ─── */
  .journal {
    background: rgba(10, 7, 18, 0.55);
    backdrop-filter: blur(20px) saturate(170%);
    -webkit-backdrop-filter: blur(20px) saturate(170%);
    border-top: 1px solid var(--border);
    padding: 8px 16px 10px;
    max-height: 120px; overflow-y: auto;
    font-family: "SF Mono", "Consolas", monospace;
    font-size: 10.5px;
    position: relative;
    z-index: 9;
    box-shadow: 0 -4px 18px rgba(0, 0, 0, 0.3);
  }
  .journal::before {
    content: "";
    position: absolute; left: 0; right: 0; top: -1px; height: 1px;
    background: linear-gradient(90deg, transparent, rgba(124, 92, 255, 0.35), rgba(255, 61, 94, 0.25), transparent);
    pointer-events: none;
  }
  .journal-line { display: flex; gap: 8px; padding: 1px 0; }
  .log-time { color: var(--text3); flex-shrink: 0; font-variant-numeric: tabular-nums; }
  .log-msg { word-break: break-word; white-space: pre-wrap; }
  :global(.log-msg.lvl-default)  { color: var(--text); }
  :global(.log-msg.lvl-progress) { color: var(--orange); }
  :global(.log-msg.lvl-ok)       { color: var(--green); }
  :global(.log-msg.lvl-error)    { color: var(--red); }
  .log-msg.ok   { color: var(--green); }
  .log-msg.info { color: var(--text2); }
  .log-msg.warn { color: var(--orange); }
  .log-msg.err  { color: var(--red); }

  :global(::-webkit-scrollbar) { width: 8px; height: 8px; }
  :global(::-webkit-scrollbar-track) { background: transparent; }
  :global(::-webkit-scrollbar-thumb) {
    background: rgba(255, 255, 255, 0.10);
    border-radius: 4px;
    border: 1px solid rgba(255, 255, 255, 0.04);
  }
  :global(::-webkit-scrollbar-thumb:hover) { background: rgba(124, 92, 255, 0.35); }

  /* Modes / responsive : when no source loaded we want full width hero */
  @media (max-width: 1100px) {
    .app.no-source .main {
      grid-template-rows: auto 1fr;
    }
  }
  /* When tracks-table appears in fullscreen, glass-style it too */
  .tracks-table {
    border-radius: 8px; overflow: hidden;
    background: rgba(255, 255, 255, 0.02);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1px solid var(--border);
  }
  .tracks-table th {
    background: rgba(255, 255, 255, 0.04);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
  }
  .tracks-table tbody tr {
    transition: background-color 150ms ease;
  }
  .tracks-table tbody tr:hover td {
    background: rgba(255, 255, 255, 0.03);
  }
  .tracks-table tr.ref td {
    background: linear-gradient(90deg, rgba(124, 92, 255, 0.12), rgba(124, 92, 255, 0.04));
  }
  .tracks-table input[type="number"]:focus {
    outline: none;
    border-color: var(--accent-soft);
    box-shadow: 0 0 0 2px rgba(124, 92, 255, 0.18);
  }

  /* Preview box (cible screen) — glass premium pill comme la card filename */
  .preview-box {
    margin-top: 10px;
    padding: 12px 14px;
    border-radius: 12px;
    background: linear-gradient(135deg, rgba(124, 92, 255, 0.10), rgba(255, 61, 94, 0.06));
    backdrop-filter: blur(14px) saturate(160%);
    -webkit-backdrop-filter: blur(14px) saturate(160%);
    border: 1px solid rgba(124, 92, 255, 0.30);
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.06),
      0 0 22px rgba(124, 92, 255, 0.10);
  }
  .preview-label {
    font-size: 10px; font-weight: 700; color: var(--accent-hot);
    text-transform: uppercase; letter-spacing: 0.7px;
    margin-bottom: 6px;
  }
  .preview-value {
    background: rgba(0, 0, 0, 0.35);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1px solid rgba(255, 255, 255, 0.06);
    border-radius: 8px;
    padding: 8px 12px;
    font-size: 12px;
    color: var(--text);
    word-break: break-all;
    line-height: 1.55;
    flex: 1;
    min-width: 0;
  }
  .preview-filename-row {
    display: flex; align-items: center; gap: 6px;
    flex-wrap: wrap;
    margin-top: 6px;
  }

  /* Section title (sync screen, etc.) */
  .section-title {
    font-size: 11px; font-weight: 700; color: var(--text2);
    text-transform: uppercase; letter-spacing: 0.6px;
    margin: 4px 0 6px;
  }

  /* TMDB inline poster (recherche list) — shadow subtile glass */
  .tmdb-poster { box-shadow: 0 3px 10px rgba(0, 0, 0, 0.35); }

  /* TMDB picked poster — shadow + bordure glass */
  .tmdb-picked-poster {
    box-shadow: 0 6px 22px rgba(0, 0, 0, 0.45);
    border: 1px solid var(--border-hi);
  }

  /* Validation pills container — petit espacement quand placé hors d'une card */
  .validation { margin-top: 2px; }

  /* Filename mono — coloration accent sur extension/segments connus via segments dans le markup
     (ici juste un léger highlight glass pour la couleur de fond) */
  .filename:hover {
    border-color: rgba(124, 92, 255, 0.5);
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.07), 0 0 26px rgba(124, 92, 255, 0.16);
  }

  /* Drop-target : feedback visuel quand on hover (Wails drop zone) */
  .drop-target:hover::before {
    border-color: rgba(124, 92, 255, 0.25);
  }

  /* Actions-row dans .fullscreen : glass container subtil pour Mux button + progress */
  .fullscreen .actions-row {
    padding: 10px 12px;
    background: rgba(26, 22, 34, 0.45);
    backdrop-filter: blur(14px) saturate(160%);
    -webkit-backdrop-filter: blur(14px) saturate(160%);
    border: 1px solid var(--border);
    border-radius: 10px;
    box-shadow: var(--shadow-card);
  }

  /* Result badge — backdrop blur cohérent */
  .result-badge {
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1px solid transparent;
  }
  .result-badge.ok { border-color: rgba(92, 201, 153, 0.30); }
  .result-badge.err { border-color: rgba(255, 61, 94, 0.30); }

  /* Hint paragraph — subtle glass when used inside cards */
  .hint code {
    background: rgba(0,0,0,0.45);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1px solid var(--border);
  }

  /* Sync screen : table inline-style cells (drift_linear / drift_unstable / low_confidence)
     gardent leurs fonds inline mais on les harmonise via :global() pour le rounded glass feel */
  :global(.tracks-table td > div[style*="background:#5a3a00"]) {
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1px solid rgba(255, 209, 102, 0.25);
  }
  :global(.tracks-table td > div[style*="background:#5a1f1f"]) {
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1px solid rgba(255, 179, 179, 0.25);
  }

  /* TMDB override input : focus glow accent */
  .tmdb-preview-override-input {
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    transition: border-color 180ms ease, box-shadow 180ms ease;
  }
  .tmdb-preview-override-input:focus {
    outline: none;
    border-color: var(--accent-soft);
    box-shadow: 0 0 0 2px rgba(124, 92, 255, 0.15);
  }

  /* Filename input focus shadow déjà OK, mais on ajoute une transition douce */
  .filename-input { transition: all 200ms cubic-bezier(0.2, 0.8, 0.2, 1); }

  /* btn-arrow danger refined */
  .btn-arrow {
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
    background: rgba(255, 255, 255, 0.025);
  }

  /* ╔════════════════════════════════════════════════════════════╗
     ║  POST-SOURCE POLISH  v4.2.0                                ║
     ║  Refonte premium liquid glass de la vue source-loaded      ║
     ╚════════════════════════════════════════════════════════════╝ */

  /* ── 1. CARDS ─ hiérarchie visuelle (Pistes = primary, autres = secondary) ── */
  .col > .card {
    /* gradient top-border subtile pour différencier les cards */
    position: relative;
  }
  .col > .card::after {
    content: "";
    position: absolute;
    left: 14px; right: 14px; top: 0;
    height: 1px;
    background: linear-gradient(90deg, transparent, rgba(124, 92, 255, 0.32), transparent);
    pointer-events: none;
    opacity: 0.7;
  }

  /* Card primary: Pistes (card-grow) — accent plus marqué + glow violet */
  .card.card-grow {
    background:
      linear-gradient(180deg, rgba(124, 92, 255, 0.045) 0%, transparent 14%),
      var(--card);
    border-color: rgba(124, 92, 255, 0.18);
    box-shadow:
      0 12px 36px rgba(0, 0, 0, 0.4),
      0 0 0 1px rgba(124, 92, 255, 0.08),
      inset 0 1px 0 rgba(255, 255, 255, 0.06);
  }
  .card.card-grow::after {
    background: linear-gradient(90deg, transparent 5%, rgba(124, 92, 255, 0.55) 50%, transparent 95%);
    opacity: 1;
    height: 1.5px;
  }
  .card.card-grow .card-title {
    font-size: 11.5px;
    color: var(--text);
    font-weight: 700;
  }
  .card.card-grow .card-title::before {
    content: "";
    display: inline-block;
    width: 4px; height: 14px;
    background: linear-gradient(180deg, var(--accent), var(--red));
    border-radius: 2px;
    margin-right: 2px;
    box-shadow: 0 0 8px rgba(124, 92, 255, 0.5);
  }

  /* ── 2. TRACK ROWS ─ hiérarchie audio/video/sub ── */
  .card.card-grow .track {
    padding: 9px 12px;
    border-radius: 10px;
    margin-bottom: 5px;
    background: rgba(255, 255, 255, 0.025);
    border-left: 3px solid transparent;
    transition: all 200ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .card.card-grow .track:hover {
    background: rgba(255, 255, 255, 0.055);
    transform: translateX(2px);
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.25), inset 0 1px 0 rgba(255, 255, 255, 0.04);
  }
  /* Color-code by type via icon child */
  .track:has(.track-icon.video) { border-left-color: rgba(124, 92, 255, 0.55); }
  .track:has(.track-icon.audio) { border-left-color: rgba(255, 181, 71, 0.50); }
  .track:has(.track-icon.sub)   { border-left-color: rgba(255, 92, 179, 0.50); }
  .track:has(.track-icon.video):hover { box-shadow: 0 4px 14px rgba(124, 92, 255, 0.18), inset 0 1px 0 rgba(255, 255, 255, 0.05); }
  .track:has(.track-icon.audio):hover { box-shadow: 0 4px 14px rgba(255, 181, 71, 0.15), inset 0 1px 0 rgba(255, 255, 255, 0.05); }
  .track:has(.track-icon.sub):hover   { box-shadow: 0 4px 14px rgba(255, 92, 179, 0.18), inset 0 1px 0 rgba(255, 255, 255, 0.05); }

  /* Bigger track icons inside card-grow for premium look */
  .card.card-grow .track-icon {
    width: 26px; height: 26px;
    border-radius: 7px;
    font-size: 12px;
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.12),
      0 2px 6px rgba(0, 0, 0, 0.3);
  }
  .card.card-grow .track-icon.video {
    background: linear-gradient(135deg, rgba(124, 92, 255, 0.35), rgba(124, 92, 255, 0.18));
    color: #fff;
    text-shadow: 0 0 6px rgba(124, 92, 255, 0.6);
  }
  .card.card-grow .track-icon.audio {
    background: linear-gradient(135deg, rgba(255, 181, 71, 0.35), rgba(255, 181, 71, 0.18));
    color: #fff;
    text-shadow: 0 0 6px rgba(255, 181, 71, 0.6);
  }
  .card.card-grow .track-icon.sub {
    background: linear-gradient(135deg, rgba(255, 92, 179, 0.35), rgba(255, 92, 179, 0.18));
    color: #fff;
    text-shadow: 0 0 6px rgba(255, 92, 179, 0.6);
  }

  /* track-label : label en bold, meta en text2 (séparateurs · plus discrets) */
  .card.card-grow .track-label {
    font-size: 12px;
    font-weight: 600;
    letter-spacing: -0.1px;
  }

  /* Track flag : pill pleinement arrondie + glow */
  .card.card-grow .track-flag {
    border-radius: 999px;
    padding: 3px 10px;
    font-size: 9.5px;
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
  }

  /* track-controls : selects + checks plus glass */
  .track-controls .chk {
    padding: 3px 9px;
    border-radius: 999px;
    font-size: 10.5px;
    font-weight: 500;
    letter-spacing: 0.2px;
  }
  .track-controls .chk:has(input:checked) {
    background: rgba(124, 92, 255, 0.18);
    border-color: rgba(124, 92, 255, 0.4);
    color: var(--accent-hot);
    box-shadow: 0 0 10px rgba(124, 92, 255, 0.18), inset 0 1px 0 rgba(255, 255, 255, 0.06);
  }

  /* ── 3. CUSTOM SELECT ARROW (premium, partout) ── */
  .field select,
  .field-row select,
  .track-controls select {
    -webkit-appearance: none;
    -moz-appearance: none;
    appearance: none;
    background-image:
      linear-gradient(45deg, transparent 50%, var(--text2) 50%),
      linear-gradient(135deg, var(--text2) 50%, transparent 50%);
    background-position:
      calc(100% - 14px) 50%,
      calc(100% - 9px) 50%;
    background-size: 5px 5px, 5px 5px;
    background-repeat: no-repeat;
    padding-right: 26px;
    cursor: pointer;
  }
  .field select:hover,
  .field-row select:hover,
  .track-controls select:hover {
    background-image:
      linear-gradient(45deg, transparent 50%, var(--accent-hot) 50%),
      linear-gradient(135deg, var(--accent-hot) 50%, transparent 50%);
    background-position:
      calc(100% - 14px) 50%,
      calc(100% - 9px) 50%;
    background-size: 5px 5px, 5px 5px;
    background-repeat: no-repeat;
    border-color: var(--border-hi);
  }
  .field select:focus,
  .field-row select:focus,
  .track-controls select:focus {
    background-image:
      linear-gradient(45deg, transparent 50%, var(--accent) 50%),
      linear-gradient(135deg, var(--accent) 50%, transparent 50%);
  }
  .field select option, .field-row select option, .track-controls select option {
    background: #1a1622;
    color: var(--text);
  }

  /* ── 4. RÉGLAGES CARD ─ inputs & selects glass premium ── */
  .field-grid { gap: 10px 12px; }
  .field-label, .field label {
    font-size: 9.5px;
    letter-spacing: 0.7px;
    color: var(--text3);
    font-weight: 600;
  }
  .field input, .field select, .field-row input, .field-row select {
    background: rgba(255, 255, 255, 0.025);
    border: 1px solid rgba(255, 255, 255, 0.07);
    border-radius: 8px;
    padding: 7px 10px;
    font-size: 12px;
    transition: all 180ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .field input:hover, .field select:hover,
  .field-row input:hover, .field-row select:hover {
    background: rgba(255, 255, 255, 0.05);
    border-color: rgba(255, 255, 255, 0.13);
  }
  .field input:focus, .field select:focus,
  .field-row input:focus, .field-row select:focus {
    background: rgba(124, 92, 255, 0.06);
    border-color: var(--accent-soft);
    box-shadow:
      0 0 0 3px rgba(124, 92, 255, 0.18),
      0 0 14px rgba(124, 92, 255, 0.10);
  }
  .field input[readonly] {
    background: rgba(255, 255, 255, 0.015);
    color: var(--text2);
    font-style: italic;
  }
  .field input[readonly]:hover {
    background: rgba(255, 255, 255, 0.02);
    border-color: rgba(255, 255, 255, 0.07);
  }

  /* ── 5. FILENAME ─ premium gradient pill avec accent extension ── */
  .filename {
    background:
      radial-gradient(ellipse at top left, rgba(124, 92, 255, 0.18), transparent 60%),
      linear-gradient(135deg, rgba(124, 92, 255, 0.10), rgba(255, 61, 94, 0.06));
    border: 1px solid rgba(124, 92, 255, 0.30);
    border-radius: 11px;
    padding: 13px 16px;
    font-size: 12.5px;
    line-height: 1.6;
    color: var(--text);
    font-weight: 500;
    letter-spacing: -0.1px;
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.06),
      0 4px 16px rgba(124, 92, 255, 0.08),
      0 0 22px rgba(124, 92, 255, 0.08);
    transition: all 220ms cubic-bezier(0.2, 0.8, 0.2, 1);
    position: relative;
    overflow: hidden;
  }
  .filename::before {
    content: "";
    position: absolute;
    inset: 0;
    background: linear-gradient(120deg, transparent 30%, rgba(255, 255, 255, 0.04) 50%, transparent 70%);
    pointer-events: none;
  }
  .filename:hover {
    border-color: rgba(124, 92, 255, 0.55);
    transform: translateY(-1px);
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.10),
      0 8px 26px rgba(124, 92, 255, 0.20),
      0 0 32px rgba(124, 92, 255, 0.15);
  }

  /* ── 6. TMDB POSTER ─ shadow profond + bord accent + lift ── */
  .tmdb-preview-poster {
    width: 84px; height: 126px;
    border-radius: 10px;
    border: 1px solid rgba(124, 92, 255, 0.35);
    box-shadow:
      0 10px 28px rgba(0, 0, 0, 0.55),
      0 0 0 1px rgba(255, 255, 255, 0.04),
      0 0 22px rgba(124, 92, 255, 0.18);
    transition: all 240ms cubic-bezier(0.2, 0.8, 0.2, 1);
    cursor: pointer;
  }
  .tmdb-preview-poster:hover {
    transform: translateY(-2px) scale(1.02);
    border-color: rgba(124, 92, 255, 0.6);
    box-shadow:
      0 14px 36px rgba(0, 0, 0, 0.6),
      0 0 0 1px rgba(255, 255, 255, 0.08),
      0 0 32px rgba(124, 92, 255, 0.30);
  }

  .tmdb-preview-title {
    font-size: 14px;
    background: linear-gradient(135deg, #fff 0%, #c8b5ff 100%);
    -webkit-background-clip: text; background-clip: text;
    -webkit-text-fill-color: transparent;
    letter-spacing: -0.2px;
  }

  /* ── 7. OUTPUT DIR & TOOLS ─ cards plus discrètes ── */
  /* Output card : fond plus subtil pour qu'elle ne dispute pas Pistes */
  .col > .card:has(> .card-title:first-child:has(+ .track:only-child)) {
    background: rgba(20, 17, 27, 0.45);
    padding: 10px 12px;
  }
  /* Output dir track : icône bleue glow */
  .track .track-icon[style*="background:rgba(94,197,255"] {
    background: linear-gradient(135deg, rgba(94, 197, 255, 0.35), rgba(94, 197, 255, 0.15)) !important;
    color: #fff !important;
    text-shadow: 0 0 6px rgba(94, 197, 255, 0.6);
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.12), 0 2px 6px rgba(0, 0, 0, 0.3);
  }

  /* Tools row : boutons plus mis en avant */
  .tools-row { gap: 8px; }
  .tools-row .btn {
    padding: 7px 12px;
    font-size: 11.5px;
    background: rgba(255, 255, 255, 0.03);
    border: 1px solid rgba(255, 255, 255, 0.07);
    border-radius: 8px;
    color: var(--text);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
  }
  .tools-row .btn:hover:not(:disabled) {
    background: rgba(124, 92, 255, 0.10);
    border-color: rgba(124, 92, 255, 0.35);
    color: var(--accent-hot);
    transform: translateY(-1px);
    box-shadow: 0 4px 14px rgba(124, 92, 255, 0.18);
  }

  /* ── 8. VALIDATION PILLS ─ taille + glow ── */
  .pill {
    padding: 5px 12px;
    font-size: 10.5px;
    letter-spacing: 0.4px;
  }
  .pill.ok    { box-shadow: 0 0 16px rgba(92, 201, 153, 0.18), inset 0 1px 0 rgba(255, 255, 255, 0.05); }
  .pill.warn  { box-shadow: 0 0 16px rgba(255, 181, 71, 0.18), inset 0 1px 0 rgba(255, 255, 255, 0.05); }
  .pill.err   { box-shadow: 0 0 16px rgba(255, 61, 94, 0.18), inset 0 1px 0 rgba(255, 255, 255, 0.05); }

  /* ── 9. CARD-TITLE ICON glow subtil ── */
  .card-title { gap: 8px; }
  .card-title-row .card-title-actions .btn {
    background: rgba(124, 92, 255, 0.10);
    border: 1px solid rgba(124, 92, 255, 0.25);
    color: var(--accent-hot);
    padding: 4px 11px;
    border-radius: 999px;
    font-size: 10.5px;
    font-weight: 600;
  }
  .card-title-row .card-title-actions .btn:hover:not(:disabled) {
    background: rgba(124, 92, 255, 0.20);
    border-color: rgba(124, 92, 255, 0.45);
    color: #fff;
    box-shadow: 0 0 14px rgba(124, 92, 255, 0.30);
  }

  /* ── 10. SCROLLBAR interne du card-grow plus discrète ── */
  .card-grow::-webkit-scrollbar { width: 6px; }
  .card-grow::-webkit-scrollbar-thumb {
    background: rgba(124, 92, 255, 0.22);
    border-radius: 3px;
    border: none;
  }
  .card-grow::-webkit-scrollbar-thumb:hover {
    background: rgba(124, 92, 255, 0.5);
  }

  /* ── 11. TMDB PREVIEW INFO ─ meta plus structurée ── */
  .tmdb-preview-meta span {
    background: rgba(255, 255, 255, 0.04);
    padding: 2px 8px;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.06);
    font-size: 10.5px;
    font-weight: 500;
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
  }

  /* ── 12. TMDB OVERRIDE ROW ─ glass subtil ── */
  .tmdb-preview-override {
    background: rgba(0, 0, 0, 0.18);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1px dashed rgba(255, 255, 255, 0.08);
    border-top: 1px dashed rgba(255, 255, 255, 0.08);
    border-radius: 8px;
    padding: 8px 10px;
    margin-top: 10px;
    padding-top: 8px;
  }

  /* ── 13. FILENAME ACTIONS ─ boutons un peu plus visibles ── */
  .filename-actions .btn-tiny {
    background: rgba(255, 255, 255, 0.035);
    border: 1px solid rgba(255, 255, 255, 0.08);
    color: var(--text2);
    padding: 5px 11px;
    border-radius: 999px;
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
  }
  .filename-actions .btn-tiny:hover:not(:disabled) {
    background: rgba(124, 92, 255, 0.14);
    border-color: rgba(124, 92, 255, 0.32);
    color: var(--accent-hot);
    transform: translateY(-1px);
  }

  /* ── 14. LANG TOGGLE ─ déjà bon mais petit refinement ── */
  .lang-toggle {
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04), 0 2px 8px rgba(0, 0, 0, 0.18);
  }

  /* ── 15. EQUILIBRAGE COLONNES ─ donner du minimum à la colonne droite ── */
  @media (min-width: 1101px) {
    .main { grid-template-columns: minmax(0, 1.05fr) minmax(0, 0.95fr); }
  }

  /* ── 16. Header card-titles avec emoji glow ── */
  .card .card-title {
    color: var(--text);
    font-weight: 700;
    font-size: 11px;
  }

  /* ── 17. Mux-status-banner taille augmentée ── */
  .mux-status-banner {
    padding: 14px 18px;
  }
  .mux-status-banner.success { animation: success-glow 2.4s ease-in-out infinite; }
  @keyframes success-glow {
    0%, 100% { box-shadow: 0 0 22px rgba(92, 201, 153, 0.18), var(--shadow-card); }
    50%      { box-shadow: 0 0 38px rgba(92, 201, 153, 0.32), var(--shadow-card); }
  }

  /* ── 18. btn-arrow plus glass et alignés ── */
  .btn-arrow {
    padding: 3px 7px;
    border-radius: 6px;
    font-size: 11px;
    background: rgba(255, 255, 255, 0.04);
    border-color: rgba(255, 255, 255, 0.06);
  }
  .btn-arrow:hover {
    background: rgba(124, 92, 255, 0.14);
    border-color: rgba(124, 92, 255, 0.30);
    color: var(--accent-hot);
  }
  .btn-arrow.danger:hover {
    background: rgba(255, 61, 94, 0.12);
    border-color: rgba(255, 61, 94, 0.35);
    color: var(--red-hot);
  }

  /* Modal LanguageTool review */
  .lt-review-overlay {
    position: fixed; inset: 0;
    background: rgba(0, 0, 0, 0.55);
    display: flex; align-items: center; justify-content: center;
    z-index: 1000;
  }
  .lt-review-modal {
    background: var(--bg, #15151b);
    border: 1px solid rgba(124, 92, 255, 0.30);
    border-radius: 10px;
    width: min(640px, 92vw);
    max-height: 80vh;
    display: flex; flex-direction: column;
    box-shadow: 0 30px 60px rgba(0, 0, 0, 0.5);
  }
  .lt-review-header {
    display: flex; align-items: center; justify-content: space-between;
    padding: 14px 18px;
    border-bottom: 1px solid rgba(255,255,255,0.08);
  }
  .lt-review-header h3 { margin: 0; font-size: 15px; }
  .lt-review-body {
    padding: 14px 18px;
    overflow-y: auto;
    flex: 1;
  }
  .lt-review-item {
    padding: 10px 0;
    border-bottom: 1px dashed rgba(255,255,255,0.08);
  }
  .lt-review-item:last-child { border-bottom: none; }
  .lt-review-line { font-size: 12px; opacity: 0.85; }
  .lt-review-snippet {
    background: rgba(255,255,255,0.04);
    padding: 6px 10px;
    border-radius: 6px;
    margin: 4px 0;
    font-size: 13px;
  }
  .lt-review-msg { font-size: 12px; opacity: 0.85; margin-bottom: 4px; }
  .lt-review-sugg { font-size: 12px; }
  .lt-sugg-pill {
    display: inline-block;
    background: rgba(124, 92, 255, 0.15);
    border: 1px solid rgba(124, 92, 255, 0.30);
    border-radius: 4px;
    padding: 2px 8px;
    margin: 0 4px 4px 0;
    font-family: monospace;
    font-size: 12px;
  }
  .lt-sugg-pill.clickable {
    cursor: pointer;
    transition: background 120ms, border-color 120ms;
  }
  .lt-sugg-pill.clickable:hover:not(:disabled) {
    background: rgba(124, 92, 255, 0.30);
    border-color: rgba(124, 92, 255, 0.60);
  }
  .lt-sugg-pill.clickable:disabled { opacity: 0.5; cursor: not-allowed; }
  .lt-review-footer {
    padding: 12px 18px;
    border-top: 1px solid rgba(255,255,255,0.08);
    text-align: right;
  }
  .lt-review-item.resolved {
    opacity: 0.55;
    background: rgba(92, 201, 153, 0.05);
  }
  .lt-resolved-pill {
    display: inline-block;
    margin-left: 8px;
    padding: 1px 8px;
    border-radius: 4px;
    background: rgba(92, 201, 153, 0.18);
    border: 1px solid rgba(92, 201, 153, 0.45);
    color: var(--green);
    font-size: 11px;
  }
  .lt-review-custom {
    display: flex; gap: 6px; align-items: center;
    margin-top: 6px;
  }
  .lt-custom-input {
    flex: 1;
    background: rgba(255,255,255,0.04);
    border: 1px solid rgba(255,255,255,0.10);
    border-radius: 4px;
    color: var(--text);
    padding: 4px 8px;
    font-size: 12px;
    font-family: monospace;
  }
  .lt-review-err {
    color: var(--red, #ff3d5e);
    font-size: 11px;
    margin-top: 4px;
  }

  /* OpenSubtitles modal */
  .os-search-row {
    display: flex; gap: 8px; align-items: flex-end;
    margin-bottom: 10px;
  }
  .os-results-list {
    margin-top: 10px; max-height: 340px; overflow-y: auto;
  }
  .os-result-row {
    padding: 8px 10px;
    border: 1px solid rgba(255,255,255,0.06);
    border-radius: 6px;
    margin-bottom: 6px;
    background: rgba(255,255,255,0.02);
  }
  .os-result-title { font-size: 13px; margin-bottom: 2px; }
  .os-result-filename { font-size: 11px; opacity: 0.7; margin-bottom: 6px; word-break: break-all; }
  .os-result-actions { display: flex; gap: 6px; justify-content: flex-end; }
  .os-pill {
    display: inline-block;
    margin-left: 6px;
    padding: 1px 6px;
    border-radius: 4px;
    background: rgba(255,255,255,0.06);
    border: 1px solid rgba(255,255,255,0.10);
    font-size: 10px;
  }

  /* Custom dict OCR (Settings) */
  .custom-dict-list {
    margin-top: 6px;
    max-height: 280px;
    overflow-y: auto;
    border: 1px solid rgba(255,255,255,0.06);
    border-radius: 6px;
    padding: 6px;
    background: rgba(0,0,0,0.15);
  }
  .custom-dict-row {
    display: flex; align-items: center; gap: 8px;
    padding: 4px 6px;
    border-bottom: 1px dashed rgba(255,255,255,0.05);
    font-size: 12px;
  }
  .custom-dict-row:last-child { border-bottom: none; }
  .custom-dict-wrong { color: var(--orange); }
  .custom-dict-arrow { opacity: 0.5; }
  .custom-dict-right { color: var(--green); flex: 1; }
  .custom-dict-auto {
    font-size: 10px;
    padding: 1px 6px;
    border-radius: 3px;
    background: rgba(124, 92, 255, 0.15);
    border: 1px solid rgba(124, 92, 255, 0.30);
    color: var(--accent-hot);
  }
</style>
