package moneypuck

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/paul/nhl-prop-model/pkg/types"
)

// MoneyPuck CSV data URLs
const (
	SkaterStatsURL = "https://moneypuck.com/moneypuck/playerData/seasonSummary/2025/regular/skaters.csv"
	GoalieStatsURL = "https://moneypuck.com/moneypuck/playerData/seasonSummary/2025/regular/goalies.csv"
	TeamStatsURL   = "https://moneypuck.com/moneypuck/playerData/seasonSummary/2025/regular/teams.csv"
)

// Client handles MoneyPuck data fetching
type Client struct {
	httpClient *http.Client
	cacheDir   string
}

// New creates a new MoneyPuck client
func New(cacheDir string) *Client {
	return &Client{
		httpClient: &http.Client{},
		cacheDir:   cacheDir,
	}
}

// SkaterAdvanced holds advanced skater stats from MoneyPuck
type SkaterAdvanced struct {
	PlayerID   int
	Name       string
	Team       string
	Position   string
	GamesPlayed int

	// Ice time
	TOI        float64
	TOI5v5     float64
	TOIPP      float64
	TOIPK      float64

	// Goals/Assists
	Goals      int
	Assists    int
	Points     int
	PrimaryAssists int
	SecondaryAssists int

	// Expected goals
	XGoals     float64
	XGoalsFor  float64
	XGoalsAgainst float64
	XGF60      float64
	XGA60      float64

	// On-ice
	OnIceXGF   float64
	OnIceXGA   float64
	OnIceXGF60 float64
	OnIceXGA60 float64

	// Shots
	Shots      int
	ShotsFor   int
	ShotsAgainst int
	CorsiFor   int
	CorsiAgainst int
	CF60       float64
	CA60       float64

	// High danger
	HDCF       int
	HDCA       int
	HDGoalsFor int
	HDGoalsAgainst int

	// Zone starts
	OZStartPct float64
	DZStartPct float64

	// PDO components
	OnIceSavePct float64
	OnIceShPct   float64
	PDO          float64
}

// GoalieAdvanced holds advanced goalie stats from MoneyPuck
type GoalieAdvanced struct {
	PlayerID    int
	Name        string
	Team        string
	GamesPlayed int

	// Saves
	Saves       int
	ShotsAgainst int
	Goals       int
	SavePct     float64

	// Expected
	XGoals      float64
	GSAx        float64 // Goals saved above expected

	// By danger
	HDSaves     int
	HDShotsAgainst int
	HDSavePct   float64
	MDSaves     int
	MDShotsAgainst int
	LDSaves     int
	LDShotsAgainst int
}

// FetchSkaterStats fetches and parses skater stats
func (c *Client) FetchSkaterStats() ([]SkaterAdvanced, error) {
	resp, err := c.httpClient.Get(SkaterStatsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch skater stats: %w", err)
	}
	defer resp.Body.Close()

	return c.parseSkaterCSV(resp.Body)
}

// FetchGoalieStats fetches and parses goalie stats
func (c *Client) FetchGoalieStats() ([]GoalieAdvanced, error) {
	resp, err := c.httpClient.Get(GoalieStatsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch goalie stats: %w", err)
	}
	defer resp.Body.Close()

	return c.parseGoalieCSV(resp.Body)
}

// LoadSkaterStatsFromFile loads stats from a local CSV file
func (c *Client) LoadSkaterStatsFromFile(path string) ([]SkaterAdvanced, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return c.parseSkaterCSV(file)
}

// LoadGoalieStatsFromFile loads stats from a local CSV file
func (c *Client) LoadGoalieStatsFromFile(path string) ([]GoalieAdvanced, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return c.parseGoalieCSV(file)
}

