<!--
  MuxActionBar : groupe de boutons à droite du header — Update / RESET /
  SETTINGS / MUX MANUEL / MUX AUTO (variantes LiHDL vs Custom PSA).
  Extraite de App.svelte en v7.1.2 (phase 1 découpage).
-->
<script>
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
      {installingUpdate ? '⟳ Installation…' : '⬇ ' + updateInfo.version}
    </button>
  {:else}
    <button class="btn btn-ghost btn-icon" on:click={onCheckUpdate} disabled={checkingUpdate}
            title="Rechercher une mise à jour">
      <span class:spin={checkingUpdate}>⟳</span>
    </button>
  {/if}
  <button class="btn btn-ghost" on:click={onReset} disabled={muxing} title="Réinitialiser la session">↻ RESET</button>
  <button class="btn" on:click={onToggleSettings}>⚙ SETTINGS</button>
  {#if muxMode === 'psa' && secondaryPath && secondaryTracksCount > 0}
    <button class="btn btn-accent" on:click={onAutomate} disabled={muxing || !sourcePath}>⚡ MUX MANUEL</button>
    <button class="btn btn-danger" on:click={onMuxAuto} disabled={muxing || !sourcePath}>🚀 MUX AUTO</button>
  {:else}
    <button class="btn btn-accent" on:click={onAutomateLihdl} disabled={muxing || !sourcePath || tracksCount === 0}>⚡ MUX MANUEL</button>
    <button class="btn btn-danger" on:click={onMuxAutoLihdl} disabled={muxing || !sourcePath || tracksCount === 0}>🚀 MUX AUTO</button>
  {/if}
</div>
