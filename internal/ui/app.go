package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/paul/nhl-prop-model/internal/api/nhl"
	"github.com/paul/nhl-prop-model/internal/model/correlation"
	"github.com/paul/nhl-prop-model/internal/model/props"
	"github.com/paul/nhl-prop-model/internal/model/scenario"
	"github.com/paul/nhl-prop-model/internal/storage"
	"github.com/paul/nhl-prop-model/pkg/types"
)

// Screen represents the current screen
type Screen int

const (
	ScreenHome Screen = iota
	ScreenSlateAnalysis
	ScreenScenarioDisplay
	ScreenPropEdgeBoard
	ScreenParlayBuilder
	ScreenPostMortem
	ScreenDashboard
)

// Model is the main Bubble Tea model
type Model struct {
	// Dependencies
	db          *storage.DB
	nhlClient   *nhl.Client
	evaluator   *props.Evaluator
	corrMatrix  *correlation.Matrix
	scenarioEng *scenario.Engine
	parlayBuilder *props.ParlayBuilder

	// UI state
	screen       Screen
	width        int
	height       int
	ready        bool
	loading      bool
	err          error
	statusMsg    string

	// Components
	spinner      spinner.Model
	homeList     list.Model

	// Data state
	games        []types.Game
	selectedGame *types.Game
	scenario     *types.GameScenario
	propEvals    []*types.PropEvaluation
	parlays      []*types.Parlay
	picks        []types.PropPick

	// Parlay builder state
	selectedLegs []*types.PropEvaluation
	currentSite  types.Site
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170")).
		MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	selectedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("212")).
		Bold(true)

	normalStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	successStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("46"))

	dimStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	borderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	tableHeaderStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Padding(0, 1)

	tableRowStyle = lipgloss.NewStyle().
		Padding(0, 1)

	starStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("226"))
)

// Initialize creates a new Model
func Initialize(db *storage.DB) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	items := []list.Item{
		menuItem{title: "Analyze Today's Slate", desc: "Fetch and analyze today's games"},
		menuItem{title: "View Pick History", desc: "Browse past picks and results"},
		menuItem{title: "Post-Mortem", desc: "Grade completed picks"},
		menuItem{title: "Dashboard", desc: "View performance statistics"},
		menuItem{title: "Settings", desc: "Configure model parameters"},
	}

	homeList := list.New(items, list.NewDefaultDelegate(), 0, 0)
	homeList.Title = "NHL Prop Model"
	homeList.SetShowStatusBar(false)
	homeList.SetFilteringEnabled(false)
	homeList.Styles.Title = titleStyle

	// Initialize dependencies
	nhlClient := nhl.New()
	corrMatrix := correlation.NewMatrix()
	evaluator := props.NewEvaluator().WithCorrMatrix(corrMatrix)
	scenarioEng := scenario.NewEngine()
	parlayBuilder := props.NewParlayBuilder(corrMatrix, evaluator)

	return Model{
		db:            db,
		nhlClient:     nhlClient,
		evaluator:     evaluator,
		corrMatrix:    corrMatrix,
		scenarioEng:   scenarioEng,
		parlayBuilder: parlayBuilder,
		screen:        ScreenHome,
		spinner:       s,
		homeList:      homeList,
		currentSite:   types.SiteUnderdog,
	}
}

type menuItem struct {
	title, desc string
}

