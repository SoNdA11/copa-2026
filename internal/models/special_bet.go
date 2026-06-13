package models

import "time"

type SpecialBet struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	BetType   string    `json:"bet_type"`
	Value     string    `json:"value"`
	Points    int       `json:"points"`
	CreatedAt time.Time `json:"created_at"`

	User *User `json:"user,omitempty"`
}

var BetTypeLabels = map[string]string{
	"champion":          "Campeão da Copa",
	"best_player":       "Melhor Jogador",
	"top_scorer":        "Artilheiro",
	"best_goalkeeper":   "Melhor Goleiro",
	"best_young_player": "Melhor Jovem",
}

func (s *SpecialBet) TypeLabel() string {
	if l, ok := BetTypeLabels[s.BetType]; ok {
		return l
	}
	return s.BetType
}

var SpecialBetPoints = map[string]int{
	"champion":          10,
	"best_player":       5,
	"top_scorer":        5,
	"best_goalkeeper":   5,
	"best_young_player": 5,
}

func (s *SpecialBet) MaxPoints() int {
	if p, ok := SpecialBetPoints[s.BetType]; ok {
		return p
	}
	return 0
}
