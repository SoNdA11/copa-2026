package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

type SyncService struct {
	db     *sql.DB
	apiURL string
	betSvc *BetService
}

type APIMatch struct {
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

type APIMatchesResponse struct {
	Games []APIMatch `json:"games"`
}

type APITeam struct {
	ID        string `json:"id"`
	NameEn    string `json:"name_en"`
	FifaCode  string `json:"fifa_code"`
	Groups    string `json:"groups"`
	Flag      string `json:"flag"`
}

type APITeamsResponse struct {
	Teams []APITeam `json:"teams"`
}

func NewSyncService(db *sql.DB, apiURL string, betSvc *BetService) *SyncService {
	return &SyncService{db: db, apiURL: apiURL, betSvc: betSvc}
}

func (s *SyncService) Start() {
	go func() {
		for {
			interval := s.determineInterval()
			time.Sleep(interval)
			if err := s.SyncMatches(); err != nil {
				log.Printf("Sync error: %v", err)
			}
		}
	}()
}

func (s *SyncService) determineInterval() time.Duration {
	var liveCount int
	s.db.QueryRow("SELECT COUNT(*) FROM matches WHERE status = 'live'").Scan(&liveCount)
	if liveCount > 0 {
		return 30 * time.Second
	}

	var upcomingSoon int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM matches
		WHERE status = 'upcoming'
		AND (match_date || ' ' || match_time)::timestamp BETWEEN NOW() AND NOW() + INTERVAL '2 hours'
	`).Scan(&upcomingSoon)
	if upcomingSoon > 0 {
		return 60 * time.Second
	}

	return 5 * time.Minute
}

func (s *SyncService) SyncMatch(matchID int64) error {
	matches, err := s.fetchMatches()
	if err != nil {
		return fmt.Errorf("failed to fetch matches: %w", err)
	}

	matchIDStr := fmt.Sprintf("%d", matchID)
	for _, apiMatch := range matches {
		if apiMatch.ID == matchIDStr {
			return s.updateMatch(apiMatch)
		}
	}

	return nil
}

func (s *SyncService) SyncMatches() error {
	matches, err := s.fetchMatches()
	if err != nil {
		return fmt.Errorf("failed to fetch matches: %w", err)
	}

	for _, apiMatch := range matches {
		if err := s.updateMatch(apiMatch); err != nil {
			log.Printf("Error updating match %s: %v", apiMatch.ID, err)
		}
	}

	knockoutSvc := NewKnockoutService(s.db)
	if err := knockoutSvc.ComputeAdvancement(); err != nil {
		log.Printf("Knockout recalculation error after sync: %v", err)
	}

	return nil
}

func (s *SyncService) fetchMatches() ([]APIMatch, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(s.apiURL + "/get/games")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response APIMatchesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse matches: %w", err)
	}

	return response.Games, nil
}

func (s *SyncService) updateMatch(apiMatch APIMatch) error {
	matchID, err := strconv.ParseInt(apiMatch.ID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid match ID %s: %w", apiMatch.ID, err)
	}

	var currentStatus, stage, matchDate, matchTime string
	var currentHomeScore, currentAwayScore sql.NullInt64
	var currentHomeTeamID, currentAwayTeamID int64
	err = s.db.QueryRow("SELECT status, home_score, away_score, home_team_id, away_team_id, stage, match_date, match_time FROM matches WHERE id = $1", matchID).Scan(&currentStatus, &currentHomeScore, &currentAwayScore, &currentHomeTeamID, &currentAwayTeamID, &stage, &matchDate, &matchTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}

	if currentStatus == "finished" {
		return nil
	}

	status := "upcoming"
	var homeScore, awayScore interface{}
	homeScore = nil
	awayScore = nil

	loc, _ := time.LoadLocation("America/Sao_Paulo")
	now := time.Now().In(loc)

	if apiMatch.Finished == "TRUE" || apiMatch.TimeElapsed == "finished" {
		status = "finished"
		homeScore = parseInt(apiMatch.HomeScore)
		awayScore = parseInt(apiMatch.AwayScore)
	} else if apiMatch.TimeElapsed != "notstarted" && apiMatch.TimeElapsed != "" {
		if matchDate != "" && matchTime != "" {
			matchStart, err := time.ParseInLocation("2006-01-02 15:04", matchDate+" "+matchTime, loc)
			if err == nil && now.Before(matchStart) {
				status = "upcoming"
			} else {
				status = "live"
				homeScore = parseInt(apiMatch.HomeScore)
				awayScore = parseInt(apiMatch.AwayScore)
			}
		} else {
			status = "live"
			homeScore = parseInt(apiMatch.HomeScore)
			awayScore = parseInt(apiMatch.AwayScore)
		}
	}

	apiHomeTeamID := parseInt(apiMatch.HomeTeamID)
	apiAwayTeamID := parseInt(apiMatch.AwayTeamID)

	newHomeTeamID := int64(0)
	newAwayTeamID := int64(0)

	// Fetch team assignments from the API, but override the known API mock bug
	// where Belgium (25) is incorrectly assigned to Match 80's Winner Group L slot.
	if apiHomeTeamID != nil && *apiHomeTeamID > 0 {
		if apiMatch.ID == "80" && *apiHomeTeamID == 25 {
			newHomeTeamID = 0
		} else {
			newHomeTeamID = int64(*apiHomeTeamID)
		}
	}
	if apiAwayTeamID != nil && *apiAwayTeamID > 0 {
		newAwayTeamID = int64(*apiAwayTeamID)
	}

	// For knockout matches, only overwrite home/away team from API if the API actually has a team ID > 0.
	// Otherwise, preserve the locally calculated team ID!
	if stage != "group" {
		if apiHomeTeamID == nil || *apiHomeTeamID <= 0 {
			newHomeTeamID = currentHomeTeamID
		}
		if apiAwayTeamID == nil || *apiAwayTeamID <= 0 {
			newAwayTeamID = currentAwayTeamID
		}
	}

	_, err = s.db.Exec(`
		UPDATE matches SET status = $1, home_team_id = $2, away_team_id = $3 WHERE id = $4
	`, status, newHomeTeamID, newAwayTeamID, matchID)
	if err != nil {
		return err
	}

	if homeScore != nil && awayScore != nil {
		_, err = s.db.Exec(`
			UPDATE matches SET home_score = $1, away_score = $2 WHERE id = $3
		`, homeScore, awayScore, matchID)
		if err != nil {
			return err
		}
	}

	if status == "finished" {
		homeScoreInt := parseInt(apiMatch.HomeScore)
		awayScoreInt := parseInt(apiMatch.AwayScore)

		if homeScoreInt != nil && awayScoreInt != nil {
			if err := s.betSvc.RecalculateMatchBets(matchID); err != nil {
				log.Printf("Error recalculating bets for match %d: %v", matchID, err)
			}
		}
	}

	return nil
}

func parseInt(s string) *int {
	if s == "" || s == "null" {
		return nil
	}
	var val int
	if _, err := fmt.Sscanf(s, "%d", &val); err != nil {
		return nil
	}
	return &val
}

func nullIntToPtr(ni sql.NullInt64) *int {
	if ni.Valid {
		v := int(ni.Int64)
		return &v
	}
	return nil
}

func (s *SyncService) SyncAllData() error {
	if err := s.SyncTeams(); err != nil {
		return err
	}
	if err := s.SyncMatches(); err != nil {
		return err
	}
	return nil
}

func (s *SyncService) SyncTeams() error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(s.apiURL + "/get/teams")
	if err != nil {
		return fmt.Errorf("failed to fetch teams: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var teamsResp APITeamsResponse
	if err := json.Unmarshal(body, &teamsResp); err != nil {
		return fmt.Errorf("failed to parse teams: %w", err)
	}

	for _, t := range teamsResp.Teams {
		_, err := s.db.Exec(`
			INSERT INTO teams (id, name, fifa_code, group_name, flag_url)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT(id) DO UPDATE SET
				name = EXCLUDED.name,
				fifa_code = EXCLUDED.fifa_code,
				group_name = EXCLUDED.group_name,
				flag_url = EXCLUDED.flag_url
		`, t.ID, t.NameEn, t.FifaCode, t.Groups, t.Flag)
		if err != nil {
			return fmt.Errorf("failed to insert team %s: %w", t.NameEn, err)
		}
	}

	log.Printf("Synced %d teams", len(teamsResp.Teams))
	return nil
}
