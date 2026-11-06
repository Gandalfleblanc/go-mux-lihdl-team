<script>
  import { run, createBubbler, stopPropagation } from 'svelte/legacy';

  const bubble = createBubbler();
  import { onMount, tick } from 'svelte';
  import JournalPane from './lib/JournalPane.svelte';
  import OutilsCard from './lib/OutilsCard.svelte';
  import MuxActionBar from './lib/MuxActionBar.svelte';
  import FilenameCard from './lib/FilenameCard.svelte';
  import VideoSettingsCard from './lib/VideoSettingsCard.svelte';
  import TmdbValidateCard from './lib/TmdbValidateCard.svelte';
  import TrackList from './lib/TrackList.svelte';
  import SourcesCard from './lib/SourcesCard.svelte';
  import HeaderFilmBar from './lib/HeaderFilmBar.svelte';
  import EmptyHero from './lib/EmptyHero.svelte';
  import Icon from './lib/Icon.svelte';
  import banner from './assets/images/banner.png';
  import logo from './assets/images/logo.png';
  import {
    GetVersion, GetConfig, SaveConfig, GetLihdlOptions,
    SelectMkvFile, SelectMkvFiles, SelectSubFiles, SelectSupFiles, SelectAudioFiles, SelectOutputDir, LocateMkvmerge, OpenFolder, SearchTmdbTV, SearchTmdbMovie, AnalyzeMkvSecondary, MoveToTrash, MoveDirContentsToTrash, LookupHydrackerURL, TestHydrackerKey, TestUnfrKey, OpenURL, GetMkvBasicInfo, ExtractRefSubs, ExtractFRAudios, CheckSubsSync, CheckPSASync, ExtractFirstAudioFromMkv, SyncSupplySubsToPSA,
    AnalyzeMkv, SearchTmdb, TestTmdbKey, FileSize, DetectSubSDH,
    Mux, CancelMux,
    CheckUpdate, InstallUpdate,
    ListAudioTracksForSync, MuxAudioSync, DetectAudioOffset,
    OCRPGSTrack, OCRSupFile, GenerateFRSrtFromPGS,
    TestLanguageToolKey,
    ApplyOCRFix,
    SearchOpenSubtitles, DownloadOpenSubtitle,
    OCRCustomDictList, OCRCustomDictAdd, OCRCustomDictRemove,
    DiscordIndexScan, DiscordIndexRead, DiscordIndexLookup, DiscordIndexRefreshRemote, DiscordIndexPushGitHub,
  } from '../wailsjs/go/main/App.js';
  import { EventsOn } from '../wailsjs/runtime/runtime.js';

  let screen = $state('source');
  let muxMode = $state('lihdl');   // 'lihdl' | 'psa'

  function switchMuxMode(mode) {
    if (muxMode === mode) return;
    muxMode = mode;
    if (mode === 'psa') {
      // Défauts mode PSA : série WEBRip Team GANDALF
      videoChoice.team = 'ASTROBOY';
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
          videoChoice.encoder = config.default_encoder || 'ASTROBOY';
          // Team : films PSA → -LiHDL ; séries PSA → -GANDALF (norme).
          videoChoice.team = /(?:^|[\s._-])S\d{1,2}E\d{1,3}(?=[\s._-]|$)/i.test(psaName) ? 'ASTROBOY' : 'LiHDL';
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
          appendLog(`✓ PSA re-détecté au switch : ${psa.source || '?'} ${target.video_codec || '?'} → Custom PSA / ASTROBOY`);
        }
        // Auto-décoche audios + subs non-FR/non-ENG sur les tracks déjà chargés.
        if (tracks.length > 0) {
          let droppedAud = 0, droppedSub = 0;
          tracks = tracks.map(t => {
            if (t.type === 'audio') {
              if (t.keep) droppedAud++;
              return { ...t, keep: false };
            }
            if (t.type === 'subtitles') {
              const isFR = /^fr/i.test(t.lang || '') || /^FR /.test(t.label || '');
              const isENG = /^en/i.test(t.lang || '') || /^ENG /.test(t.label || '');
              const keepIt = isFR || isENG;
              if (t.keep && !keepIt) droppedSub++;
              return { ...t, keep: keepIt };
            }
            return t;
          });
          if (droppedAud || droppedSub) {
            appendLog(`🎬 Mode PSA : ${droppedAud} audio(s) PSA décoché(s), ${droppedSub} sub(s) non-FR/ENG décoché(s)`);
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
  let appVersion = $state('');
  let mkvmergePath = '';

  let config = $state({
    tmdb_key: '',
    serveurperso_url: 'https://www.serveurperso.com/stats/search.php',
    output_dir: '',
    mkvmerge_path: '',
    default_encoder: 'ASTROBOY',
    custom_audio_labels: [],
    custom_subtitle_labels: [],
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
  });

  // Index Discord — état UI :
  // discordIndexEntry  = URL Discord du film actuel ("" si pas trouvé)
  // discordScanRunning = scan admin en cours
  // discordScanProgress = { scanned, total, message }
  let discordIndexEntry = $state('');
  let discordScanRunning = $state(false);
  let discordScanProgress = $state({ scanned: 0, total: 0, message: '' });
  let discordCopyOk = $state(false);
  let githubPushing = $state(false);
  let githubPushOk = $state(false);

  let options = $state({
    audio_labels: [], subtitle_labels: [],
    video_qualities: [], video_encoders: [],
    video_sources: [], video_teams: [],
    video_source_types: [], resolutions: [], target_sources: [], video_codecs: [],
  });

  let sourcePath = $state('');
  let sourceInfo = null;   // réponse AnalyzeMkv
  let tracks = $state([]);         // état UI enrichi de chaque piste

  let videoChoice = $state({
    quality: 'HDLight',
    encoder: 'ASTROBOY',
    sourceType: 'COMPLETE BluRay', // ex-source split
    sourceTeam: '',                 // team de la source (ex: Alkaline) — éditable
    team: 'LiHDL',                  // team de sortie (dans le filename)
  });
  let target = $state({
    title: '', year: '', resolution: '1080p', source: 'HDLight',
    video_codec: '', lang: 'auto',
    episode: '',          // SxxExx auto-détecté (vide = film)
    flagOverride: 'auto', // override du langflag : auto | VFF | VFQ | VFi | VO | MULTi.VFF | MULTi.VFQ | MULTi.VF2 | MULTi.VFi | FASTSUB.VOSTFR | FASTSUB.FR
  });
  const FLAG_OVERRIDE_OPTIONS = [
    'auto',
    'VFF', 'VFQ', 'VFi', 'VO',
    'MULTi.VFF', 'MULTi.VFQ', 'MULTi.VF2', 'MULTi.VFi',
    'FASTSUB.VOSTFR', 'FASTSUB.FR',
  ];

  function detectEpisode(filename) {
    // Accepte séparateurs . _ - espace autour de SxxExx (ex: Dallas_S03E01_).
    const m = /(?:^|[\s._-])S(\d{1,2})E(\d{1,3})(?=[\s._-]|$)/i.exec(String(filename || ''));
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
    // Accepte "PSA" exact ou tout team contenant "PSA" (ex: PSAarcane, PSA10bit).
    const isPSA = team === 'PSA' || /PSA/i.test(team);
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
  let lastTmdbResult = $state(null);
  // Flag de validation TMDB : tant qu'il est false ET qu'on a une fiche TMDB,
  // on bloque l'affichage des pistes/réglages pour forcer une confirmation visuelle.
  let tmdbValidated = $state(false);


  // Dropdowns pour le filename (ordre : résolution.source).
  const RESOLUTION_OPTIONS = ['720p', '1080p', '2160p'];
  const TARGET_SOURCE_OPTIONS = ['HDLight', 'WEBLight', 'WEB-DL', 'WEBRip', 'REMUX'];
  // Dropdown élargi pour le type de source de la piste vidéo.
  const VIDEO_SOURCE_TYPE_OPTIONS = [
    'REMUX', 'REMUX CUSTOM',
    'WEB-DL CUSTOM', 'WEB CUSTOM',
    'WEB', 'WEB-DL', 'WEBRip',
    'WEBRip PSA Audio SUPPLY', 'WEBRIP PSA Audio FW', 'WEBRip PSA Audio Super U',
    'BluRay', 'COMPLETE BluRay',
  ];
  const VIDEO_CODEC_OPTIONS = ['H264', 'x264', 'H265', 'x265', 'AV1'];
  let tmdbResults = $state([]);
  let tmdbResultIndex = $state(0); // index du résultat actuel dans tmdbResults (cycle via "Autre résultat")

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
  let tmdbQuery = $state('');
  let tmdbIdQuery = $state(''); // recherche par ID TMDB (séparée du nom)
  let tmdbMode = $state('movie'); // 'movie' | 'tv'
  // Toggle VFi par défaut ON : les pistes audio FR sont labellisées FR VFi.
  // Si l'utilisateur décoche → FR VFF.
  let useVFi = $state(true);
  // Règle universelle pour les subs : un label "FR Forced" ou "FR VFF Forced"
  // a default=true ET forced=true. "FR VFQ Forced" devient default+forced
  // UNIQUEMENT si aucun FR Forced non-VFQ n'est présent (cas releases qui
  // n'ont qu'une piste VFQ sans VFF).
  function isForcedFRLabel(label) {
    return /^FR( VFF)? Forced\b/.test(label || '');
  }
  function isForcedFRVFQLabel(label) {
    return /^FR VFQ Forced\b/.test(label || '');
  }
  // Applique la règle Forced à tous les subs (internes + externes + secondaires).
  // À appeler après tout changement de label sub.
  function applySubForcedRule() {
    // Mode UNFR/FASTSUB.VOSTFR : pas de doublage FR, le sub FR Full sert de
    // doublage textuel. Doit être marqué default=true (pas forced — c'est
    // un sub Full visible en permanence, pas un Forced ponctuel).
    const langFlag = String(target.flagOverride || '').toUpperCase();
    const supplyName = String(secondaryPath || '').toUpperCase();
    const isUnfrFastsub = /FASTSUB|VOSTFR/.test(langFlag) || /UNFR/.test(supplyName);

    // 1ère passe : détecte si un FR Forced non-VFQ existe parmi tous les subs.
    const allSubs = [
      ...tracks.filter(t => t.type === 'subtitles'),
      ...externalSubs,
      ...secondarySelected.filter(t => t.type === 'subtitles'),
    ];
    const hasFRForcedNonVFQ = allSubs.some(s => isForcedFRLabel(s.label));
    // Premier sub FR (peu importe variante/type) en mode UNFR/FASTSUB : on
    // le mémorise pour le marquer default=true, forced=false.
    const isFRSub = (label) => /^FR /.test(String(label || ''));
    let unfrDefaultPath = null;
    if (isUnfrFastsub) {
      // Préfère un sub FR externe synchronisé en priorité
      const ext = externalSubs.find(s => s.fromReference && isFRSub(s.label));
      if (ext) unfrDefaultPath = 'external:' + ext.path;
      else {
        const sec = secondarySelected.find(t => t.type === 'subtitles' && t.keep && isFRSub(t.label));
        if (sec) unfrDefaultPath = 'secondary:' + sec.id;
        else {
          const psa = tracks.find(t => t.type === 'subtitles' && t.keep && isFRSub(t.label));
          if (psa) unfrDefaultPath = 'tracks:' + psa.id;
        }
      }
    }

    // Détermine (default, forced) pour un sub donné :
    //   - FR/VFF Forced → default+forced toujours
    //   - FR VFQ Forced → default+forced uniquement si pas de FR Forced non-VFQ ailleurs
    //   - Mode UNFR/FASTSUB sub FR principal → default=true forced=false
    //   - Sinon → false/false
    const computeFlags = (label, sourceKey) => {
      if (isForcedFRLabel(label)) return { default: true, forced: true };
      if (!hasFRForcedNonVFQ && isForcedFRVFQLabel(label)) return { default: true, forced: true };
      if (isUnfrFastsub && unfrDefaultPath && unfrDefaultPath === sourceKey) {
        return { default: true, forced: false };
      }
      return { default: false, forced: false };
    };
    tracks = tracks.map(t => {
      if (t.type !== 'subtitles') return t;
      const flags = computeFlags(t.label, 'tracks:' + t.id);
      return { ...t, default: flags.default, forced: flags.forced };
    });
    externalSubs = externalSubs.map(s => {
      const flags = computeFlags(s.label, 'external:' + s.path);
      return { ...s, default: flags.default, forced: flags.forced };
    });
    secondarySelected = secondarySelected.map(t => {
      if (t.type !== 'subtitles') return t;
      const flags = computeFlags(t.label, 'secondary:' + t.id);
      return { ...t, default: flags.default, forced: flags.forced };
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

    // Reclassification en FR AD : UNIQUEMENT si vrais marqueurs (flag Matroska
    // visual_impaired OU track_name explicite AD/audiodescription/descriptive).
    // L'ancienne heuristique "2 pistes FR + une 2.0" faisait des faux positifs
    // (ex: VFF 5.1 + VFQ 2.0 = cas normal, PAS une AD).
    const toADIfMarked = (t, lbl) => {
      if (!looksLikeAD(t)) return lbl;
      if (!/^FR (VFF|VFQ|VFi|VOF) /.test(lbl || '')) return lbl;
      return lbl.replace(/^FR (VFF|VFQ|VFi|VOF) /, 'FR AD ');
    };
    tracks = tracks.map(t => t.type === 'audio' ? { ...t, label: toADIfMarked(t, t.label || '') } : t);
    secondarySelected = secondarySelected.map(t => t.type === 'audio' ? { ...t, label: toADIfMarked(t, t.label || '') } : t);
    if (tracks.some(t => /^FR AD /.test(t.label || '')) ||
        secondarySelected.some(t => /^FR AD /.test(t.label || ''))) {
      appendLog('🦮 Piste FR AD détectée via flag/track_name explicite');
    }
  }

  // Détecte une piste AD (audiodescription) via marqueurs EXPLICITES uniquement :
  //   1. flag Matroska flag_visual_impaired = true
  //   2. track_name contient AD / audio desc / audiodescription / audiodescrit / descriptive
  // Sans marqueur → PAS une AD (évite les faux positifs VFF 5.1 + VFQ 2.0).
  function looksLikeAD(t) {
    if (!t) return false;
    if (t.flag_visual_impaired === true || t.visualImpaired === true) return true;
    const name = String(t.track_name || t.trackName || '').toLowerCase();
    if (!name) return false;
    if (/\baudiodescri/.test(name)) return true;
    if (/\bdescriptive\b/.test(name)) return true;
    if (/\baudio\s*desc/.test(name)) return true;
    if (/\bad\b/.test(name) && /(audiodesc|malvoyant|visual|descriptive|décrit|decrit)/.test(name)) return true;
    return false;
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



  // URL fiche Hydracker (résolue via API à partir de l'ID TMDB ; vide si pas de clé)
  let hydrackerURL = $state('');

  // Source de référence (3ᵉ barre, optionnelle) : pour vérifier compatibilité
  // durée + FPS avant de récupérer les sous-titres dessus.
  let referencePath = $state('');
  let sourceMkvInfo = $state(null);     // { duration_seconds, framerate, width, height }
  let referenceMkvInfo = $state(null);
  let showReferenceBar = $state(false);

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
        // Tri par priorité LiHDL : FR Forced (0) → FR Full (10) → FR SDH (20)
        // → ENG Forced → ENG Full → ENG SDH. Sans ça, les orders incrémentaux
        // suivraient l'ordre d'extraction (ENG en 1er si backend le renvoie ainsi).
        filteredSubs.sort((a, b) => subLabelPriority(a.label) - subLabelPriority(b.label));
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
  let extractFRVFF = $state(false);
  // Toggle "inclure l'année avant l'épisode" pour les séries où le nom seul
  // est ambigu (ex: remake "The Office 2024" vs original "The Office 2005").
  // Format avec : Title.YYYY.SxxExx... ; sans : Title.SxxExx...
  let includeYearInEpisode = $state(false);
  // Flag extra détecté dans le nom SUPPLY (ex: "FiNAL"). Inséré entre
  // SxxExx et le flag langue dans le filename de sortie.
  let supplyExtraFlag = $state('');
  // Référence audio pour la sync (Chromaprint + cross-corr) : "fr" (default,
  // matche la VFF/VFi de la source LiHDL) ou "eng" (pour les cas où la VFF
  // de la source n'est pas fiable, ex: source LiHDL re-encodée avec un drift
  // sur l'audio FR mais ENG VO préservée).
  let syncRefLang = $state('fr');
  let extractFRVFQ = $state(false);
  let extractENG = $state(false);
  let frAudioExtracting = $state(false);
  let frAudioExtractionResult = $state(''); // '' | 'success' | 'error'
  let frAudioPhase = $state(''); // 'sync' | 'convert:CODEC,VARIANT' — phase courante pour le label de la progress bar
  let frAudioConvertPercent = $state(0); // 0-100 pendant la conversion AC3 (event ac3convert:progress)
  let frAudioConvertedSummary = $state(''); // "448 kb/s 5.1" ou "192 kb/s 2.0" ou "448 kb/s 5.1 / 192 kb/s 2.0" selon les conversions réelles
  let srtExtracting = $state(false); // true pendant l'extraction SRT auto au pick d'une référence
  let srtExtractionResult = $state(''); // '' | 'success' | 'error'
  let srtPhase = $state(''); // '' | 'convert_ass' — phase courante pour le label de la SRT progress bar
  let srtAssConverted = $state(false); // true si au moins un sub ASS a été converti en SRT pendant cette extraction
  let srtPercent = $state(0); // 0-100 progression de l'extraction SRT (event srtprogress)

  // ---- Vérification sync subs externes vs audio source LiHDL ----
  let subSyncChecking = $state(false);
  let subSyncResults = $state([]); // [{path, offset_ms, raw_offset_ms, confidence, method, error}]
  let subSyncPercent = $state(0);
  let subSyncCurrentName = $state('');
  let subSyncAppliedMsg = $state(''); // "✓ N SRT resyncronisé(s)" persistant après apply

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
        // ATMOS par-piste : JOC dans Format_AdditionalFeatures ou ATMOS
        // explicite dans track_name. Pas Format_Commercial (propagé).
        const exFeatures = String(ex.mi_format_features || '').toUpperCase();
        const exName = String(ex.track_name || '').toUpperCase();
        const isAtmos = (exFeatures.includes('JOC') || exName.includes('ATMOS')) && ex.codec === 'EAC3' && ch === '5.1';
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
    lihdlAutomatedOnce = false;
    seriesLabelOverrides = {};
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
    if (muxMode === 'psa') {
      // PSA : on trash UNIQUEMENT les fichiers de l'épisode muxé (PSA +
      // SUPPLY/FW + MKV synchro) pour ne pas toucher aux autres épisodes
      // potentiellement encore en attente dans "PSA en cours".
      const psaFiles = [sourcePath, secondaryPath, psaSyncedSourceMkv].filter(Boolean);
      if (psaFiles.length > 0) {
        try {
          await MoveToTrash(psaFiles);
          appendLog(`🗑 ${psaFiles.length} fichier(s) source(s) envoyé(s) à la corbeille`);
        } catch (e) {
          appendLog('⚠ corbeille fichiers PSA : ' + String(e));
        }
      }
    } else {
      // LiHDL : par défaut, vide tout le contenu du dossier "LiHDL en cours"
      // (norme : tous les fichiers source/référence/SUPPLY/subs externes y vivent).
      // MAIS en auto-continue de queue avec des épisodes restants, on ne
      // trash QUE les fichiers de l'épisode courant sinon les épisodes
      // suivants (aussi dans "LiHDL en cours") partiraient à la corbeille.
      if (queueAutoContinue && queue.length > 0) {
        const filesForThisEp = [
          sourcePath,
          referencePath,
          ...externalSubs.map(s => s.path).filter(Boolean),
          ...externalAudios.map(a => a.path).filter(Boolean),
        ].filter(Boolean);
        if (filesForThisEp.length > 0) {
          try {
            await MoveToTrash(filesForThisEp);
            appendLog(`🗑 ${filesForThisEp.length} fichier(s) de l'épisode courant envoyé(s) à la corbeille (queue préservée)`);
          } catch (e) {
            appendLog('⚠ corbeille fichiers épisode : ' + String(e));
          }
        }
      } else {
        const lihdlEnCoursDir = '/Users/gandalf/Downloads/LiHDL en cours';
        try {
          const n = await MoveDirContentsToTrash(lihdlEnCoursDir);
          if (n > 0) {
            appendLog(`🗑 ${n} élément(s) du dossier "LiHDL en cours" envoyé(s) à la corbeille`);
          }
        } catch (e) {
          appendLog('⚠ vidage "LiHDL en cours" : ' + String(e));
        }
      }
    }
    // Reset UI désactivé après mux : on garde l'état + le log visibles pour
    // que l'utilisateur puisse copier les logs et inspecter le résultat.
    // Il cliquera ↻ RESET manuellement quand il aura fini.
    appendLog('ℹ État conservé après mux — clique ↻ RESET en haut quand tu veux continuer');
    void preserveAutoMuxStatus; // arg gardé pour compat caller
  }
  let filenameCopied = $state(false);
  let filenameCopiedTimer = null;
  let filenameOverride = $state(false);   // true = l'utilisateur a pris la main sur le nom
  let manualFilename = $state('');        // valeur saisie manuellement quand override actif


  function startFilenameOverride() {
    manualFilename = previewFilename;
    filenameOverride = true;
  }

  function resetFilenameOverride() {
    filenameOverride = false;
    manualFilename = '';
  }

  // Queue batch : liste de .mkv en attente de mux.
  let queue = $state([]);
  let bottomPaneTab = $state('journal'); // 'journal' | 'queue' — tab actif dans la card secondaire en bas droite
  // Auto-continue : après un mux OK, charge le prochain de la queue et relance
  // MUX AUTO automatiquement (chain complet, épisode après épisode).
  let queueAutoContinue = $state(false);
  // Overrides série : snapshot des labels/keep/default/forced du dernier mux
  // réussi, indexés par ${type}-${lang}-${nthOfSameLang}. Rejoués sur les
  // épisodes suivants pour propager les corrections manuelles de l'user
  // (ex: FR VFi → FR AD sur ép1 s'applique à ép2, ép3, …).
  let seriesLabelOverrides = {};
  function snapshotSeriesOverrides() {
    const langCounters = { audio: {}, subtitles: {} };
    const snap = {};
    for (const t of tracks) {
      if (t.type !== 'audio' && t.type !== 'subtitles') continue;
      const lang = (t.lang || '').toLowerCase();
      langCounters[t.type][lang] = (langCounters[t.type][lang] || 0) + 1;
      const n = langCounters[t.type][lang];
      snap[`${t.type}-${lang}-${n}`] = {
        label: t.label,
        keep: t.keep !== false,
        default: !!t.default,
        forced: !!t.forced,
      };
    }
    seriesLabelOverrides = snap;
  }
  function applySeriesOverrides() {
    if (Object.keys(seriesLabelOverrides).length === 0) return { applied: 0, dropped: 0 };
    const langCounters = { audio: {}, subtitles: {} };
    let applied = 0, dropped = 0;
    tracks = tracks.map(t => {
      if (t.type !== 'audio' && t.type !== 'subtitles') return t;
      const lang = (t.lang || '').toLowerCase();
      langCounters[t.type][lang] = (langCounters[t.type][lang] || 0) + 1;
      const n = langCounters[t.type][lang];
      const ov = seriesLabelOverrides[`${t.type}-${lang}-${n}`];
      if (ov) {
        applied++;
        return { ...t, label: ov.label, keep: ov.keep, default: ov.default, forced: ov.forced };
      }
      // Pas d'override pour cette piste → l'user l'avait supprimée/décochée
      // sur l'épisode précédent. On force keep=false pour propager la
      // "suppression" (garde la piste dans la liste pour permettre re-check
      // manuel si besoin, mais elle ne rentrera pas dans le mux).
      if (t.keep !== false) {
        dropped++;
        return { ...t, keep: false, default: false, forced: false };
      }
      return t;
    });
    return { applied, dropped };
  }
  function waitForAnalyzeComplete(timeoutMs = 30000) {
    return new Promise((resolve, reject) => {
      const started = Date.now();
      const timer = setInterval(() => {
        if (tracks.length > 0 && sourceInfo) {
          clearInterval(timer);
          setTimeout(resolve, 600); // laisse suggestLabels+overrides s'appliquer
        } else if (Date.now() - started > timeoutMs) {
          clearInterval(timer);
          reject(new Error('analyze timeout'));
        }
      }, 200);
    });
  }

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
  let tmdbSearching = $state(false);

  let muxing = $state(false);
  let muxPercent = $state(0);
  let logLines = $state([]);

  // ─── Onglet Synchro Audios ────────────────────────────────────────────────
  // État indépendant du workflow LiHDL/PSA. Charge un .mkv, affiche ses
  // pistes audio, permet de désigner une piste de référence et d'appliquer
  // un décalage (ms) aux autres. Le mux résultant copie l'audio bit-à-bit
  // (mkvmerge --sync TID:offset, pas de réencodage).
  let syncSourcePath = $state('');
  let syncAudioTracks = $state([]);   // [{id, codec, language, name, channels}]
  let syncRefId = $state(null);       // track ID désigné comme référence
  let syncOffsets = $state({});       // { trackId: delayMs }
  let syncRunning = $state(false);
  let syncPercent = $state(0);
  let syncDetecting = $state({});   // { trackId: true } pendant la détection
  let syncResults = $state({});     // { trackId: {offset_ms, confidence, drift_ms, method, notes} }

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
  let logEl = $state();

  // Subs externes ajoutés (srt/sup/ass/ssa/sub/idx). Chaque entrée a le même
  // format qu'une piste interne pour l'UI unifiée, mais avec un flag external.
  let externalSubs = $state([]);
  let externalAudios = $state([]);

  // OCR PGS → SRT : état réactif partagé entre toutes les pistes (un seul OCR
  // à la fois pour ne pas saturer Tesseract). ocrTrackId = id de la piste en
  // cours (-1 si aucun). ocrProgress = { status, percent, message }.
  let ocrRunning = $state(false);
  let ocrTrackId = $state(-1);
  let ocrProgress = $state({
    status: '', percent: 0, message: '',
    total_lines: 0, corrected_lines: 0, suspicious_lines: 0, quality_score: 0, subtitles: 0,
    lt_total_issues: 0, lt_auto_fixed: 0, lt_needs_review: 0, lt_review_list: [],
  });
  // Garde la barre OCR visible quelques secondes après "done" pour afficher le score.
  let ocrLastResultAt = 0;
  // Modal "lignes à vérifier" (LanguageTool) — toggle simple.
  let showLTReview = $state(false);
  // États par-match : pour chaque match (index dans lt_review_list) on stocke
  // `resolved` (✓ après patch ou ignore) et `customText` (champ d'édition libre).
  let ltReviewState = $state([]); // [{resolved, ignored, customText, busy, error}]
  // Path du SRT actuellement affiché dans le modal review (pour ApplyOCRFix).
  let ocrCurrentSRT = '';

  // OpenSubtitles modal de recherche.
  let showOSModal = $state(false);
  let osQuery = $state('');
  let osYear = $state('');
  let osLang = $state('fr,en');
  let osResults = $state([]);
  let osSearching = $state(false);
  let osError = $state('');
  // Quand le modal a été ouvert depuis la vue post-source : on ajoute le SRT
  // résultant aux externalSubs. Sinon (page d'accueil), on demande un dossier
  // de destination.
  let osContext = 'standalone'; // 'standalone' | 'post-source'
  let osDownloading = $state('');

  // Dictionnaire custom OCR — modal Settings.
  let customDictEntries = $state([]);
  let showAddDictModal = $state(false);
  let newDictWrong = $state('');
  let newDictRight = $state('');
  let dictBusy = $state(false);

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
  let secondaryPath = $state('');
  let secondaryTracks = $state([]);      // tracks audio + subs analysées
  let secondaryMkvInfo = $state(null);   // { duration_seconds, framerate, … }
  let psaSyncStatus = $state('');         // '' | 'checking' | 'ok' | 'corrected' | 'error'
  let psaSyncOffsetMs = 0;        // offset audio détecté (à appliquer aux subs aussi)
  let psaSyncTempoRatio = 1.0;    // tempo audio détecté (à appliquer aux subs aussi)
  let psaSyncMessage = $state('');        // texte à afficher (offset, conf, etc.)
  let secondarySelected = $state([]);    // tracks sélectionnées avec label LiHDL

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
    // ATMOS : signature fiable par-piste = "JOC" dans mi_format_features.
    // Les champs Format_Commercial_IfAny / Format_Commercial sont parfois
    // propagés à TOUTES les pistes EAC3 du fichier dès qu'une seule est Atmos
    // → faux positifs. On accepte aussi "ATMOS" dans le track_name
    // (mention explicite par l'encodeur).
    const features = String(track.mi_format_features || '').toUpperCase();
    const trackName = String(track.track_name || '').toUpperCase();
    const isAtmos = features.includes('JOC') || trackName.includes('ATMOS');
    // Service kind mediainfo : VI = Visual Impaired (audiodescription), HI = Hearing Impaired
    // Descripteurs FR/EN courants : audiodescription, descriptive, malvoyant, décrit, audio description
    const isAD = /^vi$/i.test(String(track.mi_service_kind || '')) ||
                 /audio.?descrip|audiodescri|descriptive|\bad\b|vmal|malvoyant|visual.?impair|d[eé]crit/i.test(hints);
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
    else if (lang === 'nor' || lang === 'no' || lang === 'nb' || lang === 'nob' || lang === 'nn' || lang === 'nno') prefix = 'NOR VO';
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
    // Forced : flag Matroska OU hint content (cue_count < 100) OU nom explicite.
    // sub_forced_hint est calculé par le backend pour les subs texte FR/ENG.
    const forced = !!track.forced_track || !!track.sub_forced_hint;
    const isHI = /^hi$/i.test(String(track.mi_service_kind || ''));
    let fmt = 'SRT';
    if (codecId.includes('PGS') || codecId.includes('HDMV')) fmt = 'PGS';
    // Prefix : uniquement FR/ENG dans les labels LiHDL. Pour toute autre
    // langue (hun/ita/jpn/kor/spa/…), on met le code langue en majuscules
    // — sortira des options.subtitle_labels donc suggestSubLabelFlat le
    // filtre à '' et le sub ne sera pas confondu avec ENG.
    let prefix = (lang || '').toUpperCase();
    if (lang === 'fre' || lang === 'fra' || lang === 'fr') {
      if (/canad|québ|quebec|vfq/i.test(hints)) prefix = 'FR VFQ';
      else if (/vff|france/i.test(hints))       prefix = 'FR VFF';
      else                                       prefix = 'FR';
    } else if (lang === 'eng' || lang === 'en') prefix = 'ENG';
    // Règle simplifiée : Forced > SDH (explicite uniquement) > Full (défaut).
    // SDH ne se déclenche QUE si mediainfo dit ServiceKind=HI, sdh_detected
    // du backend, OU si le nom de piste contient un mot-clé explicite.
    let kind = 'Full';
    if (forced || /forc(é|e)/i.test(hints)) {
      kind = 'Forced';
    } else if (isHI || !!track.sdh_detected || /\bsdh\b|sourds|hearing|malentend/i.test(hints)) {
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
    psaSyncOffsetMs = 0;
    psaSyncTempoRatio = 1.0;
    supplyExtraFlag = ''; // reset
    const filename = p.split('/').pop() || '';
    // Détecte un flag "FiNAL" dans le nom SUPPLY → sera inséré dans le filename.
    if (/\bFiNAL\b/i.test(filename)) {
      supplyExtraFlag = 'FiNAL';
      appendLog(`🏁 Flag "FiNAL" détecté dans SUPPLY → inséré entre SxxExx et MULTi`);
    }
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
      // Offset significatif → 2 cas :
      //   A. Pas de drift → simple --sync sur la piste SUPPLY
      //   B. Drift détecté → l'audio SUPPLY a été resamplé via atempo (.ac3
      //      retourné par le backend). On drop la piste SUPPLY originale et
      //      on ajoute le .ac3 resamplé comme externalAudio (avec --sync delay).
      const applyDelay = res.offset_ms;
      const tempoRatio = res.tempo_ratio || 1.0;
      psaSyncOffsetMs = applyDelay;
      psaSyncTempoRatio = tempoRatio;
      const hasResample = !!res.resampled_audio_path;
      if (hasResample) {
        // Drop la SUPPLY audio originale (track ID = res.resampled_track_id)
        secondarySelected = secondarySelected.map(t => {
          if (t.type === 'audio' && t.id === res.resampled_track_id) {
            return { ...t, keep: false };
          }
          return t;
        });
        // Ajoute l'audio resamplé comme externalAudio. Codec + langue
        // préservés depuis le backend (AC3/EAC3 + ATMOS si JOC source).
        const ch = res.resampled_channels || 6;
        const chStr = ch === 8 ? '7.1' : ch === 6 ? '5.1' : ch === 2 ? '2.0' : `${ch}ch`;
        const codec = (res.resampled_codec || 'AC3').toUpperCase();
        const lang = String(res.resampled_language || '').toLowerCase();
        let prefix = 'ENG VO';
        if (lang === 'fre' || lang === 'fra' || lang === 'fr' || lang.startsWith('fr-')) prefix = 'FR VFi';
        else if (lang === 'eng' || lang === 'en' || lang.startsWith('en-')) prefix = 'ENG VO';
        else if (lang === 'jpn' || lang === 'ja') prefix = 'JPN VO';
        else if (lang === 'ita' || lang === 'it') prefix = 'ITA VO';
        else if (lang === 'spa' || lang === 'es') prefix = 'SPA VO';
        else if (lang === 'ger' || lang === 'de') prefix = 'GER VO';
        const atmosSuffix = (res.resampled_is_atmos && codec === 'EAC3' && chStr === '5.1') ? ' ATMOS' : '';
        const label = `${prefix} : ${codec} ${chStr}${atmosSuffix}`;
        const fileExt = codec === 'EAC3' ? 'eac3' : 'ac3';
        // Order : pour une piste FR, on la veut AVANT les autres audios
        // (norme LiHDL = FR > ENG). Pour autre langue, on l'ajoute en queue.
        const isFRprefix = prefix.startsWith('FR ');
        let resampledOrder;
        if (isFRprefix) {
          let minAudioOrder = Infinity;
          for (const t of tracks) if (t.type === 'audio' && (t.order ?? 0) < minAudioOrder) minAudioOrder = t.order ?? 0;
          for (const t of secondarySelected) if (t.type === 'audio' && (t.order ?? 0) < minAudioOrder) minAudioOrder = t.order ?? 0;
          for (const a of externalAudios) if ((a.order ?? 0) < minAudioOrder) minAudioOrder = a.order ?? 0;
          resampledOrder = (minAudioOrder === Infinity ? 100 : minAudioOrder) - 10;
        } else {
          let maxOrder = 0;
          for (const t of tracks) maxOrder = Math.max(maxOrder, t.order ?? 0);
          for (const a of externalAudios) maxOrder = Math.max(maxOrder, a.order ?? 0);
          resampledOrder = maxOrder + 10;
        }
        externalAudios = [...externalAudios, {
          path: res.resampled_audio_path,
          name: `${prefix.replace(/ /g, '.')}.${codec}.${chStr}${atmosSuffix.replace(/ /g, '.')}.${fileExt}`,
          label,
          keep: true,
          default: true,
          forced: false,
          delayMs: applyDelay,
          tempoFactor: 1.0, // déjà baked
          order: resampledOrder,
        }];
        psaSyncStatus = 'corrected';
        psaSyncMessage = `Drift ${tempoRatio.toFixed(6)} resamplé (${label}) + offset ${applyDelay} ms (conf ${conf.toFixed(2)})`;
        appendLog(`↻ Sync PSA↔SUPPLY : ${label} resamplé via atempo + offset ${applyDelay} ms ajouté en externe (conf ${conf.toFixed(2)})`);
      } else {
        // Pas de drift, simple --sync sur la SUPPLY directe.
        secondarySelected = secondarySelected.map(t => {
          if (t.type !== 'audio') return t;
          return { ...t, delayMs: applyDelay };
        });
        psaSyncStatus = 'corrected';
        psaSyncMessage = `Décalage ${res.offset_ms} ms corrigé auto (conf ${conf.toFixed(2)})`;
        appendLog(`↻ Sync PSA↔SUPPLY : décalage ${res.offset_ms} ms → mkvmerge --sync ${applyDelay} ms (conf ${conf.toFixed(2)})`);
      }
    } catch (e) {
      psaSyncStatus = 'error';
      psaSyncMessage = String(e);
      appendLog('⚠ Sync PSA↔SUPPLY : ' + String(e));
    }
  }

  function clearSecondary() {
    supplyExtraFlag = '';
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
  // Mode PSA : ré-ordonne tous les subs (PSA kept + SUPPLY + externes) selon
  // la priorité LiHDL (FR avant ENG ; Forced > Full > SDH). Sinon les subs
  // PSA gardés (order bas hérité du mkv) ou les externes synchronisés (order
  // élevé) finissent au mauvais endroit.
  function reorderPSASubs() {
    const allSubsForOrder = [
      ...tracks.filter(t => t.type === 'subtitles' && t.keep),
      ...secondarySelected.filter(t => t.type === 'subtitles'),
      ...externalSubs,
    ];
    allSubsForOrder.sort((a, b) => subLabelPriority(a.label) - subLabelPriority(b.label));
    let subOrder = 1000;
    for (const s of allSubsForOrder) {
      s.order = subOrder;
      subOrder += 10;
    }
    // Force la réactivité Svelte sur les 3 sources mutées.
    tracks = [...tracks];
    secondarySelected = [...secondarySelected];
    externalSubs = [...externalSubs];
  }

  function automate() {
    if (!sourcePath) { appendLog('⚠ Charge d\'abord le fichier PSA'); return; }
    if (!secondaryPath || secondaryTracks.length === 0) {
      appendLog('⚠ Charge un fichier SUPPLY/FW d\'abord');
      return;
    }
    // Détection mode FASTSUB.VOSTFR / UNFR : pas de doublage FR, juste un sub
    // FR à coller sur la VO ENG. Dans ce cas on drop TOUS les autres subs
    // (PSA + SUPPLY non-FR) et on garde UNIQUEMENT le sub FR du SUPPLY.
    const langFlagEarly = String(target.flagOverride || '').toUpperCase();
    const supplyName = String(secondaryPath || '').toUpperCase();
    const isUnfrFastsub = /FASTSUB|VOSTFR/.test(langFlagEarly) || /UNFR/.test(supplyName);
    // 1. Drop audios PSA (toujours remplacés par SUPPLY). Pour les subs PSA,
    //    ne décocher QUE ceux dont SUPPLY fournit l'équivalent (même langue +
    //    même variante Forced/Full/SDH) — sinon on perdrait des subs uniques au
    //    PSA (typiquement ENG SDH absent du SUPPLY).
    //    En mode UNFR/FASTSUB : DROP TOUS les subs PSA d'office.
    const supplySubVariants = new Set();
    for (const st of secondaryTracks) {
      if (st.type !== 'subtitles') continue;
      const slang = String(st.language || '').toLowerCase();
      const baseLang = (slang === 'fre' || slang === 'fra' || slang === 'fr' || slang.startsWith('fr-')) ? 'FR'
                     : (slang === 'eng' || slang === 'en' || slang.startsWith('en-')) ? 'ENG'
                     : '';
      if (!baseLang) continue;
      const sname = String(st.track_name || '').toLowerCase();
      const sIsHI = /^hi$/i.test(String(st.mi_service_kind || '')) || /\bsdh\b|sourds|hearing|malentend/i.test(sname);
      const sVariant = st.forced ? 'Forced' : (sIsHI ? 'SDH' : 'Full');
      supplySubVariants.add(`${baseLang}|${sVariant}`);
    }
    tracks = tracks.map(t => {
      if (t.type === 'audio') return { ...t, keep: false };
      if (t.type === 'subtitles') {
        if (isUnfrFastsub) return { ...t, keep: false }; // mode UNFR : drop tous les subs PSA
        const lbl = String(t.label || '').toUpperCase();
        const baseLang = lbl.startsWith('FR ') ? 'FR' : lbl.startsWith('ENG ') ? 'ENG' : '';
        const variant = /\bFORCED\b/.test(lbl) ? 'Forced' : /\bSDH\b/.test(lbl) ? 'SDH' : 'Full';
        if (baseLang && supplySubVariants.has(`${baseLang}|${variant}`)) {
          return { ...t, keep: false };
        }
        return t; // pas d'équivalent SUPPLY → on garde le sub PSA
      }
      return t;
    });
    // 2. Construit secondarySelected depuis SUPPLY tracks avec auto-labels.
    // Préserve les modifs utilisateur/checkPSASync/syncSupplySubsToPSA
    // (keep:false, delayMs, tempoRatio) sinon écrasées quand automate est
    // rappelé avant le mux.
    const prevDelayByID = {};
    const prevTempoByID = {};
    const prevKeepFalseByID = {};
    // Lookup complet des pistes précédentes pour préserver TOUTES les modifs
    // manuelles user (label, default, forced, language, order). Sans ça, MUX
    // AUTO qui rappelle automate() juste avant doMux() écrase tout ce que
    // l'user a changé dans les dropdowns/checkboxes.
    const prevTrackByID = {};
    for (const t of secondarySelected) {
      prevTrackByID[t.id] = t;
      if (t.type === 'audio') {
        if (t.delayMs) prevDelayByID[t.id] = t.delayMs;
        if (t.tempoRatio && t.tempoRatio !== 1.0) prevTempoByID[t.id] = t.tempoRatio;
      }
      // keep:false préservé pour audio ET subs (syncSupplySubsToPSA décoche
      // les subs SUPPLY après alass).
      if (t.keep === false) prevKeepFalseByID[t.id] = true;
    }
    const hadPrevSelection = Object.keys(prevTrackByID).length > 0;
    // Si l'user a DÉJÀ configuré secondarySelected (via 1er automate + modifs
    // manuelles), on skip complètement le rebuild. Les modifs (suppressions
    // via ✕, réordonnements ↑↓, changements de labels) sont préservées.
    // Le rebuild ne se fait qu'au 1er appel (secondarySelected vide) — après
    // reset, changement de SUPPLY, ou nouveau chargement.
    if (hadPrevSelection) {
      appendLog('⚡ Automatisé (rappel) : ' + secondarySelected.filter(t=>t.type==='audio').length + ' audio(s) + ' + secondarySelected.filter(t=>t.type==='subtitles').length + ' sub(s) — modifs manuelles préservées');
      return;
    }
    // Détecte si syncSupplySubsToPSA a déjà tourné (= externalSubs avec
    // fromReference et label SRT) pour empêcher le re-marquage default+forced
    // du sub SUPPLY original.
    const hasExternalSyncedFRSub = externalSubs.some(s =>
      s.fromReference && /^FR /.test(s.label || '') && /SRT/.test(s.label || '')
    );
    const seenLangSubs = {};
    let order = 100; // après la vidéo (qui sera order=0..99)
    secondarySelected = secondaryTracks.map(t => {
      // Si la piste existait déjà dans secondarySelected (= 2ᵉ+ appel automate,
      // typiquement via MUX AUTO), on la restitue TELLE QUELLE pour préserver
      // toutes les modifs manuelles de l'user (label, default, forced, etc.).
      // Seules les pistes nouvelles ou après un RESET passent par le calcul auto.
      const prev = prevTrackByID[t.id];
      if (prev) {
        return { ...prev, codec: t.codec, codec_id: t.codec_id };
      }
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
        else if (/^NOR /.test(label))            language = 'nor';
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
        // Norme PSA : on garde les subs FR + ENG (toutes variantes).
        // Les autres langues sont filtrées (return null).
        // En mode UNFR/FASTSUB : on garde UNIQUEMENT le sub FR (pas l'ENG).
        const isFRsub = /^FR /.test(label);
        const isENGsub = /^ENG /.test(label);
        if (isUnfrFastsub) {
          if (!isFRsub) return null;
        } else {
          if (!isFRsub && !isENGsub) return null;
        }
        const isForcedFR = /^FR( VFF)? Forced\b/.test(label);
        // En mode UNFR/FASTSUB.VOSTFR : sub FR principal → default uniquement
        // (pas forced — c'est un sub Full visible en permanence, pas un sub
        // Forced classique pour traduire les rares dialogues étrangers).
        if (isUnfrFastsub && isFRsub) {
          defaultFlag = true;
          forcedFlag = false;
        } else if (isForcedFR) {
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
        keep: prevKeepFalseByID[t.id] ? false : keepFlag,
        default: defaultFlag,
        forced: forcedFlag,
        delayMs: prevDelayByID[t.id] || 0,
        tempoRatio: prevTempoByID[t.id] || 1.0,
        order: order++,
      };
    }).filter(Boolean);
    // Marque la 1ère piste audio FR comme default.
    // Si secondarySelected préexistait (2ᵉ+ appel automate), on skip les
    // règles auto pour ne PAS écraser les modifs manuelles de l'user.
    const firstFr = secondarySelected.find(t => t.type === 'audio' && t.language === 'fre');
    if (firstFr && !hadPrevSelection) firstFr.default = true;
    // Cas FASTSUB / VOSTFR : pas de doublage FR, on a la VO + sub FR. Donc :
    // - 1ère audio (typiquement ENG VO) → default
    // - 1er sub FR → default + forced (affiché auto sur la VO)
    const langFlag = String(target.flagOverride || '').toUpperCase();
    const isFastsubVO = /FASTSUB|VOSTFR/.test(langFlag);
    if (isFastsubVO && !firstFr && !hadPrevSelection) {
      const firstAudio = secondarySelected.find(t => t.type === 'audio');
      if (firstAudio) firstAudio.default = true;
      // Si syncSupplySubsToPSA a ajouté un sub FR synchronisé en externe,
      // on ne re-marque PAS le sub SUPPLY original (qui est désynchro). Le
      // sub externe synchronisé prend le rôle default+forced via son flag
      // interne (sr.Default côté backend).
      if (!hasExternalSyncedFRSub) {
        const firstFRSub = secondarySelected.find(t => t.type === 'subtitles' && /^FR /.test(t.label || ''));
        if (firstFRSub) {
          firstFRSub.default = true;
          firstFRSub.forced = false;
          appendLog(`🇫🇷 ${langFlag} : audio VO + sub FR marqué default (pas forced — sub Full)`);
        }
      } else {
        // Marque le 1er sub FR externe synchronisé comme default (pas forced
        // en mode FASTSUB.VOSTFR — c'est un sub Full visible en permanence).
        // Priorité Forced > Full pour le default.
        const extFRForced = externalSubs.find(s => s.fromReference && /^FR /.test(s.label || '') && /Forced/.test(s.label || ''));
        const extFRFull = externalSubs.find(s => s.fromReference && /^FR /.test(s.label || '') && !/Forced/.test(s.label || ''));
        const target = extFRForced || extFRFull;
        if (target) {
          target.default = true;
          target.forced = false;
          externalSubs = [...externalSubs]; // trigger reactivity
          appendLog(`🇫🇷 ${langFlag} : audio VO + sub FR synchronisé externe marqué default (pas forced)`);
        }
      }
    }

    // Reclassification FR AD : uniquement via marqueurs explicites
    // (flag_visual_impaired ou track_name AD/audiodescription). L'ancienne
    // heuristique "2 pistes + 2.0" créait des faux positifs sur VFF+VFQ.
    if (!hadPrevSelection) {
      for (const t of secondarySelected) {
        if (t.type !== 'audio') continue;
        if (!/^FR (VFF|VFQ|VFi|VOF) /.test(t.label || '')) continue;
        if (looksLikeAD(t)) {
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

    reorderPSASubs();

    // Debug : log de chaque piste audio avec ses indicateurs Atmos depuis mediainfo
    for (const raw of secondaryTracks) {
      if (raw.type !== 'audio') continue;
      appendLog(`[audio] id=${raw.id} lang=${raw.language} codec="${raw.codec||''}" codec_id="${raw.codec_id||''}" mi_format="${raw.mi_format||''}" profile="${raw.mi_format_profile||''}" features="${raw.mi_format_features||''}" commercial="${raw.mi_format_commercial||''}" name="${raw.track_name||''}"`);
    }
    // Plus de 2ᵉ passe sur les subs : par défaut Full, SDH uniquement si
    // mediainfo (ServiceKind=HI) ou track_name explicite (sourds/hearing/SDH).
    secondarySelected = [...secondarySelected];
    // 3. Réglages mode + Team. Custom PSA :
    //   - Série (tmdbMode === 'tv') → team GANDALF
    //   - Film (tmdbMode === 'movie') → team LiHDL (norme : films PSA sortent
    //     sous -LiHDL, séries PSA sous -GANDALF)
    // Mode Film : on n'impose PAS d'épisode (sinon le filename final aurait
    // un SxxExx pour un film). Mode TV : detect SxxExx depuis le nom de
    // fichier, fallback S01E01 si rien détecté.
    if (tmdbMode === 'movie') {
      videoChoice.team = 'LiHDL';
      target.episode = '';
    } else {
      videoChoice.team = 'ASTROBOY';
      if (!target.episode) {
        target.episode = detectEpisode((sourcePath || '').split('/').pop()) || 'S01E01';
      }
    }
    // 4. Norme LiHDL : swap FR VFF ↔ FR VFi selon useVFi (true par défaut sauf
    //    si VFQ détectée). Sans VFQ, la piste FR principale doit être VFi
    //    (même si y'a une AD à côté). Avec VFQ, elle reste VFF.
    applyVFiSwap();
    // 5. Applique la règle Forced/Default sur tous les subs en faisant
    //    l'analyse globale (gère le cas "VFQ Forced sans VFF Forced" → VFQ
    //    devient default+forced). Doit tourner APRÈS le build de
    //    secondarySelected pour voir tous les subs en même temps.
    applySubForcedRule();
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
    const w = parseInt(m[1], 10);
    const h = parseInt(m[2], 10);
    // 4K anamorphique très courant : 3840x1920 (2:1) ou 3840x1600 (2.4:1).
    // La hauteur peut descendre à 1600 en cropped, la largeur reste ~3840.
    // On détecte 2160p par largeur >= 3800 OU hauteur >= 2000.
    if (w >= 3800 || h >= 2000) return '2160p';
    if (w >= 1900 || h >= 1070) return '1080p';
    if (w >= 1200 || h >= 700)  return '720p';
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
    // Norme release : "&" → "and" (TMDB renvoie souvent l'esperluette).
    const cleaned = String(s || '').trim()
      .replace(/&/g, 'and')          // & → and (sinon perdu par le regex suivant)
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
    // Mode série : SxxExx remplace l'année (sauf si toggle "inclure l'année"
    // est activé, ex: pour distinguer remake/original). Mode film : année.
    if (target.episode) {
      if (includeYearInEpisode) {
        const yearMatch = String(target.title || '').match(/\((\d{4})\)\s*$/);
        const year = target.year || (yearMatch ? yearMatch[1] : '');
        if (year) parts.push(year);
      }
      parts.push(target.episode);
      // Flag extra du SUPPLY (ex: FiNAL) inséré entre SxxExx et MULTi.
      if (supplyExtraFlag) parts.push(supplyExtraFlag);
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
    // Pour 2160p : insérer 4KLight + tags HDR. Détection via mediainfo de la
    // piste vidéo + fallback sur le nom du fichier source PSA (parfois
    // mediainfo ne détecte pas DV/HDR10+ alors que le titre les annonce).
    if (target.resolution === '2160p') {
      parts.push('4KLight');
      const vt = tracks.find(t => t.type === 'video');
      const hdr = vt
        ? ((vt.mi_hdr_format || '') + ' ' + (vt.mi_hdr_compat || '') + ' ' + (vt.mi_hdr_string || '') + ' ' + (vt.mi_hdr_profile || '')).toLowerCase()
        : '';
      const fname = (sourcePath || '').split('/').pop() || '';
      const fnameUp = fname.toUpperCase();
      const hasDV = /dolby ?vision|dvhe|dvh1/.test(hdr) || /\bDV\b/.test(fnameUp);
      const hasHDR10Plus = /hdr10\+/.test(hdr) || /\bHDR10\+|\bHDR10PLUS\b/.test(fnameUp);
      const hasHDR = /\bhdr\b/.test(hdr) || /\bHDR\b/.test(fnameUp);
      if (hasDV) parts.push('DV');
      if (hasHDR10Plus) parts.push('HDR10Plus');
      else if (hasHDR) parts.push('HDR');
    }
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
    // Si la team == l'encodeur (ex: PSA ASTROBOY/ASTROBOY), on omet la team
    // pour éviter la duplication "Custom PSA ASTROBOY By ASTROBOY" → on garde
    // juste "Custom PSA By ASTROBOY".
    const team = (videoChoice.team && videoChoice.team !== videoChoice.encoder) ? videoChoice.team : '';
    return `${videoChoice.quality}${team ? ' ' + team : ''} By ${videoChoice.encoder} (Source ${src})`;
  }



  // --- actions ---
  function openMkv(path) {
    lihdlAutomatedOnce = false;
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
        videoChoice.encoder = config.default_encoder || 'ASTROBOY';
        // Team : GANDALF par défaut (séries). Pour les films PSA c'est LiHDL —
        // détecté via absence de pattern SxxExx dans le nom de fichier.
        const isSeries = /(?:^|[\s._-])S\d{1,2}E\d{1,3}(?=[\s._-]|$)/i.test(filename);
        videoChoice.team = isSeries ? 'ASTROBOY' : 'LiHDL';
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
        // Auto-détection résolution depuis le filename PSA.
        if (/\b2160p\b/i.test(filename) || /\b4K\b/i.test(filename)) {
          target.resolution = '2160p';
        } else if (/\b1080p\b/i.test(filename)) {
          target.resolution = '1080p';
        } else if (/\b720p\b/i.test(filename)) {
          target.resolution = '720p';
        }
        appendLog(`✓ PSA détecté : ${psa.source || '?'} ${target.resolution} ${target.video_codec || '?'} → Custom PSA / ASTROBOY`);
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
        // Le flag Matroska forced_track n'est PAS toujours mis par les rippers.
        // sub_forced_hint (backend, basé sur cue count < 100) sert de backup.
        forced: !!t.forced_track || !!t.sub_forced_hint,
        sub_forced_hint: !!t.sub_forced_hint,
        cue_count: typeof t.cue_count === 'number' ? t.cue_count : -1,
        sdh_detected: !!t.sdh_detected,
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
        mi_hdr_format: t.mi_hdr_format || '',
        mi_hdr_compat: t.mi_hdr_compat || '',
        mi_hdr_profile: t.mi_hdr_profile || '',
        mi_hdr_string: t.mi_hdr_string || '',
      };
      if (t.type === 'audio')     base.label = suggestAudioLabelFlat(base);
      if (t.type === 'subtitles') base.label = suggestSubLabelFlat(base);
      if (t.type === 'video')     base.label = '';
      return base;
    });
    // Les labels ont été pré-remplis via suggestAudioLabelFlat (qui appelle
    // le même inferAudioLabel qu'automateLihdl). À partir de là, tout appel
    // ultérieur d'automateLihdl doit PRÉSERVER les modifs manuelles de l'user
    // (ex: correction FR VFi → FR AD) au lieu de tout recalculer.
    lihdlAutomatedOnce = true;
    // Rejoue les overrides série (labels/keep/default/forced du précédent mux
    // dans le batch). Permet de propager les corrections manuelles ET les
    // suppressions de pistes à travers tous les épisodes de la série.
    const res = applySeriesOverrides();
    if (res.applied > 0 || res.dropped > 0) {
      const parts = [];
      if (res.applied > 0) parts.push(`${res.applied} label(s) rejoué(s)`);
      if (res.dropped > 0) parts.push(`${res.dropped} piste(s) auto-décochée(s) (retirées sur ép. précédent)`);
      appendLog(`⤴ ${parts.join(', ')} depuis l'épisode précédent (queue)`);
    }
    // Auto-set résolution depuis les dimensions vidéo réelles (pas juste le
    // filename) — couvre les cas anamorphiques 4K (3840x1920 = 2160p) et les
    // fichiers mal nommés. Prime sur toute détection filename précédente.
    const detectedRes = resolutionFromTracks();
    if (detectedRes) {
      if (target.resolution !== detectedRes) {
        appendLog(`✓ Résolution auto : ${target.resolution || '?'} → ${detectedRes} (via dimensions vidéo réelles)`);
      }
      target.resolution = detectedRes;
    }
    // Auto-config mode PSA : la PSA fournit la vidéo, le SUPPLY/FW/Super U
    // fournit les audios+subs. Donc on décoche d'office les audios PSA
    // (seront remplacés) et on ne garde que les subs FR + ENG.
    if (muxMode === 'psa') {
      let droppedAud = 0, droppedSub = 0;
      tracks = tracks.map(t => {
        if (t.type === 'audio') {
          if (t.keep) droppedAud++;
          return { ...t, keep: false };
        }
        if (t.type === 'subtitles') {
          const isFR = /^fr/i.test(t.lang || '') || /^FR /.test(t.label || '');
          const isENG = /^en/i.test(t.lang || '') || /^ENG /.test(t.label || '');
          const keepIt = isFR || isENG;
          if (t.keep && !keepIt) droppedSub++;
          return { ...t, keep: keepIt };
        }
        return t;
      });
      if (droppedAud || droppedSub) {
        appendLog(`🎬 Mode PSA : ${droppedAud} audio(s) PSA décoché(s) (remplacés par SUPPLY), ${droppedSub} sub(s) non-FR/ENG décoché(s)`);
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
    // Construire d'abord la liste sans order, puis trier par priorité LiHDL
    // pour éviter que l'ordre du dialog (Subtitles03/02/01) ne se retrouve
    // affiché tel quel — on veut FR Forced → FR Full → ENG etc.
    const newSubs = [];
    for (const p of paths) {
      const name = basename(p);
      let size = -1;
      try { size = await FileSize(p); } catch {}
      const forcedByName = /forced/i.test(name);
      const forcedBySize = isLikelyForcedBySize(name, size);
      const forced = forcedByName || forcedBySize;
      let label = suggestSubLabelFromFilename(name);
      if (forced && label && label.includes(' Full ')) {
        label = label.replace(' Full ', ' Forced ');
      }
      // Détection SDH par analyse contenu (crochets [BRUIT], ♪, locuteurs
      // MAJUSCULES:, parenthèses (soupir)…) — indépendante du nom. Skip si
      // déjà Forced (sub court) ou pas SRT texte.
      if (!forced && label && label.includes(' Full ') && /\.(srt|ass|ssa)$/i.test(name)) {
        try {
          const res = await DetectSubSDH(p);
          if (res && res.is_sdh) {
            const newLabel = label.replace(' Full ', ' SDH ');
            if (options.subtitle_labels.includes(newLabel)) {
              label = newLabel;
              appendLog(`♿ SDH détecté (score ${res.score}) → ${name} : ${label}`);
            }
          }
        } catch {}
      }
      newSubs.push({
        path: p, name, size,
        keep: true, default: false,
        forced, label,
      });
    }
    newSubs.sort((a, b) => subLabelPriority(a.label) - subLabelPriority(b.label));
    let maxOrder = 0;
    for (const t of tracks) maxOrder = Math.max(maxOrder, t.order ?? 0);
    for (const s of externalSubs) maxOrder = Math.max(maxOrder, s.order ?? 0);
    for (const s of newSubs) {
      maxOrder += 10;
      s.order = maxOrder;
      externalSubs = [...externalSubs, s];
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

  // Mode PSA : ajoute un MKV "synchro avec la PSA" → l'app extrait sa 1ère
  // piste audio FR (ou ENG fallback) et l'ajoute comme externalAudio. Drop
  // automatiquement les audios SUPPLY puisque cet audio les remplace.
  let psaSyncedAudioPath = $state('');   // path du .ac3/.eac3 extrait
  let psaSyncedSourceMkv = $state('');   // path du MKV source choisi (pour affichage)
  async function pickPSASyncedAudio() {
    try {
      const p = await SelectMkvFile();
      if (!p) return;
      psaSyncedSourceMkv = p;
      appendLog(`🎵 Extraction audio depuis ${basename(p)}…`);
      const res = await ExtractFirstAudioFromMkv(p);
      if (res.error) {
        appendLog('❌ ' + res.error);
        psaSyncedSourceMkv = '';
        return;
      }
      psaSyncedAudioPath = res.path;
      // Vire d'éventuels audios resampled ajoutés par checkPSASync auto
      // (le MKV synchro fourni par le user les remplace).
      externalAudios = externalAudios.filter(a => !(a.path && a.path.includes('.resampled.ac3')));
      // Ajoute comme externalAudio.
      let maxOrder = 0;
      for (const t of tracks) maxOrder = Math.max(maxOrder, t.order ?? 0);
      for (const a of externalAudios) maxOrder = Math.max(maxOrder, a.order ?? 0);
      let size = -1;
      try { size = await FileSize(res.path); } catch {}
      externalAudios = [...externalAudios, {
        path: res.path,
        name: basename(res.path),
        size,
        keep: true,
        default: true,
        forced: false,
        label: res.label || 'FR VFi : AC3 5.1',
        delayMs: 0,
        tempoFactor: 1.0,
        order: maxOrder + 10,
      }];
      // Drop tous les audios SUPPLY (remplacés par cet audio synchro).
      secondarySelected = secondarySelected.map(t =>
        t.type === 'audio' ? { ...t, keep: false } : t
      );
      psaSyncStatus = 'ok';
      psaSyncMessage = `Audio synchro extrait : ${res.label}`;
      appendLog(`✓ Audio extrait (${res.codec} ${res.channels}ch) → label: ${res.label}. Audios SUPPLY décochés.`);
    } catch (e) {
      appendLog('❌ ' + String(e));
    }
  }
  // Mode PSA : extrait les subs FR/ENG du SUPPLY et les synchronise sur la PSA
  // via alass. Les subs corrigés sont ajoutés comme externalSubs et les subs
  // SUPPLY originaux sont décochés.
  let psaSubSyncRunning = $state(false);
  async function syncSupplySubsToPSA() {
    if (!sourcePath) { appendLog('⚠ PSA non chargée'); return; }
    if (!secondaryPath) { appendLog('⚠ SUPPLY non chargée'); return; }
    psaSubSyncRunning = true;
    try {
      const driven = (psaSyncOffsetMs !== 0 || (psaSyncTempoRatio && psaSyncTempoRatio !== 1.0));
      if (driven) {
        appendLog(`🎯 Sync subs SUPPLY → PSA (audio-driven : offset ${psaSyncOffsetMs} ms, tempo ${psaSyncTempoRatio.toFixed(6)})…`);
      } else {
        appendLog(`🔎 Sync subs SUPPLY → PSA via alass…`);
      }
      const results = await SyncSupplySubsToPSA(sourcePath, secondaryPath, psaSyncOffsetMs, psaSyncTempoRatio || 1.0, psaSyncedSourceMkv || '');
      if (!results || results.length === 0) {
        appendLog('ℹ Aucun sub FR/ENG texte trouvé dans SUPPLY');
        return;
      }
      // Drop les subs SUPPLY (les externalSubs synchronisés les remplacent).
      secondarySelected = secondarySelected.map(t =>
        t.type === 'subtitles' ? { ...t, keep: false } : t
      );
      // Ajoute chaque sub corrigé comme externalSub.
      let maxOrder = 0;
      for (const t of tracks) maxOrder = Math.max(maxOrder, t.order ?? 0);
      for (const s of externalSubs) maxOrder = Math.max(maxOrder, s.order ?? 0);
      for (const r of results) {
        maxOrder += 10;
        let size = -1;
        try { size = await FileSize(r.path); } catch {}
        externalSubs = [...externalSubs, {
          path: r.path,
          name: basename(r.path),
          size,
          keep: true,
          default: !!r.default,
          forced: !!r.forced,
          label: r.label,
          delayMs: 0, // alass a déjà baked l'offset
          tempoFactor: 1.0,
          order: maxOrder,
          fromReference: true,
        }];
      }
      const synced = results.filter(r => r.synced).length;
      appendLog(`✓ ${results.length} sub(s) ajoutés (${synced} synchronisés alass, ${results.length - synced} bruts) — subs SUPPLY décochés`);
      reorderPSASubs();
    } catch (e) {
      appendLog('❌ Sync subs : ' + String(e));
    } finally {
      psaSubSyncRunning = false;
    }
  }

  function clearPSASyncedAudio() {
    if (psaSyncedAudioPath) {
      externalAudios = externalAudios.filter(a => a.path !== psaSyncedAudioPath);
    }
    psaSyncedAudioPath = '';
    psaSyncedSourceMkv = '';
    // Re-coche les audios SUPPLY (le user n'a plus d'audio externe).
    secondarySelected = secondarySelected.map(t =>
      t.type === 'audio' ? { ...t, keep: true } : t
    );
    appendLog('↩ Audio synchro retiré, audios SUPPLY re-cochés');
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
    // Adapte la track interne (camelCase) au format attendu par inferSubLabel
    // (snake_case + hints mediainfo/backend). Bénéficie AUTOMATIQUEMENT des
    // détections VFQ/VFF (canadian/quebec/vff), Forced (flag+cue count),
    // SDH (mediainfo/sdh_detected/track_name) au chargement initial.
    const raw = {
      language: t.lang,
      track_name: t.name,
      codec_id: t.codecId,
      codec: t.codec,
      forced_track: t.forced,
      sub_forced_hint: t.sub_forced_hint,
      sdh_detected: t.sdh_detected,
      mi_title: t.mi_title,
      mi_service_kind: t.mi_service_kind,
      mi_service_kind_name: t.mi_service_kind_name,
    };
    const candidate = inferSubLabel(raw, 0);
    return options.subtitle_labels.includes(candidate) ? candidate : '';
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
    const ms = /^(.+?)[\s._-]S\d{1,2}E\d{1,3}(?=[\s._-]|$)/i.exec(name);
    if (ms) name = ms[1];
    else {
      // Film : tout avant l'année 4 chiffres 19xx/20xx (séparateur . _ - espace).
      const my = /^(.+?)[\s._-](?:19|20)\d{2}(?=[\s._-]|$)/.exec(name);
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
    // Détection auto série uniquement via pattern SxxExx — le mode PSA n'impose
    // PLUS le mode TV (PSA fait aussi des films, l'user peut toggle Film/TV
    // dans la card de validation TMDB si l'auto-detect se trompe).
    const isSeries = /(?:^|[\s._-])S\d{1,2}E\d{1,3}(?=[\s._-]|$)/i.test(filename);
    const forceTV = isSeries;
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
  // Bascule Film ↔ Série dans la card de validation TMDB. Relance auto la
  // recherche avec le titre courant si on a déjà une query (cas user qui
  // s'aperçoit que TMDB a sorti une série étrangère pour son film, ou vice-versa).
  async function switchTmdbMode(newMode) {
    if (newMode === tmdbMode) return;
    tmdbMode = newMode;
    // Bascule Film ↔ Série : nettoie/restaure target.episode pour que le
    // filename final reflète le bon mode (Film → année dans le filename ;
    // Série → SxxExx).
    if (newMode === 'movie') {
      target.episode = '';
    } else if (newMode === 'tv' && !target.episode && sourcePath) {
      target.episode = detectEpisode(sourcePath.split('/').pop()) || 'S01E01';
    }
    if (tmdbQuery && tmdbQuery.trim()) {
      lastTmdbResult = null;
      tmdbResults = [];
      await manualTmdbSearchFromCard();
    }
  }

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

  // --- Labels audio/sub personnalisés (mergés avec les labels figés LiHDL) ---
  let newAudioLabel = $state('');
  let newSubtitleLabel = $state('');

  async function addCustomAudioLabel() {
    const lbl = newAudioLabel.trim();
    if (!lbl) return;
    if (!config.custom_audio_labels) config.custom_audio_labels = [];
    if (config.custom_audio_labels.includes(lbl)) {
      appendLog('⚠ Label audio déjà présent : ' + lbl);
      return;
    }
    config.custom_audio_labels = [...config.custom_audio_labels, lbl];
    newAudioLabel = '';
    await SaveConfig(config);
    options = await GetLihdlOptions();
    appendLog('✓ Label audio ajouté : ' + lbl);
  }

  async function removeCustomAudioLabel(idx) {
    const lbl = (config.custom_audio_labels || [])[idx];
    config.custom_audio_labels = (config.custom_audio_labels || []).filter((_, i) => i !== idx);
    await SaveConfig(config);
    options = await GetLihdlOptions();
    if (lbl) appendLog('🗑 Label audio supprimé : ' + lbl);
  }

  async function addCustomSubtitleLabel() {
    const lbl = newSubtitleLabel.trim();
    if (!lbl) return;
    if (!config.custom_subtitle_labels) config.custom_subtitle_labels = [];
    if (config.custom_subtitle_labels.includes(lbl)) {
      appendLog('⚠ Label sub déjà présent : ' + lbl);
      return;
    }
    config.custom_subtitle_labels = [...config.custom_subtitle_labels, lbl];
    newSubtitleLabel = '';
    await SaveConfig(config);
    options = await GetLihdlOptions();
    appendLog('✓ Label sub ajouté : ' + lbl);
  }

  async function removeCustomSubtitleLabel(idx) {
    const lbl = (config.custom_subtitle_labels || [])[idx];
    config.custom_subtitle_labels = (config.custom_subtitle_labels || []).filter((_, i) => i !== idx);
    await SaveConfig(config);
    options = await GetLihdlOptions();
    if (lbl) appendLog('🗑 Label sub supprimé : ' + lbl);
  }

  // --- Options personnalisées card « Réglages piste vidéo » (générique) ---
  // Permet d'ajouter / supprimer des valeurs dans chaque dropdown sans toucher
  // au code. Mergées côté backend avec les listes figées.
  let newCustomVideoOption = $state({
    custom_video_qualities: '',
    custom_video_encoders: '',
    custom_video_teams: '',
    custom_video_source_types: '',
    custom_resolutions: '',
    custom_target_sources: '',
    custom_video_codecs: '',
  });
  async function addCustomVideoOption(configKey, label) {
    const val = (newCustomVideoOption[configKey] || '').trim();
    if (!val) return;
    if (!config[configKey]) config[configKey] = [];
    if (config[configKey].includes(val)) {
      appendLog(`⚠ ${label} déjà présent : ${val}`);
      return;
    }
    config[configKey] = [...config[configKey], val];
    newCustomVideoOption[configKey] = '';
    await SaveConfig(config);
    options = await GetLihdlOptions();
    appendLog(`✓ ${label} ajouté : ${val}`);
  }
  async function removeCustomVideoOption(configKey, idx, label) {
    const val = (config[configKey] || [])[idx];
    config[configKey] = (config[configKey] || []).filter((_, i) => i !== idx);
    await SaveConfig(config);
    options = await GetLihdlOptions();
    if (val) appendLog(`🗑 ${label} supprimé : ${val}`);
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

  let tmdbTest = $state({ running: false, ok: null, message: '' });
  let hydrackerTest = $state({ running: false, ok: null, message: '' });
  let unfrTest = $state({ running: false, ok: null, message: '' });
  let ltTest = $state({ running: false, ok: null, message: '' });

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
  let updateInfo = $state(null);       // {version, url, notes} si dispo
  let checkingUpdate = $state(false);
  let installingUpdate = $state(false);

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
    // Si un MKV synchro est fourni en ligne ③ (mode PSA), on EXCLUT toujours
    // les audios SUPPLY (l'audio extrait du MKV synchro les remplace).
    const secAudios = psaSyncedAudioPath
      ? []
      : secondarySelected.filter(t => t.type === 'audio' && t.keep).map(t => ({
          ID: t.id,
          Name: t.label || '',
          Language: t.language || mapLangCode(t.label || ''),
          Default: !!t.default,
          Forced: !!t.forced,
          VisualImpaired: isAD(t.label),
          DelayMs: t.delayMs || 0,
          TempoRatio: t.tempoRatio || 1.0,
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
        no_chapters: false, // PSA ET LiHDL : on garde toujours les chapitres natifs de la source
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
  let lihdlAutomatedOnce = false;
  function automateLihdl(navigateToCible = true) {
    if (!sourcePath) { appendLog('⚠ Charge un fichier d\'abord'); return; }
    if (tracks.length === 0) { appendLog('⚠ Pistes pas encore analysées — patiente'); return; }
    // 2e appel ou + : préserve les modifs manuelles de l'user (label, keep,
    // default, forced) au lieu de tout recalculer via inferAudioLabel — sinon
    // un FR VFi corrigé à la main en FR AD reviendrait à FR VFi au MUX AUTO.
    const preserveUserEdits = lihdlAutomatedOnce;
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
        sub_forced_hint: t.sub_forced_hint,
        cue_count: t.cue_count,
        sdh_detected: t.sdh_detected,
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
        const newLabel = preserveUserEdits && t.label ? t.label : inferAudioLabel(raw);
        const isFR = /^FR /.test(newLabel);
        // Préserve keep: false (piste FR remplacée par extraction) ; sinon true.
        const newKeep = t.keep === false ? false : true;
        // Default : si externes FR présentes, jamais de default sur interne.
        // En preserveUserEdits, on garde le default choisi par l'user.
        const isDefault = preserveUserEdits ? !!t.default
          : (!hasExternalFRAudio && newKeep && isFR && !firstFRdone);
        if (isDefault) firstFRdone = true;
        if (newKeep) nbAudio++;
        const newForced = preserveUserEdits ? !!t.forced : false;
        return { ...t, label: newLabel, keep: newKeep, default: isDefault, forced: newForced };
      } else { // subtitles
        const newLabel = preserveUserEdits && t.label ? t.label : inferSubLabel(raw, 0);
        const isForcedFR = /^FR( VFF)? Forced\b/.test(newLabel);
        nbSubs++;
        return {
          ...t,
          label: newLabel,
          keep: preserveUserEdits ? (t.keep !== false) : true,
          default: preserveUserEdits ? !!t.default : isForcedFR,
          forced: preserveUserEdits ? !!t.forced : isForcedFR,
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

    // Reclassification FR AD : uniquement via marqueurs explicites
    // (flag_visual_impaired ou track_name AD/audiodescription).
    tracks = tracks.map(t => {
      if (t.type !== 'audio') return t;
      if (!/^FR (VFF|VFQ|VFi|VOF) /.test(t.label || '')) return t;
      if (looksLikeAD(t)) {
        return { ...t, label: t.label.replace(/^FR (VFF|VFQ|VFi|VOF) /, 'FR AD ') };
      }
      return t;
    });

    // Subs externes : applique aussi la règle FR Forced → keep + default + forced
    // (les externes ne passent pas par automateLihdl ci-dessus, on les traite ici).
    // En preserveUserEdits (2e appel+), on garde keep/default/forced choisis par
    // l'user — ex: l'user a mis Default sur FR Full, on ne doit pas l'écraser.
    externalSubs = externalSubs.map(s => {
      const isForcedFR = /^FR( VFF)? Forced\b/.test(s.label || '');
      return {
        ...s,
        keep: preserveUserEdits ? (s.keep !== false) : true,
        default: preserveUserEdits ? !!s.default : isForcedFR,
        forced: preserveUserEdits ? !!s.forced : isForcedFR,
      };
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

    // TEAM (sortie) : LiHDL par défaut au 1er automate, préservé ensuite
    // si l'user l'a changé manuellement (ex: ASTROBOY) — même logique que
    // les tracks (preserveUserEdits).
    if (!preserveUserEdits) {
      videoChoice.team = 'LiHDL';
    }
    // Ne pas vider l'épisode en mode Série (sinon SxxExx disparaît du filename
    // après MUX AUTO et le nom se retrouve avec l'année à la place, ex:
    // House.Of.The.Dragon.2022... au lieu de House.Of.The.Dragon.S03E02...).
    if (tmdbMode === 'movie') {
      target.episode = '';
    } else if (!target.episode) {
      target.episode = detectEpisode((sourcePath || '').split('/').pop()) || 'S01E01';
    }

    // Norme LiHDL : ré-applique l'ordre des pistes (FR VFi/VFF avant FR VFQ)
    // ET réassigne le flag default sur la 1ère piste audio en ordre LiHDL.
    // Sinon, la boucle map ci-dessus a mis default=true sur la 1ère FR rencontrée
    // dans l'ordre SOURCE (souvent FR VFQ si le mkv a VFQ avant VFF).
    applyLihdlTrackOrder();

    if (preserveUserEdits) {
      appendLog(`⚡ LiHDL automatisé (rappel) : ${nbAudio} audio(s) + ${nbSubs} sub(s) — modifs manuelles préservées`);
    } else {
      appendLog(`⚡ LiHDL automatisé : ${nbAudio} audio(s) + ${nbSubs} sub(s) labellisés → Cible`);
    }
    lihdlAutomatedOnce = true;
    if (navigateToCible) screen = 'cible';
  }

  // 🚀 MUX AUTO LiHDL : automateLihdl puis doMux direct (sans nav vers Cible).
  // Si une piste FR Full est en PGS seul (pas de FR Full SRT ni interne ni
  // externe), génère un SRT via l'app Mac externe « PGS vers SRT.app » (OCR
  // pgsrip + relecture Opus) et l'ajoute en externalSubs comme FR Full : SRT.
  // Appelé avant doMux en MUX AUTO. Fail-soft : si ça rate, on continue avec
  // le PGS seul.
  async function maybeGenerateFRSrtFromPGSAuto() {
    try {
      const frFullSrtExists =
        tracks.some(t => t.type === 'subtitles' && t.keep !== false && /^FR Full : SRT\b/.test(t.label || '')) ||
        externalSubs.some(s => s.keep !== false && /^FR Full : SRT\b/.test(s.label || ''));
      if (frFullSrtExists) return;
      // Cherche une piste FR Full PGS interne (gardée).
      const pgsTrack = tracks.find(t =>
        t.type === 'subtitles' &&
        t.keep !== false &&
        /^FR Full : PGS\b/.test(t.label || '')
      );
      if (!pgsTrack) return;
      appendLog('🔠 FR Full PGS sans SRT détecté → OCR + relecture Opus auto…');
      // Décoche la piste PGS pour qu'elle ne finisse pas en double dans le mux.
      tracks = tracks.map(t => t === pgsTrack ? { ...t, keep: false } : t);
      const srtPath = await GenerateFRSrtFromPGS(sourcePath, pgsTrack.id);
      if (!srtPath) throw new Error('srt vide');
      let size = -1;
      try { size = await FileSize(srtPath); } catch {}
      // Norme LiHDL : FR Full : SRT (y compris généré depuis OCR PGS) passe
      // EN PREMIER dans la liste des sous-titres — order < tous les autres.
      let minOrder = 0;
      for (const t of tracks) {
        if (t.type !== 'subtitles') continue;
        if (t.order == null) continue;
        if (t.order < minOrder) minOrder = t.order;
      }
      for (const s of externalSubs) {
        if (s.order == null) continue;
        if (s.order < minOrder) minOrder = s.order;
      }
      externalSubs = [...externalSubs, {
        path: srtPath,
        name: srtPath.split('/').pop(),
        size,
        keep: true,
        default: !!pgsTrack.default,
        forced: !!pgsTrack.forced,
        label: 'FR Full : SRT',
        order: minOrder - 10,
      }];
      // Attend que Svelte propage l'ajout — sans ça effectiveFilename
      // (langFlagClient → hasFrSub) peut être computé avant que externalSubs
      // soit vu comme modifié, donnant DUAL au lieu de MULTi dans le filename
      // final du mux.
      await tick();
      appendLog('✓ FR Full : SRT ajouté au mux depuis OCR PGS');
    } catch (e) {
      appendLog('⚠ Génération FR SRT depuis PGS échouée : ' + String(e) + ' — mux avec PGS seul');
    }
  }

  async function muxAutoLihdl() {
    if (!sourcePath) { appendLog('⚠ Charge un fichier d\'abord'); return; }
    autoMuxStatus = '';
    if (autoMuxStatusTimer) { clearTimeout(autoMuxStatusTimer); autoMuxStatusTimer = null; }
    automateLihdl(false); // pas de navigation, on reste sur source pour la barre de progress
    await new Promise(r => setTimeout(r, 200));
    await maybeGenerateFRSrtFromPGSAuto();
    // Snapshot des labels/keep/default/forced JUSTE avant le mux → propagé
    // aux épisodes suivants de la queue si Auto-continue est actif.
    snapshotSeriesOverrides();
    const ok = await doMux();
    autoMuxStatus = ok ? 'success' : 'error';
    // Pas d'auto-clear timer : la barre verte/rouge reste visible jusqu'au prochain mux ou reset manuel.
    if (ok) await autoResetAfterMux(true);
    // Auto-continue : charge le prochain épisode et relance MUX AUTO.
    if (ok && queueAutoContinue && queue.length > 0) {
      const nextPath = queue[0];
      queue = queue.slice(1);
      // Mémorise la validation TMDB avant openMkv (qui la reset).
      const prevTmdb = lastTmdbResult;
      appendLog(`🤖 Auto-continue : chargement de ${nextPath.split('/').pop()} (${queue.length} restant(s))…`);
      openMkv(nextPath);
      try {
        await waitForAnalyzeComplete();
        // Auto-valide la fiche TMDB (même série sur tous les épisodes de la
        // queue → pas besoin de re-demander confirmation à chaque épisode).
        if (prevTmdb) {
          lastTmdbResult = prevTmdb;
          tmdbValidated = true;
          appendLog(`🤖 Auto-continue : fiche TMDB « ${prevTmdb.titre_fr || prevTmdb.titre_vo} » ré-appliquée`);
        }
        await muxAutoLihdl(); // récursion — la queue diminue à chaque tour
      } catch (e) {
        appendLog('❌ Auto-continue interrompu : ' + String(e));
      }
    } else if (ok && queueAutoContinue && queue.length === 0) {
      appendLog('✅ Auto-continue : queue vidée — série terminée');
    }
  }

  // MUX AUTO : automate puis lance le mux directement, sans passer par Cible.
  let autoMuxStatus = $state('');   // '' | 'success' | 'error'
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
    await maybeGenerateFRSrtFromPGSAuto();
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

    // Migration : ancien encodeur GANDALF renommé ASTROBOY. Met à jour la
    // config sauvegardée pour que le dropdown ne montre pas une option orpheline.
    if (config.default_encoder === 'GANDALF') {
      config.default_encoder = 'ASTROBOY';
      try { await SaveConfig(config); } catch {}
    }

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
  // Lookup Discord à chaque changement de TMDB ID : on demande au backend
  // (qui regarde l'index local d'abord, puis le remote si configuré). Best-
  // effort silencieux ; aucune erreur ne remonte côté UI.
  run(() => {
    if (lastTmdbResult && lastTmdbResult.tmdb_id) {
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
  });
  // Réactif Svelte : à chaque changement audio (interne, externe, secondary),
  // recalcule les labels SRT FR ↔ FR VFF selon la présence VFQ. Le guard
  // dans applyFRSubVFFLabels évite les boucles infinies.
  run(() => {
    void tracks; void externalAudios; void secondarySelected;
    applyFRSubVFFLabels();
  });
  // Réactif Svelte : si une VFQ apparaît dans les pistes audio, l'autre piste
  // FR (typiquement VFi) doit devenir FR VFF (norme LiHDL — quand y'a 2 pistes
  // FR, l'une est VFF/internationale, l'autre est VFQ québécoise).
  run(() => {
    void tracks; void externalAudios; void secondarySelected;
    const hasVFQ = tracks.some(t => t.type === 'audio' && /^FR VFQ/.test(t.label || ''))
      || externalAudios.some(a => /^FR VFQ/.test(a.label || ''))
      || secondarySelected.some(t => t.type === 'audio' && /^FR VFQ/.test(t.label || ''));
    if (hasVFQ && useVFi) {
      useVFi = false;
      applyVFiSwap(); // FR VFi → FR VFF (useVFi=false)
    }
  });
  // $derived (runes) suit automatiquement tout ce que lit buildFilenameClient().
  let previewFilename = $derived(buildFilenameClient());
  // Le nom utilisé pour le mux : manuel si override, sinon généré.
  let effectiveFilename = $derived(filenameOverride ? manualFilename : previewFilename);
  // Recharge la liste du dico OCR quand on entre dans l'écran Réglages.
  run(() => {
    if (screen === 'reglages' && typeof window !== 'undefined') { loadCustomDictReactive(); }
  });
  // Réactif : true si FPS PSA ≈ FPS SUPPLY (±0.05) ET audio sync OK ou corrigé.
  let psaSourcesValid = $derived(!!(
    sourceMkvInfo && secondaryMkvInfo &&
    Math.abs((sourceMkvInfo.framerate || 0) - (secondaryMkvInfo.framerate || 0)) <= 0.05 &&
    (psaSyncStatus === 'ok' || psaSyncStatus === 'corrected')
  ));
  let previewVideoName = $derived(videoTrackNameClient());
  // Auto-fill du codec cible quand aucun n'est explicitement choisi.
  run(() => {
    if (!target.video_codec) target.video_codec = suggestedVideoCodec();
  });
  let suggestedCodecDisplay = $derived(suggestedVideoCodec());
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
        <div class="brand-version">{appVersion || 'v?'} · BY ASTROBOY</div>
      </div>
    </div>

    <div class="mode-switch">
      <button class="mode-btn" class:active={muxMode === 'lihdl'} onclick={() => switchMuxMode('lihdl')}><Icon name="bolt" /> MUX LiHDL</button>
      <button class="mode-btn" class:active={muxMode === 'psa'}   onclick={() => switchMuxMode('psa')}><Icon name="film" /> Custom PSA</button>
    </div>

    <!-- Film-bar : si TMDB sélectionné, on affiche poster + identité ; sinon placeholder -->
    <HeaderFilmBar {lastTmdbResult} {discordIndexEntry} openURL={OpenURL} />

    <MuxActionBar
      {updateInfo}
      {installingUpdate}
      {checkingUpdate}
      {muxing}
      {sourcePath}
      {muxMode}
      {secondaryPath}
      secondaryTracksCount={secondaryTracks.length}
      tracksCount={tracks.length}
      onInstallUpdate={doInstallUpdate}
      onCheckUpdate={checkForUpdate}
      onReset={resetAll}
      onToggleSettings={() => screen = (screen === 'reglages' ? 'source' : 'reglages')}
      onAutomate={automate}
      onMuxAuto={muxAuto}
      onAutomateLihdl={() => automateLihdl()}
      onMuxAutoLihdl={muxAutoLihdl}
    />
  </header>

  <!-- Sub-nav : Source / Cible / Sync / Réglages (utilisée seulement si on quitte 'source') -->
  {#if screen !== 'source'}
    <nav class="subnav">
      <button class="btn btn-ghost" onclick={() => screen = 'source'}>← Retour</button>
      <button class="subnav-btn" class:active={screen === 'source'}   onclick={() => screen = 'source'}>Source</button>
      <button class="subnav-btn" class:active={screen === 'cible'}    onclick={() => screen = 'cible'}>Cible</button>
      <button class="subnav-btn" class:active={screen === 'sync'}     onclick={() => screen = 'sync'}>Synchro Audios</button>
      <button class="subnav-btn" class:active={screen === 'reglages'} onclick={() => screen = 'reglages'}>Réglages</button>
    </nav>
  {/if}

  <!-- Mux progress bar : si en cours, on affiche au-dessus du main -->
  {#if muxing}
    <div class="mux-progress-bar">
      <button class="btn btn-danger btn-stop-small" onclick={stopMux}><Icon name="stop" /> Stop</button>
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
          <div class="auto-status done"><Icon name="check-circle" /> Mux terminé avec succès — fichier prêt dans ton dossier de sortie</div>
        </div>
      {:else if autoMuxStatus === 'error' && !muxing}
        <div class="card mux-status-banner error">
          <div class="auto-status error"><Icon name="x-circle" /> Mux échoué — vérifie les logs en bas</div>
        </div>
      {/if}

      <!-- Card unifiée : PSA + SUPPLY/FW (en mode PSA) ou seul (en mode LiHDL).
           Cachée tant que TMDB pas validé pour ne pas polluer la page d'accueil. -->
      {#if tmdbValidated || !sourcePath}
        <SourcesCard
          {muxMode}
          {sourcePath}
          {sourceMkvInfo}
          {psaSourcesValid}
          {secondaryPath}
          {secondaryTracks}
          {secondaryMkvInfo}
          {psaSyncStatus}
          {psaSyncMessage}
          {psaSubSyncRunning}
          {psaSyncedAudioPath}
          {psaSyncedSourceMkv}
          bind:externalSubs
          {srtExtracting}
          {srtExtractionResult}
          {srtPhase}
          {srtPercent}
          {srtAssConverted}
          {subSyncChecking}
          {subSyncResults}
          {subSyncAppliedMsg}
          {subSyncPercent}
          {subSyncCurrentName}
          {referencePath}
          {referenceMkvInfo}
          bind:showReferenceBar
          bind:extractFRVFF
          bind:extractFRVFQ
          bind:extractENG
          bind:syncRefLang
          {frAudioExtracting}
          {frAudioExtractionResult}
          {frAudioPhase}
          {frAudioConvertPercent}
          {frAudioConvertedSummary}
          {basename}
          {formatDuration}
          {checkCompat}
          {pickMkvDialog}
          {pickSecondaryDialog}
          {pickPSASyncedAudio}
          {clearPSASyncedAudio}
          {syncSupplySubsToPSA}
          {pickSubsDialog}
          {runSubSyncCheck}
          {applySubSyncResults}
          {dismissSubSyncResults}
          {pickReferenceDialog}
          {clearReference}
          {runRefSubsExtraction}
          {runFRAudioExtraction}
        />
      {/if}

      <!-- Queue déplacée dans la card secondaire en bas droite (tab "Queue"). -->


      <!-- Tracks (vidéo + audio + subs) — vue compacte mockup. Cachées tant que TMDB pas validé pour qu'on reste visuellement sur l'accueil. -->
      {#if tracks.length > 0 && tmdbValidated}
        <TrackList
          bind:tracks
          bind:externalAudios
          bind:externalSubs
          bind:secondarySelected
          {videoChoice}
          {previewVideoName}
          {options}
          {psaSyncedAudioPath}
          {ocrRunning}
          {ocrTrackId}
          {ocrProgress}
          bind:showLTReview
          {basename}
          {formatBytes}
          {isPGSTrack}
          {isPGSExternal}
          onPickAudio={pickAudioDialog}
          onPickSubs={pickSubsDialog}
          onMoveTrack={moveTrack}
          onMoveAudioTrack={moveAudioTrack}
          onRemoveInternalTrack={removeInternalTrack}
          onRemoveSecondaryTrack={removeSecondaryTrack}
          onRemoveExternalAudio={removeExternalAudio}
          onRemoveExternalSub={removeExternalSub}
          onRunOCR={runOCR}
          onRunOCRExternalSup={runOCRExternalSup}
          {onSubLabelChange}
        />
      {/if}
    </div><!-- /col gauche -->

    <!-- COLONNE DROITE -->
    <div class="col">

      <!-- Empty state : visible quand aucune source OU quand source chargée mais TMDB pas encore validé (trouvé OU pas trouvé) -->
      {#if (!sourcePath && tracks.length === 0) || (sourcePath && !tmdbValidated)}
        <EmptyHero
          {sourcePath}
          {muxMode}
          {ocrRunning}
          {ocrTrackId}
          {ocrProgress}
          onPickMkv={pickMkvDialog}
          onPickMultipleMkv={pickMultipleMkvDialog}
          onPickAndOCR={pickAndOCRStandaloneSup}
          onOpenOSModal={() => openOSModal('standalone')}
          onOpenSync={() => screen = 'sync'}
        />
      {/if}

      <!-- Card validation TMDB : 2 états (rien trouvé / fiche à confirmer). -->
      <TmdbValidateCard
        bind:tmdbMode
        bind:tmdbQuery
        bind:tmdbIdQuery
        {tmdbSearching}
        bind:tmdbValidated
        {lastTmdbResult}
        {tmdbResults}
        {tmdbResultIndex}
        {sourcePath}
        onSearchById={searchTmdbById}
        onManualSearch={manualTmdbSearchFromCard}
        onCycleNext={cycleNextTmdbResult}
        onSwitchMode={switchTmdbMode}
      />

      <!-- Card FICHE FILM supprimée — infos déjà visibles dans la card validation et le header film-bar. -->

      <!-- Réglages piste vidéo -->
      {#if tracks.length > 0 && tracks.some(t => t.type === 'video') && tmdbValidated}
        <VideoSettingsCard
          bind:videoChoice
          bind:target
          {options}
          bind:tmdbMode
          bind:includeYearInEpisode
          {sourcePath}
          {suggestedCodecDisplay}
          flagOverrideOptions={FLAG_OVERRIDE_OPTIONS}
          resolutionOptions={RESOLUTION_OPTIONS}
          targetSourceOptions={TARGET_SOURCE_OPTIONS}
          videoCodecOptions={VIDEO_CODEC_OPTIONS}
          videoSourceTypeOptions={VIDEO_SOURCE_TYPE_OPTIONS}
          {onSourceTypeChange}
          {detectEpisode}
        />
      {/if}

      <!-- Filename preview -->
      {#if tracks.length > 0 && tmdbValidated}
        <FilenameCard
          {lastTmdbResult}
          bind:target
          {filenameOverride}
          bind:manualFilename
          {previewFilename}
          {effectiveFilename}
          {filenameCopied}
          {effectiveOutputDir}
          onCopy={copyFilename}
          onOpenDir={openOutputDir}
          onStartOverride={startFilenameOverride}
          onResetOverride={resetFilenameOverride}
        />
      {/if}

      <!-- Toggle FR VFi + liens externes (TMDB / Wikipédia / Hydra / UNFR) -->
      {#if lastTmdbResult}
        <div class="card vfq-links-card">
          <div class="card-title"><Icon name="globe-alt" /> Langues, Sous-Titres & Liens</div>
          {#if muxMode === 'lihdl'}
            <label class="vfq-toggle">
              <input type="checkbox" bind:checked={useVFi} onchange={applyVFiSwap} />
              <span class:vfq-yes={useVFi} class:vfq-no={!useVFi}>
                {useVFi ? '✓ FR VFi (international)' : '☐ FR VFF (France)'}
              </span>
              {#if lastTmdbResult.tmdb_id}
                <button class="vfq-link" type="button" onclick={() => OpenURL(`https://www.themoviedb.org/movie/${lastTmdbResult.tmdb_id}/translations`)}>TMDB <Icon name="arrow-top-right-on-square" /></button>
                <button class="vfq-link" type="button" onclick={() => OpenURL(`https://fr.wikipedia.org/w/index.php?title=Special:Search&go=Go&search=${encodeURIComponent(lastTmdbResult.titre_fr || lastTmdbResult.titre_vo || '')}`)}>Wikipédia <Icon name="arrow-top-right-on-square" /></button>
              {/if}
            </label>
          {/if}
          {#if lastTmdbResult.tmdb_id}
            <div class="vfq-toggle" style:margin-top="6px">
              <span class="srt-label"><Icon name="magnifying-glass" /> Sous-titres SRT :</span>
              <button class="vfq-link" type="button" onclick={() => OpenURL(hydrackerURL || `https://hydracker.com/titles?search=${encodeURIComponent(lastTmdbResult.titre_fr || lastTmdbResult.titre_vo || '')}`)}>Hydra <Icon name="arrow-top-right-on-square" /></button>
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
        <OutilsCard
          {ocrRunning}
          {ocrTrackId}
          {ocrProgress}
          bind:showLTReview
          onOpenSync={() => screen = 'sync'}
          onOpenCible={() => screen = 'cible'}
          onPickAndOCR={pickAndOCRStandaloneSup}
          onOpenOSModal={() => openOSModal('post-source')}
        />
      {/if}

      <!-- Card secondaire en bas droite : tabs Journal | Queue -->
      <!-- Visible dès qu'une source est chargée (y compris pendant la validation
           TMDB) pour permettre de voir les erreurs quand ça bloque sur « Aucun
           film identifié ». Avant tmdbValidated, le journal est critique. -->
      {#if sourcePath || queue.length > 0}
        <JournalPane
          {logLines}
          bind:logEl
          bind:bottomPaneTab
          {queue}
          bind:queueAutoContinue
          {muxing}
          {sourcePath}
          onQueueLoad={queueLoad}
          onQueueRemove={queueRemove}
          onQueueNext={queueNext}
          onQueueClear={() => queue = []}
        />
      {/if}

    </div><!-- /col droite -->

    {:else if screen === 'cible'}
    <div class="fullscreen">
      <div class="card">
        <div class="card-title">Recherche TMDB</div>
        <div class="lang-toggle" style:margin-bottom="8px">
          <button class:active={tmdbMode === 'movie'} onclick={() => tmdbMode = 'movie'}><Icon name="film" /> Film</button>
          <button class:active={tmdbMode === 'tv'}    onclick={() => tmdbMode = 'tv'}><Icon name="tv" /> Série</button>
        </div>
        <div class="field-row">
          <input type="text" bind:value={tmdbQuery} placeholder={tmdbMode === 'tv' ? 'Titre de série…' : 'Titre du film…'} onkeydown={(e) => e.key === 'Enter' && searchTmdb()} />
          <button class="btn btn-accent" onclick={searchTmdb} disabled={tmdbSearching}>{tmdbSearching ? '…' : 'Chercher par titre'}</button>
        </div>
        <div class="field-row" style:margin-top="6px">
          <input type="text" bind:value={tmdbIdQuery} placeholder="ID TMDB numérique (ex: 12345)…" inputmode="numeric" pattern="[0-9]*" onkeydown={(e) => e.key === 'Enter' && searchTmdbById()} />
          <button class="btn btn-accent" onclick={searchTmdbById} disabled={tmdbSearching}>{tmdbSearching ? '…' : 'Chercher par ID'}</button>
        </div>
        {#if tmdbMode === 'tv' && !config.tmdb_key}
          <div class="field-hint"><Icon name="exclamation-triangle" /> Clé API TMDB requise pour chercher des séries — Réglages.</div>
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
                  <input type="checkbox" bind:checked={useVFi} onchange={applyVFiSwap} />
                  <span class:vfq-yes={useVFi} class:vfq-no={!useVFi}>
                    {useVFi ? '✓ FR VFi (doublage international)' : '☐ FR VFF (doublage France métropolitaine)'}
                  </span>
                </label>
              {/if}
              {#if lastTmdbResult.overview}
                <div class="tmdb-picked-overview">{lastTmdbResult.overview}</div>
              {:else if !config.tmdb_key}
                <div class="field-hint"><Icon name="light-bulb" /> Renseigne ta clé API TMDB dans Réglages pour afficher la description.</div>
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
                <button class="tmdb-item" onclick={() => pickTmdb(r)}>
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
                  <button class="tmdb-item" onclick={() => pickTmdb(r)}>
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
            <select value={target.episode ? 'tv' : 'movie'} onchange={(e) => {
              if (e.currentTarget.value === 'tv' && !target.episode) {
                target.episode = detectEpisode((sourcePath || '').split('/').pop()) || 'S01E01';
              } else if (e.currentTarget.value === 'movie') {
                target.episode = '';
              }
            }}>
              <option value="movie">Film</option>
              <option value="tv">Série</option>
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
              <button class:active={target.lang === 'auto'} onclick={() => target.lang = 'auto'}>Auto</button>
              <button class:active={target.lang === 'vf'}   onclick={() => target.lang = 'vf'}>VF</button>
              <button class:active={target.lang === 'vo'}   onclick={() => target.lang = 'vo'}>VO</button>
            </div>
          {/if}
          <div class="preview-filename-row">
            {#if filenameOverride}
              <input class="filename-input mono" type="text" bind:value={manualFilename} placeholder="nom-de-fichier.mkv" />
              <button class="btn-copy" onclick={resetFilenameOverride} title="Revenir à l'auto"><Icon name="arrow-uturn-left" /> Auto</button>
            {:else}
              <div class="preview-value mono">{previewFilename || '—'}</div>
              {#if previewFilename}
                <button class="btn-copy" onclick={startFilenameOverride} title="Modifier manuellement"><Icon name="pencil" /> Modifier</button>
              {/if}
            {/if}
            {#if effectiveFilename}
              <button class="btn-copy" onclick={copyFilename} title="Copier">{filenameCopied ? '✓ Copié' : '📋 Copier'}</button>
              <button class="btn-copy" onclick={openOutputDir} disabled={!effectiveOutputDir()} title="Ouvrir dossier de sortie"><Icon name="folder-open" /> Dossier</button>
            {/if}
          </div>
        </div>
      </div>

      <div class="actions-row">
        {#if muxing}
          <button class="btn btn-danger" onclick={stopMux}><Icon name="stop" /> Stop</button>
          <div class="progress-bar"><div class="progress-fill" style:width="{muxPercent}%"></div></div>
          <span class="mono">{muxPercent}%</span>
        {:else}
          <button class="btn btn-accent" onclick={doMux} disabled={!sourcePath || !effectiveFilename}>Muxer</button>
        {/if}
      </div>
    </div><!-- /fullscreen cible -->

    {:else if screen === 'sync'}
    <div class="fullscreen">
      <div class="card">
        <div class="card-title"><Icon name="speaker-wave" /> Synchro des Pistes Audios</div>
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
            <button class="btn-secondary" onclick={syncPickFile}>
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
                           onchange={() => syncSetRef(t.id)} />
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
                      <span style="opacity:.7;"><Icon name="clock" /> analyse…</span>
                    {:else}
                      <button class="btn-secondary"
                              onclick={() => syncDetectOne(t.id)}
                              disabled={!syncSourcePath}><Icon name="viewfinder-circle" /> Détecter</button>
                      {#if syncResults[t.id]}
                        {@const r = syncResults[t.id]}
                        <div style="font-size:.78em; opacity:.85; margin-top:4px;">
                          conf {r.confidence.toFixed(2)}
                          {#if r.drift_ms > 0} · drift {r.drift_ms} ms{/if}
                        </div>
                        {#if r.method === 'drift_linear'}
                          <div style="margin-top:4px; padding:4px 6px; background:#5a3a00; color:#ffd166; border-radius:4px; font-size:.78em;">
                            <Icon name="exclamation-triangle" /> FPS différent — resample atempo {r.tempo_factor.toFixed(6)} ({((r.tempo_factor - 1) * 100).toFixed(3)}%) sera appliqué au mux (1 génération AC3/EAC3 perdue).
                          </div>
                        {:else if r.method === 'drift_unstable'}
                          <div style="margin-top:4px; padding:4px 6px; background:#5a1f1f; color:#ffb3b3; border-radius:4px; font-size:.78em;">
                            <Icon name="exclamation-triangle" /> Drift instable — recalage non fiable, vérifier manuellement.
                          </div>
                        {:else if r.method === 'low_confidence'}
                          <div style="margin-top:4px; padding:4px 6px; background:#3a3a3a; color:#bbb; border-radius:4px; font-size:.78em;">
                            <Icon name="exclamation-triangle" /> Corrélation faible — résultat peu fiable.
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
            <Icon name="viewfinder-circle" /> « Détecter » mesure l'offset par cross-correlation (extraction PCM mono
            8 kHz via ffmpeg, ~3 min en début de film + vérif drift à 85% sur films
            &gt;20 min). « Appliquer » mux un nouveau .mkv en copiant l'audio bit-à-bit
            (AC3/EAC3 préservés).
          </p>

          <div class="actions-row" style="gap:10px;">
            <button class="btn-secondary"
                    onclick={syncDetectAll}
                    disabled={!syncSourcePath || syncRefId === null}>
              <Icon name="viewfinder-circle" /> Détecter toutes les pistes
            </button>
            {#if syncRunning}
              <div class="progress-bar"><div class="progress-fill" style:width="{syncPercent}%"></div></div>
              <span class="mono">{syncPercent}%</span>
            {:else}
              <button class="btn-primary"
                      onclick={syncApply}
                      disabled={!syncSourcePath || syncRefId === null}>
                <Icon name="check-circle" /> Appliquer le recalage
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
            <button class="btn-test" onclick={doTestTmdbKey} disabled={tmdbTest.running}>
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
            <button class="btn-test" onclick={doTestHydrackerKey} disabled={hydrackerTest.running}>
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
            <button class="btn-test" onclick={doTestUnfrKey} disabled={unfrTest.running}>
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
        <div class="card-title">Encodeur</div>
        <div class="field"><label>Ton pseudo encodeur (remplace « By GANDALF » dans le nom de piste vidéo)</label>
          <div class="field-row">
            <select bind:value={config.default_encoder} style="flex:1;">
              {#each options.video_encoders as e}<option>{e}</option>{/each}
            </select>
          </div>
          <div class="field-hint">
            Appliqué automatiquement à chaque chargement de source. Tu peux toujours le changer ponctuellement dans « Réglages piste vidéo ».
          </div>
        </div>
      </div>

      <div class="card">
        <div class="card-title">Labels audio personnalisés</div>
        <div class="field"><label>Ajouter une langue/qualité audio (format figé : « LANG VARIANTE : CODEC CANAUX »)</label>
          <div class="field-row">
            <input type="text" bind:value={newAudioLabel} placeholder="ex: BRE VO : AC3 5.1" onkeydown={(e) => e.key === 'Enter' && addCustomAudioLabel()} />
            <button class="btn-test" onclick={addCustomAudioLabel} disabled={!newAudioLabel.trim()}>+ Ajouter</button>
          </div>
          <div class="field-hint">Mergé avec les labels figés LiHDL. Visible immédiatement dans les dropdowns audio.</div>
        </div>
        {#if config.custom_audio_labels && config.custom_audio_labels.length > 0}
          <div class="custom-labels-list">
            {#each config.custom_audio_labels as lbl, i}
              <span class="custom-label-pill">
                <span class="mono">{lbl}</span>
                <button class="custom-label-remove" onclick={() => removeCustomAudioLabel(i)} title="Supprimer ce label">✕</button>
              </span>
            {/each}
          </div>
        {/if}
      </div>

      <div class="card">
        <div class="card-title">Labels sous-titres personnalisés</div>
        <div class="field"><label>Ajouter une langue de sous-titre (format figé : « LANG VARIANTE : FORMAT »)</label>
          <div class="field-row">
            <input type="text" bind:value={newSubtitleLabel} placeholder="ex: BRE Full : SRT" onkeydown={(e) => e.key === 'Enter' && addCustomSubtitleLabel()} />
            <button class="btn-test" onclick={addCustomSubtitleLabel} disabled={!newSubtitleLabel.trim()}>+ Ajouter</button>
          </div>
          <div class="field-hint">Mergé avec les labels figés LiHDL. Visible immédiatement dans les dropdowns sous-titres.</div>
        </div>
        {#if config.custom_subtitle_labels && config.custom_subtitle_labels.length > 0}
          <div class="custom-labels-list">
            {#each config.custom_subtitle_labels as lbl, i}
              <span class="custom-label-pill">
                <span class="mono">{lbl}</span>
                <button class="custom-label-remove" onclick={() => removeCustomSubtitleLabel(i)} title="Supprimer ce label">✕</button>
              </span>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Dropdowns personnalisés de la card « Réglages piste vidéo » -->
      <div class="card">
        <div class="card-title"><Icon name="adjustments-horizontal" /> Dropdowns Réglages piste vidéo (personnalisés)</div>
        <div class="field-hint" style="margin-bottom:10px;">Ajoute ou retire des valeurs dans chaque dropdown de la card. Mergé avec les valeurs par défaut, visible immédiatement.</div>
        {#each [
          { key: 'custom_video_qualities',     label: 'Qualité',       placeholder: 'ex: HDLight 10bit' },
          { key: 'custom_video_encoders',      label: 'Encodeur',      placeholder: 'ex: NewEncoder' },
          { key: 'custom_video_teams',         label: 'Team (sortie)', placeholder: 'ex: MYTEAM' },
          { key: 'custom_video_source_types',  label: 'Type source',   placeholder: 'ex: WEB-DL AMZN' },
          { key: 'custom_resolutions',         label: 'Résolution',    placeholder: 'ex: 1440p' },
          { key: 'custom_target_sources',      label: 'Source (sortie)', placeholder: 'ex: BRRip' },
          { key: 'custom_video_codecs',        label: 'Codec vidéo',   placeholder: 'ex: AV1-10b' },
        ] as grp}
          <div class="field" style="margin-top:10px;">
            <label>{grp.label}</label>
            <div class="field-row">
              <input type="text"
                value={newCustomVideoOption[grp.key] || ''}
                placeholder={grp.placeholder}
                oninput={(e) => { newCustomVideoOption[grp.key] = e.target.value; newCustomVideoOption = {...newCustomVideoOption}; }}
                onkeydown={(e) => e.key === 'Enter' && addCustomVideoOption(grp.key, grp.label)} />
              <button class="btn-test" onclick={() => addCustomVideoOption(grp.key, grp.label)}
                disabled={!(newCustomVideoOption[grp.key] || '').trim()}>+ Ajouter</button>
            </div>
            {#if config[grp.key] && config[grp.key].length > 0}
              <div class="custom-labels-list">
                {#each config[grp.key] as val, i}
                  <span class="custom-label-pill">
                    <span class="mono">{val}</span>
                    <button class="custom-label-remove" onclick={() => removeCustomVideoOption(grp.key, i, grp.label)} title="Supprimer">✕</button>
                  </span>
                {/each}
              </div>
            {/if}
          </div>
        {/each}
      </div>

      <div class="card">
        <div class="card-title">Dossiers de sortie</div>
        <div class="field"><label><Icon name="bolt" /> MUX LiHDL (films)</label>
          <div class="field-row">
            <input type="text" bind:value={config.output_dir_lihdl} placeholder="/Users/…/Mux/LiHDL" readonly />
            <button class="btn-test" onclick={pickOutputDirLihdl}>Choisir…</button>
            <button class="btn-test" onclick={() => config.output_dir_lihdl && OpenFolder(config.output_dir_lihdl)} disabled={!config.output_dir_lihdl} title="Ouvrir dans le Finder"><Icon name="folder-open" /></button>
          </div>
        </div>
        <div class="field"><label><Icon name="film" /> MUX CUSTOM PSA SERIES</label>
          <div class="field-row">
            <input type="text" bind:value={config.output_dir_psa} placeholder="/Users/…/Mux/PSA" readonly />
            <button class="btn-test" onclick={pickOutputDirPSA}>Choisir…</button>
            <button class="btn-test" onclick={() => config.output_dir_psa && OpenFolder(config.output_dir_psa)} disabled={!config.output_dir_psa} title="Ouvrir dans le Finder"><Icon name="folder-open" /></button>
          </div>
        </div>
        <div class="field"><label>Dossier fallback (si l'un des 2 ci-dessus est vide)</label>
          <div class="field-row">
            <input type="text" bind:value={config.output_dir} placeholder="/Users/…/Mux" readonly />
            <button class="btn-test" onclick={pickOutputDir}>Choisir…</button>
            <button class="btn-test" onclick={() => config.output_dir && OpenFolder(config.output_dir)} disabled={!config.output_dir} title="Ouvrir dans le Finder"><Icon name="folder-open" /></button>
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
            <button class="btn-test" onclick={doTestLanguageToolKey} disabled={ltTest.running}>
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
                <button class="btn btn-ghost btn-tiny" onclick={() => removeCustomDictEntry(e.wrong)} title="Supprimer">✕</button>
              </div>
            {/each}
          {/if}
        </div>
        <div style="margin-top:8px;display:flex;gap:8px;">
          <button class="btn btn-ghost btn-tiny" onclick={() => { newDictWrong=''; newDictRight=''; showAddDictModal = true; }}>+ Ajouter</button>
          <button class="btn btn-ghost btn-tiny" onclick={loadCustomDict}><Icon name="arrow-path" /> Recharger</button>
        </div>
      </div>

      <div class="card">
        <div class="card-title">Index Discord (admin)</div>
        <div class="field-hint" style="margin-bottom:8px;">
          Permet aux users de l'app d'ouvrir le post Discord d'un film de la Team via un bouton <b><Icon name="arrow-top-right-on-square" /> Discord</b> dans le header film-bar.
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
          <div style="font-size:11.5px;font-weight:700;color:var(--text2);text-transform:uppercase;letter-spacing:0.6px;margin-bottom:8px;"><Icon name="arrow-up-tray" /> Push GitHub direct (admin) — verrouillé</div>
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
          <button class="btn btn-accent" onclick={doDiscordScan} disabled={discordScanRunning || !config.discord_bot_token || !config.discord_forum_id}>
            {discordScanRunning ? '⟳ Scan en cours…' : '🔄 Mettre à jour l\'index'}
          </button>
          <button class="btn btn-ghost" onclick={doDiscordCopy} disabled={discordScanRunning}>
            {discordCopyOk ? '✓ Copié' : '📋 Copier le JSON'}
          </button>
          <button class="btn btn-accent" onclick={doDiscordPushGitHub} disabled={discordScanRunning || githubPushing || !config.github_token || !config.github_repo}>
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
        <button class="btn btn-accent" onclick={saveSettings}>Enregistrer</button>
      </div>
    </div><!-- /fullscreen reglages -->
    {/if}

  </main>

  {#if showLTReview}
    <div class="lt-review-overlay" onclick={() => showLTReview = false} onkeydown={(e) => e.key === 'Escape' && (showLTReview = false)} role="presentation">
      <div class="lt-review-modal" onclick={stopPropagation(bubble('click'))} role="dialog" aria-modal="true" aria-label="Lignes à vérifier (LanguageTool)">
        <div class="lt-review-header">
          <h3><Icon name="magnifying-glass" /> Lignes à vérifier — LanguageTool ({ocrProgress.lt_needs_review})</h3>
          <button class="btn btn-ghost btn-tiny" onclick={() => showLTReview = false}>✕</button>
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
                <div class="lt-review-msg"><Icon name="chat-bubble-left-ellipsis" /> {m.message}</div>
                {#if !(ltReviewState[idx] && ltReviewState[idx].resolved)}
                  {#if m.suggestions && m.suggestions.length > 0}
                    <div class="lt-review-sugg">
                      Suggestions cliquables :
                      {#each m.suggestions as s}
                        <button class="lt-sugg-pill clickable"
                                disabled={ltReviewState[idx] && ltReviewState[idx].busy}
                                onclick={() => applyReviewFix(idx, s)}>
                          {s}
                        </button>
                      {/each}
                    </div>
                  {/if}
                  <div class="lt-review-custom">
                    <input type="text" class="lt-custom-input" placeholder="Correction libre…"
                           bind:value={ltReviewState[idx].customText}
                           onkeydown={(e) => e.key === 'Enter' && applyReviewFix(idx, ltReviewState[idx].customText)}
                           disabled={ltReviewState[idx] && ltReviewState[idx].busy} />
                    <button class="btn btn-accent btn-tiny"
                            disabled={!ltReviewState[idx] || !ltReviewState[idx].customText || !ltReviewState[idx].customText.trim() || ltReviewState[idx].busy}
                            onclick={() => applyReviewFix(idx, ltReviewState[idx].customText)}>
                      {ltReviewState[idx] && ltReviewState[idx].busy ? '…' : 'Appliquer'}
                    </button>
                    <button class="btn btn-ghost btn-tiny"
                            onclick={() => ignoreReviewMatch(idx)}
                            title="Ignorer ce match (pas une vraie erreur)">
                      Ignorer
                    </button>
                  </div>
                  {#if ltReviewState[idx] && ltReviewState[idx].error}
                    <div class="lt-review-err"><Icon name="exclamation-triangle" /> {ltReviewState[idx].error}</div>
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
          <button class="btn btn-accent" onclick={() => showLTReview = false}>Fermer</button>
        </div>
      </div>
    </div>
  {/if}

  {#if showOSModal}
    <div class="lt-review-overlay" onclick={() => showOSModal = false} onkeydown={(e) => e.key === 'Escape' && (showOSModal = false)} role="presentation">
      <div class="lt-review-modal" onclick={stopPropagation(bubble('click'))} role="dialog" aria-modal="true" aria-label="OpenSubtitles">
        <div class="lt-review-header">
          <h3><Icon name="magnifying-glass" /> OpenSubtitles — recherche SRT</h3>
          <button class="btn btn-ghost btn-tiny" onclick={() => showOSModal = false}>✕</button>
        </div>
        <div class="lt-review-body">
          {#if !config.opensubtitles_api_key}
            <div class="empty-hint" style="padding:12px;border:1px solid var(--orange);border-radius:8px;background:rgba(255,181,71,0.08);">
              <Icon name="exclamation-triangle" /> Clé API OpenSubtitles manquante.
              <button class="btn btn-ghost btn-tiny" style="margin-left:8px" onclick={() => { showOSModal = false; screen = 'reglages'; }}>
                Configurer dans Settings
              </button>
            </div>
          {/if}
          <div class="os-search-row">
            <div class="field" style="flex:2">
              <label>Titre</label>
              <input type="text" bind:value={osQuery} placeholder="ex: Inception" onkeydown={(e) => e.key === 'Enter' && searchOpenSubtitles()} />
            </div>
            <div class="field" style="flex:1">
              <label>Année</label>
              <input type="text" bind:value={osYear} placeholder="ex: 2010" inputmode="numeric" />
            </div>
            <div class="field" style="flex:1">
              <label>Langues</label>
              <input type="text" bind:value={osLang} placeholder="fr,en" />
            </div>
            <button class="btn btn-accent" onclick={searchOpenSubtitles} disabled={osSearching || !config.opensubtitles_api_key}>
              {osSearching ? '⏳' : 'Chercher'}
            </button>
          </div>
          {#if osError}
            <div class="lt-review-err"><Icon name="exclamation-triangle" /> {osError}</div>
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
                      <button class="btn btn-ghost btn-tiny" onclick={() => OpenURL(r.url)}><Icon name="arrow-top-right-on-square" /> Voir</button>
                    {/if}
                    <button class="btn btn-accent btn-tiny" onclick={() => downloadOSResult(r)} disabled={osDownloading === r.id}>
                      {osDownloading === r.id ? '⏳ Téléchargement…' : '⬇ Utiliser'}
                    </button>
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
        <div class="lt-review-footer">
          <button class="btn btn-ghost" onclick={() => showOSModal = false}>Fermer</button>
        </div>
      </div>
    </div>
  {/if}

  {#if showAddDictModal}
    <div class="lt-review-overlay" onclick={() => showAddDictModal = false} onkeydown={(e) => e.key === 'Escape' && (showAddDictModal = false)} role="presentation">
      <div class="lt-review-modal" onclick={stopPropagation(bubble('click'))} role="dialog" aria-modal="true" aria-label="Ajouter une entrée au dictionnaire OCR">
        <div class="lt-review-header">
          <h3>+ Ajouter une entrée au dictionnaire OCR</h3>
          <button class="btn btn-ghost btn-tiny" onclick={() => showAddDictModal = false}>✕</button>
        </div>
        <div class="lt-review-body">
          <div class="field">
            <label>Texte OCR fautif (wrong)</label>
            <input type="text" bind:value={newDictWrong} placeholder="ex: Charli xex" />
          </div>
          <div class="field">
            <label>Correction (right)</label>
            <input type="text" bind:value={newDictRight} placeholder="ex: Charli XCX" onkeydown={(e) => e.key === 'Enter' && addCustomDictEntry()} />
          </div>
        </div>
        <div class="lt-review-footer">
          <button class="btn btn-ghost" onclick={() => showAddDictModal = false}>Annuler</button>
          <button class="btn btn-accent" onclick={addCustomDictEntry} disabled={dictBusy || !newDictWrong.trim() || !newDictRight.trim()}>
            {dictBusy ? '…' : 'Ajouter'}
          </button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Le journal est rendu dans la col droite quand sourcePath && tmdbValidated && screen === 'source'.
       Sinon on garde un placeholder vide pour ne pas casser appendLog (logEl est lié dans la card). -->
</div><!-- /app -->

