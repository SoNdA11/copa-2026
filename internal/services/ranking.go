package services

import (
	"database/sql"

	"copa-2026/internal/models"
)

type RankingService struct {
	db *sql.DB
}

func NewRankingService(db *sql.DB) *RankingService {
	return &RankingService{db: db}
}

func (s *RankingService) GetRanking() ([]models.UserRanking, error) {
	rows, err := s.db.Query(`
		SELECT u.id, u.name,
			COALESCE(b.total_bet_points, 0) + COALESCE(s.total_special_points, 0) + COALESCE(u.points_adjustment, 0) as total_points
		FROM users u
		LEFT JOIN (
			SELECT user_id, SUM(points) as total_bet_points
			FROM bets
			GROUP BY user_id
		) b ON b.user_id = u.id
		LEFT JOIN (
			SELECT user_id, SUM(points) as total_special_points
			FROM special_bets
			GROUP BY user_id
		) s ON s.user_id = u.id
		WHERE COALESCE(u.is_admin, 0) = 0
		ORDER BY total_points DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rankings []models.UserRanking
	position := 1
	for rows.Next() {
		var r models.UserRanking
		if err := rows.Scan(&r.UserID, &r.Name, &r.Points); err != nil {
			return nil, err
		}
		r.Position = position
		position++
		rankings = append(rankings, r)
	}

	return rankings, rows.Err()
}

func (s *RankingService) GetUserPosition(userID int64) (*models.UserRanking, error) {
	rankings, err := s.GetRanking()
	if err != nil {
		return nil, err
	}

	for _, r := range rankings {
		if r.UserID == userID {
			return &r, nil
		}
	}

	return nil, nil
}

func (s *RankingService) GetUserTotalPoints(userID int64) (int, error) {
	var total int
	err := s.db.QueryRow(`
		SELECT COALESCE(SUM(points), 0) + COALESCE((SELECT points_adjustment FROM users WHERE id = $1), 0) FROM (
			SELECT points FROM bets WHERE user_id = $2
			UNION ALL
			SELECT points FROM special_bets WHERE user_id = $3
		)
	`, userID, userID, userID).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}
