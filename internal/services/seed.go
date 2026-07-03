package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type SeedService struct {
	db *sql.DB
}

func NewSeedService(db *sql.DB) *SeedService {
	return &SeedService{db: db}
}

type SeedTeam struct {
	ID       string `json:"id"`
	NameEn   string `json:"name_en"`
	FifaCode string `json:"fifa_code"`
	Groups   string `json:"groups"`
	Flag     string `json:"flag"`
}

type SeedMatch struct {
	ID          string `json:"id"`
	HomeTeamID  string `json:"home_team_id"`
	AwayTeamID  string `json:"away_team_id"`
	HomeScore   string `json:"home_score"`
	AwayScore   string `json:"away_score"`
	Group       string `json:"group"`
	Matchday    string `json:"matchday"`
	LocalDate   string `json:"local_date"`
	StadiumID   string `json:"stadium_id"`
	Finished    string `json:"finished"`
	TimeElapsed string `json:"time_elapsed"`
	Type        string `json:"type"`
}

type SeedStadium struct {
	ID     string `json:"id"`
	CityEn string `json:"city_en"`
}

func (s *SeedService) SeedFromFiles() error {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM teams").Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		log.Printf("Database already has %d teams, skipping seed", count)
		s.ensureMatches()
		s.updateMatchDetails()
		return nil
	}

	if err := s.seedTeams(); err != nil {
		return fmt.Errorf("seed teams: %w", err)
	}

	if err := s.seedMatches(); err != nil {
		return fmt.Errorf("seed matches: %w", err)
	}

	return nil
}

func (s *SeedService) ensureMatches() {
	s.db.Exec(`
		INSERT INTO teams (id, name, fifa_code, group_name, flag_url)
		VALUES (0, 'TBD', 'tbd', '', '')
		ON CONFLICT (id) DO NOTHING
	`)

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM matches").Scan(&count)
	if count >= 104 {
		return
	}

	data, err := os.ReadFile("data/seed/football.matches.json")
	if err != nil {
		log.Printf("Could not read matches for ensure: %v", err)
		return
	}

	var seedMatches []SeedMatch
	if err := json.Unmarshal(data, &seedMatches); err != nil {
		log.Printf("Could not parse matches for ensure: %v", err)
		return
	}

	inserted := 0
	for _, m := range seedMatches {
		var exists int
		s.db.QueryRow("SELECT 1 FROM matches WHERE id = $1", m.ID).Scan(&exists)
		if exists == 1 {
			continue
		}

		date, matchTime := "", ""
		stadiums := s.loadStadiums()
		cityName := stadiumCity(stadiums, m.StadiumID)
		if cityName != "" {
			date, matchTime = s.toBrasiliaTime(m.LocalDate, stadiumOffset(cityName))
		}
		if date == "" {
			date = "2026-01-01"
		}

		s.db.Exec(`
			INSERT INTO matches (id, home_team_id, away_team_id, match_date, match_time, stage, group_name, stadium, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'upcoming')
			ON CONFLICT (id) DO NOTHING
		`, m.ID, m.HomeTeamID, m.AwayTeamID, date, matchTime, m.Type, m.Group, cityName)
		inserted++
	}
	if inserted > 0 {
		log.Printf("Inserted %d missing matches from seed", inserted)
	}
}

func (s *SeedService) updateMatchDetails() {
	stadiums := s.loadStadiums()

	data, err := os.ReadFile("data/seed/football.matches.json")
	if err != nil {
		log.Printf("Could not read matches for timezone update: %v", err)
		return
	}

	var seedMatches []SeedMatch
	if err := json.Unmarshal(data, &seedMatches); err != nil {
		log.Printf("Could not parse matches for timezone update: %v", err)
		return
	}

	count := 0
	for _, sm := range seedMatches {
		cityName := stadiumCity(stadiums, sm.StadiumID)
		if cityName == "" {
			continue
		}

		date, matchTime := s.toBrasiliaTime(sm.LocalDate, stadiumOffset(cityName))
		if date == "" {
			continue
		}

		// Base update: stadium and time
		_, err := s.db.Exec(
			"UPDATE matches SET stadium = $1, match_date = $2, match_time = $3 WHERE id = $4",
			cityName, date, matchTime, sm.ID,
		)
		if err == nil {
			count++
		}

		// Sync status/scores with seed: only set finished when seed has valid scores
		// Never downgrade matches that are already processed (have points awarded)
		if sm.Finished == "TRUE" || sm.TimeElapsed == "finished" {
			hs, err1 := strconv.Atoi(sm.HomeScore)
			as, err2 := strconv.Atoi(sm.AwayScore)
			if err1 == nil && err2 == nil {
				s.db.Exec(
					"UPDATE matches SET status = 'finished', home_score = $1, away_score = $2 WHERE id = $3 AND (status != 'finished' OR home_score IS NULL)",
					hs, as, sm.ID,
				)
			}
		}

		// Update home/away team IDs for all matches where seed has real team IDs
		if sm.HomeTeamID != "" && sm.HomeTeamID != "0" {
			s.db.Exec("UPDATE matches SET home_team_id = $1 WHERE id = $2", sm.HomeTeamID, sm.ID)
		}
		if sm.AwayTeamID != "" && sm.AwayTeamID != "0" {
			s.db.Exec("UPDATE matches SET away_team_id = $1 WHERE id = $2", sm.AwayTeamID, sm.ID)
		}
	}
	if count > 0 {
		log.Printf("Updated %d matches with Brasília time and results", count)
	}

	// Populate knockout labels programmatically for any matches that still have them empty
	s.setKnockoutLabels()
}

