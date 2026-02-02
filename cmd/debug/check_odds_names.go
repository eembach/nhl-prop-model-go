// +build ignore

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/paul/nhl-prop-model/internal/api/odds"
)

func main() {
	apiKey := os.Getenv("ODDS_API_KEY")
	if apiKey == "" {
		fmt.Println("ODDS_API_KEY not set")
		return
	}

	client := odds.NewWithKey(apiKey)

	fmt.Println("Fetching events...")
	events, err := client.GetEvents()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Found %d events\n\n", len(events))

	// Get props for first event
	if len(events) == 0 {
		return
	}

	event := events[0]
	fmt.Printf("Checking props for: %s vs %s\n", event.AwayTeam, event.HomeTeam)
	fmt.Printf("Event ID: %s\n\n", event.ID)

	props, err := client.GetPlayerProps(event.ID, []string{"player_shots_on_goal", "player_points"})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Found %d props\n\n", len(props))

	// Show sample of player names
	fmt.Println("Sample SOG props:")
	fmt.Println(strings.Repeat("-", 60))
	sogCount := 0
	for _, p := range props {
		if strings.Contains(p.Market, "shots") && sogCount < 10 {
			fmt.Printf("  %-25s Line: %.1f  Over: %+d  Under: %+d\n",
				p.PlayerName, p.Line, p.OverPrice, p.UnderPrice)
			sogCount++
		}
	}

	fmt.Println("\nSample Points props:")
	fmt.Println(strings.Repeat("-", 60))
	ptsCount := 0
	for _, p := range props {
		if strings.Contains(p.Market, "points") && ptsCount < 10 {
			fmt.Printf("  %-25s Line: %.1f  Over: %+d  Under: %+d\n",
				p.PlayerName, p.Line, p.OverPrice, p.UnderPrice)
			ptsCount++
		}
	}
}
