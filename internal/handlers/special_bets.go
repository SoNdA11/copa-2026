package handlers

import (
	"database/sql"
	"net/http"

	"copa-2026/internal/models"
	"copa-2026/internal/services"
)

type SpecialBetHandler struct {
	db       *sql.DB
	renderer *Renderer
}

func NewSpecialBetHandler(db *sql.DB, renderer *Renderer) *SpecialBetHandler {
	return &SpecialBetHandler{db: db, renderer: renderer}
}

func (h *SpecialBetHandler) List(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	bets, err := h.getUserSpecialBets(user.ID)
	if err != nil {
		http.Error(w, "Erro ao carregar palpites", http.StatusInternalServerError)
		return
	}

	teams, err := h.getTeams()
	if err != nil {
		http.Error(w, "Erro ao carregar times", http.StatusInternalServerError)
		return
	}

	type SpecialBetType struct {
		Key         string
		Label       string
		MaxPoints   int
		Description string
	}

	betTypes := []SpecialBetType{
		{Key: "champion", Label: "Campeão da Copa", MaxPoints: 10, Description: "Quem vai levantar a taça?"},
		{Key: "best_player", Label: "Melhor Jogador", MaxPoints: 5, Description: "Quem será eleito o melhor da Copa?"},
		{Key: "top_scorer", Label: "Artilheiro", MaxPoints: 5, Description: "Quem vai fazer mais gols?"},
		{Key: "best_goalkeeper", Label: "Melhor Goleiro", MaxPoints: 5, Description: "Quem será o melhor goleiro da Copa?"},
		{Key: "best_young_player", Label: "Melhor Jovem", MaxPoints: 5, Description: "Quem será o melhor jogador jovem da Copa?"},
	}

	specialData := struct {
		Bets  map[string]*models.SpecialBet
		Types []SpecialBetType
		Teams []models.Team
	}{
		Bets:  bets,
		Types: betTypes,
		Teams: teams,
	}

	data := PageData{
		Title: "Palpites Especiais",
		User:  user,
		Data:  specialData,
	}

	h.renderer.Render(w, "cmd/web/templates/pages/special_bets.html", data)
}

func (h *SpecialBetHandler) Place(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Error(w, "Não autorizado", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Erro no formulário", http.StatusBadRequest)
		return
	}

	if !services.IsSpecialBettingOpen() {
		http.Error(w, "Prazo de palpites especiais encerrado", http.StatusBadRequest)
		return
	}

	betType := r.FormValue("bet_type")
	value := r.FormValue("value")

	if betType == "" || value == "" {
		http.Error(w, "Preencha todos os campos", http.StatusBadRequest)
		return
	}

	_, err := h.db.Exec(`
		INSERT INTO special_bets (user_id, bet_type, value)
		VALUES ($1, $2, $3)
		ON CONFLICT(user_id, bet_type) DO UPDATE SET
			value = EXCLUDED.value,
			created_at = CURRENT_TIMESTAMP
	`, user.ID, betType, value)
	if err != nil {
		http.Error(w, "Erro ao salvar palpite", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/special", http.StatusSeeOther)
}

func (h *SpecialBetHandler) getUserSpecialBets(userID int64) (map[string]*models.SpecialBet, error) {
	rows, err := h.db.Query(`
		SELECT id, user_id, bet_type, value, points, created_at
		FROM special_bets WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bets := make(map[string]*models.SpecialBet)
	for rows.Next() {
		var b models.SpecialBet
		if err := rows.Scan(&b.ID, &b.UserID, &b.BetType, &b.Value, &b.Points, &b.CreatedAt); err != nil {
			return nil, err
		}
		bets[b.BetType] = &b
	}

	return bets, rows.Err()
}

func (h *SpecialBetHandler) AllBets(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)

	type UserSpecialRow struct {
		UserName       string
		Champion       string
		BestPlayer     string
		TopScorer      string
		BestGoalkeeper string
		BestYoung      string
		ChampionPts       int
		BestPlayerPts     int
		TopScorerPts      int
		BestGoalkeeperPts int
		BestYoungPts      int
	}

	rows, err := h.db.Query(`
		SELECT u.name,
			COALESCE(MAX(CASE WHEN s.bet_type = 'champion' THEN s.value END), '-') as champion,
			COALESCE(MAX(CASE WHEN s.bet_type = 'best_player' THEN s.value END), '-') as best_player,
			COALESCE(MAX(CASE WHEN s.bet_type = 'top_scorer' THEN s.value END), '-') as top_scorer,
			COALESCE(MAX(CASE WHEN s.bet_type = 'best_goalkeeper' THEN s.value END), '-') as best_goalkeeper,
			COALESCE(MAX(CASE WHEN s.bet_type = 'best_young_player' THEN s.value END), '-') as best_young_player,
			COALESCE(MAX(CASE WHEN s.bet_type = 'champion' THEN s.points END), 0) as champion_pts,
			COALESCE(MAX(CASE WHEN s.bet_type = 'best_player' THEN s.points END), 0) as best_player_pts,
			COALESCE(MAX(CASE WHEN s.bet_type = 'top_scorer' THEN s.points END), 0) as top_scorer_pts,
			COALESCE(MAX(CASE WHEN s.bet_type = 'best_goalkeeper' THEN s.points END), 0) as best_goalkeeper_pts,
			COALESCE(MAX(CASE WHEN s.bet_type = 'best_young_player' THEN s.points END), 0) as best_young_pts
		FROM users u
		LEFT JOIN special_bets s ON s.user_id = u.id
		WHERE u.is_admin = 0
		GROUP BY u.id, u.name
		ORDER BY u.name
	`)
	if err != nil {
		http.Error(w, "Erro ao carregar palpites", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var specialRows []UserSpecialRow
	for rows.Next() {
		var r UserSpecialRow
		if err := rows.Scan(&r.UserName, &r.Champion, &r.BestPlayer, &r.TopScorer, &r.BestGoalkeeper, &r.BestYoung,
			&r.ChampionPts, &r.BestPlayerPts, &r.TopScorerPts, &r.BestGoalkeeperPts, &r.BestYoungPts); err != nil {
			continue
		}
		specialRows = append(specialRows, r)
	}

	data := PageData{
		Title: "Palpites Especiais",
		User:  user,
		Data:  specialRows,
	}

	h.renderer.Render(w, "cmd/web/templates/pages/special_all.html", data)
}

func (h *SpecialBetHandler) getTeams() ([]models.Team, error) {
	rows, err := h.db.Query("SELECT id, name, fifa_code, group_name, flag_url FROM teams ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []models.Team
	for rows.Next() {
		var t models.Team
		var flagURL sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &t.FifaCode, &t.GroupName, &flagURL); err != nil {
			return nil, err
		}
		if flagURL.Valid {
			t.FlagURL = flagURL.String
		}
		teams = append(teams, t)
	}

	return teams, rows.Err()
}
