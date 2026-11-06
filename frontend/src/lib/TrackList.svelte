<!--
  TrackList : card « 🎞️ Pistes » avec video, audio et subs mergés
  (internes + externes + secondaires), contrôles label/Keep/Default/Forced,
  OCR PGS → SRT, boutons monter/descendre/supprimer, barre de progression OCR
  par piste. Extraite de App.svelte en v7.1.6 (phase 1 découpage).
-->
<script>
  import Icon from './Icon.svelte';
  export let tracks = [];
  export let externalAudios = [];
  export let externalSubs = [];
  export let secondarySelected = [];
  export let videoChoice = { quality: '' };
  export let previewVideoName = '';
  export let options = { audio_labels: [], subtitle_labels: [] };
  export let psaSyncedAudioPath = '';
  export let ocrRunning = false;
  export let ocrTrackId = -1;
  export let ocrProgress = { status: '', percent: 0, message: '', subtitles: 0, corrected_lines: 0, lt_auto_fixed: 0, lt_needs_review: 0, quality_score: 0 };
  export let showLTReview = false;
  export let basename = (s) => (s || '').split('/').pop();
  export let formatBytes = (n) => String(n);
  export let isPGSTrack = () => false;
  export let isPGSExternal = () => false;
  export let onPickAudio = () => {};
  export let onPickSubs = () => {};
  export let onMoveTrack = (kind, idx, dir) => {};
  export let onMoveAudioTrack = (kind, idx, dir) => {};
  export let onRemoveInternalTrack = (id) => {};
  export let onRemoveSecondaryTrack = (idx, type) => {};
  export let onRemoveExternalAudio = (idx) => {};
  export let onRemoveExternalSub = (idx) => {};
  export let onRunOCR = (track) => {};
  export let onRunOCRExternalSup = (idx) => {};
  export let onSubLabelChange = () => {};

  $: audioCount = tracks.filter(t=>t.type==='audio' && t.keep).length + externalAudios.filter(a=>a.keep).length + secondarySelected.filter(t=>t.type==='audio' && t.keep).length;
  $: subCount   = tracks.filter(t=>t.type==='subtitles' && t.keep).length + externalSubs.filter(s=>s.keep).length + secondarySelected.filter(t=>t.type==='subtitles' && t.keep).length;
  $: videoCount = tracks.filter(t=>t.type==='video').length;
</script>

