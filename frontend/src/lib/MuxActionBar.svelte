<!--
  MuxActionBar : groupe de boutons à droite du header — Update / RESET /
  SETTINGS / MUX MANUEL / MUX AUTO (variantes LiHDL vs Custom PSA).
  Extraite de App.svelte en v7.1.2 (phase 1 découpage).
-->
<script>
  import Icon from './Icon.svelte';
  export let updateInfo = null;
  export let installingUpdate = false;
  export let checkingUpdate = false;
  export let muxing = false;
  export let sourcePath = '';
  export let muxMode = 'lihdl';
  export let secondaryPath = '';
  export let secondaryTracksCount = 0;
  export let tracksCount = 0;
  export let onInstallUpdate = () => {};
  export let onCheckUpdate = () => {};
  export let onReset = () => {};
  export let onToggleSettings = () => {};
  export let onAutomate = () => {};         // Custom PSA ⚡
  export let onMuxAuto = () => {};          // Custom PSA 🚀
  export let onAutomateLihdl = () => {};    // LiHDL ⚡
  export let onMuxAutoLihdl = () => {};     // LiHDL 🚀
</script>

<div class="header-actions">
  {#if updateInfo}
    <button class="btn update-pill available" on:click={onInstallUpdate} disabled={installingUpdate}
            title="Installer {updateInfo.version}">
      {#if installingUpdate}<span class="spin"><Icon name="arrow-path" /></span> Installation…{:else}<Icon name="download" /> {updateInfo.version}{/if}
    </button>
  {:else}
    <button class="btn btn-ghost btn-icon" on:click={onCheckUpdate} disabled={checkingUpdate}
            title="Rechercher une mise à jour">
      <span class:spin={checkingUpdate}><Icon name="arrow-path" /></span>
    </button>
  {/if}
  <button class="btn btn-ghost" on:click={onReset} disabled={muxing} title="Réinitialiser la session"><Icon name="arrow-uturn-left" /> RESET</button>
  <button class="btn" on:click={onToggleSettings}><Icon name="cog" /> SETTINGS</button>
  {#if muxMode === 'psa' && secondaryPath && secondaryTracksCount > 0}
    <button class="btn btn-accent" on:click={onAutomate} disabled={muxing || !sourcePath}><Icon name="bolt" /> MUX MANUEL</button>
    <button class="btn btn-danger" on:click={onMuxAuto} disabled={muxing || !sourcePath}><Icon name="rocket" /> MUX AUTO</button>
  {:else}
    <button class="btn btn-accent" on:click={onAutomateLihdl} disabled={muxing || !sourcePath || tracksCount === 0}><Icon name="bolt" /> MUX MANUEL</button>
    <button class="btn btn-danger" on:click={onMuxAutoLihdl} disabled={muxing || !sourcePath || tracksCount === 0}><Icon name="rocket" /> MUX AUTO</button>
  {/if}
</div>
