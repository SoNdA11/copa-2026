package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"

	"copa-2026/internal/models"
	"copa-2026/internal/services"
)

type AdminHandler struct {
	db          *sql.DB
	betSvc      *services.BetService
	syncSvc     *services.SyncService
	knockoutSvc *services.KnockoutService
	authSvc     *services.AuthService
	renderer    *Renderer
}

func NewAdminHandler(db *sql.DB, betSvc *services.BetService, syncSvc *services.SyncService, knockoutSvc *services.KnockoutService, authSvc *services.AuthService, renderer *Renderer) *AdminHandler {
	return &AdminHandler{db: db, betSvc: betSvc, syncSvc: syncSvc, knockoutSvc: knockoutSvc, authSvc: authSvc, renderer: renderer}
}

type adminUserRow struct {
	ID               int64
	Name             string
	Points           int
	PointsAdjustment int
	Total            int
}

type adminMatchRow struct {
	ID         int64
	HomeTeam   string
	AwayTeam   string
	HomeScore  *int
	AwayScore  *int
	MatchDate  string
	MatchTime  string
	Stage      string
	GroupName  string
	Status     string
}

type adminMatchPageData struct {
	Matches []adminMatchRow
	Flash   string
}

func (h *AdminHandler) UsersPage(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	rows, err := h.db.Query(`
		SELECT u.id, u.name,
			COALESCE((SELECT SUM(points) FROM bets WHERE user_id = u.id), 0) +
			COALESCE((SELECT SUM(points) FROM special_bets WHERE user_id = u.id), 0) as points,
			COALESCE(u.points_adjustment, 0) as adjustment
		FROM users u
		WHERE u.is_admin = 0 AND u.group_id = $1
		ORDER BY u.name
	`, user.GroupID)
	if err != nil {
		http.Error(w, "Erro ao carregar usuários", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []adminUserRow
	for rows.Next() {
		var u adminUserRow
		if err := rows.Scan(&u.ID, &u.Name, &u.Points, &u.PointsAdjustment); err != nil {
			continue
		}
		u.Total = u.Points + u.PointsAdjustment
		users = append(users, u)
	}

	data := PageData{
		Title: "Administração",
		User:  user,
		Data:  users,
		Flash: r.URL.Query().Get("flash"),
	}
	h.renderer.Render(w, "cmd/web/templates/pages/admin.html", data)
}

func (h *AdminHandler) UpdatePoints(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Error(w, "Não autorizado", http.StatusUnauthorized)
		return
	}

	userIDStr := r.FormValue("user_id")
	if userIDStr == "" {
		http.Error(w, "ID do usuário não informado", http.StatusBadRequest)
		return
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}

	adjustmentStr := r.FormValue("adjustment")
	if adjustmentStr == "" {
		adjustmentStr = "0"
	}

	adjustment, err := strconv.Atoi(adjustmentStr)
	if err != nil {
		http.Error(w, "Ajuste inválido", http.StatusBadRequest)
		return
	}

	var targetGroupID int64
	err = h.db.QueryRow("SELECT group_id FROM users WHERE id = $1", userID).Scan(&targetGroupID)
	if err != nil || targetGroupID != user.GroupID {
		http.Error(w, "Usuário não encontrado", http.StatusNotFound)
		return
	}

	if _, err := h.db.Exec("UPDATE users SET points_adjustment = $1 WHERE id = $2 AND is_admin = 0", adjustment, userID); err != nil {
		http.Error(w, "Erro ao atualizar pontos", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin?flash=ajuste+salvo", http.StatusSeeOther)
}

func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Error(w, "Não autorizado", http.StatusUnauthorized)
		return
	}

	userIDStr := r.FormValue("user_id")
	if userIDStr == "" {
		http.Error(w, "ID do usuário não informado", http.StatusBadRequest)
		return
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}

	var targetGroupID int64
	err = h.db.QueryRow("SELECT group_id FROM users WHERE id = $1", userID).Scan(&targetGroupID)
	if err != nil || targetGroupID != user.GroupID {
		http.Error(w, "Usuário não encontrado", http.StatusNotFound)
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, "Erro ao iniciar transação", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM bets WHERE user_id = $1", userID); err != nil {
		http.Error(w, "Erro ao excluir palpites", http.StatusInternalServerError)
		return
	}

	if _, err := tx.Exec("DELETE FROM special_bets WHERE user_id = $1", userID); err != nil {
		http.Error(w, "Erro ao excluir palpites especiais", http.StatusInternalServerError)
		return
	}

	if _, err := tx.Exec("DELETE FROM users WHERE id = $1 AND is_admin = 0", userID); err != nil {
		http.Error(w, "Erro ao excluir usuário", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Erro ao finalizar transação", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin?flash=usuario+excluido", http.StatusSeeOther)
}

type adminMatchData struct {
	Matches []adminMatchRow
	Flash   string
}

func (h *AdminHandler) MatchesPage(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	stage := r.URL.Query().Get("stage")
	query := `
		SELECT m.id, COALESCE(ht.name, 'TBD'), COALESCE(at.name, 'TBD'),
			m.home_score, m.away_score, m.match_date, m.match_time, m.stage, m.group_name, m.status
		FROM matches m
		LEFT JOIN teams ht ON ht.id = m.home_team_id
		LEFT JOIN teams at ON at.id = m.away_team_id
	`
	if stage != "" && stage != "all" {
		query += " WHERE m.stage = $1"
		query += " ORDER BY m.match_date, m.match_time"
	} else {
		query += " ORDER BY m.match_date, m.match_time"
	}

	var rows *sql.Rows
	var err error
	if stage != "" && stage != "all" {
		rows, err = h.db.Query(query, stage)
	} else {
		rows, err = h.db.Query(query)
	}
	if err != nil {
		http.Error(w, "Erro ao carregar jogos", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var matches []adminMatchRow
	for rows.Next() {
		var m adminMatchRow
		var homeScore, awayScore sql.NullInt64
		var groupName sql.NullString
		if err := rows.Scan(&m.ID, &m.HomeTeam, &m.AwayTeam, &homeScore, &awayScore, &m.MatchDate, &m.MatchTime, &m.Stage, &groupName, &m.Status); err != nil {
			continue
		}
		if homeScore.Valid {
			v := int(homeScore.Int64)
			m.HomeScore = &v
		}
		if awayScore.Valid {
			v := int(awayScore.Int64)
			m.AwayScore = &v
		}
		if groupName.Valid {
			m.GroupName = groupName.String
		}
		matches = append(matches, m)
	}

	data := PageData{
		Title: "Admin - Jogos",
		User:  user,
		Data: adminMatchData{
			Matches: matches,
			Flash:   r.URL.Query().Get("flash"),
		},
	}
	h.renderer.Render(w, "cmd/web/templates/pages/admin_matches.html", data)
}

func (h *AdminHandler) UpdateMatch(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Error(w, "Não autorizado", http.StatusUnauthorized)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Erro no formulário", http.StatusBadRequest)
		return
	}

	matchIDStr := r.FormValue("match_id")
	matchID, err := strconv.ParseInt(matchIDStr, 10, 64)
	if err != nil {
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}

	status := r.FormValue("status")
	homeScoreStr := r.FormValue("home_score")
	awayScoreStr := r.FormValue("away_score")

	var homeScore, awayScore interface{}
	homeScore = nil
	awayScore = nil

	if status == "finished" {
		hs, err := strconv.Atoi(homeScoreStr)
		if err != nil {
			http.Error(w, "Placar inválido", http.StatusBadRequest)
			return
		}
		as, err := strconv.Atoi(awayScoreStr)
		if err != nil {
			http.Error(w, "Placar inválido", http.StatusBadRequest)
			return
		}
		homeScore = hs
		awayScore = as
	}

	_, err = h.db.Exec(`
		UPDATE matches SET status = $1, home_score = $2, away_score = $3 WHERE id = $4
	`, status, homeScore, awayScore, matchID)
	if err != nil {
		http.Error(w, "Erro ao atualizar jogo", http.StatusInternalServerError)
		return
	}

	if status == "finished" {
		if err := h.betSvc.RecalculateMatchBets(matchID); err != nil {
			log.Printf("Error recalculating bets for match %d: %v", matchID, err)
		}
	}

	http.Redirect(w, r, "/admin/matches?flash=jogo+atualizado", http.StatusSeeOther)
}

func (h *AdminHandler) ForceSync(w http.ResponseWriter, r *http.Request) {
	go func() {
		log.Println("[Admin] Forcing full sync...")
		if err := h.syncSvc.SyncAllData(); err != nil {
			log.Printf("[Admin] Sync error: %v", err)
		}
		h.betSvc.RecalculateAllFinishedMatches()
		h.knockoutSvc.RecalculateAll()
		log.Println("[Admin] Sync complete.")
	}()
	http.Redirect(w, r, "/admin/matches?flash=sync+iniciado", http.StatusSeeOther)
}

func (h *AdminHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	admin := GetUserFromSession(r)
	if admin == nil {
		http.Error(w, "Não autorizado", http.StatusUnauthorized)
		return
	}

	userID, _ := strconv.ParseInt(r.FormValue("user_id"), 10, 64)
	newPass := r.FormValue("new_password")
	if userID == 0 || len(newPass) < 4 {
		http.Redirect(w, r, "/admin?flash=senha+deve+ter+pelo+menos+4+caracteres", http.StatusSeeOther)
		return
	}

	var targetGroupID int64
	err := h.db.QueryRow("SELECT group_id FROM users WHERE id = $1", userID).Scan(&targetGroupID)
	if err != nil || targetGroupID != admin.GroupID {
		http.Error(w, "Usuário não encontrado", http.StatusNotFound)
		return
	}

	if err := h.authSvc.ResetPassword(userID, newPass); err != nil {
		http.Error(w, "Erro ao redefinir senha", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin?flash=senha+redefinida", http.StatusSeeOther)
}

func (h *AdminHandler) MatchBetsPage(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	matchIDStr := r.URL.Query().Get("match_id")
	matchID, err := strconv.ParseInt(matchIDStr, 10, 64)
	if err != nil {
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}

	match, err := h.betSvc.GetMatchByID(matchID)
	if err != nil {
		http.Error(w, "Jogo não encontrado", http.StatusNotFound)
		return
	}

	bets, err := h.betSvc.GetAllBetsForMatch(matchID)
	if err != nil {
		http.Error(w, "Erro ao carregar palpites", http.StatusInternalServerError)
		return
	}

	history, err := h.betSvc.GetBetHistory(matchID)
	if err != nil {
		log.Printf("Error loading bet history for match %d: %v", matchID, err)
	}

	type matchBetsPageData struct {
		Match   *models.Match
		Bets    []models.Bet
		History []services.BetHistoryEntry
	}

	data := PageData{
		Title: "Palpites - " + strconv.FormatInt(matchID, 10),
		User:  user,
		Data: matchBetsPageData{
			Match:   match,
			Bets:    bets,
			History: history,
		},
	}

	h.renderer.Render(w, "cmd/web/templates/pages/admin_match_bets.html", data)
}
