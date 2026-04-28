package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	version "github.com/hashicorp/go-version"
	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
	goldast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"golang.org/x/term"
)

const tuiReleaseHistoryLimit = 100

type tuiCmd struct {
	cmd         *cobra.Command
	newProvider providerFactory
}

type tuiScreen int

type tuiColumnFocus int

const (
	tuiScreenMenu tuiScreen = iota
	tuiScreenTargets
	tuiScreenTargetsLoading
	tuiScreenDetail
	tuiScreenDetailLoading
)

const (
	tuiFocusTargets tuiColumnFocus = iota
	tuiFocusVersions
)

type tuiMenuItem struct {
	Title       string
	Description string
}

type changelogTarget struct {
	Binary          *config.Binary
	Name            string
	CurrentVersion  string
	LatestVersion   string
	LatestURL       string
	ProviderID      string
	SupportsHistory bool
}

type changelogRelease struct {
	Version     string
	URL         string
	PublishedAt *time.Time
	Body        string
	Summary     string
}

type changelogDetail struct {
	Target        changelogTarget
	Releases      []changelogRelease
	SelectedIndex int
	Notice        string
}

type outdatedLoadedMsg struct {
	targets []changelogTarget
	err     error
}

type detailLoadedMsg struct {
	targetKey string
	detail    *changelogDetail
}

type tuiModel struct {
	width       int
	height      int
	screen      tuiScreen
	newProvider providerFactory

	menuItems  []tuiMenuItem
	menuCursor int

	targets       []changelogTarget
	targetCursor  int
	targetsNotice string

	detail        *changelogDetail
	detailFocus   tuiColumnFocus
	detailCache   map[string]*changelogDetail
	detailLoading bool
}

var (
	tuiFrameStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("239")).
			Foreground(lipgloss.Color("252"))

	tuiPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("239")).
			Padding(1, 1)

	tuiShellStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("239")).
			Padding(0, 0)

	tuiColumnStyle = lipgloss.NewStyle().Padding(0, 1)

	tuiSelectedCardStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(lipgloss.Color("81")).
				Foreground(lipgloss.Color("230")).
				Padding(0, 1)

	tuiPassiveCardStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(lipgloss.Color("239")).
				Foreground(lipgloss.Color("252")).
				Padding(0, 1)

	tuiDividerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("239"))
	tuiKeyStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))

	tuiTitleStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	tuiLabelStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	tuiMutedStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	tuiAccentStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
	tuiSuccessStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("114"))
	tuiWarningStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("221"))
	tuiSelectionStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24")).Bold(true)
	tuiHeadingStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("153"))
	tuiAddedStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("114"))
	tuiChangedStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
	tuiFixedStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("221"))
	tuiImprovedStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("177"))
	tuiMarkdownCodeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("238"))
	tuiMarkdownLinkStyle  = lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("81"))
	tuiMarkdownQuoteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	tuiSummaryStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	tuiUnavailableStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("209"))
)

func newTUICmd() *tuiCmd {
	root := &tuiCmd{newProvider: providers.New}
	cmd := &cobra.Command{
		Use:           "tui",
		Short:         "Launch the interactive TUI",
		Hidden:        true,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !tuiTerminalReady() {
				return fmt.Errorf("interactive terminal required")
			}
			return runTUIWithProvider(root.newProvider)
		},
	}

	root.cmd = cmd
	return root
}

func shouldLaunchZeroArgTUI(args []string) bool {
	return len(args) == 0 && tuiTerminalReady()
}

func tuiTerminalReady() bool {
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}

	return term.IsTerminal(int(os.Stdin.Fd())) &&
		term.IsTerminal(int(os.Stdout.Fd())) &&
		term.IsTerminal(int(os.Stderr.Fd()))
}

func runTUI() error {
	return runTUIWithProvider(providers.New)
}

func runTUIWithProvider(newProvider providerFactory) error {
	model := newTUIModel(newProvider)
	_, err := tea.NewProgram(model).Run()
	return err
}

func newTUIModel(newProvider providerFactory) tuiModel {
	return tuiModel{
		screen:      tuiScreenMenu,
		newProvider: newProvider,
		detailCache: map[string]*changelogDetail{},
		menuItems: []tuiMenuItem{{
			Title:       "Changelog",
			Description: "Inspect outdated packages and read release notes between the installed and latest versions.",
		}},
	}
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case outdatedLoadedMsg:
		m.targets = msg.targets
		m.targetCursor = 0
		m.targetsNotice = ""
		m.detail = nil
		m.detailLoading = false
		m.detailCache = map[string]*changelogDetail{}
		m.detailFocus = tuiFocusVersions
		if msg.err != nil {
			m.screen = tuiScreenTargets
			m.targets = nil
			m.targetsNotice = fmt.Sprintf("Unable to load outdated packages.\n\n%s", msg.err.Error())
			return m, nil
		}
		if len(msg.targets) == 0 {
			m.screen = tuiScreenTargets
			return m, nil
		}
		return m.activateTarget(0, tuiFocusVersions)
	case detailLoadedMsg:
		m.detailCache[msg.targetKey] = msg.detail
		currentTarget, ok := m.selectedTarget()
		if !ok || changelogTargetKey(currentTarget) != msg.targetKey {
			return m, nil
		}
		m.screen = tuiScreenDetail
		m.detail = msg.detail
		m.detailLoading = false
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
	}

	switch m.screen {
	case tuiScreenMenu:
		return m.updateMenu(msg)
	case tuiScreenTargets, tuiScreenTargetsLoading:
		return m.updateTargets(msg)
	case tuiScreenDetail, tuiScreenDetailLoading:
		return m.updateDetail(msg)
	default:
		return m, nil
	}
}

