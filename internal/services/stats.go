package services

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
)

type StatsService struct {
	db       *sql.DB
	mu       sync.Mutex
	mockInit bool
	mockRank []userStatsMock
	mockBets []userBetMock
	mockDays []matchDayMock
}

type userStatsMock struct {
	UserID    int64
	Name      string
	GroupID   int64
	GroupName string
}

type userBetMock struct {
	UserID    int64
	MatchID   int64
	HomeScore int
	AwayScore int
	Points    int
	DayIndex  int
}

type matchDayMock struct {
	Label    string
	HomeTeam string
	AwayTeam string
	HomeReal int
	AwayReal int
}

type RankingEvolutionPoint struct {
	Label  string `json:"label"`
	Points int    `json:"points"`
}

type RankingEvolutionSeries struct {
	Name    string                  `json:"name"`
	Data    []RankingEvolutionPoint `json:"data"`
	GroupID int64                   `json:"group_id"`
}

type MatchBetDistribution struct {
	Score   string `json:"score"`
	Count   int    `json:"count"`
	IsExact bool   `json:"is_exact"`
}

type UserAccuracyStats struct {
	TotalBets     int     `json:"total_bets"`
	ExactScore    int     `json:"exact_score"`
	CorrectWinner int     `json:"correct_winner"`
	Wrong         int     `json:"wrong"`
	AccuracyPct   float64 `json:"accuracy_pct"`
	AvgPoints     float64 `json:"avg_points"`
	TotalPoints   int     `json:"total_points"`
}

type PointsPerDay struct {
	Label  string `json:"label"`
	Points int    `json:"points"`
	Bets   int    `json:"bets"`
}

type ScoreHeatmapEntry struct {
	Score string `json:"score"`
	Count int    `json:"count"`
}

type GlobalInsight struct {
	TotalUsers         int    `json:"total_users"`
	TotalBets          int    `json:"total_bets"`
	TotalMatches       int    `json:"total_matches"`
	MostBetMatch       string `json:"most_bet_match"`
	MostPredictedScore string `json:"most_predicted_score"`
	TopExactUser       string `json:"top_exact_user"`
	AvgBetsPerUser     int    `json:"avg_bets_per_user"`
}

type BubbleDataPoint struct {
	Name       string  `json:"name"`
	TotalBets  int     `json:"total_bets"`
	TotalPoints int    `json:"total_points"`
	BubbleSize int     `json:"bubble_size"`
	Accuracy   float64 `json:"accuracy"`
	ExactScore int     `json:"exact_score"`
}

type RadarMetrics struct {
	ExactScore    float64 `json:"exact_score"`
	CorrectWinner float64 `json:"correct_winner"`
	Specials      float64 `json:"specials"`
	AvgPoints     float64 `json:"avg_points"`
	Consistency   float64 `json:"consistency"`
	TotalBets     float64 `json:"total_bets"`
}

type RadarData struct {
	User     RadarMetrics `json:"user"`
	GroupAvg RadarMetrics `json:"group_avg"`
}

func NewStatsService(db *sql.DB) *StatsService {
	return &StatsService{db: db}
}

func (s *StatsService) hasRealBets() bool {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM bets").Scan(&count)
	if err != nil || count == 0 {
		return false
	}
	return true
}

func (s *StatsService) getOrInitMock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mockInit {
		return
	}
	s.mockInit = true
	s.initMockData()
}

