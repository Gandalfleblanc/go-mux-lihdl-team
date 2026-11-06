<!--
  TmdbValidateCard : card de validation TMDB avec 2 états :
   1) « AUCUNE FICHE TMDB — RECHERCHE MANUELLE » quand tmdbQuery est renseigné
      mais aucun résultat (recherche à retenter ou titre à éditer).
   2) « 📺/🎬 FICHE IDENTIFIÉE — CONFIRME POUR CONTINUER » quand lastTmdbResult
      est présent (bouton Valider ou Autre résultat).
  Extraite de App.svelte en v7.1.5 (phase 1 découpage).
-->
<script>
  import Icon from './Icon.svelte';
  export let tmdbMode = 'movie';
  export let tmdbQuery = '';
  export let tmdbIdQuery = '';
  export let tmdbSearching = false;
  export let tmdbValidated = false;
  export let lastTmdbResult = null;
  export let tmdbResults = [];
  export let tmdbResultIndex = 0;
  export let sourcePath = '';
  export let onSearchById = () => {};
  export let onManualSearch = () => {};
  export let onCycleNext = () => {};
  export let onSwitchMode = (m) => {};
</script>

<!-- État 1 : pas de résultat TMDB auto -->
{#if sourcePath && !tmdbValidated && !lastTmdbResult && tmdbQuery}
  <div class="card tmdb-validate-card">
    <div class="tmdb-validate-header">
      <div class="tmdb-validate-badge"><Icon name="exclamation-triangle" /> AUCUNE FICHE TMDB — RECHERCHE MANUELLE</div>
      <div class="tmdb-validate-force-mini" title="Forcer un ID TMDB">
        <span class="tmdb-validate-force-mini-hash">#</span>
        <input
          type="text"
          class="tmdb-validate-force-mini-input"
          bind:value={tmdbIdQuery}
          placeholder="ID TMDB"
          inputmode="numeric"
          pattern="[0-9]*"
          on:keydown={(e) => e.key === 'Enter' && onSearchById()}
        />
        <button class="tmdb-validate-force-mini-btn" on:click={onSearchById} disabled={tmdbSearching || !tmdbIdQuery} title="Forcer cet ID">
          {tmdbSearching ? '⏳' : '↻'}
        </button>
      </div>
    </div>
    <div class="tmdb-validate-body">
      <div class="tmdb-validate-poster placeholder">{tmdbMode === 'tv' ? '📺' : '🎞️'}</div>
      <div class="tmdb-validate-info">
        <div class="tmdb-validate-title" style="opacity:0.7;font-style:italic;">Aucun{tmdbMode === 'tv' ? 'e série' : ' film'} identifié{tmdbMode === 'tv' ? 'e' : ''} automatiquement</div>
        <div class="tmdb-validate-desc" style="margin-top:8px;">
          Choisis le type, édite le titre puis Entrée (ou clique <Icon name="magnifying-glass" /> Rechercher).<br>
          Ou colle un ID TMDB dans le champ <b class="mono">#</b> en haut à droite.
        </div>
        <div style="display:flex;gap:6px;margin-top:12px;">
          <button type="button" class="tmdb-mode-toggle" class:active={tmdbMode === 'movie'} on:click={() => tmdbMode = 'movie'}><Icon name="film" /> Film</button>
          <button type="button" class="tmdb-mode-toggle" class:active={tmdbMode === 'tv'} on:click={() => tmdbMode = 'tv'}><Icon name="tv" /> Série</button>
        </div>
        <div style="display:flex;gap:8px;align-items:center;margin-top:10px;">
          <input
            type="text"
            bind:value={tmdbQuery}
            placeholder={tmdbMode === 'tv' ? 'Titre de série…' : 'Titre du film…'}
            style="flex:1;padding:10px 14px;font-size:14px;background:rgba(255,255,255,0.05);border:1px solid rgba(255,255,255,0.15);border-radius:6px;color:#fff;"
            on:keydown={(e) => e.key === 'Enter' && onManualSearch()}
          />
        </div>
      </div>
    </div>
    <div class="tmdb-validate-actions">
      <button class="btn btn-accent tmdb-validate-cta" on:click={onManualSearch} disabled={tmdbSearching || !tmdbQuery.trim()}>
        {tmdbSearching ? '⏳ Recherche…' : `🔍 Rechercher ${tmdbMode === 'tv' ? 'la série' : 'le film'} sur TMDB`}
      </button>
    </div>
  </div>
{/if}

<!-- État 2 : fiche identifiée, à confirmer -->
{#if sourcePath && !tmdbValidated && lastTmdbResult}
  <div class="card tmdb-validate-card">
    <div class="tmdb-validate-header">
      <div class="tmdb-validate-badge"><Icon name={tmdbMode === 'tv' ? 'tv' : 'film'} /> {tmdbMode === 'tv' ? 'SÉRIE IDENTIFIÉE' : 'FILM IDENTIFIÉ'} — CONFIRME POUR CONTINUER</div>
      <div style="display:flex;gap:8px;align-items:center;">
        <div style="display:flex;gap:4px;" title="Basculer entre recherche Film / Série et relancer">
          <button type="button" class="tmdb-mode-toggle small" class:active={tmdbMode === 'movie'} on:click={() => onSwitchMode('movie')} disabled={tmdbSearching}><Icon name="film" /></button>
          <button type="button" class="tmdb-mode-toggle small" class:active={tmdbMode === 'tv'} on:click={() => onSwitchMode('tv')} disabled={tmdbSearching}><Icon name="tv" /></button>
        </div>
        <div class="tmdb-validate-force-mini" title="Pas le bon film ? Forcer l'ID TMDB">
          <span class="tmdb-validate-force-mini-hash">#</span>
          <input
            type="text"
            class="tmdb-validate-force-mini-input"
            bind:value={tmdbIdQuery}
            placeholder="ID TMDB"
            inputmode="numeric"
            pattern="[0-9]*"
            on:keydown={(e) => e.key === 'Enter' && onSearchById()}
          />
          <button class="tmdb-validate-force-mini-btn" on:click={onSearchById} disabled={tmdbSearching || !tmdbIdQuery} title="Forcer cet ID">
            {tmdbSearching ? '⏳' : '↻'}
          </button>
        </div>
      </div>
    </div>
    <div class="tmdb-validate-body">
      {#if lastTmdbResult.poster_url}
        <img src={lastTmdbResult.poster_url} alt="Affiche du film" class="tmdb-validate-poster" />
      {:else}
        <div class="tmdb-validate-poster placeholder"><Icon name="film" /></div>
      {/if}
      <div class="tmdb-validate-info">
        <div class="tmdb-validate-title">{lastTmdbResult.titre_fr || lastTmdbResult.titre_vo || '—'}{lastTmdbResult.annee_fr ? ` (${lastTmdbResult.annee_fr})` : ''}</div>
        {#if lastTmdbResult.titre_vo && lastTmdbResult.titre_fr && lastTmdbResult.titre_vo !== lastTmdbResult.titre_fr}
          <div class="tmdb-validate-original">VO : {lastTmdbResult.titre_vo}</div>
        {/if}
        <div class="tmdb-validate-meta">
          {#if lastTmdbResult.tmdb_id}<span class="tmdb-validate-pill">TMDB {lastTmdbResult.tmdb_id}</span>{/if}
          {#if lastTmdbResult.duree}<span class="tmdb-validate-pill"><Icon name="clock" /> {lastTmdbResult.duree}</span>{/if}
          {#if lastTmdbResult.note > 0}<span class="tmdb-validate-pill rating">★ {lastTmdbResult.note}</span>{/if}
          {#if lastTmdbResult.original_language}<span class="tmdb-validate-pill"><Icon name="globe-alt" /> {String(lastTmdbResult.original_language).toUpperCase()}</span>{/if}
        </div>
        {#if lastTmdbResult.description || lastTmdbResult.overview || lastTmdbResult.synopsis}
          <div class="tmdb-validate-desc">{lastTmdbResult.description || lastTmdbResult.overview || lastTmdbResult.synopsis}</div>
        {/if}
      </div>
    </div>
    <div class="tmdb-validate-actions">
      {#if tmdbResults && tmdbResults.length > 1}
        <button class="btn btn-ghost" on:click={onCycleNext} title="Cycler vers le résultat suivant si la fiche affichée n'est pas la bonne">
          <Icon name="arrow-path" /> Autre résultat ({tmdbResultIndex + 1}/{tmdbResults.length})
        </button>
      {/if}
      <button class="btn btn-accent tmdb-validate-cta" on:click={() => { tmdbValidated = true; }}><Icon name="check" /> {tmdbMode === 'tv' ? "C'est bien cette série" : "C'est bien ce film"} — continuer</button>
    </div>
  </div>
{/if}
