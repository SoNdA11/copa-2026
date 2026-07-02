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
	db         *sql.DB
	betSvc     *services.BetService
	syncSvc    *services.SyncService
	bracketSvc *services.BracketService
	renderer   *Renderer
}

func NewMatchHandler(db *sql.DB, betSvc *services.BetService, syncSvc *services.SyncService, bracketSvc *services.BracketService, renderer *Renderer) *MatchHandler {
	return &MatchHandler{db: db, betSvc: betSvc, syncSvc: syncSvc, bracketSvc: bracketSvc, renderer: renderer}
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
				match.BetAdvancingTeamID = bet.AdvancingTeamID
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
		match.BetAdvancingTeamID = bet.AdvancingTeamID
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
		MatchID          int64
		HomeScore        int
		AwayScore        int
		AdvancingTeamID  int64
	}

	userBets := make(map[int64]userBet)
	if user != nil {
		rows, err := h.db.Query("SELECT match_id, home_score, away_score, COALESCE(advancing_team_id, 0) FROM bets WHERE user_id = $1", user.ID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var ub userBet
				if rows.Scan(&ub.MatchID, &ub.HomeScore, &ub.AwayScore, &ub.AdvancingTeamID) == nil {
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
					m.BetAdvancingTeamID = ub.AdvancingTeamID
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
			m.BetAdvancingTeamID = ub.AdvancingTeamID
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
	UserName         string
	UserID           int64
	HomeScore        int
	AwayScore        int
	AdvancingTeamID  int64
	AdvancingTeamName string
	TotalPoints      int
	AvatarURL        string
}

type GroupedBetUser struct {
	UserID           int64
	UserName         string
	AvatarURL        string
	TotalPoints      int
	IsCurrent        bool
	AdvancingTeamName string
}

type GroupedBet struct {
	HomeScore        int
	AwayScore        int
	AdvancingTeamName string
	Users            []GroupedBetUser
	Count            int
	IsUserBet        bool
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

	// Load match to get team names for advancing team display
	match, err := h.getMatchDetail(id)
	if err != nil {
		http.Error(w, "Jogo não encontrado", http.StatusNotFound)
		return
	}

	rows, err := h.db.Query(`
		SELECT u.name, u.id, b.home_score, b.away_score, COALESCE(b.advancing_team_id, 0),
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
		if err := rows.Scan(&b.UserName, &b.UserID, &b.HomeScore, &b.AwayScore, &b.AdvancingTeamID, &b.TotalPoints, &b.AvatarURL); err != nil {
			continue
		}
		// Compute advancing team name
		if b.AdvancingTeamID > 0 && match.HomeTeam != nil && match.AwayTeam != nil {
			if b.AdvancingTeamID == match.HomeTeam.ID {
				b.AdvancingTeamName = match.HomeTeam.Name
			} else if b.AdvancingTeamID == match.AwayTeam.ID {
				b.AdvancingTeamName = match.AwayTeam.Name
			}
		}
		bets = append(bets, b)
	}

	groupMap := make(map[string]*GroupedBet)
	for _, b := range bets {
		isKnockout := isKnockoutStage(match.Stage)
		// Include advancing team in group key for knockout matches
		key := fmt.Sprintf("%d-%d", b.HomeScore, b.AwayScore)
		if isKnockout && b.AdvancingTeamID > 0 {
			key += fmt.Sprintf("-%d", b.AdvancingTeamID)
		}
		gb, ok := groupMap[key]
		if !ok {
			gb = &GroupedBet{
				HomeScore: b.HomeScore,
				AwayScore: b.AwayScore,
			}
			if isKnockout {
				gb.AdvancingTeamName = b.AdvancingTeamName
			}
			groupMap[key] = gb
		}
		userBet := user != nil && b.UserID == user.ID
		gb.Users = append(gb.Users, GroupedBetUser{
			UserID:           b.UserID,
			UserName:         b.UserName,
			AvatarURL:        b.AvatarURL,
			TotalPoints:      b.TotalPoints,
			IsCurrent:        userBet,
			AdvancingTeamName: b.AdvancingTeamName,
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

type PosMatch struct {
	Match     models.Match
	X, Y      int
	HasUserBet bool
	BetHome   int
	BetAway   int
	Round     string
	Half      string
}

type LineSeg struct {
	X1, Y1, X2, Y2 int
	Round          string
}

type BracketLayout struct {
	Matches  []PosMatch
	Lines    []LineSeg
	Rounds   []RoundHeader
	Width    int
	Height   int
}

type RoundHeader struct {
	Name  string
	X     int
	Y     int
}

func (h *MatchHandler) Bracket(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)

	userBets := make(map[int64]struct{ home, away int; adv int64 })
	if user != nil {
		rows, err := h.db.Query("SELECT match_id, home_score, away_score, COALESCE(advancing_team_id, 0) FROM bets WHERE user_id = $1", user.ID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var mid int64
				var hs, as int
				var adv int64
				if rows.Scan(&mid, &hs, &as, &adv) == nil {
					userBets[mid] = struct{ home, away int; adv int64 }{hs, as, adv}
				}
			}
		}
	}


	loadAll := func(stage string) []models.Match {
		rows, err := h.db.Query(`
			SELECT m.id, COALESCE(m.home_team_id, 0), COALESCE(m.away_team_id, 0),
				m.home_score, m.away_score,
				m.match_date, m.match_time, m.stage, COALESCE(m.group_name, ''), COALESCE(m.stadium, ''), m.status,
				COALESCE(ht.id, 0), COALESCE(ht.name, 'TBD'), COALESCE(ht.fifa_code, ''), COALESCE(ht.group_name, ''), COALESCE(ht.flag_url, ''),
				COALESCE(at.id, 0), COALESCE(at.name, 'TBD'), COALESCE(at.fifa_code, ''), COALESCE(at.group_name, ''), COALESCE(at.flag_url, ''),
				COALESCE(m.home_team_label, ''), COALESCE(m.away_team_label, '')
			FROM matches m
			LEFT JOIN teams ht ON ht.id = m.home_team_id
			LEFT JOIN teams at ON at.id = m.away_team_id
			WHERE m.stage = $1
			ORDER BY m.id
		`, stage)
		if err != nil {
			return nil
		}
		defer rows.Close()
		var matches []models.Match
		for rows.Next() {
			var m models.Match
			m.HomeTeam = &models.Team{}
			m.AwayTeam = &models.Team{}
			var homeScore, awayScore sql.NullInt64
			err := rows.Scan(
				&m.ID, &m.HomeTeamID, &m.AwayTeamID, &homeScore, &awayScore,
				&m.MatchDate, &m.MatchTime, &m.Stage, &m.GroupName, &m.Stadium, &m.Status,
				&m.HomeTeam.ID, &m.HomeTeam.Name, &m.HomeTeam.FifaCode, &m.HomeTeam.GroupName, &m.HomeTeam.FlagURL,
				&m.AwayTeam.ID, &m.AwayTeam.Name, &m.AwayTeam.FifaCode, &m.AwayTeam.GroupName, &m.AwayTeam.FlagURL,
				&m.HomeTeamLabel, &m.AwayTeamLabel,
			)
			if err != nil {
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
			matches = append(matches, m)
		}
		return matches
	}

	leftR32Order := []int64{74, 77, 73, 75, 83, 84, 81, 82}
	rightR32Order := []int64{76, 78, 79, 80, 86, 88, 85, 87}

	leftR16Order := []int64{89, 90, 93, 94}
	rightR16Order := []int64{91, 92, 95, 96}

	leftQFOrder := []int64{97, 98}
	rightQFOrder := []int64{99, 100}

	leftSFOrder := []int64{101}
	rightSFOrder := []int64{102}

	roundData := map[string][]models.Match{
		"r32": loadAll("r32"),
		"r16": loadAll("r16"),
		"qf":  loadAll("qf"),
		"sf":  loadAll("sf"),
	}

	var layout BracketLayout

	findMatch := func(matches []models.Match, id int64) (models.Match, bool) {
		for _, m := range matches {
			if m.ID == id {
				return m, true
			}
		}
		return models.Match{}, false
	}

	createPosMatch := func(m models.Match, round string, side string, x, y int) PosMatch {
		pm := PosMatch{
			Match: m, Round: round, Half: side, X: x, Y: y,
		}
		if ub, ok := userBets[m.ID]; ok {
			pm.HasUserBet = true
			pm.BetHome, pm.BetAway = ub.home, ub.away
			pm.Match.BetAdvancingTeamID = ub.adv
		}
		return pm
	}

	mw := 240
	mh := 64

	// Coordinates configuration
	leftX := []int{20, 300, 580, 860}
	rightX := []int{2260, 1980, 1700, 1420} // corresponding to Col 9, 8, 7, 6

	yR32 := []int{20, 112, 204, 296, 450, 542, 634, 726}
	yR16 := []int{66, 250, 496, 680}
	yQF := []int{158, 588}
	ySF := []int{373}

	yCoords := map[string][]int{
		"r32": yR32,
		"r16": yR16,
		"qf":  yQF,
		"sf":  ySF,
	}

	// 1. Process Left Wing Matches
	for i, id := range leftR32Order {
		if m, ok := findMatch(roundData["r32"], id); ok {
			layout.Matches = append(layout.Matches, createPosMatch(m, "r32", "left", leftX[0], yR32[i]))
		}
	}
	for i, id := range leftR16Order {
		if m, ok := findMatch(roundData["r16"], id); ok {
			layout.Matches = append(layout.Matches, createPosMatch(m, "r16", "left", leftX[1], yR16[i]))
		}
	}
	for i, id := range leftQFOrder {
		if m, ok := findMatch(roundData["qf"], id); ok {
			layout.Matches = append(layout.Matches, createPosMatch(m, "qf", "left", leftX[2], yQF[i]))
		}
	}
	for i, id := range leftSFOrder {
		if m, ok := findMatch(roundData["sf"], id); ok {
			layout.Matches = append(layout.Matches, createPosMatch(m, "sf", "left", leftX[3], ySF[i]))
		}
	}

	// 2. Process Center Matches
	var finalMatch, thirdMatch *models.Match
	if f, err := h.getMatchDetail(104); err == nil {
		finalMatch = f
	}
	if t, err := h.getMatchDetail(103); err == nil {
		thirdMatch = t
	}

	if finalMatch != nil {
		layout.Matches = append(layout.Matches, createPosMatch(*finalMatch, "final", "center", 1140, 270))
	}
	if thirdMatch != nil {
		layout.Matches = append(layout.Matches, createPosMatch(*thirdMatch, "third", "center", 1140, 476))
	}

	// 3. Process Right Wing Matches
	for i, id := range rightSFOrder {
		if m, ok := findMatch(roundData["sf"], id); ok {
			layout.Matches = append(layout.Matches, createPosMatch(m, "sf", "right", rightX[3], ySF[i]))
		}
	}
	for i, id := range rightQFOrder {
		if m, ok := findMatch(roundData["qf"], id); ok {
			layout.Matches = append(layout.Matches, createPosMatch(m, "qf", "right", rightX[2], yQF[i]))
		}
	}
	for i, id := range rightR16Order {
		if m, ok := findMatch(roundData["r16"], id); ok {
			layout.Matches = append(layout.Matches, createPosMatch(m, "r16", "right", rightX[1], yR16[i]))
		}
	}
	for i, id := range rightR32Order {
		if m, ok := findMatch(roundData["r32"], id); ok {
			layout.Matches = append(layout.Matches, createPosMatch(m, "r32", "right", rightX[0], yR32[i]))
		}
	}

	// 4. Generate Line Segments
	stages := []string{"r32", "r16", "qf"}
	nextStages := []string{"r16", "qf", "sf"}

	// Left Wing Lines
	for si := 0; si < len(stages); si++ {
		st := stages[si]
		nst := nextStages[si]
		yVals := yCoords[st]
		nextYVals := yCoords[nst]
		xStart := leftX[si] + mw
		xEnd := leftX[si+1]
		xMid := (xStart + xEnd) / 2

		for p := 0; p < len(nextYVals); p++ {
			y1 := yVals[2*p] + mh/2
			y2 := yVals[2*p+1] + mh/2
			y3 := nextYVals[p] + mh/2

			layout.Lines = append(layout.Lines,
				LineSeg{X1: xStart, Y1: y1, X2: xMid, Y2: y1},
				LineSeg{X1: xStart, Y1: y2, X2: xMid, Y2: y2},
				LineSeg{X1: xMid, Y1: y1, X2: xMid, Y2: y2},
				LineSeg{X1: xMid, Y1: y3, X2: xEnd, Y2: y3},
			)
		}
	}

	// Right Wing Lines
	for si := 0; si < len(stages); si++ {
		st := stages[si]
		nst := nextStages[si]
		yVals := yCoords[st]
		nextYVals := yCoords[nst]
		xStart := rightX[si]
		xEnd := rightX[si+1] + mw
		xMid := (xStart + xEnd) / 2

		for p := 0; p < len(nextYVals); p++ {
			y1 := yVals[2*p] + mh/2
			y2 := yVals[2*p+1] + mh/2
			y3 := nextYVals[p] + mh/2

			layout.Lines = append(layout.Lines,
				LineSeg{X1: xStart, Y1: y1, X2: xMid, Y2: y1},
				LineSeg{X1: xStart, Y1: y2, X2: xMid, Y2: y2},
				LineSeg{X1: xMid, Y1: y1, X2: xMid, Y2: y2},
				LineSeg{X1: xMid, Y1: y3, X2: xEnd, Y2: y3},
			)
		}
	}

	// Center SF to Final & 3rd Place Lines
	sfLeftY := ySF[0] + mh/2   // 405
	sfRightY := ySF[0] + mh/2  // 405
	finalY := 270 + mh/2       // 302
	thirdY := 476 + mh/2       // 508

	// SF Left (Col 4 X=860) to Final (Col 5 X=1140) and 3rd Place (Col 5 X=1140)
	xStartL := leftX[3] + mw   // 860 + 240 = 1100
	xEndL := 1140
	xMidL := (xStartL + xEndL) / 2 // 1120

	layout.Lines = append(layout.Lines,
		// To Final
		LineSeg{X1: xStartL, Y1: sfLeftY, X2: xMidL, Y2: sfLeftY},
		LineSeg{X1: xMidL, Y1: finalY, X2: xMidL, Y2: sfLeftY},
		LineSeg{X1: xMidL, Y1: finalY, X2: xEndL, Y2: finalY},
		// To 3rd Place
		LineSeg{X1: xMidL, Y1: sfLeftY, X2: xMidL, Y2: thirdY},
		LineSeg{X1: xMidL, Y1: thirdY, X2: xEndL, Y2: thirdY},
	)

	// SF Right (Col 6 X=1420) to Final (Col 5 X=1140) and 3rd Place (Col 5 X=1140)
	xStartR := rightX[3]      // 1420
	xEndR := 1140 + mw         // 1140 + 240 = 1380
	xMidR := (xStartR + xEndR) / 2 // 1400

	layout.Lines = append(layout.Lines,
		// To Final
		LineSeg{X1: xStartR, Y1: sfRightY, X2: xMidR, Y2: sfRightY},
		LineSeg{X1: xMidR, Y1: sfRightY, X2: xMidR, Y2: finalY},
		LineSeg{X1: xMidR, Y1: finalY, X2: xEndR, Y2: finalY},
		// To 3rd Place
		LineSeg{X1: xMidR, Y1: sfRightY, X2: xMidR, Y2: thirdY},
		LineSeg{X1: xMidR, Y1: thirdY, X2: xEndR, Y2: thirdY},
	)

	layout.Width = 2520
	layout.Height = 820

	loc, _ := time.LoadLocation("America/Sao_Paulo")
	todayStr := time.Now().In(loc).Format("2006-01-02")
	for i := range layout.Matches {
		layout.Matches[i].Match.IsToday = layout.Matches[i].Match.MatchDate == todayStr
	}

	data := PageData{
		Title: "Mata-Mata",
		User:  user,
		Data:  layout,
	}
	h.renderer.Render(w, "cmd/web/templates/pages/bracket.html", data)
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

func isKnockoutStage(stage string) bool {
	switch stage {
	case "r32", "r16", "qf", "sf", "third", "final":
		return true
	}
	return false
}
