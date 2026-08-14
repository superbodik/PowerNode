package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EggHandler struct {
	DB *pgxpool.Pool
}

type eggVariableSummary struct {
	Name         string `json:"name"`
	EnvVariable  string `json:"env_variable"`
	DefaultValue string `json:"default_value"`
	IsEditable   bool   `json:"is_editable"`
	Rules        string `json:"rules"`
	// IsPort marks the single variable (at most one per egg) that should be
	// kept in sync with whatever port the server actually gets allocated --
	// see 0032_egg_variable_port_and_secret_flags.sql.
	IsPort bool `json:"is_port"`
	// AutoGenerate marks a variable the create-server form should fill with
	// a random value up front instead of leaving blank for the user to
	// invent one, e.g. RELAY_SECRET.
	AutoGenerate bool `json:"auto_generate"`
}

type eggSummary struct {
	ID             int    `json:"id"`
	Category       string `json:"category"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	DockerImage    string `json:"docker_image"`
	StartupCommand string `json:"startup_command"`
	// Enabled eggs are the default catalog available to everyone. A
	// disabled egg still exists (and its variables/servers keep working)
	// but is only surfaced through the Plugins page, which can flip this
	// flag back on -- see 0022_egg_enabled_flag.sql.
	Enabled   bool                 `json:"enabled"`
	Variables []eggVariableSummary `json:"variables"`
}

func (h *EggHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT id, category, name, description, docker_image, startup_command, enabled
		FROM eggs ORDER BY category, name`)
	if err != nil {
		http.Error(w, "failed to list eggs", http.StatusInternalServerError)
		return
	}

	eggs := make([]eggSummary, 0)
	for rows.Next() {
		var e eggSummary
		if err := rows.Scan(&e.ID, &e.Category, &e.Name, &e.Description, &e.DockerImage, &e.StartupCommand, &e.Enabled); err != nil {
			http.Error(w, "failed to read eggs", http.StatusInternalServerError)
			return
		}
		e.Variables = []eggVariableSummary{}
		eggs = append(eggs, e)
	}
	rows.Close()

	for i := range eggs {
		varRows, err := h.DB.Query(r.Context(),
			`SELECT name, env_variable, default_value, is_editable, rules, is_port, auto_generate
			 FROM egg_variables WHERE egg_id = $1 ORDER BY id`, eggs[i].ID)
		if err != nil {
			continue
		}
		for varRows.Next() {
			var v eggVariableSummary
			if err := varRows.Scan(&v.Name, &v.EnvVariable, &v.DefaultValue, &v.IsEditable, &v.Rules, &v.IsPort, &v.AutoGenerate); err == nil {
				eggs[i].Variables = append(eggs[i].Variables, v)
			}
		}
		varRows.Close()
	}

	writeJSON(w, http.StatusOK, eggs)
}

type setEggEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

// SetEnabled is the "install"/"uninstall" action for a feature-toggle
// plugin (e.g. streaming) -- admin-only, see router.go.
func (h *EggHandler) SetEnabled(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid egg id", http.StatusBadRequest)
		return
	}
	var req setEggEnabledRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	tag, err := h.DB.Exec(r.Context(), `UPDATE eggs SET enabled = $1 WHERE id = $2`, req.Enabled, id)
	if err != nil {
		http.Error(w, "failed to update egg", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "egg not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
