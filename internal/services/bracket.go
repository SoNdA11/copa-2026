package services

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"copa-2026/internal/models"
)

type BracketService struct {
	db          *sql.DB
	knockoutSvc *KnockoutService
}

func NewBracketService(db *sql.DB, knockoutSvc *KnockoutService) *BracketService {
	return &BracketService{
		db:          db,
		knockoutSvc: knockoutSvc,
	}
}

// ResolveUserTeam recursively resolves the team ID playing in a given label/match for a specific user
func (s *BracketService) ResolveUserTeam(userID int64, label string) (int64, error) {
	if label == "" {
		return 0, nil
	}

	// If it's a Winner Match X label, we recursively resolve
	if strings.HasPrefix(label, "Winner Match ") {
		var prevMatchID int64
		if _, err := fmt.Sscanf(label, "Winner Match %d", &prevMatchID); err != nil {
			return 0, nil
		}

		// Fetch the previous match definition
		var homeLabel, awayLabel string
		var dbHomeID, dbAwayID int64
		err := s.db.QueryRow(`
			SELECT home_team_id, away_team_id, COALESCE(home_team_label, ''), COALESCE(away_team_label, '')
			FROM matches WHERE id = $1
		`, prevMatchID).Scan(&dbHomeID, &dbAwayID, &homeLabel, &awayLabel)
		if err != nil {
			return 0, err
		}

		// Recursively resolve the home and away teams for the previous match
		homeID := dbHomeID
		if homeID == 0 && homeLabel != "" {
			homeID, _ = s.ResolveUserTeam(userID, homeLabel)
		}
		awayID := dbAwayID
		if awayID == 0 && awayLabel != "" {
			awayID, _ = s.ResolveUserTeam(userID, awayLabel)
		}

		// If we don't even know the teams playing in the previous match, we can't have a winner
		if homeID == 0 && awayID == 0 {
			return 0, nil
		}

		// Now query the user's prediction (bet) for that previous match
		var betHome, betAway int
		var advTeamID int64
		err = s.db.QueryRow(`
			SELECT home_score, away_score, COALESCE(advancing_team_id, 0)
			FROM bets WHERE user_id = $1 AND match_id = $2
		`, userID, prevMatchID).Scan(&betHome, &betAway, &advTeamID)
		if err == sql.ErrNoRows {
			return 0, nil // User has not predicted this match yet
		} else if err != nil {
			return 0, err
		}

		// If advancing_team_id is explicitly set, use it
		if advTeamID > 0 {
			return advTeamID, nil
		}

		// Otherwise, infer from predicted score
		if betHome > betAway {
			return homeID, nil
		} else if betAway > betHome {
			return awayID, nil
		}

		return 0, nil // Undecided draw
	}

	// If it's a group standings label, we try to resolve it from the group standings
	// Winners, runners-up, or 3rd place teams.
	// We check if the real matches table has already resolved it.
	var teamID int64
	err := s.db.QueryRow(`
		SELECT id FROM teams WHERE name = $1 OR fifa_code = $1
	`, label).Scan(&teamID)
	if err == nil && teamID > 0 {
		return teamID, nil
	}

	// Fallback to computing the group standings dynamically if the real matches table is not updated
	standings, err := s.knockoutSvc.computeGroupStandings()
	if err == nil {
		labelToTeam := make(map[string]int64)
		winners := make(map[string]*groupTeamStat)
		runnersUp := make(map[string]*groupTeamStat)
		var thirdPlaced []*groupTeamStat

		for g, stds := range standings {
			if len(stds) < 3 {
				continue
			}
			winners[g] = stds[0]
			runnersUp[g] = stds[1]
			thirdPlaced = append(thirdPlaced, stds[2:]...)
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

		for g, t := range winners {
			labelToTeam[fmt.Sprintf("Winner Group %s", g)] = t.TeamID
		}
		for g, t := range runnersUp {
			labelToTeam[fmt.Sprintf("Runner-up Group %s", g)] = t.TeamID
		}
		for _, t := range top8Third {
			labelToTeam[fmt.Sprintf("3rd Group %s", t.Group)] = t.TeamID
		}

		if id, ok := s.knockoutSvc.resolveTeamID(label, labelToTeam); ok {
			return id, nil
		}
	}

	return 0, nil
}

// GetUserSimulatedMatches returns matches for a given stage with home/away teams resolved based on the user's previous predictions
func (s *BracketService) GetUserSimulatedMatches(userID int64, stage string) ([]models.Match, error) {
	rows, err := s.db.Query(`
		SELECT id, home_team_id, away_team_id, home_score, away_score, match_date, match_time, stage, COALESCE(group_name, ''), stadium, status, COALESCE(home_team_label, ''), COALESCE(away_team_label, '')
		FROM matches
		WHERE stage = $1
		ORDER BY id
	`, stage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []models.Match
	for rows.Next() {
		var m models.Match
		var homeScore, awayScore sql.NullInt64
		err := rows.Scan(
			&m.ID, &m.HomeTeamID, &m.AwayTeamID, &homeScore, &awayScore,
			&m.MatchDate, &m.MatchTime, &m.Stage, &m.GroupName, &m.Stadium, &m.Status,
			&m.HomeTeamLabel, &m.AwayTeamLabel,
		)
		if err != nil {
			return nil, err
		}

		if homeScore.Valid {
			val := int(homeScore.Int64)
			m.HomeScore = &val
		}
		if awayScore.Valid {
			val := int(awayScore.Int64)
			m.AwayScore = &val
		}

		// Resolve simulated teams if they are set to 0 in the database
		homeID := m.HomeTeamID
		if homeID == 0 && m.HomeTeamLabel != "" {
			homeID, _ = s.ResolveUserTeam(userID, m.HomeTeamLabel)
		}
		awayID := m.AwayTeamID
		if awayID == 0 && m.AwayTeamLabel != "" {
			awayID, _ = s.ResolveUserTeam(userID, m.AwayTeamLabel)
		}

		// Fetch actual Team models
		m.HomeTeam = s.fetchTeam(homeID, m.HomeTeamLabel)
		m.AwayTeam = s.fetchTeam(awayID, m.AwayTeamLabel)

		// Fetch user's prediction (bet)
		var betHome, betAway int
		var advTeamID int64
		var isFav int
		err = s.db.QueryRow(`
			SELECT home_score, away_score, COALESCE(advancing_team_id, 0), COALESCE(is_favorite, 0)
			FROM bets WHERE user_id = $1 AND match_id = $2
		`, userID, m.ID).Scan(&betHome, &betAway, &advTeamID, &isFav)
		if err == nil {
			m.HasUserBet = true
			m.BetHomeScore = betHome
			m.BetAwayScore = betAway
			m.AdvancingTeamID = advTeamID
			m.IsFavorite = isFav == 1
		}

		matches = append(matches, m)
	}

	return matches, rows.Err()
}

func (s *BracketService) fetchTeam(id int64, label string) *models.Team {
	var t models.Team
	if id == 0 {
		t.ID = 0
		t.Name = label
		if t.Name == "" {
			t.Name = "A definir"
		}
		t.GroupName = ""
		t.FlagURL = ""
		return &t
	}

	var flagURL sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, group_name, flag_url FROM teams WHERE id = $1
	`, id).Scan(&t.ID, &t.Name, &t.GroupName, &flagURL)
	if err != nil {
		t.ID = id
		t.Name = fmt.Sprintf("Time %d", id)
		return &t
	}
	if flagURL.Valid {
		t.FlagURL = flagURL.String
	}
	return &t
}

// PlaceKnockoutBet saves a knockout match bet, including advancing team selection and favorite flag
func (s *BracketService) PlaceKnockoutBet(userID int64, matchID int64, homeScore int, awayScore int, advancingTeamID int64, isFavorite bool) error {
	var matchStatus, matchDate, matchTime, stage string
	var homeTeamID, awayTeamID int64
	var homeLabel, awayLabel string

	err := s.db.QueryRow(`
		SELECT status, match_date, match_time, stage, home_team_id, away_team_id, COALESCE(home_team_label, ''), COALESCE(away_team_label, '')
		FROM matches WHERE id = $1
	`, matchID).Scan(&matchStatus, &matchDate, &matchTime, &stage, &homeTeamID, &awayTeamID, &homeLabel, &awayLabel)
	if err != nil {
		return fmt.Errorf("jogo não encontrado")
	}

	if matchStatus != "upcoming" {
		return fmt.Errorf("jogo já iniciou ou terminou")
	}

	loc, _ := time.LoadLocation("America/Sao_Paulo")
	now := time.Now().In(loc)
	scheduled, err := time.ParseInLocation("2006-01-02 15:04", matchDate+" "+matchTime, loc)
	if err == nil && now.After(scheduled) {
		return fmt.Errorf("o horário do jogo já passou")
	}

	// Resolve the home/away teams for the user's context
	resolvedHome := homeTeamID
	if resolvedHome == 0 && homeLabel != "" {
		resolvedHome, _ = s.ResolveUserTeam(userID, homeLabel)
	}
	resolvedAway := awayTeamID
	if resolvedAway == 0 && awayLabel != "" {
		resolvedAway, _ = s.ResolveUserTeam(userID, awayLabel)
	}

	// Advancing team validation
	if advancingTeamID == 0 {
		if homeScore > awayScore {
			advancingTeamID = resolvedHome
		} else if awayScore > homeScore {
			advancingTeamID = resolvedAway
		} else {
			return fmt.Errorf("em caso de empate, selecione quem avança")
		}
	} else if advancingTeamID != resolvedHome && advancingTeamID != resolvedAway {
		return fmt.Errorf("equipe avançando inválida")
	}

	favVal := 0
	if isFavorite {
		favVal = 1
	}

	_, err = s.db.Exec(`
		INSERT INTO bets (user_id, match_id, home_score, away_score, advancing_team_id, is_favorite)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT(user_id, match_id) DO UPDATE SET
			home_score = EXCLUDED.home_score,
			away_score = EXCLUDED.away_score,
			advancing_team_id = EXCLUDED.advancing_team_id,
			is_favorite = EXCLUDED.is_favorite,
			updated_at = CURRENT_TIMESTAMP
	`, userID, matchID, homeScore, awayScore, advancingTeamID, favVal)
	if err != nil {
		return err
	}

	// Calculate and update points dynamically
	_ = s.CalculateUserStagePoints(userID, stage)

	return nil
}

// GetRealAdvancingTeam determines the actual team that advanced in the real match
func (s *BracketService) GetRealAdvancingTeam(matchID int64) (int64, error) {
	var homeTeamID, awayTeamID int64
	var homeScore, awayScore sql.NullInt64
	var stage string
	err := s.db.QueryRow(`
		SELECT home_team_id, away_team_id, home_score, away_score, stage
		FROM matches WHERE id = $1
	`, matchID).Scan(&homeTeamID, &awayTeamID, &homeScore, &awayScore, &stage)
	if err != nil {
		return 0, err
	}
	if !homeScore.Valid || !awayScore.Valid {
		return 0, nil // Not finished yet
	}
	if homeScore.Int64 > awayScore.Int64 {
		return homeTeamID, nil
	}
	if homeScore.Int64 < awayScore.Int64 {
		return awayTeamID, nil
	}
	// If it's a draw, look at subsequent matches where home_team_label or away_team_label is "Winner Match <matchID>"
	winnerLabel := fmt.Sprintf("Winner Match %d", matchID)
	var nextHomeTeamID, nextAwayTeamID int64
	err = s.db.QueryRow(`
		SELECT home_team_id, away_team_id
		FROM matches
		WHERE home_team_label = $1 OR away_team_label = $1
	`, winnerLabel).Scan(&nextHomeTeamID, &nextAwayTeamID)
	if err != nil {
		// If no next match (e.g. final), we assume there's no draw or we check penalties score.
		// For the World Cup final, the API will output a winner or we can look up the champion.
		return 0, nil
	}
	if nextHomeTeamID == homeTeamID || nextAwayTeamID == homeTeamID {
		return homeTeamID, nil
	}
	if nextHomeTeamID == awayTeamID || nextAwayTeamID == awayTeamID {
		return awayTeamID, nil
	}
	return 0, nil
}

// CalculateKnockoutMatchPoints computes points for a single knockout match.
//
// Rules:
//   - Match decided in normal time (realHome != realAway):
//     exact score = 5, correct winner = 3, nothing = 0
//   - Match tied after 90' (realHome == realAway):
//     exact score + correct advance = 8 (5+3)
//     exact score only = 5
//     correct advance only (but wrong score) = 5
//     predicted draw (any draw score, wrong advance) = 1
//     nothing = 0
func CalculateKnockoutMatchPoints(betHome, betAway, realHome, realAway int, advTeamID, realAdv int64) int {
	if realHome > realAway || realAway > realHome {
		// Decided in normal time
		if betHome == realHome && betAway == realAway {
			return 5
		}
		if (realHome > realAway && betHome > betAway) ||
			(realAway > realHome && betAway > betHome) {
			return 3
		}
		return 0
	}

	// Tied after 90'
	if realHome == realAway {
		isDrawBet := betHome == betAway
		isExact := isDrawBet && betHome == realHome && betAway == realAway
		isCorrectAdvance := realAdv > 0 && advTeamID > 0 && advTeamID == realAdv

		if isExact && isCorrectAdvance {
			return 8
		}
		if isExact {
			return 5
		}
		if isDrawBet && isCorrectAdvance {
			return 5
		}
		if isDrawBet {
			return 1
		}
		return 0
	}

	return 0
}

// CalculateUserStagePoints calculates the point breakdown for a user in a given stage
func (s *BracketService) CalculateUserStagePoints(userID int64, stage string) error {
	rows, err := s.db.Query(`
		SELECT id, home_score, away_score, status
		FROM matches
		WHERE stage = $1
	`, stage)
	if err != nil {
		return err
	}
	defer rows.Close()

	var matchIDs []int64
	var realScores map[int64][2]int = make(map[int64][2]int)

	for rows.Next() {
		var id int64
		var hs, as sql.NullInt64
		var status string
		if err := rows.Scan(&id, &hs, &as, &status); err == nil {
			matchIDs = append(matchIDs, id)
			if status == "finished" && hs.Valid && as.Valid {
				realScores[id] = [2]int{int(hs.Int64), int(as.Int64)}
			}
		}
	}

	if len(matchIDs) == 0 {
		return nil
	}

	commonPoints := 0
	exactScoreBonus := 0
	allFavoritesCorrect := true
	hasFavorites := false

	for _, matchID := range matchIDs {
		var betHome, betAway int
		var advTeamID int64
		var isFav int
		err := s.db.QueryRow(`
			SELECT home_score, away_score, COALESCE(advancing_team_id, 0), COALESCE(is_favorite, 0)
			FROM bets WHERE user_id = $1 AND match_id = $2
		`, userID, matchID).Scan(&betHome, &betAway, &advTeamID, &isFav)
		if err == sql.ErrNoRows {
			continue
		} else if err != nil {
			return err
		}

		isMatchFinished := false
		var realHome, realAway int
		if scores, ok := realScores[matchID]; ok {
			isMatchFinished = true
			realHome = scores[0]
			realAway = scores[1]
		}

		if isMatchFinished {
			realAdv, _ := s.GetRealAdvancingTeam(matchID)
			matchPoints := CalculateKnockoutMatchPoints(betHome, betAway, realHome, realAway, advTeamID, realAdv)

			commonPoints += matchPoints

			s.db.Exec(`UPDATE bets SET points = $1 WHERE user_id = $2 AND match_id = $3`, matchPoints, userID, matchID)

			if isFav == 1 {
				hasFavorites = true
				if realAdv == 0 || advTeamID != realAdv {
					allFavoritesCorrect = false
				}
			}
		} else {
			if isFav == 1 {
				hasFavorites = true
				allFavoritesCorrect = false
			}
		}
	}

	favoritesBonus := 0
	if hasFavorites && allFavoritesCorrect {
		favoritesBonus = 20
	}

	totalPoints := commonPoints + exactScoreBonus + favoritesBonus

	_, err = s.db.Exec(`
		INSERT INTO user_stage_points (user_id, stage, common_points, exact_score_bonus, favorites_bonus, total_points)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT(user_id, stage) DO UPDATE SET
			common_points = EXCLUDED.common_points,
			exact_score_bonus = EXCLUDED.exact_score_bonus,
			favorites_bonus = EXCLUDED.favorites_bonus,
			total_points = EXCLUDED.total_points
	`, userID, stage, commonPoints, exactScoreBonus, favoritesBonus, totalPoints)
	if err != nil {
		log.Printf("Failed to save stage points: %v", err)
	}

	return nil
}

func (s *BracketService) ResolveSimulatedMatch(userID int64, m *models.Match) {
	if m.Stage == "group" {
		return
	}
	homeID := m.HomeTeamID
	if homeID == 0 && m.HomeTeamLabel != "" {
		homeID, _ = s.ResolveUserTeam(userID, m.HomeTeamLabel)
	}
	awayID := m.AwayTeamID
	if awayID == 0 && m.AwayTeamLabel != "" {
		awayID, _ = s.ResolveUserTeam(userID, m.AwayTeamLabel)
	}
	m.HomeTeamID = homeID
	m.AwayTeamID = awayID
	m.HomeTeam = s.fetchTeam(homeID, m.HomeTeamLabel)
	m.AwayTeam = s.fetchTeam(awayID, m.AwayTeamLabel)
}