func (m tuiModel) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "up", "k":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
	case "down", "j":
		if m.menuCursor < len(m.menuItems)-1 {
			m.menuCursor++
		}
	case "enter":
		m.screen = tuiScreenTargetsLoading
		m.targets = nil
		m.targetCursor = 0
		m.targetsNotice = ""
		m.detail = nil
		m.detailLoading = false
		m.detailCache = map[string]*changelogDetail{}
		return m, loadOutdatedTargetsCmd(m.newProvider)
	case "q", "esc":
		return m, tea.Quit
	}

	return m, nil
}

func (m tuiModel) updateTargets(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.screen == tuiScreenTargetsLoading {
		switch key.String() {
		case "q", "esc":
			m.screen = tuiScreenMenu
			return m, nil
		}
		return m, nil
	}

	switch key.String() {
	case "up", "k":
		if m.targetCursor > 0 {
			m.targetCursor--
		}
	case "down", "j":
		if m.targetCursor < len(m.targets)-1 {
			m.targetCursor++
		}
	case "enter":
		if len(m.targets) == 0 {
			return m, nil
		}
		return m.activateTarget(m.targetCursor, tuiFocusVersions)
	case "q", "esc", "backspace":
		m.screen = tuiScreenMenu
	}

	return m, nil
}

func (m tuiModel) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.detail == nil {
		return m, nil
	}

	switch key.String() {
	case "left", "h":
		m.detailFocus = tuiFocusTargets
	case "right", "l":
		m.detailFocus = tuiFocusVersions
	case "up", "k":
		if m.detailFocus == tuiFocusTargets {
			if m.targetCursor > 0 {
				return m.activateTarget(m.targetCursor-1, tuiFocusTargets)
			}
			return m, nil
		}
		if m.detail.SelectedIndex > 0 {
			m.detail.SelectedIndex--
		}
	case "down", "j":
		if m.detailFocus == tuiFocusTargets {
			if m.targetCursor < len(m.targets)-1 {
				return m.activateTarget(m.targetCursor+1, tuiFocusTargets)
			}
			return m, nil
		}
		if m.detail.SelectedIndex < len(m.detail.Releases)-1 {
			m.detail.SelectedIndex++
		}
	case "g":
		if m.detailFocus == tuiFocusTargets {
			if len(m.targets) == 0 || m.targetCursor == 0 {
				return m, nil
			}
			return m.activateTarget(0, tuiFocusTargets)
		}
		m.detail.SelectedIndex = 0
	case "G":
		if m.detailFocus == tuiFocusTargets {
			if len(m.targets) == 0 || m.targetCursor == len(m.targets)-1 {
				return m, nil
			}
			return m.activateTarget(len(m.targets)-1, tuiFocusTargets)
		}
		if len(m.detail.Releases) > 0 {
			m.detail.SelectedIndex = len(m.detail.Releases) - 1
		}
	case "q", "esc", "backspace":
		m.screen = tuiScreenMenu
	}

	return m, nil
}

func (m tuiModel) View() tea.View {
	width, height := m.viewportSize()
	innerWidth := maxInt(20, width-2)
	innerHeight := maxInt(8, height-2)

	content := m.renderScreen(innerWidth, innerHeight)
	view := tea.NewView(tuiFrameStyle.Width(innerWidth).Height(innerHeight).Render(content))
	view.AltScreen = true
	return view
}

func (m tuiModel) selectedTarget() (changelogTarget, bool) {
	if len(m.targets) == 0 || m.targetCursor < 0 || m.targetCursor >= len(m.targets) {
		return changelogTarget{}, false
	}
	return m.targets[m.targetCursor], true
}

func (m tuiModel) activateTarget(index int, focus tuiColumnFocus) (tuiModel, tea.Cmd) {
	if len(m.targets) == 0 {
		m.detail = nil
		m.detailLoading = false
		return m, nil
	}
	if index < 0 {
		index = 0
	}
	if index >= len(m.targets) {
		index = len(m.targets) - 1
	}

	m.targetCursor = index
	m.detailFocus = focus
	m.screen = tuiScreenDetail

	target := m.targets[index]
	key := changelogTargetKey(target)
	if detail, ok := m.detailCache[key]; ok {
		m.detail = detail
		m.detailLoading = false
		return m, nil
	}

	m.detail = &changelogDetail{
		Target: target,
		Notice: "Loading release history...",
	}
	m.detailLoading = true
	return m, loadChangelogDetailCmd(target, m.newProvider)
}

func (m tuiModel) viewportSize() (int, int) {
	width := m.width
	height := m.height
	if width <= 0 {
		width = 120
	}
	if height <= 0 {
		height = 36
	}
	return width, height
}

func (m tuiModel) renderScreen(width, height int) string {
	switch m.screen {
	case tuiScreenMenu:
		return m.renderMenu(width, height)
	case tuiScreenTargets, tuiScreenTargetsLoading:
		return m.renderTargets(width, height)
	case tuiScreenDetail, tuiScreenDetailLoading:
		return m.renderDetail(width, height)
	default:
		return ""
	}
}

func (m tuiModel) renderMenu(width, height int) string {
	header := renderHeader(width, "bin", "Main menu", "? Help")
	bodyHeight := maxInt(8, height-4)
	panel := tuiPanelStyle.Width(maxInt(20, width-4)).Height(bodyHeight).Render(m.renderMenuBody(maxInt(16, width-8), maxInt(6, bodyHeight-2)))
	footer := renderFooter(width, "1 item", "", renderKeymap("j/k", "Move", "enter", "Open", "q", "Quit"))
	return joinScreenParts(width, height, header, panel, footer)
}

