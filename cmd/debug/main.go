package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/paul/nhl-prop-model/internal/api/nhl"
)

func main() {
	client := nhl.New()

	// Get a recent game
	games, err := client.GetSchedule(time.Now().AddDate(0, 0, -1))
	if err != nil {
		fmt.Printf("Error fetching schedule: %v\n", err)
		return
	}

	if len(games) == 0 {
		fmt.Println("No games found")
		return
	}

	// Find a completed game
	var gameID int
	for _, g := range games {
		if g.State == "final" {
			gameID = g.ID
			fmt.Printf("Using game: %s @ %s (ID: %d)\n", g.AwayTeamAbbr, g.HomeTeamAbbr, g.ID)
			break
		}
	}

	if gameID == 0 {
		fmt.Println("No completed games found")
		return
	}

	// Fetch raw boxscore
	raw, err := client.GetRaw(fmt.Sprintf("/gamecenter/%d/boxscore", gameID))
	if err != nil {
		fmt.Printf("Error fetching raw boxscore: %v\n", err)
		return
	}

	// Pretty print a portion
	var data map[string]interface{}
	json.Unmarshal(raw, &data)

	// Check playerByGameStats structure
	if pbgs, ok := data["playerByGameStats"].(map[string]interface{}); ok {
		if homeTeam, ok := pbgs["homeTeam"].(map[string]interface{}); ok {
			if forwards, ok := homeTeam["forwards"].([]interface{}); ok && len(forwards) > 0 {
				fmt.Println("\nSample forward data:")
				if f, ok := forwards[0].(map[string]interface{}); ok {
					pretty, _ := json.MarshalIndent(f, "", "  ")
					fmt.Println(string(pretty))
				}
			}
		}
	}

	// Also fetch the actual boxscore through our client
	fmt.Println("\n--- Parsed Boxscore ---")
	box, err := client.GetBoxscore(gameID)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Home Score: %d, Away Score: %d\n", box.HomeScore, box.AwayScore)
	fmt.Printf("Players with stats: %d\n", len(box.PlayerStats))

	// Show sample player
	for id, ps := range box.PlayerStats {
		fmt.Printf("\nPlayer %d: TOI=%.1f, Goals=%d, Assists=%d, Points=%d, SOG=%d, Hits=%d, Blocks=%d\n",
			id, ps.TOI, ps.Goals, ps.Assists, ps.Points, ps.SOG, ps.Hits, ps.Blocks)
		break
	}
}
