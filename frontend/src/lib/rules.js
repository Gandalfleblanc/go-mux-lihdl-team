// Règles LiHDL pures (sans état d'écran) : détection épisode/source, labels
// audio/sous-titres, flag langue, nommage. Testées dans rules.test.js.

export function detectEpisode(filename) {
  // Accepte séparateurs . _ - espace autour de SxxExx (ex: Dallas_S03E01_).
  const m = /(?:^|[\s._-])S(\d{1,2})E(\d{1,3})(?=[\s._-]|$)/i.exec(String(filename || ''));
  if (!m) return '';
  const s = m[1].padStart(2, '0');
  const e = m[2].padStart(2, '0');
  return `S${s}E${e}`;
}

export function detectSourceType(filename) {
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

export function parsePsaSourceInfo(filename) {
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

export function parseSupplyInfo(filename) {
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

export function looksLikeAD(t) {
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

export function inferAudioLabel(track) {
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

export function inferSubLabel(track, _indexAmongSameLang) {
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

export function mapLangCode(lbl) {
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

export function dotify(s) {
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

export function subLabelPriority(label) {
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

export function cleanQueryFromFilename(filename) {
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

export function langFlag(labels, hasFrSub) {
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
  const prefix = hasFrSub ? 'MULTi' : 'DUAL';
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

// Titre normalisé pour comparer nom de fichier et fiches TMDB :
// minuscules, sans accents, « & » = « and », ponctuation ignorée.
export function normalizeTitle(s) {
  return String(s || '')
    .toLowerCase()
    .normalize('NFD').replace(/[̀-ͯ]/g, '')
    .replace(/&/g, ' and ')
    .replace(/[^a-z0-9]+/g, ' ')
    .trim();
}

// Année 19xx/20xx isolée dans un nom de fichier (pas « 1080p »).
export function yearFromFilename(filename) {
  const m = /(?:^|[\s._-])((?:19|20)\d{2})(?=[\s._-]|$)/.exec(String(filename || ''));
  return m ? m[1] : '';
}

// Index du seul résultat TMDB dont le titre (VO ou FR) correspond exactement
// à la recherche — et à l'année du fichier si elle est connue. -1 si aucun
// ou plusieurs : dans ce cas on demande confirmation à l'utilisateur.
export function findUnambiguousTmdbMatch(query, fileYear, results) {
  const q = normalizeTitle(query);
  if (!q || !Array.isArray(results)) return -1;
  const hits = [];
  results.forEach((r, i) => {
    const sameTitle = [r.titre_vo, r.titre_fr].some((t) => normalizeTitle(t) === q);
    const sameYear = !fileYear || String(r.annee_fr || '') === String(fileYear);
    if (sameTitle && sameYear) hits.push(i);
  });
  return hits.length === 1 ? hits[0] : -1;
}

// Mode Saison : regroupe les .mkv d'un dossier par épisode (SxxExx).
//  - mode 'psa'   : chaque épisode = 1 PSA (vidéo) + 1 SUPPLY/FW (audios+subs)
//  - mode 'lihdl' : chaque épisode = 1 fichier source
// Les fichiers sans SxxExx sont listés à part (ignored). Les manques et
// doublons sont signalés dans `issues` pour que l'utilisateur les voie
// avant de lancer.
export function pairSeasonFiles(paths, mode) {
  const byEp = new Map();
  const ignored = [];
  for (const p of paths || []) {
    const name = String(p).split('/').pop();
    const ep = detectEpisode(name);
    if (!ep) { ignored.push(p); continue; }
    if (!byEp.has(ep)) byEp.set(ep, []);
    byEp.get(ep).push(p);
  }
  const episodes = [...byEp.keys()].sort().map((ep) => {
    const files = byEp.get(ep);
    const issues = [];
    let source = '', supply = '';
    if (mode === 'psa') {
      const isPsa = (f) => parsePsaSourceInfo(String(f).split('/').pop()).isPSA;
      const psas = files.filter(isPsa);
      const others = files.filter((f) => !isPsa(f));
      source = psas[0] || '';
      supply = others[0] || '';
      if (!source) issues.push('PSA manquante');
      if (!supply) issues.push('SUPPLY manquant');
      if (psas.length > 1 || others.length > 1) issues.push('plusieurs fichiers pour cet épisode — le 1er est utilisé');
    } else {
      source = files[0];
      if (files.length > 1) issues.push('plusieurs fichiers pour cet épisode — le 1er est utilisé');
    }
    return { episode: ep, source, supply, issues };
  });
  return { episodes, ignored };
}

// Nom de sortie modifié à la main : au passage à l'épisode suivant, on garde
// la retouche (ex. VOSTFR) mais on remplace le SxxExx par le nouvel épisode.
// Renvoie '' si ce n'est pas possible (pas de SxxExx dans le nom ou nouvel
// épisode inconnu) → l'appelant revient alors au nom automatique.
export function carryManualFilename(manual, newEpisode) {
  if (!manual || !newEpisode) return '';
  const re = /(^|[\s._-])S\d{1,2}E\d{1,3}(?=[\s._-]|$)/i;
  if (!re.test(manual)) return '';
  return manual.replace(re, (_, sep) => sep + newEpisode);
}