func (s *StatsService) initMockData() {
	s.mockDays = []matchDayMock{
		{Label: "14/06", HomeTeam: "Brasil", AwayTeam: "Sérvia", HomeReal: 2, AwayReal: 0},
		{Label: "14/06", HomeTeam: "Argentina", AwayTeam: "México", HomeReal: 3, AwayReal: 1},
		{Label: "15/06", HomeTeam: "Alemanha", AwayTeam: "Japão", HomeReal: 1, AwayReal: 1},
		{Label: "15/06", HomeTeam: "França", AwayTeam: "Inglaterra", HomeReal: 2, AwayReal: 2},
		{Label: "16/06", HomeTeam: "Portugal", AwayTeam: "Espanha", HomeReal: 1, AwayReal: 0},
		{Label: "16/06", HomeTeam: "Holanda", AwayTeam: "Bélgica", HomeReal: 2, AwayReal: 1},
		{Label: "17/06", HomeTeam: "Itália", AwayTeam: "Croácia", HomeReal: 1, AwayReal: 0},
		{Label: "17/06", HomeTeam: "Uruguai", AwayTeam: "Chile", HomeReal: 2, AwayReal: 0},
		{Label: "18/06", HomeTeam: "Inglaterra", AwayTeam: "Brasil", HomeReal: 1, AwayReal: 2},
		{Label: "19/06", HomeTeam: "Espanha", AwayTeam: "Argentina", HomeReal: 2, AwayReal: 3},
	}

	s.mockRank = []userStatsMock{
		{UserID: 101, Name: "João", GroupID: 1, GroupName: "Taberna"},
		{UserID: 102, Name: "Maria", GroupID: 1, GroupName: "Taberna"},
		{UserID: 103, Name: "Pedro", GroupID: 1, GroupName: "Taberna"},
		{UserID: 104, Name: "Ana", GroupID: 1, GroupName: "Taberna"},
		{UserID: 105, Name: "Carlos", GroupID: 1, GroupName: "Taberna"},
		{UserID: 201, Name: "Lucas", GroupID: 2, GroupName: "UERN"},
		{UserID: 202, Name: "Julia", GroupID: 2, GroupName: "UERN"},
		{UserID: 203, Name: "Rafael", GroupID: 2, GroupName: "UERN"},
	}

	pointsOpts := []int{3, 1, 0}
	rng := rand.New(rand.NewSource(42))

	for _, user := range s.mockRank {
		for di, day := range s.mockDays {
			homeBet := rng.Intn(5)
			awayBet := rng.Intn(5)

			p := 0
			if homeBet == day.HomeReal && awayBet == day.AwayReal {
				p = 3
			} else {
				realDiff := day.HomeReal - day.AwayReal
				betDiff := homeBet - awayBet
				if realDiff > 0 && betDiff > 0 || realDiff < 0 && betDiff < 0 || realDiff == 0 && betDiff == 0 {
					p = pointsOpts[rng.Intn(2)+1]
				}
			}

			s.mockBets = append(s.mockBets, userBetMock{
				UserID:    user.UserID,
				MatchID:   int64(di + 1),
				HomeScore: homeBet,
				AwayScore: awayBet,
				Points:    p,
				DayIndex:  di,
			})
		}
	}
}

func (s *StatsService) GetRankingEvolution(groupID int64) []RankingEvolutionSeries {
	if s.hasRealBets() {
		return s.getRankingEvolutionDB(groupID)
	}
	s.getOrInitMock()
	return s.getRankingEvolutionMock(groupID)
}

func (s *StatsService) getRankingEvolutionMock(groupID int64) []RankingEvolutionSeries {
	seriesMap := make(map[int64]*RankingEvolutionSeries)
	days := len(s.mockDays)

	for _, user := range s.mockRank {
		if user.GroupID != groupID {
			continue
		}
		series := &RankingEvolutionSeries{Name: user.Name, GroupID: groupID}
		cumulative := 0
		for di := 0; di < days; di++ {
			dayPoints := 0
			for _, bet := range s.mockBets {
				if bet.UserID == user.UserID && bet.DayIndex == di {
					dayPoints += bet.Points
				}
			}
			cumulative += dayPoints
			series.Data = append(series.Data, RankingEvolutionPoint{
				Label:  s.mockDays[di].Label,
				Points: cumulative,
			})
		}
		seriesMap[user.UserID] = series
	}

	var result []RankingEvolutionSeries
	for _, ser := range seriesMap {
		result = append(result, *ser)
	}
	sort.Slice(result, func(i, j int) bool {
		lastI := result[i].Data[len(result[i].Data)-1].Points
		lastJ := result[j].Data[len(result[j].Data)-1].Points
		return lastI > lastJ
	})
	return result
}