func (i menuItem) Title() string       { return i.title }
func (i menuItem) Description() string { return i.desc }
func (i menuItem) FilterValue() string { return i.title }

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.homeList.SetSize(msg.Width-4, msg.Height-4)
		m.ready = true
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, keys.Back):
			return m.handleBack()

		case key.Matches(msg, keys.Enter):
			return m.handleEnter()

		case key.Matches(msg, keys.Tab):
			return m.handleTab()
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case gamesLoadedMsg:
		m.loading = false
		m.games = msg.games
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.screen = ScreenSlateAnalysis
		}
		return m, nil

	case scenarioLoadedMsg:
		m.loading = false
		m.scenario = msg.scenario
		m.screen = ScreenScenarioDisplay
		return m, nil

	case propsEvaluatedMsg:
		m.loading = false
		m.propEvals = msg.evals
		m.screen = ScreenPropEdgeBoard
		return m, nil

	case parlaysBuiltMsg:
		m.loading = false
		m.parlays = msg.parlays
		return m, nil

	case picksLoadedMsg:
		m.loading = false
		m.picks = msg.picks
		return m, nil

	case errMsg:
		m.loading = false
		m.err = msg.err
		return m, nil
	}

	// Update current screen components
	switch m.screen {
	case ScreenHome:
		var cmd tea.Cmd
		m.homeList, cmd = m.homeList.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleBack() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenHome:
		return m, tea.Quit
	case ScreenSlateAnalysis:
		m.screen = ScreenHome
	case ScreenScenarioDisplay:
		m.screen = ScreenSlateAnalysis
	case ScreenPropEdgeBoard:
		m.screen = ScreenScenarioDisplay
	case ScreenParlayBuilder:
		m.screen = ScreenPropEdgeBoard
	case ScreenPostMortem:
		m.screen = ScreenHome
	case ScreenDashboard:
		m.screen = ScreenHome
	}
	return m, nil
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenHome:
		item, ok := m.homeList.SelectedItem().(menuItem)
		if !ok {
			return m, nil
		}

		switch item.title {
		case "Analyze Today's Slate":
			m.loading = true
			return m, fetchGames(m.nhlClient)

		case "View Pick History":
			m.loading = true
			return m, loadPicks(storage.NewPickStore(m.db))

		case "Post-Mortem":
			m.screen = ScreenPostMortem
			m.loading = true
			return m, loadUngradedPicks(storage.NewPickStore(m.db))

		case "Dashboard":
			m.screen = ScreenDashboard
			return m, nil
		}

	case ScreenSlateAnalysis:
		// Select a game
		if len(m.games) > 0 {
			m.selectedGame = &m.games[0] // TODO: proper selection
			m.loading = true
			return m, analyzeGame(m.selectedGame, m.scenarioEng, m.nhlClient)
		}

	case ScreenScenarioDisplay:
		// Proceed to prop evaluation
		m.loading = true
		return m, evaluateProps(m.selectedGame, m.scenario, m.evaluator, m.nhlClient)

	case ScreenPropEdgeBoard:
		// Proceed to parlay builder
		m.screen = ScreenParlayBuilder
		return m, buildParlays(m.propEvals, m.scenario, m.currentSite, m.parlayBuilder)
	}

	return m, nil
}

func (m Model) handleTab() (tea.Model, tea.Cmd) {
	// Cycle through sites in parlay builder
	if m.screen == ScreenParlayBuilder {
		switch m.currentSite {
		case types.SiteUnderdog:
			m.currentSite = types.SitePrizePicks
		case types.SitePrizePicks:
			m.currentSite = types.SiteDraftKings
		case types.SiteDraftKings:
			m.currentSite = types.SiteBetr
		case types.SiteBetr:
			m.currentSite = types.SiteUnderdog
		}
		return m, buildParlays(m.propEvals, m.scenario, m.currentSite, m.parlayBuilder)
	}
	return m, nil
}

// View implements tea.Model
func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var content string

	switch m.screen {
	case ScreenHome:
		content = m.viewHome()
	case ScreenSlateAnalysis:
		content = m.viewSlateAnalysis()
	case ScreenScenarioDisplay:
		content = m.viewScenarioDisplay()
	case ScreenPropEdgeBoard:
		content = m.viewPropEdgeBoard()
	case ScreenParlayBuilder:
		content = m.viewParlayBuilder()
	case ScreenPostMortem:
		content = m.viewPostMortem()
	case ScreenDashboard:
		content = m.viewDashboard()
	}

	if m.loading {
		content = m.viewLoading()
	}

	if m.err != nil {
		content += "\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	// Add help footer
	help := dimStyle.Render("\n[Enter] Select  [Esc] Back  [q] Quit")

	return content + help
}

func (m Model) viewLoading() string {
	return fmt.Sprintf("\n\n   %s Loading...\n\n", m.spinner.View())
}

func (m Model) viewHome() string {
	return "\n" + m.homeList.View()
}