func (m tuiModel) renderMenuBody(width, height int) string {
	contentWidth := width
	if contentWidth > 72 {
		contentWidth = 72
	}
	if contentWidth < 20 {
		contentWidth = 20
	}

	lines := []string{
		tuiTitleStyle.Render("MENU"),
		"",
	}

	for i, item := range m.menuItems {
		row := item.Title
		if i == m.menuCursor {
			row = tuiSelectionStyle.Width(contentWidth).Render(row)
		} else {
			row = tuiAccentStyle.Render(row)
		}
		lines = append(lines, row)
		lines = append(lines, wrapStyledText(item.Description, contentWidth, tuiMutedStyle)...)
		lines = append(lines, "")
	}

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, strings.Join(lines, "\n"))
}

func (m tuiModel) renderTargets(width, height int) string {
	header := renderHeader(width, "bin changelog", "Select package", "? Help")
	bodyHeight := maxInt(8, height-4)
	panelWidth := width - 10
	if panelWidth > 86 {
		panelWidth = 86
	}
	if panelWidth < 32 {
		panelWidth = width - 4
	}
	panel := tuiPanelStyle.Width(maxInt(28, panelWidth)).Height(bodyHeight).Render(m.renderTargetsBody(maxInt(24, panelWidth-4), maxInt(6, bodyHeight-2)))
	footerText := renderKeymap("j/k", "Navigate", "enter", "View", "q", "Back")
	if m.screen == tuiScreenTargetsLoading {
		footerText = "Loading outdated packages..."
	}
	footer := renderFooter(width, "", fmt.Sprintf("%d packages", len(m.targets)), footerText)
	return joinScreenParts(width, height, header, lipgloss.PlaceHorizontal(width, lipgloss.Center, panel), footer)
}

func (m tuiModel) renderTargetsBody(width, height int) string {
	if m.screen == tuiScreenTargetsLoading {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, tuiMutedStyle.Render("Loading outdated packages..."))
	}

	if m.targetsNotice != "" {
		lines := []string{tuiWarningStyle.Render("Unable to load outdated packages"), ""}
		lines = append(lines, wrapStyledText(m.targetsNotice, width, tuiMutedStyle)...)
		return fitLines(lines, height)
	}

	if len(m.targets) == 0 {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, strings.Join([]string{
			tuiTitleStyle.Render("All binaries are up to date."),
			"",
			tuiMutedStyle.Render("No changelog entries are available because there are no outdated packages."),
		}, "\n"))
	}

	lines := []string{
		tuiTitleStyle.Render("OUTDATED PACKAGES"),
		tuiMutedStyle.Render(fmt.Sprintf("Showing %d packages", len(m.targets))),
		"",
	}

	for i, target := range m.targets {
		lines = append(lines, renderTargetCard(target, i == m.targetCursor, true, width))
		lines = append(lines, "")
	}

	return fitLines(lines, height)
}

func (m tuiModel) renderDetail(width, height int) string {
	title := "changelog"
	status := "Loading release history"
	if target, ok := m.selectedTarget(); ok {
		status = tuiAccentStyle.Render(target.Name) + "  |  " + tuiMutedStyle.Render(target.CurrentVersion+" -> "+target.LatestVersion)
		if m.detailLoading {
			status += tuiMutedStyle.Render("  loading...")
		}
	}
	header := renderHeader(width, title, status, "? Help")
	bodyHeight := maxInt(8, height-4)
	body := m.renderDetailBody(width, bodyHeight)
	footerStatus := "Loading"
	footerHelp := renderKeymap("h/l", "Focus", "j/k", "Move", "g/G", "Top/Bottom", "q", "Menu")
	if m.detail != nil {
		count := len(m.detail.Releases)
		index := 0
		if count > 0 {
			index = m.detail.SelectedIndex + 1
		}
		footerStatus = fmt.Sprintf("%s  |  %d packages  |  update %d of %d", focusLabel(m.detailFocus), len(m.targets), index, count)
	}
	if m.detailLoading {
		footerHelp = renderKeymap("h/l", "Focus", "j/k", "Move", "q", "Menu")
	}
	footer := renderFooter(width, "", footerStatus, footerHelp)
	return joinScreenParts(width, height, header, body, footer)
}

func (m tuiModel) renderDetailBody(width, height int) string {
	if m.detail == nil {
		return tuiShellStyle.Width(maxInt(20, width-4)).Height(maxInt(6, height)).Render(lipgloss.Place(maxInt(16, width-6), maxInt(4, height-2), lipgloss.Center, lipgloss.Center, tuiMutedStyle.Render("Loading release history...")))
	}

	shellWidth := maxInt(40, width-4)
	shellHeight := maxInt(6, height)
	innerWidth := maxInt(36, shellWidth-2)
	innerHeight := maxInt(4, shellHeight-2)

	leftWidth := innerWidth / 5
	if leftWidth < 22 {
		leftWidth = 22
	}
	if leftWidth > 28 {
		leftWidth = 28
	}
	middleWidth := innerWidth / 5
	if middleWidth < 22 {
		middleWidth = 22
	}
	if middleWidth > 28 {
		middleWidth = 28
	}
	rightWidth := maxInt(32, innerWidth-leftWidth-middleWidth-2)

	left := tuiColumnStyle.Width(leftWidth).Height(innerHeight).Render(m.renderTargetList(maxInt(16, leftWidth-2), innerHeight))
	middle := tuiColumnStyle.Width(middleWidth).Height(innerHeight).Render(m.renderReleaseList(maxInt(16, middleWidth-2), innerHeight))
	right := tuiColumnStyle.Width(rightWidth).Height(innerHeight).Render(m.renderReleaseBody(maxInt(24, rightWidth-2), innerHeight))
	content := lipgloss.JoinHorizontal(lipgloss.Top, left, renderVerticalDivider(innerHeight), middle, renderVerticalDivider(innerHeight), right)

	return lipgloss.PlaceHorizontal(width, lipgloss.Center, tuiShellStyle.Width(shellWidth).Height(shellHeight).Render(content))
}

