package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"copa-2026/internal/models"
	"copa-2026/internal/services"
)

type MatchHandler struct {
	db       *sql.DB
	betSvc   *services.BetService
	syncSvc  *services.SyncService
	renderer *Renderer
}

func NewMatchHandler(db *sql.DB, betSvc *services.BetService, syncSvc *services.SyncService, renderer *Renderer) *MatchHandler {
	return &MatchHandler{db: db, betSvc: betSvc, syncSvc: syncSvc, renderer: renderer}
}

type GroupStanding struct {
	Position     int
	TeamName     string
	TeamFlag     string
	Played       int
	Wins         int
	Draws        int
	Losses       int
	GoalsFor     int
	GoalsAgainst int
	GoalDiff     int
	Points       int
}

type GroupInfo struct {
	Name      string
	Standings []GroupStanding
	Matches   []models.Match
}

type SectionedMatches struct {
	Today    []models.Match
	Upcoming []models.Match
	Past     []models.Match
}

func (h *MatchHandler) InlineBetForm(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Jogo inválido", http.StatusBadRequest)
		return
	}

	cancel := r.URL.Query().Get("cancel") == "true"

	match, err := h.getMatchDetail(id)
	if err != nil {
		http.Error(w, "Jogo não encontrado", http.StatusNotFound)
		return
	}

	user := GetUserFromSession(r)

	loc, _ := time.LoadLocation("America/Sao_Paulo")
	todayStr := time.Now().In(loc).Format("2006-01-02")
	match.IsToday = match.MatchDate == todayStr
	match.IsPast = match.MatchDate < todayStr

	if cancel || match.Status != "upcoming" || user == nil {
		if user != nil {
			bet, _ := h.betSvc.GetUserBet(user.ID, id)
			if bet != nil {
				match.HasUserBet = true
				match.BetHomeScore = bet.HomeScore
				match.BetAwayScore = bet.AwayScore
			}
		}
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

	bet, _ := h.betSvc.GetUserBet(user.ID, id)
	if bet != nil {
		match.HasUserBet = true
		match.BetHomeScore = bet.HomeScore
		match.BetAwayScore = bet.AwayScore
	}

	data := PageData{
		Data: match,
		Bet:  bet,
		User: user,
	}

	tmpl, err := LoadPageTemplate(
		"cmd/web/templates/partials/inline_bet.html",
		"cmd/web/templates/partials/match_row.html",
	)
	if err != nil {
		http.Error(w, "Erro ao renderizar", http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "inline_bet", data); err != nil {
		http.Error(w, "Erro ao renderizar", http.StatusInternalServerError)
	}
}

func (h *MatchHandler) List(w http.ResponseWriter, r *http.Request) {
	stage := r.URL.Query().Get("stage")

	user := GetUserFromSession(r)

	type userBet struct {
		MatchID      int64
		HomeScore    int
		AwayScore    int
	}

	userBets := make(map[int64]userBet)
	if user != nil {
		rows, err := h.db.Query("SELECT match_id, home_score, away_score FROM bets WHERE user_id = $1", user.ID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var ub userBet
				if rows.Scan(&ub.MatchID, &ub.HomeScore, &ub.AwayScore) == nil {
					userBets[ub.MatchID] = ub
				}
			}
		}
	}

	if stage == "group" {
		groups, err := h.getGroupStandings()
		if err != nil {
			http.Error(w, "Erro ao carregar grupos", http.StatusInternalServerError)
			return
		}
		loc, _ := time.LoadLocation("America/Sao_Paulo")
		todayStr := time.Now().In(loc).Format("2006-01-02")
		for gi := range groups {
			for mi := range groups[gi].Matches {
				m := &groups[gi].Matches[mi]
				if ub, ok := userBets[m.ID]; ok {
					m.HasUserBet = true
					m.BetHomeScore = ub.HomeScore
					m.BetAwayScore = ub.AwayScore
				}
				m.IsToday = m.MatchDate == todayStr
				m.IsPast = m.MatchDate < todayStr
			}
		}
		data := PageData{
			Title: "Jogos",
			User:  user,
			Data:  groups,
			Stage: stage,
		}
		h.renderer.Render(w, "cmd/web/templates/pages/matches.html", data)
		return
	}

	matches, err := h.getMatches(stage)
	if err != nil {
		http.Error(w, "Erro ao carregar jogos", http.StatusInternalServerError)
		return
	}

	loc, _ := time.LoadLocation("America/Sao_Paulo")
	todayStr := time.Now().In(loc).Format("2006-01-02")
	for i := range matches {
		m := &matches[i]
		if ub, ok := userBets[m.ID]; ok {
			m.HasUserBet = true
			m.BetHomeScore = ub.HomeScore
			m.BetAwayScore = ub.AwayScore
		}
		m.IsToday = m.MatchDate == todayStr
		m.IsPast = m.MatchDate < todayStr
	}

	var sectioned SectionedMatches
	for _, m := range matches {
		switch {
		case m.IsToday:
			sectioned.Today = append(sectioned.Today, m)
		case m.IsPast:
			sectioned.Past = append(sectioned.Past, m)
		default:
			sectioned.Upcoming = append(sectioned.Upcoming, m)
		}
	}

	data := PageData{
		Title: "Jogos",
		User:  user,
		Data:  sectioned,
		Stage: stage,
	}

	h.renderer.Render(w, "cmd/web/templates/pages/matches.html", data)
}

