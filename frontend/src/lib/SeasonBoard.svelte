<!--
  SeasonBoard : tableau de bord du mode Saison. Une carte par épisode (fichiers
  associés, étape en cours, nom de sortie, remarques), progression globale,
  boutons Lancer / Arrêter / Fermer et fin du journal.
-->
<script>
  import Icon from './Icon.svelte';

  let { season, logTail = [], onStart, onStop, onClose, onRelaunchEpisode } = $props();

  const STATUS = {
    pending: { label: 'En attente', icon: 'clock' },
    running: { label: 'En cours', icon: 'arrow-path' },
    done:    { label: 'Muxé', icon: 'check-circle' },
    review:  { label: 'À vérifier', icon: 'exclamation-triangle' },
    error:   { label: 'Erreur', icon: 'x-circle' },
    skipped: { label: 'Ignoré', icon: 'stop' },
  };

  const name = (p) => String(p || '').split('/').pop();
  let total = $derived(season.episodes.length);
  let finished = $derived(season.episodes.filter((e) => ['done', 'review', 'error', 'skipped'].includes(e.status)).length);
  let doneCount = $derived(season.episodes.filter((e) => e.status === 'done').length);
  let reviewCount = $derived(season.episodes.filter((e) => e.status === 'review' || e.status === 'error').length);
  let blocking = $derived(season.episodes.filter((e) => e.issues.length && (!e.source || (season.mode === 'psa' && !e.supply))).length);
  let pct = $derived(total ? Math.round((finished / total) * 100) : 0);
</script>

<div class="season">
  <header class="season-head card">
    <div class="season-title">
      <div class="season-kicker">Mode Saison · {season.mode === 'psa' ? 'Custom PSA' : 'MUX LiHDL'}</div>
      <h2>{name(season.dir)}</h2>
      <div class="season-stats">
        <span><b>{total}</b> épisodes</span>
        <span class="ok"><b>{doneCount}</b> muxés</span>
        {#if reviewCount}<span class="warn"><b>{reviewCount}</b> à vérifier</span>{/if}
        {#if season.ignored.length}<span><b>{season.ignored.length}</b> fichier(s) ignoré(s), sans SxxExx</span>{/if}
      </div>
    </div>
    <div class="season-actions">
      {#if season.running}
        <button class="btn" onclick={onStop} disabled={season.stopRequested}>
          <Icon name="stop" /> {season.stopRequested ? 'Arrêt après cet épisode…' : 'Arrêter après cet épisode'}
        </button>
      {:else}
        <button class="btn btn-ghost" onclick={onClose}><Icon name="arrow-uturn-left" /> Fermer</button>
        <button class="btn btn-danger" onclick={onStart} disabled={total === 0 || finished === total}>
          <Icon name="rocket" /> {finished > 0 && finished < total ? 'Reprendre la saison' : 'Lancer la saison'}
        </button>
      {/if}
    </div>
    <div class="season-progress"><div class="season-progress-fill" style="width: {pct}%"></div></div>
  </header>

  {#if blocking && !season.running}
    <div class="season-banner"><Icon name="exclamation-triangle" /> {blocking} épisode(s) incomplet(s) seront ignorés. Ajoute les fichiers manquants dans le dossier puis rouvre la saison.</div>
  {/if}

  <div class="season-grid">
    {#each season.episodes as ep, i (ep.episode)}
      {@const st = STATUS[ep.status] || STATUS.pending}
      <article class="ep-card status-{ep.status}" class:current={season.currentIdx === i && season.running}>
        <div class="ep-top">
          <div class="ep-code">{ep.episode}</div>
          <div class="ep-status"><span class:spin={ep.status === 'running'}><Icon name={st.icon} /></span> {st.label}</div>
        </div>
        <div class="ep-files">
          <div class="ep-file" title={ep.source}><span class="ep-file-tag">{season.mode === 'psa' ? 'PSA' : 'SRC'}</span>{name(ep.source) || '— manquant —'}</div>
          {#if season.mode === 'psa'}
            <div class="ep-file" title={ep.supply}><span class="ep-file-tag sup">SUP</span>{name(ep.supply) || '— manquant —'}</div>
          {/if}
        </div>
        {#if ep.status === 'running' && ep.step}
          <div class="ep-step">{ep.step}…</div>
        {/if}
        {#if ep.output}
          <div class="ep-output" title={ep.output}>{ep.output}</div>
        {/if}
        {#each [...ep.issues, ...ep.notes] as n}
          <div class="ep-note"><Icon name="exclamation-triangle" /> {n}</div>
        {/each}
        {#if !season.running && (ep.status === 'review' || ep.status === 'error')}
          <button class="btn btn-ghost btn-tiny ep-open" onclick={() => onRelaunchEpisode(i)}><Icon name="folder-open" /> Ouvrir cet épisode à la main</button>
        {/if}
      </article>
    {/each}
  </div>

  {#if logTail.length}
    <div class="season-log card">
      {#each logTail as l}
        <div class="journal-line"><span class="log-time">{l.time}</span> <span class="log-msg lvl-{l.level}">{l.text}</span></div>
      {/each}
    </div>
  {/if}
</div>