func (m tuiModel) renderTargetList(width, height int) string {
	lines := []string{
		renderColumnTitle("BINS", m.detailFocus == tuiFocusTargets),
		tuiMutedStyle.Render(fmt.Sprintf("Showing %d packages", len(m.targets))),
		"",
	}

	for i, target := range m.targets {
		lines = append(lines, renderTargetCard(target, i == m.targetCursor, m.detailFocus == tuiFocusTargets && i == m.targetCursor, width))
		lines = append(lines, "")
	}

	return fitLines(lines, height)
}

func (m tuiModel) renderReleaseList(width, height int) string {
	lines := []string{
		renderColumnTitle("VERSIONS", m.detailFocus == tuiFocusVersions),
		tuiMutedStyle.Render(fmt.Sprintf("Showing %d updates", len(m.detail.Releases))),
		"",
	}

	if m.detailLoading {
		lines = append(lines, tuiMutedStyle.Render("Loading versions..."))
		lines = append(lines, "", renderReleaseLegend("Installed", m.detail.Target.CurrentVersion))
		lines = append(lines, renderReleaseLegend("Latest", m.detail.Target.LatestVersion))
		return fitLines(lines, height)
	}

	for i, release := range m.detail.Releases {
		lines = append(lines, renderReleaseRailItem(release, i == m.detail.SelectedIndex, m.detailFocus == tuiFocusVersions && i == m.detail.SelectedIndex, width))
		lines = append(lines, "")
	}

	lines = append(lines, tuiDividerStyle.Render(strings.Repeat("-", maxInt(8, width))))
	lines = append(lines, "")
	lines = append(lines, renderReleaseLegend("Installed", m.detail.Target.CurrentVersion))
	lines = append(lines, renderReleaseLegend("Latest", m.detail.Target.LatestVersion))

	if m.detail.Notice != "" {
		lines = append(lines, "", tuiUnavailableStyle.Render(m.detail.Notice))
	}

	return fitLines(lines, height)
}

func (m tuiModel) renderReleaseBody(width, height int) string {
	lines := []string{
		renderColumnTitle("CHANGELOG", false),
		"",
	}

	if m.detailLoading {
		lines = append(lines,
			tuiLabelStyle.Render(m.detail.Target.Name),
			tuiMutedStyle.Render(m.detail.Target.Binary.Path),
			"",
			tuiMutedStyle.Render("Loading release notes..."),
		)
		return fitLines(lines, height)
	}

	if len(m.detail.Releases) == 0 {
		lines = append(lines, tuiLabelStyle.Render(m.detail.Target.Name))
		lines = append(lines, tuiMutedStyle.Render(m.detail.Target.Binary.Path), "")
		lines = append(lines, wrapStyledText(m.detail.Notice, width, tuiUnavailableStyle)...)
		return fitLines(lines, height)
	}

	release := m.detail.Releases[m.detail.SelectedIndex]
	summary, notes := splitReleaseBody(release.Body)
	if summary == "" {
		summary = release.Summary
	}
	lines = append(lines,
		tuiLabelStyle.Render(m.detail.Target.Name),
		tuiMutedStyle.Render(m.detail.Target.Binary.Path),
		"",
		renderReleaseTitle(release),
		"",
	)
	if summary != "" {
		lines = append(lines, wrapStyledText(summary, width, tuiSummaryStyle)...)
		lines = append(lines, "")
	}
	lines = append(lines, tuiDividerStyle.Render(strings.Repeat("-", maxInt(4, width))), "")
	bodyLines := renderMarkdownBody(notes, width)
	lines = append(lines, bodyLines...)
	if release.URL != "" {
		lines = append(lines, "", tuiMutedStyle.Render("Source: "+release.URL))
	}
	return fitLines(lines, height)
}

func loadOutdatedTargetsCmd(newProvider providerFactory) tea.Cmd {
	return func() tea.Msg {
		targets, err := loadOutdatedChangelogTargets(newProvider)
		return outdatedLoadedMsg{targets: targets, err: err}
	}
}

func loadChangelogDetailCmd(target changelogTarget, newProvider providerFactory) tea.Cmd {
	return func() tea.Msg {
		detail, err := loadChangelogDetail(target, newProvider)
		if err != nil {
			detail = buildUnavailableDetail(target, err)
		}
		return detailLoadedMsg{targetKey: changelogTargetKey(target), detail: detail}
	}
}

func loadOutdatedChangelogTargets(newProvider providerFactory) ([]changelogTarget, error) {
	cfg := config.Get()
	updates, _, err := collectAvailableUpdates(cfg.Bins, newProvider, false, defaultUpdateParallelism)
	if err != nil {
		return nil, err
	}

	targets := make([]changelogTarget, 0, len(updates))
	for _, update := range updates {
		provider, err := newProvider(update.binary.URL, update.binary.Provider)
		if err != nil {
			return nil, err
		}

		_, supportsHistory := provider.(providers.ReleaseHistoryProvider)
		targets = append(targets, changelogTarget{
			Binary:          update.binary,
			Name:            filepath.Base(update.binary.Path),
			CurrentVersion:  update.binary.Version,
			LatestVersion:   update.info.version,
			LatestURL:       update.info.url,
			ProviderID:      provider.GetID(),
			SupportsHistory: supportsHistory,
		})
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Binary.Path < targets[j].Binary.Path
	})

	return targets, nil
}

