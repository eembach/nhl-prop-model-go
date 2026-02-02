package nhl

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/paul/nhl-prop-model/pkg/types"
)

const (
	baseURL     = "https://api-web.nhle.com/v1"
	statsAPIURL = "https://api.nhle.com/stats/rest/en"
)

// Client is an NHL API client
type Client struct {
	http *resty.Client
}

// New creates a new NHL API client
func New() *Client {
	client := resty.New().
		SetBaseURL(baseURL).
		SetTimeout(30 * time.Second).
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second)

	return &Client{http: client}
}

// GetSchedule fetches the schedule for a date range
func (c *Client) GetSchedule(date time.Time) ([]types.Game, error) {
	dateStr := date.Format("2006-01-02")

	var resp scheduleResponse
	_, err := c.http.R().
		SetResult(&resp).
		Get(fmt.Sprintf("/schedule/%s", dateStr))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch schedule: %w", err)
	}

	var games []types.Game
	for _, day := range resp.GameWeek {
		// Only include games from the requested date
		if day.Date != dateStr {
			continue
		}
		for _, g := range day.Games {
			game := types.Game{
				ID:           g.ID,
				Season:       fmt.Sprintf("%d", g.Season),
				GameDate:     parseDate(g.GameDate),
				StartTime:    parseTime(g.StartTimeUTC),
				HomeTeamID:   g.HomeTeam.ID,
				HomeTeamAbbr: g.HomeTeam.Abbrev,
				AwayTeamID:   g.AwayTeam.ID,
				AwayTeamAbbr: g.AwayTeam.Abbrev,
				Venue:        g.Venue.Default,
				State:        mapGameState(g.GameState),
				HomeScore:    g.HomeTeam.Score,
				AwayScore:    g.AwayTeam.Score,
			}
			games = append(games, game)
		}
	}

	return games, nil
}

// GetTodaySchedule fetches today's schedule
func (c *Client) GetTodaySchedule() ([]types.Game, error) {
	return c.GetSchedule(time.Now())
}

// GetBoxscore fetches a game's boxscore
func (c *Client) GetBoxscore(gameID int) (*types.Boxscore, error) {
	var resp boxscoreResponse
	_, err := c.http.R().
		SetResult(&resp).
		Get(fmt.Sprintf("/gamecenter/%d/boxscore", gameID))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch boxscore: %w", err)
	}

	box := &types.Boxscore{
		GameID:      gameID,
		FetchedAt:   time.Now(),
		HomeScore:   resp.HomeTeam.Score,
		AwayScore:   resp.AwayTeam.Score,
		PlayerStats: make(map[int]types.BoxscorePlayerStats),
		GoalieStats: make(map[int]types.BoxscoreGoalieStats),
	}

	// Parse home team players
	parseTeamPlayers(resp.PlayerByGameStats.HomeTeam, resp.HomeTeam.ID, box)
	// Parse away team players
	parseTeamPlayers(resp.PlayerByGameStats.AwayTeam, resp.AwayTeam.ID, box)

	return box, nil
}

// GetRoster fetches a team's roster
func (c *Client) GetRoster(teamAbbr string) ([]types.Player, error) {
	var resp rosterResponse
	_, err := c.http.R().
		SetResult(&resp).
		Get(fmt.Sprintf("/roster/%s/current", teamAbbr))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch roster: %w", err)
	}

	var players []types.Player

	for _, p := range resp.Forwards {
		players = append(players, convertPlayer(p, teamAbbr))
	}
	for _, p := range resp.Defensemen {
		players = append(players, convertPlayer(p, teamAbbr))
	}
	for _, p := range resp.Goalies {
		players = append(players, convertPlayer(p, teamAbbr))
	}

	return players, nil
}

