package models

type Match struct {
	ID          int64  `json:"id"`
	HomeTeamID  int64  `json:"home_team_id"`
	AwayTeamID  int64  `json:"away_team_id"`
	HomeScore   *int   `json:"home_score"`
	AwayScore   *int   `json:"away_score"`
	MatchDate   string `json:"match_date"`
	MatchTime   string `json:"match_time"`
	Stage       string `json:"stage"`
	GroupName   string `json:"group_name"`
	Stadium     string `json:"stadium"`
	Status      string `json:"status"`

	HomeTeam   *Team `json:"home_team,omitempty"`
	AwayTeam   *Team `json:"away_team,omitempty"`
	HasUserBet   bool `json:"-"`
	BetHomeScore int  `json:"-"`
	BetAwayScore int  `json:"-"`
}

func (m *Match) StageLabel() string {
	labels := map[string]string{
		"group": "Fase de Grupos",
		"r32":   "16 Avos",
		"r16":   "Oitavas",
		"qf":    "Quartas",
		"sf":    "Semi",
		"third": "3º Lugar",
		"final": "Final",
	}
	if l, ok := labels[m.Stage]; ok {
		return l
	}
	return m.Stage
}

func (m *Match) IsKnockout() bool {
	return m.Stage != "group"
}

func (m *Match) HasResult() bool {
	return m.HomeScore != nil && m.AwayScore != nil
}
