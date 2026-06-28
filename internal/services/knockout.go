package services

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
)

type KnockoutService struct {
	db *sql.DB
}

type groupTeamStat struct {
	TeamID   int64
	Name     string
	FlagURL  string
	Group    string
	Played   int
	Wins     int
	Draws    int
	Losses   int
	GF       int
	GA       int
	GD       int
	Points   int
}

func NewKnockoutService(db *sql.DB) *KnockoutService {
	return &KnockoutService{db: db}
}

func (s *KnockoutService) ComputeAdvancement() error {
	groups, err := s.computeGroupStandings()
	if err != nil {
		return fmt.Errorf("compute standings: %w", err)
	}

	winners := make(map[string]*groupTeamStat)
	runnersUp := make(map[string]*groupTeamStat)
	var thirdPlaced []*groupTeamStat

	for g, standings := range groups {
		if len(standings) < 3 {
			continue
		}
		winners[g] = standings[0]
		runnersUp[g] = standings[1]
		thirdPlaced = append(thirdPlaced, standings[2:]...)
	}

	sort.Slice(thirdPlaced, func(i, j int) bool {
		if thirdPlaced[i].Points != thirdPlaced[j].Points {
			return thirdPlaced[i].Points > thirdPlaced[j].Points
		}
		if thirdPlaced[i].GD != thirdPlaced[j].GD {
			return thirdPlaced[i].GD > thirdPlaced[j].GD
		}
		return thirdPlaced[i].GF > thirdPlaced[j].GF
	})

	top8Third := thirdPlaced
	if len(top8Third) > 8 {
		top8Third = top8Third[:8]
	}

	thirdMap := make(map[string]*groupTeamStat)
	for _, t := range top8Third {
		thirdMap[t.Group] = t
	}

	labelToTeam := make(map[string]int64)
	for g, t := range winners {
		labelToTeam[fmt.Sprintf("Winner Group %s", g)] = t.TeamID
	}
	for g, t := range runnersUp {
		labelToTeam[fmt.Sprintf("Runner-up Group %s", g)] = t.TeamID
	}
	if len(groups) == 12 {
		for _, t := range top8Third {
			labelToTeam[fmt.Sprintf("3rd Group %s", t.Group)] = t.TeamID
		}
	}

	if err := s.updateR32Matches(labelToTeam, thirdMap); err != nil {
		return fmt.Errorf("update r32: %w", err)
	}

	if err := s.propagateWinners(); err != nil {
		return fmt.Errorf("propagate winners: %w", err)
	}

	return nil
}