func (m Model) viewSlateAnalysis() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Today's Slate"))
	b.WriteString("\n\n")

	if len(m.games) == 0 {
		b.WriteString(dimStyle.Render("No games scheduled"))
		return b.String()
	}

	// Table header
	header := tableHeaderStyle.Render(fmt.Sprintf("%-5s %-10s %-5s %-20s %-8s",
		"", "Away", "vs", "Home", "Time"))
	b.WriteString(header)
	b.WriteString("\n")

	// Game rows
	for i, game := range m.games {
		status := " "
		if m.selectedGame != nil && m.selectedGame.ID == game.ID {
			status = "▶"
		}

		timeStr := game.StartTime.Format("3:04 PM")
		row := fmt.Sprintf("%-5s %-10s %-5s %-20s %-8s",
			status, game.AwayTeamAbbr, "@", game.HomeTeamAbbr, timeStr)

		if i%2 == 0 {
			b.WriteString(tableRowStyle.Render(row))
		} else {
			b.WriteString(tableRowStyle.Copy().Background(lipgloss.Color("235")).Render(row))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("%d games on today's slate", len(m.games))))

	return b.String()
}

func (m Model) viewScenarioDisplay() string {
	var b strings.Builder

	if m.selectedGame == nil || m.scenario == nil {
		return "No game selected"
	}

	b.WriteString(titleStyle.Render(fmt.Sprintf("%s @ %s - Scenario Analysis",
		m.selectedGame.AwayTeamAbbr, m.selectedGame.HomeTeamAbbr)))
	b.WriteString("\n\n")

	// Primary scenario
	primary := m.scenario.Primary
	b.WriteString(selectedStyle.Render(fmt.Sprintf("Primary: %s (%.0f pts)",
		primary.Scenario, primary.Score)))
	b.WriteString("\n")
	b.WriteString(normalStyle.Render(primary.Scenario.Description()))
	b.WriteString("\n")

	if len(primary.Factors) > 0 {
		b.WriteString(dimStyle.Render("Factors: "))
		b.WriteString(strings.Join(primary.Factors, ", "))
		b.WriteString("\n")
	}

	// Secondary scenario
	if m.scenario.Secondary != nil {
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render(fmt.Sprintf("Secondary: %s (%.0f pts)",
			m.scenario.Secondary.Scenario, m.scenario.Secondary.Score)))
		b.WriteString("\n")
	}

	// Metrics
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Key Metrics:"))
	b.WriteString("\n")
	metrics := m.scenario.Metrics
	b.WriteString(fmt.Sprintf("  Combined Pace: %.1f CF/60\n", metrics.CombinedPace))
	b.WriteString(fmt.Sprintf("  Combined GSAx: %.1f\n", metrics.CombinedGSAx))
	b.WriteString(fmt.Sprintf("  Expected Total: %.1f\n", metrics.ExpectedTotal))
	b.WriteString(fmt.Sprintf("  Combined Fatigue: %.2f\n", metrics.CombinedFatigue))

	// Preferred props
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Preferred Props:"))
	b.WriteString("\n")
	preferred := primary.Scenario.GetPreferredProps()
	for _, p := range preferred.Preferred {
		b.WriteString(fmt.Sprintf("  ✓ %s\n", p))
	}

	if len(preferred.Avoid) > 0 {
		b.WriteString(subtitleStyle.Render("Avoid:"))
		b.WriteString("\n")
		for _, a := range preferred.Avoid {
			b.WriteString(fmt.Sprintf("  ✗ %s\n", a))
		}
	}

	return b.String()
}

func (m Model) viewPropEdgeBoard() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Prop Edge Board"))
	b.WriteString("\n\n")

	if len(m.propEvals) == 0 {
		b.WriteString(dimStyle.Render("No props available"))
		return b.String()
	}

	// Header
	header := tableHeaderStyle.Render(fmt.Sprintf("%-15s %-5s %-12s %-6s %-8s %-6s %-5s %-8s",
		"Player", "Team", "Market", "Line", "Our Line", "Edge", "Stars", "Status"))
	b.WriteString(header)
	b.WriteString("\n")

	// Prop rows (max 15 for display)
	displayCount := 15
	if len(m.propEvals) < displayCount {
		displayCount = len(m.propEvals)
	}

	for i := 0; i < displayCount; i++ {
		eval := m.propEvals[i]

		stars := strings.Repeat("★", eval.Stars) + strings.Repeat("☆", 5-eval.Stars)
		status := "✓"
		if !eval.PassesGuardrails {
			status = "✗"
		}

		name := truncate(eval.Line.PlayerName, 15)
		market := string(eval.Line.Market)
		if len(market) > 12 {
			market = market[:12]
		}

		row := fmt.Sprintf("%-15s %-5s %-12s %-6.1f %-8.2f %-6.1f %-5s %-8s",
			name, eval.Line.TeamAbbr, market, eval.Line.Line,
			eval.OurLine, eval.Edge, stars, status)

		if !eval.PassesGuardrails {
			b.WriteString(dimStyle.Render(row))
		} else if eval.Stars >= 4 {
			b.WriteString(selectedStyle.Render(row))
		} else {
			b.WriteString(normalStyle.Render(row))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("Showing %d of %d props", displayCount, len(m.propEvals))))

	return b.String()
}

