// Package discordindex builds and consumes a JSON index that maps a TMDB ID
// to the URL of its Discord post inside a Team forum channel.
//
// Two roles:
//   - admin : configure un bot Discord + un forum channel ID, lance ScanForum
//     pour produire le JSON, puis publie ce JSON manuellement (ex: GitHub raw).
//   - user  : récupère ce JSON via FetchRemoteIndex (cache 24h) et fait des
//     LookupTmdb pour afficher un bouton "↗ Discord" si une entrée existe.
//
// Sécurité : le bot Discord (et son token) n'est JAMAIS appelé côté user.
// Aucune méthode de ce package n'utilise le token sauf ScanForum, qui n'est
// invoqué que par l'action explicite "Mettre à jour l'index" dans Settings.
package discordindex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	discordAPI = "https://discord.com/api/v10"
	// Pause entre deux requêtes Discord. Discord autorise ~50 r/s globalement ;
	// 400ms = 2.5 r/s, soit 20× sous la limite — confortable même avec plusieurs
	// forums scannés en parallèle. Les retries 429 sont gérés via Retry-After
	// dans discordGet, donc on peut être agressif sans risque.
	rateDelay = 400 * time.Millisecond
	// Durée du cache du JSON remote côté user.
	remoteCacheTTL = 24 * time.Hour
	userAgent      = "GoMuxLiHDL-DiscordIndex/1.0"
)

