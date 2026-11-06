<!--
  SourcesCard : card unifiée « 📁 Sources » avec :
   - Ligne ① Source principale (MKV encodé ou PSA selon muxMode)
   - Ligne ② SUPPLY/FW + Ligne ③ MKV synchro (mode PSA uniquement)
   - Ligne ② Sous-titres externes + Ligne ③ Source de référence (mode LiHDL)
     avec sous-module de sync alass (SRT vs audio) + extraction FR audio
  Extraite de App.svelte en v7.1.7 (phase 1 découpage).
-->
<script>
  import Icon from './Icon.svelte';
  // Mode + état principal
  export let muxMode = 'lihdl';
  export let sourcePath = '';
  export let sourceMkvInfo = null;
  export let psaSourcesValid = false;
  // SUPPLY (PSA)
  export let secondaryPath = '';
  export let secondaryTracks = [];
  export let secondaryMkvInfo = null;
  export let psaSyncStatus = '';
  export let psaSyncMessage = '';
  export let psaSubSyncRunning = false;
  // MKV synchro (PSA)
  export let psaSyncedAudioPath = '';
  export let psaSyncedSourceMkv = '';
  // Subs externes (LiHDL)
  export let externalSubs = [];
  export let srtExtracting = false;
  export let srtExtractionResult = '';
  export let srtPhase = '';
  export let srtPercent = 0;
  export let srtAssConverted = false;
  // Sub-sync module (LiHDL)
  export let subSyncChecking = false;
  export let subSyncResults = [];
  export let subSyncAppliedMsg = '';
  export let subSyncPercent = 0;
  export let subSyncCurrentName = '';
  // Reference row (LiHDL)
  export let referencePath = '';
  export let referenceMkvInfo = null;
  export let showReferenceBar = false;
  export let extractFRVFF = false;
  export let extractFRVFQ = false;
  export let extractENG = false;
  export let syncRefLang = 'fr';
  export let frAudioExtracting = false;
  export let frAudioExtractionResult = '';
  export let frAudioPhase = '';
  export let frAudioConvertPercent = 0;
  export let frAudioConvertedSummary = '';
  // Helpers
  export let basename = (s) => (s || '').split('/').pop();
  export let formatDuration = (s) => String(s);
  export let checkCompat = () => ({ duration_ok: false, framerate_ok: false });
  // Callbacks
  export let pickMkvDialog = () => {};
  export let pickSecondaryDialog = () => {};
  export let pickPSASyncedAudio = () => {};
  export let clearPSASyncedAudio = () => {};
  export let syncSupplySubsToPSA = () => {};
  export let pickSubsDialog = () => {};
  export let runSubSyncCheck = () => {};
  export let applySubSyncResults = () => {};
  export let dismissSubSyncResults = () => {};
  export let pickReferenceDialog = () => {};
  export let clearReference = () => {};
  export let runRefSubsExtraction = () => {};
  export let runFRAudioExtraction = () => {};
</script>