// GetPlayerStats fetches season stats for a player
func (c *Client) GetPlayerStats(playerID int, season string) (*types.PlayerStats, error) {
	var resp playerStatsResponse
	_, err := c.http.R().
		SetResult(&resp).
		Get(fmt.Sprintf("/player/%d/landing", playerID))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch player stats: %w", err)
	}

	// Find the current season stats
	for _, s := range resp.SeasonTotals {
		if s.Season == parseInt(season) && s.LeagueAbbrev == "NHL" && s.GameTypeID == 2 {
			stats := &types.PlayerStats{
				PlayerID:      playerID,
				GamesPlayed:   s.GamesPlayed,
				Goals:         s.Goals,
				Assists:       s.Assists,
				Points:        s.Points,
				PPGoals:       s.PowerPlayGoals,
				PPPoints:      s.PowerPlayPoints,
				ShootingPct:   s.ShootingPctg * 100, // API returns decimal, convert to percentage
				SOGTotal:      s.Shots,
			}

			if s.GamesPlayed > 0 {
				stats.PointsPerGame = float64(s.Points) / float64(s.GamesPlayed)
				stats.SOG = float64(s.Shots) / float64(s.GamesPlayed)
			}

			// Parse TOI
			stats.TOI = parseTOI(s.AvgTOI)

			return stats, nil
		}
	}

	return nil, nil
}

// GetPlayerGameLog fetches recent game log for a player
func (c *Client) GetPlayerGameLog(playerID int, season string, limit int) ([]PlayerGameLog, error) {
	var resp playerGameLogResponse
	_, err := c.http.R().
		SetResult(&resp).
		Get(fmt.Sprintf("/player/%d/game-log/%s/2", playerID, season))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch game log: %w", err)
	}

	games := resp.GameLog
	if limit > 0 && len(games) > limit {
		games = games[:limit]
	}

	var logs []PlayerGameLog
	for _, g := range games {
		logs = append(logs, PlayerGameLog{
			GameID:        g.GameID,
			GameDate:      parseDate(g.GameDate),
			OpponentAbbr:  g.OpponentAbbrev,
			Goals:         g.Goals,
			Assists:       g.Assists,
			Points:        g.Points,
			Shots:         g.Shots,
			Hits:          g.Hits,
			Blocks:        g.BlockedShots,
			PIM:           g.PIM,
			TOI:           parseTOI(g.TOI),
			PPGoals:       g.PowerPlayGoals,
			PPPoints:      g.PowerPlayPoints,
		})
	}

	return logs, nil
}

// GetStandings fetches current standings
func (c *Client) GetStandings() ([]types.TeamStats, error) {
	var resp standingsResponse
	_, err := c.http.R().
		SetResult(&resp).
		Get("/standings/now")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch standings: %w", err)
	}

	var stats []types.TeamStats
	for _, s := range resp.Standings {
		ts := types.TeamStats{
			TeamAbbr:     s.TeamAbbrev.Default,
			Wins:         s.Wins,
			Losses:       s.Losses,
			OTLosses:     s.OTLosses,
			Points:       s.Points,
			GF:           s.GoalFor,
			GA:           s.GoalAgainst,
			GFPerGame:    float64(s.GoalFor) / float64(s.GamesPlayed),
			GAPerGame:    float64(s.GoalAgainst) / float64(s.GamesPlayed),
		}
		stats = append(stats, ts)
	}

	return stats, nil
}

// GetTeams fetches all NHL teams
func (c *Client) GetTeams() ([]types.Team, error) {
	// Use standings to get current teams
	var resp standingsResponse
	_, err := c.http.R().
		SetResult(&resp).
		Get("/standings/now")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch teams: %w", err)
	}

	var teams []types.Team
	for _, s := range resp.Standings {
		team := types.Team{
			Name:         s.TeamName.Default,
			Abbreviation: s.TeamAbbrev.Default,
			Division:     s.DivisionName,
			Conference:   s.ConferenceName,
		}
		teams = append(teams, team)
	}

	return teams, nil
}

// PlayerGameLog represents a single game in a player's game log
type PlayerGameLog struct {
	GameID       int
	GameDate     time.Time
	OpponentAbbr string
	Goals        int
	Assists      int
	Points       int
	Shots        int
	Hits         int
	Blocks       int
	PIM          int
	TOI          float64
	PPGoals      int
	PPPoints     int
}

// API response types

type scheduleResponse struct {
	GameWeek []struct {
		Date  string `json:"date"`
		Games []struct {
			ID           int    `json:"id"`
			Season       int    `json:"season"`
			GameDate     string `json:"gameDate"`
			StartTimeUTC string `json:"startTimeUTC"`
			GameState    string `json:"gameState"`
			Venue        struct {
				Default string `json:"default"`
			} `json:"venue"`
			HomeTeam struct {
				ID     int    `json:"id"`
				Abbrev string `json:"abbrev"`
				Score  int    `json:"score"`
			} `json:"homeTeam"`
			AwayTeam struct {
				ID     int    `json:"id"`
				Abbrev string `json:"abbrev"`
				Score  int    `json:"score"`
			} `json:"awayTeam"`
		} `json:"games"`
	} `json:"gameWeek"`
}