// Entry est une entrée de l'index : un TMDB ID → un post Discord.
//
// LastMessageID + ThreadID sont stockés pour le scan incrémental :
// si Discord renvoie le même last_message_id pour ce thread qu'au scan
// précédent, on sait que rien n'a bougé et on peut skipper le fetch du
// premier message.
type Entry struct {
	TmdbID        string `json:"tmdb_id"`
	URL           string `json:"url"`
	Title         string `json:"title,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
	ThreadID      string `json:"thread_id,omitempty"`
	LastMessageID string `json:"last_message_id,omitempty"`
}

// Index est le format JSON sérialisé.
type Index struct {
	Version     int              `json:"version"`
	GeneratedAt string           `json:"generated_at"`
	Entries     map[string]Entry `json:"entries"`
}

// tmdbRegex extrait l'ID numérique d'un lien themoviedb.org/movie/<id> ou /tv/<id>.
var tmdbRegex = regexp.MustCompile(`themoviedb\.org/(?:movie|tv)/(\d+)`)

// --- Discord API minimal types ---

type discordThreadList struct {
	Threads []discordChannel `json:"threads"`
	HasMore bool             `json:"has_more"`
}

type discordChannel struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	LastMessageID  string                 `json:"last_message_id,omitempty"`
	ThreadMetadata *discordThreadMetadata `json:"thread_metadata,omitempty"`
}

type discordThreadMetadata struct {
	Archived     bool   `json:"archived"`
	ArchiveTimestamp string `json:"archive_timestamp"`
}

type discordMessage struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	ChannelID string `json:"channel_id"`
	Timestamp string `json:"timestamp"`
}

// --- Public API ---

// ScanForum interroge l'API Discord et bâtit l'index.
//   botToken       : "Bot xxx" sans le préfixe (le préfixe est ajouté ici).
//   forumChannelID : l'ID numérique du forum channel (ex: "1234…").
//   progressFn     : callback (scanned, total, message) ; total peut être 0
//                    quand on ne connaît pas encore le compte.
//
// Délègue à ScanForumIncremental sans index existant — équivalent à un scan
// complet "from scratch". Conservé pour compat API.
func ScanForum(ctx context.Context, botToken, forumChannelID string, progressFn func(int, int, string)) (*Index, error) {
	return ScanForumIncremental(ctx, botToken, forumChannelID, nil, progressFn)
}

// ScanForumIncremental fait la même chose que ScanForum mais réutilise les
// entries d'un index précédent (existing) quand le LastMessageID du thread
// Discord est inchangé → skip du fetch du premier message du thread.
//
// existing peut être nil (1er scan) ; dans ce cas tous les threads sont fetchés.
//
// Implémentation : récupère active threads + archived public threads (paginé
// par before=ts), puis pour chaque thread :
//   1. si on a une entry existante avec le même thread_id ET le même
//      last_message_id, on la réutilise telle quelle (pas de fetch HTTP).
//   2. sinon on lit le 1er message du thread (le post initial d'un forum
//      thread porte le même ID que le thread) et on extrait le TMDB ID.
//
// Les entries "orphelines" (présentes dans existing mais dont on ne re-voit
// plus le thread Discord) sont CONSERVÉES par défaut — Discord peut très bien
// nous renvoyer une page paginée incomplète, on évite de purger sur un scan
// éphémèrement bancal. La purge de threads vraiment supprimés se fera sur
// décision admin (suppression manuelle de l'entry).
func ScanForumIncremental(ctx context.Context, botToken, forumChannelID string, existing *Index, progressFn func(int, int, string)) (*Index, error) {
	botToken = strings.TrimSpace(botToken)
	forumChannelID = strings.TrimSpace(forumChannelID)
	if botToken == "" {
		return nil, errors.New("discordindex: bot token vide")
	}
	if forumChannelID == "" {
		return nil, errors.New("discordindex: forum channel ID vide")
	}
	if progressFn == nil {
		progressFn = func(int, int, string) {}
	}

	// Index reverse : thread_id → entry, pour retrouver une entry à partir du
	// thread sans scanner toutes les entries (clé d'index = TMDB ID, pas
	// thread ID). Construit une seule fois.
	byThread := map[string]Entry{}
	if existing != nil {
		for _, e := range existing.Entries {
			if e.ThreadID != "" {
				byThread[e.ThreadID] = e
			}
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}

	progressFn(0, 0, "Récupération des threads actifs…")
	active, err := listActiveThreads(ctx, client, botToken, forumChannelID)
	if err != nil {
		return nil, fmt.Errorf("threads actifs: %w", err)
	}

	progressFn(0, len(active), fmt.Sprintf("%d threads actifs — récupération des archives…", len(active)))
	archived, err := listAllArchivedThreads(ctx, client, botToken, forumChannelID, progressFn)
	if err != nil {
		return nil, fmt.Errorf("threads archivés: %w", err)
	}

	all := append([]discordChannel{}, active...)
	all = append(all, archived...)
	total := len(all)
	progressFn(0, total, fmt.Sprintf("%d threads à scanner (%d actifs + %d archivés)", total, len(active), len(archived)))

	idx := &Index{
		Version:     1,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:     map[string]Entry{},
	}
	// Set des thread IDs vus sur ce forum dans ce scan, pour préserver les
	// entries orphelines APPARTENANT à ce forum (on ne les ré-injecte que si
	// on n'a pas vu le thread — mais on les garde quand même, vu qu'on ne
	// distingue pas "Discord en a oublié de nous le retourner" de "supprimé").
	seenThreads := make(map[string]bool, len(all))
	for _, t := range all {
		seenThreads[t.ID] = true
	}

	// Résout le guild_id une seule fois pour construire les URLs des messages.
	guildID, _ := fetchGuildIDForChannel(ctx, client, botToken, forumChannelID)
	scanned := 0
	reused := 0
	for _, t := range all {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		scanned++

		// Skip incrémental : si on connaît ce thread et que son last_message_id
		// est inchangé depuis le dernier scan → réutilise l'entry telle quelle.
		// Note : l'API Discord renvoie last_message_id dans les payloads de
		// thread (active ET archived/public). Si elle ne le renvoie pas, on
		// ne peut pas trancher, donc on refait le fetch (sécurité).
		if prev, ok := byThread[t.ID]; ok && t.LastMessageID != "" && prev.LastMessageID == t.LastMessageID {
			idx.Entries[prev.TmdbID] = prev
			reused++
			if scanned%50 == 0 || scanned == total {
				progressFn(scanned, total, fmt.Sprintf("⏭ %s (inchangé, %d réutilisés)", t.Name, reused))
			}
			continue
		}

		// Throttle : on attend AVANT chaque requête réseau (skip n'attend pas).
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(rateDelay):
		}

		msg, err := fetchOriginalPostMessage(ctx, client, botToken, t.ID)
		if err != nil {
			progressFn(scanned, total, fmt.Sprintf("⚠ %s : %s", t.Name, err.Error()))
			// On garde quand même l'entry précédente si elle existait — on n'efface
			// pas un travail antérieur sur un échec ponctuel.
			if prev, ok := byThread[t.ID]; ok {
				idx.Entries[prev.TmdbID] = prev
			}
			continue
		}
		if msg == nil {
			progressFn(scanned, total, fmt.Sprintf("⚠ %s : aucun message", t.Name))
			if prev, ok := byThread[t.ID]; ok {
				idx.Entries[prev.TmdbID] = prev
			}
			continue
		}
		tmdbID := extractTmdbID(msg.Content)
		if tmdbID == "" {
			progressFn(scanned, total, fmt.Sprintf("· %s : pas de lien TMDB", t.Name))
			continue
		}
		// Construit l'URL du message Discord (deeplink) — guild résolu une fois
		// avant la boucle. Si pas de guild résolu, on tombe sur "@me" qui ouvre
		// quand même le thread côté client desktop Discord.
		gid := guildID
		if gid == "" {
			gid = "@me"
		}
		url := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", gid, t.ID, msg.ID)
		// last_message_id pour le scan suivant : si l'API ne le renvoie pas,
		// on stocke au minimum l'ID du 1er message qu'on vient de fetcher
		// (mieux que vide ; sera remplacé au prochain scan si Discord renvoie
		// le vrai last_message_id du thread).
		lastMsg := t.LastMessageID
		if lastMsg == "" {
			lastMsg = msg.ID
		}
		idx.Entries[tmdbID] = Entry{
			TmdbID:        tmdbID,
			URL:           url,
			Title:         t.Name,
			UpdatedAt:     msg.Timestamp,
			ThreadID:      t.ID,
			LastMessageID: lastMsg,
		}
		progressFn(scanned, total, fmt.Sprintf("✓ TMDB %s — %s", tmdbID, t.Name))
	}

	// Préserve les entries orphelines de ce forum (threads qu'on n'a pas vu
	// retomber dans active+archived sur ce scan). Discord paginer mal arrive ;
	// dans le doute on garde. Une vraie purge se fait à la main côté admin.
	if existing != nil {
		for _, e := range existing.Entries {
			if e.ThreadID == "" {
				continue
			}
			if seenThreads[e.ThreadID] {
				continue
			}
			// thread non revu : on garde si pas déjà présent dans le merge
			if _, ok := idx.Entries[e.TmdbID]; !ok {
				idx.Entries[e.TmdbID] = e
			}
		}
	}

	progressFn(total, total, fmt.Sprintf("Terminé : %d entrées TMDB indexées sur %d threads (%d réutilisés sans fetch)", len(idx.Entries), total, reused))
	return idx, nil
}

// SaveIndex écrit le JSON dans le fichier passé. Permissions 0644 (pas de
// secret dans ce fichier — c'est par construction destiné à être public).
func SaveIndex(idx *Index, path string) error {
	if idx == nil {
		return errors.New("discordindex: index nil")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadIndex lit le JSON local. Retourne (nil, nil) si le fichier n'existe pas.
func LoadIndex(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	if idx.Entries == nil {
		idx.Entries = map[string]Entry{}
	}
	return &idx, nil
}

// FetchRemoteIndex télécharge le JSON depuis l'URL (typiquement GitHub raw).
// Cache local : si le fichier existe et est plus récent que remoteCacheTTL,
// on le réutilise sans toucher au réseau. Si l'URL est vide, retourne le cache
// éventuel ou nil. Best-effort : en cas d'erreur réseau on retourne le cache
// précédent s'il existe.
func FetchRemoteIndex(ctx context.Context, url, cachePath string) (*Index, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		// pas de remote configuré → on utilise juste le cache local s'il existe
		return LoadIndex(cachePath)
	}

	// Cache encore valide ?
	if info, err := os.Stat(cachePath); err == nil {
		if time.Since(info.ModTime()) < remoteCacheTTL {
			if idx, err := LoadIndex(cachePath); err == nil && idx != nil {
				return idx, nil
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return LoadIndex(cachePath)
	}
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// fallback sur cache
		if idx, _ := LoadIndex(cachePath); idx != nil {
			return idx, nil
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if idx, _ := LoadIndex(cachePath); idx != nil {
			return idx, nil
		}
		return nil, fmt.Errorf("discordindex: remote HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		if idx, _ := LoadIndex(cachePath); idx != nil {
			return idx, nil
		}
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		if idx2, _ := LoadIndex(cachePath); idx2 != nil {
			return idx2, nil
		}
		return nil, err
	}
	if idx.Entries == nil {
		idx.Entries = map[string]Entry{}
	}
	// Persist cache (best effort)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err == nil {
		_ = os.WriteFile(cachePath, data, 0644)
	}
	return &idx, nil
}

// LookupTmdb retourne l'URL Discord pour ce TMDB ID, ou "" si pas trouvé.
func LookupTmdb(idx *Index, tmdbID string) string {
	if idx == nil || idx.Entries == nil {
		return ""
	}
	tmdbID = strings.TrimSpace(tmdbID)
	if tmdbID == "" {
		return ""
	}
	if e, ok := idx.Entries[tmdbID]; ok {
		return e.URL
	}
	// tolérer les TMDB IDs stockés sans/avec leading zero
	if n, err := strconv.Atoi(tmdbID); err == nil {
		if e, ok := idx.Entries[strconv.Itoa(n)]; ok {
			return e.URL
		}
	}
	return ""
}

// --- Helpers HTTP Discord ---

func discordGet(ctx context.Context, client *http.Client, botToken, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discordAPI+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+botToken)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		// rate-limited : on respecte Retry-After (en secondes float)
		ra := resp.Header.Get("Retry-After")
		secs, _ := strconv.ParseFloat(ra, 64)
		if secs <= 0 {
			secs = 2
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(secs*1000) * time.Millisecond):
		}
		return discordGet(ctx, client, botToken, path, out)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		// IMPORTANT : ne JAMAIS logger le token. On ne logge que le path et le code.
		return fmt.Errorf("Discord HTTP %d %s : %s", resp.StatusCode, path, sanitizeBody(string(body)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// sanitizeBody supprime tout ce qui ressemble à un Authorization (paranoïa).
func sanitizeBody(s string) string {
	if i := strings.Index(strings.ToLower(s), "authorization"); i >= 0 {
		return "[redacted]"
	}
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

func listActiveThreads(ctx context.Context, client *http.Client, botToken, forumID string) ([]discordChannel, error) {
	// /guilds/{guild.id}/threads/active liste TOUS les threads actifs du
	// guild — on n'a pas le guild ici. À la place, l'endpoint forum-channel-
	// scoped n'existe pas pour les actifs ; on dérive le guild via /channels/{id}.
	guildID, err := fetchGuildIDForChannel(ctx, client, botToken, forumID)
	if err != nil || guildID == "" {
		// fallback : on retourne vide, on s'en remet aux archived.
		return nil, nil
	}
	// L'API guild-active liste TOUS les threads du guild ; on récupère parent_id
	// pour filtrer ceux qui appartiennent à notre forum channel.
	var raw struct {
		Threads []map[string]interface{} `json:"threads"`
	}
	if err := discordGet(ctx, client, botToken, "/guilds/"+guildID+"/threads/active", &raw); err != nil {
		return nil, err
	}
	out := make([]discordChannel, 0, len(raw.Threads))
	for _, m := range raw.Threads {
		parent, _ := m["parent_id"].(string)
		if parent != forumID {
			continue
		}
		id, _ := m["id"].(string)
		name, _ := m["name"].(string)
		lastMsg, _ := m["last_message_id"].(string)
		out = append(out, discordChannel{ID: id, Name: name, LastMessageID: lastMsg})
	}
	return out, nil
}

func listAllArchivedThreads(ctx context.Context, client *http.Client, botToken, forumID string, progressFn func(int, int, string)) ([]discordChannel, error) {
	out := []discordChannel{}
	before := ""
	page := 0
	for {
		page++
		path := "/channels/" + forumID + "/threads/archived/public?limit=100"
		if before != "" {
			path += "&before=" + before
		}
		if progressFn != nil {
			progressFn(0, 0, fmt.Sprintf("Archives : page %d (%d threads récupérés)…", page, len(out)))
		}
		var resp discordThreadList
		if err := discordGet(ctx, client, botToken, path, &resp); err != nil {
			return out, err
		}
		out = append(out, resp.Threads...)
		if progressFn != nil {
			progressFn(0, 0, fmt.Sprintf("Archives : page %d → %d threads cumulés", page, len(out)))
		}
		if !resp.HasMore || len(resp.Threads) == 0 {
			break
		}
		// pagination : before = archive_timestamp du dernier thread
		last := resp.Threads[len(resp.Threads)-1]
		if last.ThreadMetadata == nil || last.ThreadMetadata.ArchiveTimestamp == "" {
			break
		}
		before = last.ThreadMetadata.ArchiveTimestamp
		// throttle entre pages
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(rateDelay):
		}
	}
	return out, nil
}

// fetchOriginalPostMessage récupère le message d'origine d'un thread forum.
// Dans un forum channel : le post initial est un message dont l'ID == ID du
// thread. On le récupère via /channels/<thread>/messages/<thread>.
func fetchOriginalPostMessage(ctx context.Context, client *http.Client, botToken, threadID string) (*discordMessage, error) {
	var msg discordMessage
	err := discordGet(ctx, client, botToken, "/channels/"+threadID+"/messages/"+threadID, &msg)
	if err == nil && msg.ID != "" {
		return &msg, nil
	}
	// Fallback : derniers messages, on prend le plus ancien.
	var msgs []discordMessage
	if err2 := discordGet(ctx, client, botToken, "/channels/"+threadID+"/messages?limit=50", &msgs); err2 != nil {
		return nil, err2
	}
	if len(msgs) == 0 {
		return nil, nil
	}
	oldest := &msgs[len(msgs)-1]
	return oldest, nil
}

// fetchGuildIDForChannel résout le guild_id d'un channel via /channels/{id}.
func fetchGuildIDForChannel(ctx context.Context, client *http.Client, botToken, channelID string) (string, error) {
	var resp struct {
		GuildID string `json:"guild_id"`
	}
	if err := discordGet(ctx, client, botToken, "/channels/"+channelID, &resp); err != nil {
		return "", err
	}
	return resp.GuildID, nil
}

func extractTmdbID(content string) string {
	m := tmdbRegex.FindStringSubmatch(content)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}
