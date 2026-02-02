package odds

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/paul/nhl-prop-model/pkg/types"
)

// ManualInput handles manual odds input from files or CLI
type ManualInput struct {
	lines []types.PropLine
}

// NewManualInput creates a new manual input handler
func NewManualInput() *ManualInput {
	return &ManualInput{}
}

// LoadFromJSON loads prop lines from a JSON file
func (m *ManualInput) LoadFromJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var lines []types.PropLine
	if err := json.Unmarshal(data, &lines); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Set fetch time for all lines
	now := time.Now()
	for i := range lines {
		lines[i].FetchedAt = now
		lines[i].UpdatedAt = now
		lines[i].ImpliedProb = calculateImpliedProb(lines[i].Price)
	}

	m.lines = append(m.lines, lines...)
	return nil
}

// AddLine manually adds a prop line
func (m *ManualInput) AddLine(line types.PropLine) {
	line.FetchedAt = time.Now()
	line.UpdatedAt = time.Now()
	line.ImpliedProb = calculateImpliedProb(line.Price)
	m.lines = append(m.lines, line)
}

// GetLines returns all loaded lines
func (m *ManualInput) GetLines() []types.PropLine {
	return m.lines
}

// GetLinesByGame returns lines for a specific game
func (m *ManualInput) GetLinesByGame(gameID int) []types.PropLine {
	var result []types.PropLine
	for _, line := range m.lines {
		if line.GameID == gameID {
			result = append(result, line)
		}
	}
	return result
}

// GetLinesByPlayer returns lines for a specific player
func (m *ManualInput) GetLinesByPlayer(playerID int) []types.PropLine {
	var result []types.PropLine
	for _, line := range m.lines {
		if line.PlayerID == playerID {
			result = append(result, line)
		}
	}
	return result
}

// Clear removes all loaded lines
func (m *ManualInput) Clear() {
	m.lines = nil
}

// calculateImpliedProb converts American odds to implied probability
func calculateImpliedProb(price int) float64 {
	if price > 0 {
		return 100.0 / (float64(price) + 100.0)
	} else if price < 0 {
		return float64(-price) / (float64(-price) + 100.0)
	}
	return 0.5
}

// LineVerifier verifies lines against multiple sources
type LineVerifier struct {
	sources map[string][]types.PropLine
}

// NewLineVerifier creates a new line verifier
func NewLineVerifier() *LineVerifier {
	return &LineVerifier{
		sources: make(map[string][]types.PropLine),
	}
}

// AddSource adds lines from a source
func (v *LineVerifier) AddSource(name string, lines []types.PropLine) {
	v.sources[name] = lines
}

// VerifyLine checks if a line exists in at least 2 sources
func (v *LineVerifier) VerifyLine(line types.PropLine) (bool, []string) {
	var matches []string

	for source, lines := range v.sources {
		for _, l := range lines {
			if l.PlayerID == line.PlayerID &&
				l.Market == line.Market &&
				l.Line == line.Line {
				matches = append(matches, source)
				break
			}
		}
	}

	return len(matches) >= 2, matches
}

// FindDiscrepancies finds lines with significant price differences
func (v *LineVerifier) FindDiscrepancies(threshold float64) []LineDiscrepancy {
	var discrepancies []LineDiscrepancy

	// Group lines by player+market+line
	type key struct {
		playerID int
		market   types.PropMarket
		line     float64
	}
	grouped := make(map[key]map[string]types.PropLine)

	for source, lines := range v.sources {
		for _, line := range lines {
			k := key{line.PlayerID, line.Market, line.Line}
			if grouped[k] == nil {
				grouped[k] = make(map[string]types.PropLine)
			}
			grouped[k][source] = line
		}
	}

	// Check for discrepancies
	for k, sourceLines := range grouped {
		if len(sourceLines) < 2 {
			continue
		}

		var prices []int
		var sources []string
		for source, line := range sourceLines {
			prices = append(prices, line.Price)
			sources = append(sources, source)
		}

		// Calculate price range in implied prob
		minProb := calculateImpliedProb(prices[0])
		maxProb := minProb
		for _, price := range prices[1:] {
			prob := calculateImpliedProb(price)
			if prob < minProb {
				minProb = prob
			}
			if prob > maxProb {
				maxProb = prob
			}
		}

		if maxProb-minProb > threshold {
			discrepancies = append(discrepancies, LineDiscrepancy{
				PlayerID: k.playerID,
				Market:   k.market,
				Line:     k.line,
				Sources:  sources,
				Prices:   prices,
				Range:    maxProb - minProb,
			})
		}
	}

	return discrepancies
}

// LineDiscrepancy represents a significant difference between sources
type LineDiscrepancy struct {
	PlayerID int
	Market   types.PropMarket
	Line     float64
	Sources  []string
	Prices   []int
	Range    float64 // Difference in implied probability
}

// PropLineTemplate helps create prop lines quickly
type PropLineTemplate struct {
	GameID    int
	TeamAbbr  string
	Timeframe types.Timeframe
	Source    string
}

// CreateLine creates a prop line from the template
func (t *PropLineTemplate) CreateLine(playerID int, playerName string, market types.PropMarket, line float64, side types.PropSide, price int) types.PropLine {
	return types.PropLine{
		GameID:      t.GameID,
		PlayerID:    playerID,
		PlayerName:  playerName,
		TeamAbbr:    t.TeamAbbr,
		Market:      market,
		Timeframe:   t.Timeframe,
		Line:        line,
		Side:        side,
		Price:       price,
		ImpliedProb: calculateImpliedProb(price),
		Source:      t.Source,
		FetchedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}