func loadChangelogDetail(target changelogTarget, newProvider providerFactory) (*changelogDetail, error) {
	provider, err := newProvider(target.Binary.URL, target.Binary.Provider)
	if err != nil {
		return nil, err
	}

	history, err := providers.GetReleaseHistory(provider, tuiReleaseHistoryLimit)
	if err != nil {
		if errors.Is(err, providers.ErrReleaseHistoryUnsupported) {
			return buildUnavailableDetail(target, err), nil
		}
		return nil, err
	}

	filtered := filterChangelogReleases(history, target.CurrentVersion, target.LatestVersion)
	if len(filtered) == 0 {
		return &changelogDetail{
			Target: target,
			Notice: "No release notes were found between the installed and latest versions.",
		}, nil
	}

	releases := make([]changelogRelease, 0, len(filtered))
	for _, release := range filtered {
		body := strings.TrimSpace(release.Body)
		if body == "" {
			body = "No release notes were published for this version."
		}
		releases = append(releases, changelogRelease{
			Version:     release.Version,
			URL:         release.URL,
			PublishedAt: release.PublishedAt,
			Body:        body,
			Summary:     summarizeReleaseBody(body),
		})
	}

	return &changelogDetail{
		Target:        target,
		Releases:      releases,
		SelectedIndex: 0,
	}, nil
}

func buildUnavailableDetail(target changelogTarget, err error) *changelogDetail {
	notice := fmt.Sprintf("Unable to show release history for provider %q.", target.ProviderID)
	if err != nil {
		notice = fmt.Sprintf("%s\n\n%s", notice, err.Error())
	}

	return &changelogDetail{
		Target: target,
		Notice: notice,
	}
}

func filterChangelogReleases(releases []*providers.ReleaseInfo, currentVersion, latestVersion string) []*providers.ReleaseInfo {
	if len(releases) == 0 {
		return nil
	}

	startIndex := -1
	for i, release := range releases {
		if release != nil && release.Version == latestVersion {
			startIndex = i
			break
		}
	}
	if startIndex >= 0 {
		filtered := make([]*providers.ReleaseInfo, 0, len(releases))
		for i := startIndex; i < len(releases); i++ {
			release := releases[i]
			if release == nil {
				continue
			}
			if release.Version == currentVersion {
				break
			}
			filtered = append(filtered, release)
		}
		reverseReleaseInfos(filtered)
		if len(filtered) > 0 {
			return filtered
		}
	}

	currentSemver, currentErr := version.NewVersion(currentVersion)
	latestSemver, latestErr := version.NewVersion(latestVersion)
	if currentErr != nil || latestErr != nil {
		return nil
	}

	type semverRelease struct {
		release *providers.ReleaseInfo
		semver  *version.Version
	}

	filtered := make([]semverRelease, 0, len(releases))
	for _, release := range releases {
		if release == nil {
			continue
		}
		releaseSemver, err := version.NewVersion(release.Version)
		if err != nil {
			continue
		}
		if releaseSemver.GreaterThan(currentSemver) && !releaseSemver.GreaterThan(latestSemver) {
			filtered = append(filtered, semverRelease{release: release, semver: releaseSemver})
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].semver.LessThan(filtered[j].semver)
	})

	out := make([]*providers.ReleaseInfo, 0, len(filtered))
	for _, release := range filtered {
		out = append(out, release.release)
	}

	return out
}

func reverseReleaseInfos(releases []*providers.ReleaseInfo) {
	for i, j := 0, len(releases)-1; i < j; i, j = i+1, j-1 {
		releases[i], releases[j] = releases[j], releases[i]
	}
}

func renderHeader(width int, left, center, right string) string {
	leftWidth := lipgloss.Width(left) + 2
	rightWidth := lipgloss.Width(right) + 2
	centerWidth := width - leftWidth - rightWidth
	if centerWidth < 0 {
		centerWidth = 0
	}
	return padRight(tuiTitleStyle.Render(left), leftWidth) + centerText(center, centerWidth) + padLeft(renderHelpLabel(right), rightWidth)
}

func renderFooter(width int, left, center, right string) string {
	leftWidth := lipgloss.Width(left) + 2
	rightWidth := lipgloss.Width(right) + 2
	centerWidth := width - leftWidth - rightWidth
	if centerWidth < 0 {
		centerWidth = 0
	}
	return padRight(tuiMutedStyle.Render(left), leftWidth) + centerText(center, centerWidth) + padLeft(right, rightWidth)
}

func joinScreenParts(width, height int, header, body, footer string) string {
	lines := []string{header, tuiDividerStyle.Render(strings.Repeat("-", maxInt(4, width))), body, tuiDividerStyle.Render(strings.Repeat("-", maxInt(4, width))), footer}
	return fitLines(lines, height)
}

func renderReleaseTitle(release changelogRelease) string {
	parts := []string{tuiAccentStyle.Render(release.Version)}
	if date := formatReleaseDate(release.PublishedAt); date != "" {
		parts = append(parts, tuiMutedStyle.Render(date))
	}
	return strings.Join(parts, "  ")
}

func formatReleaseDate(ts *time.Time) string {
	if ts == nil {
		return ""
	}
	return ts.Format("2006-01-02")
}

func changelogTargetKey(target changelogTarget) string {
	if target.Binary != nil && target.Binary.Path != "" {
		return target.Binary.Path
	}
	return target.Name + "|" + target.CurrentVersion + "|" + target.LatestVersion
}

func summarizeReleaseBody(body string) string {
	clean := strings.ReplaceAll(body, "\r", "")
	for _, line := range strings.Split(clean, "\n") {
		trimmed := sanitizeMarkdownLine(line)
		if trimmed != "" {
			wrapped := wrapText(trimmed, 34)
			if len(wrapped) >= 2 {
				return wrapped[0] + " " + wrapped[1]
			}
			return wrapped[0]
		}
	}
	return "No release summary available."
}

