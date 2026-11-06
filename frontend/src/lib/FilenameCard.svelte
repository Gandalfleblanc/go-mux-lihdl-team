<!--
  FilenameCard : preview du nom de fichier final avec toggle Auto/VF/VO,
  mode override manuel, et actions Modifier/Copier/Dossier.
  Extraite de App.svelte en v7.1.3 (phase 1 découpage).
-->
<script>
  import Icon from './Icon.svelte';
  export let lastTmdbResult = null;
  export let target = { lang: 'auto' };
  export let filenameOverride = false;
  export let manualFilename = '';
  export let previewFilename = '';
  export let effectiveFilename = '';
  export let filenameCopied = false;
  export let effectiveOutputDir = () => '';
  export let onCopy = () => {};
  export let onOpenDir = () => {};
  export let onStartOverride = () => {};
  export let onResetOverride = () => {};
</script>

<div class="card">
  <div class="card-title"><Icon name="document-text" /> Nom de fichier final</div>
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
      <button class="btn btn-ghost btn-tiny" on:click={onResetOverride} title="Revenir à l'auto"><Icon name="arrow-uturn-left" /> Auto</button>
      {#if effectiveFilename}
        <button class="btn btn-ghost btn-tiny" on:click={onCopy}>{filenameCopied ? '✓ Copié' : '📋 Copier'}</button>
        <button class="btn btn-ghost btn-tiny" on:click={onOpenDir} disabled={!effectiveOutputDir()}><Icon name="folder-open" /> Dossier</button>
      {/if}
    </div>
  {:else}
    <div class="filename mono">{previewFilename || '—'}</div>
    <div class="filename-actions">
      {#if previewFilename}
        <button class="btn btn-ghost btn-tiny" on:click={onStartOverride} title="Modifier manuellement"><Icon name="pencil" /> Modifier</button>
      {/if}
      {#if effectiveFilename}
        <button class="btn btn-ghost btn-tiny" on:click={onCopy}>{filenameCopied ? '✓ Copié' : '📋 Copier'}</button>
        <button class="btn btn-ghost btn-tiny" on:click={onOpenDir} disabled={!effectiveOutputDir()}><Icon name="folder-open" /> Dossier</button>
      {/if}
    </div>
  {/if}
</div>