<div class="card drop-target" style:--wails-drop-target="drop">
  <div class="card-title"><Icon name="folder" /> Sources</div>

  <div class="source-list">

  <!-- Ligne ① Source principale -->
  <div class="source-row" class:filled={sourcePath} style={muxMode === 'psa' && psaSourcesValid ? 'border-left:3px solid #7ad17a;background:rgba(122,209,122,0.08);' : ''}>
    <div class="source-num">{muxMode === 'psa' && psaSourcesValid ? '✓' : '1'}</div>
    <div class="source-info">
      <div class="source-label">{muxMode === 'psa' ? 'Source PSA (vidéo gardée)' : 'Source encodée (vidéo gardée)'}</div>
      {#if sourcePath}
        <div class="source-value">{sourcePath.split('/').pop()}</div>
        {#if sourceMkvInfo}
          <div class="source-value" style="opacity:0.7;font-size:11px;"><Icon name="clock" /> {formatDuration(sourceMkvInfo.duration_seconds)} · <Icon name="film" /> {sourceMkvInfo.framerate || '?'} fps</div>
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
    <div class="source-row" class:filled={secondaryPath} style={psaSourcesValid ? 'border-left:3px solid #7ad17a;background:rgba(122,209,122,0.08);' : ''}>
      <div class="source-num">{psaSourcesValid ? '✓' : '2'}</div>
      <div class="source-info">
        <div class="source-label">Source SUPPLY / FW / Super U (audios + subs)</div>
        {#if secondaryPath}
          <div class="source-value">{secondaryPath.split('/').pop()} · {secondaryTracks.length} piste(s)</div>
          {#if secondaryMkvInfo}
            <div class="source-value" style="opacity:0.7;font-size:11px;"><Icon name="clock" /> {formatDuration(secondaryMkvInfo.duration_seconds)} · <Icon name="film" /> {secondaryMkvInfo.framerate || '?'} fps</div>
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
      <div class="source-row-actions">
        {#if secondaryPath}
          <button class="btn-primary btn-tiny" on:click={syncSupplySubsToPSA} disabled={psaSubSyncRunning} title="Extrait les subs FR/ENG du SUPPLY et les synchronise sur la PSA via alass">
            {psaSubSyncRunning ? '⏳' : '🔎 Sync subs → PSA'}
          </button>
        {/if}
        <button class="btn btn-ghost" on:click={pickSecondaryDialog}>
          {secondaryPath ? 'Changer' : 'Choisir SUPPLY/FW/Super U'}
        </button>
      </div>
    </div>

    <!-- Ligne ③ MKV synchro (mode PSA) -->
    <div class="source-row" class:filled={psaSyncedAudioPath} style={psaSyncedAudioPath ? 'border-left:3px solid #7ad17a;background:rgba(122,209,122,0.08);' : ''}>
      <div class="source-num">{psaSyncedAudioPath ? '✓' : '3'}</div>
      <div class="source-info">
        <div class="source-label">MKV synchro (optionnel · audio extrait + bypass SUPPLY audio)</div>
        {#if psaSyncedAudioPath}
          <div class="source-value">{basename(psaSyncedSourceMkv)}</div>
          <div class="source-value" style="opacity:0.7;font-size:11px;">Audio extrait + audios SUPPLY décochés</div>
        {:else}
          <div class="source-value empty">— Optionnel : un .mkv dont l'audio est déjà synchro avec la PSA —</div>
        {/if}
      </div>
      <div class="source-row-actions">
        <button class="btn btn-ghost" on:click={pickPSASyncedAudio}>
          {psaSyncedAudioPath ? 'Changer' : 'Choisir MKV'}
        </button>
        {#if psaSyncedAudioPath}
          <button class="btn btn-ghost btn-icon" on:click={clearPSASyncedAudio} title="Retirer le MKV synchro">✕</button>
        {/if}
      </div>
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
                  <Icon name="arrow-path" /> Conversion ASS → SRT (via ffmpeg) — {srtPercent}%
                {:else}
                  <Icon name="clock" /> Extraction SRT + détection sync — {srtPercent}%
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
              <div class="extract-progress">
                <span class="extract-progress-label">
                  <Icon name="magnifying-glass" /> Analyse sync SRT — {subSyncPercent}% {subSyncCurrentName ? `(${subSyncCurrentName})` : ''}
                </span>
                <progress max="100" value={subSyncPercent}></progress>
              </div>
            {:else if subSyncResults.length === 0 && subSyncAppliedMsg}
              <div class="extract-progress done">
                <span class="extract-progress-label">{subSyncAppliedMsg}</span>
                <progress max="100" value="100"></progress>
              </div>
              <div class="sync-actions" style="margin-top:6px;">
                <button class="btn btn-tiny" on:click={runSubSyncCheck}><Icon name="magnifying-glass" /> Re-vérifier</button>
                <button class="btn btn-ghost btn-tiny" on:click={dismissSubSyncResults}>Masquer</button>
              </div>
            {:else if subSyncResults.length === 0}
              <button class="btn btn-tiny" on:click={runSubSyncCheck}>
                <Icon name="magnifying-glass" /> Vérifier sync SRT vs audio source
              </button>
            {:else}
              <div class="sync-results-header">Synchronisation alass (SRT corrigé prêt) :</div>
              <ul class="sync-results-list">
                {#each subSyncResults as r}
                  <li class:has-offset={!r.error && r.synced_path} class:err={!!r.error}>
                    <span class="sync-result-name mono">{basename(r.path)}</span>
                    {#if r.error}
                      <span class="sync-result-status">✕ {r.error}</span>
                    {:else if r.rejected_offset_ms}
                      <span class="sync-result-status">
                        ✓ sub original conservé (alass proposait {r.rejected_offset_ms > 0 ? '+' : ''}{r.rejected_offset_ms} ms — hors seuil, probablement faux match)
                      </span>
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

    <!-- Ligne ③ Source de référence -->
    {#if showReferenceBar}
      {@const compat = checkCompat(sourceMkvInfo, referenceMkvInfo)}
      <div class="source-row source-row-stacked" class:filled={referencePath}>
        <div class="source-num">3</div>
        <div class="source-info">
          <div class="source-label">Source de référence (extraction sous-titres + FR audio sur demande)</div>
          {#if referencePath}
            <div class="source-value">{basename(referencePath)}</div>
            {#if referenceMkvInfo && sourceMkvInfo}
              {@const refSyncOK = (Array.isArray(subSyncResults) && subSyncResults.length > 0 && subSyncResults.every(r => !r.error)) || frAudioExtractionResult === 'success'}
              <div class="compat-grid">
                <span>Durée : {formatDuration(referenceMkvInfo.duration_seconds)} · {referenceMkvInfo.framerate || '?'} fps</span>
                <span>VFQ : <span class:compat-ok={referenceMkvInfo.has_vfq_audio} class:compat-bad={!referenceMkvInfo.has_vfq_audio}>{referenceMkvInfo.has_vfq_audio ? '✓ ' + (referenceMkvInfo.vfq_track_info || 'piste détectée') : '✗ aucune piste FR Canada'}</span></span>
                {#if refSyncOK}
                  <span class="compat-ok">✓ Sync valide</span>
                {/if}
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
              <span style="opacity:0.7;font-size:12px;"><Icon name="viewfinder-circle" /> Référence sync :</span>
              <label class="vfq-toggle">
                <input type="radio" name="syncRefLang" bind:group={syncRefLang} value="fr" disabled={frAudioExtracting} />
                <span>FR (VFF/VFi)</span>
              </label>
              <label class="vfq-toggle">
                <input type="radio" name="syncRefLang" bind:group={syncRefLang} value="fr-ca" disabled={frAudioExtracting} />
                <span>FR VFQ</span>
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
                      <Icon name="magnifying-glass" /> Détection sync audio…
                    {:else if frAudioPhase.startsWith('convert:')}
                      {@const parts = frAudioPhase.slice(8).split(',')}
                      <Icon name="arrow-path" /> Conversion {parts[0]} → AC3 ({parts[1] || ''}) — {frAudioConvertPercent}%
                    {:else if frAudioPhase === 'convert'}
                      <Icon name="arrow-path" /> Conversion vers AC3 — {frAudioConvertPercent}%
                    {:else}
                      <Icon name="clock" /> Extraction FR audio + détection sync…
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
