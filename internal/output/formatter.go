package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/paul/nhl-prop-model/pkg/types"
)

// Formatter handles output formatting for tables and exports
type Formatter struct{}

// NewFormatter creates a new formatter
func NewFormatter() *Formatter {
	return &Formatter{}
}

// PropEdgeBoardTable formats evaluations as a table string
func (f *Formatter) PropEdgeBoardTable(evals []*types.PropEvaluation) string {
	var b strings.Builder

	// Header
	header := fmt.Sprintf("%-5s | %-15s | %-5s | %-12s | %-6s | %-8s | %-8s | %-6s | %-5s | %-10s | %-8s",
		"Game", "Player", "Team", "Market", "Line", "Our Line", "p(Over)", "Edge", "Stars", "Stop", "Flags")
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", len(header)))
	b.WriteString("\n")

	for _, eval := range evals {
		stars := strings.Repeat("★", eval.Stars) + strings.Repeat("☆", 5-eval.Stars)
		stopLine := fmt.Sprintf("%.1f@%d", eval.StopLine, eval.StopPrice)

		flags := ""
		if len(eval.RiskFlags) > 0 {
			flags = strings.Join(eval.RiskFlags[:min(3, len(eval.RiskFlags))], ",")
		}

		row := fmt.Sprintf("%-5d | %-15s | %-5s | %-12s | %-6.1f | %-8.2f | %-8.1f%% | %-6.1f | %-5s | %-10s | %-8s",
			eval.Line.GameID,
			truncate(eval.Line.PlayerName, 15),
			eval.Line.TeamAbbr,
			truncate(string(eval.Line.Market), 12),
			eval.Line.Line,
			eval.OurLine,
			eval.ProbOver*100,
			eval.Edge,
			stars,
			stopLine,
			flags,
		)
		b.WriteString(row)
		b.WriteString("\n")
	}

	return b.String()
}

// ConfidenceRankedTable formats evaluations as a confidence-ranked table
func (f *Formatter) ConfidenceRankedTable(evals []*types.PropEvaluation) string {
	var b strings.Builder

	header := fmt.Sprintf("%-4s | %-4s | %-15s | %-12s | %-6s | %-10s | %-40s | %-10s | %-8s",
		"Rank", "Tier", "Player", "Market", "Side", "Script", "Rationale", "Stop", "Sites")
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", len(header)))
	b.WriteString("\n")

	for i, eval := range evals {
		rationale := ""
		if len(eval.Determinants) > 0 {
			rationale = strings.Join(eval.Determinants, ", ")
			if len(rationale) > 40 {
				rationale = rationale[:37] + "..."
			}
		}

		sites := ""
		if eval.FitsUD {
			sites += "UD "
		}
		if eval.FitsPP {
			sites += "PP "
		}
		if eval.FitsDK {
			sites += "DK "
		}
		if eval.FitsBetr {
			sites += "Betr"
		}

		row := fmt.Sprintf("%-4d | %-4d | %-15s | %-12s | %-6s | %-10s | %-40s | %-10.1f | %-8s",
			i+1,
			eval.Tier,
			truncate(eval.Line.PlayerName, 15),
			truncate(string(eval.Line.Market), 12),
			eval.Line.Side,
			eval.ScenarioTag,
			rationale,
			eval.StopLine,
			sites,
		)
		b.WriteString(row)
		b.WriteString("\n")
	}

	return b.String()
}

// ParlayTable formats parlays as a table string
func (f *Formatter) ParlayTable(parlays []*types.Parlay) string {
	var b strings.Builder

	for i, parlay := range parlays {
		b.WriteString(fmt.Sprintf("=== Parlay %c: %s ===\n", 'A'+i, parlay.Site))
		b.WriteString(fmt.Sprintf("Scenario: %s | Legs: %d | Joint P: %.1f%% | Payout: %.2fx | EV: %.2f\n",
			parlay.ScenarioTag, len(parlay.Legs), parlay.JointProb*100, parlay.Payout, parlay.EV))
		b.WriteString("\n")

		// Legs
		for _, leg := range parlay.Legs {
			b.WriteString(fmt.Sprintf("  • %s | %s | %s %.1f | %.1f%% hit | Stop: %.1f\n",
				leg.PlayerName, leg.Market, leg.Side, leg.Line,
				leg.ProbHit*100, leg.StopLine))
		}

		// Notes
		if len(parlay.CorrelationNotes) > 0 {
			b.WriteString(fmt.Sprintf("\nCorrelation: %s\n", strings.Join(parlay.CorrelationNotes, ", ")))
		}
		if len(parlay.RiskFlags) > 0 {
			b.WriteString(fmt.Sprintf("Warnings: %s\n", strings.Join(parlay.RiskFlags, ", ")))
		}

		b.WriteString("\n")
	}

	return b.String()
}

// PostMortemTable formats picks for post-mortem grading
func (f *Formatter) PostMortemTable(picks []types.PropPick) string {
	var b strings.Builder

	header := fmt.Sprintf("%-15s | %-12s | %-6s | %-6s | %-8s | %-8s | %-6s | %-8s",
		"Player", "Market", "Line", "Side", "Open", "Close", "CLV", "Result")
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", len(header)))
	b.WriteString("\n")

	for _, pick := range picks {
		result := string(pick.Result)
		if result == "" {
			result = "pending"
		}

		row := fmt.Sprintf("%-15s | %-12s | %-6.1f | %-6s | %-8.1f | %-8.1f | %-6.1f | %-8s",
			truncate(pick.PlayerName, 15),
			truncate(string(pick.Market), 12),
			pick.Line,
			pick.Side,
			pick.OpenLine,
			pick.CloseLine,
			pick.CLV,
			result,
		)
		b.WriteString(row)
		b.WriteString("\n")
	}

	return b.String()
}

