package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"copa-2026/internal/models"
	"copa-2026/internal/services"
)

type ProfileHandler struct {
	betSvc     *services.BetService
	rankingSvc *services.RankingService
	db         *sql.DB
	renderer   *Renderer
}

func NewProfileHandler(betSvc *services.BetService, rankingSvc *services.RankingService, db *sql.DB, renderer *Renderer) *ProfileHandler {
	return &ProfileHandler{betSvc: betSvc, rankingSvc: rankingSvc, db: db, renderer: renderer}
}

func (h *ProfileHandler) UserBets(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)

	targetUserIDStr := chi.URLParam(r, "id")
	targetUserID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Usuário inválido", http.StatusBadRequest)
		return
	}

	var userName string
	var isAdmin bool
	err = h.db.QueryRow("SELECT name, is_admin FROM users WHERE id = $1", targetUserID).Scan(&userName, &isAdmin)
	if err != nil {
		http.Error(w, "Usuário não encontrado", http.StatusNotFound)
		return
	}

	bets, err := h.betSvc.GetUserBets(targetUserID)
	if err != nil {
		http.Error(w, "Erro ao carregar palpites", http.StatusInternalServerError)
		return
	}

	specialBets, err := h.getUserSpecialBets(targetUserID)
	if err != nil {
		http.Error(w, "Erro ao carregar palpites especiais", http.StatusInternalServerError)
		return
	}

	rankPos, _ := h.rankingSvc.GetUserPosition(targetUserID)

	type UserProfileData struct {
		UserName    string
		Rank        *models.UserRanking
		MatchBets   []models.Bet
		SpecialBets []models.SpecialBet
	}

	profileData := UserProfileData{
		UserName:    userName,
		Rank:        rankPos,
		MatchBets:   bets,
		SpecialBets: specialBets,
	}

	data := PageData{
		Title: "Palpites de " + userName,
		User:  user,
		Data:  profileData,
	}

	h.renderer.Render(w, "cmd/web/templates/pages/user_profile.html", data)
}

func (h *ProfileHandler) UserBetsPartial(w http.ResponseWriter, r *http.Request) {
	targetUserIDStr := chi.URLParam(r, "id")
	targetUserID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Usuário inválido", http.StatusBadRequest)
		return
	}

	bets, err := h.betSvc.GetUserBets(targetUserID)
	if err != nil {
		http.Error(w, "Erro ao carregar palpites", http.StatusInternalServerError)
		return
	}

	specialBets, err := h.getUserSpecialBets(targetUserID)
	if err != nil {
		http.Error(w, "Erro ao carregar palpites especiais", http.StatusInternalServerError)
		return
	}

	type UserBetsPartialData struct {
		UserName    string
		MatchBets   []models.Bet
		SpecialBets []models.SpecialBet
	}

	var userName string
	h.db.QueryRow("SELECT name FROM users WHERE id = $1", targetUserID).Scan(&userName)

	partialData := UserBetsPartialData{
		UserName:    userName,
		MatchBets:   bets,
		SpecialBets: specialBets,
	}

	user := GetUserFromSession(r)
	data := PageData{
		Title: "Palpites de " + userName,
		User:  user,
		Data:  partialData,
	}

	tmpl, err := LoadPageTemplate("cmd/web/templates/partials/user_bets_partial.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.ExecuteTemplate(w, "user_bets_partial", data)
}

func (h *ProfileHandler) getUserSpecialBets(userID int64) ([]models.SpecialBet, error) {
	rows, err := h.db.Query(`
		SELECT id, user_id, bet_type, value, points, created_at
		FROM special_bets WHERE user_id = $1
		ORDER BY bet_type
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bets []models.SpecialBet
	for rows.Next() {
		var b models.SpecialBet
		if err := rows.Scan(&b.ID, &b.UserID, &b.BetType, &b.Value, &b.Points, &b.CreatedAt); err != nil {
			return nil, err
		}
		bets = append(bets, b)
	}
	return bets, rows.Err()
}
