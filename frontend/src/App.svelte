<script>
  import { onMount } from 'svelte';
  import banner from './assets/images/banner.png';
  import {
    GetVersion, GetConfig, SaveConfig, GetLihdlOptions,
    SelectMkvFile, SelectOutputDir, LocateMkvmerge,
    AnalyzeMkv, SearchTmdb,
    BuildFilename, VideoTrackName,
    Mux, CancelMux,
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

  let videoChoice = { quality: 'HDLight', encoder: 'GANDALF', source: 'REMUX LiHDL', team: 'LiHDL' };
  let target = { title: '', year: '' };
  let tmdbResults = [];
  let tmdbQuery = '';
  let tmdbSearching = false;

  let muxing = false;
  let muxPercent = 0;
  let logLines = [];
  let logEl;
  let dragging = false;

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

  // Auto-suggest d'un libellé LiHDL pour une piste audio selon langue/codec/canaux.
  function suggestAudioLabel(t) {
    const lang = (t.properties.language || '').toLowerCase();
    const codec = (t.codec || t.properties.codec_id || '').toUpperCase();
    const ch = t.properties.audio_channels || 2;
    const ac3 = codec.includes('AC-3') && !codec.includes('E-AC'); // AC-3 pas EAC-3
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

  function suggestSubLabel(t) {
    const lang = (t.properties.language || '').toLowerCase();
    const codec = (t.codec || t.properties.codec_id || '').toUpperCase();
    const isPGS = codec.includes('PGS') || codec.includes('HDMV');
    const fmt = isPGS ? 'PGS' : 'SRT';
    const forced = !!t.properties.forced_track;
    if (lang === 'fre' || lang === 'fra' || lang === 'fr') {
      const kind = forced ? 'Forced' : 'Full';
      return `FR ${kind} : ${fmt}`;
    }
    if (lang === 'eng' || lang === 'en') return `ENG VO : ${fmt}`;
    return '';
  }

  function mapLangCode(lbl) {
    // déduit le code iso 639-2 à partir du libellé LiHDL.
    if (lbl.startsWith('FR')) return 'fre';
    if (lbl.startsWith('ENG')) return 'eng';
    if (lbl.startsWith('JPN')) return 'jpn';
    if (lbl.startsWith('ITA')) return 'ita';
    return '';
  }

  // Détecte le codec vidéo LiHDL depuis la piste vidéo du mkv.
  function videoCodecLihdl() {
    const v = tracks.find(t => t.type === 'video');
    if (!v) return '';
    const c = ((v.codec || '') + ' ' + (v.properties.codec_id || '')).toUpperCase();
    if (c.includes('AV1') || c.includes('AV01')) return 'AV1';
    if (c.includes('HEVC') || c.includes('H.265') || c.includes('H265') || c.includes('MPEGH')) return 'HEVC';
    if (c.includes('AVC') || c.includes('H.264') || c.includes('H264') || c.includes('MPEG4')) return 'AVC';
    return '';
  }

  function resolutionFromTracks() {
    const v = tracks.find(t => t.type === 'video');
    if (!v) return '';
    const dim = v.properties.pixel_dimensions || '';
    const m = /(\d+)x(\d+)/.exec(dim);
    if (!m) return '';
    const h = parseInt(m[2], 10);
    if (h >= 2100) return '2160p';
    if (h >= 1070) return '1080p';
    if (h >= 700)  return '720p';
    if (h >= 560)  return '576p';
    return h + 'p';
  }

  function audioCodecsForFilename() {
    // Liste des "AC3.5.1", "EAC3.2.0", etc. des pistes audio gardées.
    const out = [];
    for (const t of tracks) {
      if (t.type !== 'audio' || !t.keep) continue;
      const lbl = t.label || '';
      const m = /: (AC3|EAC3) (\d\.\d)/.exec(lbl);
      if (m) out.push(`${m[1]}.${m[2]}`);
    }
    return out;
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

  function dotify(s) {
    return String(s || '').trim().replace(/\s+/g, '.');
  }

  function buildFilenameClient() {
    if (!sourceInfo) return '';
    const parts = [];
    if (target.title) parts.push(dotify(target.title));
    if (target.year)  parts.push(target.year);
    if (videoChoice.source === 'REMUX CUSTOM LiHDL') parts.push('CUSTOM');
    parts.push(langFlagClient(keptAudioLabels()));
    const res = resolutionFromTracks(); if (res) parts.push(res);
    // Source + format : pour REMUX on met BluRay puis REMUX. Sinon la qualité.
    if (videoChoice.source.includes('REMUX')) {
      parts.push('BluRay');
      parts.push('REMUX');
    } else {
      parts.push(videoChoice.quality);
    }
    for (const ac of audioCodecsForFilename()) parts.push(ac);
    const vc = videoCodecLihdl(); if (vc) parts.push(vc);
    let name = parts.filter(Boolean).join('.');
    if (videoChoice.team) name += '-' + videoChoice.team;
    return name + '.mkv';
  }

  function videoTrackNameClient() {
    return `${videoChoice.quality} By ${videoChoice.encoder} Source ${videoChoice.source} ${videoChoice.team}`;
  }

  // Réactivité : on référence chaque dépendance pour que Svelte détecte
  // les changements et recalcule (sinon il n'analyse pas l'intérieur des fns).
  $: previewFilename = (function() {
    const _deps = [tracks.length, videoChoice.quality, videoChoice.encoder,
                   videoChoice.source, videoChoice.team, target.title, target.year];
    void _deps;
    return buildFilenameClient();
  })();
  $: previewVideoName = (function() {
    const _deps = [videoChoice.quality, videoChoice.encoder, videoChoice.source, videoChoice.team];
    void _deps;
    return videoTrackNameClient();
  })();

  // --- actions ---
  function openMkv(path) {
    sourcePath = path;
    sourceInfo = null;
    tracks = [];
    AnalyzeMkv(path); // fire-and-forget, résultat via event 'analyze:result'
  }

  function finalizeAnalyze(rawTracks) {
    appendLog('🎯 finalizeAnalyze appelé avec ' + rawTracks.length + ' pistes');
    sourceInfo = { tracks: rawTracks };
    tracks = rawTracks.map(t => {
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
      };
      if (t.type === 'audio')     base.label = suggestAudioLabelFlat(base);
      if (t.type === 'subtitles') base.label = suggestSubLabelFlat(base);
      if (t.type === 'video')     base.label = '';
      return base;
    });
    if (sourcePath) maybeAutoFillTitle(sourcePath);
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
    if (lang === 'eng' || lang === 'en') return `ENG VO : ${fmt}`;
    return '';
  }

  async function maybeAutoFillTitle(path) {
    const name = path.split('/').pop().replace(/\.[^.]+$/, '');
    tmdbQuery = name;
    try {
      tmdbSearching = true;
      const r = await SearchTmdb(name);
      tmdbResults = r || [];
      if (r && r.length === 1) {
        target.title = r[0].titre_fr || r[0].titre_vo || '';
        target.year  = r[0].annee_fr || '';
        appendLog('✓ TMDB : ' + target.title + ' (' + target.year + ')');
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
      const r = await SearchTmdb(tmdbQuery);
      tmdbResults = r || [];
    } catch (e) {
      appendLog('❌ TMDB : ' + String(e));
    } finally {
      tmdbSearching = false;
    }
  }

  function pickTmdb(r) {
    target.title = r.titre_fr || r.titre_vo || '';
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

  async function saveSettings() {
    await SaveConfig(config);
    appendLog('✓ Réglages enregistrés');
    mkvmergePath = await LocateMkvmerge();
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
    EventsOn('mux:done', () => { muxing = false; muxPercent = 0; });
    EventsOn('file:dropped', (path) => { openMkv(path); });
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
  });
</script>

<main>
  <header class="topbar">
    <img class="banner" src={banner} alt="LiHDL" />
    <div class="topbar-right">
      <span class="version">v{appVersion}</span>
      <button class="btn-icon" on:click={() => screen = 'reglages'} title="Réglages">⚙</button>
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
          <div class="drop-sub">ou</div>
          <button class="btn-primary" on:click={pickMkvDialog}>Choisir un fichier</button>
        {:else}
          <div class="drop-title">{sourcePath.split('/').pop()}</div>
          <div class="drop-sub">{sourcePath}</div>
          <button class="btn-ghost" on:click={pickMkvDialog}>Changer</button>
        {/if}
      </div>

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
                <label>Source
                  <select bind:value={videoChoice.source}>
                    {#each options.video_sources as s}<option>{s}</option>{/each}
                  </select>
                </label>
                <label>Team
                  <select bind:value={videoChoice.team}>
                    {#each options.video_teams as tm}<option>{tm}</option>{/each}
                  </select>
                </label>
              </div>
              <div class="track-preview mono">→ {previewVideoName}</div>
            </div>
          {/each}
        </div>

        <!-- Audio -->
        {#if tracks.some(t => t.type === 'audio')}
          <div class="card">
            <div class="section-title">Pistes audio</div>
            {#each tracks.filter(t => t.type === 'audio') as t}
              <div class="track-row">
                <div class="track-meta">
                  <span class="badge audio">AUDIO</span>
                  <span class="mono">#{t.id} · {t.codec} · {t.lang || '??'} · {t.channels || '?'}ch</span>
                  {#if t.name}<span class="track-current">« {t.name} »</span>{/if}
                </div>
                <div class="track-controls">
                  <select bind:value={t.label}>
                    <option value="">— choisir —</option>
                    {#each options.audio_labels as lbl}<option>{lbl}</option>{/each}
                  </select>
                  <label class="chk"><input type="checkbox" bind:checked={t.keep}/> Garder</label>
                  <label class="chk"><input type="checkbox" bind:checked={t.default}/> Default</label>
                  <label class="chk"><input type="checkbox" bind:checked={t.forced}/> Forced</label>
                </div>
              </div>
            {/each}
          </div>
        {/if}

        <!-- Subtitles -->
        {#if tracks.some(t => t.type === 'subtitles')}
          <div class="card">
            <div class="section-title">Sous-titres</div>
            {#each tracks.filter(t => t.type === 'subtitles') as t}
              <div class="track-row">
                <div class="track-meta">
                  <span class="badge subs">SUBS</span>
                  <span class="mono">#{t.id} · {t.codec} · {t.lang || '??'}</span>
                  {#if t.name}<span class="track-current">« {t.name} »</span>{/if}
                </div>
                <div class="track-controls">
                  <select bind:value={t.label}>
                    <option value="">— choisir —</option>
                    {#each options.subtitle_labels as lbl}<option>{lbl}</option>{/each}
                  </select>
                  <label class="chk"><input type="checkbox" bind:checked={t.keep}/> Garder</label>
                  <label class="chk"><input type="checkbox" bind:checked={t.default}/> Default</label>
                  <label class="chk"><input type="checkbox" bind:checked={t.forced}/> Forced</label>
                </div>
              </div>
            {/each}
          </div>
        {/if}

        <div class="actions-row">
          <button class="btn-primary" on:click={() => screen = 'cible'}>Suivant → Cible</button>
        </div>
      {/if}

    {:else if screen === 'cible'}
      <div class="card">
        <div class="section-title">Recherche TMDB</div>
        <div class="field-row">
          <input type="text" bind:value={tmdbQuery} placeholder="Titre du film…" on:keydown={(e) => e.key === 'Enter' && searchTmdb()} />
          <button class="btn-primary" on:click={searchTmdb} disabled={tmdbSearching}>{tmdbSearching ? '…' : 'Chercher'}</button>
        </div>
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
        <div class="field"><label>Titre</label>
          <input type="text" bind:value={target.title} placeholder="Titre du film" />
        </div>
        <div class="field"><label>Année</label>
          <input type="text" bind:value={target.year} placeholder="2025" maxlength="4" />
        </div>
        <div class="preview-box">
          <div class="preview-label">Nom de fichier final</div>
          <div class="preview-value mono">{previewFilename || '—'}</div>
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
        <div class="section-title">TMDB / index serveurperso</div>
        <div class="field"><label>Clé API TMDB (optionnelle)</label>
          <input type="password" bind:value={config.tmdb_key} placeholder="laisse vide si tu utilises juste serveurperso" />
        </div>
        <div class="field"><label>URL de l'index serveurperso</label>
          <input type="text" bind:value={config.serveurperso_url} />
        </div>
      </div>

      <div class="card">
        <div class="section-title">Dossier de sortie</div>
        <div class="field-row">
          <input type="text" bind:value={config.output_dir} placeholder="/Users/…/Mux" readonly />
          <button class="btn-test" on:click={pickOutputDir}>Choisir…</button>
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
        <div class="field"><label>Source</label>
          <select bind:value={config.default_source}>
            {#each options.video_sources as s}<option>{s}</option>{/each}
          </select>
        </div>
        <div class="field"><label>Team</label>
          <select bind:value={config.default_team}>
            {#each options.video_teams as tm}<option>{tm}</option>{/each}
          </select>
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
      radial-gradient(1200px 700px at 50% -200px, rgba(230, 57, 70, 0.08), transparent 60%),
      var(--bg);
  }

  main {
    display: flex;
    flex-direction: column;
    min-height: 100vh;
    text-align: left;
  }

  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 20px;
    border-bottom: 1px solid var(--border);
    background: rgba(13, 10, 16, 0.8);
  }
  .banner { height: 36px; object-fit: contain; }
  .topbar-right { display: flex; align-items: center; gap: 10px; }
  .version { color: var(--text3); font-size: 11px; font-variant-numeric: tabular-nums; }
  .btn-icon {
    width: 34px; height: 34px; border-radius: 8px;
    border: 1px solid var(--border);
    background: rgba(255,255,255,0.03);
    color: var(--text2); font-size: 16px; cursor: pointer;
    transition: all 150ms;
  }
  .btn-icon:hover { background: rgba(255,255,255,0.08); color: var(--text); }

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
    padding: 36px 20px;
  }
  .drop-icon { font-size: 42px; margin-bottom: 10px; }
  .drop-title { font-size: 15px; font-weight: 700; color: var(--text); }
  .drop-sub { font-size: 12px; color: var(--text2); margin: 4px 0 10px; word-break: break-all; }

  .track-row {
    padding: 10px 0;
    border-top: 1px dashed var(--border);
  }
  .track-row:first-child { border-top: none; }
  .track-row.video .video-dropdowns {
    display: grid; grid-template-columns: repeat(4, 1fr);
    gap: 10px; margin-top: 8px;
  }
  .video-dropdowns label { font-size: 11px; color: var(--text3); display: flex; flex-direction: column; gap: 4px; }
  .track-preview { font-size: 12px; color: var(--green); margin-top: 8px; }

  .track-meta { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  .track-current { color: var(--text3); font-size: 11px; font-style: italic; }
  .badge {
    padding: 2px 7px; border-radius: 4px; font-size: 10px; font-weight: 700;
    letter-spacing: 1px;
  }
  .badge.audio { background: rgba(0,180,216,0.15); color: var(--blue-hot); }
  .badge.subs  { background: rgba(255,214,10,0.15); color: var(--yellow); }
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

  .preview-box {
    margin-top: 10px; padding: 12px 14px;
    background: rgba(0,0,0,0.35); border: 1px solid var(--border);
    border-radius: 8px;
  }
  .preview-label {
    font-size: 10px; color: var(--text3); text-transform: uppercase;
    letter-spacing: 1px; margin-bottom: 6px;
  }
  .preview-value { color: var(--green); font-size: 13px; word-break: break-all; }

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
