<!--
  JournalPane : card secondaire en bas droite avec tabs Journal / Queue.
  Extraite de App.svelte en v7.1 (phase 1 découpage).

  Zéro changement visuel — même markup, mêmes classes CSS (toujours dans le
  style global de App.svelte). Les props remontent au parent via les
  callbacks + bind pour logEl (le parent fait appendLog + scroll via cette
  ref).
-->
<script>
  export let logLines = [];
  export let logEl = null;         // bind:this remonté au parent pour scroll auto
  export let bottomPaneTab = 'journal';
  export let queue = [];
  export let queueAutoContinue = false;
  export let muxing = false;
  export let sourcePath = '';
  export let onQueueLoad = (i) => {};
  export let onQueueRemove = (i) => {};
  export let onQueueNext = () => {};
  export let onQueueClear = () => {};
</script>

<div class="card journal-card journal-inline">
  <div class="bottom-pane-tabs">
    <button class="bottom-pane-tab" class:active={bottomPaneTab === 'journal'} on:click={() => bottomPaneTab = 'journal'}>
      📜 Journal{logLines.length > 0 ? ` (${logLines.length})` : ''}
    </button>
    <button class="bottom-pane-tab" class:active={bottomPaneTab === 'queue'} on:click={() => bottomPaneTab = 'queue'}>
      📋 Queue{queue.length > 0 ? ` (${queue.length})` : ''}
    </button>
  </div>

  {#if bottomPaneTab === 'journal'}
    <div class="journal-scroll" bind:this={logEl}>
      {#if logLines.length === 0}
        <div class="journal-line"><span class="log-time">--:--:--</span><span class="log-msg lvl-info">Journal vide.</span></div>
      {:else}
        {#each logLines as l}
          <div class="journal-line">
            <span class="log-time">{l.time}</span>
            <span class="log-msg lvl-{l.level}">{l.text}</span>
          </div>
        {/each}
      {/if}
    </div>
  {:else}
    <!-- bind:this conservé caché pour ne pas casser appendLog/logEl scroll quand le tab Queue est actif -->
    <div hidden bind:this={logEl}>{#each logLines as l}<span>{l.text}</span>{/each}</div>
    <div class="queue-pane">
      {#if queue.length === 0}
        <div class="queue-empty">
          <div class="queue-empty-icon">📋</div>
          <div class="queue-empty-title">File vide</div>
        </div>
      {:else}
        <label class="queue-auto-toggle" title="Après chaque mux OK, charge automatiquement l'épisode suivant et relance MUX AUTO. Les corrections manuelles de labels sont propagées aux épisodes suivants.">
          <input type="checkbox" bind:checked={queueAutoContinue} disabled={muxing} />
          <span>🤖 Auto-continuer (série complète)</span>
        </label>
        <ul class="queue-list">
          {#each queue as p, i}
            <li class="queue-row" class:current={p === sourcePath}>
              <span class="queue-idx">{i + 1}</span>
              <span class="queue-name mono" title={p}>{p.split('/').pop()}</span>
              <button class="btn btn-ghost btn-tiny" on:click={() => onQueueLoad(i)} disabled={muxing}>Charger</button>
              <button class="btn btn-ghost btn-icon" on:click={() => onQueueRemove(i)} title="Retirer">✕</button>
            </li>
          {/each}
        </ul>
        <div class="queue-actions">
          <button class="btn btn-ghost btn-tiny" on:click={onQueueNext} disabled={muxing}>↓ Charger le suivant</button>
          <button class="btn btn-ghost btn-tiny" on:click={onQueueClear}>🗑 Vider</button>
        </div>
      {/if}
    </div>
  {/if}
</div>
