package services

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"copa-2026/internal/models"
)

var specialBettingDeadline = time.Date(2026, 6, 23, 2, 59, 59, 0, time.UTC)

func IsSpecialBettingOpen() bool {
	return time.Now().Before(specialBettingDeadline)
}

type BetService struct {
	db *sql.DB
}

func NewBetService(db *sql.DB) *BetService {
	return &BetService{db: db}
}

func (s *BetService) PlaceBet(userID, matchID int64, homeScore, awayScore int) error {
	var matchStatus, matchDate, matchTime string
	err := s.db.QueryRow("SELECT status, match_date, match_time FROM matches WHERE id = $1", matchID).Scan(&matchStatus, &matchDate, &matchTime)
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

	var existingID int
	var existingHome, existingAway int
	err = s.db.QueryRow(`
		SELECT id, home_score, away_score FROM bets WHERE user_id = $1 AND match_id = $2
	`, userID, matchID).Scan(&existingID, &existingHome, &existingAway)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	_, err = s.db.Exec(`
		INSERT INTO bets (user_id, match_id, home_score, away_score)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT(user_id, match_id) DO UPDATE SET
			home_score = EXCLUDED.home_score,
			away_score = EXCLUDED.away_score,
			updated_at = CURRENT_TIMESTAMP
	`, userID, matchID, homeScore, awayScore)
	if err != nil {
		return err
	}

	if existingID > 0 && (existingHome != homeScore || existingAway != awayScore) {
		s.db.Exec(`
			INSERT INTO bet_history (bet_id, user_id, match_id, old_home_score, old_away_score, new_home_score, new_away_score)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, existingID, userID, matchID, existingHome, existingAway, homeScore, awayScore)
	}

	return nil
}

func (s *BetService) GetUserBet(userID, matchID int64) (*models.Bet, error) {
	bet := &models.Bet{}
	var updatedAt sql.NullString
	err := s.db.QueryRow(`
		SELECT id, user_id, match_id, home_score, away_score, COALESCE(advancing_team_id, 0), points, created_at, updated_at
		FROM bets WHERE user_id = $1 AND match_id = $2
	`, userID, matchID).Scan(&bet.ID, &bet.UserID, &bet.MatchID, &bet.HomeScore, &bet.AwayScore, &bet.AdvancingTeamID, &bet.Points, &bet.CreatedAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return bet, nil
}

func (s *BetService) GetUserBets(userID int64) ([]models.Bet, error) {
	rows, err := s.db.Query(`
		SELECT b.id, b.user_id, b.match_id, b.home_score, b.away_score, COALESCE(b.advancing_team_id, 0), b.points, b.created_at, b.updated_at,
			m.id, m.home_team_id, m.away_team_id, m.home_score, m.away_score, m.match_date, m.match_time, m.stage, m.group_name, m.stadium, m.status,
			ht.id, ht.name, ht.fifa_code, ht.group_name, ht.flag_url,
			at.id, at.name, at.fifa_code, at.group_name, at.flag_url
		FROM bets b
		JOIN matches m ON m.id = b.match_id
		JOIN teams ht ON ht.id = m.home_team_id
		JOIN teams at ON at.id = m.away_team_id
		WHERE b.user_id = $1
		ORDER BY m.match_date, m.match_time
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanBets(rows)
}

func (s *BetService) GetAllBetsForMatch(matchID int64) ([]models.Bet, error) {
	rows, err := s.db.Query(`
		SELECT b.id, b.user_id, b.match_id, b.home_score, b.away_score, COALESCE(b.advancing_team_id, 0), b.points, b.created_at, b.updated_at,
			m.id, m.home_team_id, m.away_team_id, m.home_score, m.away_score, m.match_date, m.match_time, m.stage, m.group_name, m.stadium, m.status,
			u.id, u.name
		FROM bets b
		JOIN matches m ON m.id = b.match_id
		JOIN users u ON u.id = b.user_id
		WHERE b.match_id = $1
		ORDER BY u.name
	`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bets []models.Bet
	for rows.Next() {
		var b models.Bet
		var updatedAt sql.NullString
		var homeScore, awayScore sql.NullInt64
		var stadium, groupName sql.NullString

		b.Match = &models.Match{}
		b.User = &models.User{}

		err := rows.Scan(&b.ID, &b.UserID, &b.MatchID, &b.HomeScore, &b.AwayScore, &b.AdvancingTeamID, &b.Points, &b.CreatedAt, &updatedAt,
			&b.Match.ID, &b.Match.HomeTeamID, &b.Match.AwayTeamID, &homeScore, &awayScore,
			&b.Match.MatchDate, &b.Match.MatchTime, &b.Match.Stage, &groupName, &stadium, &b.Match.Status,
			&b.User.ID, &b.User.Name)
		if err != nil {
			return nil, err
		}

		if homeScore.Valid {
			hs := int(homeScore.Int64)
			b.Match.HomeScore = &hs
		}
		if awayScore.Valid {
			as := int(awayScore.Int64)
			b.Match.AwayScore = &as
		}
		if stadium.Valid {
			b.Match.Stadium = stadium.String
		}
		if groupName.Valid {
			b.Match.GroupName = groupName.String
		}

		bets = append(bets, b)
	}

	return bets, rows.Err()
}

func (s *BetService) RecalculateMatchBets(matchID int64) error {
	match, err := s.GetMatchByID(matchID)
	if err != nil {
		return err
	}
	if !match.HasResult() || match.Status != "finished" {
		return nil
	}

	bets, err := s.GetAllBetsForMatch(matchID)
	if err != nil {
		return err
	}

	isKnockout := match.Stage != "group"

	for _, bet := range bets {
		var points int
		if isKnockout {
			// Fetch advancing_team_id from match for knockout scoring
			var realAdv int64
			s.db.QueryRow("SELECT COALESCE(advancing_team_id, 0) FROM matches WHERE id = $1", matchID).Scan(&realAdv)
			match.AdvancingTeamID = realAdv
			points = bet.CalculateKnockoutPoints(match)
		} else {
			points = bet.CalculatePoints(match)
		}
		_, err := s.db.Exec("UPDATE bets SET points = $1 WHERE id = $2", points, bet.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *BetService) RecalculateAllFinishedMatches() {
	rows, err := s.db.Query("SELECT id FROM matches WHERE status = 'finished' AND home_score IS NOT NULL AND away_score IS NOT NULL")
	if err != nil {
		log.Printf("Error querying finished matches for recalculation: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			continue
		}
		if err := s.RecalculateMatchBets(id); err != nil {
			log.Printf("Error recalculating match %d: %v", id, err)
		}
	}
}

func (s *BetService) ResolveSpecialBet(betType string, correctValue string) error {
	points, ok := models.SpecialBetPoints[betType]
	if !ok {
		return fmt.Errorf("tipo de aposta inválido: %s", betType)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("UPDATE special_bets SET points = 0 WHERE bet_type = $1", betType); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE special_bets SET points = $1 WHERE bet_type = $2 AND value = $3", points, betType, correctValue); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *BetService) GetMatchByID(matchID int64) (*models.Match, error) {
	m := &models.Match{}
	m.HomeTeam = &models.Team{}
	m.AwayTeam = &models.Team{}
	var homeScore, awayScore sql.NullInt64
	var stadium, groupName sql.NullString
	err := s.db.QueryRow(`
		SELECT m.id, m.home_team_id, m.away_team_id,
			m.home_score, m.away_score, m.match_date, m.match_time, m.stage, m.group_name, m.stadium, m.status,
			COALESCE(ht.id, 0), COALESCE(ht.name, ''), COALESCE(ht.fifa_code, ''), COALESCE(ht.group_name, ''), COALESCE(ht.flag_url, ''),
			COALESCE(at.id, 0), COALESCE(at.name, ''), COALESCE(at.fifa_code, ''), COALESCE(at.group_name, ''), COALESCE(at.flag_url, '')
		FROM matches m
		LEFT JOIN teams ht ON ht.id = m.home_team_id
		LEFT JOIN teams at ON at.id = m.away_team_id
		WHERE m.id = $1
	`, matchID).Scan(&m.ID, &m.HomeTeamID, &m.AwayTeamID, &homeScore, &awayScore, &m.MatchDate, &m.MatchTime, &m.Stage, &groupName, &stadium, &m.Status,
		&m.HomeTeam.ID, &m.HomeTeam.Name, &m.HomeTeam.FifaCode, &m.HomeTeam.GroupName, &m.HomeTeam.FlagURL,
		&m.AwayTeam.ID, &m.AwayTeam.Name, &m.AwayTeam.FifaCode, &m.AwayTeam.GroupName, &m.AwayTeam.FlagURL)
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

type BetHistoryEntry struct {
	BetID       int64
	UserID      int64
	UserName    string
	MatchID     int64
	OldHome     *int
	OldAway     *int
	NewHome     int
	NewAway     int
	ChangedAt   string
}

func (s *BetService) GetBetHistory(matchID int64) ([]BetHistoryEntry, error) {
	rows, err := s.db.Query(`
		SELECT bh.bet_id, bh.user_id, u.name, bh.match_id,
			bh.old_home_score, bh.old_away_score, bh.new_home_score, bh.new_away_score, bh.changed_at
		FROM bet_history bh
		JOIN users u ON u.id = bh.user_id
		WHERE bh.match_id = $1
		ORDER BY bh.changed_at
	`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []BetHistoryEntry
	for rows.Next() {
		var e BetHistoryEntry
		var oldHome, oldAway sql.NullInt64
		var changedAt sql.NullString
		if err := rows.Scan(&e.BetID, &e.UserID, &e.UserName, &e.MatchID, &oldHome, &oldAway, &e.NewHome, &e.NewAway, &changedAt); err != nil {
			return nil, err
		}
		if oldHome.Valid {
			v := int(oldHome.Int64)
			e.OldHome = &v
		}
		if oldAway.Valid {
			v := int(oldAway.Int64)
			e.OldAway = &v
		}
		if changedAt.Valid {
			e.ChangedAt = changedAt.String
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func scanBets(rows *sql.Rows) ([]models.Bet, error) {
	var bets []models.Bet
	for rows.Next() {
		var b models.Bet
		var updatedAt sql.NullString
		var homeScore, awayScore sql.NullInt64
		var stadium, groupName sql.NullString

		b.Match = &models.Match{}
		b.Match.HomeTeam = &models.Team{}
		b.Match.AwayTeam = &models.Team{}

		err := rows.Scan(
			&b.ID, &b.UserID, &b.MatchID, &b.HomeScore, &b.AwayScore, &b.AdvancingTeamID, &b.Points, &b.CreatedAt, &updatedAt,
			&b.Match.ID, &b.Match.HomeTeamID, &b.Match.AwayTeamID, &homeScore, &awayScore,
			&b.Match.MatchDate, &b.Match.MatchTime, &b.Match.Stage, &groupName, &stadium, &b.Match.Status,
			&b.Match.HomeTeam.ID, &b.Match.HomeTeam.Name, &b.Match.HomeTeam.FifaCode, &b.Match.HomeTeam.GroupName, &b.Match.HomeTeam.FlagURL,
			&b.Match.AwayTeam.ID, &b.Match.AwayTeam.Name, &b.Match.AwayTeam.FifaCode, &b.Match.AwayTeam.GroupName, &b.Match.AwayTeam.FlagURL,
		)
		if err != nil {
			return nil, err
		}

		if homeScore.Valid {
			hs := int(homeScore.Int64)
			b.Match.HomeScore = &hs
		}
		if awayScore.Valid {
			as := int(awayScore.Int64)
			b.Match.AwayScore = &as
		}
		if stadium.Valid {
			b.Match.Stadium = stadium.String
		}
		if groupName.Valid {
			b.Match.GroupName = groupName.String
		}

		bets = append(bets, b)
	}
	return bets, rows.Err()
}
