package scenario

import (
	"testing"

	"github.com/paul/nhl-prop-model/pkg/types"
)

func TestScenarioEngine(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name             string
		homeStats        *types.TeamStats
		awayStats        *types.TeamStats
		homeGoalie       *types.GoalieStats
		awayGoalie       *types.GoalieStats
		ctx              *types.GameContext
		expectedScenario types.ScenarioType
	}{
		{
			name: "Run-and-Gun - high pace, weak goalies",
			homeStats: &types.TeamStats{
				CF60:       65.0,
				CA60:       60.0,
				GFPerGame:  3.5,
				GAPerGame:  3.3,
				PPPct:      22.0,
				PKPct:      78.0,
				PIMPerGame: 8.0,
			},
			awayStats: &types.TeamStats{
				CF60:       62.0,
				CA60:       58.0,
				GFPerGame:  3.3,
				GAPerGame:  3.5,
				PPPct:      21.0,
				PKPct:      79.0,
				PIMPerGame: 9.0,
			},
			homeGoalie: &types.GoalieStats{GSAx: -5.0},
			awayGoalie: &types.GoalieStats{GSAx: -3.0},
			ctx: &types.GameContext{
				TotalLine:      7.0,
				ImpliedHomeWin: 0.52,
				ImpliedAwayWin: 0.48,
			},
			expectedScenario: types.ScenarioRunAndGun,
		},
		{
			name: "Goalie Duel - elite goalies, low pace",
			homeStats: &types.TeamStats{
				CF60:       48.0,
				CA60:       50.0,
				GFPerGame:  2.5,
				GAPerGame:  2.3,
				PPPct:      18.0,
				PKPct:      85.0,
				PIMPerGame: 6.0,
			},
			awayStats: &types.TeamStats{
				CF60:       50.0,
				CA60:       48.0,
				GFPerGame:  2.4,
				GAPerGame:  2.5,
				PPPct:      17.0,
				PKPct:      84.0,
				PIMPerGame: 5.0,
			},
			homeGoalie: &types.GoalieStats{GSAx: 8.0},
			awayGoalie: &types.GoalieStats{GSAx: 10.0},
			ctx: &types.GameContext{
				TotalLine:      5.0,
				ImpliedHomeWin: 0.50,
				ImpliedAwayWin: 0.50,
			},
			expectedScenario: types.ScenarioGoalieDuel,
		},
		{
			name: "Special Teams - high PIM, weak PK",
			homeStats: &types.TeamStats{
				CF60:       55.0,
				CA60:       55.0,
				GFPerGame:  3.0,
				GAPerGame:  3.0,
				PPPct:      28.0, // Elite PP
				PKPct:      75.0, // Weak PK
				PIMPerGame: 12.0, // High PIM
			},
			awayStats: &types.TeamStats{
				CF60:       55.0,
				CA60:       55.0,
				GFPerGame:  3.0,
				GAPerGame:  3.0,
				PPPct:      25.0,
				PKPct:      74.0,
				PIMPerGame: 14.0,
			},
			homeGoalie: &types.GoalieStats{GSAx: 0.0},
			awayGoalie: &types.GoalieStats{GSAx: 0.0},
			ctx: &types.GameContext{
				TotalLine:      6.0,
				ImpliedHomeWin: 0.55,
				ImpliedAwayWin: 0.45,
			},
			expectedScenario: types.ScenarioSpecialTeams,
		},
		{
			name: "Score Effects - big favorite",
			homeStats: &types.TeamStats{
				CF60:       58.0,
				CA60:       52.0,
				GFPerGame:  3.5,
				GAPerGame:  2.5,
				PPPct:      24.0,
				PKPct:      82.0,
				PIMPerGame: 8.0,
			},
			awayStats: &types.TeamStats{
				CF60:       50.0,
				CA60:       58.0,
				GFPerGame:  2.4,
				GAPerGame:  3.3,
				PPPct:      18.0,
				PKPct:      76.0,
				PIMPerGame: 9.0,
			},
			homeGoalie: &types.GoalieStats{GSAx: 3.0},
			awayGoalie: &types.GoalieStats{GSAx: -2.0},
			ctx: &types.GameContext{
				TotalLine:      6.5,
				ImpliedHomeWin: 0.72, // Big favorite
				ImpliedAwayWin: 0.28,
				HomeSpread:     -1.5,
			},
			expectedScenario: types.ScenarioScoreEffects,
		},
		{
			name: "Bench Slog - fatigued teams",
			homeStats: &types.TeamStats{
				CF60:       48.0,
				CA60:       52.0,
				GFPerGame:  2.5,
				GAPerGame:  2.8,
				PPPct:      18.0,
				PKPct:      80.0,
				PIMPerGame: 7.0,
			},
			awayStats: &types.TeamStats{
				CF60:       50.0,
				CA60:       50.0,
				GFPerGame:  2.6,
				GAPerGame:  2.7,
				PPPct:      19.0,
				PKPct:      81.0,
				PIMPerGame: 6.0,
			},
			homeGoalie: &types.GoalieStats{GSAx: 2.0},
			awayGoalie: &types.GoalieStats{GSAx: 2.0},
			ctx: &types.GameContext{
				TotalLine:      5.5,
				ImpliedHomeWin: 0.52,
				ImpliedAwayWin: 0.48,
				HomeFatigue:    0.15, // B2B fatigue
				AwayFatigue:    0.12,
			},
			expectedScenario: types.ScenarioBenchSlog,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scenario := engine.ClassifyGame(tt.homeStats, tt.awayStats, tt.homeGoalie, tt.awayGoalie, tt.ctx)

			if scenario.Primary.Scenario != tt.expectedScenario {
				t.Logf("Scenario scores:")
				for _, s := range scenario.AllScores {
					t.Logf("  %s: %.1f (factors: %v)", s.Scenario, s.Score, s.Factors)
				}
				t.Errorf("Expected primary scenario %s, got %s", tt.expectedScenario, scenario.Primary.Scenario)
			}

			// Verify primary score is highest
			for _, s := range scenario.AllScores {
				if s.Score > scenario.Primary.Score {
					t.Errorf("Primary score (%.1f) is not highest, %s has %.1f",
						scenario.Primary.Score, s.Scenario, s.Score)
				}
			}
		})
	}
}