var koLabels = map[int64][2]string{
	73:  {"Runner-up Group A", "Runner-up Group B"},
	74:  {"Winner Group E", "Runner-up Group D"},
	75:  {"Winner Group F", "Runner-up Group C"},
	76:  {"Winner Group C", "Runner-up Group F"},
	77:  {"Winner Group I", "3rd Group F"},
	78:  {"Runner-up Group E", "3rd Group I"},
	79:  {"Winner Group A", "3rd Group E"},
	80:  {"Winner Group L", "3rd Group E/H/I/J/K"},
	81:  {"Winner Group D", "3rd Group B"},
	82:  {"Winner Group G", "3rd Group A/E/H/I/J"},
	83:  {"Runner-up Group K", "Runner-up Group L"},
	84:  {"Winner Group H", "Runner-up Group J"},
	85:  {"Winner Group B", "3rd Group E/F/G/I/J"},
	86:  {"Winner Group J", "Runner-up Group H"},
	87:  {"Winner Group K", "3rd Group L"},
	88:  {"Runner-up Group D", "Runner-up Group G"},
	89:  {"Winner Match 74", "Winner Match 77"},
	90:  {"Winner Match 73", "Winner Match 75"},
	91:  {"Winner Match 76", "Winner Match 78"},
	92:  {"Winner Match 79", "Winner Match 80"},
	93:  {"Winner Match 83", "Winner Match 84"},
	94:  {"Winner Match 81", "Winner Match 82"},
	95:  {"Winner Match 86", "Winner Match 88"},
	96:  {"Winner Match 85", "Winner Match 87"},
	97:  {"Winner Match 89", "Winner Match 90"},
	98:  {"Winner Match 93", "Winner Match 94"},
	99:  {"Winner Match 91", "Winner Match 92"},
	100: {"Winner Match 95", "Winner Match 96"},
	101: {"Winner Match 97", "Winner Match 98"},
	102: {"Winner Match 99", "Winner Match 100"},
	103: {"Loser Match 101", "Loser Match 102"},
	104: {"Winner Match 101", "Winner Match 102"},
}

func (s *SeedService) setKnockoutLabels() {
	knocked := false
	for id, labels := range koLabels {
		homeLabel := labels[0]
		awayLabel := labels[1]
		if homeLabel == "" && awayLabel == "" {
			continue
		}
		var curHome, curAway string
		err := s.db.QueryRow("SELECT COALESCE(home_team_label,''), COALESCE(away_team_label,'') FROM matches WHERE id = $1", id).Scan(&curHome, &curAway)
		if err != nil {
			continue
		}
		newHome := curHome
		newAway := curAway
		if curHome == "" && homeLabel != "" {
			newHome = homeLabel
		}
		if curAway == "" && awayLabel != "" {
			newAway = awayLabel
		}
		if newHome != curHome || newAway != curAway {
			s.db.Exec("UPDATE matches SET home_team_label = $1, away_team_label = $2 WHERE id = $3", newHome, newAway, id)
			knocked = true
		}
	}
	if knocked {
		log.Printf("Knockout labels populated programmatically")
	}
}