func (s *KnockoutService) computeGroupStandings() (map[string][]*groupTeamStat, error) {
	rows, err := s.db.Query(`
		SELECT id, name, group_name, flag_url FROM teams WHERE id > 0 ORDER BY group_name, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type teamInfo struct {
		ID      int64
		Name    string
		Group   string
		FlagURL string
	}

	var allTeams []teamInfo
	for rows.Next() {
		var t teamInfo
		var flagURL sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &t.Group, &flagURL); err != nil {
			continue
		}
		if flagURL.Valid {
			t.FlagURL = flagURL.String
		}
		allTeams = append(allTeams, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	teamByID := make(map[int64]teamInfo)
	for _, t := range allTeams {
		teamByID[t.ID] = t
	}

	matchRows, err := s.db.Query(`
		SELECT m.home_team_id, m.away_team_id, m.home_score, m.away_score, m.group_name
		FROM matches m
		WHERE m.stage = 'group' AND m.home_score IS NOT NULL AND m.away_score IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer matchRows.Close()

	stats := make(map[string]map[int64]*groupTeamStat)
	for _, t := range allTeams {
		if stats[t.Group] == nil {
			stats[t.Group] = make(map[int64]*groupTeamStat)
		}
		stats[t.Group][t.ID] = &groupTeamStat{
			TeamID:  t.ID,
			Name:    t.Name,
			FlagURL: t.FlagURL,
			Group:   t.Group,
		}
	}

	for matchRows.Next() {
		var homeID, awayID int64
		var homeScore, awayScore int
		var groupName sql.NullString
		if err := matchRows.Scan(&homeID, &awayID, &homeScore, &awayScore, &groupName); err != nil {
			continue
		}
		gn := ""
		if groupName.Valid {
			gn = groupName.String
		}

		hs := stats[gn][homeID]
		as := stats[gn][awayID]
		if hs == nil || as == nil {
			continue
		}

		hs.Played++
		as.Played++
		hs.GF += homeScore
		hs.GA += awayScore
		as.GF += awayScore
		as.GA += homeScore

		if homeScore > awayScore {
			hs.Wins++
			hs.Points += 3
			as.Losses++
		} else if homeScore < awayScore {
			as.Wins++
			as.Points += 3
			hs.Losses++
		} else {
			hs.Draws++
			as.Draws++
			hs.Points++
			as.Points++
		}
	}
	if err := matchRows.Err(); err != nil {
		return nil, err
	}

	result := make(map[string][]*groupTeamStat)
	groupKeys := make([]string, 0, len(stats))
	for k := range stats {
		groupKeys = append(groupKeys, k)
	}
	sort.Strings(groupKeys)

	for _, g := range groupKeys {
		teamStats := stats[g]
		list := make([]*groupTeamStat, 0, len(teamStats))
		var incomplete bool
		for _, t := range teamStats {
			t.GD = t.GF - t.GA
			list = append(list, t)
			if t.Played < 3 {
				incomplete = true
			}
		}
		if incomplete {
			continue
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Points != list[j].Points {
				return list[i].Points > list[j].Points
			}
			if list[i].GD != list[j].GD {
				return list[i].GD > list[j].GD
			}
			if list[i].GF != list[j].GF {
				return list[i].GF > list[j].GF
			}
			return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
		})
		result[g] = list
	}

	return result, nil
}