func splitReleaseBody(body string) (string, string) {
	clean := strings.ReplaceAll(body, "\r", "")
	if strings.TrimSpace(clean) == "" {
		return "", ""
	}

	lines := strings.Split(clean, "\n")
	firstIndex := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			firstIndex = i
			break
		}
	}
	if firstIndex < 0 {
		return "", clean
	}

	firstLine := strings.TrimSpace(lines[firstIndex])
	if isReleaseSectionHeading(firstLine) || isBulletLine(firstLine) {
		return "", clean
	}

	summary := sanitizeMarkdownLine(firstLine)
	notes := strings.Join(lines[firstIndex+1:], "\n")
	return summary, strings.TrimSpace(notes)
}

func renderMarkdownBody(body string, width int) []string {
	clean := strings.TrimSpace(strings.ReplaceAll(body, "\r", ""))
	if clean == "" {
		return []string{tuiMutedStyle.Render("No release notes were published for this version.")}
	}

	source := []byte(clean)
	doc := goldmark.DefaultParser().Parse(text.NewReader(source))
	lines := trimMarkdownLines(renderMarkdownBlocks(doc, source, maxInt(1, width)))
	if len(lines) == 0 {
		return wrapReleaseBody(clean, width)
	}
	return lines
}

func renderMarkdownBlocks(parent goldast.Node, source []byte, width int) []string {
	lines := []string{}
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		rendered := trimMarkdownLines(renderMarkdownBlock(child, source, width))
		if len(rendered) == 0 {
			continue
		}
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, rendered...)
	}
	return lines
}

func renderMarkdownBlock(node goldast.Node, source []byte, width int) []string {
	switch n := node.(type) {
	case *goldast.Heading:
		label := strings.TrimSpace(renderMarkdownPlainInline(node, source))
		if label == "" {
			return nil
		}
		return []string{renderReleaseHeadingLabel(label)}
	case *goldast.Paragraph:
		content := strings.TrimSpace(renderMarkdownInline(node, source))
		if content == "" {
			return nil
		}
		return wrapText(content, width)
	case *goldast.TextBlock:
		content := strings.TrimSpace(renderMarkdownInline(node, source))
		if content == "" {
			return nil
		}
		return wrapText(content, width)
	case *goldast.Blockquote:
		childLines := trimMarkdownLines(renderMarkdownBlocks(node, source, maxInt(1, width-2)))
		if len(childLines) == 0 {
			return nil
		}
		prefix := tuiMarkdownQuoteStyle.Render("| ")
		return prefixMarkdownLines(childLines, prefix, prefix)
	case *goldast.List:
		return renderMarkdownList(n, source, width)
	case *goldast.FencedCodeBlock:
		return renderMarkdownCodeBlock(string(n.Lines().Value(source)), width)
	case *goldast.CodeBlock:
		return renderMarkdownCodeBlock(string(n.Lines().Value(source)), width)
	case *goldast.ThematicBreak:
		return []string{tuiDividerStyle.Render(strings.Repeat("-", maxInt(3, width)))}
	default:
		if node.FirstChild() != nil {
			return renderMarkdownBlocks(node, source, width)
		}
	}
	return nil
}

func renderMarkdownList(list *goldast.List, source []byte, width int) []string {
	lines := []string{}
	index := list.Start
	if index <= 0 {
		index = 1
	}
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		marker := "- "
		if list.IsOrdered() {
			marker = fmt.Sprintf("%d. ", index)
			index++
		}
		rendered := renderMarkdownListItem(item, source, width, marker)
		if len(rendered) == 0 {
			continue
		}
		if len(lines) > 0 && !list.IsTight {
			lines = append(lines, "")
		}
		lines = append(lines, rendered...)
	}
	return lines
}

func renderMarkdownListItem(item goldast.Node, source []byte, width int, marker string) []string {
	contentWidth := maxInt(4, width-lipgloss.Width(marker))
	lines := []string{}
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		rendered := trimMarkdownLines(renderMarkdownBlock(child, source, contentWidth))
		if len(rendered) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, rendered...)
	}
	if len(lines) == 0 {
		return []string{marker}
	}
	return prefixMarkdownLines(lines, marker, strings.Repeat(" ", lipgloss.Width(marker)))
}

func renderMarkdownCodeBlock(body string, width int) []string {
	clean := strings.TrimRight(strings.ReplaceAll(body, "\r", ""), "\n")
	if clean == "" {
		return nil
	}

	lines := []string{}
	for _, line := range strings.Split(clean, "\n") {
		wrapped := wrapText(line, width)
		if len(wrapped) == 0 {
			lines = append(lines, tuiMarkdownCodeStyle.Render(""))
			continue
		}
		for _, part := range wrapped {
			lines = append(lines, tuiMarkdownCodeStyle.Render(part))
		}
	}
	return lines
}

func renderMarkdownInline(parent goldast.Node, source []byte) string {
	var out strings.Builder
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		out.WriteString(renderMarkdownInlineNode(child, source))
	}
	return out.String()
}

func renderMarkdownInlineNode(node goldast.Node, source []byte) string {
	switch n := node.(type) {
	case *goldast.Text:
		text := string(n.Value(source))
		if n.HardLineBreak() {
			return text + "\n"
		}
		if n.SoftLineBreak() {
			return text + " "
		}
		return text
	case *goldast.String:
		return string(n.Value)
	case *goldast.CodeSpan:
		content := strings.TrimSpace(renderMarkdownPlainInline(node, source))
		if content == "" {
			return ""
		}
		return tuiMarkdownCodeStyle.Render(content)
	case *goldast.Emphasis:
		content := renderMarkdownInline(node, source)
		if strings.TrimSpace(content) == "" {
			return content
		}
		style := lipgloss.NewStyle().Italic(true)
		if n.Level > 1 {
			style = style.Bold(true)
		}
		return style.Render(content)
	case *goldast.Link:
		label := strings.TrimSpace(renderMarkdownInline(node, source))
		destination := string(n.Destination)
		if label == "" {
			label = destination
		}
		if label == "" {
			return ""
		}
		rendered := tuiMarkdownLinkStyle.Render(label)
		if destination == "" || label == destination {
			return rendered
		}
		return rendered + tuiMutedStyle.Render(" ("+destination+")")
	case *goldast.AutoLink:
		return tuiMarkdownLinkStyle.Render(string(n.URL(source)))
	case *goldast.Image:
		label := strings.TrimSpace(renderMarkdownPlainInline(node, source))
		if label == "" {
			label = string(n.Destination)
		}
		if label == "" {
			return tuiMutedStyle.Render("[image]")
		}
		return tuiMutedStyle.Render("[image] ") + tuiMarkdownLinkStyle.Render(label)
	default:
		if node.FirstChild() != nil {
			return renderMarkdownInline(node, source)
		}
	}
	return ""
}

