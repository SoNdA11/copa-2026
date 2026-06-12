package models

import "time"

type Bet struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	MatchID   int64     `json:"match_id"`
	HomeScore int       `json:"home_score"`
	AwayScore int       `json:"away_score"`
	Points    int       `json:"points"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Match *Match `json:"match,omitempty"`
	User  *User  `json:"user,omitempty"`
}

func (b *Bet) IsCorrectWinner(match *Match) bool {
	if match.HomeScore == nil || match.AwayScore == nil {
		return false
	}

	mHome := *match.HomeScore
	mAway := *match.AwayScore

	if mHome == mAway {
		return b.HomeScore == b.AwayScore
	}
	return (mHome > mAway && b.HomeScore > b.AwayScore) ||
		(mHome < mAway && b.HomeScore < b.AwayScore)
}

func (b *Bet) IsExactScore(match *Match) bool {
	if match.HomeScore == nil || match.AwayScore == nil {
		return false
	}
	return *match.HomeScore == b.HomeScore && *match.AwayScore == b.AwayScore
}

func (b *Bet) CalculatePoints(match *Match) int {
	if !match.HasResult() {
		return 0
	}

	if b.IsExactScore(match) {
		return 3
	}

	if b.IsCorrectWinner(match) {
		return 1
	}

	return 0
}