type matchDayInfo struct {
	Label    string
	MatchIDs []int64
	HomeTeam string
	AwayTeam string
}

func (s *StatsService) getRankingEvolutionDB(groupID int64) []RankingEvolutionSeries {
	rows, err := s.db.Query(`
		SELECT DISTINCT m.match_date
		FROM matches m
		JOIN bets b ON b.match_id = m.id
		JOIN users u ON u.id = b.user_id
		WHERE u.group_id = $1 AND m.home_score IS NOT NULL
		ORDER BY m.match_date
	`, groupID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var dates []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			continue
		}
		dates = append(dates, d)
	}
	if len(dates) == 0 {
		return nil
	}

	// For each date, get all users and their cumulative points
	users, err := s.db.Query(`
		SELECT id, name FROM users WHERE group_id = $1 AND COALESCE(is_admin,0) = 0 ORDER BY id
	`, groupID)
	if err != nil {
		return nil
	}
	defer users.Close()

	type userInfo struct {
		ID   int64
		Name string
	}
	var userList []userInfo
	for users.Next() {
		var u userInfo
		if err := users.Scan(&u.ID, &u.Name); err != nil {
			continue
		}
		userList = append(userList, u)
	}

	var result []RankingEvolutionSeries
	for _, u := range userList {
		series := RankingEvolutionSeries{Name: u.Name, GroupID: groupID}
		cumulative := 0
		for _, date := range dates {
			var dayPoints int
			s.db.QueryRow(`
				SELECT COALESCE(SUM(b.points), 0)
				FROM bets b
				JOIN matches m ON m.id = b.match_id
				WHERE b.user_id = $1 AND m.match_date = $2
			`, u.ID, date).Scan(&dayPoints)
			cumulative += dayPoints
			label := date
			if len(label) > 5 {
				label = label[5:]
			}
			series.Data = append(series.Data, RankingEvolutionPoint{
				Label:  label,
				Points: cumulative,
			})
		}
		result = append(result, series)
	}

	sort.Slice(result, func(i, j int) bool {
		di := result[i].Data
		dj := result[j].Data
		if len(di) == 0 || len(dj) == 0 {
			return false
		}
		return di[len(di)-1].Points > dj[len(dj)-1].Points
	})
	return result
}

func (s *StatsService) GetMatchBetDistribution(matchID int64, groupID int64) []MatchBetDistribution {
	if s.hasRealBets() {
		return s.getMatchBetDistributionDB(matchID, groupID)
	}
	s.getOrInitMock()
	return s.getMatchBetDistributionMock(matchID, groupID)
}

