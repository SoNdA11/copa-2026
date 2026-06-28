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

		// If the seed file already has a result, update status and score too
		if sm.Finished == "TRUE" || sm.TimeElapsed == "finished" {
			hs, err1 := strconv.Atoi(sm.HomeScore)
			as, err2 := strconv.Atoi(sm.AwayScore)
			if err1 == nil && err2 == nil {
				s.db.Exec(
					"UPDATE matches SET status = 'finished', home_score = $1, away_score = $2 WHERE id = $3 AND status != 'finished'",
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
