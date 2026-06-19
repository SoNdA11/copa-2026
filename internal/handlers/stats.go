package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"copa-2026/internal/services"
)

type StatsHandler struct {
	statsSvc *services.StatsService
	renderer *Renderer
}

func NewStatsHandler(statsSvc *services.StatsService, renderer *Renderer) *StatsHandler {
	return &StatsHandler{statsSvc: statsSvc, renderer: renderer}
}

func (h *StatsHandler) RankingEvolution(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	groupID := int64(1)
	if user != nil {
		groupID = user.GroupID
	}

	data := h.statsSvc.GetRankingEvolution(groupID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *StatsHandler) MatchDistribution(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	groupID := int64(1)
	if user != nil {
		groupID = user.GroupID
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}

	data := h.statsSvc.GetMatchBetDistribution(id, groupID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *StatsHandler) UserAccuracy(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Error(w, "Não autorizado", http.StatusUnauthorized)
		return
	}

	data := h.statsSvc.GetUserAccuracy(user.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *StatsHandler) UserPointsPerDay(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Error(w, "Não autorizado", http.StatusUnauthorized)
		return
	}

	data := h.statsSvc.GetUserPointsPerDay(user.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *StatsHandler) ScoreHeatmap(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	groupID := int64(1)
	if user != nil {
		groupID = user.GroupID
	}

	data := h.statsSvc.GetScoreHeatmap(groupID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *StatsHandler) GlobalInsights(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	groupID := int64(1)
	if user != nil {
		groupID = user.GroupID
	}

	data := h.statsSvc.GetGlobalInsights(groupID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *StatsHandler) InsightsPage(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	groupID := int64(1)
	if user != nil {
		groupID = user.GroupID
	}

	insights := h.statsSvc.GetGlobalInsights(groupID)

	data := PageData{
		Title: "Insights",
		User:  user,
		Data:  insights,
	}
	h.renderer.Render(w, "cmd/web/templates/pages/insights.html", data)
}

type DashboardData struct {
	Insights services.GlobalInsight `json:"-"`
}

func (h *StatsHandler) DashboardPage(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	title := "Início"
	if user != nil {
		title = "Bolão " + user.GroupName
	}

	groupID := int64(1)
	if user != nil {
		groupID = user.GroupID
	}

	insights := h.statsSvc.GetGlobalInsights(groupID)

	data := PageData{
		Title: title,
		User:  user,
		Data: DashboardData{
			Insights: insights,
		},
	}
	h.renderer.Render(w, "cmd/web/templates/pages/dashboard.html", data)
}

func (h *StatsHandler) BubbleData(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	groupID := int64(1)
	if user != nil {
		groupID = user.GroupID
	}

	data := h.statsSvc.GetBubbleData(groupID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *StatsHandler) RadarData(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	groupID := int64(1)
	if user != nil {
		groupID = user.GroupID
	}

	var userID int64
	if user != nil {
		userID = user.ID
	}

	data := h.statsSvc.GetRadarData(userID, groupID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