func renderMarkdownPlainInline(parent goldast.Node, source []byte) string {
	var out strings.Builder
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		out.WriteString(renderMarkdownPlainInlineNode(child, source))
	}
	return out.String()
}

func renderMarkdownPlainInlineNode(node goldast.Node, source []byte) string {
	switch n := node.(type) {
	case *goldast.Text:
		text := string(n.Value(source))
		if n.HardLineBreak() {
			return text + "\n"
		}
		if n.SoftLineBreak() {
			return text + " "
		}
		return text
	case *goldast.String:
		return string(n.Value)
	case *goldast.CodeSpan, *goldast.Emphasis:
		return renderMarkdownPlainInline(node, source)
	case *goldast.Link:
		label := strings.TrimSpace(renderMarkdownPlainInline(node, source))
		if label != "" {
			return label
		}
		return string(n.Destination)
	case *goldast.AutoLink:
		return string(n.URL(source))
	case *goldast.Image:
		label := strings.TrimSpace(renderMarkdownPlainInline(node, source))
		if label != "" {
			return label
		}
		return string(n.Destination)
	default:
		if node.FirstChild() != nil {
			return renderMarkdownPlainInline(node, source)
		}
	}
	return ""
}

func prefixMarkdownLines(lines []string, firstPrefix, nextPrefix string) []string {
	if len(lines) == 0 {
		return nil
	}

	out := make([]string, 0, len(lines))
	usedFirst := false
	for _, line := range lines {
		if line == "" {
			out = append(out, "")
			continue
		}
		prefix := nextPrefix
		if !usedFirst {
			prefix = firstPrefix
			usedFirst = true
		}
		out = append(out, prefix+line)
	}
	if !usedFirst {
		return []string{firstPrefix}
	}
	return out
}

func trimMarkdownLines(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}

func wrapReleaseBody(body string, width int) []string {
	clean := strings.ReplaceAll(body, "\r", "")
	if strings.TrimSpace(clean) == "" {
		return []string{tuiMutedStyle.Render("No release notes were published for this version.")}
	}

	lines := []string{}
	for _, paragraph := range strings.Split(clean, "\n") {
		trimmed := strings.TrimSpace(paragraph)
		if trimmed == "" {
			lines = append(lines, "")
			continue
		}
		if isReleaseSectionHeading(trimmed) {
			lines = append(lines, renderReleaseHeading(trimmed))
			lines = append(lines, "")
			continue
		}
		if isBulletLine(trimmed) {
			bulletWidth := maxInt(4, width-2)
			wrapped := wrapText(trimBulletMarker(trimmed), bulletWidth)
			for i, line := range wrapped {
				prefix := "  "
				if i == 0 {
					prefix = "- "
				}
				lines = append(lines, prefix+line)
			}
			continue
		}

		wrapped := wrapText(sanitizeMarkdownLine(trimmed), width)
		for _, line := range wrapped {
			lines = append(lines, line)
		}
	}
	return lines
}

func wrapStyledText(text string, width int, style lipgloss.Style) []string {
	wrapped := wrapText(text, width)
	if len(wrapped) == 0 {
		return nil
	}
	lines := make([]string, 0, len(wrapped))
	for _, line := range wrapped {
		lines = append(lines, style.Render(line))
	}
	return lines
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	paragraphs := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")
	out := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			out = append(out, "")
			continue
		}

		words := strings.Fields(paragraph)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}

		line := words[0]
		for _, word := range words[1:] {
			candidate := line + " " + word
			if lipgloss.Width(candidate) <= width {
				line = candidate
				continue
			}
			out = append(out, line)
			line = word
		}
		out = append(out, line)
	}
	return out
}

func sanitizeMarkdownLine(line string) string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "### ")
	trimmed = strings.TrimPrefix(trimmed, "## ")
	trimmed = strings.TrimPrefix(trimmed, "# ")
	trimmed = strings.TrimPrefix(trimmed, "- ")
	trimmed = strings.TrimPrefix(trimmed, "* ")
	trimmed = strings.TrimPrefix(trimmed, "+ ")
	return strings.TrimSpace(trimmed)
}

func styleForReleaseLine(line string) lipgloss.Style {
	clean := strings.ToLower(sanitizeMarkdownLine(line))
	if strings.HasPrefix(clean, "added") {
		return tuiAddedStyle
	}
	if strings.HasPrefix(clean, "changed") {
		return tuiChangedStyle
	}
	if strings.HasPrefix(clean, "fixed") {
		return tuiFixedStyle
	}
	if strings.HasPrefix(clean, "improved") || strings.HasPrefix(clean, "improvement") {
		return tuiImprovedStyle
	}
	if strings.HasPrefix(strings.TrimSpace(line), "#") {
		return tuiHeadingStyle
	}
	return lipgloss.NewStyle()
}

func renderReleaseHeading(line string) string {
	label := sanitizeMarkdownLine(line)
	return renderReleaseHeadingLabel(label)
}

