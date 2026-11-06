<!--
  VideoSettingsCard : card « 🎛️ Réglages piste vidéo » avec les 12 champs
  (Qualité / Encodeur / Type source / Team source / Résolution / Source sortie
  / Codec vidéo / Team sortie / Flag langue / Titre / Épisode-Année / Codec
  auto-suggéré). Extraite de App.svelte en v7.1.4 (phase 1 découpage).
-->
<script>
  export let videoChoice = { quality: '', encoder: '', sourceType: '', sourceTeam: '', team: '' };
  export let target = { resolution: '', source: '', video_codec: '', flagOverride: 'auto', title: '', episode: '', year: '' };
  export let options = { video_qualities: [], video_encoders: [], video_source_types: [], video_teams: [], resolutions: [], target_sources: [], video_codecs: [] };
  export let tmdbMode = 'movie';
  export let includeYearInEpisode = false;
  export let sourcePath = '';
  export let suggestedCodecDisplay = '';
  export let flagOverrideOptions = [];
  export let resolutionOptions = [];
  export let targetSourceOptions = [];
  export let videoCodecOptions = [];
  export let videoSourceTypeOptions = [];
  export let onSourceTypeChange = () => {};
  export let detectEpisode = () => '';
</script>

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
        {#each (options.video_source_types && options.video_source_types.length > 0 ? options.video_source_types : videoSourceTypeOptions) as s}<option>{s}</option>{/each}
      </select>
    </div>
    <div class="field">
      <span class="field-label">Team source</span>
      <input type="text" bind:value={videoChoice.sourceTeam} placeholder="ex: Alkaline" />
    </div>
    <div class="field">
      <span class="field-label">Résolution</span>
      <select bind:value={target.resolution}>
        {#each (options.resolutions && options.resolutions.length > 0 ? options.resolutions : resolutionOptions) as r}<option>{r}</option>{/each}
      </select>
    </div>
    <div class="field">
      <span class="field-label">Source (sortie)</span>
      <select bind:value={target.source}>
        {#each (options.target_sources && options.target_sources.length > 0 ? options.target_sources : targetSourceOptions) as s}<option>{s}</option>{/each}
      </select>
    </div>
    <div class="field">
      <span class="field-label">Codec vidéo</span>
      <select bind:value={target.video_codec}>
        {#each (options.video_codecs && options.video_codecs.length > 0 ? options.video_codecs : videoCodecOptions) as c}<option>{c}</option>{/each}
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
        {#each flagOverrideOptions as f}
          <option value={f}>{f === 'auto' ? 'Auto' : f}</option>
        {/each}
      </select>
    </div>
    <div class="field">
      <span class="field-label">Titre</span>
      <input type="text" bind:value={target.title} placeholder="Titre" />
    </div>
    <div class="field">
      <span class="field-label" style="display:flex;justify-content:space-between;align-items:center;gap:6px;">
        <span>{target.episode ? 'Épisode' : 'Année'}</span>
        <span style="display:flex;gap:3px;">
          <button type="button" class="tmdb-mode-toggle small" class:active={!target.episode} on:click={() => { target.episode = ''; tmdbMode = 'movie'; }} title="Mode Film : année dans le filename">🎬</button>
          <button type="button" class="tmdb-mode-toggle small" class:active={!!target.episode} on:click={() => { tmdbMode = 'tv'; if (!target.episode) target.episode = (sourcePath ? detectEpisode(sourcePath.split('/').pop()) : '') || 'S01E01'; }} title="Mode Série : SxxExx dans le filename">📺</button>
        </span>
      </span>
      {#if target.episode}
        <input type="text" bind:value={target.episode} placeholder="S01E01" maxlength="10" />
        <label class="chk" style="margin-top:6px;font-size:11px;" title="Inclure l'année avant SxxExx (ex: pour remake — Title.2024.S01E01)">
          <input type="checkbox" bind:checked={includeYearInEpisode} />
          <span>Inclure année</span>
        </label>
      {:else}
        <input type="text" bind:value={target.year} placeholder="2025" maxlength="4" />
      {/if}
    </div>
    {#if target.episode && includeYearInEpisode}
      <div class="field">
        <span class="field-label">Année (incluse)</span>
        <input type="text" bind:value={target.year} placeholder="2024" maxlength="4" />
      </div>
    {/if}
    <div class="field">
      <span class="field-label">Codec auto-suggéré</span>
      <input type="text" value={suggestedCodecDisplay || '—'} readonly />
    </div>
  </div>
</div>
