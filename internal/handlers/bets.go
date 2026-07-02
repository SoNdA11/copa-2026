package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"copa-2026/internal/models"
	"copa-2026/internal/services"
)

type BetHandler struct {
	betSvc     *services.BetService
	bracketSvc *services.BracketService
	db         *sql.DB
	renderer   *Renderer
}

func NewBetHandler(betSvc *services.BetService, bracketSvc *services.BracketService, db *sql.DB, renderer *Renderer) *BetHandler {
	return &BetHandler{betSvc: betSvc, bracketSvc: bracketSvc, db: db, renderer: renderer}
}

func (h *BetHandler) Place(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Error(w, "Não autorizado", http.StatusUnauthorized)
		return
	}

	matchIDStr := chi.URLParam(r, "id")
	matchID, err := strconv.ParseInt(matchIDStr, 10, 64)
	if err != nil {
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Erro no formulário", http.StatusBadRequest)
		return
	}

	homeScoreStr := r.FormValue("home_score")
	awayScoreStr := r.FormValue("away_score")

	homeScore, err := strconv.Atoi(homeScoreStr)
	if err != nil || homeScore < 0 || homeScore > 20 {
		http.Error(w, "Placar inválido", http.StatusBadRequest)
		return
	}

	awayScore, err := strconv.Atoi(awayScoreStr)
	if err != nil || awayScore < 0 || awayScore > 20 {
		http.Error(w, "Placar inválido", http.StatusBadRequest)
		return
	}

	existing, _ := h.betSvc.GetUserBet(user.ID, matchID)

	var stage string
	err = h.db.QueryRow("SELECT stage FROM matches WHERE id = $1", matchID).Scan(&stage)
	if err != nil {
		http.Error(w, "Jogo não encontrado", http.StatusNotFound)
		return
	}

	if isKnockoutStage(stage) {
		advancingTeamID := int64(0)
		if val := r.FormValue("advancing_team_id"); val != "" {
			advancingTeamID, _ = strconv.ParseInt(val, 10, 64)
		}
		isFavorite := r.FormValue("is_favorite") == "1" || r.FormValue("is_favorite") == "true" || r.FormValue("is_favorite") == "on"

		if err := h.bracketSvc.PlaceKnockoutBet(user.ID, matchID, homeScore, awayScore, advancingTeamID, isFavorite); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
	} else {
		if err := h.betSvc.PlaceBet(user.ID, matchID, homeScore, awayScore); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
	}

	bet, _ := h.betSvc.GetUserBet(user.ID, matchID)
	match := h.getMatchWithTeams(matchID)
	if isKnockoutStage(match.Stage) {
		h.bracketSvc.ResolveSimulatedMatch(user.ID, match)
	}

	w.Header().Set("HX-Trigger", "bet-placed")

	inline := r.URL.Query().Get("inline") == "true"

	if inline {
		match.HasUserBet = true
		match.BetHomeScore = bet.HomeScore
		match.BetAwayScore = bet.AwayScore
		match.BetAdvancingTeamID = bet.AdvancingTeamID
		loc, _ := time.LoadLocation("America/Sao_Paulo")
		todayStr := time.Now().In(loc).Format("2006-01-02")
		match.IsToday = match.MatchDate == todayStr
		match.IsPast = match.MatchDate < todayStr

		tmpl, err := LoadPageTemplate("cmd/web/templates/partials/match_row.html")
		if err != nil {
			http.Error(w, "Erro ao renderizar", http.StatusInternalServerError)
			return
		}
		if err := tmpl.ExecuteTemplate(w, "match_row", match); err != nil {
			http.Error(w, "Erro ao renderizar", http.StatusInternalServerError)
		}
		return
	}

	flash := "Palpite salvo!"
	if existing != nil {
		flash = "Palpite atualizado!"
	}

	data := PageData{
		Data:  match,
		Bet:   bet,
		Flash: flash,
	}

	tmpl, err := LoadPageTemplate(
		"cmd/web/templates/partials/bet_form.html",
	)
	if err != nil {
		http.Error(w, "Erro ao renderizar", http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, "bet_form", data); err != nil {
		http.Error(w, "Erro ao renderizar", http.StatusInternalServerError)
	}
}

func (h *BetHandler) getMatchWithTeams(matchID int64) *models.Match {
	m := &models.Match{}
	m.HomeTeam = &models.Team{}
	m.AwayTeam = &models.Team{}

	var homeScore, awayScore sql.NullInt64
	var stadium, groupName sql.NullString

	err := h.db.QueryRow(`
		SELECT m.id, COALESCE(m.home_team_id, 0), COALESCE(m.away_team_id, 0),
			m.home_score, m.away_score,
			m.match_date, m.match_time, m.stage, m.group_name, m.stadium, m.status,
			COALESCE(ht.id, 0), COALESCE(ht.name, 'TBD'), COALESCE(ht.fifa_code, ''), COALESCE(ht.group_name, ''), COALESCE(ht.flag_url, ''),
			COALESCE(at.id, 0), COALESCE(at.name, 'TBD'), COALESCE(at.fifa_code, ''), COALESCE(at.group_name, ''), COALESCE(at.flag_url, '')
		FROM matches m
		LEFT JOIN teams ht ON ht.id = m.home_team_id
		LEFT JOIN teams at ON at.id = m.away_team_id
		WHERE m.id = $1
	`, matchID).Scan(
		&m.ID, &m.HomeTeamID, &m.AwayTeamID, &homeScore, &awayScore,
		&m.MatchDate, &m.MatchTime, &m.Stage, &groupName, &stadium, &m.Status,
		&m.HomeTeam.ID, &m.HomeTeam.Name, &m.HomeTeam.FifaCode, &m.HomeTeam.GroupName, &m.HomeTeam.FlagURL,
		&m.AwayTeam.ID, &m.AwayTeam.Name, &m.AwayTeam.FifaCode, &m.AwayTeam.GroupName, &m.AwayTeam.FlagURL,
	)
	if err != nil {
		return nil
	}

	if homeScore.Valid {
		hs := int(homeScore.Int64)
		m.HomeScore = &hs
	}
	if awayScore.Valid {
		as := int(awayScore.Int64)
		m.AwayScore = &as
	}
	if stadium.Valid {
		m.Stadium = stadium.String
	}
	if groupName.Valid {
		m.GroupName = groupName.String
	}

	return m
}

func (h *BetHandler) MyBets(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	bets, err := h.betSvc.GetUserBets(user.ID)
	if err != nil {
		http.Error(w, "Erro ao carregar palpites", http.StatusInternalServerError)
		return
	}

	specialBets, err := h.getUserSpecialBets(user.ID)
	if err != nil {
		http.Error(w, "Erro ao carregar palpites especiais", http.StatusInternalServerError)
		return
	}

	type MyBetsData struct {
		MatchBets   []models.Bet
		SpecialBets []models.SpecialBet
	}

	data := PageData{
		Title: "Meus Palpites",
		User:  user,
		Data: MyBetsData{
			MatchBets:   bets,
			SpecialBets: specialBets,
		},
	}

	h.renderer.Render(w, "cmd/web/templates/pages/my_bets.html", data)
}

func (h *BetHandler) getUserSpecialBets(userID int64) ([]models.SpecialBet, error) {
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