func (h *MatchHandler) getGroupStandings() ([]GroupInfo, error) {
	rows, err := h.db.Query(`
		SELECT id, name, fifa_code, group_name, flag_url FROM teams ORDER BY group_name, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type teamInfo struct {
		ID       int64
		Name     string
		FlagURL  string
		Group    string
	}

	var allTeams []teamInfo
	for rows.Next() {
		var t teamInfo
		var flagURL sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, new(string), &t.Group, &flagURL); err != nil {
			return nil, err
		}
		if flagURL.Valid {
			t.FlagURL = flagURL.String
		}
		allTeams = append(allTeams, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	teamsByGroup := make(map[string][]teamInfo)
	for _, t := range allTeams {
		teamsByGroup[t.Group] = append(teamsByGroup[t.Group], t)
	}

	matches, err := h.getMatches("group")
	if err != nil {
		return nil, err
	}

	type teamStats struct {
		Played, Wins, Draws, Losses, GF, GA int
		FlagURL                              string
	}

	var groups []GroupInfo

	groupKeys := make([]string, 0, len(teamsByGroup))
	for k := range teamsByGroup {
		groupKeys = append(groupKeys, k)
	}
	sort.Strings(groupKeys)

	for _, groupName := range groupKeys {
		groupTeams := teamsByGroup[groupName]

		stats := make(map[string]*teamStats)
		for _, t := range groupTeams {
			stats[t.Name] = &teamStats{FlagURL: t.FlagURL}
		}

		var groupMatches []models.Match
		for _, m := range matches {
			if m.GroupName == groupName {
				groupMatches = append(groupMatches, m)
				if m.HasResult() {
					hs := stats[m.HomeTeam.Name]
					as := stats[m.AwayTeam.Name]
					if hs != nil && as != nil {
						hs.Played++
						as.Played++
						hs.GF += *m.HomeScore
						hs.GA += *m.AwayScore
						as.GF += *m.AwayScore
						as.GA += *m.HomeScore
						if *m.HomeScore > *m.AwayScore {
							hs.Wins++
							as.Losses++
						} else if *m.HomeScore < *m.AwayScore {
							as.Wins++
							hs.Losses++
						} else {
							hs.Draws++
							as.Draws++
						}
					}
				}
			}
		}

		standings := make([]GroupStanding, 0, len(groupTeams))
		for _, t := range groupTeams {
			s := stats[t.Name]
			if s == nil {
				continue
			}
			standings = append(standings, GroupStanding{
				TeamName:     t.Name,
				TeamFlag:     s.FlagURL,
				Played:       s.Played,
				Wins:         s.Wins,
				Draws:        s.Draws,
				Losses:       s.Losses,
				GoalsFor:     s.GF,
				GoalsAgainst: s.GA,
				GoalDiff:     s.GF - s.GA,
				Points:       s.Wins*3 + s.Draws,
			})
		}

		sort.Slice(standings, func(i, j int) bool {
			if standings[i].Points != standings[j].Points {
				return standings[i].Points > standings[j].Points
			}
			if standings[i].GoalDiff != standings[j].GoalDiff {
				return standings[i].GoalDiff > standings[j].GoalDiff
			}
			return standings[i].GoalsFor > standings[j].GoalsFor
		})
		for i := range standings {
			standings[i].Position = i + 1
		}

		groups = append(groups, GroupInfo{
			Name:      groupName,
			Standings: standings,
			Matches:   groupMatches,
		})
	}

	return groups, nil
}

func (h *MatchHandler) Detail(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Jogo inválido", http.StatusBadRequest)
		return
	}

	go func() {
		if err := h.syncSvc.SyncMatch(id); err != nil {
			log.Printf("On-demand sync error for match %d: %v", id, err)
		}
	}()

	match, err := h.getMatchDetail(id)
	if err != nil {
		http.Error(w, "Jogo não encontrado", http.StatusNotFound)
		return
	}

	user := GetUserFromSession(r)
	pagePath := "cmd/web/templates/pages/match_detail.html"

	data := PageData{
		Title: match.HomeTeam.Name + " vs " + match.AwayTeam.Name,
		User:  user,
		Data:  match,
	}

	if user != nil {
		bet, _ := h.betSvc.GetUserBet(user.ID, id)
		data.Bet = bet
	}

	h.renderer.Render(w, pagePath, data)
}

func (h *MatchHandler) getMatches(stage string) ([]models.Match, error) {
	var rows *sql.Rows
	var err error

	query := `
		SELECT m.id, COALESCE(m.home_team_id, 0), COALESCE(m.away_team_id, 0),
			m.home_score, m.away_score,
			m.match_date, m.match_time, m.stage, m.group_name, m.stadium, m.status,
			COALESCE(ht.id, 0), COALESCE(ht.name, 'TBD'), COALESCE(ht.fifa_code, ''), COALESCE(ht.group_name, ''), COALESCE(ht.flag_url, ''),
			COALESCE(at.id, 0), COALESCE(at.name, 'TBD'), COALESCE(at.fifa_code, ''), COALESCE(at.group_name, ''), COALESCE(at.flag_url, '')
		FROM matches m
		LEFT JOIN teams ht ON ht.id = m.home_team_id
		LEFT JOIN teams at ON at.id = m.away_team_id
	`

	if stage != "" && stage != "all" {
		query += " WHERE m.stage = $1"
		query += " ORDER BY m.match_date, m.match_time"
		rows, err = h.db.Query(query, stage)
	} else {
		query += " ORDER BY m.match_date, m.match_time"
		rows, err = h.db.Query(query)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMatches(rows)
}

func (h *MatchHandler) getMatchDetail(id int64) (*models.Match, error) {
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
	`, id).Scan(
		&m.ID, &m.HomeTeamID, &m.AwayTeamID, &homeScore, &awayScore,
		&m.MatchDate, &m.MatchTime, &m.Stage, &groupName, &stadium, &m.Status,
		&m.HomeTeam.ID, &m.HomeTeam.Name, &m.HomeTeam.FifaCode, &m.HomeTeam.GroupName, &m.HomeTeam.FlagURL,
		&m.AwayTeam.ID, &m.AwayTeam.Name, &m.AwayTeam.FifaCode, &m.AwayTeam.GroupName, &m.AwayTeam.FlagURL,
	)
	if err != nil {
		return nil, err
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

	return m, nil
}

type MatchGroupBet struct {
	UserName    string
	UserID      int64
	HomeScore   int
	AwayScore   int
	TotalPoints int
	AvatarURL   string
}

type GroupedBetUser struct {
	UserID      int64
	UserName    string
	AvatarURL   string
	TotalPoints int
	IsCurrent   bool
}

type GroupedBet struct {
	HomeScore int
	AwayScore int
	Users     []GroupedBetUser
	Count     int
	IsUserBet bool
}

func (h *MatchHandler) GroupBets(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Jogo inválido", http.StatusBadRequest)
		return
	}

	user := GetUserFromSession(r)
	groupID := int64(1)
	if user != nil {
		groupID = user.GroupID
	}

	rows, err := h.db.Query(`
		SELECT u.name, u.id, b.home_score, b.away_score,
			COALESCE((SELECT SUM(points) FROM bets WHERE user_id = u.id), 0) +
			COALESCE((SELECT SUM(points) FROM special_bets WHERE user_id = u.id), 0) +
			COALESCE(u.points_adjustment, 0) as total_points,
			COALESCE(u.avatar_url, '')
		FROM bets b
		JOIN users u ON u.id = b.user_id
		WHERE b.match_id = $1 AND COALESCE(u.is_admin, 0) = 0 AND u.group_id = $2
		ORDER BY u.name
	`, id, groupID)
	if err != nil {
		http.Error(w, "Erro ao carregar palpites", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var bets []MatchGroupBet
	for rows.Next() {
		var b MatchGroupBet
		if err := rows.Scan(&b.UserName, &b.UserID, &b.HomeScore, &b.AwayScore, &b.TotalPoints, &b.AvatarURL); err != nil {
			continue
		}
		bets = append(bets, b)
	}

	groupMap := make(map[string]*GroupedBet)
	for _, b := range bets {
		key := fmt.Sprintf("%d-%d", b.HomeScore, b.AwayScore)
		gb, ok := groupMap[key]
		if !ok {
			gb = &GroupedBet{
				HomeScore: b.HomeScore,
				AwayScore: b.AwayScore,
			}
			groupMap[key] = gb
		}
		userBet := user != nil && b.UserID == user.ID
		gb.Users = append(gb.Users, GroupedBetUser{
			UserID:      b.UserID,
			UserName:    b.UserName,
			AvatarURL:   b.AvatarURL,
			TotalPoints: b.TotalPoints,
			IsCurrent:   userBet,
		})
		if userBet {
			gb.IsUserBet = true
		}
		gb.Count++
	}

	grouped := make([]*GroupedBet, 0, len(groupMap))
	for _, gb := range groupMap {
		sort.Slice(gb.Users, func(i, j int) bool {
			return gb.Users[i].UserName < gb.Users[j].UserName
		})
		grouped = append(grouped, gb)
	}

	sort.Slice(grouped, func(i, j int) bool {
		if grouped[i].IsUserBet != grouped[j].IsUserBet {
			return grouped[i].IsUserBet
		}
		return grouped[i].Count > grouped[j].Count
	})

	data := PageData{
		Title: "Palpites do Grupo",
		User:  user,
		Data:  grouped,
	}

	tmpl, err := LoadPageTemplate("cmd/web/templates/partials/match_bets_group.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.ExecuteTemplate(w, "match_bets_group", data)
}

func scanMatches(rows *sql.Rows) ([]models.Match, error) {
	var matches []models.Match
	for rows.Next() {
		var m models.Match
		m.HomeTeam = &models.Team{}
		m.AwayTeam = &models.Team{}
		var homeScore, awayScore sql.NullInt64
		var stadium, groupName sql.NullString

		err := rows.Scan(
			&m.ID, &m.HomeTeamID, &m.AwayTeamID, &homeScore, &awayScore,
			&m.MatchDate, &m.MatchTime, &m.Stage, &groupName, &stadium, &m.Status,
			&m.HomeTeam.ID, &m.HomeTeam.Name, &m.HomeTeam.FifaCode, &m.HomeTeam.GroupName, &m.HomeTeam.FlagURL,
			&m.AwayTeam.ID, &m.AwayTeam.Name, &m.AwayTeam.FifaCode, &m.AwayTeam.GroupName, &m.AwayTeam.FlagURL,
		)
		if err != nil {
			return nil, err
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

		matches = append(matches, m)
	}

	return matches, rows.Err()
}