func renderReleaseHeadingLabel(label string) string {
	clean := strings.ToLower(label)
	switch {
	case strings.HasPrefix(clean, "added"):
		return tuiAddedStyle.Render("+ " + label)
	case strings.HasPrefix(clean, "changed"):
		return tuiChangedStyle.Render("~ " + label)
	case strings.HasPrefix(clean, "fixed"):
		return tuiFixedStyle.Render("! " + label)
	case strings.HasPrefix(clean, "improved"), strings.HasPrefix(clean, "improvement"):
		return tuiImprovedStyle.Render("^ " + label)
	default:
		return tuiHeadingStyle.Render(label)
	}
}

func isReleaseSectionHeading(line string) bool {
	clean := strings.ToLower(sanitizeMarkdownLine(line))
	return strings.HasPrefix(clean, "added") ||
		strings.HasPrefix(clean, "changed") ||
		strings.HasPrefix(clean, "fixed") ||
		strings.HasPrefix(clean, "improved") ||
		strings.HasPrefix(strings.TrimSpace(line), "#")
}

func isBulletLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ")
}

func trimBulletMarker(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
		return strings.TrimSpace(trimmed[2:])
	}
	return trimmed
}

func fitLines(lines []string, height int) string {
	flattened := make([]string, 0, len(lines))
	for _, line := range lines {
		flattened = append(flattened, strings.Split(line, "\n")...)
	}

	if len(flattened) > height {
		if height > 0 {
			flattened = flattened[:height]
			flattened[height-1] = tuiMutedStyle.Render("...")
		}
	}
	for len(flattened) < height {
		flattened = append(flattened, "")
	}
	return strings.Join(flattened, "\n")
}

func renderReleaseRailItem(release changelogRelease, selected, active bool, width int) string {
	date := formatReleaseDate(release.PublishedAt)
	header := spaceBetween("o "+truncateText(release.Version, maxInt(8, width-12)), date, width)
	summaryWidth := maxInt(12, width-4)
	summaryLines := wrapStyledText(release.Summary, summaryWidth, tuiMutedStyle)
	entryLines := []string{header}
	entryLines = append(entryLines, summaryLines...)
	entry := strings.Join(entryLines, "\n")
	if selected {
		if !active {
			return tuiPassiveCardStyle.Width(width).Render(entry)
		}
		return tuiSelectedCardStyle.Width(width).Render(entry)
	}
	return tuiAccentStyle.Render(header) + "\n" + strings.Join(summaryLines, "\n")
}

func renderTargetCard(target changelogTarget, selected, active bool, width int) string {
	status := target.CurrentVersion + " -> " + target.LatestVersion
	statusStyle := tuiSuccessStyle
	if !target.SupportsHistory {
		statusStyle = tuiUnavailableStyle
		status += "  no history"
	}

	lines := []string{
		truncateText(target.Name, width),
		statusStyle.Render(status),
	}
	if selected {
		lines = append(lines, tuiMutedStyle.Render(truncateText(target.Binary.Path, width)))
	}

	content := strings.Join(lines, "\n")
	if selected {
		if !active {
			return tuiPassiveCardStyle.Width(width).Render(content)
		}
		return tuiSelectedCardStyle.Width(width).Render(content)
	}
	return tuiAccentStyle.Render(truncateText(target.Name, width)) + "\n" + statusStyle.Render(truncateText(status, width))
}

func renderColumnTitle(label string, active bool) string {
	if active {
		return tuiKeyStyle.Render("● ") + tuiTitleStyle.Render(label)
	}
	return tuiTitleStyle.Render(label)
}

func focusLabel(focus tuiColumnFocus) string {
	if focus == tuiFocusTargets {
		return "Focus: bins"
	}
	return "Focus: versions"
}

func renderReleaseLegend(label, version string) string {
	return tuiMutedStyle.Render("o ") + label + " (" + version + ")"
}

func renderVerticalDivider(height int) string {
	lines := make([]string, 0, height)
	for i := 0; i < height; i++ {
		lines = append(lines, tuiDividerStyle.Render("│"))
	}
	return strings.Join(lines, "\n")
}

func renderHelpLabel(text string) string {
	if strings.HasPrefix(text, "?") {
		return tuiKeyStyle.Render("?") + tuiMutedStyle.Render(strings.TrimPrefix(text, "?"))
	}
	return tuiMutedStyle.Render(text)
}

func renderKeymap(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	segments := make([]string, 0, len(parts)/2)
	for i := 0; i+1 < len(parts); i += 2 {
		segments = append(segments, tuiKeyStyle.Render(parts[i])+" "+tuiMutedStyle.Render(parts[i+1]))
	}
	return strings.Join(segments, "   ")
}

func spaceBetween(left, right string, width int) string {
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	if right == "" || leftWidth+rightWidth+1 >= width {
		if right == "" {
			return left
		}
		return left + " " + right
	}
	return left + strings.Repeat(" ", width-leftWidth-rightWidth) + right
}

func truncateText(text string, width int) string {
	if width <= 0 || lipgloss.Width(text) <= width {
		return text
	}
	if width <= 3 {
		return text[:width]
	}
	return text[:width-3] + "..."
}

func padRight(text string, width int) string {
	textWidth := lipgloss.Width(text)
	if width <= textWidth {
		return text
	}
	return text + strings.Repeat(" ", width-textWidth)
}

func padLeft(text string, width int) string {
	textWidth := lipgloss.Width(text)
	if width <= textWidth {
		return text
	}
	return strings.Repeat(" ", width-textWidth) + text
}

func centerText(text string, width int) string {
	textWidth := lipgloss.Width(text)
	if width <= textWidth {
		return text
	}
	left := (width - textWidth) / 2
	right := width - textWidth - left
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
