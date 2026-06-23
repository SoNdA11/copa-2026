package handlers

import (
	"database/sql"
	"net/http"
)

type Renderer struct {
	db *sql.DB
}

func NewRenderer(db *sql.DB) *Renderer {
	return &Renderer{db: db}
}

func (r *Renderer) Render(w http.ResponseWriter, pagePath string, data PageData) {
	if data.User != nil && data.User.ID > 0 {
		var avatarURL string
		r.db.QueryRow("SELECT COALESCE(avatar_url, '') FROM users WHERE id = $1", data.User.ID).Scan(&avatarURL)
		data.User.AvatarURL = avatarURL
	}

	allFiles := []string{
		"cmd/web/templates/layout.html",
		"cmd/web/templates/partials/nav.html",
		"cmd/web/templates/partials/match_row.html",
		"cmd/web/templates/partials/bet_form.html",
		"cmd/web/templates/partials/ranking_table.html",
		"cmd/web/templates/partials/group_standings.html",
		pagePath,
	}

	tmpl, err := LoadPageTemplate(allFiles...)
	if err != nil {
		http.Error(w, "Erro ao carregar página: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.ExecuteTemplate(w, "layout", data)
	if err != nil {
		http.Error(w, "Erro ao renderizar página: "+err.Error(), http.StatusInternalServerError)
	}
}
