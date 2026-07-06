package handlers

import (
	"net/http"

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
}

type eggSummary struct {
	ID             int                  `json:"id"`
	Category       string               `json:"category"`
	Name           string               `json:"name"`
	Description    string               `json:"description"`
	DockerImage    string               `json:"docker_image"`
	StartupCommand string               `json:"startup_command"`
	Variables      []eggVariableSummary `json:"variables"`
}

func (h *EggHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT id, category, name, description, docker_image, startup_command
		FROM eggs ORDER BY category, name`)
	if err != nil {
		http.Error(w, "failed to list eggs", http.StatusInternalServerError)
		return
	}

	eggs := make([]eggSummary, 0)
	for rows.Next() {
		var e eggSummary
		if err := rows.Scan(&e.ID, &e.Category, &e.Name, &e.Description, &e.DockerImage, &e.StartupCommand); err != nil {
			http.Error(w, "failed to read eggs", http.StatusInternalServerError)
			return
		}
		e.Variables = []eggVariableSummary{}
		eggs = append(eggs, e)
	}
	rows.Close()

	for i := range eggs {
		varRows, err := h.DB.Query(r.Context(),
			`SELECT name, env_variable, default_value, is_editable, rules
			 FROM egg_variables WHERE egg_id = $1 ORDER BY id`, eggs[i].ID)
		if err != nil {
			continue
		}
		for varRows.Next() {
			var v eggVariableSummary
			if err := varRows.Scan(&v.Name, &v.EnvVariable, &v.DefaultValue, &v.IsEditable, &v.Rules); err == nil {
				eggs[i].Variables = append(eggs[i].Variables, v)
			}
		}
		varRows.Close()
	}

	writeJSON(w, http.StatusOK, eggs)
}
