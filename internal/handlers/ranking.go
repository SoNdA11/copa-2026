package handlers

import (
	"net/http"

	"copa-2026/internal/services"
)

type RankingHandler struct {
	rankingSvc *services.RankingService
	renderer   *Renderer
}

func NewRankingHandler(rankingSvc *services.RankingService, renderer *Renderer) *RankingHandler {
	return &RankingHandler{rankingSvc: rankingSvc, renderer: renderer}
}

func (h *RankingHandler) List(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	groupID := int64(1)
	if user != nil {
		groupID = user.GroupID
	}

	rankings, err := h.rankingSvc.GetRanking(groupID)
	if err != nil {
		http.Error(w, "Erro ao carregar ranking", http.StatusInternalServerError)
		return
	}

	data := PageData{
		Title: "Ranking",
		User:  user,
		Data:  rankings,
	}

	h.renderer.Render(w, "cmd/web/templates/pages/ranking.html", data)
}

func (h *RankingHandler) Partial(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	groupID := int64(1)
	if user != nil {
		groupID = user.GroupID
	}

	rankings, err := h.rankingSvc.GetRanking(groupID)
	if err != nil {
		http.Error(w, "Erro ao carregar ranking", http.StatusInternalServerError)
		return
	}

	data := PageData{
		Title: "Ranking",
		User:  user,
		Data:  rankings,
	}

	tmpl, err := LoadPageTemplate("cmd/web/templates/partials/ranking_table.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "ranking_table", data)
}
