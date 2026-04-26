<script>
  import { onMount } from 'svelte';
  import banner from './assets/images/banner.png';
  import logo from './assets/images/logo.png';
  import {
    GetVersion, GetConfig, SaveConfig, GetLihdlOptions,
    SelectMkvFile, SelectSubFiles, SelectAudioFiles, SelectOutputDir, LocateMkvmerge, OpenFolder, SearchTmdbTV, SearchTmdbMovie, AnalyzeMkvSecondary, MoveToTrash, MoveDirContentsToTrash, LookupHydrackerURL, TestHydrackerKey, TestUnfrKey, OpenURL, GetMkvBasicInfo, ExtractRefSubs, ExtractFRAudios, CheckSubsSync,
    AnalyzeMkv, SearchTmdb, TestTmdbKey, FileSize,
    Mux, CancelMux,
    CheckUpdate, InstallUpdate,
    ListAudioTracksForSync, MuxAudioSync, DetectAudioOffset,
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

  // Dropdowns pour le filename (ordre : résolution.source).
  const RESOLUTION_OPTIONS = ['720p', '1080p', '2160p'];
  const TARGET_SOURCE_OPTIONS = ['HDLight', 'WEBLight', 'WEB-DL', 'WEBRip'];
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
    // Auto-extraction des sous-titres FR/ENG en SRT (texte uniquement, exclut PGS/VobSub).
    // Si la source LiHDL est chargée, détecte aussi le décalage et l'applique au mux.
    srtExtracting = true;
    srtExtractionResult = '';
    srtPhase = '';
    srtAssConverted = false;
    srtPercent = 0;
    try {
      appendLog('⏳ Extraction des sous-titres FR/ENG compatibles…');
      const subs = await ExtractRefSubs(p, sourcePath || '');
      if (subs && subs.length > 0) {
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
    subSyncChecking = true;
    subSyncResults = [];
    subSyncPercent = 0;
    subSyncCurrentName = '';
    subSyncAppliedMsg = '';
    try {
      appendLog(`🔎 Sync alass de ${srtSubs.length} sous-titre(s) vs source…`);
      const reqs = srtSubs.map(s => ({ path: s.path }));
      subSyncResults = await CheckSubsSync(reqs, sourcePath);
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
      const extractionsRaw = await ExtractFRAudios(referencePath, !!extractFRVFF, !!extractFRVFQ, !!extractENG, sourcePath);
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
    const savedStatus = autoMuxStatus;
    resetAll();
    if (preserveAutoMuxStatus) {
      autoMuxStatus = savedStatus;
    }
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

  // Source secondaire (SUPPLY/FW) — récupère uniquement audios + subs.
  let secondaryPath = '';
  let secondaryTracks = [];      // tracks audio + subs analysées
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
    const filename = p.split('/').pop() || '';
    AnalyzeMkvSecondary(p);
    appendLog('🔍 Analyse secondaire : ' + filename);

    // Auto-fill depuis le nom de fichier SUPPLY/FW.
    const supply = parseSupplyInfo(filename);
    const psaName = (sourcePath || '').split('/').pop() || '';
    const psa = parsePsaSourceInfo(psaName);
    // sourceType combiné PSA + SUPPLY (ex: "WEBRip PSA Audio Supply").
    if (psa.isPSA && psa.source && supply.team) {
      videoChoice.sourceType = `${psa.source} PSA Audio ${supply.team}`;
      appendLog(`✓ sourceType : ${videoChoice.sourceType}`);
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
        // Tous les subs sont gardés. Default + Forced cochés UNIQUEMENT
        // pour "FR VFF Forced" ou "FR Forced" (générique).
        keepFlag = true;
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
    });
    // Marque la 1ère piste audio FR comme default.
    const firstFr = secondarySelected.find(t => t.type === 'audio' && t.language === 'fre');
    if (firstFr) firstFr.default = true;

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
        if (psa.videoCodec) target.video_codec = psa.videoCodec;
        appendLog(`✓ PSA détecté : ${psa.source || '?'} ${psa.videoCodec || '?'} → Custom PSA / GANDALF`);
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
    // Norme LiHDL : si une piste FR VFQ est présente parmi les audios, la 2e piste
    // FR doit rester FR VFF (pas FR VFi). On désactive donc le toggle VFi auto.
    const hasVFQ = tracks.some(t => t.type === 'audio' && /^FR VFQ/.test(t.label || ''));
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
    // Série : tout avant SxxExx
    const ms = /^(.+?)\.S\d{1,2}E\d{1,3}\b/i.exec(name);
    if (ms) name = ms[1];
    else {
      // Film : tout avant l'année (4 chiffres 19xx ou 20xx)
      const my = /^(.+?)\.(?:19|20)\d{2}\b/.exec(name);
      if (my) name = my[1];
    }
    // Strip un éventuel ".2024" en suffixe (cas: Title.2024.S01E15...)
    name = name.replace(/\.(?:19|20)\d{2}$/, '');
    return name.trim();
  }

  async function maybeAutoFillTitle(path) {
    const filename = path.split('/').pop() || '';
    const cleanedDotted = cleanQueryFromFilename(filename);          // "The.Boys" — pour l'index
    const cleanedSpaces = cleanedDotted.replace(/\./g, ' ').trim();  // "The Boys" — pour l'API TMDB
    const isSeries = /\bS\d{1,2}E\d{1,3}\b/i.test(filename);
    const forceTV = isSeries || muxMode === 'psa';
    tmdbQuery = cleanedDotted;
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
      tmdbResults = r || [];
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
      tmdbResults = r || [];
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

  let tmdbTest = { running: false, ok: null, message: '' };
  let hydrackerTest = { running: false, ok: null, message: '' };
  let unfrTest = { running: false, ok: null, message: '' };

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
      } catch (e) {
        appendLog('❌ secondary:tracks parse : ' + String(e));
      }
    });
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
      <button class="reset-btn" on:click={resetAll} disabled={muxing} title="Réinitialiser la session">
        <span>↻</span>
        <span class="reset-label">RESET</span>
      </button>
      <button class="settings-btn" on:click={() => screen = 'reglages'}>
        <span class="gear">⚙</span>
        <span class="settings-label">SETTINGS</span>
      </button>
    </div>
  </header>

  <nav class="mux-mode-tabs">
    <button class:active={muxMode === 'lihdl'} on:click={() => switchMuxMode('lihdl')}>⚡ MUX LiHDL</button>
    <button class:active={muxMode === 'psa'}   on:click={() => switchMuxMode('psa')}>🎬 CUSTOM PSA SERIES</button>
  </nav>

  <nav class="tabs">
    <button class:active={screen === 'source'}   on:click={() => screen = 'source'}>Source</button>
    <button class:active={screen === 'cible'}    on:click={() => screen = 'cible'}>Cible</button>
    <button class:active={screen === 'sync'}     on:click={() => screen = 'sync'}>Synchro Audios</button>
    <button class:active={screen === 'reglages'} on:click={() => screen = 'reglages'}>Réglages</button>
  </nav>

  <section class="content">
    {#if screen === 'source'}
      <!-- Bandeau persistant : statut du dernier MUX AUTO (succès / erreur).
           Reste visible après l'auto-reset jusqu'au prochain mux ou clic ↻ RESET. -->
      {#if autoMuxStatus === 'success' && !muxing}
        <div class="card mux-status-banner success">
          <div class="auto-status done">✅ Mux terminé avec succès — fichier prêt dans ton dossier de sortie</div>
        </div>
      {:else if autoMuxStatus === 'error' && !muxing}
        <div class="card mux-status-banner error">
          <div class="auto-status error">❌ Mux échoué — vérifie les logs en bas</div>
        </div>
      {/if}

      <!-- Card unifiée : PSA + SUPPLY/FW (en mode PSA) ou seul (en mode LiHDL) -->
      <div class="card sources-card drop-target" style:--wails-drop-target="drop">
        <div class="section-title">Sources</div>

        <!-- Ligne ① PSA -->
        <div class="source-row">
          <div class="source-info">
            <div class="source-label">
              <span class="source-num">①</span>
              {muxMode === 'psa' ? 'Source PSA' : 'Source encodée'}
              <span class="source-hint">(vidéo gardée)</span>
            </div>
            {#if sourcePath}
              <div class="source-filename mono">{sourcePath.split('/').pop()}</div>
            {:else}
              <div class="source-empty">— Aucun fichier sélectionné —</div>
            {/if}
          </div>
          <button class="btn-primary" on:click={pickMkvDialog}>
            {sourcePath ? 'Changer' : (muxMode === 'psa' ? 'Choisir un fichier PSA' : 'Choisir un fichier')}
          </button>
        </div>

        <!-- Ligne ② SUPPLY/FW (mode PSA uniquement) -->
        {#if muxMode === 'psa'}
          <div class="source-row secondary">
            <div class="source-info">
              <div class="source-label">
                <span class="source-num">②</span>
                Source SUPPLY / FW
                <span class="source-hint">(audios + subs)</span>
              </div>
              {#if secondaryPath}
                <div class="source-filename mono">{secondaryPath.split('/').pop()}</div>
                <div class="source-meta">{secondaryTracks.length} piste(s) audio/sub détectée(s)</div>
              {:else}
                <div class="source-empty">— Aucun fichier sélectionné —</div>
              {/if}
            </div>
            <button class="btn-primary" on:click={pickSecondaryDialog}>
              {secondaryPath ? 'Changer' : 'Choisir un fichier SUPPLY/FW'}
            </button>
          </div>
        {/if}

        <!-- Ligne ② Sous-titres externes (mode LiHDL uniquement) -->
        {#if muxMode === 'lihdl'}
          <div class="source-row secondary">
            <div class="source-info">
              <div class="source-label">
                <span class="source-num">②</span>
                Sous-titres
                <span class="source-hint">(SRT/PGS/ASS · optionnel)</span>
              </div>
              {#if externalSubs.length > 0}
                <div class="source-filename mono">
                  {externalSubs.length} fichier(s) : {basename(externalSubs[0].path)}{externalSubs.length > 1 ? ` +${externalSubs.length - 1} autre(s)` : ''}
                </div>
              {:else}
                <div class="source-empty">— Aucun sous-titre chargé —</div>
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
              <button class="btn-primary" on:click={pickSubsDialog} disabled={srtExtracting}>
                {externalSubs.length > 0 ? '+ Ajouter' : 'Choisir des fichiers'}
              </button>
              {#if externalSubs.length > 0}
                <button class="btn-icon" on:click={() => externalSubs = []} title="Vider la liste" disabled={srtExtracting}>✕</button>
              {/if}
            </div>
          </div>

          <!-- Ligne ③ Source de référence : compat durée/FPS + extraction SRT (auto) + extraction FR audio (toggles) avec sync auto -->
          {#if showReferenceBar}
            {@const compat = checkCompat(sourceMkvInfo, referenceMkvInfo)}
            <div class="source-row secondary">
              <div class="source-info">
                <div class="source-label">
                  <span class="source-num">③</span>
                  Source de référence
                  <span class="source-hint">(extraction SRT auto + FR audio sur demande, sync auto)</span>
                </div>
                {#if referencePath}
                  <div class="source-filename mono">{basename(referencePath)}</div>
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
                <button class="btn-primary" on:click={pickReferenceDialog} disabled={frAudioExtracting}>
                  {referencePath ? 'Changer' : 'Choisir un fichier'}
                </button>
                <button class="btn-icon" on:click={() => { clearReference(); showReferenceBar = false; }} title="Retirer la source de référence" disabled={frAudioExtracting}>✕</button>
              </div>
            </div>
          {:else}
            <button class="source-row-placeholder" on:click={() => { showReferenceBar = true; }}>+ Ajouter source de référence (SRT auto + FR audio sur demande)</button>
          {/if}

        {/if}
      </div>

      <!-- Actions : barre d'automatisation (mode PSA, après chargement des 2) -->
      <!-- Barre d'actions mode LiHDL : MUX MANUEL / MUX AUTO / Suivant -->
      {#if muxMode === 'lihdl' && sourcePath && tracks.length > 0}
        <div class="card automate-bar">
          <!-- Poster TMDB + identité du film, pour vérification visuelle avant MUX AUTO -->
          {#if lastTmdbResult && lastTmdbResult.poster_url}
            <div class="tmdb-preview">
              <img src={lastTmdbResult.poster_url} alt="Poster {lastTmdbResult.titre_fr || lastTmdbResult.titre_vo}" class="tmdb-preview-poster" loading="lazy" />
              <div class="tmdb-preview-info">
                <div class="tmdb-preview-title">{lastTmdbResult.titre_fr || lastTmdbResult.titre_vo}</div>
                {#if lastTmdbResult.titre_vo && lastTmdbResult.titre_fr && lastTmdbResult.titre_vo !== lastTmdbResult.titre_fr}
                  <div class="tmdb-preview-vo">VO : {lastTmdbResult.titre_vo}</div>
                {/if}
                <div class="tmdb-preview-meta">
                  {#if lastTmdbResult.annee_fr}<span>📅 {lastTmdbResult.annee_fr}</span>{/if}
                  {#if lastTmdbResult.duree}<span>⏱️ {lastTmdbResult.duree}</span>{/if}
                  {#if lastTmdbResult.note > 0}<span>★ {lastTmdbResult.note}</span>{/if}
                  {#if lastTmdbResult.original_language}<span>🌐 {lastTmdbResult.original_language.toUpperCase()}</span>{/if}
                </div>
                {#if lastTmdbResult.tmdb_id}
                  <button class="vfq-link tmdb-preview-link" type="button" on:click={() => OpenURL(`https://www.themoviedb.org/movie/${lastTmdbResult.tmdb_id}`)}>↗ Vérifier sur TMDB (ID {lastTmdbResult.tmdb_id})</button>
                {/if}
                <!-- Override : si ce n'est pas le bon film, taper un ID TMDB manuellement -->
                <div class="tmdb-preview-override">
                  <span class="tmdb-preview-override-label">Pas le bon film ? Forcer l'ID :</span>
                  <input
                    type="text"
                    class="tmdb-preview-override-input"
                    bind:value={tmdbIdQuery}
                    placeholder="ex: 12345"
                    inputmode="numeric"
                    pattern="[0-9]*"
                    on:keydown={(e) => e.key === 'Enter' && searchTmdbById()}
                  />
                  <button class="btn-tiny tmdb-preview-override-btn" on:click={searchTmdbById} disabled={tmdbSearching || !tmdbIdQuery}>
                    {tmdbSearching ? '⏳' : '↻ Forcer'}
                  </button>
                </div>
              </div>
            </div>
          {/if}
          <div class="actions-row" style:gap="8px">
            <button class="btn-primary" on:click={() => automateLihdl()} disabled={muxing}>⚡ MUX MANUEL</button>
            <button class="btn-primary btn-auto" on:click={muxAutoLihdl} disabled={muxing}>🚀 MUX AUTO</button>
            <button class="btn-primary" on:click={() => screen = 'cible'} disabled={muxing}>Suivant → Cible</button>
          </div>
          <!-- Recherche sous-titres SRT : liens externes (via Wails BrowserOpenURL) -->
          {#if lastTmdbResult && lastTmdbResult.tmdb_id}
            <div class="vfq-toggle" style:margin-top="8px">
              <span class="srt-label">🔍 Recherche sous-titres SRT</span>
              <button class="vfq-link" type="button" on:click={() => OpenURL(hydrackerURL || `https://hydracker.com/titles?search=${encodeURIComponent(lastTmdbResult.titre_fr || lastTmdbResult.titre_vo || '')}`)}>vérifier sur Hydra ↗</button>
              <button class="vfq-link" type="button" on:click={() => OpenURL(`https://unfr.pw/?d=fiche&movieid=${lastTmdbResult.tmdb_id}`)}>vérifier sur UNFR ↗</button>
            </div>
          {/if}
          <label class="vfq-toggle" style:margin-top="8px">
            <input type="checkbox" bind:checked={useVFi} on:change={applyVFiSwap} />
            <span class:vfq-yes={useVFi} class:vfq-no={!useVFi}>
              {useVFi ? '✓ FR VFi (doublage international)' : '☐ FR VFF (doublage France métropolitaine)'}
            </span>
            {#if lastTmdbResult && lastTmdbResult.tmdb_id}
              <button class="vfq-link" type="button" on:click={() => OpenURL(`https://www.themoviedb.org/movie/${lastTmdbResult.tmdb_id}/translations`)}>vérifier sur TMDB ↗</button>
              <button class="vfq-link" type="button" on:click={() => OpenURL(`https://fr.wikipedia.org/w/index.php?title=Special:Search&go=Go&search=${encodeURIComponent(lastTmdbResult.titre_fr || lastTmdbResult.titre_vo || '')}`)}>vérifier sur Wikipédia ↗</button>
            {/if}
          </label>

          {#if muxing}
            <div class="auto-progress">
              <div class="auto-progress-row">
                <button class="btn-cancel btn-stop" on:click={stopMux}>⏹ Stop</button>
                <div class="progress-bar"><div class="progress-fill" style:width="{muxPercent}%"></div></div>
                <span class="mono">{muxPercent}%</span>
              </div>
              <div class="auto-status">Mux en cours…</div>
            </div>
          {:else if autoMuxStatus === 'success'}
            <div class="auto-progress">
              <div class="progress-bar done"><div class="progress-fill done-fill" style:width="100%"></div></div>
              <div class="auto-status done">✅ Mux terminé avec succès — fichier prêt dans ton dossier de sortie</div>
            </div>
          {:else if autoMuxStatus === 'error'}
            <div class="auto-progress">
              <div class="progress-bar error"><div class="progress-fill error-fill" style:width="100%"></div></div>
              <div class="auto-status error">❌ Mux échoué — vérifie les logs en bas</div>
            </div>
          {/if}
        </div>
      {/if}

      {#if muxMode === 'psa' && secondaryPath && secondaryTracks.length > 0}
        <div class="card automate-bar">
          <div class="actions-row" style:gap="8px">
            <button class="btn-primary" on:click={automate} disabled={muxing || !sourcePath}>⚡ MUX MANUEL</button>
            <button class="btn-primary btn-auto" on:click={muxAuto} disabled={muxing || !sourcePath}>🚀 MUX AUTO</button>
            <button class="btn-primary" on:click={() => screen = 'cible'} disabled={muxing}>Suivant → Cible</button>
          </div>

          {#if muxing}
            <div class="auto-progress">
              <div class="auto-progress-row">
                <button class="btn-cancel btn-stop" on:click={stopMux}>⏹ Stop</button>
                <div class="progress-bar"><div class="progress-fill" style:width="{muxPercent}%"></div></div>
                <span class="mono">{muxPercent}%</span>
              </div>
              <div class="auto-status">Mux en cours…</div>
            </div>
          {:else if autoMuxStatus === 'success'}
            <div class="auto-progress">
              <div class="progress-bar done"><div class="progress-fill done-fill" style:width="100%"></div></div>
              <div class="auto-status done">✅ Mux terminé avec succès — fichier prêt dans ton dossier de sortie</div>
            </div>
          {:else if autoMuxStatus === 'error'}
            <div class="auto-progress">
              <div class="progress-bar error"><div class="progress-fill error-fill" style:width="100%"></div></div>
              <div class="auto-status error">❌ Mux échoué — vérifie les logs en bas</div>
            </div>
          {:else if secondarySelected.length > 0}
            <div class="field-hint" style:margin-top="8px">
              ✓ {secondarySelected.filter(t=>t.type==='audio').length} audio(s) + {secondarySelected.filter(t=>t.type==='subtitles').length} sub(s) prêts à muxer
            </div>
          {/if}
        </div>
      {/if}

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
                  <select bind:value={videoChoice.sourceType} on:change={onSourceTypeChange}>
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
          {#if tracks.some(t => t.type === 'audio') || externalAudios.length > 0 || secondarySelected.some(t => t.type === 'audio')}
            {@const internalAudios = tracks.filter(t => t.type === 'audio')}
            {@const secondaryAudios = secondarySelected.filter(t => t.type === 'audio')}
            {@const mergedAudios = [
              ...internalAudios.map((t, i) => ({ kind: 'internal', idx: i, ref: t, order: t.order ?? 0 })),
              ...externalAudios.map((a, i) => ({ kind: 'external', idx: i, ref: a, order: a.order ?? 0 })),
              ...secondaryAudios.map((s, i) => ({ kind: 'secondary', idx: i, ref: s, order: s.order ?? 0 })),
            ].sort((a, b) => a.order - b.order)}
            {#each mergedAudios as item (item.kind + '-' + item.idx)}
              <div class="track-row" class:dropped={!item.ref.keep}>
                <div class="track-meta">
                  {#if item.kind === 'internal'}
                    <span class="badge audio">AUDIO</span>
                    <span class="mono">#{item.ref.id} · {item.ref.codec} · {item.ref.lang || '??'} · {item.ref.channels || '?'}ch</span>
                    {#if item.ref.name}<span class="track-current">« {item.ref.name} »</span>{/if}
                  {:else if item.kind === 'secondary'}
                    <span class="badge audio-ext">SUPPLY/FW</span>
                    <span class="mono">#{item.ref.id} · {item.ref.codec || ''} · {item.ref.language || '??'}</span>
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
                    {:else if item.kind === 'secondary'}
                      <button class="btn-arrow danger" title="Retirer du SUPPLY" on:click={() => removeSecondaryTrack(item.idx, 'audio')}>✕</button>
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
          {#if tracks.some(t => t.type === 'subtitles') || externalSubs.length > 0 || secondarySelected.some(t => t.type === 'subtitles')}
            {@const internalSubs = tracks.filter(t => t.type === 'subtitles')}
            {@const secondarySubs = secondarySelected.filter(t => t.type === 'subtitles')}
            {@const mergedSubs = [
              ...internalSubs.map((t, i) => ({ kind: 'internal', idx: i, ref: t, order: t.order ?? 0 })),
              ...externalSubs.map((s, i) => ({ kind: 'external', idx: i, ref: s, order: s.order ?? 0 })),
              ...secondarySubs.map((s, i) => ({ kind: 'secondary', idx: i, ref: s, order: s.order ?? 0 })),
            ].sort((a, b) => a.order - b.order)}
            {#each mergedSubs as item (item.kind + '-' + item.idx)}
              <div class="track-row">
                <div class="track-meta">
                  {#if item.kind === 'internal'}
                    <span class="badge subs">SUBS</span>
                    <span class="mono">#{item.ref.id} · {item.ref.codec} · {item.ref.lang || '??'}</span>
                    {#if item.ref.name}<span class="track-current">« {item.ref.name} »</span>{/if}
                  {:else if item.kind === 'secondary'}
                    <span class="badge subs-ext">SUPPLY/FW</span>
                    <span class="mono">#{item.ref.id} · {item.ref.codec || ''} · {item.ref.language || '??'}</span>
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
                    {:else if item.kind === 'secondary'}
                      <button class="btn-arrow danger" title="Retirer du SUPPLY" on:click={() => removeSecondaryTrack(item.idx, 'subtitles')}>✕</button>
                    {:else}
                      <button class="btn-arrow danger" title="Supprimer" on:click={() => removeExternalSub(item.idx)}>✕</button>
                    {/if}
                  </div>
                </div>
                <div class="track-controls">
                  <select bind:value={item.ref.label} on:change={onSubLabelChange}>
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
          <input type="text" bind:value={tmdbQuery} placeholder={tmdbMode === 'tv' ? 'Titre de série…' : 'Titre du film…'} on:keydown={(e) => e.key === 'Enter' && searchTmdb()} />
          <button class="btn-primary" on:click={searchTmdb} disabled={tmdbSearching}>{tmdbSearching ? '…' : 'Chercher par titre'}</button>
        </div>
        <div class="field-row" style:margin-top="6px">
          <input type="text" bind:value={tmdbIdQuery} placeholder="ID TMDB numérique (ex: 12345)…" inputmode="numeric" pattern="[0-9]*" on:keydown={(e) => e.key === 'Enter' && searchTmdbById()} />
          <button class="btn-primary" on:click={searchTmdbById} disabled={tmdbSearching}>{tmdbSearching ? '…' : 'Chercher par ID'}</button>
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
        <div class="section-title">Titre cible</div>
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
          <button class="btn-cancel btn-stop" on:click={stopMux}>⏹ Stop</button>
          <div class="progress-bar"><div class="progress-fill" style:width="{muxPercent}%"></div></div>
          <span class="mono">{muxPercent}%</span>
        {:else}
          <button class="btn-primary" on:click={doMux} disabled={!sourcePath || !effectiveFilename}>Muxer</button>
        {/if}
      </div>

    {:else if screen === 'sync'}
      <div class="card">
        <div class="section-title">🔊 Synchro des Pistes Audios</div>
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

    {:else if screen === 'reglages'}
      <div class="card">
        <div class="section-title">TMDB</div>
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
        <div class="field"><label>Index TMDB primaire (proxy uklm)</label>
          <input type="text" bind:value={config.serveurperso_url} placeholder="https://tmdb.uklm.xyz/search.php" />
          <div class="field-hint">
            Index principal — par défaut <b>tmdb.uklm.xyz</b> (réécrit, plus complet).
          </div>
        </div>
        <div class="field"><label>Index TMDB fallback (serveurperso query)</label>
          <input type="text" bind:value={config.fallback_index} placeholder="https://www.serveurperso.com/stats/search.php" />
          <div class="field-hint">
            Endpoint <b>?query=</b> de serveurperso. Interrogé automatiquement si le primaire renvoie 0 résultat.
          </div>
        </div>
      </div>

      <div class="card">
        <div class="section-title">Recherche sous-titres SRT</div>
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
        <div class="section-title">Dossiers de sortie</div>
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
        <div class="section-title">MKVToolNix</div>
        <div class="field"><label>Chemin mkvmerge (optionnel — auto-détecté sinon)</label>
          <input type="text" bind:value={config.mkvmerge_path} placeholder="/opt/homebrew/bin/mkvmerge" />
        </div>
        <div class="field-hint">Détecté actuellement : <b class="mono">{mkvmergePath || 'introuvable'}</b></div>
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

  .reset-btn {
    display: inline-flex; align-items: center; gap: 6px;
    padding: 7px 13px; border-radius: 10px;
    border: 1px solid var(--border);
    background: rgba(255,255,255,0.04);
    color: var(--text2);
    font: inherit; font-size: 11px; font-weight: 700;
    letter-spacing: 1.5px;
    cursor: pointer; transition: all 150ms;
  }
  .reset-btn:hover:not(:disabled) {
    background: rgba(239,68,68,0.18);
    border-color: rgba(239,68,68,0.5);
    color: rgb(255,180,180);
  }
  .reset-btn:hover:not(:disabled) span:first-child { transform: rotate(-180deg); }
  .reset-btn span:first-child { transition: transform 200ms; font-size: 14px; display: inline-block; }
  .reset-btn:disabled { opacity: 0.4; cursor: not-allowed; }
  .reset-label { font-size: 11px; }

  .mux-mode-tabs {
    display: flex; gap: 8px; padding: 8px 20px;
    background: var(--bg-tint);
    border-bottom: 1px solid var(--border);
  }
  .mux-mode-tabs button {
    flex: 1; padding: 10px 18px; border: 1px solid var(--border); border-radius: 8px;
    background: rgba(0,0,0,0.25); color: var(--text2);
    font: inherit; font-size: 13px; font-weight: 700; cursor: pointer;
    letter-spacing: 0.3px;
    transition: all 150ms;
  }
  .mux-mode-tabs button:hover { color: var(--text); border-color: var(--border-hover); }
  .mux-mode-tabs button.active {
    color: var(--red-hot);
    border-color: var(--red);
    background: linear-gradient(180deg, rgba(255,40,80,0.15), rgba(255,40,80,0.05));
    box-shadow: 0 0 0 1px var(--red), inset 0 1px 0 rgba(255,255,255,0.05);
  }

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
  .drop-label { font-size: 11px; color: var(--text3); text-transform: uppercase; letter-spacing: 1px; margin-bottom: 6px; }
  .drop-hint { color: var(--text2); text-transform: none; letter-spacing: 0; }

  /* Card unifiée Sources — design harmonisé moderne */
  .sources-card { padding: 18px 18px 18px 18px; }
  .sources-card .section-title { margin-bottom: 14px; font-size: 13px; letter-spacing: 1px; }

  .source-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: 16px;
    padding: 14px 18px;
    background: linear-gradient(180deg, rgba(255,255,255,0.025), rgba(0,0,0,0.18));
    border: 1px solid var(--border);
    border-radius: 10px;
    margin-bottom: 10px;
    min-height: 90px;
    box-sizing: border-box;
    transition: all 150ms;
  }
  .source-row:hover { border-color: var(--border-hover); background: linear-gradient(180deg, rgba(255,255,255,0.04), rgba(0,0,0,0.2)); }
  .source-row:last-child { margin-bottom: 0; }
  .source-row.secondary { border-left: 3px solid rgba(50,130,200,0.55); }

  .source-info { overflow: hidden; min-width: 0; }
  .source-label {
    display: flex; align-items: center; gap: 6px;
    font-size: 12px; color: var(--text2); font-weight: 600;
    text-transform: uppercase; letter-spacing: 0.5px;
  }
  .source-num {
    display: inline-flex; align-items: center; justify-content: center;
    width: 22px; height: 22px; border-radius: 50%;
    background: rgba(230,57,70,0.15); color: var(--red-hot);
    font-size: 13px; font-weight: 700; flex-shrink: 0;
  }
  .source-row.secondary .source-num { background: rgba(50,130,200,0.15); color: rgb(120,170,220); }
  .source-hint { color: var(--text3); font-weight: 400; font-size: 11px; text-transform: none; letter-spacing: 0; }

  .source-filename {
    font-size: 13px; color: var(--green); margin-top: 4px;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
    font-family: "JetBrains Mono", "SF Mono", ui-monospace, monospace;
  }
  .source-empty {
    font-size: 12px; color: var(--text3); margin-top: 4px; font-style: italic;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .source-meta {
    font-size: 11px; color: var(--text3); margin-top: 4px;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }

  .fr-audio-options {
    display: flex; flex-direction: row; gap: 14px; margin-top: 6px;
    flex-wrap: wrap; align-items: center;
  }
  .fr-audio-options .vfq-toggle {
    margin: 0;
  }

  .compat-grid {
    display: flex; flex-direction: row; flex-wrap: wrap;
    gap: 6px 18px; margin-top: 6px;
    font-size: 12px; color: var(--text2);
  }
  .compat-grid .compat-ok { color: #51cf66; font-weight: 600; }
  .compat-grid .compat-bad { color: #ff6b6b; font-weight: 600; }

  .extract-progress {
    display: flex; flex-direction: column; gap: 4px;
    margin-top: 8px;
  }
  .extract-progress-label {
    font-size: 11px; color: var(--text2); font-weight: 500;
  }
  .extract-progress progress {
    width: 100%; height: 6px; border: none; border-radius: 3px;
    background: rgba(0,0,0,0.4); overflow: hidden;
  }
  .extract-progress progress::-webkit-progress-bar {
    background: rgba(0,0,0,0.4); border-radius: 3px;
  }
  .extract-progress progress::-webkit-progress-value {
    background: linear-gradient(90deg, var(--red), var(--red-hot));
    border-radius: 3px;
  }
  /* Indeterminate progress (no value) — animated stripes */
  .extract-progress progress:not([value]) {
    background:
      linear-gradient(90deg, transparent 0%, var(--red-hot) 50%, transparent 100%) 0/40% 100% no-repeat,
      rgba(0,0,0,0.4);
    animation: indeterminate-slide 1.4s linear infinite;
  }
  @keyframes indeterminate-slide {
    0%   { background-position: -40% 0; }
    100% { background-position: 140% 0; }
  }

  /* État succès : barre pleine verte + label vert */
  .extract-progress.done .extract-progress-label { color: #51cf66; font-weight: 600; }
  .extract-progress.done progress[value]::-webkit-progress-value {
    background: linear-gradient(90deg, #2f9e44, #51cf66);
  }
  .extract-progress.done progress[value] {
    background: rgba(81, 207, 102, 0.15);
  }

  /* État erreur : barre pleine rouge + label rouge */
  .extract-progress.err .extract-progress-label { color: #ff6b6b; font-weight: 600; }
  .extract-progress.err progress[value]::-webkit-progress-value {
    background: linear-gradient(90deg, #c92a2a, #ff6b6b);
  }
  .extract-progress.err progress[value] {
    background: rgba(255, 107, 107, 0.15);
  }

  /* Preview TMDB (poster + infos) avant MUX AUTO */
  .tmdb-preview {
    display: flex; flex-direction: row; gap: 16px; align-items: flex-start;
    padding: 12px;
    background: linear-gradient(135deg, rgba(108, 99, 255, 0.08), rgba(0,0,0,0.2));
    border: 1px solid var(--border);
    border-radius: 10px;
    margin-bottom: 14px;
  }
  .tmdb-preview-poster {
    width: 100px; height: 150px; flex-shrink: 0;
    object-fit: cover;
    border-radius: 8px;
    border: 1px solid var(--border);
    box-shadow: 0 4px 12px -4px rgba(0,0,0,0.5);
  }
  .tmdb-preview-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  .tmdb-preview-title {
    font-size: 16px; font-weight: 700; color: var(--text); line-height: 1.2;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .tmdb-preview-vo {
    font-size: 12px; color: var(--text2); font-style: italic;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .tmdb-preview-meta {
    display: flex; gap: 12px; flex-wrap: wrap; margin-top: 4px;
    font-size: 12px; color: var(--text2);
  }
  .tmdb-preview-link {
    align-self: flex-start; margin-top: 6px;
    font-size: 11px;
  }
  .tmdb-preview-override {
    display: flex; flex-direction: row; gap: 6px; align-items: center;
    margin-top: 8px; padding-top: 8px;
    border-top: 1px dashed var(--border);
    font-size: 11px;
  }
  .tmdb-preview-override-label { color: var(--text3); white-space: nowrap; }
  .tmdb-preview-override-input {
    flex: 1; max-width: 130px;
    padding: 4px 8px; font-size: 12px;
    background: rgba(0,0,0,0.3);
    border: 1px solid var(--border);
    border-radius: 5px;
    color: var(--text);
    font-family: "JetBrains Mono", "SF Mono", ui-monospace, monospace;
  }
  .tmdb-preview-override-input:focus {
    outline: none; border-color: var(--red-hot);
    background: rgba(0,0,0,0.4);
  }
  .tmdb-preview-override-btn {
    padding: 4px 10px; font-size: 11px;
    background: rgba(108, 99, 255, 0.15);
    border: 1px solid rgba(108, 99, 255, 0.4);
    color: var(--text);
    border-radius: 5px;
    cursor: pointer;
    transition: all 150ms;
  }
  .tmdb-preview-override-btn:hover:not(:disabled) {
    background: rgba(108, 99, 255, 0.25);
    border-color: rgba(108, 99, 255, 0.6);
  }
  .tmdb-preview-override-btn:disabled { opacity: 0.4; cursor: not-allowed; }

  /* Bandeau persistant statut MUX AUTO */
  .mux-status-banner.success {
    border: 1px solid rgba(81, 207, 102, 0.4);
    background: linear-gradient(180deg, rgba(81, 207, 102, 0.08), rgba(47, 158, 68, 0.05));
  }
  .mux-status-banner.error {
    border: 1px solid rgba(255, 107, 107, 0.4);
    background: linear-gradient(180deg, rgba(255, 107, 107, 0.08), rgba(201, 42, 42, 0.05));
  }

  /* Module sync subs externes */
  .sub-sync-module {
    margin-top: 8px;
    padding: 8px 10px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.03);
    border: 1px solid var(--border);
  }
  .sub-sync-module .btn-tiny { font-size: 11px; padding: 4px 10px; }
  .sync-results-header {
    font-size: 11px; font-weight: 600; color: var(--text2);
    text-transform: uppercase; letter-spacing: 0.5px;
    margin-bottom: 4px;
  }
  .sync-results-list {
    list-style: none; padding: 0; margin: 0;
    display: flex; flex-direction: column; gap: 3px;
    font-size: 12px;
  }
  .sync-results-list li {
    display: flex; gap: 12px; justify-content: space-between; align-items: center;
    padding: 3px 6px; border-radius: 4px;
  }
  .sync-results-list li.has-offset { background: rgba(252, 196, 25, 0.10); }
  .sync-results-list li.has-offset .sync-result-status { color: #fcc419; font-weight: 600; }
  .sync-results-list li.no-offset .sync-result-status { color: #51cf66; }
  .sync-results-list li.err .sync-result-status { color: #ff6b6b; }
  .sync-result-name { color: var(--text2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sync-actions { display: flex; gap: 6px; margin-top: 6px; }

  .source-row-actions {
    display: flex; flex-direction: row; gap: 6px; flex-shrink: 0;
    align-items: center; justify-content: flex-end;
  }
  .source-row-actions .btn-primary {
    flex: 1; min-width: 0; max-width: 170px; white-space: nowrap;
    padding: 9px 12px; font-size: 12px;
  }
  .source-row-actions .btn-icon {
    width: 36px; height: 36px; flex-shrink: 0;
    display: flex; align-items: center; justify-content: center;
    background: rgba(0,0,0,0.3); border: 1px solid var(--border);
    border-radius: 6px; color: var(--text3);
    font-size: 14px; cursor: pointer; transition: all 150ms;
  }
  .source-row-actions .btn-icon:hover {
    color: rgb(255,150,150); border-color: rgba(239,68,68,0.5); background: rgba(239,68,68,0.12);
  }

  .compat-ok { color: rgb(80,220,120); font-weight: 700; margin-left: 4px; }
  .compat-bad { color: rgb(255,120,120); font-weight: 700; margin-left: 4px; }

  /* Bouton "+ Ajouter source de référence" placeholder, taille d'une source-row */
  .source-row-placeholder {
    display: flex; align-items: center; justify-content: center;
    height: 56px; padding: 0;
    background: transparent;
    border: 1px dashed var(--border); border-radius: 10px;
    color: var(--text3); font-size: 12px; cursor: pointer;
    transition: all 150ms; width: 100%;
  }
  .source-row-placeholder:hover { color: var(--text); border-color: var(--border-hover); background: rgba(255,255,255,0.02); }
  .automate-bar { padding: 14px 16px; border-color: rgba(230,57,70,0.4); background: linear-gradient(180deg, rgba(230,57,70,0.06), rgba(0,0,0,0.2)); position: relative; }
  .btn-reset-corner {
    position: absolute; top: 10px; right: 10px;
    width: 28px; height: 28px; border-radius: 50%;
    background: rgba(0,0,0,0.35); border: 1px solid var(--border);
    color: var(--text3); font-size: 14px; cursor: pointer;
    display: flex; align-items: center; justify-content: center;
    transition: all 150ms;
  }
  .btn-reset-corner:hover:not(:disabled) {
    background: rgba(239,68,68,0.18);
    border-color: rgba(239,68,68,0.5);
    color: rgb(255,150,150);
    transform: rotate(-90deg);
  }
  .btn-reset-corner:disabled { opacity: 0.4; cursor: not-allowed; }
  .btn-auto {
    background: linear-gradient(180deg, rgb(180,140,40), rgb(140,100,20));
    border-color: rgb(220,180,80);
    color: white;
  }
  .btn-auto:hover:not(:disabled) { background: linear-gradient(180deg, rgb(220,180,80), rgb(180,140,40)); }
  .auto-progress { margin-top: 12px; }
  .auto-progress-row { display: flex; align-items: center; gap: 10px; }
  .auto-progress .progress-bar { flex: 1; }
  .progress-bar.done { background: rgba(40,180,80,0.15); border-color: rgba(40,180,80,0.4); }
  .progress-bar.error { background: rgba(220,60,60,0.15); border-color: rgba(220,60,60,0.4); }
  .progress-fill.done-fill { background: linear-gradient(90deg, rgb(40,180,80), rgb(60,220,120)); }
  .progress-fill.error-fill { background: linear-gradient(90deg, rgb(220,60,60), rgb(255,100,100)); }
  .auto-status { font-size: 12px; color: var(--text2); margin-top: 6px; text-align: center; }
  .auto-status.done { color: rgb(80,220,120); font-weight: 600; }
  .auto-status.error { color: rgb(255,120,120); font-weight: 600; }

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
  .btn-stop {
    font-weight: 700; padding: 10px 18px; font-size: 14px;
    border-width: 2px; border-color: rgba(239,68,68,0.7);
    background: rgba(239,68,68,0.18); color: rgb(255,180,180);
  }
  .btn-stop:hover { background: rgba(239,68,68,0.32); color: white; }

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
  .preview-filename-row { display: flex; gap: 10px; align-items: flex-start; flex-wrap: wrap; }
  .filename-input {
    flex: 1; min-width: 280px; font-size: 13px;
    padding: 6px 8px; background: rgba(0,0,0,0.5);
    color: var(--green); border: 1px solid var(--border); border-radius: 6px;
  }
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
  .tmdb-alts { margin-top: 8px; }
  .tmdb-alts summary { cursor: pointer; font-size: 11px; color: var(--text3); padding: 4px 0; }
  .tmdb-alts summary:hover { color: var(--text); }
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

  .tmdb-picked {
    display: flex; gap: 14px; align-items: flex-start;
    margin-top: 12px; padding: 12px;
    background: linear-gradient(180deg, rgba(230,57,70,0.08), rgba(230,57,70,0.02));
    border: 1px solid rgba(230,57,70,0.4); border-radius: 10px;
  }
  .tmdb-picked-poster { width: 90px; height: 135px; object-fit: cover; border-radius: 6px; flex-shrink: 0; }
  .tmdb-picked-body { flex: 1; min-width: 0; }
  .tmdb-picked-title { font-weight: 700; font-size: 15px; color: var(--text); }
  .tmdb-picked-vo { font-size: 12px; color: var(--text2); margin-top: 2px; font-style: italic; }
  .tmdb-picked-meta { font-size: 11px; color: var(--text3); margin-top: 4px; }
  .tmdb-picked-overview { font-size: 12px; color: var(--text2); margin-top: 8px; line-height: 1.5; }
  .vfq-toggle {
    display: flex; flex-wrap: wrap; align-items: center; gap: 8px;
    margin-top: 6px; font-size: 11px; cursor: pointer;
  }
  .vfq-toggle input[type="checkbox"] { cursor: pointer; }
  .vfq-toggle .vfq-yes { color: rgb(80,220,120); font-weight: 600; }
  .vfq-toggle .vfq-no { color: rgb(255,200,100); font-weight: 600; }
  .vfq-link {
    color: var(--text3); text-decoration: underline; font-size: 10px;
    background: none; border: none; padding: 0; cursor: pointer; font-family: inherit;
  }
  .vfq-link:hover { color: var(--text); }

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
