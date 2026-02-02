# NHL Player-Prop Betting Model

A Go-based interactive CLI application that implements an NHL player-prop prediction model with statistical distributions, scenario-based game classification, correlation-aware parlay construction, and a post-mortem learning system.

## Features

- **Statistical Distributions**: Hierarchical Poisson-Gamma for SOG, Zero-Inflated Negative Binomial for physical stats, Poisson-Binomial mixture for scoring
- **Scenario Engine**: Five game scripts (Run-and-Gun, Goalie Duel, Special Teams, Score Effects, Bench Slog)
- **Correlation Fabric**: Scenario-conditioned correlation pairs for parlay construction
- **Prop Evaluator**: Tier hierarchy, guardrails, edge calculation, stop-line logic
- **Interactive TUI**: Bubble Tea-based interface for slate analysis, prop evaluation, and parlay building
- **Post-Mortem System**: Auto-grading from boxscore API, CLV calculation, performance tracking
- **Data Sources**: NHL API integration, MoneyPuck xG data support

## Installation

```bash
# Clone the repository
cd nhl-prop-model

# Download dependencies
go mod tidy

# Build the binary
go build -o nhlprop ./cmd/nhlprop
```

## Usage

```bash
# Run the interactive CLI
./nhlprop

# Or run directly with go
go run cmd/nhlprop/main.go
```

## Architecture

```
nhl-prop-model/
├── cmd/nhlprop/main.go              # Entry point
├── internal/
│   ├── api/                         # Data fetching
│   │   ├── nhl/                     # NHL API client
│   │   ├── moneypuck/               # xG data (CSV)
│   │   └── odds/                    # Odds input (manual)
│   ├── model/                       # Core prediction logic
│   │   ├── distributions/           # Statistical models
│   │   ├── scenario/                # Game script engine
│   │   ├── correlation/             # Correlation fabric
│   │   ├── props/                   # Prop evaluation + guardrails
│   │   └── calibration/             # Bias adjustments
│   ├── storage/                     # SQLite persistence
│   ├── ui/                          # Bubble Tea interactive CLI
│   └── output/                      # Formatters (tables, export)
├── pkg/types/                       # Domain types
└── data/                            # Static data + migrations
```

## Key Components

### Statistical Distributions

- **Hierarchical Poisson-Gamma** (`poisson.go`): For shots on goal with covariates (TOI, OZ%, PPTOI, pace, opponent CA/60, rink bias, fatigue)
- **Zero-Inflated Negative Binomial** (`negbinom.go`): For Hits/Blocks/PIM with arena and ref biases
- **Poisson-Binomial Mixture** (`mixture.go`): For Points/Goals/Assists based on opportunities × conversion

### Scenario Engine

| Script | Triggers | Preferred Props |
|--------|----------|-----------------|
| Run-and-Gun | High pace, weak GSAx | SOG/Points/Goals Over |
| Goalie Duel | Elite GSAx, low pace | SOG/Points Under |
| Special-Teams | Weak PK, high PIM | PP Points Over, PIM Over |
| Score-Effects | Big favorite | Trailing team SOG/Points Over |
| Bench-Slog | B2B, trap systems | SOG/Points Under, Hits/Blocks Over |

### Guardrails

- No depth-role Overs (4th line, volatile PP2)
- Top-6 F / Top-4 D required for point props
- PP1 required for assist props
- Team total sanity check (penalize if GF/G < 2.5)
- TOI minimums: 14 min (F), 18 min (D)

### Correlation Pairs

- SOG Over ↔ Points Over: +0.45 base, +0.65 in Run-and-Gun
- SOG Over ↔ Opp Saves Over: +0.55 base, +0.70 in Run-and-Gun
- PIM Over ↔ PP Points Over: +0.40 base, +0.60 in Special-Teams

## Data Storage

SQLite database stored at `~/.nhlprop/nhlprop.db` with tables:
- `picks`: Full pick tracking with open/close lines, results, CLV
- `parlays`: Parlay tracking with legs junction table
- `calibration`: Arena bias, ref crews, fatigue penalties
- `prop_family_stats`: Hit rates by market type
- `game_scenarios`: Scenario prediction accuracy

## Navigation

- **Enter**: Select / Proceed
- **Esc**: Go back
- **Tab**: Switch sites (in parlay builder)
- **q**: Quit

## Dependencies

- `gonum.org/v1/gonum` - Statistics
- `github.com/charmbracelet/bubbletea` - Interactive TUI
- `github.com/charmbracelet/bubbles` - TUI components
- `github.com/charmbracelet/lipgloss` - TUI styling
- `github.com/mattn/go-sqlite3` - Database
- `github.com/go-resty/resty/v2` - HTTP client

## License

MIT