func (s *SeedService) seedTeams() error {
	data, err := os.ReadFile("data/seed/football.teams.json")
	if err != nil {
		return fmt.Errorf("read teams file: %w", err)
	}

	var teams []SeedTeam
	if err := json.Unmarshal(data, &teams); err != nil {
		return fmt.Errorf("parse teams: %w", err)
	}

	// Insert TBD placeholder for knockout matches without defined teams
	s.db.Exec(`
		INSERT INTO teams (id, name, fifa_code, group_name, flag_url)
		VALUES (0, 'TBD', 'tbd', '', '')
		ON CONFLICT (id) DO NOTHING
	`)

	for _, t := range teams {
		flagURL := t.Flag
		if flagURL == "" {
			flagURL = fmt.Sprintf("https://flagcdn.com/w80/%s.png", t.FifaCode)
		}
		_, err := s.db.Exec(`
			INSERT INTO teams (id, name, fifa_code, group_name, flag_url)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO NOTHING
		`, t.ID, t.NameEn, t.FifaCode, t.Groups, flagURL)
		if err != nil {
			return fmt.Errorf("insert team %s: %w", t.NameEn, err)
		}
	}

	log.Printf("Seeded %d teams", len(teams))
	return nil
}

func (s *SeedService) seedMatches() error {
	data, err := os.ReadFile("data/seed/football.matches.json")
	if err != nil {
		return fmt.Errorf("read matches file: %w", err)
	}

	var matches []SeedMatch
	if err := json.Unmarshal(data, &matches); err != nil {
		return fmt.Errorf("parse matches: %w", err)
	}

	stadiums := s.loadStadiums()

	for _, m := range matches {
		date, time := s.toBrasiliaTime(m.LocalDate, stadiumOffset(stadiumCity(stadiums, m.StadiumID)))
		cityName := stadiumCity(stadiums, m.StadiumID)

		_, err := s.db.Exec(`
			INSERT INTO matches (id, home_team_id, away_team_id, match_date, match_time, stage, group_name, stadium, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'upcoming')
			ON CONFLICT (id) DO NOTHING
		`, m.ID, m.HomeTeamID, m.AwayTeamID, date, time, m.Type, m.Group, cityName)
		if err != nil {
			return fmt.Errorf("insert match %s: %w", m.ID, err)
		}
	}

	log.Printf("Seeded %d matches", len(matches))
	return nil
}

func (s *SeedService) loadStadiums() map[string]string {
	data, err := os.ReadFile("data/seed/football.stadiums.json")
	if err != nil {
		log.Printf("Could not read stadiums file: %v", err)
		return nil
	}

	var raw []map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Printf("Could not parse stadiums: %v", err)
		return nil
	}

	stadiums := make(map[string]string)
	for _, st := range raw {
		id := fmt.Sprintf("%v", st["id"])
		city := ""
		if c, ok := st["city_en"].(string); ok {
			city = c
		}
		if id != "" && city != "" {
			stadiums[id] = city
		}
	}
	return stadiums
}

func stadiumCity(stadiums map[string]string, id string) string {
	if stadiums == nil {
		return ""
	}
	return stadiums[id]
}

var offsetByCityKeyword = map[string]int{
	"mexico":         3,
	"guadalajara":    3,
	"monterrey":      3,
	"dallas":         2,
	"houston":        2,
	"kansas":         2,
	"atlanta":        1,
	"miami":          1,
	"boston":         1,
	"philadelphia":   1,
	"new york":       1,
	"toronto":        1,
	"vancouver":      4,
	"seattle":        4,
	"san francisco":  4,
	"los angeles":    4,
}

func stadiumOffset(city string) int {
	lower := strings.ToLower(city)
	for keyword, offset := range offsetByCityKeyword {
		if strings.Contains(lower, keyword) {
			return offset
		}
	}
	return 1
}

func fixupDate(date string) string {
	if len(date) != 10 {
		return date
	}
	return date[6:10] + "-" + date[0:2] + "-" + date[3:5]
}

func (s *SeedService) toBrasiliaTime(localDate string, offsetHours int) (string, string) {
	if len(localDate) < 16 {
		if len(localDate) >= 10 {
			d := fixupDate(localDate[:10])
			return d, ""
		}
		return localDate, ""
	}

	datePart := localDate[:10]
	timePart := localDate[11:16]

	parsed, err := time.Parse("01/02/2006 15:04", localDate[:16])
	if err != nil {
		parsed, err = time.Parse("2006-01-02 15:04", fixupDate(datePart)+" "+timePart)
		if err != nil {
			return fixupDate(datePart), timePart
		}
	}

	parsed = parsed.Add(time.Duration(offsetHours) * time.Hour)

	return parsed.Format("2006-01-02"), parsed.Format("15:04")
}
