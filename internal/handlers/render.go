package handlers

import (
	"net/http"
)

type Renderer struct{}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) Render(w http.ResponseWriter, pagePath string, data PageData) {
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
