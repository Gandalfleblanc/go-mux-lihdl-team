<!--
  HeaderFilmBar : barre centrale du header — affiche le poster TMDB + titre +
  année + liens (TMDB, Discord), ou un placeholder « Aucun film identifié »
  avant chargement d'une source. Extraite de App.svelte en v7.1.8.
-->
<script>
  import Icon from './Icon.svelte';
  export let lastTmdbResult = null;
  export let discordIndexEntry = '';
  export let openURL = () => {};
  export let canChange = false;
  export let onChange = () => {};
</script>

{#if lastTmdbResult}
  <div class="film-bar">
    {#if lastTmdbResult.poster_url}
      <img src={lastTmdbResult.poster_url} alt="" class="film-poster" loading="lazy" />
    {:else}
      <div class="film-poster placeholder"></div>
    {/if}
    <div class="film-info">
      <div class="film-title">{lastTmdbResult.titre_fr || lastTmdbResult.titre_vo || '—'}{lastTmdbResult.annee_fr ? ` (${lastTmdbResult.annee_fr})` : ''}</div>
      <div class="film-meta">
        {#if lastTmdbResult.tmdb_id}<button type="button" class="film-id film-id-link" on:click={() => openURL(lastTmdbResult.url || `https://www.themoviedb.org/movie/${lastTmdbResult.tmdb_id}`)} title="Ouvrir la fiche TMDB"><Icon name="arrow-top-right-on-square" /> TMDB</button>{/if}
        {#if discordIndexEntry}<button type="button" class="film-id film-id-link" on:click={() => openURL(discordIndexEntry)} title="Ouvrir le post Discord de la Team"><Icon name="arrow-top-right-on-square" /> Discord</button>{/if}
        {#if lastTmdbResult.duree}<span><Icon name="clock" /> {lastTmdbResult.duree}</span>{/if}
        {#if lastTmdbResult.note > 0}<span>★ {lastTmdbResult.note}</span>{/if}
        {#if canChange}<button type="button" class="film-id film-id-link" on:click={onChange} title="Mauvaise fiche ? Revenir à la confirmation TMDB"><Icon name="pencil" /> Changer</button>{/if}
      </div>
    </div>
  </div>
{:else}
  <div class="film-bar empty">
    <div class="film-poster placeholder"></div>
    <div class="film-info">
      <div class="film-title">— Aucun film identifié —</div>
      <div class="film-meta"><span>Charge une source MKV pour démarrer</span></div>
    </div>
  </div>
{/if}