func (c *Client) parseSkaterCSV(r io.Reader) ([]SkaterAdvanced, error) {
	reader := csv.NewReader(r)

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, err
	}

	// Create column index map
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.ToLower(col)] = i
	}

	var skaters []SkaterAdvanced

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		skater := SkaterAdvanced{
			PlayerID:    getInt(record, colMap, "playerid"),
			Name:        getString(record, colMap, "name"),
			Team:        getString(record, colMap, "team"),
			Position:    getString(record, colMap, "position"),
			GamesPlayed: getInt(record, colMap, "games_played"),
			TOI:         getFloat(record, colMap, "icetime"),
			Goals:       getInt(record, colMap, "i_f_goals"),
			Assists:     getInt(record, colMap, "i_f_assists"),
			Points:      getInt(record, colMap, "i_f_points"),
			XGoals:      getFloat(record, colMap, "i_f_xgoals"),
			XGF60:       getFloat(record, colMap, "i_f_xgoals_per_60"),
			Shots:       getInt(record, colMap, "i_f_shots"),
			CorsiFor:    getInt(record, colMap, "i_f_corsi"),
			CF60:        getFloat(record, colMap, "onicecorisfor_per_60"),
			CA60:        getFloat(record, colMap, "onicecorsiagainst_per_60"),
			OZStartPct:  getFloat(record, colMap, "offensivezonestartpct"),
		}

		// Calculate derived metrics
		if skater.GamesPlayed > 0 {
			skater.OnIceXGF60 = getFloat(record, colMap, "onicexgoalsfor_per_60")
			skater.OnIceXGA60 = getFloat(record, colMap, "onicexgoalsagainst_per_60")

			// PDO
			savePct := getFloat(record, colMap, "onicesavepct")
			shPct := getFloat(record, colMap, "oniceshootingpct")
			skater.OnIceSavePct = savePct
			skater.OnIceShPct = shPct
			skater.PDO = (savePct + shPct) * 100
		}

		skaters = append(skaters, skater)
	}

	return skaters, nil
}

func (c *Client) parseGoalieCSV(r io.Reader) ([]GoalieAdvanced, error) {
	reader := csv.NewReader(r)

	header, err := reader.Read()
	if err != nil {
		return nil, err
	}

	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.ToLower(col)] = i
	}

	var goalies []GoalieAdvanced

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		goalie := GoalieAdvanced{
			PlayerID:     getInt(record, colMap, "playerid"),
			Name:         getString(record, colMap, "name"),
			Team:         getString(record, colMap, "team"),
			GamesPlayed:  getInt(record, colMap, "games_played"),
			Saves:        getInt(record, colMap, "saves"),
			ShotsAgainst: getInt(record, colMap, "shotsagainst"),
			Goals:        getInt(record, colMap, "goals"),
			XGoals:       getFloat(record, colMap, "xgoals"),
		}

		if goalie.ShotsAgainst > 0 {
			goalie.SavePct = float64(goalie.Saves) / float64(goalie.ShotsAgainst)
		}

		// GSAx = Expected Goals - Actual Goals
		goalie.GSAx = goalie.XGoals - float64(goalie.Goals)

		// High danger saves if available
		goalie.HDSaves = getInt(record, colMap, "hdsaves")
		goalie.HDShotsAgainst = getInt(record, colMap, "hdshotsagainst")
		if goalie.HDShotsAgainst > 0 {
			goalie.HDSavePct = float64(goalie.HDSaves) / float64(goalie.HDShotsAgainst)
		}

		goalies = append(goalies, goalie)
	}

	return goalies, nil
}

// MergeWithPlayerStats merges MoneyPuck data into player stats
func MergeWithPlayerStats(stats *types.PlayerStats, advanced *SkaterAdvanced) {
	if advanced == nil {
		return
	}

	stats.XGF60 = advanced.XGF60
	stats.XGA60 = advanced.XGA60
	stats.OnIceXGF60 = advanced.OnIceXGF60
	stats.OnIceXGA60 = advanced.OnIceXGA60
	stats.OZStartPct = advanced.OZStartPct
	stats.PDO = advanced.PDO
	stats.HDCF = float64(advanced.HDCF)
}

// MergeWithGoalieStats merges MoneyPuck data into goalie stats
func MergeWithGoalieStats(stats *types.GoalieStats, advanced *GoalieAdvanced) {
	if advanced == nil {
		return
	}

	stats.GSAx = advanced.GSAx
	stats.HDSA = advanced.HDSavePct
}

// Helper functions for CSV parsing
func getString(record []string, colMap map[string]int, col string) string {
	if idx, ok := colMap[col]; ok && idx < len(record) {
		return record[idx]
	}
	return ""
}

func getInt(record []string, colMap map[string]int, col string) int {
	if idx, ok := colMap[col]; ok && idx < len(record) {
		val, _ := strconv.Atoi(record[idx])
		return val
	}
	return 0
}

func getFloat(record []string, colMap map[string]int, col string) float64 {
	if idx, ok := colMap[col]; ok && idx < len(record) {
		val, _ := strconv.ParseFloat(record[idx], 64)
		return val
	}
	return 0
}