<div class="card card-grow">
  <div class="card-title-row">
    <div class="card-title"><Icon name="film" /> Pistes ({videoCount} vidéo · {audioCount} audio · {subCount} subs)</div>
    <div class="card-title-actions">
      <button class="btn btn-ghost btn-tiny" on:click={onPickAudio}>+ audio</button>
      <button class="btn btn-ghost btn-tiny" on:click={onPickSubs}>+ sub</button>
    </div>
  </div>

  <!-- Vidéo -->
  {#if videoCount > 0}
    <div class="tracks-section">
      <div class="tracks-section-header video"><span class="tracks-section-dot"></span><span class="tracks-section-label"><Icon name="play" /> Piste vidéo</span><span class="tracks-section-count">{videoCount}</span></div>
      {#each tracks.filter(t => t.type === 'video') as t}
        <div class="track">
          <div class="track-icon video"><Icon name="play" /></div>
          <div class="track-label">#{t.id} · {t.codec} · {t.pixelDims || ''} → {previewVideoName}</div>
          <div class="track-flag">{videoChoice.quality}</div>
        </div>
      {/each}
    </div>
  {/if}

  <!-- Audio -->
  {#if tracks.some(t => t.type === 'audio' && t.keep) || externalAudios.some(a => a.keep) || (secondarySelected.some(t => t.type === 'audio' && t.keep) && !psaSyncedAudioPath)}
    {@const internalAudios = tracks.filter(t => t.type === 'audio')}
    {@const secondaryAudios = psaSyncedAudioPath ? [] : secondarySelected.filter(t => t.type === 'audio')}
    {@const mergedAudios = [
      ...internalAudios.map((t, i) => ({ kind: 'internal', idx: i, ref: t, order: t.order ?? 0 })),
      ...externalAudios.map((a, i) => ({ kind: 'external', idx: i, ref: a, order: a.order ?? 0 })),
      ...secondaryAudios.map((s, i) => ({ kind: 'secondary', idx: i, ref: s, order: s.order ?? 0 })),
    ].filter(item => item.ref.keep).sort((a, b) => a.order - b.order)}
    <div class="tracks-section">
      <div class="tracks-section-header audio"><span class="tracks-section-dot"></span><span class="tracks-section-label"><Icon name="musical-note" /> Pistes audio</span><span class="tracks-section-count">{audioCount}</span><button class="btn-add-inline" title="Ajouter un fichier audio externe" on:click={onPickAudio}>+</button></div>
    {#each mergedAudios as item (item.kind + '-' + item.idx)}
      <div class="track track-editable" class:dropped={!item.ref.keep}>
        <div class="track-icon audio"><Icon name="musical-note" /></div>
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
            <button class="btn-arrow" title="Monter" on:click={() => onMoveAudioTrack(item.kind, item.idx, -1)}>↑</button>
            <button class="btn-arrow" title="Descendre" on:click={() => onMoveAudioTrack(item.kind, item.idx, +1)}>↓</button>
            {#if item.kind === 'internal'}
              <button class="btn-arrow danger" title="Supprimer" on:click={() => onRemoveInternalTrack(item.ref.id)}>✕</button>
            {:else if item.kind === 'secondary'}
              <button class="btn-arrow danger" title="Retirer du SUPPLY" on:click={() => onRemoveSecondaryTrack(item.idx, 'audio')}>✕</button>
            {:else}
              <button class="btn-arrow danger" title="Supprimer" on:click={() => onRemoveExternalAudio(item.idx)}>✕</button>
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
  {#if tracks.some(t => t.type === 'subtitles' && t.keep) || externalSubs.some(s => s.keep) || secondarySelected.some(t => t.type === 'subtitles' && t.keep)}
    {@const internalSubs = tracks.filter(t => t.type === 'subtitles')}
    {@const secondarySubs = secondarySelected.filter(t => t.type === 'subtitles')}
    {@const mergedSubs = [
      ...internalSubs.map((t, i) => ({ kind: 'internal', idx: i, ref: t, order: t.order ?? 0 })),
      ...externalSubs.map((s, i) => ({ kind: 'external', idx: i, ref: s, order: s.order ?? 0 })),
      ...secondarySubs.map((s, i) => ({ kind: 'secondary', idx: i, ref: s, order: s.order ?? 0 })),
    ].filter(item => item.ref.keep).sort((a, b) => a.order - b.order)}
    <div class="tracks-section">
      <div class="tracks-section-header sub"><span class="tracks-section-dot"></span><span class="tracks-section-label"><Icon name="chat-bubble-left-ellipsis" /> Sous-titres</span><span class="tracks-section-count">{subCount}</span><button class="btn-add-inline" title="Ajouter un fichier sous-titre externe" on:click={onPickSubs}>+</button></div>
    {#each mergedSubs as item (item.kind + '-' + item.idx)}
      <div class="track track-editable">
        <div class="track-icon sub"><Icon name="chat-bubble-left-ellipsis" /></div>
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
                on:click={() => onRunOCR(item.ref)}
              ><Icon name="language" /></button>
            {:else if item.kind === 'external' && isPGSExternal(item.ref)}
              <button
                class="btn-arrow"
                title={ocrRunning ? `OCR en cours…` : 'OCR PGS .sup → SRT (Tesseract)'}
                disabled={ocrRunning}
                on:click={() => onRunOCRExternalSup(item.idx)}
              ><Icon name="language" /></button>
            {/if}
            <button class="btn-arrow" title="Monter" on:click={() => onMoveTrack(item.kind, item.idx, -1)}>↑</button>
            <button class="btn-arrow" title="Descendre" on:click={() => onMoveTrack(item.kind, item.idx, +1)}>↓</button>
            {#if item.kind === 'internal'}
              <button class="btn-arrow danger" title="Supprimer" on:click={() => onRemoveInternalTrack(item.ref.id)}>✕</button>
            {:else if item.kind === 'secondary'}
              <button class="btn-arrow danger" title="Retirer du SUPPLY" on:click={() => onRemoveSecondaryTrack(item.idx, 'subtitles')}>✕</button>
            {:else}
              <button class="btn-arrow danger" title="Supprimer" on:click={() => onRemoveExternalSub(item.idx)}>✕</button>
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
                  <Icon name="language" /> OCR · {ocrProgress.status || '…'} · <b>{ocrProgress.percent}%</b>
                  {#if ocrProgress.message}— {ocrProgress.message}{/if}
                {:else if ocrProgress.status === 'done'}
                  <Icon name="check-circle" /> <b>OCR terminé</b> — qualité estimée <b>{ocrProgress.quality_score.toFixed(1)}%</b>
                  ({ocrProgress.subtitles} sous-titres · {ocrProgress.corrected_lines} lignes nettoyées · {ocrProgress.lt_auto_fixed} corrections auto · {ocrProgress.lt_needs_review} à vérifier)
                  {#if ocrProgress.lt_needs_review > 0}
                    <button class="btn btn-ghost btn-tiny" style="margin-left:8px" on:click={() => showLTReview = true}>Voir les {ocrProgress.lt_needs_review} lignes à vérifier</button>
                  {/if}
                {:else if ocrProgress.status === 'error'}
                  <Icon name="x-circle" /> OCR échoué — {ocrProgress.message}
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