func (s *StatsService) getMatchBetDistributionMock(matchID int64, groupID int64) []MatchBetDistribution {
	mid := int(matchID)
	if mid < 1 || mid > len(s.mockDays) {
		return nil
	}

	realHome := s.mockDays[mid-1].HomeReal
	realAway := s.mockDays[mid-1].AwayReal

	userIDs := make(map[int64]bool)
	for _, u := range s.mockRank {
		if u.GroupID == groupID {
			userIDs[u.UserID] = true
		}
	}

	counts := make(map[string]int)
	for _, bet := range s.mockBets {
		if bet.MatchID == matchID && userIDs[bet.UserID] {
			key := fmt.Sprintf("%d-%d", bet.HomeScore, bet.AwayScore)
			counts[key]++
		}
	}

	var result []MatchBetDistribution
	for score, count := range counts {
		var h, a int
		fmt.Sscanf(score, "%d-%d", &h, &a)
		isExact := h == realHome && a == realAway
		result = append(result, MatchBetDistribution{
			Score: score, Count: count, IsExact: isExact,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})
	return result
}

func (s *StatsService) getMatchBetDistributionDB(matchID int64, groupID int64) []MatchBetDistribution {
	var realHome, realAway sql.NullInt64
	s.db.QueryRow("SELECT home_score, away_score FROM matches WHERE id = $1", matchID).Scan(&realHome, &realAway)
	realHomeV := 0
	realAwayV := 0
	if realHome.Valid { realHomeV = int(realHome.Int64) }
	if realAway.Valid { realAwayV = int(realAway.Int64) }

	rows, err := s.db.Query(`
		SELECT b.home_score, b.away_score, COUNT(*) as cnt
		FROM bets b
		JOIN users u ON u.id = b.user_id
		WHERE b.match_id = $1 AND u.group_id = $2
		GROUP BY b.home_score, b.away_score
		ORDER BY cnt DESC
	`, matchID, groupID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []MatchBetDistribution
	for rows.Next() {
		var h, a, cnt int
		if err := rows.Scan(&h, &a, &cnt); err != nil {
			continue
		}
		score := fmt.Sprintf("%d-%d", h, a)
		isExact := h == realHomeV && a == realAwayV
		result = append(result, MatchBetDistribution{
			Score: score, Count: cnt, IsExact: isExact,
		})
	}
	return result
}

func (s *StatsService) GetUserAccuracy(userID int64) UserAccuracyStats {
	if s.hasRealBets() {
		return s.getUserAccuracyDB(userID)
	}
	s.getOrInitMock()
	return s.getUserAccuracyMock(userID)
}

func (s *StatsService) getUserAccuracyMock(userID int64) UserAccuracyStats {
	var stats UserAccuracyStats
	for _, bet := range s.mockBets {
		if bet.UserID != userID {
			continue
		}
		stats.TotalBets++
		switch bet.Points {
		case 3:
			stats.ExactScore++
		case 1:
			stats.CorrectWinner++
		default:
			stats.Wrong++
		}
		stats.TotalPoints += bet.Points
	}
	if stats.TotalBets > 0 {
		stats.AccuracyPct = math.Round(float64(stats.ExactScore+stats.CorrectWinner) / float64(stats.TotalBets) * 100)
		stats.AvgPoints = math.Round(float64(stats.TotalPoints)/float64(stats.TotalBets)*100) / 100
	}
	return stats
}

func (s *StatsService) getUserAccuracyDB(userID int64) UserAccuracyStats {
	var stats UserAccuracyStats

	rows, err := s.db.Query("SELECT points FROM bets WHERE user_id = $1", userID)
	if err != nil {
		return stats
	}
	defer rows.Close()

	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			continue
		}
		stats.TotalBets++
		stats.TotalPoints += p
		switch p {
		case 3:
			stats.ExactScore++
		case 1:
			stats.CorrectWinner++
		default:
			stats.Wrong++
		}
	}

	if stats.TotalBets > 0 {
		stats.AccuracyPct = math.Round(float64(stats.ExactScore+stats.CorrectWinner) / float64(stats.TotalBets) * 100)
		stats.AvgPoints = math.Round(float64(stats.TotalPoints)/float64(stats.TotalBets)*100) / 100
	}
	return stats
}

func (s *StatsService) GetUserPointsPerDay(userID int64) []PointsPerDay {
	if s.hasRealBets() {
		return s.getUserPointsPerDayDB(userID)
	}
	s.getOrInitMock()
	return s.getUserPointsPerDayMock(userID)
}

func (s *StatsService) getUserPointsPerDayMock(userID int64) []PointsPerDay {
	dayMap := make(map[int]*PointsPerDay)
	for _, bet := range s.mockBets {
		if bet.UserID != userID {
			continue
		}
		entry, ok := dayMap[bet.DayIndex]
		if !ok {
			entry = &PointsPerDay{Label: s.mockDays[bet.DayIndex].Label}
		}
		entry.Points += bet.Points
		entry.Bets++
		dayMap[bet.DayIndex] = entry
	}
	var result []PointsPerDay
	for i := 0; i < len(s.mockDays); i++ {
		if entry, ok := dayMap[i]; ok {
			result = append(result, *entry)
		}
	}
	return result
}

