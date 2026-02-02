package odds

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client interfaces with The Odds API for live betting lines
type Client struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// Event represents an NHL game from the odds API
type Event struct {
	ID           string    `json:"id"`
	SportKey     string    `json:"sport_key"`
	SportTitle   string    `json:"sport_title"`
	CommenceTime time.Time `json:"commence_time"`
	HomeTeam     string    `json:"home_team"`
	AwayTeam     string    `json:"away_team"`
}

// PlayerProp represents a player prop bet
type PlayerProp struct {
	PlayerName string
	Market     string  // "player_points", "player_shots_on_goal", etc.
	Line       float64
	OverPrice  int     // American odds
	UnderPrice int     // American odds
	Bookmaker  string
}

// OddsResponse from The Odds API
type OddsResponse struct {
	ID           string    `json:"id"`
	SportKey     string    `json:"sport_key"`
	CommenceTime time.Time `json:"commence_time"`
	HomeTeam     string    `json:"home_team"`
	AwayTeam     string    `json:"away_team"`
	Bookmakers   []struct {
		Key        string `json:"key"`
		Title      string `json:"title"`
		LastUpdate string `json:"last_update"`
		Markets    []struct {
			Key        string `json:"key"`
			LastUpdate string `json:"last_update"`
			Outcomes   []struct {
				Name        string  `json:"name"`
				Description string  `json:"description"`
				Price       int     `json:"price"`
				Point       float64 `json:"point"`
			} `json:"outcomes"`
		} `json:"markets"`
	} `json:"bookmakers"`
}

// New creates a new Odds API client
func New() *Client {
	apiKey := os.Getenv("ODDS_API_KEY")
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: "https://api.the-odds-api.com/v4",
	}
}

// NewWithKey creates a client with a specific API key
func NewWithKey(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: "https://api.the-odds-api.com/v4",
	}
}

// HasAPIKey returns true if an API key is configured
func (c *Client) HasAPIKey() bool {
	return c.apiKey != ""
}

// GetEvents returns today's NHL games
func (c *Client) GetEvents() ([]Event, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("ODDS_API_KEY not set")
	}

	url := fmt.Sprintf("%s/sports/icehockey_nhl/events?apiKey=%s", c.baseURL, c.apiKey)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error: %d", resp.StatusCode)
	}

	var events []Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, err
	}

	return events, nil
}

// GetPlayerProps returns player props for a specific event
func (c *Client) GetPlayerProps(eventID string, markets []string) ([]PlayerProp, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("ODDS_API_KEY not set")
	}

	marketStr := strings.Join(markets, ",")
	url := fmt.Sprintf("%s/sports/icehockey_nhl/events/%s/odds?apiKey=%s&regions=us&markets=%s&oddsFormat=american",
		c.baseURL, eventID, c.apiKey, marketStr)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error: %d", resp.StatusCode)
	}

	var oddsResp OddsResponse
	if err := json.NewDecoder(resp.Body).Decode(&oddsResp); err != nil {
		return nil, err
	}

	var props []PlayerProp

	for _, book := range oddsResp.Bookmakers {
		for _, market := range book.Markets {
			// Group outcomes by player
			playerOutcomes := make(map[string][]struct {
				Name  string
				Price int
				Point float64
			})

			for _, outcome := range market.Outcomes {
				// Parse player name from description or name
				playerName := outcome.Description
				if playerName == "" {
					playerName = outcome.Name
				}

				// Clean up "Over" / "Under" from name
				playerName = strings.TrimSuffix(playerName, " Over")
				playerName = strings.TrimSuffix(playerName, " Under")

				playerOutcomes[playerName] = append(playerOutcomes[playerName], struct {
					Name  string
					Price int
					Point float64
				}{
					Name:  outcome.Name,
					Price: outcome.Price,
					Point: outcome.Point,
				})
			}

			// Build props from grouped outcomes
			for playerName, outcomes := range playerOutcomes {
				if len(outcomes) >= 2 {
					prop := PlayerProp{
						PlayerName: playerName,
						Market:     market.Key,
						Bookmaker:  book.Title,
					}

					for _, o := range outcomes {
						if strings.Contains(strings.ToLower(o.Name), "over") {
							prop.OverPrice = o.Price
							prop.Line = o.Point
						} else if strings.Contains(strings.ToLower(o.Name), "under") {
							prop.UnderPrice = o.Price
						}
					}

					if prop.Line > 0 {
						props = append(props, prop)
					}
				}
			}
		}
	}

	return props, nil
}

// GetAllPlayerProps returns all available player props for today's games
func (c *Client) GetAllPlayerProps() (map[string][]PlayerProp, error) {
	events, err := c.GetEvents()
	if err != nil {
		return nil, err
	}

	result := make(map[string][]PlayerProp)
	markets := []string{"player_points", "player_shots_on_goal", "player_assists", "player_goals"}

	for _, event := range events {
		// Only get props for games starting in the next 24 hours
		if time.Until(event.CommenceTime) > 24*time.Hour {
			continue
		}

		props, err := c.GetPlayerProps(event.ID, markets)
		if err != nil {
			continue
		}

		gameKey := fmt.Sprintf("%s@%s", event.AwayTeam, event.HomeTeam)
		result[gameKey] = props

		// Rate limiting
		time.Sleep(500 * time.Millisecond)
	}

	return result, nil
}

// FindPlayerLine searches for a specific player's line
func (c *Client) FindPlayerLine(playerName string, market string) (*PlayerProp, error) {
	allProps, err := c.GetAllPlayerProps()
	if err != nil {
		return nil, err
	}

	// Normalize search name
	searchName := strings.ToLower(playerName)

	for _, props := range allProps {
		for _, prop := range props {
			if strings.Contains(strings.ToLower(prop.PlayerName), searchName) &&
				strings.Contains(prop.Market, market) {
				return &prop, nil
			}
		}
	}

	return nil, fmt.Errorf("no line found for %s %s", playerName, market)
}

// VerifyLineExists checks if a player has a line posted for a given market
func (c *Client) VerifyLineExists(playerName string, market string, allProps map[string][]PlayerProp) bool {
	searchName := strings.ToLower(playerName)

	for _, props := range allProps {
		for _, prop := range props {
			propName := strings.ToLower(prop.PlayerName)
			if strings.Contains(propName, searchName) || strings.Contains(searchName, propName) {
				if strings.Contains(prop.Market, market) {
					return true
				}
			}
		}
	}

	return false
}

// MarketToAPIKey converts our market names to The Odds API market keys
func MarketToAPIKey(market string) string {
	switch {
	case strings.Contains(market, "SOG"):
		return "player_shots_on_goal"
	case strings.Contains(market, "Points"):
		return "player_points"
	case strings.Contains(market, "Goals"):
		return "player_goals"
	case strings.Contains(market, "Assists"):
		return "player_assists"
	default:
		return ""
	}
}
