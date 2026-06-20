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
	var matchStatus string
	err := s.db.QueryRow("SELECT status FROM matches WHERE id = $1", matchID).Scan(&matchStatus)
	if err != nil {
		return fmt.Errorf("jogo não encontrado")
	}
	if matchStatus != "upcoming" {
		return fmt.Errorf("jogo já iniciou ou terminou")
	}

	_, err = s.db.Exec(`
		INSERT INTO bets (user_id, match_id, home_score, away_score)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT(user_id, match_id) DO UPDATE SET
			home_score = EXCLUDED.home_score,
			away_score = EXCLUDED.away_score,
			updated_at = CURRENT_TIMESTAMP
	`, userID, matchID, homeScore, awayScore)
	return err
}

func (s *BetService) GetUserBet(userID, matchID int64) (*models.Bet, error) {
	bet := &models.Bet{}
	var updatedAt sql.NullString
	err := s.db.QueryRow(`
		SELECT id, user_id, match_id, home_score, away_score, points, created_at, updated_at
		FROM bets WHERE user_id = $1 AND match_id = $2
	`, userID, matchID).Scan(&bet.ID, &bet.UserID, &bet.MatchID, &bet.HomeScore, &bet.AwayScore, &bet.Points, &bet.CreatedAt, &updatedAt)
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
		SELECT b.id, b.user_id, b.match_id, b.home_score, b.away_score, b.points, b.created_at, b.updated_at,
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
		SELECT b.id, b.user_id, b.match_id, b.home_score, b.away_score, b.points, b.created_at, b.updated_at,
			m.id, m.home_team_id, m.away_team_id, m.home_score, m.away_score, m.match_date, m.match_time, m.stage, m.group_name, m.stadium, m.status
		FROM bets b
		JOIN matches m ON m.id = b.match_id
		WHERE b.match_id = $1
		ORDER BY b.id
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

		err := rows.Scan(&b.ID, &b.UserID, &b.MatchID, &b.HomeScore, &b.AwayScore, &b.Points, &b.CreatedAt, &updatedAt,
			&b.Match.ID, &b.Match.HomeTeamID, &b.Match.AwayTeamID, &homeScore, &awayScore,
			&b.Match.MatchDate, &b.Match.MatchTime, &b.Match.Stage, &groupName, &stadium, &b.Match.Status)
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

	for _, bet := range bets {
		points := bet.CalculatePoints(match)
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

func (s *BetService) GetMatchByID(matchID int64) (*models.Match, error) {
	m := &models.Match{}
	var homeScore, awayScore sql.NullInt64
	var stadium, groupName sql.NullString
	err := s.db.QueryRow(`
		SELECT id, home_team_id, away_team_id, home_score, away_score, match_date, match_time, stage, group_name, stadium, status
		FROM matches WHERE id = $1
	`, matchID).Scan(&m.ID, &m.HomeTeamID, &m.AwayTeamID, &homeScore, &awayScore, &m.MatchDate, &m.MatchTime, &m.Stage, &groupName, &stadium, &m.Status)
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
			&b.ID, &b.UserID, &b.MatchID, &b.HomeScore, &b.AwayScore, &b.Points, &b.CreatedAt, &updatedAt,
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