func (s *StatsService) getUserPointsPerDayDB(userID int64) []PointsPerDay {
	rows, err := s.db.Query(`
		SELECT m.match_date, COUNT(b.id), COALESCE(SUM(b.points), 0)
		FROM bets b
		JOIN matches m ON m.id = b.match_id
		WHERE b.user_id = $1
		GROUP BY m.match_date
		ORDER BY m.match_date
	`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []PointsPerDay
	for rows.Next() {
		var date string
		var cnt, pts int
		if err := rows.Scan(&date, &cnt, &pts); err != nil {
			continue
		}
		label := date
		if len(label) > 5 {
			label = label[5:]
		}
		result = append(result, PointsPerDay{Label: label, Points: pts, Bets: cnt})
	}
	return result
}

func (s *StatsService) GetScoreHeatmap(groupID int64) []ScoreHeatmapEntry {
	if s.hasRealBets() {
		return s.getScoreHeatmapDB(groupID)
	}
	s.getOrInitMock()
	return s.getScoreHeatmapMock(groupID)
}

func (s *StatsService) getScoreHeatmapMock(groupID int64) []ScoreHeatmapEntry {
	userIDs := make(map[int64]bool)
	for _, u := range s.mockRank {
		if u.GroupID == groupID {
			userIDs[u.UserID] = true
		}
	}
	counts := make(map[string]int)
	for _, bet := range s.mockBets {
		if userIDs[bet.UserID] {
			key := fmt.Sprintf("%d-%d", bet.HomeScore, bet.AwayScore)
			counts[key]++
		}
	}
	var result []ScoreHeatmapEntry
	for score, count := range counts {
		result = append(result, ScoreHeatmapEntry{Score: score, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})
	if len(result) > 15 {
		result = result[:15]
	}
	return result
}

func (s *StatsService) getScoreHeatmapDB(groupID int64) []ScoreHeatmapEntry {
	rows, err := s.db.Query(`
		SELECT b.home_score || '-' || b.away_score as score, COUNT(*) as cnt
		FROM bets b
		JOIN users u ON u.id = b.user_id
		WHERE u.group_id = $1
		GROUP BY b.home_score, b.away_score
		ORDER BY cnt DESC
		LIMIT 15
	`, groupID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []ScoreHeatmapEntry
	for rows.Next() {
		var score string
		var cnt int
		if err := rows.Scan(&score, &cnt); err != nil {
			continue
		}
		result = append(result, ScoreHeatmapEntry{Score: score, Count: cnt})
	}
	return result
}

func (s *StatsService) GetGlobalInsights(groupID int64) GlobalInsight {
	if s.hasRealBets() {
		return s.getGlobalInsightsDB(groupID)
	}
	s.getOrInitMock()
	return s.getGlobalInsightsMock(groupID)
}

func (s *StatsService) getGlobalInsightsMock(groupID int64) GlobalInsight {
	userIDs := make(map[int64]bool)
	var groupUsers []userStatsMock
	for _, u := range s.mockRank {
		if u.GroupID == groupID {
			userIDs[u.UserID] = true
			groupUsers = append(groupUsers, u)
		}
	}

	matchBets := make(map[int64]int)
	scoreCounts := make(map[string]int)
	userExact := make(map[int64]int)
	totalBets := 0

	for _, bet := range s.mockBets {
		if !userIDs[bet.UserID] {
			continue
		}
		totalBets++
		matchBets[bet.MatchID]++
		key := fmt.Sprintf("%d-%d", bet.HomeScore, bet.AwayScore)
		scoreCounts[key]++
		if bet.Points == 3 {
			userExact[bet.UserID]++
		}
	}

	mostBetMatchID := int64(0)
	maxBets := 0
	for mid, count := range matchBets {
		if count > maxBets {
			maxBets = count
			mostBetMatchID = mid
		}
	}
	mostBetMatchLabel := ""
	if int(mostBetMatchID) >= 1 && int(mostBetMatchID) <= len(s.mockDays) {
		d := s.mockDays[mostBetMatchID-1]
		mostBetMatchLabel = d.HomeTeam + " vs " + d.AwayTeam
	}

	mostScore := ""
	maxScore := 0
	for sc, count := range scoreCounts {
		if count > maxScore {
			maxScore = count
			mostScore = sc
		}
	}

	topExactUser := ""
	maxExact := 0
	for uid, count := range userExact {
		if count > maxExact {
			maxExact = count
			for _, u := range groupUsers {
				if u.UserID == uid {
					topExactUser = u.Name
					break
				}
			}
		}
	}

	userCount := len(groupUsers)
	avgBets := 0
	if userCount > 0 {
		avgBets = totalBets / userCount
	}

	return GlobalInsight{
		TotalUsers:         userCount,
		TotalBets:          totalBets,
		TotalMatches:       len(s.mockDays),
		MostBetMatch:       mostBetMatchLabel,
		MostPredictedScore: mostScore,
		TopExactUser:       topExactUser,
		AvgBetsPerUser:     avgBets,
	}
}

func (s *StatsService) getGlobalInsightsDB(groupID int64) GlobalInsight {
	var ins GlobalInsight

	s.db.QueryRow("SELECT COUNT(*) FROM users WHERE group_id = $1 AND COALESCE(is_admin,0) = 0", groupID).Scan(&ins.TotalUsers)
	s.db.QueryRow("SELECT COUNT(*) FROM matches WHERE home_score IS NOT NULL").Scan(&ins.TotalMatches)

	row := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM bets b
		JOIN users u ON u.id = b.user_id
		WHERE u.group_id = $1
	`, groupID)
	row.Scan(&ins.TotalBets)

	if ins.TotalUsers > 0 {
		ins.AvgBetsPerUser = ins.TotalBets / ins.TotalUsers
	}

	s.db.QueryRow(`
		SELECT ht.name || ' vs ' || at.name
		FROM bets b
		JOIN users u ON u.id = b.user_id
		JOIN matches m ON m.id = b.match_id
		LEFT JOIN teams ht ON ht.id = m.home_team_id
		LEFT JOIN teams at ON at.id = m.away_team_id
		WHERE u.group_id = $1
		GROUP BY m.id, ht.name, at.name
		ORDER BY COUNT(*) DESC
		LIMIT 1
	`, groupID).Scan(&ins.MostBetMatch)

	s.db.QueryRow(`
		SELECT b.home_score || '-' || b.away_score
		FROM bets b
		JOIN users u ON u.id = b.user_id
		WHERE u.group_id = $1
		GROUP BY b.home_score, b.away_score
		ORDER BY COUNT(*) DESC
		LIMIT 1
	`, groupID).Scan(&ins.MostPredictedScore)

	s.db.QueryRow(`
		SELECT u.name
		FROM bets b
		JOIN users u ON u.id = b.user_id
		WHERE u.group_id = $1 AND b.points = 3
		GROUP BY u.id, u.name
		ORDER BY COUNT(*) DESC
		LIMIT 1
	`, groupID).Scan(&ins.TopExactUser)

	return ins
}

func (s *StatsService) GetBubbleData(groupID int64) []BubbleDataPoint {
	if s.hasRealBets() {
		return s.getBubbleDataDB(groupID)
	}
	s.getOrInitMock()
	return s.getBubbleDataMock(groupID)
}

func (s *StatsService) getBubbleDataMock(groupID int64) []BubbleDataPoint {
	userIDs := make(map[int64]bool)
	var groupUsers []userStatsMock
	for _, u := range s.mockRank {
		if u.GroupID == groupID {
			userIDs[u.UserID] = true
			groupUsers = append(groupUsers, u)
		}
	}

	userBets := make(map[int64]int)
	userPoints := make(map[int64]int)
	userExact := make(map[int64]int)
	userCorrect := make(map[int64]int)

	for _, bet := range s.mockBets {
		if !userIDs[bet.UserID] {
			continue
		}
		userBets[bet.UserID]++
		userPoints[bet.UserID] += bet.Points
		switch bet.Points {
		case 3:
			userExact[bet.UserID]++
		case 1:
			userCorrect[bet.UserID]++
		}
	}

	var result []BubbleDataPoint
	for _, u := range groupUsers {
		totalBets := userBets[u.UserID]
		totalPoints := userPoints[u.UserID]
		exact := userExact[u.UserID]
		correct := userCorrect[u.UserID]
		accuracy := 0.0
		if totalBets > 0 {
			accuracy = math.Round(float64(exact+correct) / float64(totalBets) * 100)
		}
		bubbleSize := 10 + exact*4
		if bubbleSize < 15 {
			bubbleSize = 15
		}
		result = append(result, BubbleDataPoint{
			Name: u.Name, TotalBets: totalBets, TotalPoints: totalPoints,
			BubbleSize: bubbleSize, Accuracy: accuracy, ExactScore: exact,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalPoints > result[j].TotalPoints
	})
	return result
}

func (s *StatsService) getBubbleDataDB(groupID int64) []BubbleDataPoint {
	rows, err := s.db.Query(`
		SELECT u.id, u.name,
			COUNT(b.id) as total_bets,
			COALESCE(SUM(b.points), 0) as total_points,
			COALESCE(SUM(CASE WHEN b.points = 3 THEN 1 ELSE 0 END), 0) as exact,
			COALESCE(SUM(CASE WHEN b.points = 1 THEN 1 ELSE 0 END), 0) as correct
		FROM users u
		LEFT JOIN bets b ON b.user_id = u.id
		WHERE u.group_id = $1 AND COALESCE(u.is_admin,0) = 0
		GROUP BY u.id, u.name
		ORDER BY total_points DESC
	`, groupID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []BubbleDataPoint
	for rows.Next() {
		var id int64
		var name string
		var totalBets, totalPoints, exact, correct int
		if err := rows.Scan(&id, &name, &totalBets, &totalPoints, &exact, &correct); err != nil {
			continue
		}
		accuracy := 0.0
		if totalBets > 0 {
			accuracy = math.Round(float64(exact+correct) / float64(totalBets) * 100)
		}
		bubbleSize := 10 + exact*4
		if bubbleSize < 15 {
			bubbleSize = 15
		}
		result = append(result, BubbleDataPoint{
			Name: name, TotalBets: totalBets, TotalPoints: totalPoints,
			BubbleSize: bubbleSize, Accuracy: accuracy, ExactScore: exact,
		})
	}
	return result
}

func (s *StatsService) GetRadarData(userID int64, groupID int64) RadarData {
	if s.hasRealBets() {
		return s.getRadarDataDB(userID, groupID)
	}
	s.getOrInitMock()
	return s.getRadarDataMock(userID, groupID)
}

func (s *StatsService) getRadarDataMock(userID int64, groupID int64) RadarData {
	userIDs := make(map[int64]bool)
	var groupUsers []userStatsMock
	for _, u := range s.mockRank {
		if u.GroupID == groupID {
			userIDs[u.UserID] = true
			groupUsers = append(groupUsers, u)
		}
	}

	calcMetrics := func(uid int64) RadarMetrics {
		exact := 0
		correct := 0
		total := 0
		points := 0
		activeDays := 0
		dayPoints := make(map[int]int)

		for _, bet := range s.mockBets {
			if bet.UserID != uid {
				continue
			}
			total++
			points += bet.Points
			dayPoints[bet.DayIndex] += bet.Points
			switch bet.Points {
			case 3:
				exact++
			case 1:
				correct++
			}
		}

		for _, p := range dayPoints {
			if p > 0 {
				activeDays++
			}
		}

		avgPts := 0.0
		if total > 0 {
			avgPts = math.Round(float64(points)/float64(total)*100) / 100
		}
		consistency := math.Round(float64(activeDays) / float64(len(s.mockDays)) * 100)

		return RadarMetrics{
			ExactScore: float64(exact), CorrectWinner: float64(correct),
			Specials: 0, AvgPoints: avgPts, Consistency: consistency, TotalBets: float64(total),
		}
	}

	userMetrics := calcMetrics(userID)

	var avgExact, avgCorrect, avgAvgPts, avgConsistency, avgTotalBets float64
	count := 0
	for _, u := range groupUsers {
		if u.UserID == userID {
			continue
		}
		m := calcMetrics(u.UserID)
		avgExact += m.ExactScore
		avgCorrect += m.CorrectWinner
		avgAvgPts += m.AvgPoints
		avgConsistency += m.Consistency
		avgTotalBets += m.TotalBets
		count++
	}

	if count > 0 {
		avgExact = math.Round(avgExact/float64(count)*100) / 100
		avgCorrect = math.Round(avgCorrect/float64(count)*100) / 100
		avgAvgPts = math.Round(avgAvgPts/float64(count)*100) / 100
		avgConsistency = math.Round(avgConsistency/float64(count)*100) / 100
		avgTotalBets = math.Round(avgTotalBets/float64(count)*100) / 100
	}

	return RadarData{
		User: userMetrics,
		GroupAvg: RadarMetrics{
			ExactScore: avgExact, CorrectWinner: avgCorrect,
			Specials: 0, AvgPoints: avgAvgPts, Consistency: avgConsistency, TotalBets: avgTotalBets,
		},
	}
}

func (s *StatsService) getRadarDataDB(userID int64, groupID int64) RadarData {
	// Count total match days with results for consistency calculation
	var totalMatchDays int
	s.db.QueryRow("SELECT COUNT(DISTINCT match_date) FROM matches WHERE home_score IS NOT NULL").Scan(&totalMatchDays)
	if totalMatchDays == 0 {
		totalMatchDays = 1
	}

	calcMetrics := func(uid int64) RadarMetrics {
		var exact, correct, total, points int
		var activeDays float64

		s.db.QueryRow(`
			SELECT COUNT(*), COALESCE(SUM(points), 0),
				COALESCE(SUM(CASE WHEN points = 3 THEN 1 ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN points = 1 THEN 1 ELSE 0 END), 0)
			FROM bets WHERE user_id = $1
		`, uid).Scan(&total, &points, &exact, &correct)

		s.db.QueryRow(`
			SELECT COUNT(DISTINCT m.match_date)
			FROM bets b
			JOIN matches m ON m.id = b.match_id
			WHERE b.user_id = $1 AND b.points > 0
		`, uid).Scan(&activeDays)

		var specials int
		s.db.QueryRow("SELECT COUNT(*) FROM special_bets WHERE user_id = $1 AND points > 0", uid).Scan(&specials)

		avgPts := 0.0
		if total > 0 {
			avgPts = math.Round(float64(points)/float64(total)*100) / 100
		}
		consistency := math.Round(activeDays / float64(totalMatchDays) * 100)

		return RadarMetrics{
			ExactScore: float64(exact), CorrectWinner: float64(correct),
			Specials: float64(specials), AvgPoints: avgPts,
			Consistency: consistency, TotalBets: float64(total),
		}
	}

	userMetrics := calcMetrics(userID)

	// Group average (excluding current user)
	rows, err := s.db.Query(`
		SELECT id FROM users WHERE group_id = $1 AND COALESCE(is_admin,0) = 0 AND id != $2
	`, groupID, userID)
	if err != nil {
		return RadarData{User: userMetrics}
	}
	defer rows.Close()

	var avgExact, avgCorrect, avgSpecials, avgAvgPts, avgConsistency, avgTotalBets float64
	var count int
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			continue
		}
		m := calcMetrics(uid)
		avgExact += m.ExactScore
		avgCorrect += m.CorrectWinner
		avgSpecials += m.Specials
		avgAvgPts += m.AvgPoints
		avgConsistency += m.Consistency
		avgTotalBets += m.TotalBets
		count++
	}

	if count > 0 {
		avgExact = math.Round(avgExact/float64(count)*100) / 100
		avgCorrect = math.Round(avgCorrect/float64(count)*100) / 100
		avgSpecials = math.Round(avgSpecials/float64(count)*100) / 100
		avgAvgPts = math.Round(avgAvgPts/float64(count)*100) / 100
		avgConsistency = math.Round(avgConsistency/float64(count)*100) / 100
		avgTotalBets = math.Round(avgTotalBets/float64(count)*100) / 100
	}

	return RadarData{
		User: userMetrics,
		GroupAvg: RadarMetrics{
			ExactScore: avgExact, CorrectWinner: avgCorrect,
			Specials: avgSpecials, AvgPoints: avgAvgPts,
			Consistency: avgConsistency, TotalBets: avgTotalBets,
		},
	}
}