type boxscoreResponse struct {
	HomeTeam struct {
		ID    int `json:"id"`
		Score int `json:"score"`
	} `json:"homeTeam"`
	AwayTeam struct {
		ID    int `json:"id"`
		Score int `json:"score"`
	} `json:"awayTeam"`
	PlayerByGameStats struct {
		HomeTeam teamPlayerStats `json:"homeTeam"`
		AwayTeam teamPlayerStats `json:"awayTeam"`
	} `json:"playerByGameStats"`
}

type teamPlayerStats struct {
	Forwards   []boxscorePlayer `json:"forwards"`
	Defense    []boxscorePlayer `json:"defense"`
	Goalies    []boxscoreGoalie `json:"goalies"`
}

type boxscorePlayer struct {
	PlayerID    int    `json:"playerId"`
	Name        struct {
		Default string `json:"default"`
	} `json:"name"`
	Goals       int    `json:"goals"`
	Assists     int    `json:"assists"`
	Points      int    `json:"points"`
	SOG         int    `json:"sog"`
	Hits        int    `json:"hits"`
	BlockedShots int   `json:"blockedShots"`
	PIM         int    `json:"pim"`
	TOI         string `json:"toi"`
	PowerPlayGoals int `json:"powerPlayGoals"`
	PlusMinus   int    `json:"plusMinus"`
}

type boxscoreGoalie struct {
	PlayerID     int    `json:"playerId"`
	Name         struct {
		Default string `json:"default"`
	} `json:"name"`
	TOI          string  `json:"toi"`
	Saves        int     `json:"saves"`
	ShotsAgainst int     `json:"shotsAgainst"`
	GoalsAgainst int     `json:"goalsAgainst"`
	SavePctg     float64 `json:"savePctg"`
	Decision     string  `json:"decision"`
}

type rosterResponse struct {
	Forwards   []rosterPlayer `json:"forwards"`
	Defensemen []rosterPlayer `json:"defensemen"`
	Goalies    []rosterPlayer `json:"goalies"`
}

type rosterPlayer struct {
	ID        int `json:"id"`
	FirstName struct {
		Default string `json:"default"`
	} `json:"firstName"`
	LastName struct {
		Default string `json:"default"`
	} `json:"lastName"`
	SweaterNumber int    `json:"sweaterNumber"`
	PositionCode  string `json:"positionCode"`
}

type playerStatsResponse struct {
	SeasonTotals []struct {
		Season        int     `json:"season"`
		GameTypeID    int     `json:"gameTypeId"`
		LeagueAbbrev  string  `json:"leagueAbbrev"`
		GamesPlayed   int     `json:"gamesPlayed"`
		Goals         int     `json:"goals"`
		Assists       int     `json:"assists"`
		Points        int     `json:"points"`
		Shots         int     `json:"shots"`
		PowerPlayGoals int    `json:"powerPlayGoals"`
		PowerPlayPoints int   `json:"powerPlayPoints"`
		ShootingPctg  float64 `json:"shootingPctg"`
		AvgTOI        string  `json:"avgToi"`
	} `json:"seasonTotals"`
}

type playerGameLogResponse struct {
	GameLog []struct {
		GameID         int    `json:"gameId"`
		GameDate       string `json:"gameDate"`
		OpponentAbbrev string `json:"opponentAbbrev"`
		Goals          int    `json:"goals"`
		Assists        int    `json:"assists"`
		Points         int    `json:"points"`
		Shots          int    `json:"shots"`
		Hits           int    `json:"hits"`
		BlockedShots   int    `json:"blockedShots"`
		PIM            int    `json:"pim"`
		TOI            string `json:"toi"`
		PowerPlayGoals int    `json:"powerPlayGoals"`
		PowerPlayPoints int   `json:"powerPlayPoints"`
	} `json:"gameLog"`
}

