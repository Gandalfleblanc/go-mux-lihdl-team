package discordindex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// PushToGitHub écrit ou met à jour un fichier dans un repo GitHub via l'API
// Contents (PUT /repos/{owner}/{repo}/contents/{path}).
//
// Paramètres :
//   - token  : PAT GitHub (scope `repo`)
//   - repo   : "owner/name" (ex : "Gandalfleblanc/go-mux-lihdl-team")
//   - branch : nom de branche (défaut "main" si vide)
//   - path   : chemin du fichier dans le repo (ex : "discord_index.json")
//   - content: contenu du fichier (octets bruts ; sera base64-encodé en interne)
//   - message: message du commit
//
// Retourne le SHA du nouveau blob ou une erreur. Aucun token ni payload n'est
// jamais loggé : les erreurs sont sanitizées.
func PushToGitHub(ctx context.Context, token, repo, branch, path string, content []byte, message string) (string, error) {
	token = strings.TrimSpace(token)
	repo = strings.TrimSpace(repo)
	branch = strings.TrimSpace(branch)
	path = strings.TrimSpace(strings.TrimPrefix(path, "/"))

	if token == "" {
		return "", errors.New("token GitHub manquant (Réglages → Index Discord → Push GitHub)")
	}
	if repo == "" || !strings.Contains(repo, "/") {
		return "", errors.New("repo GitHub invalide — format attendu : owner/name")
	}
	if branch == "" {
		branch = "main"
	}
	if path == "" {
		return "", errors.New("path du fichier GitHub manquant")
	}
	if message == "" {
		message = fmt.Sprintf("chore(discord-index): update %s [%s]", path, time.Now().UTC().Format(time.RFC3339))
	}

	client := &http.Client{Timeout: 30 * time.Second}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", repo, path)

	// 1. Récupère le SHA actuel du fichier (s'il existe) — requis pour update.
	sha, err := getCurrentSHA(ctx, client, token, apiURL, branch)
	if err != nil {
		return "", sanitizeErr(err, token)
	}

	// 2. PUT le nouveau contenu.
	payload := map[string]any{
		"message": message,
		"content": base64.StdEncoding.EncodeToString(content),
		"branch":  branch,
	}
	if sha != "" {
		payload["sha"] = sha
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "PUT", apiURL, bytes.NewReader(body))
	if err != nil {
		return "", sanitizeErr(err, token)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", sanitizeErr(err, token)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// On extrait le message d'erreur GitHub si présent (sans le token).
		var ge struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(respBody, &ge)
		msg := strings.TrimSpace(ge.Message)
		if msg == "" {
			msg = strings.TrimSpace(string(respBody))
		}
		return "", fmt.Errorf("GitHub API %d : %s", resp.StatusCode, sanitizeStr(msg, token))
	}

	var ok struct {
		Content struct {
			SHA string `json:"sha"`
		} `json:"content"`
	}
	_ = json.Unmarshal(respBody, &ok)
	return ok.Content.SHA, nil
}

// getCurrentSHA renvoie le SHA actuel du fichier (vide si n'existe pas).
func getCurrentSHA(ctx context.Context, client *http.Client, token, apiURL, branch string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL+"?ref="+branch, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil // fichier n'existe pas encore — création OK sans SHA
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GET contents %d : %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var meta struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return "", err
	}
	return meta.SHA, nil
}

// sanitizeErr retire le token éventuel du message d'erreur.
func sanitizeErr(err error, token string) error {
	if err == nil {
		return nil
	}
	s := sanitizeStr(err.Error(), token)
	if s == err.Error() {
		return err
	}
	return errors.New(s)
}

// sanitizeStr remplace le token par [redacted] dans une string.
func sanitizeStr(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "[redacted]")
}
