package handlers

import (
	"database/sql"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"

	"copa-2026/internal/models"
	"copa-2026/internal/services"
)

type MatchHandler struct {
	db       *sql.DB
	betSvc   *services.BetService
	renderer *Renderer
}

func NewMatchHandler(db *sql.DB, betSvc *services.BetService, renderer *Renderer) *MatchHandler {
	return &MatchHandler{db: db, betSvc: betSvc, renderer: renderer}
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

func (h *MatchHandler) List(w http.ResponseWriter, r *http.Request) {
	stage := r.URL.Query().Get("stage")

	user := GetUserFromSession(r)

	if stage == "group" {
		groups, err := h.getGroupStandings()
		if err != nil {
			http.Error(w, "Erro ao carregar grupos", http.StatusInternalServerError)
			return
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

	data := PageData{
		Title: "Jogos",
		User:  user,
		Data:  matches,
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