type standingsResponse struct {
	Standings []struct {
		TeamAbbrev struct {
			Default string `json:"default"`
		} `json:"teamAbbrev"`
		TeamName struct {
			Default string `json:"default"`
		} `json:"teamName"`
		ConferenceName string `json:"conferenceName"`
		DivisionName   string `json:"divisionName"`
		GamesPlayed    int    `json:"gamesPlayed"`
		Wins           int    `json:"wins"`
		Losses         int    `json:"losses"`
		OTLosses       int    `json:"otLosses"`
		Points         int    `json:"points"`
		GoalFor        int    `json:"goalFor"`
		GoalAgainst    int    `json:"goalAgainst"`
	} `json:"standings"`
}

// Helper functions

func parseDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func parseTOI(s string) float64 {
	// Format: "MM:SS" or "HH:MM:SS"
	parts := strings.Split(s, ":")
	if len(parts) == 2 {
		mins, _ := strconv.Atoi(parts[0])
		secs, _ := strconv.Atoi(parts[1])
		return float64(mins) + float64(secs)/60.0
	}
	if len(parts) == 3 {
		hours, _ := strconv.Atoi(parts[0])
		mins, _ := strconv.Atoi(parts[1])
		secs, _ := strconv.Atoi(parts[2])
		return float64(hours*60+mins) + float64(secs)/60.0
	}
	return 0
}

func mapGameState(state string) types.GameState {
	switch state {
	case "LIVE", "CRIT":
		return types.GameStateLive
	case "FINAL", "OFF":
		return types.GameStateFinal
	case "PPD":
		return types.GameStatePostponed
	default:
		return types.GameStateScheduled
	}
}

func convertPlayer(p rosterPlayer, teamAbbr string) types.Player {
	return types.Player{
		ID:        p.ID,
		Name:      fmt.Sprintf("%s %s", p.FirstName.Default, p.LastName.Default),
		FirstName: p.FirstName.Default,
		LastName:  p.LastName.Default,
		TeamAbbr:  teamAbbr,
		Position:  types.Position(p.PositionCode),
		Number:    p.SweaterNumber,
		Active:    true,
	}
}

func parseTeamPlayers(team teamPlayerStats, teamID int, box *types.Boxscore) {
	// Parse forwards and defense
	for _, p := range append(team.Forwards, team.Defense...) {
		box.PlayerStats[p.PlayerID] = types.BoxscorePlayerStats{
			PlayerID:   p.PlayerID,
			TeamID:     teamID,
			TOI:        parseTOI(p.TOI),
			Goals:      p.Goals,
			Assists:    p.Assists,
			Points:     p.Points,
			SOG:        p.SOG,
			Hits:       p.Hits,
			Blocks:     p.BlockedShots,
			PIM:        p.PIM,
			PPGoals:    p.PowerPlayGoals,
			PlusMinus:  p.PlusMinus,
		}
	}

	// Parse goalies
	for _, g := range team.Goalies {
		box.GoalieStats[g.PlayerID] = types.BoxscoreGoalieStats{
			PlayerID:     g.PlayerID,
			TeamID:       teamID,
			TOI:          parseTOI(g.TOI),
			Saves:        g.Saves,
			ShotsAgainst: g.ShotsAgainst,
			GoalsAgainst: g.GoalsAgainst,
			SavePct:      g.SavePctg,
			Decision:     g.Decision,
		}
	}
}

// CalculateRecentStats calculates recent stats from game logs
func CalculateRecentStats(logs []PlayerGameLog) types.PlayerStats {
	if len(logs) == 0 {
		return types.PlayerStats{}
	}

	var stats types.PlayerStats
	stats.RecentGamesPlayed = len(logs)

	var totalSOG, totalTOI float64
	for _, g := range logs {
		stats.RecentGoals += g.Goals
		stats.RecentAssists += g.Assists
		stats.RecentPoints += g.Points
		totalSOG += float64(g.Shots)
		totalTOI += g.TOI
	}

	n := float64(len(logs))
	stats.RecentSOG = totalSOG / n
	stats.RecentTOI = totalTOI / n
	stats.RecentPPG = float64(stats.RecentPoints) / n

	return stats
}

// JSON helper for debugging
func (c *Client) GetRaw(path string) (json.RawMessage, error) {
	var result json.RawMessage
	_, err := c.http.R().
		SetResult(&result).
		Get(path)
	return result, err
}
