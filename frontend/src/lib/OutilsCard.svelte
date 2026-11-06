<!--
  OutilsCard : card « Outils additionnels » affichée sous les pistes quand une
  source est chargée + TMDB validé. Boutons d'accès aux écrans sync / cible /
  OCR / OpenSubtitles, et barre de progression OCR standalone.
  Extraite de App.svelte en v7.1.1 (phase 1 découpage).

  Zéro changement visuel — classes CSS toujours dans le style global.
-->
<script>
  import Icon from './Icon.svelte';
  export let ocrRunning = false;
  export let ocrTrackId = -1;
  export let ocrProgress = { status: '', percent: 0, message: '', subtitles: 0, corrected_lines: 0, lt_auto_fixed: 0, lt_needs_review: 0, quality_score: 0 };
  export let showLTReview = false;
  export let onOpenSync = () => {};
  export let onOpenCible = () => {};
  export let onPickAndOCR = () => {};
  export let onOpenOSModal = () => {};
</script>

<div class="card">
  <div class="card-title"><Icon name="speaker-wave" /> Outils additionnels</div>
  <div class="tools-row">
    <button class="btn btn-ghost btn-tiny" on:click={onOpenSync}><Icon name="speaker-wave" /> Synchro audios</button>
    <button class="btn btn-ghost btn-tiny" on:click={onOpenCible}><Icon name="viewfinder-circle" /> Vue Cible détaillée</button>
    <button class="btn btn-ghost btn-tiny" on:click={onPickAndOCR} disabled={ocrRunning} title="OCR PGS .sup → SRT (Tesseract + cleanup regex FR)"><Icon name="language" /> OCR PGS → SRT</button>
    <button class="btn btn-ghost btn-tiny" on:click={onOpenOSModal} title="Chercher un SRT existant sur OpenSubtitles"><Icon name="magnifying-glass" /> OpenSubtitles</button>
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