func TestDetermineActualScenario(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name     string
		boxscore *types.Boxscore
		expected types.ScenarioType
	}{
		{
			name: "High scoring game",
			boxscore: &types.Boxscore{
				HomeScore: 5,
				AwayScore: 4,
			},
			expected: types.ScenarioRunAndGun,
		},
		{
			name: "Low scoring game",
			boxscore: &types.Boxscore{
				HomeScore: 1,
				AwayScore: 2,
			},
			expected: types.ScenarioGoalieDuel,
		},
		{
			name: "Blowout",
			boxscore: &types.Boxscore{
				HomeScore: 6,
				AwayScore: 1,
			},
			expected: types.ScenarioScoreEffects,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize player stats map
			tt.boxscore.PlayerStats = make(map[int]types.BoxscorePlayerStats)

			actual := engine.DetermineActualScenario(tt.boxscore)
			if actual != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, actual)
			}
		})
	}
}

func TestScenarioPreferredProps(t *testing.T) {
	tests := []struct {
		scenario types.ScenarioType
		shouldHavePreferred bool
		shouldHaveAvoid     bool
	}{
		{types.ScenarioRunAndGun, true, true},
		{types.ScenarioGoalieDuel, true, true},
		{types.ScenarioSpecialTeams, true, false},
		{types.ScenarioScoreEffects, true, false},
		{types.ScenarioBenchSlog, true, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.scenario), func(t *testing.T) {
			prefs := tt.scenario.GetPreferredProps()

			if tt.shouldHavePreferred && len(prefs.Preferred) == 0 {
				t.Errorf("Expected preferred props for %s", tt.scenario)
			}

			if tt.shouldHaveAvoid && len(prefs.Avoid) == 0 {
				t.Errorf("Expected avoid props for %s", tt.scenario)
			}
		})
	}
}

func BenchmarkScenarioClassification(b *testing.B) {
	engine := NewEngine()

	homeStats := &types.TeamStats{
		CF60: 55.0, CA60: 53.0, GFPerGame: 3.0, GAPerGame: 2.8,
		PPPct: 21.0, PKPct: 80.0, PIMPerGame: 8.0,
	}
	awayStats := &types.TeamStats{
		CF60: 53.0, CA60: 55.0, GFPerGame: 2.9, GAPerGame: 3.0,
		PPPct: 20.0, PKPct: 79.0, PIMPerGame: 9.0,
	}
	homeGoalie := &types.GoalieStats{GSAx: 2.0}
	awayGoalie := &types.GoalieStats{GSAx: -1.0}
	ctx := &types.GameContext{TotalLine: 6.0, ImpliedHomeWin: 0.55, ImpliedAwayWin: 0.45}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.ClassifyGame(homeStats, awayStats, homeGoalie, awayGoalie, ctx)
	}
}