func (s *KnockoutService) updateR32Matches(labelToTeam map[string]int64, thirdMap map[string]*groupTeamStat) error {
	rows, err := s.db.Query(`
		SELECT id, home_team_id, away_team_id, home_team_label, away_team_label FROM matches WHERE stage = 'r32'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var matchID, currentHomeID, currentAwayID int64
		var homeLabel, awayLabel string
		if err := rows.Scan(&matchID, &currentHomeID, &currentAwayID, &homeLabel, &awayLabel); err != nil {
			continue
		}

		if currentHomeID > 0 && currentAwayID > 0 {
			continue
		}

		homeID, homeOk := s.resolveTeamID(homeLabel, labelToTeam)
		awayID, awayOk := s.resolveTeamID(awayLabel, labelToTeam)

		if homeOk && awayOk {
			s.db.Exec(`UPDATE matches SET home_team_id = $1, away_team_id = $2 WHERE id = $3`, homeID, awayID, matchID)
		} else if homeOk && currentHomeID == 0 {
			s.db.Exec(`UPDATE matches SET home_team_id = $1 WHERE id = $2`, homeID, matchID)
		} else if awayOk && currentAwayID == 0 {
			s.db.Exec(`UPDATE matches SET away_team_id = $1 WHERE id = $2`, awayID, matchID)
		}
	}

	return rows.Err()
}

func (s *KnockoutService) resolveTeamID(label string, labelToTeam map[string]int64) (int64, bool) {
	if id, ok := labelToTeam[label]; ok {
		return id, true
	}

	if strings.HasPrefix(label, "3rd Group ") {
		groupsStr := label[9:]
		groups := strings.Split(groupsStr, "/")
		for _, g := range groups {
			thirdLabel := fmt.Sprintf("3rd Group %s", g)
			if id, ok := labelToTeam[thirdLabel]; ok {
				return id, true
			}
		}
	}

	return 0, false
}

func (s *KnockoutService) propagateWinners() error {
	stages := []string{"r32", "r16", "qf", "sf", "third"}

	for _, stage := range stages {
		rows, err := s.db.Query(`
			SELECT id, home_team_id, away_team_id, home_team_label, away_team_label FROM matches WHERE stage = $1
		`, stage)
		if err != nil {
			return err
		}

		var matchIDs, homeIDs, awayIDs []int64
		var homeLabels, awayLabels []string

		for rows.Next() {
			var matchID, homeID, awayID int64
			var homeLabel, awayLabel string
			if err := rows.Scan(&matchID, &homeID, &awayID, &homeLabel, &awayLabel); err != nil {
				continue
			}
			matchIDs = append(matchIDs, matchID)
			homeIDs = append(homeIDs, homeID)
			awayIDs = append(awayIDs, awayID)
			homeLabels = append(homeLabels, homeLabel)
			awayLabels = append(awayLabels, awayLabel)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		for i, matchID := range matchIDs {
			currentHomeID := homeIDs[i]
			currentAwayID := awayIDs[i]

			if currentHomeID > 0 && currentAwayID > 0 {
				continue
			}

			homeLabel := homeLabels[i]
			awayLabel := awayLabels[i]
			homeWinner := s.getWinnerFromLabel(homeLabel)
			if homeWinner == 0 {
				homeWinner = s.getLoserFromLabel(homeLabel)
			}
			awayWinner := s.getWinnerFromLabel(awayLabel)
			if awayWinner == 0 {
				awayWinner = s.getLoserFromLabel(awayLabel)
			}

			if homeWinner > 0 && currentHomeID == 0 {
				s.db.Exec(`UPDATE matches SET home_team_id = $1 WHERE id = $2`, homeWinner, matchID)
			}
			if awayWinner > 0 && currentAwayID == 0 {
				s.db.Exec(`UPDATE matches SET away_team_id = $1 WHERE id = $2`, awayWinner, matchID)
			}
		}
	}

	return nil
}

func (s *KnockoutService) getWinnerFromLabel(label string) int64 {
	if !strings.HasPrefix(label, "Winner Match ") {
		return 0
	}

	var matchID int64
	if _, err := fmt.Sscanf(label, "Winner Match %d", &matchID); err != nil {
		return 0
	}

	var homeScore, awayScore sql.NullInt64
	err := s.db.QueryRow(
		"SELECT home_score, away_score FROM matches WHERE id = $1", matchID,
	).Scan(&homeScore, &awayScore)
	if err != nil || !homeScore.Valid || !awayScore.Valid {
		return 0
	}

	if homeScore.Int64 > awayScore.Int64 {
		var homeID int64
		s.db.QueryRow("SELECT home_team_id FROM matches WHERE id = $1", matchID).Scan(&homeID)
		return homeID
	} else if awayScore.Int64 > homeScore.Int64 {
		var awayID int64
		s.db.QueryRow("SELECT away_team_id FROM matches WHERE id = $1", matchID).Scan(&awayID)
		return awayID
	}

	return 0
}

func (s *KnockoutService) getLoserFromLabel(label string) int64 {
	if !strings.HasPrefix(label, "Loser Match ") {
		return 0
	}

	var matchID int64
	if _, err := fmt.Sscanf(label, "Loser Match %d", &matchID); err != nil {
		return 0
	}

	var homeScore, awayScore sql.NullInt64
	err := s.db.QueryRow(
		"SELECT home_score, away_score FROM matches WHERE id = $1", matchID,
	).Scan(&homeScore, &awayScore)
	if err != nil || !homeScore.Valid || !awayScore.Valid {
		return 0
	}

	if homeScore.Int64 < awayScore.Int64 {
		var homeID int64
		s.db.QueryRow("SELECT home_team_id FROM matches WHERE id = $1", matchID).Scan(&homeID)
		return homeID
	} else if awayScore.Int64 < homeScore.Int64 {
		var awayID int64
		s.db.QueryRow("SELECT away_team_id FROM matches WHERE id = $1", matchID).Scan(&awayID)
		return awayID
	}

	return 0
}

func (s *KnockoutService) RecalculateAll() {
	log.Print("Recalculating knockout advancement...")
	if err := s.ComputeAdvancement(); err != nil {
		log.Printf("Knockout recalculation error: %v", err)
	}
}
