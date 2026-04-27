// Command discord-indexer scanne un (ou plusieurs) forum(s) Discord et écrit
// l'index TMDB → URL Discord dans `discord_index.json` à la racine du repo.
//
// Conçu pour tourner sans tête dans GitHub Actions :
//   - lit DISCORD_BOT_TOKEN et DISCORD_FORUM_IDS depuis l'env (séparés par
//     virgule, espace, point-virgule ou newline) ;
//   - charge l'index existant à la racine du repo (s'il existe) pour permettre
//     le scan incrémental (skip des threads dont last_message_id est inchangé) ;
//   - écrit l'index mergé en place ; le workflow décide ensuite de commit
//     ou non en regardant `git diff`.
//
// IMPORTANT : on ne logge JAMAIS le token. Les erreurs renvoyées par le
// package discordindex sont déjà sanitizées.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"go-mux-lihdl-team/internal/discordindex"
)

func main() {
	token := strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN"))
	forumsRaw := strings.TrimSpace(os.Getenv("DISCORD_FORUM_IDS"))
	if token == "" || forumsRaw == "" {
		log.Fatal("missing DISCORD_BOT_TOKEN or DISCORD_FORUM_IDS")
	}
	splitter := func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t' || r == ';'
	}
	rawTokens := strings.FieldsFunc(forumsRaw, splitter)
	forums := make([]string, 0, len(rawTokens))
	for _, t := range rawTokens {
		t = strings.TrimSpace(t)
		if t != "" {
			forums = append(forums, t)
		}
	}
	if len(forums) == 0 {
		log.Fatal("no valid forum IDs in DISCORD_FORUM_IDS")
	}

	indexPath := "discord_index.json"
	var existing *discordindex.Index
	if data, err := os.ReadFile(indexPath); err == nil {
		var idx discordindex.Index
		if err := json.Unmarshal(data, &idx); err == nil {
			if idx.Entries == nil {
				idx.Entries = map[string]discordindex.Entry{}
			}
			existing = &idx
			fmt.Printf("→ Index existant chargé : %d entrées (incrémental activé)\n", len(idx.Entries))
		} else {
			fmt.Printf("⚠ Index existant illisible (%v) — scan complet\n", err)
		}
	} else {
		fmt.Println("→ Pas d'index existant — scan complet (1ʳᵉ exécution)")
	}
	if existing == nil {
		existing = &discordindex.Index{Version: 1, Entries: map[string]discordindex.Entry{}}
	}

	merged := &discordindex.Index{
		Version:     1,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:     map[string]discordindex.Entry{},
	}
	progress := func(scanned, total int, msg string) {
		if total > 0 {
			fmt.Printf("[%d/%d] %s\n", scanned, total, msg)
		} else {
			fmt.Printf("[…] %s\n", msg)
		}
	}

	for i, fid := range forums {
		fmt.Printf("=== Forum %d/%d : %s ===\n", i+1, len(forums), fid)
		idx, err := discordindex.ScanForumIncremental(context.Background(), token, fid, existing, progress)
		if err != nil {
			log.Printf("forum %s : %v", fid, err)
			continue
		}
		for k, v := range idx.Entries {
			if e, ok := merged.Entries[k]; ok {
				if v.UpdatedAt > e.UpdatedAt {
					merged.Entries[k] = v
				}
			} else {
				merged.Entries[k] = v
			}
		}
	}

	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(indexPath, out, 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("✓ Index sauvé : %d entrées → %s\n", len(merged.Entries), indexPath)
}