// PropFamilyStatsTable formats prop family statistics
func (f *Formatter) PropFamilyStatsTable(stats []types.PropFamilyStats) string {
	var b strings.Builder

	header := fmt.Sprintf("%-15s | %-4s | %-8s | %-8s | %-8s | %-8s | %-8s",
		"Market", "Tier", "Total", "Win", "HitRate", "Avg Edge", "Avg CLV")
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", len(header)))
	b.WriteString("\n")

	for _, stat := range stats {
		row := fmt.Sprintf("%-15s | %-4d | %-8d | %-8d | %-8.1f%% | %-8.1f | %-8.1f",
			stat.Market,
			stat.Tier,
			stat.TotalPicks,
			stat.Wins,
			stat.HitRate*100,
			stat.AvgEdge,
			stat.AvgCLV,
		)
		b.WriteString(row)
		b.WriteString("\n")
	}

	return b.String()
}

// ExportEvalsToCSV exports evaluations to a CSV file
func (f *Formatter) ExportEvalsToCSV(evals []*types.PropEvaluation, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Header
	header := []string{
		"Game", "Player", "Team", "Market", "Timeframe", "Line", "Side", "Price",
		"OurLine", "ProbOver", "ProbUnder", "Edge", "EV", "Tier", "Stars",
		"ScenarioTag", "ScenarioFit", "PassesGuardrails", "StopLine", "StopPrice",
		"Determinants", "RiskFlags", "FitsUD", "FitsPP", "FitsDK", "FitsBetr",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Rows
	for _, eval := range evals {
		row := []string{
			fmt.Sprintf("%d", eval.Line.GameID),
			eval.Line.PlayerName,
			eval.Line.TeamAbbr,
			string(eval.Line.Market),
			string(eval.Line.Timeframe),
			fmt.Sprintf("%.1f", eval.Line.Line),
			string(eval.Line.Side),
			fmt.Sprintf("%d", eval.Line.Price),
			fmt.Sprintf("%.2f", eval.OurLine),
			fmt.Sprintf("%.4f", eval.ProbOver),
			fmt.Sprintf("%.4f", eval.ProbUnder),
			fmt.Sprintf("%.2f", eval.Edge),
			fmt.Sprintf("%.4f", eval.EV),
			fmt.Sprintf("%d", eval.Tier),
			fmt.Sprintf("%d", eval.Stars),
			string(eval.ScenarioTag),
			fmt.Sprintf("%.2f", eval.ScenarioFit),
			fmt.Sprintf("%t", eval.PassesGuardrails),
			fmt.Sprintf("%.1f", eval.StopLine),
			fmt.Sprintf("%d", eval.StopPrice),
			strings.Join(eval.Determinants, "; "),
			strings.Join(eval.RiskFlags, "; "),
			fmt.Sprintf("%t", eval.FitsUD),
			fmt.Sprintf("%t", eval.FitsPP),
			fmt.Sprintf("%t", eval.FitsDK),
			fmt.Sprintf("%t", eval.FitsBetr),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// ExportEvalsToJSON exports evaluations to a JSON file
func (f *Formatter) ExportEvalsToJSON(evals []*types.PropEvaluation, path string) error {
	data, err := json.MarshalIndent(evals, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ExportParlaysToJSON exports parlays to a JSON file
func (f *Formatter) ExportParlaysToJSON(parlays []*types.Parlay, path string) error {
	data, err := json.MarshalIndent(parlays, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ExportPicksToCSV exports picks to a CSV file
func (f *Formatter) ExportPicksToCSV(picks []types.PropPick, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"ID", "GameID", "Player", "Team", "Market", "Timeframe", "Side",
		"Line", "OpenLine", "CloseLine", "OpenPrice", "ClosePrice",
		"OurLine", "ProbHit", "Edge", "Tier", "Stars", "ScenarioTag",
		"Result", "ActualValue", "CLV", "ScenarioHit", "CreatedAt", "GradedAt",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, pick := range picks {
		gradedAt := ""
		if pick.GradedAt != nil {
			gradedAt = pick.GradedAt.Format(time.RFC3339)
		}

		row := []string{
			fmt.Sprintf("%d", pick.ID),
			fmt.Sprintf("%d", pick.GameID),
			pick.PlayerName,
			pick.TeamAbbr,
			string(pick.Market),
			string(pick.Timeframe),
			string(pick.Side),
			fmt.Sprintf("%.1f", pick.Line),
			fmt.Sprintf("%.1f", pick.OpenLine),
			fmt.Sprintf("%.1f", pick.CloseLine),
			fmt.Sprintf("%d", pick.OpenPrice),
			fmt.Sprintf("%d", pick.ClosePrice),
			fmt.Sprintf("%.2f", pick.OurLine),
			fmt.Sprintf("%.4f", pick.ProbHit),
			fmt.Sprintf("%.2f", pick.Edge),
			fmt.Sprintf("%d", pick.Tier),
			fmt.Sprintf("%d", pick.Stars),
			string(pick.ScenarioTag),
			string(pick.Result),
			fmt.Sprintf("%.1f", pick.ActualValue),
			fmt.Sprintf("%.2f", pick.CLV),
			fmt.Sprintf("%t", pick.ScenarioHit),
			pick.CreatedAt.Format(time.RFC3339),
			gradedAt,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// Helper functions

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
