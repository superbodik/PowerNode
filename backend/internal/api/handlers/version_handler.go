package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type VersionHandler struct {
	Commit    string
	BuildDate string
	SourceDir string
	RepoSlug  string
}

func (h *VersionHandler) Get(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"commit":     h.Commit,
		"build_date": h.BuildDate,
		"source_dir": h.SourceDir,
		"repo_slug":  h.RepoSlug,
	})
}

func (h *VersionHandler) CheckUpdate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	latest, err := latestCommitSHA(ctx, h.RepoSlug)
	if err != nil {
		http.Error(w, "failed to reach GitHub: "+err.Error(), http.StatusBadGateway)
		return
	}

	updateAvailable := h.Commit != "unknown" && h.Commit != "" && !strings.HasPrefix(latest, h.Commit)

	writeJSON(w, http.StatusOK, map[string]any{
		"current_commit":   h.Commit,
		"latest_commit":    latest[:min(12, len(latest))],
		"update_available": updateAvailable,
	})
}

func latestCommitSHA(ctx context.Context, repoSlug string) (string, error) {
	url := "https://api.github.com/repos/" + repoSlug + "/commits/main"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var body struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.SHA, nil
}