func (m Model) viewParlayBuilder() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf("Parlay Builder - %s", m.currentSite)))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("[Tab] Switch site"))
	b.WriteString("\n\n")

	if len(m.parlays) == 0 {
		b.WriteString(dimStyle.Render("No parlays built"))
		return b.String()
	}

	for i, parlay := range m.parlays {
		// Parlay header
		b.WriteString(selectedStyle.Render(fmt.Sprintf("Card %c: %d legs | EV: %.2f | Joint P: %.1f%%",
			'A'+i, len(parlay.Legs), parlay.EV, parlay.JointProb*100)))
		b.WriteString("\n")

		// Legs
		for _, leg := range parlay.Legs {
			b.WriteString(fmt.Sprintf("  • %s %s %s %.1f (%.1f%% edge)\n",
				leg.PlayerName, leg.Market, leg.Side, leg.Line, leg.Edge))
		}

		// Validation
		if !parlay.CorrelationValid {
			b.WriteString(errorStyle.Render("  ⚠ Correlation issues"))
			b.WriteString("\n")
		}

		if len(parlay.RiskFlags) > 0 {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  Warnings: %s", strings.Join(parlay.RiskFlags, ", "))))
			b.WriteString("\n")
		}

		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) viewPostMortem() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Post-Mortem"))
	b.WriteString("\n\n")

	if len(m.picks) == 0 {
		b.WriteString(dimStyle.Render("No ungraded picks"))
		return b.String()
	}

	b.WriteString(subtitleStyle.Render(fmt.Sprintf("%d picks to grade", len(m.picks))))
	b.WriteString("\n\n")

	// Header
	header := tableHeaderStyle.Render(fmt.Sprintf("%-15s %-12s %-6s %-8s %-6s %-8s",
		"Player", "Market", "Line", "Result", "Actual", "CLV"))
	b.WriteString(header)
	b.WriteString("\n")

	for i, pick := range m.picks {
		if i >= 10 {
			b.WriteString(dimStyle.Render("...and more"))
			break
		}

		name := truncate(pick.PlayerName, 15)
		result := string(pick.Result)
		if result == "" {
			result = "pending"
		}

		row := fmt.Sprintf("%-15s %-12s %-6.1f %-8s %-6.1f %-8.1f",
			name, pick.Market, pick.Line, result, pick.ActualValue, pick.CLV)

		switch pick.Result {
		case types.PropResultWin:
			b.WriteString(successStyle.Render(row))
		case types.PropResultLoss:
			b.WriteString(errorStyle.Render(row))
		default:
			b.WriteString(normalStyle.Render(row))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) viewDashboard() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Performance Dashboard"))
	b.WriteString("\n\n")

	// This would load from storage in a real implementation
	b.WriteString(subtitleStyle.Render("Prop Family Hit Rates"))
	b.WriteString("\n")
	b.WriteString("  Points O0.5: 72.3% (144/199)\n")
	b.WriteString("  SOG Over:    68.1% (98/144)\n")
	b.WriteString("  Assists:     61.2% (56/92)\n")
	b.WriteString("  AGS:         42.1% (24/57)\n")

	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Scenario Accuracy"))
	b.WriteString("\n")
	b.WriteString("  Run-and-Gun:    64.2%\n")
	b.WriteString("  Goalie Duel:    71.8%\n")
	b.WriteString("  Special Teams:  58.3%\n")
	b.WriteString("  Score Effects:  55.1%\n")
	b.WriteString("  Bench Slog:     66.7%\n")

	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("CLV Metrics"))
	b.WriteString("\n")
	b.WriteString("  Avg CLV: +2.3 pp\n")
	b.WriteString("  CLV Win Rate: 58.2%\n")

	return b.String()
}

// Helper functions

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// Key bindings
var keys = keyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
	),
}

type keyMap struct {
	Quit  key.Binding
	Back  key.Binding
	Enter key.Binding
	Tab   key.Binding
}
