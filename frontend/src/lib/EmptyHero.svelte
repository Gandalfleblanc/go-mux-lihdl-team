<!--
  EmptyHero : écran d'accueil avec hero « Prêt à muxer » / « Source chargée »,
  CTA Choisir MKV / queue, drop-tip, hints et bloc Outils additionnels
  (OCR / OpenSubtitles / Synchro audios) intégré quand aucune source.
  Extraite de App.svelte en v7.1.9 (phase 1 découpage).
-->
<script>
  import Icon from './Icon.svelte';
  export let sourcePath = '';
  export let muxMode = 'lihdl';
  export let ocrRunning = false;
  export let ocrTrackId = -1;
  export let ocrProgress = { status: '', percent: 0, message: '', subtitles: 0, corrected_lines: 0, lt_auto_fixed: 0, lt_needs_review: 0, quality_score: 0 };
  export let onPickMkv = () => {};
  export let onPickMultipleMkv = () => {};
  export let onOpenSeason = () => {};
  export let onPickAndOCR = () => {};
  export let onOpenOSModal = () => {};
  export let onOpenSync = () => {};
</script>

<div class="card empty-hero" class:compact={sourcePath}>
  <div class="empty-hero-aurora" aria-hidden="true"></div>
  <div class="empty-hero-content">
    <div class="empty-hero-badge">{muxMode === 'psa' ? 'CUSTOM PSA · SUPPLY/FW MUX' : 'LiHDL · FILM MUX PIPELINE'}</div>
    <div class="empty-hero-icon">
      <span class="empty-hero-icon-bg" aria-hidden="true"></span>
      <span class="empty-hero-icon-glyph">{#if sourcePath}<Icon name="check" />{:else}<Icon name="play" />{/if}</span>
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
        <button class="btn btn-accent empty-hero-cta" on:click={onPickMkv}>
          <span><Icon name="folder" /></span> Choisir un MKV
        </button>
        <button class="btn empty-hero-cta-secondary" on:click={onPickMultipleMkv}>
          <span><Icon name="queue-list" /></span> Choisir plusieurs (queue)
        </button>
        <button class="btn btn-danger empty-hero-cta season-cta" on:click={onOpenSeason} title="Choisis le dossier d'une saison : chaque épisode est associé et muxé automatiquement">
          <Icon name="tv" /> Mode Saison
        </button>
      </div>
      <div class="empty-hero-droptip">↓ Ou glisse plusieurs MKV ici pour les mettre en file ↓</div>
      <div class="empty-hero-hints">
        <span class="empty-hero-hint"><Icon name="bolt" /> MUX rapide bit-à-bit</span>
        <span class="empty-hero-hint-dot">·</span>
        <span class="empty-hero-hint"><Icon name="film" /> Fiche TMDB auto</span>
        <span class="empty-hero-hint-dot">·</span>
        <span class="empty-hero-hint"><Icon name="speaker-wave" /> Sync audio AC3/EAC3</span>
      </div>

      <!-- Outils additionnels intégrés -->
      <div class="empty-hero-tools">
        <div class="empty-hero-tools-title"><Icon name="speaker-wave" /> Outils additionnels</div>
        <div class="empty-hero-tools-row">
          <button class="btn btn-ghost" on:click={onPickAndOCR} disabled={ocrRunning} title="OCR PGS .sup → SRT (Tesseract + regex + LanguageTool)"><Icon name="language" /> OCR PGS → SRT</button>
          <button class="btn btn-ghost" on:click={onOpenOSModal} title="Chercher un SRT existant sur OpenSubtitles (gain de minutes vs OCR)"><Icon name="magnifying-glass" /> OpenSubtitles</button>
          <button class="btn btn-ghost" on:click={onOpenSync} title="Synchroniser des pistes audio (.ac3/.mka/.mkv)"><Icon name="speaker-wave" /> SYNCHRO AUDIOS</button>
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
                <Icon name="language" /> OCR · {ocrProgress.status || '…'} · <b>{ocrProgress.percent}%</b>
                {#if ocrProgress.message}— {ocrProgress.message}{/if}
              {:else if ocrProgress.status === 'done'}
                <Icon name="check-circle" /> <b>OCR terminé</b> — qualité estimée <b>{ocrProgress.quality_score.toFixed(1)}%</b>
                ({ocrProgress.subtitles} sous-titres
                {#if ocrProgress.corrected_lines}· {ocrProgress.corrected_lines} regex{/if}
                {#if ocrProgress.lt_auto_fixed}· {ocrProgress.lt_auto_fixed} LT auto{/if}
                {#if ocrProgress.lt_needs_review}· {ocrProgress.lt_needs_review} à vérifier{/if})
              {:else if ocrProgress.status === 'error'}
                <Icon name="x-circle" /> OCR échoué — {ocrProgress.message}
              {/if}
            </div>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
