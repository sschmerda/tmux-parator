package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sschmerda/tmux-parator/internal/config"
	"github.com/sschmerda/tmux-parator/internal/discovery"
	"github.com/sschmerda/tmux-parator/internal/fuzzy"
	"github.com/sschmerda/tmux-parator/internal/pathsearch"
	"github.com/sschmerda/tmux-parator/internal/theme"
	"github.com/sschmerda/tmux-parator/internal/tmux"
)

type sessionClient interface {
	ListSessions(context.Context) ([]tmux.Session, error)
	SwitchSession(context.Context, string) error
	SwitchLastSession(context.Context) error
	KillSession(context.Context, string) error
	NewSession(context.Context, string, string, tmux.SessionMetadata) error
	TagSession(context.Context, string, tmux.SessionMetadata) error
}

type mode int

const (
	modeBrowse mode = iota
	modeConfirmKill
	modeCommands
	modeHelp
	modeCreateSession
	modePathSearch
)

type confirmChoice int

const (
	confirmCancel confirmChoice = iota
	confirmYes
)

type Model struct {
	client               sessionClient
	roots                []config.Root
	discovery            discovery.Options
	pathConfig           pathsearchConfig
	glyphs               config.Glyphs
	glyphColors          config.GlyphColors
	columns              config.Columns
	sessions             []tmux.Session
	rootItems            []discovery.Candidate
	candidates           []candidate
	filtered             []candidate
	pathItems            []candidate
	pathResult           []candidate
	pathCompletions      []candidate
	filter               string
	commandInput         string
	pathInput            string
	pathRoot             string
	createText           string
	cursor               int
	scroll               int
	filterRestoreCursor  int
	filterRestoreScroll  int
	filterRestoreActive  bool
	pathCursor           int
	pathScroll           int
	helpCursor           int
	helpScroll           int
	commandCursor        int
	commandScroll        int
	confirmChoice        confirmChoice
	pathCompletionCursor int
	pathCompletionInput  string
	pathCompletionRoot   string
	pathCompletionQuery  string
	pathStream           <-chan pathsearch.Batch
	pathCancel           context.CancelFunc
	width                int
	height               int
	err                  error
	rootErr              error
	pathErr              error
	mode                 mode
	previousMode         mode
	commandPreviousMode  mode
	loading              bool
	pathBusy             bool
	styles               styles
}

type pathsearchConfig struct {
	enabled bool
	roots   []string
	options pathsearch.Options
}

type sessionsLoadedMsg struct {
	sessions []tmux.Session
	err      error
}

type switchedMsg struct {
	err error
}

type killedMsg struct {
	err error
}

type createdMsg struct {
	name string
	err  error
}

type rootsLoadedMsg struct {
	roots []discovery.Candidate
	err   error
}

type pathBatchMsg struct {
	root   string
	stream <-chan pathsearch.Batch
	batch  pathsearch.Batch
}

type pathCompletionsLoadedMsg struct {
	root        string
	query       string
	input       string
	completions []pathsearch.Candidate
	fallback    pathsearch.Candidate
	direction   int
	err         error
}

type styles struct {
	root          lipgloss.Style
	title         lipgloss.Style
	session       lipgloss.Style
	selected      lipgloss.Style
	chip          lipgloss.Style
	selectedChip  lipgloss.Style
	glyph         lipgloss.Style
	match         lipgloss.Style
	selectedMatch lipgloss.Style
	search        lipgloss.Style
	searchPrompt  lipgloss.Style
	searchBox     lipgloss.Style
	appFrame      lipgloss.Style
	popupFrame    lipgloss.Style
	popupBody     lipgloss.Style
	popupMuted    lipgloss.Style
	popupAccent   lipgloss.Style
	statusShow    lipgloss.Style
	statusSkip    lipgloss.Style
	muted         lipgloss.Style
	error         lipgloss.Style
	warn          lipgloss.Style
	filterLabel   lipgloss.Style
	filterValue   lipgloss.Style
}

func NewModel(client sessionClient, activeTheme theme.Theme, roots []config.Root, discoveryOptions discovery.Options, pathSearch config.PathSearch, glyphs config.Glyphs, glyphColors config.GlyphColors, columns config.Columns) Model {
	glyphs = normalizeUIGlyphs(glyphs)
	glyphColors = normalizeUIGlyphColors(glyphColors)
	columns = normalizeUIColumns(columns)
	return Model{
		client:      client,
		roots:       roots,
		discovery:   discoveryOptions,
		glyphs:      glyphs,
		glyphColors: glyphColors,
		columns:     columns,
		pathConfig: pathsearchConfig{
			enabled: pathSearch.Enabled,
			roots:   pathSearch.Roots,
			options: pathsearch.Options{
				Backend:        pathSearch.Backend,
				MaxDepth:       pathSearch.MaxDepth,
				SkipHidden:     pathSearch.SkipHidden,
				SkipGitignored: pathSearch.SkipGitignored,
				SkipDirs:       pathSearch.SkipDirs,
				Limit:          pathSearch.Limit,
			},
		},
		loading: true,
		styles:  newStyles(activeTheme),
	}
}

func newStyles(activeTheme theme.Theme) styles {
	background := lipgloss.Color(activeTheme.Background)
	selectedBackground := lipgloss.Color(activeTheme.SelectedBG)
	modalBackground := lipgloss.Color(activeTheme.SearchBG)
	return styles{
		root:          lipgloss.NewStyle().Foreground(lipgloss.Color(activeTheme.Query)).Background(background),
		title:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(activeTheme.Title)).Background(background),
		session:       lipgloss.NewStyle().Foreground(lipgloss.Color(activeTheme.Query)).Background(background),
		selected:      lipgloss.NewStyle().Foreground(lipgloss.Color(activeTheme.SelectedFG)).Background(selectedBackground).Bold(true),
		chip:          lipgloss.NewStyle().Foreground(lipgloss.Color(activeTheme.Chip)).Background(lipgloss.Color(activeTheme.ChipBG)).Bold(true),
		selectedChip:  lipgloss.NewStyle().Foreground(lipgloss.Color(activeTheme.SelectedChip)).Background(lipgloss.Color(activeTheme.SelectedChipBG)).Bold(true),
		glyph:         lipgloss.NewStyle().Foreground(lipgloss.Color(activeTheme.Glyph)).Background(selectedBackground),
		match:         lipgloss.NewStyle().Foreground(lipgloss.Color(activeTheme.MatchFG)).Background(background).Bold(true),
		selectedMatch: lipgloss.NewStyle().Foreground(lipgloss.Color(activeTheme.SelectedMatchFG)).Background(selectedBackground).Bold(true),
		search:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(activeTheme.SearchFG)).Background(lipgloss.Color(activeTheme.SearchBG)),
		searchPrompt:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(activeTheme.Glyph)).Background(lipgloss.Color(activeTheme.SearchBG)),
		searchBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(activeTheme.PromptBorder)).
			BorderBackground(background).
			Background(lipgloss.Color(activeTheme.SearchBG)).
			Padding(0, 1),
		appFrame: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(activeTheme.PaletteBorder)).
			BorderBackground(background).
			Background(background),
		popupFrame: lipgloss.NewStyle().
			Foreground(lipgloss.Color(activeTheme.Header)).
			Background(modalBackground).
			Bold(true),
		popupBody: lipgloss.NewStyle().
			Foreground(lipgloss.Color(activeTheme.SearchFG)).
			Background(modalBackground),
		popupMuted: lipgloss.NewStyle().
			Foreground(lipgloss.Color(activeTheme.Muted)).
			Background(modalBackground),
		popupAccent: lipgloss.NewStyle().
			Foreground(lipgloss.Color(activeTheme.Header)).
			Background(modalBackground).
			Bold(true),
		statusShow: lipgloss.NewStyle().
			Bold(true).
			Foreground(background).
			Background(lipgloss.Color(activeTheme.Header)).
			Padding(0, 1),
		statusSkip: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(activeTheme.Muted)).
			Background(lipgloss.Color(activeTheme.ChipBG)).
			Padding(0, 1),
		muted:       lipgloss.NewStyle().Foreground(lipgloss.Color(activeTheme.Muted)).Background(background),
		error:       lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b")).Background(background),
		warn:        lipgloss.NewStyle().Foreground(lipgloss.Color(activeTheme.Header)).Background(background).Bold(true),
		filterLabel: lipgloss.NewStyle().Foreground(lipgloss.Color(activeTheme.Prompt)).Background(background).Bold(true),
		filterValue: lipgloss.NewStyle().Foreground(lipgloss.Color(activeTheme.Query)).Background(background),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadSessions(), m.loadRoots())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case sessionsLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.sessions = msg.sessions
			m.rebuildCandidates()
			m.applyFilter()
		}
		return m, nil
	case switchedMsg:
		m.err = msg.err
		if msg.err == nil {
			return m, tea.Quit
		}
		return m, nil
	case killedMsg:
		m.loading = false
		m.mode = modeBrowse
		m.err = msg.err
		if msg.err != nil {
			return m, nil
		}
		return m, m.loadSessions()
	case createdMsg:
		m.loading = false
		m.mode = modeBrowse
		m.err = msg.err
		if msg.err != nil {
			if tmux.IsDuplicateSessionError(msg.err) {
				m.err = nil
				return m, tea.Batch(m.loadSessions(), m.switchSession(msg.name))
			}
			return m, nil
		}
		return m, tea.Batch(m.loadSessions(), m.switchSession(msg.name))
	case rootsLoadedMsg:
		m.loading = false
		m.rootErr = msg.err
		if msg.err == nil {
			m.rootItems = msg.roots
			m.rebuildCandidates()
			m.applyFilter()
		}
		return m, nil
	case pathBatchMsg:
		pathSearchVisible := m.mode == modePathSearch || (m.mode == modeHelp && m.previousMode == modePathSearch)
		if !pathSearchVisible || msg.root != m.pathRoot || msg.stream != m.pathStream {
			return m, nil
		}
		if len(msg.batch.Candidates) > 0 {
			m.pathItems = append(m.pathItems, candidatesFromPaths(msg.batch.Candidates)...)
			m.applyPathFilter()
		}
		if msg.batch.Done {
			m.pathBusy = false
			m.pathErr = msg.batch.Err
			m.pathStream = nil
			m.pathCancel = nil
			return m, nil
		}
		return m, m.waitPathBatch(msg.root, m.pathStream)
	case pathCompletionsLoadedMsg:
		if m.mode != modePathSearch || msg.root != m.pathRoot || msg.query != m.pathQuery() || msg.input != m.pathInput {
			return m, nil
		}
		m.pathErr = msg.err
		if msg.err != nil {
			m.clearPathCompletion()
			return m, nil
		}
		m.pathCompletions = candidatesFromPaths(msg.completions)
		if len(m.pathCompletions) == 0 {
			m.clearPathCompletion()
			if msg.fallback.Path != "" {
				m.pathRoot = msg.fallback.Path
				m.pathInput = displayPathInput(msg.fallback.Path) + "/"
				m.pathCursor = 0
				m.pathScroll = 0
				m.pathBusy = true
				return m, m.startPathStream(m.pathRoot)
			}
			return m, nil
		}
		m.pathCompletionInput = msg.input
		m.pathCompletionRoot = msg.root
		m.pathCompletionQuery = msg.query
		m.pathCompletionCursor = -1
		m.advancePathCompletion(msg.direction)
		return m, m.startPathStreamPreserveCompletion(m.pathRoot)
	case tea.KeyMsg:
		return m.updateKey(msg)
	}

	return m, nil
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		if m.clearVisibleError() {
			return m, nil
		}
	}
	if m.mode == modeCommands {
		return m.updateCommandKey(msg)
	}
	if m.mode == modeHelp {
		switch msg.String() {
		case "esc", "?":
			m.mode = m.previousMode
		case "up", "k":
			if m.helpCursor > 0 {
				m.helpCursor--
				m.ensureHelpCursorVisible()
			}
		case "down", "j":
			if m.helpCursor < len(helpItemsForMode(m.previousMode))-1 {
				m.helpCursor++
				m.ensureHelpCursorVisible()
			}
		}
		return m, nil
	}
	if m.mode == modeCreateSession {
		return m.updateCreateKey(msg)
	}
	if m.mode == modePathSearch {
		return m.updatePathSearchKey(msg)
	}
	if m.mode == modeConfirmKill {
		switch msg.String() {
		case "y", "Y":
			return m.confirmKill()
		case "n", "N", "esc":
			m.mode = modeBrowse
			return m, nil
		case "left", "up", "shift+tab":
			m.confirmChoice = confirmCancel
		case "right", "down", "tab":
			m.confirmChoice = confirmYes
		case "enter":
			if m.confirmChoice == confirmYes {
				return m.confirmKill()
			}
			m.mode = modeBrowse
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "ctrl+g":
		m.openCommands(m.mode)
	case "ctrl+@":
		return m, m.switchLastSession()
	case "?":
		m.openHelp(m.mode)
	case "ctrl+r":
		m.loading = true
		return m, tea.Batch(m.loadSessions(), m.loadRoots())
	case "ctrl+n":
		m.mode = modeCreateSession
		m.createText = ""
	case "ctrl+t":
		return m.openPathSearch()
	case "alt+h", "meta+h":
		m.toggleDiscoveryHidden()
		m.loading = true
		return m, m.loadRoots()
	case "alt+i", "meta+i":
		m.toggleDiscoveryGitignored()
		m.loading = true
		return m, m.loadRoots()
	case "up":
		if m.cursor > 0 {
			m.cursor--
			m.ensureCursorVisible()
		}
	case "down":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.ensureCursorVisible()
		}
	case "tab":
		m.jumpBrowseSection(1)
	case "shift+tab":
		m.jumpBrowseSection(-1)
	case "enter":
		selected, ok := m.selected()
		if !ok {
			return m, nil
		}
		return m, m.openCandidate(selected)
	case "ctrl+k":
		if selected, ok := m.selected(); ok && selected.kind == candidateSession {
			m.mode = modeConfirmKill
			m.confirmChoice = confirmCancel
		}
	case "backspace", "ctrl+h":
		if m.filter != "" {
			m.removeBrowseFilterRune()
		}
	default:
		if len(msg.String()) == 1 && !msg.Alt {
			m.addBrowseFilterText(msg.String())
		}
	}

	return m, nil
}

func (m Model) updateCommandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.commandMatches()
	switch msg.String() {
	case "esc", "ctrl+g":
		m.mode = m.commandPreviousMode
	case "?":
		m.openHelp(m.mode)
	case "up":
		if m.commandCursor > 0 {
			m.commandCursor--
			m.ensureCommandCursorVisible()
		}
	case "down":
		if m.commandCursor < len(items)-1 {
			m.commandCursor++
			m.ensureCommandCursorVisible()
		}
	case "enter":
		if len(items) == 0 || m.commandCursor < 0 || m.commandCursor >= len(items) {
			return m, nil
		}
		return m.runCommand(items[m.commandCursor].item)
	case "backspace", "ctrl+h":
		if m.commandInput != "" {
			runes := []rune(m.commandInput)
			m.commandInput = string(runes[:len(runes)-1])
			m.clampCommandCursor()
		}
	default:
		if len(msg.Runes) > 0 && !msg.Alt {
			m.commandInput += string(msg.Runes)
			m.clampCommandCursor()
		}
	}
	return m, nil
}

func (m Model) updateCreateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeBrowse
	case "enter":
		name := strings.TrimSpace(m.createText)
		if name == "" {
			return m, nil
		}
		wd, err := os.Getwd()
		if err != nil {
			m.err = fmt.Errorf("resolve current path: %w", err)
			m.mode = modeBrowse
			return m, nil
		}
		m.loading = true
		m.mode = modeBrowse
		return m, m.createSessionWithMetadata(name, "", tmux.SessionMetadata{
			CreatedByParator: true,
			Kind:             "path",
			Path:             wd,
			Glyph:            m.glyphs.Path,
			GlyphColor:       m.glyphColors.Path,
		})
	case "backspace", "ctrl+h":
		if m.createText != "" {
			runes := []rune(m.createText)
			m.createText = string(runes[:len(runes)-1])
		}
	default:
		if len(msg.Runes) > 0 {
			m.createText += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) updatePathSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+t":
		m.stopPathStream()
		m.mode = modeBrowse
		return m, nil
	case "esc":
		m.stopPathStream()
		m.mode = modeBrowse
		return m, nil
	case "ctrl+g":
		m.openCommands(modePathSearch)
		return m, nil
	case "ctrl+@":
		m.stopPathStream()
		m.mode = modeBrowse
		return m, m.switchLastSession()
	case "?":
		m.openHelp(modePathSearch)
		return m, nil
	case "ctrl+r":
		m.pathBusy = true
		m.clearPathCompletion()
		return m, m.startPathStream(m.pathRoot)
	case "ctrl+o":
		m.cyclePathRoot()
		return m, m.startPathStream(m.pathRoot)
	case "alt+h", "meta+h":
		m.pathConfig.options.SkipHidden = !m.pathConfig.options.SkipHidden
		m.pathBusy = true
		m.clearPathCompletion()
		return m, m.startPathStream(m.pathRoot)
	case "alt+i", "meta+i":
		m.pathConfig.options.SkipGitignored = !m.pathConfig.options.SkipGitignored
		m.pathBusy = true
		m.clearPathCompletion()
		return m, m.startPathStream(m.pathRoot)
	case "ctrl+p":
		selected, ok := m.typedPathCandidate()
		if !ok {
			m.pathErr = errPathInputUnavailable(m.pathInput)
			return m, nil
		}
		m.loading = true
		m.stopPathStream()
		m.mode = modeBrowse
		return m, m.openCandidate(selected)
	case "tab":
		if m.hasPathCompletionCycle() {
			m.advancePathCompletion(1)
			return m, m.startPathStreamPreserveCompletion(m.pathRoot)
		}
		return m, m.loadPathCompletions(1)
	case "shift+tab":
		if m.hasPathCompletionCycle() {
			m.advancePathCompletion(-1)
			return m, m.startPathStreamPreserveCompletion(m.pathRoot)
		}
		return m, m.loadPathCompletions(-1)
	case "right":
		if m.hasPathCompletionCycle() {
			m.clearPathCompletion()
		}
	case "left":
		if m.hasPathCompletionCycle() {
			m.clearPathCompletion()
		}
	case "up":
		if m.pathCursor > 0 {
			m.pathCursor--
			m.ensurePathCursorVisible()
		}
	case "down":
		if m.pathCursor < len(m.pathResult)-1 {
			m.pathCursor++
			m.ensurePathCursorVisible()
		}
	case "enter":
		selected, ok := m.selectedPath()
		if !ok {
			return m, nil
		}
		m.loading = true
		m.stopPathStream()
		m.mode = modeBrowse
		return m, m.openCandidate(selected)
	case "backspace", "ctrl+h":
		if m.pathInput != "" {
			m.pathInput = m.pathInput[:len(m.pathInput)-1]
			m.clearPathCompletion()
			return m, m.applyPathInputChange()
		}
	default:
		if len(msg.String()) == 1 && !msg.Alt {
			m.pathInput += msg.String()
			m.clearPathCompletion()
			return m, m.applyPathInputChange()
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.showsPathSearchBase() {
		return m.pathSearchView()
	}

	var b strings.Builder
	s := m.styles

	b.WriteString(renderInsetSearchBox("❯ ", m.filter, m.innerWidth(), s))
	b.WriteString("\n\n")

	if !m.loading && len(m.sessions) == 0 && len(m.rootItems) == 0 && m.err == nil && m.rootErr == nil {
		b.WriteString(s.muted.Render("no tmux sessions or configured root candidates found"))
		b.WriteString("\n")
	}

	limit := m.listLimit()
	columns := m.renderColumns(m.filtered)
	contentWidth := m.contentWidth()
	rowsUsed := 0
	renderedCandidates := 0
	if hasVisibleBrowseColumns(columns) && rowsUsed < limit {
		b.WriteString(renderInsetRow(renderColumnOffsetRow(renderBrowseColumnHeader(s, contentWidth-searchBoxInnerOffset, columns), s), m.innerWidth(), s))
		b.WriteString("\n")
		rowsUsed++
	}
	if m.showTopOverflow(limit) && rowsUsed < limit {
		b.WriteString(renderInsetRow(s.muted.Render("..."), m.innerWidth(), s))
		b.WriteString("\n")
		rowsUsed++
	}
	for i := m.scroll; i < len(m.filtered); i++ {
		item := m.filtered[i]
		if rowsUsed >= limit {
			break
		}
		if m.sectionHeaderBefore(i) {
			b.WriteString(renderInsetDividerRow(renderBrowseSectionHeader(m.sectionTitleForIndex(i), browseSectionRank(item) == m.currentBrowseSectionRank(), s, contentWidth+1), m.innerWidth(), s))
			b.WriteString("\n")
			rowsUsed++
			if rowsUsed >= limit {
				break
			}
		}
		b.WriteString(renderInsetRow(m.renderCandidateRow(item, i == m.cursor, contentWidth, columns), m.innerWidth(), s))
		b.WriteString("\n")
		rowsUsed++
		renderedCandidates++
	}

	if m.scroll+renderedCandidates < len(m.filtered) && rowsUsed < limit {
		b.WriteString(renderInsetRow(s.muted.Render("..."), m.innerWidth(), s))
		b.WriteString("\n")
	}

	appendAnchoredFooter(&b, renderInsetRow(renderFooterAlignedRow(renderStatusFooter(false, m.discovery.SkipHidden, m.discovery.SkipGitignored, helpText, contentWidth-searchBoxInnerOffset, s), s), m.innerWidth(), s), m.innerHeight())
	return m.renderWithOverlay(b.String())
}

func (m Model) pathSearchView() string {
	var b strings.Builder
	s := m.styles

	b.WriteString(renderInsetSearchBox("path ❯ ", m.pathInput, m.innerWidth(), s))
	b.WriteString("\n\n")

	if !m.pathBusy && len(m.pathResult) == 0 && m.pathErr == nil {
		b.WriteString(s.muted.Render("no directories found"))
		b.WriteString("\n")
	}

	limit := m.pathListLimit()
	columns := m.renderColumns(m.pathResult)
	contentWidth := m.contentWidth()
	rowsUsed := 0
	renderedCandidates := 0
	if hasVisibleBrowseColumns(columns) && rowsUsed < limit {
		b.WriteString(renderInsetRow(renderColumnOffsetRow(renderBrowseColumnHeader(s, contentWidth-searchBoxInnerOffset, columns), s), m.innerWidth(), s))
		b.WriteString("\n")
		rowsUsed++
	}
	if rowsUsed < limit {
		b.WriteString(renderInsetDividerRow(renderBrowseSectionHeader("paths", true, s, contentWidth+1), m.innerWidth(), s))
		b.WriteString("\n")
		rowsUsed++
	}
	if m.showPathTopOverflow(limit) && rowsUsed < limit {
		b.WriteString(renderInsetRow(s.muted.Render("..."), m.innerWidth(), s))
		b.WriteString("\n")
		rowsUsed++
	}
	for i := m.pathScroll; i < len(m.pathResult); i++ {
		if rowsUsed >= limit {
			break
		}
		item := m.pathResult[i]
		b.WriteString(renderInsetRow(m.renderCandidateRow(item, i == m.pathCursor, contentWidth, columns), m.innerWidth(), s))
		b.WriteString("\n")
		rowsUsed++
		renderedCandidates++
	}
	if m.pathScroll+renderedCandidates < len(m.pathResult) && rowsUsed < limit {
		b.WriteString(renderInsetRow(s.muted.Render("..."), m.innerWidth(), s))
		b.WriteString("\n")
	}

	footerHelp := pathSearchHelpText
	if m.pathErr != nil {
		footerHelp = "error: " + m.pathErr.Error() + " | " + pathSearchHelpText
	}
	appendAnchoredFooter(&b, renderInsetRow(renderFooterAlignedRow(renderStatusFooter(true, m.pathConfig.options.SkipHidden, m.pathConfig.options.SkipGitignored, footerHelp, contentWidth-searchBoxInnerOffset, s), s), m.innerWidth(), s), m.innerHeight())
	return m.renderWithOverlay(b.String())
}

func appendAnchoredFooter(b *strings.Builder, footer string, height int) {
	content := strings.TrimRight(b.String(), "\n")
	b.Reset()
	b.WriteString(content)
	if height <= 0 {
		b.WriteString("\n")
		b.WriteString(footer)
		return
	}
	currentHeight := lipgloss.Height(content)
	if currentHeight == 0 && b.Len() == 0 {
		currentHeight = 0
	}
	blankLines := height - 1 - currentHeight
	if blankLines < 1 {
		blankLines = 1
	}
	b.WriteString(strings.Repeat("\n", blankLines))
	b.WriteString(footer)
}

func (m *Model) toggleDiscoveryHidden() {
	next := !m.discovery.SkipHidden
	m.discovery.SkipHidden = next
	for i := range m.roots {
		m.roots[i].SkipHidden = next
	}
}

func (m *Model) toggleDiscoveryGitignored() {
	next := !m.discovery.SkipGitignored
	m.discovery.SkipGitignored = next
	for i := range m.roots {
		m.roots[i].SkipGitignored = next
	}
}

func (m Model) renderWithOverlay(content string) string {
	innerWidth := m.innerWidth()
	innerHeight := m.innerHeight()
	base := m.styles.root.Width(innerWidth).Height(innerHeight).Render(content)
	if m.mode == modeConfirmKill {
		base = placeCenteredOverlay(base, renderConfirmKill(m.styles, m.confirmKillSessionName(), m.confirmChoice, innerWidth), innerWidth, innerHeight)
	}
	if m.mode == modeCreateSession {
		base = placeCenteredOverlay(base, renderCreateSession(m.styles, m.createText, innerWidth), innerWidth, innerHeight)
	}
	if m.mode == modeCommands || (m.mode == modeHelp && m.previousMode == modeCommands) {
		base = placeOverlay(base, renderCommandPalette(m.styles, m.commandMatches(), m.commandInput, m.commandCursor, m.commandScroll, innerWidth, innerHeight), innerWidth, innerHeight)
	}
	if m.mode == modeHelp {
		base = placeOverlay(base, renderHelpPanel(m.styles, m.previousMode, m.helpCursor, m.helpScroll, innerWidth, innerHeight), innerWidth, innerHeight)
	}
	if message, ok := m.errorMessage(); ok {
		base = placeCenteredOverlay(base, renderErrorPopup(m.styles, message, innerWidth), innerWidth, innerHeight)
	}
	return m.styles.appFrame.Width(innerWidth).Height(innerHeight).Render(base)
}

func placeOverlay(base string, overlay string, width int, height int) string {
	if width <= 0 || height <= 0 {
		return overlay
	}
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	overlayWidth := lipgloss.Width(overlay)
	overlayHeight := len(overlayLines)
	x := (width - overlayWidth) / 2
	if x < 0 {
		x = 0
	}
	y := helpOverlayTopMargin(height)
	if y+overlayHeight > height {
		overlayHeight = height - y
	}
	if overlayHeight < 1 {
		return base
	}
	for i := 0; i < overlayHeight; i++ {
		targetLine := padLineToWidth(baseLines[y+i], width)
		overlayLine := overlayLines[i]
		overlayLineWidth := lipgloss.Width(overlayLine)
		rightStart := x + overlayLineWidth
		if rightStart > width {
			rightStart = width
		}
		baseLines[y+i] = ansi.Cut(targetLine, 0, x) + overlayLine + ansi.Cut(targetLine, rightStart, width)
	}
	if len(baseLines) > height {
		baseLines = baseLines[:height]
	}
	return strings.Join(baseLines, "\n")
}

func placeCenteredOverlay(base string, overlay string, width int, height int) string {
	if width <= 0 || height <= 0 {
		return overlay
	}
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	overlayWidth := lipgloss.Width(overlay)
	overlayHeight := len(overlayLines)
	x := (width - overlayWidth) / 2
	if x < 0 {
		x = 0
	}
	y := (height - overlayHeight) / 2
	if y < 0 {
		y = 0
	}
	if y+overlayHeight > height {
		overlayHeight = height - y
	}
	if overlayHeight < 1 {
		return base
	}
	for i := 0; i < overlayHeight; i++ {
		targetLine := padLineToWidth(baseLines[y+i], width)
		overlayLine := overlayLines[i]
		overlayLineWidth := lipgloss.Width(overlayLine)
		rightStart := x + overlayLineWidth
		if rightStart > width {
			rightStart = width
		}
		baseLines[y+i] = ansi.Cut(targetLine, 0, x) + overlayLine + ansi.Cut(targetLine, rightStart, width)
	}
	if len(baseLines) > height {
		baseLines = baseLines[:height]
	}
	return strings.Join(baseLines, "\n")
}

func helpOverlayTopMargin(height int) int {
	if height < 12 {
		return 0
	}
	return 1
}

func padLineToWidth(line string, width int) string {
	lineWidth := lipgloss.Width(line)
	if lineWidth >= width {
		return line
	}
	return line + strings.Repeat(" ", width-lineWidth)
}

func (m *Model) rebuildCandidates() {
	m.candidates = candidatesFromSessions(m.sessions, m.sessionOrigins(), m.rootItems)
	m.candidates = append(m.candidates, candidatesFromRootsWithPathDetail(m.rootItems, true)...)
}

func (m *Model) applyFilter() {
	if strings.TrimSpace(m.filter) == "" {
		m.filtered = m.candidates
		m.clampCursor()
		return
	}

	fuzzyCandidates := make([]fuzzy.Candidate, 0, len(m.candidates))
	for _, item := range m.candidates {
		fuzzyCandidates = append(fuzzyCandidates, item.fuzzyCandidate())
	}

	matches := fuzzy.Filter(fuzzyCandidates, m.filter)
	sortMainMatches(matches)
	filtered := make([]candidate, 0, len(matches))
	for _, match := range matches {
		item, ok := match.Candidate.Value.(candidate)
		if !ok {
			continue
		}
		item.matchIndexes = match.TitleIndexes
		item.fieldIndexes = match.FieldIndexes
		filtered = append(filtered, item)
	}
	m.filtered = filtered
	m.clampCursor()
}

func (m *Model) addBrowseFilterText(value string) {
	if m.filter == "" && !m.filterRestoreActive {
		m.filterRestoreCursor = m.cursor
		m.filterRestoreScroll = m.scroll
		m.filterRestoreActive = true
	}
	m.filter += value
	m.applyFilter()
	m.cursor = 0
	m.scroll = 0
	m.clampCursor()
}

func (m *Model) removeBrowseFilterRune() {
	if m.filter == "" {
		return
	}
	runes := []rune(m.filter)
	m.filter = string(runes[:len(runes)-1])
	m.applyFilter()
	if m.filter == "" {
		if m.filterRestoreActive {
			m.cursor = m.filterRestoreCursor
			m.scroll = m.filterRestoreScroll
			m.filterRestoreActive = false
		}
		m.clampCursor()
		m.ensureCursorVisible()
		return
	}
	m.cursor = 0
	m.scroll = 0
	m.clampCursor()
}

func sortMainMatches(matches []fuzzy.Match) {
	sort.SliceStable(matches, func(i, j int) bool {
		left, leftOK := matches[i].Candidate.Value.(candidate)
		right, rightOK := matches[j].Candidate.Value.(candidate)
		if !leftOK || !rightOK {
			return matches[i].Score > matches[j].Score
		}
		leftRank := candidateGroupRank(left)
		rightRank := candidateGroupRank(right)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		leftTitle := strings.ToLower(left.title())
		rightTitle := strings.ToLower(right.title())
		if leftTitle != rightTitle {
			return leftTitle < rightTitle
		}
		return strings.ToLower(left.detail()) < strings.ToLower(right.detail())
	})
}

func candidateGroupRank(item candidate) int {
	switch item.kind {
	case candidateSession:
		return 0
	default:
		return 1
	}
}

func (m *Model) applyPathFilter() {
	query := m.pathQuery()
	if strings.TrimSpace(query) == "" {
		m.pathResult = m.pathItems
		m.clampPathCursor()
		return
	}

	fuzzyCandidates := make([]fuzzy.Candidate, 0, len(m.pathItems))
	for _, item := range m.pathItems {
		item.pathDetail = pathSearchMatchPath(item.path(), m.pathRoot)
		fuzzyCandidates = append(fuzzyCandidates, item.fuzzyCandidate())
	}

	matches := fuzzy.Filter(fuzzyCandidates, query)
	sortPathMatches(matches, m.pathRoot, query)
	filtered := make([]candidate, 0, len(matches))
	for _, match := range matches {
		item, ok := match.Candidate.Value.(candidate)
		if !ok {
			continue
		}
		item.matchIndexes = match.TitleIndexes
		item.fieldIndexes = match.FieldIndexes
		filtered = append(filtered, item)
	}
	m.pathResult = filtered
	m.clampPathCursor()
}

func (m *Model) applyPathInputChange() tea.Cmd {
	root, _ := parsePathInput(m.pathInput, m.defaultPathRoot())
	m.pathErr = nil
	if root != m.pathRoot {
		m.pathRoot = root
		m.pathCursor = 0
		m.pathScroll = 0
		m.pathBusy = true
		return m.startPathStream(root)
	}
	m.applyPathFilter()
	return nil
}

func (m Model) pathQuery() string {
	_, query := parsePathInput(m.pathInput, m.defaultPathRoot())
	return query
}

func pathSearchMatchPath(path string, root string) string {
	rootPath, err := pathsearch.ExpandRoot(root)
	if err != nil {
		return filepath.ToSlash(path)
	}
	relativePath, err := filepath.Rel(rootPath, path)
	if err != nil || relativePath == "." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || relativePath == ".." {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relativePath)
}

func (m Model) defaultPathRoot() string {
	if strings.TrimSpace(m.pathRoot) != "" {
		return m.pathRoot
	}
	if len(m.pathConfig.roots) > 0 && strings.TrimSpace(m.pathConfig.roots[0]) != "" {
		return m.pathConfig.roots[0]
	}
	return "~"
}

func parsePathInput(input string, fallbackRoot string) (string, string) {
	if input == "" {
		return fallbackRoot, ""
	}
	if input == "/" {
		return "/", ""
	}
	if strings.HasSuffix(input, "/") {
		root := strings.TrimSuffix(input, "/")
		if root == "" {
			root = "/"
		}
		return root, ""
	}
	separator := strings.LastIndex(input, "/")
	if separator >= 0 {
		root := input[:separator]
		if root == "" {
			root = "/"
		}
		return root, input[separator+1:]
	}
	return fallbackRoot, input
}

func rootPrompt(root string) string {
	switch root {
	case "~", ".", "..":
		return root + "/"
	default:
		return root
	}
}

func displayPathInput(path string) string {
	home, err := os.UserHomeDir()
	if err == nil {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+string(filepath.Separator)) {
			return "~/" + filepath.ToSlash(strings.TrimPrefix(path, home+string(filepath.Separator)))
		}
	}
	return filepath.ToSlash(path)
}

func (m Model) typedPathCandidate() (candidate, bool) {
	path := strings.TrimSpace(m.pathInput)
	if path == "" {
		return candidate{}, false
	}
	expanded, err := pathsearch.ExpandRoot(path)
	if err != nil {
		return candidate{}, false
	}
	info, err := os.Stat(expanded)
	if err != nil || !info.IsDir() {
		return candidate{}, false
	}
	return candidate{kind: candidatePath, fsPath: pathsearch.Candidate{Name: filepath.Base(expanded), Path: expanded}}, true
}

func errPathInputUnavailable(input string) error {
	if strings.TrimSpace(input) == "" {
		return fmt.Errorf("typed path is empty")
	}
	return fmt.Errorf("typed path is not an available directory: %s", input)
}

func sortPathMatches(matches []fuzzy.Match, root string, query string) {
	rootPath, err := pathsearch.ExpandRoot(root)
	if err != nil {
		rootPath = root
	}
	sort.SliceStable(matches, func(i, j int) bool {
		left, leftOK := matches[i].Candidate.Value.(candidate)
		right, rightOK := matches[j].Candidate.Value.(candidate)
		if !leftOK || !rightOK || left.kind != candidatePath || right.kind != candidatePath {
			return matches[i].Score > matches[j].Score
		}
		leftLiteralRank := pathLiteralRank(left, query)
		rightLiteralRank := pathLiteralRank(right, query)
		if leftLiteralRank != rightLiteralRank {
			return leftLiteralRank > rightLiteralRank
		}
		leftTitleMatch := len(matches[i].TitleIndexes) > 0
		rightTitleMatch := len(matches[j].TitleIndexes) > 0
		if leftTitleMatch != rightTitleMatch {
			return leftTitleMatch
		}
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		leftDepth := relativePathDepth(rootPath, left.path())
		rightDepth := relativePathDepth(rootPath, right.path())
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		leftName := strings.ToLower(left.title())
		rightName := strings.ToLower(right.title())
		if leftName != rightName {
			return leftName < rightName
		}
		return strings.ToLower(left.path()) < strings.ToLower(right.path())
	})
}

func pathLiteralRank(item candidate, query string) int {
	searchValues := []string{item.title()}
	if item.pathDetail != "" {
		searchValues = append(searchValues, item.pathDetail)
		for _, component := range strings.Split(filepath.ToSlash(item.pathDetail), "/") {
			if component != "" {
				searchValues = append(searchValues, component)
			}
		}
	}
	rank := 0
	for _, token := range strings.Fields(strings.ToLower(query)) {
		if token == "" {
			continue
		}
		best := 0
		for _, value := range searchValues {
			if valueRank := literalTokenRank(value, token); valueRank > best {
				best = valueRank
			}
		}
		rank += best
	}
	return rank
}

func literalTokenRank(value string, token string) int {
	value = strings.ToLower(value)
	switch {
	case value == token:
		return 5
	case strings.HasPrefix(value, token):
		return 4
	case containsAfterSeparator(value, token):
		return 3
	case strings.Contains(value, token):
		return 2
	case prefixSubsequence(value, token):
		return 1
	default:
		return 0
	}
}

func prefixSubsequence(value string, token string) bool {
	if token == "" || value == "" {
		return false
	}
	valueRunes := []rune(value)
	tokenRunes := []rune(token)
	if valueRunes[0] != tokenRunes[0] {
		return false
	}
	tokenIndex := 0
	for _, r := range valueRunes {
		if r != tokenRunes[tokenIndex] {
			continue
		}
		tokenIndex++
		if tokenIndex == len(tokenRunes) {
			return true
		}
	}
	return false
}

func containsAfterSeparator(value string, token string) bool {
	offset := strings.Index(value, token)
	for offset >= 0 {
		if offset == 0 || isNameSeparator(rune(value[offset-1])) {
			return true
		}
		next := offset + len(token)
		if next >= len(value) {
			break
		}
		nextOffset := strings.Index(value[next:], token)
		if nextOffset < 0 {
			break
		}
		offset = next + nextOffset
	}
	return false
}

func isNameSeparator(r rune) bool {
	switch r {
	case '-', '_', '.', ' ', '/', '\\':
		return true
	default:
		return false
	}
}

func relativePathDepth(root string, path string) int {
	relativePath, err := filepath.Rel(root, path)
	if err != nil || relativePath == "." {
		return 0
	}
	relativePath = filepath.Clean(relativePath)
	depth := 0
	for _, part := range strings.Split(relativePath, string(filepath.Separator)) {
		if part != "" && part != "." {
			depth++
		}
	}
	return depth
}

func (m Model) loadPathCompletions(direction int) tea.Cmd {
	root := m.pathRoot
	query := m.pathQuery()
	input := m.pathInput
	options := m.pathConfig.options
	selected, hasSelected := m.selectedPath()
	var fallback pathsearch.Candidate
	if strings.TrimSpace(query) != "" && hasSelected && selected.kind == candidatePath {
		fallback = selected.fsPath
	}
	return func() tea.Msg {
		children, err := pathsearch.DirectChildren(root, options)
		if err != nil {
			return pathCompletionsLoadedMsg{root: root, query: query, input: input, fallback: fallback, direction: direction, err: err}
		}
		completions := filterPathCompletions(children, query)
		return pathCompletionsLoadedMsg{root: root, query: query, input: input, completions: completions, fallback: fallback, direction: direction}
	}
}

func filterPathCompletions(children []pathsearch.Candidate, query string) []pathsearch.Candidate {
	query = strings.TrimSpace(query)
	if query == "" {
		return children
	}
	items := candidatesFromPaths(children)
	fuzzyCandidates := make([]fuzzy.Candidate, 0, len(items))
	for _, item := range items {
		fuzzyCandidates = append(fuzzyCandidates, item.fuzzyCandidate())
	}
	matches := fuzzy.Filter(fuzzyCandidates, query)
	completions := make([]pathsearch.Candidate, 0, len(matches))
	for _, match := range matches {
		item, ok := match.Candidate.Value.(candidate)
		if ok && item.kind == candidatePath {
			completions = append(completions, item.fsPath)
		}
	}
	return completions
}

func (m *Model) clampCursor() {
	if len(m.filtered) == 0 {
		m.cursor = 0
		m.scroll = 0
		return
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.scroll >= len(m.filtered) {
		m.scroll = len(m.filtered) - 1
	}
	m.ensureCursorVisible()
}

func (m *Model) jumpBrowseSection(direction int) {
	starts := m.sectionStartIndexes()
	if len(starts) == 0 {
		return
	}
	if direction >= 0 {
		for _, start := range starts {
			if start > m.cursor {
				m.cursor = start
				m.ensureCursorVisible()
				return
			}
		}
		m.cursor = starts[0]
		m.ensureCursorVisible()
		return
	}
	for i := len(starts) - 1; i >= 0; i-- {
		if starts[i] < m.cursor {
			m.cursor = starts[i]
			m.ensureCursorVisible()
			return
		}
	}
	m.cursor = starts[len(starts)-1]
	m.ensureCursorVisible()
}

func (m Model) sectionStartIndexes() []int {
	starts := make([]int, 0, 2)
	for i := range m.filtered {
		if m.sectionHeaderBefore(i) {
			starts = append(starts, i)
		}
	}
	return starts
}

func (m *Model) clampPathCursor() {
	if len(m.pathResult) == 0 {
		m.pathCursor = 0
		m.pathScroll = 0
		return
	}
	if m.pathCursor >= len(m.pathResult) {
		m.pathCursor = len(m.pathResult) - 1
	}
	if m.pathScroll >= len(m.pathResult) {
		m.pathScroll = len(m.pathResult) - 1
	}
	m.ensurePathCursorVisible()
}

func (m Model) selected() (candidate, bool) {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return candidate{}, false
	}
	return m.filtered[m.cursor], true
}

func (m Model) selectedPath() (candidate, bool) {
	if len(m.pathResult) == 0 || m.pathCursor < 0 || m.pathCursor >= len(m.pathResult) {
		return candidate{}, false
	}
	return m.pathResult[m.pathCursor], true
}

func (m Model) confirmKillSessionName() string {
	selected, ok := m.selected()
	if !ok {
		return ""
	}
	return selected.sessionName()
}

func (m Model) confirmKill() (tea.Model, tea.Cmd) {
	selected, ok := m.selected()
	if !ok {
		m.mode = modeBrowse
		return m, nil
	}
	m.loading = true
	return m, m.killSession(selected.session.Name)
}

func (m Model) errorMessage() (string, bool) {
	if m.err != nil {
		return m.err.Error(), true
	}
	if m.rootErr != nil {
		return "load roots: " + m.rootErr.Error(), true
	}
	return "", false
}

func (m *Model) clearVisibleError() bool {
	if m.showsPathSearchBase() && m.pathErr != nil {
		m.pathErr = nil
		return true
	}
	if m.err != nil {
		m.err = nil
		return true
	}
	if m.rootErr != nil {
		m.rootErr = nil
		return true
	}
	return false
}

func (m Model) listLimit() int {
	return m.availableListRows(len(m.filtered) + m.dividerCount())
}

func (m Model) availableListRows(visibleRows int) int {
	height := m.innerHeight()
	if height <= 0 {
		return visibleRows
	}
	limit := height - 7
	if limit < 1 {
		return 1
	}
	if limit > visibleRows {
		return visibleRows
	}
	return limit
}

func (m Model) innerWidth() int {
	if m.width < 3 {
		return 80
	}
	return m.width - 2
}

func (m Model) contentWidth() int {
	width := m.innerWidth() - rowLeftInset() - rowRightInset()
	if width < 1 {
		return m.innerWidth()
	}
	return width
}

func (m Model) innerHeight() int {
	if m.height < 3 {
		return m.height
	}
	return m.height - 2
}

func (m Model) pathListLimit() int {
	return m.availableListRows(len(m.pathResult) + 2)
}

func (m *Model) ensureCursorVisible() {
	if len(m.filtered) == 0 {
		m.scroll = 0
		return
	}
	if m.cursor < m.scroll {
		m.scroll = m.cursor
		return
	}
	limit := m.listLimit()
	for m.cursorRowsFromScroll(limit) > limit && m.scroll < m.cursor {
		m.scroll++
	}
}

func (m *Model) ensurePathCursorVisible() {
	if len(m.pathResult) == 0 {
		m.pathScroll = 0
		return
	}
	if m.pathCursor < m.pathScroll {
		m.pathScroll = m.pathCursor
		return
	}
	limit := m.pathListLimit()
	for m.pathCursor-m.pathScroll+1 > limit && m.pathScroll < m.pathCursor {
		m.pathScroll++
	}
}

func (m Model) cursorRowsFromScroll(limit int) int {
	rows := 0
	for i := m.scroll; i <= m.cursor && i < len(m.filtered); i++ {
		if m.sectionHeaderBefore(i) {
			rows++
		}
		rows++
	}
	if m.showTopOverflow(limit) {
		rows++
	}
	return rows
}

func (m Model) showTopOverflow(limit int) bool {
	return m.scroll > 0 && limit > 1
}

func (m Model) showPathTopOverflow(limit int) bool {
	return m.pathScroll > 0 && limit > 1
}

func (m Model) showsPathSearchBase() bool {
	if m.mode == modePathSearch {
		return true
	}
	if m.mode == modeCommands && m.commandPreviousMode == modePathSearch {
		return true
	}
	return m.mode == modeHelp && ((m.previousMode == modePathSearch) || (m.previousMode == modeCommands && m.commandPreviousMode == modePathSearch))
}

func (m *Model) openCommands(previous mode) {
	m.commandPreviousMode = previous
	m.mode = modeCommands
	m.commandInput = ""
	m.commandCursor = 0
	m.commandScroll = 0
}

func (m *Model) ensureCommandCursorVisible() {
	limit := commandListHeight(m.height)
	if m.commandCursor < m.commandScroll {
		m.commandScroll = m.commandCursor
		return
	}
	for m.commandCursor-m.commandScroll+1 > limit && m.commandScroll < m.commandCursor {
		m.commandScroll++
	}
}

func (m *Model) clampCommandCursor() {
	items := m.commandMatches()
	if len(items) == 0 {
		m.commandCursor = 0
		m.commandScroll = 0
		return
	}
	if m.commandCursor >= len(items) {
		m.commandCursor = len(items) - 1
	}
	if m.commandCursor < 0 {
		m.commandCursor = 0
	}
	m.ensureCommandCursorVisible()
}

func (m *Model) openHelp(previous mode) {
	m.previousMode = previous
	m.mode = modeHelp
	m.helpCursor = 0
	m.helpScroll = 0
}

func (m *Model) ensureHelpCursorVisible() {
	limit := helpListHeight(m.height)
	if m.helpCursor < m.helpScroll {
		m.helpScroll = m.helpCursor
		return
	}
	for m.helpCursor-m.helpScroll+1 > limit && m.helpScroll < m.helpCursor {
		m.helpScroll++
	}
}

func (m *Model) clearPathCompletion() {
	m.pathCompletions = nil
	m.pathCompletionCursor = -1
	m.pathCompletionInput = ""
	m.pathCompletionRoot = ""
	m.pathCompletionQuery = ""
}

func (m Model) hasPathCompletionCycle() bool {
	return len(m.pathCompletions) > 0 && m.pathCompletionInput != ""
}

func (m *Model) advancePathCompletion(direction int) {
	if len(m.pathCompletions) == 0 {
		return
	}
	if m.pathCompletionCursor < 0 || m.pathCompletionCursor >= len(m.pathCompletions) {
		m.pathCompletionCursor = 0
		if direction < 0 {
			m.pathCompletionCursor = len(m.pathCompletions) - 1
		}
	} else if direction < 0 {
		m.pathCompletionCursor--
		if m.pathCompletionCursor < 0 {
			m.pathCompletionCursor = len(m.pathCompletions) - 1
		}
	} else {
		m.pathCompletionCursor++
		if m.pathCompletionCursor >= len(m.pathCompletions) {
			m.pathCompletionCursor = 0
		}
	}
	selected := m.pathCompletions[m.pathCompletionCursor]
	m.pathRoot = selected.path()
	m.pathInput = displayPathInput(selected.path()) + "/"
	m.pathCursor = 0
	m.pathScroll = 0
	m.pathBusy = true
}

func (m *Model) cyclePathRoot() {
	roots := []string{"~", "/", ".", ".."}
	next := roots[0]
	for i, root := range roots {
		if m.pathRoot == root {
			next = roots[(i+1)%len(roots)]
			break
		}
	}
	m.pathRoot = next
	m.pathInput = rootPrompt(next)
	m.pathCursor = 0
	m.pathScroll = 0
	m.pathBusy = true
	m.clearPathCompletion()
}

func (m Model) loadSessions() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.client.ListSessions(context.Background())
		return sessionsLoadedMsg{sessions: sessions, err: err}
	}
}

func (m Model) switchSession(name string) tea.Cmd {
	return func() tea.Msg {
		return switchedMsg{err: m.client.SwitchSession(context.Background(), name)}
	}
}

func (m Model) switchLastSession() tea.Cmd {
	return func() tea.Msg {
		return switchedMsg{err: m.client.SwitchLastSession(context.Background())}
	}
}

func (m Model) tagAndSwitchSession(name string, metadata tmux.SessionMetadata) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if err := m.client.TagSession(ctx, name, metadata); err != nil {
			return switchedMsg{err: err}
		}
		return switchedMsg{err: m.client.SwitchSession(ctx, name)}
	}
}

func (m Model) killSession(name string) tea.Cmd {
	return func() tea.Msg {
		return killedMsg{err: m.client.KillSession(context.Background(), name)}
	}
}

func (m Model) createSession(name string, path string) tea.Cmd {
	return m.createSessionWithMetadata(name, path, tmux.SessionMetadata{})
}

func (m Model) createSessionWithMetadata(name string, path string, metadata tmux.SessionMetadata) tea.Cmd {
	return func() tea.Msg {
		return createdMsg{name: name, err: m.client.NewSession(context.Background(), name, path, metadata)}
	}
}

func (m Model) loadRoots() tea.Cmd {
	return func() tea.Msg {
		roots, err := discovery.Discover(context.Background(), m.roots, m.discovery)
		return rootsLoadedMsg{roots: roots, err: err}
	}
}

func (m Model) waitPathBatch(root string, stream <-chan pathsearch.Batch) tea.Cmd {
	return func() tea.Msg {
		batch, ok := <-stream
		if !ok {
			return pathBatchMsg{root: root, stream: stream, batch: pathsearch.Batch{Done: true}}
		}
		return pathBatchMsg{root: root, stream: stream, batch: batch}
	}
}

func (m Model) openPathSearch() (tea.Model, tea.Cmd) {
	if !m.pathConfig.enabled {
		m.err = nil
		m.pathErr = nil
		return m, nil
	}
	root := "~"
	if len(m.pathConfig.roots) > 0 && strings.TrimSpace(m.pathConfig.roots[0]) != "" {
		root = m.pathConfig.roots[0]
	}
	m.mode = modePathSearch
	m.pathRoot = root
	m.pathInput = rootPrompt(root)
	m.pathItems = nil
	m.pathResult = nil
	m.pathCursor = 0
	m.pathScroll = 0
	m.pathErr = nil
	m.pathBusy = true
	m.clearPathCompletion()
	return m, m.startPathStream(root)
}

func (m *Model) startPathStream(root string) tea.Cmd {
	m.stopPathStream()
	ctx, cancel := context.WithCancel(context.Background())
	stream := pathsearch.Stream(ctx, root, m.pathConfig.options)
	m.pathCancel = cancel
	m.pathStream = stream
	m.pathItems = nil
	m.pathResult = nil
	m.pathCursor = 0
	m.pathScroll = 0
	m.pathErr = nil
	m.pathBusy = true
	m.clearPathCompletion()
	return m.waitPathBatch(root, stream)
}

func (m *Model) startPathStreamPreserveCompletion(root string) tea.Cmd {
	m.stopPathStream()
	ctx, cancel := context.WithCancel(context.Background())
	stream := pathsearch.Stream(ctx, root, m.pathConfig.options)
	m.pathCancel = cancel
	m.pathStream = stream
	m.pathItems = nil
	m.pathResult = nil
	m.pathCursor = 0
	m.pathScroll = 0
	m.pathErr = nil
	m.pathBusy = true
	return m.waitPathBatch(root, stream)
}

func (m *Model) stopPathStream() {
	if m.pathCancel != nil {
		m.pathCancel()
	}
	m.pathCancel = nil
	m.pathStream = nil
	m.pathBusy = false
}

func (m Model) openCandidate(selected candidate) tea.Cmd {
	if selected.kind == candidateRoot || selected.kind == candidatePath {
		if session, ok := m.sessionForPath(selected.path()); ok {
			return m.tagAndSwitchSession(session.Name, selected.sessionMetadata())
		}
		name := m.availableSessionName(selected.sessionName())
		metadata := selected.sessionMetadata()
		metadata.BaseName = selected.sessionName()
		if selected.kind == candidatePath && metadata.Glyph == "" {
			metadata.Glyph = m.glyphs.Path
		}
		if selected.kind == candidatePath && metadata.GlyphColor == "" {
			metadata.GlyphColor = m.glyphColors.Path
		}
		return m.createSessionWithMetadata(name, selected.path(), metadata)
	}
	return m.switchSession(selected.session.Name)
}

func (m Model) sessionForPath(path string) (tmux.Session, bool) {
	for _, session := range m.sessions {
		if strings.TrimSpace(session.Metadata.Path) != "" && session.Metadata.Path == path {
			return session, true
		}
	}
	return tmux.Session{}, false
}

func (m Model) availableSessionName(base string) string {
	base = sanitizeSessionName(base)
	if !m.hasSession(base) {
		return base
	}
	for index := 2; ; index++ {
		name := base + "_" + strconv.Itoa(index)
		if !m.hasSession(name) {
			return name
		}
	}
}

func (m Model) hasSession(name string) bool {
	for _, session := range m.sessions {
		if session.Name == name {
			return true
		}
	}
	return false
}

func (m Model) sessionOrigins() map[string]string {
	origins := make(map[string]string, len(m.sessions))
	for _, session := range m.sessions {
		if session.Metadata.Kind != "" {
			origins[session.Name] = originLabel(session.Metadata.Kind)
		}
	}
	return origins
}

func originLabel(mode string) string {
	switch strings.TrimSpace(mode) {
	case "repo":
		return "repo"
	case "subdir":
		return "subdir"
	case "path":
		return "path"
	case "worktree":
		return "worktree"
	case "manual":
		return "manual"
	default:
		return "manual"
	}
}

func normalizeUIGlyphs(glyphs config.Glyphs) config.Glyphs {
	if glyphs.Repo == "" {
		glyphs.Repo = "\ue702"
	}
	if glyphs.Subdir == "" {
		glyphs.Subdir = "\uf0c9"
	}
	if glyphs.Path == "" {
		glyphs.Path = "\U000f024b"
	}
	if glyphs.Worktree == "" {
		glyphs.Worktree = "\U000f0655"
	}
	if glyphs.Manual == "" {
		glyphs.Manual = "\uebc8"
	}
	return glyphs
}

func normalizeUIGlyphColors(colors config.GlyphColors) config.GlyphColors {
	if colors.Repo == "" {
		colors.Repo = "#f14e32"
	}
	if colors.Subdir == "" {
		colors.Subdir = "#7aa2f7"
	}
	if colors.Path == "" {
		colors.Path = "#7dcfff"
	}
	if colors.Worktree == "" {
		colors.Worktree = "#9ece6a"
	}
	if colors.Manual == "" {
		colors.Manual = "#a599e9"
	}
	return colors
}

func normalizeUIColumns(columns config.Columns) config.Columns {
	if columns == (config.Columns{}) {
		columns.Chip = config.Column{Show: true, Width: originChipWidth(), MaxWidth: originChipWidth()}
		columns.Root = config.Column{Show: true, Width: originChipWidth(), MaxWidth: autoRootColumnDefaultMaxWidth}
		columns.Name = config.Column{Show: true, Width: 28, MaxWidth: autoNameColumnDefaultMaxWidth}
		columns.Path = config.Column{Show: true, Width: 0, MaxWidth: 0}
		return columns
	}
	columns.Chip = normalizeUIColumn(columns.Chip, originChipWidth(), originChipWidth())
	columns.Root = normalizeUIColumn(columns.Root, originChipWidth(), autoRootColumnDefaultMaxWidth)
	columns.Name = normalizeUIColumn(columns.Name, 28, autoNameColumnDefaultMaxWidth)
	columns.Path = normalizeUIColumn(columns.Path, 0, 0)
	return columns
}

func normalizeUIColumn(column config.Column, defaultWidth int, defaultMaxWidth int) config.Column {
	if column.Width < 0 {
		column.Width = defaultWidth
	}
	if column.MaxWidth < 0 {
		column.MaxWidth = defaultMaxWidth
	}
	if column.MaxWidth == 0 && defaultMaxWidth > 0 {
		column.MaxWidth = defaultMaxWidth
	}
	return column
}

func (m Model) renderColumns(items []candidate) config.Columns {
	columns := normalizeUIColumns(m.columns)
	if columns.Chip.Show && columns.Chip.Width == 0 {
		columns.Chip.Width = originChipWidth()
	}
	if columns.Root.Show && columns.Root.Width == 0 {
		columns.Root.Width = autoRootColumnWidth(items, columns.Root.MaxWidth)
	}
	if columns.Name.Show && columns.Name.Width == 0 {
		columns.Name.Width = autoNameColumnWidth(items, columns.Name.MaxWidth)
	}
	return columns
}

func autoRootColumnWidth(items []candidate, maxWidth int) int {
	width := 0
	for _, item := range items {
		if itemWidth := lipgloss.Width(item.rootLabel()); itemWidth > width {
			width = itemWidth
		}
	}
	return clampAutoColumnWidth(width, 1, maxWidth)
}

func autoNameColumnWidth(items []candidate, maxWidth int) int {
	width := 0
	for _, item := range items {
		if itemWidth := lipgloss.Width(item.title()); itemWidth > width {
			width = itemWidth
		}
	}
	return clampAutoColumnWidth(width, 1, maxWidth)
}

func clampAutoColumnWidth(width int, minWidth int, maxWidth int) int {
	if width < minWidth {
		return minWidth
	}
	if maxWidth > 0 && width > maxWidth {
		return maxWidth
	}
	return width
}

const (
	autoRootColumnDefaultMaxWidth = 20
	autoNameColumnDefaultMaxWidth = 40
)

func (m Model) originGlyph(mode string) string {
	return originGlyph(mode, m.glyphs)
}

func originGlyph(mode string, glyphs config.Glyphs) string {
	glyphs = normalizeUIGlyphs(glyphs)
	switch strings.TrimSpace(mode) {
	case "repo":
		return glyphs.Repo
	case "subdir":
		return glyphs.Subdir
	case "path":
		return glyphs.Path
	case "worktree":
		return glyphs.Worktree
	case "manual":
		return glyphs.Manual
	default:
		return glyphs.Manual
	}
}

func originGlyphColor(mode string, selected bool, colors config.GlyphColors) lipgloss.Color {
	colors = normalizeUIGlyphColors(colors)
	if selected {
		return selectedOriginGlyphColor(mode, colors)
	}
	switch strings.TrimSpace(mode) {
	case "repo":
		return lipgloss.Color(colors.Repo)
	case "subdir":
		return lipgloss.Color(colors.Subdir)
	case "path":
		return lipgloss.Color(colors.Path)
	case "worktree":
		return lipgloss.Color(colors.Worktree)
	case "manual":
		return lipgloss.Color(colors.Manual)
	default:
		return lipgloss.Color(colors.Manual)
	}
}

func selectedOriginGlyphColor(mode string, colors config.GlyphColors) lipgloss.Color {
	colors = normalizeUIGlyphColors(colors)
	switch strings.TrimSpace(mode) {
	case "repo":
		return lipgloss.Color(colors.Repo)
	case "subdir":
		return lipgloss.Color(colors.Subdir)
	case "path":
		return lipgloss.Color(colors.Path)
	case "worktree":
		return lipgloss.Color(colors.Worktree)
	case "manual":
		return lipgloss.Color(colors.Manual)
	default:
		return lipgloss.Color(colors.Manual)
	}
}

func (m Model) sectionHeaderBefore(index int) bool {
	if index < 0 || index >= len(m.filtered) {
		return false
	}
	if index == 0 {
		return true
	}
	return browseSectionRank(m.filtered[index]) != browseSectionRank(m.filtered[index-1])
}

func (m Model) dividerBefore(index int) bool {
	return m.sectionHeaderBefore(index) && index > 0
}

func (m Model) dividerCount() int {
	return m.sectionHeaderCount()
}

func (m Model) sectionHeaderCount() int {
	count := 0
	for i := range m.filtered {
		if m.sectionHeaderBefore(i) {
			count++
		}
	}
	return count
}

func browseSectionRank(item candidate) int {
	if item.kind == candidateSession {
		return 0
	}
	return 1
}

func (m Model) currentBrowseSectionRank() int {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return -1
	}
	return browseSectionRank(m.filtered[m.cursor])
}

func (m Model) sectionTitleForIndex(index int) string {
	if index < 0 || index >= len(m.filtered) {
		return ""
	}
	if m.filtered[index].kind == candidateSession {
		return "open sessions"
	}
	return "available workspaces"
}

func renderBrowseSectionHeader(title string, active bool, s styles, width int) string {
	if strings.TrimSpace(title) == "" {
		title = "results"
	}
	labelStyle := s.muted
	lineStyle := s.muted
	labelText := "  " + title + " "
	if active {
		labelStyle = lipgloss.NewStyle().
			Foreground(s.popupAccent.GetForeground()).
			Bold(true)
		lineStyle = s.filterLabel
		labelText = "  " + title + " "
	}
	prefix := labelStyle.Render(labelText)
	lineWidth := width - lipgloss.Width(prefix)
	if lineWidth < 1 {
		lineWidth = 1
	}
	return prefix + lineStyle.Render(strings.Repeat("─", lineWidth))
}

func hasVisibleBrowseColumns(columns config.Columns) bool {
	return columns.Chip.Show || columns.Root.Show || columns.Name.Show || columns.Path.Show
}

func renderBrowseColumnHeader(s styles, width int, columns config.Columns) string {
	columns = normalizeUIColumns(columns)
	headerStyle := lipgloss.NewStyle().
		Foreground(s.popupAccent.GetForeground()).
		Background(s.popupBody.GetBackground()).
		Bold(true)
	parts := make([]string, 0, 3)
	if columns.Chip.Show {
		parts = append(parts, renderHeaderColumn("kind", headerStyle, columns.Chip.Width, lipgloss.Left))
	}
	if columns.Root.Show {
		parts = append(parts, renderHeaderColumn("root", headerStyle, columns.Root.Width, lipgloss.Left))
	}
	if columns.Name.Show {
		parts = append(parts, renderHeaderColumn("name", headerStyle, columns.Name.Width, lipgloss.Left))
	}
	renderedColumns := strings.Join(parts, s.root.Render(" "))
	if !columns.Path.Show {
		return fitRow("  "+renderedColumns, width)
	}
	pathWidth := columns.Path.Width
	if pathWidth <= 0 {
		prefixWidth := lipgloss.Width("  " + renderedColumns + "  ")
		pathWidth = width - prefixWidth
		if pathWidth < 1 {
			pathWidth = 1
		}
		if columns.Path.MaxWidth > 0 && pathWidth > columns.Path.MaxWidth {
			pathWidth = columns.Path.MaxWidth
		}
	}
	pathHeader := renderHeaderColumn("path", headerStyle, pathWidth, lipgloss.Left)
	if renderedColumns == "" {
		return fitRow("  "+pathHeader, width)
	}
	return fitRow("  "+renderedColumns+s.root.Render("  ")+pathHeader, width)
}

func renderHeaderColumn(label string, style lipgloss.Style, width int, align lipgloss.Position) string {
	label = truncateDots(label, width)
	if width <= 0 {
		return style.Render(" " + label + " ")
	}
	return style.Width(width).Align(align).Render(" " + label)
}

func (m Model) renderCandidateRow(item candidate, selected bool, width int, columns config.Columns) string {
	return renderCandidateRow(item, selected, m.styles, width, m.glyphs, m.glyphColors, columns)
}

func renderCandidate(item candidate, selected bool, s styles, glyphs config.Glyphs, glyphColors config.GlyphColors, columns config.Columns) string {
	textStyle := s.session
	detailStyle := s.muted
	highlightStyle := s.match
	chipStyle := s.chip
	chipGlyphBackground := s.chip.GetBackground()
	if selected {
		textStyle = s.selected
		detailStyle = s.selected
		highlightStyle = s.selectedMatch
		chipStyle = s.selectedChip.Background(s.selected.GetBackground())
		chipGlyphBackground = chipStyle.GetBackground()
	}
	glyphStyle := lipgloss.NewStyle().
		Foreground(candidateGlyphColor(item, selected, glyphColors)).
		Background(chipGlyphBackground)
	parts := make([]string, 0, 3)
	if columns.Chip.Show {
		parts = append(parts, renderOriginChip(candidateOrigin(item), candidateGlyph(item, glyphs), chipStyle, glyphStyle, columns.Chip.Width))
	}
	if columns.Root.Show {
		parts = append(parts, renderRootColumn(item.rootLabel(), chipStyle, highlightStyle, item.fieldIndexes[fieldRoot], columns.Root.Width))
	}
	if columns.Name.Show {
		parts = append(parts, renderNameColumn(item.title(), textStyle, highlightStyle, item.matchIndexes, columns.Name.Width))
	}
	renderedColumns := strings.Join(parts, textStyle.Render(" "))
	detail := item.detail()
	if detail == "" || !columns.Path.Show {
		return renderedColumns
	}
	if renderedColumns == "" {
		return renderPathColumn(item, detail, detailStyle, highlightStyle, columns.Path.Width)
	}
	return renderedColumns + detailStyle.Render("  ") + renderPathColumn(item, detail, detailStyle, highlightStyle, columns.Path.Width)
}

func renderMatchedDetail(item candidate, detail string, detailStyle lipgloss.Style, highlightStyle lipgloss.Style) string {
	if indexes := detailMatchIndexes(item, detail); len(indexes) > 0 {
		return renderMatchedText(detail, detailStyle, highlightStyle, indexes)
	}
	return detailStyle.Render(detail)
}

func detailMatchIndexes(item candidate, detail string) []int {
	if indexes := item.fieldIndexes[fieldCompactPath]; len(indexes) > 0 {
		return indexesWithinDetail(indexes, detail)
	}
	if item.kind != candidatePath {
		if indexes := item.fieldIndexes[fieldPath]; len(indexes) > 0 {
			return indexesWithinDetail(indexes, detail)
		}
		if indexes := item.fieldIndexes[fieldDetail]; len(indexes) > 0 {
			return indexesWithinDetail(indexes, detail)
		}
		return nil
	}
	indexes := pathTitleDetailIndexes(item, detail)
	indexes = append(indexes, pathSearchFieldDetailIndexes(item, detail)...)
	indexes = append(indexes, indexesWithinDetail(item.fieldIndexes[fieldDetail], detail)...)
	if len(indexes) == 0 {
		return nil
	}
	return uniqueSortedInts(indexes)
}

func pathTitleDetailIndexes(item candidate, detail string) []int {
	if item.kind != candidatePath || len(item.matchIndexes) == 0 {
		return nil
	}
	title := item.title()
	if title == "" {
		return nil
	}
	offset := strings.LastIndex(detail, title)
	if offset < 0 {
		return nil
	}
	indexes := make([]int, 0, len(item.matchIndexes))
	for _, index := range item.matchIndexes {
		if index < 0 || index >= len(title) {
			continue
		}
		indexes = append(indexes, offset+index)
	}
	return indexes
}

func pathSearchFieldDetailIndexes(item candidate, detail string) []int {
	indexes := item.fieldIndexes[fieldPath]
	if len(indexes) == 0 {
		return nil
	}
	searchPath := item.pathDetail
	if searchPath == "" {
		return indexesWithinDetail(indexes, detail)
	}
	detail = filepath.ToSlash(detail)
	searchPath = filepath.ToSlash(searchPath)
	offset := strings.LastIndex(detail, searchPath)
	if offset < 0 {
		return nil
	}
	shifted := make([]int, 0, len(indexes))
	for _, index := range indexes {
		if index < 0 || index >= len([]rune(searchPath)) {
			continue
		}
		shifted = append(shifted, offset+index)
	}
	return indexesWithinDetail(shifted, detail)
}

func indexesWithinDetail(indexes []int, detail string) []int {
	if len(indexes) == 0 {
		return nil
	}
	limit := len([]rune(detail))
	filtered := make([]int, 0, len(indexes))
	for _, index := range indexes {
		if index >= 0 && index < limit {
			filtered = append(filtered, index)
		}
	}
	return filtered
}

func uniqueSortedInts(indexes []int) []int {
	sort.Ints(indexes)
	result := indexes[:0]
	previous := -1
	for _, index := range indexes {
		if index == previous {
			continue
		}
		result = append(result, index)
		previous = index
	}
	return result
}

func renderPathColumn(item candidate, detail string, detailStyle lipgloss.Style, highlightStyle lipgloss.Style, width int) string {
	if width <= 0 {
		return renderMatchedDetail(item, detail, detailStyle, highlightStyle)
	}
	detail = truncateDots(detail, width)
	return detailStyle.Width(width).Render(renderMatchedDetail(item, detail, detailStyle, highlightStyle))
}

func renderCandidateRow(item candidate, selected bool, s styles, width int, glyphs config.Glyphs, glyphColors config.GlyphColors, columns config.Columns) string {
	columns = normalizeUIColumns(columns)
	columnWidth := width - searchBoxInnerOffset
	if columnWidth < 1 {
		columnWidth = width
	}
	columns = fitPathColumnToRow(item, s, glyphs, glyphColors, columns, columnWidth)
	columnGap := s.root.Render(strings.Repeat(" ", searchBoxInnerOffset))
	if selected {
		line := s.root.Render("  ") + s.glyph.Render("▌") + renderCandidate(item, true, s, glyphs, glyphColors, columns)
		return padSelectedRow(line, width, s)
	}
	return fitRow("  "+columnGap+renderCandidate(item, false, s, glyphs, glyphColors, columns), width)
}

func fitPathColumnToRow(item candidate, s styles, glyphs config.Glyphs, glyphColors config.GlyphColors, columns config.Columns, width int) config.Columns {
	if width <= 0 || !columns.Path.Show || columns.Path.Width > 0 || item.detail() == "" {
		return columns
	}
	prefixWidth := lipgloss.Width("  " + renderCandidate(item, false, s, glyphs, glyphColors, withoutPathColumn(columns)) + "  ")
	available := width - prefixWidth
	if available < 1 {
		available = 1
	}
	if columns.Path.MaxWidth > 0 && available > columns.Path.MaxWidth {
		available = columns.Path.MaxWidth
	}
	columns.Path.Width = available
	return columns
}

func withoutPathColumn(columns config.Columns) config.Columns {
	columns.Path.Show = false
	return columns
}

func padSelectedRow(line string, width int, s styles) string {
	if width > 0 && lipgloss.Width(line) > width {
		return ansi.Cut(line, 0, width)
	}
	fill := width - lipgloss.Width(line)
	if fill <= 0 {
		return line
	}
	return line + s.selected.Render(strings.Repeat(" ", fill))
}

func fitRow(line string, width int) string {
	if width > 0 && lipgloss.Width(line) > width {
		return ansi.Cut(line, 0, width)
	}
	return line
}

func renderSearchBox(prompt string, value string, width int, s styles) string {
	if width < 1 {
		width = 80
	}
	boxWidth := width - 2
	if boxWidth < 1 {
		boxWidth = 1
	}
	contentWidth := boxWidth - 4
	if contentWidth < 1 {
		contentWidth = 1
	}
	cursor := "█"
	inputBudget := contentWidth - lipgloss.Width(prompt) - lipgloss.Width(cursor)
	if inputBudget < 1 {
		inputBudget = 1
	}
	content := s.searchPrompt.Render(prompt) +
		s.search.Render(truncate(value, inputBudget)) +
		s.searchPrompt.Render(cursor)
	return s.searchBox.Width(boxWidth).Render(content)
}

const searchBoxSideInset = 2
const columnGutterWidth = 2
const searchBoxInnerOffset = 1

func renderInsetSearchBox(prompt string, value string, width int, s styles) string {
	leftInset := searchBoxOuterInset()
	rightInset := searchBoxOuterInset()
	if width <= leftInset+rightInset+4 {
		return renderSearchBox(prompt, value, width, s)
	}
	return lipgloss.NewStyle().
		Width(width).
		Background(s.root.GetBackground()).
		PaddingLeft(leftInset).
		PaddingRight(rightInset).
		Render(renderSearchBox(prompt, value, width-leftInset-rightInset, s))
}

func renderInsetRow(row string, width int, s styles) string {
	return renderInsetRowWithInsets(row, width, rowLeftInset(), rowRightInset(), s)
}

func renderInsetDividerRow(row string, width int, s styles) string {
	rightInset := rowRightInset() - 1
	if rightInset < 0 {
		rightInset = 0
	}
	return renderInsetRowWithInsets(row, width, rowLeftInset(), rightInset, s)
}

func renderInsetRowWithInsets(row string, width int, leftInset int, rightInset int, s styles) string {
	if width <= leftInset+rightInset {
		return fitRow(row, width)
	}
	contentWidth := width - leftInset - rightInset
	row = fitRow(row, contentWidth)
	padding := contentWidth - lipgloss.Width(row)
	if padding < 0 {
		padding = 0
	}
	leftPad := s.root.Render(strings.Repeat(" ", leftInset))
	rightPad := s.root.Render(strings.Repeat(" ", rightInset))
	return leftPad + row + s.root.Render(strings.Repeat(" ", padding)) + rightPad
}

func rowLeftInset() int {
	return searchBoxSideInset
}

func rowRightInset() int {
	return searchBoxOuterInset() + searchBoxInnerOffset
}

func renderColumnOffsetRow(row string, s styles) string {
	return s.root.Render(strings.Repeat(" ", searchBoxInnerOffset)) + row
}

func renderFooterAlignedRow(row string, s styles) string {
	return s.root.Render(strings.Repeat(" ", columnGutterWidth+searchBoxInnerOffset)) + row
}

func searchBoxOuterInset() int {
	return searchBoxSideInset + columnGutterWidth
}

func searchBoxTextInset() int {
	return 2
}

func renderStatusFooter(pathMode bool, skipHidden bool, skipGitignored bool, help string, width int, s styles) string {
	footer := renderPathModeChip(pathMode, s) +
		s.root.Render(" ") +
		renderStatusChip("HIDDEN", skipHidden, s) +
		s.root.Render(" ") +
		renderStatusChip("IGNORED", skipGitignored, s)
	if strings.TrimSpace(help) != "" {
		footer += s.root.Render("  ") + s.muted.Render(help)
	}
	if width > 0 && lipgloss.Width(footer) > width {
		return ansi.Cut(footer, 0, width)
	}
	return footer
}

func renderPathModeChip(active bool, s styles) string {
	state := "OFF"
	style := s.statusSkip
	if active {
		state = "ON"
		style = s.statusShow
	}
	return style.Width(statusChipWidth).Align(lipgloss.Center).Render("PATH " + state)
}

func renderStatusChip(label string, skip bool, s styles) string {
	state := "SHOW"
	style := s.statusShow
	if skip {
		state = "SKIP"
		style = s.statusSkip
	}
	return style.Width(statusChipWidth).Align(lipgloss.Center).Render(label + " " + state)
}

const statusChipWidth = 14

func truncate(value string, maxWidth int) string {
	if maxWidth < 1 {
		return ""
	}
	if lipgloss.Width(value) <= maxWidth {
		return value
	}
	if maxWidth == 1 {
		return "…"
	}
	runes := []rune(value)
	for len(runes) > 0 && lipgloss.Width(string(runes)+"…") > maxWidth {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

func truncateDots(value string, maxWidth int) string {
	value = strings.TrimSpace(value)
	if maxWidth < 1 {
		return ""
	}
	if lipgloss.Width(value) <= maxWidth {
		return value
	}
	if maxWidth <= 3 {
		return strings.Repeat(".", maxWidth)
	}
	runes := []rune(value)
	suffix := "..."
	for len(runes) > 0 && lipgloss.Width(string(runes)+suffix) > maxWidth {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + suffix
}

func renderOriginChip(origin string, glyph string, labelStyle lipgloss.Style, glyphStyle lipgloss.Style, width int) string {
	label := originLabel(origin)
	labelWidth := width - 4
	if labelWidth < 1 {
		labelWidth = originChipLabelWidth()
	}
	label = truncateDots(label, labelWidth)
	padding := labelWidth - lipgloss.Width(label)
	if padding < 0 {
		padding = 0
	}
	if strings.TrimSpace(glyph) == "" {
		glyph = originGlyph(origin, config.Glyphs{})
	}
	return labelStyle.Render(" ") +
		glyphStyle.Render(glyph) +
		labelStyle.Render(" "+label+strings.Repeat(" ", padding)+" ")
}

func renderRootColumn(root string, style lipgloss.Style, highlightStyle lipgloss.Style, indexes []int, width int) string {
	root = truncateDots(root, width)
	if len(indexes) == 0 {
		return style.Width(width).Align(lipgloss.Left).Render(root)
	}
	return style.Width(width).Align(lipgloss.Left).Render(renderMatchedText(root, style, highlightStyle, indexes))
}

func renderNameColumn(name string, style lipgloss.Style, highlightStyle lipgloss.Style, indexes []int, width int) string {
	name = truncate(name, width)
	if len(indexes) == 0 {
		return style.Width(width).Render(name)
	}
	return style.Width(width).Render(renderMatchedText(name, style, highlightStyle, indexes))
}

func originChipLabelWidth() int {
	return len("worktree")
}

func originChipWidth() int {
	return originChipLabelWidth() + 4
}

var rootColumnWidth = originChipWidth()

const nameColumnWidth = 28

func candidateOrigin(item candidate) string {
	if item.kind == candidateSession {
		return item.origin
	}
	if item.kind == candidateRoot {
		return originLabel(rootMode(item.root))
	}
	if item.kind == candidatePath {
		return originLabel("path")
	}
	return "manual"
}

func candidateGlyph(item candidate, glyphs config.Glyphs) string {
	switch item.kind {
	case candidateSession:
		if strings.TrimSpace(item.session.Metadata.Glyph) != "" {
			return item.session.Metadata.Glyph
		}
	case candidateRoot:
		if strings.TrimSpace(item.root.Glyph) != "" {
			return item.root.Glyph
		}
	}
	return originGlyph(candidateOrigin(item), glyphs)
}

func candidateGlyphColor(item candidate, selected bool, glyphColors config.GlyphColors) lipgloss.Color {
	switch item.kind {
	case candidateSession:
		if color := strings.TrimSpace(item.session.Metadata.GlyphColor); color != "" {
			return lipgloss.Color(color)
		}
	case candidateRoot:
		if color := strings.TrimSpace(item.root.GlyphColor); color != "" {
			return lipgloss.Color(color)
		}
	}
	return originGlyphColor(candidateOrigin(item), selected, glyphColors)
}

func renderMatchedText(value string, textStyle lipgloss.Style, highlightStyle lipgloss.Style, indexes []int) string {
	if len(indexes) == 0 {
		return textStyle.Render(value)
	}

	matched := make(map[int]bool, len(indexes))
	valueRunes := []rune(value)
	for _, index := range indexes {
		if index < 0 || index >= len(valueRunes) {
			continue
		}
		matched[index] = true
	}

	var b strings.Builder
	for index, r := range value {
		part := string(r)
		if matched[index] {
			b.WriteString(highlightStyle.Render(part))
			continue
		}
		b.WriteString(textStyle.Render(part))
	}
	return b.String()
}

type helpItem struct {
	Key         string
	Action      string
	Description string
}

type commandID string

const (
	commandOpenSelected  commandID = "open-selected"
	commandOpenLast      commandID = "open-last-session"
	commandOpenTyped     commandID = "open-typed"
	commandKillSession   commandID = "kill-session"
	commandNewSession    commandID = "new-session"
	commandPathSearch    commandID = "path-search"
	commandCycleRoot     commandID = "cycle-root"
	commandReload        commandID = "reload"
	commandToggleHidden  commandID = "toggle-hidden"
	commandToggleIgnored commandID = "toggle-ignored"
	commandHelp          commandID = "help"
	commandQuit          commandID = "quit"
)

type commandItem struct {
	ID             commandID
	Title          string
	Key            string
	Description    string
	Enabled        bool
	DisabledReason string
}

type commandMatch struct {
	item         commandItem
	titleIndexes []int
}

func (item commandItem) fuzzyCandidate() fuzzy.Candidate {
	return fuzzy.Candidate{
		Title:    item.Title,
		Category: "command",
		Aliases:  []string{item.Key, string(item.ID)},
		Fields: []fuzzy.Field{
			{Name: "description", Value: item.Description, Weight: 120},
			{Name: "disabled", Value: item.DisabledReason, Weight: 80},
		},
		Value: item,
	}
}

func (m Model) commandItems() []commandItem {
	if m.commandPreviousMode == modePathSearch {
		typedReason := ""
		if _, ok := m.typedPathCandidate(); !ok {
			typedReason = "The typed prompt is not an existing directory."
		}
		return []commandItem{
			{ID: commandOpenSelected, Title: "Open selected result", Key: "<enter>", Description: "Create or switch to a tmux session for the selected fuzzy path result.", Enabled: len(m.pathResult) > 0, DisabledReason: "There is no selected path result."},
			{ID: commandOpenTyped, Title: "Open typed path", Key: "<c-p>", Description: "Open the exact typed prompt path when it exists as a directory.", Enabled: typedReason == "", DisabledReason: typedReason},
			{ID: commandCycleRoot, Title: "Cycle prompt root", Key: "<c-o>", Description: "Cycle the path prompt through ~/ / ./ ../.", Enabled: true},
			{ID: commandToggleHidden, Title: pathToggleHiddenTitle(m.pathConfig.options.SkipHidden), Key: "<meta-h>", Description: "Toggle whether hidden directories are skipped in the current path search.", Enabled: true},
			{ID: commandToggleIgnored, Title: pathToggleIgnoredTitle(m.pathConfig.options.SkipGitignored), Key: "<meta-i>", Description: "Toggle whether gitignored directories are skipped in the current path search.", Enabled: true},
			{ID: commandReload, Title: "Reload path search", Key: "<c-r>", Description: "Restart the current streamed path search.", Enabled: true},
			{ID: commandHelp, Title: "Show help", Key: "?", Description: "Show help for the command palette.", Enabled: true},
			{ID: commandQuit, Title: "Quit", Key: "<esc>", Description: "Quit tmux-parator.", Enabled: true},
		}
	}
	selected, ok := m.selected()
	killReason := ""
	if !ok {
		killReason = "There is no selected candidate."
	} else if selected.kind != candidateSession {
		killReason = "The selected candidate is not an open tmux session."
	}
	return []commandItem{
		{ID: commandOpenSelected, Title: "Open selected", Key: "<enter>", Description: "Switch to an existing session or create one for the selected root.", Enabled: ok, DisabledReason: "There is no selected candidate."},
		{ID: commandOpenLast, Title: "Open last session", Key: "<c-`>", Description: "Switch to tmux's last active session.", Enabled: true},
		{ID: commandKillSession, Title: "Kill selected session", Key: "<c-k>", Description: "Ask for confirmation before killing the selected tmux session.", Enabled: killReason == "", DisabledReason: killReason},
		{ID: commandNewSession, Title: "Create session in current path", Key: "<c-n>", Description: "Create a path session in the current working directory.", Enabled: true},
		{ID: commandPathSearch, Title: "Create session from path", Key: "<c-t>", Description: "Open filesystem path search, then create or switch to a session for the selected path.", Enabled: m.pathConfig.enabled, DisabledReason: "Path search is disabled in config."},
		{ID: commandToggleHidden, Title: discoveryToggleHiddenTitle(m.discovery.SkipHidden), Key: "<meta-h>", Description: "Toggle whether hidden directories are skipped for configured repos and subdirs.", Enabled: true},
		{ID: commandToggleIgnored, Title: discoveryToggleIgnoredTitle(m.discovery.SkipGitignored), Key: "<meta-i>", Description: "Toggle whether gitignored directories are skipped for configured repos and subdirs.", Enabled: true},
		{ID: commandReload, Title: "Reload", Key: "<c-r>", Description: "Reload tmux sessions and configured root candidates.", Enabled: true},
		{ID: commandHelp, Title: "Show help", Key: "?", Description: "Show help for the command palette.", Enabled: true},
		{ID: commandQuit, Title: "Quit", Key: "<esc>", Description: "Quit tmux-parator.", Enabled: true},
	}
}

func (m Model) commandMatches() []commandMatch {
	items := m.commandItems()
	if strings.TrimSpace(m.commandInput) == "" {
		matches := make([]commandMatch, 0, len(items))
		for _, item := range items {
			matches = append(matches, commandMatch{item: item})
		}
		return matches
	}
	fuzzyCandidates := make([]fuzzy.Candidate, 0, len(items))
	for _, item := range items {
		fuzzyCandidates = append(fuzzyCandidates, item.fuzzyCandidate())
	}
	fuzzyMatches := fuzzy.Filter(fuzzyCandidates, m.commandInput)
	matches := make([]commandMatch, 0, len(fuzzyMatches))
	for _, match := range fuzzyMatches {
		item, ok := match.Candidate.Value.(commandItem)
		if !ok {
			continue
		}
		matches = append(matches, commandMatch{item: item, titleIndexes: match.TitleIndexes})
	}
	return matches
}

func discoveryToggleHiddenTitle(skip bool) string {
	if skip {
		return "Show hidden configured paths"
	}
	return "Skip hidden configured paths"
}

func discoveryToggleIgnoredTitle(skip bool) string {
	if skip {
		return "Show gitignored configured paths"
	}
	return "Skip gitignored configured paths"
}

func pathToggleHiddenTitle(skip bool) string {
	if skip {
		return "Show hidden path results"
	}
	return "Skip hidden path results"
}

func pathToggleIgnoredTitle(skip bool) string {
	if skip {
		return "Show gitignored path results"
	}
	return "Skip gitignored path results"
}

func (m Model) runCommand(item commandItem) (tea.Model, tea.Cmd) {
	if !item.Enabled {
		return m.notifyCommandUnavailable(item), nil
	}
	switch item.ID {
	case commandOpenSelected:
		if m.commandPreviousMode == modePathSearch {
			selected, ok := m.selectedPath()
			if !ok {
				return m.notifyCommandUnavailable(commandItem{Title: item.Title, DisabledReason: "There is no selected path result."}), nil
			}
			m.loading = true
			m.stopPathStream()
			m.mode = modeBrowse
			return m, m.openCandidate(selected)
		}
		selected, ok := m.selected()
		if !ok {
			return m.notifyCommandUnavailable(item), nil
		}
		m.mode = modeBrowse
		return m, m.openCandidate(selected)
	case commandOpenLast:
		m.mode = modeBrowse
		return m, m.switchLastSession()
	case commandOpenTyped:
		selected, ok := m.typedPathCandidate()
		if !ok {
			return m.notifyCommandUnavailable(item), nil
		}
		m.loading = true
		m.stopPathStream()
		m.mode = modeBrowse
		return m, m.openCandidate(selected)
	case commandKillSession:
		m.mode = modeConfirmKill
		m.confirmChoice = confirmCancel
	case commandNewSession:
		m.mode = modeCreateSession
		m.createText = ""
	case commandPathSearch:
		return m.openPathSearch()
	case commandCycleRoot:
		m.mode = modePathSearch
		m.cyclePathRoot()
		return m, m.startPathStream(m.pathRoot)
	case commandReload:
		if m.commandPreviousMode == modePathSearch {
			m.mode = modePathSearch
			m.pathBusy = true
			m.clearPathCompletion()
			return m, m.startPathStream(m.pathRoot)
		}
		m.mode = modeBrowse
		m.loading = true
		return m, tea.Batch(m.loadSessions(), m.loadRoots())
	case commandToggleHidden:
		if m.commandPreviousMode == modePathSearch {
			m.mode = modePathSearch
			m.pathConfig.options.SkipHidden = !m.pathConfig.options.SkipHidden
			m.pathBusy = true
			m.clearPathCompletion()
			return m, m.startPathStream(m.pathRoot)
		}
		m.mode = modeBrowse
		m.toggleDiscoveryHidden()
		m.loading = true
		return m, m.loadRoots()
	case commandToggleIgnored:
		if m.commandPreviousMode == modePathSearch {
			m.mode = modePathSearch
			m.pathConfig.options.SkipGitignored = !m.pathConfig.options.SkipGitignored
			m.pathBusy = true
			m.clearPathCompletion()
			return m, m.startPathStream(m.pathRoot)
		}
		m.mode = modeBrowse
		m.toggleDiscoveryGitignored()
		m.loading = true
		return m, m.loadRoots()
	case commandHelp:
		m.openHelp(modeCommands)
	case commandQuit:
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) notifyCommandUnavailable(item commandItem) Model {
	reason := strings.TrimSpace(item.DisabledReason)
	if reason == "" {
		reason = "Command is not available in the current context."
	}
	if m.commandPreviousMode == modePathSearch {
		m.mode = modePathSearch
		m.pathErr = fmt.Errorf("%s: %s", item.Title, reason)
		return m
	}
	m.mode = modeBrowse
	m.err = fmt.Errorf("%s: %s", item.Title, reason)
	return m
}

func renderCommandPalette(s styles, matches []commandMatch, query string, cursor int, scroll int, appWidth int, appHeight int) string {
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(matches) {
		cursor = len(matches) - 1
	}
	width := helpPanelWidth(appWidth)
	description := "Type to fuzzy-search commands."
	if len(matches) > 0 && cursor >= 0 {
		description = matches[cursor].item.Description
		if !matches[cursor].item.Enabled && strings.TrimSpace(matches[cursor].item.DisabledReason) != "" {
			description = matches[cursor].item.DisabledReason
		}
	}
	descriptionLines := wrapHelpDescription(description, helpDescriptionTextWidth(width))
	height := commandListHeight(appHeight)
	if scroll < 0 {
		scroll = 0
	}
	if scroll > cursor {
		scroll = cursor
	}
	if scroll+height > len(matches) {
		scroll = len(matches) - height
		if scroll < 0 {
			scroll = 0
		}
	}

	panelHeight := helpMainBoxHeight(appHeight)
	contentWidth := width - 2
	horizontalPadding := 3
	bodyWidth := contentWidth - horizontalPadding*2
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	rowWidth := bodyWidth - 2
	if rowWidth < 20 {
		rowWidth = bodyWidth
	}
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(renderCommandInput(query, s, bodyWidth))
	b.WriteString("\n\n")
	for rowIndex := 0; rowIndex < height; rowIndex++ {
		itemIndex := scroll + rowIndex
		row := strings.Repeat(" ", rowWidth)
		if itemIndex < len(matches) {
			row = renderCommandRow(matches[itemIndex], itemIndex == cursor, s, rowWidth)
		} else if rowIndex == 0 && len(matches) == 0 {
			row = s.muted.Render(truncate("No matching commands", rowWidth))
		}
		if rowWidth != bodyWidth {
			row += s.root.Render(" ") + renderHelpScrollbar(rowIndex, scroll, height, len(matches), s)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	mainBox := renderTitledBox("Commands", helpPositionLabel(cursor, len(matches)), b.String(), width, panelHeight, horizontalPadding, s)
	descriptionBox := renderHelpDescription(descriptionLines, s, width)
	return lipgloss.JoinVertical(lipgloss.Left, mainBox, descriptionBox)
}

func renderCommandInput(query string, s styles, width int) string {
	return renderSearchBox("❯ ", query, width, s)
}

func renderCommandRow(match commandMatch, selected bool, s styles, width int) string {
	item := match.item
	keyWidth := 12
	key := truncate(item.Key, keyWidth)
	titleBudget := width - keyWidth - 3
	if titleBudget < 1 {
		titleBudget = 1
	}
	title := truncate(item.Title, titleBudget)
	titleIndexes := match.titleIndexes
	if !item.Enabled {
		title = truncate(item.Title+" (disabled)", titleBudget)
		titleIndexes = nil
	}
	renderedTitle := renderMatchedText(title, s.muted, s.match, titleIndexes)
	line := s.warn.Width(keyWidth).Render(key) + s.muted.Render("  ") + renderedTitle
	if !item.Enabled {
		line = s.muted.Width(keyWidth).Render(key) + s.muted.Render("  ") + s.muted.Render(title)
	}
	if selected {
		line = s.selected.Width(keyWidth).Render(key) + s.selected.Render("  "+title)
		return padSelectedRow(line, width, s)
	}
	return line
}

func renderConfirmKill(s styles, sessionName string, choice confirmChoice, appWidth int) string {
	width := helpPanelWidth(appWidth)
	if width > 72 {
		width = 72
	}
	if width < 42 {
		width = 42
	}
	bodyWidth := width - 8
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	if sessionName == "" {
		sessionName = "selected session"
	}
	message := "Kill tmux session " + strconv.Quote(sessionName) + "?"
	lines := wrapHelpDescription(message, bodyWidth)
	lines = append(lines, "")
	lines = append(lines, renderConfirmAction("Cancel", "n/<esc>", choice == confirmCancel, s)+s.popupBody.Render("   ")+renderConfirmAction("Confirm", "y", choice == confirmYes, s))
	return renderCenteredTitledBox("Confirm", "", strings.Join(lines, "\n"), width, len(lines)+4, 3, s)
}

func renderConfirmAction(label string, key string, selected bool, s styles) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}
	text := prefix + key + " " + label
	if selected {
		return s.popupAccent.Render(text)
	}
	return s.popupMuted.Render(text)
}

func renderCreateSession(s styles, value string, appWidth int) string {
	width := helpPanelWidth(appWidth)
	if width > 72 {
		width = 72
	}
	if width < 46 {
		width = 46
	}
	bodyWidth := width - 8
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	lines := []string{
		s.popupMuted.Render("Create a path session in the current path."),
		"",
		renderSearchBox("name ❯ ", value, bodyWidth, s),
		"",
		s.popupAccent.Render("<enter>") + s.popupMuted.Render(" create") + s.popupBody.Render("   ") + s.popupAccent.Render("<esc>") + s.popupMuted.Render(" cancel"),
	}
	return renderCenteredTitledBox("Create Session", "", strings.Join(lines, "\n"), width, len(lines)+4, 3, s)
}

func renderErrorPopup(s styles, message string, appWidth int) string {
	width := helpPanelWidth(appWidth)
	if width > 96 {
		width = 96
	}
	if width < 46 {
		width = 46
	}
	bodyWidth := width - 8
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	lines := wrapHelpDescription(message, bodyWidth)
	lines = append(lines, "")
	lines = append(lines, s.popupAccent.Render("<esc>")+s.popupMuted.Render(" dismiss")+s.popupBody.Render("   ")+s.popupAccent.Render("<c-r>")+s.popupMuted.Render(" retry/reload"))
	return renderCenteredTitledBox("Error", "", strings.Join(lines, "\n"), width, len(lines)+4, 3, s)
}

func renderHelpPanel(s styles, previous mode, cursor int, scroll int, appWidth int, appHeight int) string {
	items := helpItemsForMode(previous)
	if len(items) == 0 {
		items = helpItemsForMode(modeBrowse)
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(items) {
		cursor = len(items) - 1
	}
	width := helpPanelWidth(appWidth)
	description := helpDescription(items[cursor])
	descriptionLines := wrapHelpDescription(description, helpDescriptionTextWidth(width))
	height := helpListHeight(appHeight)
	if scroll < 0 {
		scroll = 0
	}
	if scroll > cursor {
		scroll = cursor
	}
	if scroll+height > len(items) {
		scroll = len(items) - height
		if scroll < 0 {
			scroll = 0
		}
	}

	panelHeight := helpMainBoxHeight(appHeight)
	contentWidth := width - 2
	horizontalPadding := 3
	bodyWidth := contentWidth - horizontalPadding*2
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	rowWidth := bodyWidth - 2
	if rowWidth < 20 {
		rowWidth = bodyWidth
	}
	var b strings.Builder
	b.WriteString("\n")
	for rowIndex := 0; rowIndex < height; rowIndex++ {
		itemIndex := scroll + rowIndex
		row := strings.Repeat(" ", rowWidth)
		if itemIndex < len(items) {
			row = renderHelpRow(items[itemIndex], itemIndex == cursor, s, rowWidth)
		}
		if rowWidth != bodyWidth {
			row += s.root.Render(" ") + renderHelpScrollbar(rowIndex, scroll, height, len(items), s)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	mainBox := renderTitledBox(helpTitle(previous), helpPositionLabel(cursor, len(items)), b.String(), width, panelHeight, horizontalPadding, s)
	descriptionBox := renderHelpDescription(descriptionLines, s, width)
	return lipgloss.JoinVertical(lipgloss.Left, mainBox, descriptionBox)
}

func renderHelpRow(item helpItem, selected bool, s styles, width int) string {
	keyWidth := 14
	key := truncate(item.Key, keyWidth)
	actionBudget := width - keyWidth - 3
	if actionBudget < 1 {
		actionBudget = 1
	}
	line := s.warn.Width(keyWidth).Render(key) + s.muted.Render("  ") + s.muted.Render(truncate(item.Action, actionBudget))
	if selected {
		line = s.selected.Width(keyWidth).Render(key) + s.selected.Render("  "+truncate(item.Action, actionBudget))
		return padSelectedRow(line, width, s)
	}
	return line
}

func renderHelpScrollbar(row int, scroll int, visible int, total int, s styles) string {
	if total <= visible || visible <= 0 {
		return s.root.Render(" ")
	}
	thumbSize := visible * visible / total
	if thumbSize < 1 {
		thumbSize = 1
	}
	if thumbSize > visible {
		thumbSize = visible
	}
	maxScroll := total - visible
	thumbStart := 0
	if maxScroll > 0 {
		thumbStart = scroll * (visible - thumbSize) / maxScroll
	}
	if row >= thumbStart && row < thumbStart+thumbSize {
		return s.warn.Render("┃")
	}
	return s.muted.Render("│")
}

func helpPositionLabel(cursor int, total int) string {
	if total < 1 {
		return " 0/0 "
	}
	return fmt.Sprintf(" %d/%d ", cursor+1, total)
}

func helpDescription(item helpItem) string {
	if item.Description != "" {
		return item.Description
	}
	return item.Action
}

func helpDescriptionTextWidth(width int) int {
	horizontalPadding := 3
	textWidth := width - 2 - horizontalPadding*2
	if textWidth < 1 {
		textWidth = 1
	}
	return textWidth
}

func wrapHelpDescription(description string, width int) []string {
	if width < 1 {
		width = 1
	}
	paragraphs := strings.Split(description, "\n")
	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := ""
		for _, word := range words {
			parts := splitHelpWord(word, width)
			for _, part := range parts {
				if current == "" {
					current = part
					continue
				}
				word = part
				if lipgloss.Width(current)+1+lipgloss.Width(word) <= width {
					current += " " + word
					continue
				}
				lines = append(lines, current)
				current = word
			}
		}
		if current != "" {
			lines = append(lines, current)
		}
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func splitHelpWord(word string, width int) []string {
	if lipgloss.Width(word) <= width {
		return []string{word}
	}
	parts := make([]string, 0, 2)
	remaining := []rune(word)
	for len(remaining) > 0 {
		cut := len(remaining)
		for cut > 1 && lipgloss.Width(string(remaining[:cut])) > width {
			cut--
		}
		parts = append(parts, string(remaining[:cut]))
		remaining = remaining[cut:]
	}
	return parts
}

func renderHelpDescription(lines []string, s styles, width int) string {
	if width < 20 {
		width = 20
	}
	horizontalPadding := 3
	pad := s.root.Render(strings.Repeat(" ", horizontalPadding))
	top := s.muted.Render("╭") + s.muted.Render(strings.Repeat("─", width-2)) + s.muted.Render("╮")
	bottom := s.muted.Render("╰") + s.muted.Render(strings.Repeat("─", width-2)) + s.muted.Render("╯")
	if len(lines) == 0 {
		lines = []string{""}
	}
	rendered := make([]string, 0, len(lines)+2)
	rendered = append(rendered, top)
	for _, descriptionLine := range lines {
		line := pad + s.muted.Render(descriptionLine) + pad
		rightPadding := width - 2 - lipgloss.Width(line)
		if rightPadding < 0 {
			rightPadding = 0
		}
		rendered = append(rendered, s.muted.Render("│")+line+s.root.Render(strings.Repeat(" ", rightPadding))+s.muted.Render("│"))
	}
	rendered = append(rendered, bottom)
	return strings.Join(rendered, "\n")
}

func renderTitledBox(title string, bottomLabel string, content string, width int, height int, horizontalPadding int, s styles) string {
	if width < 20 {
		width = 20
	}
	if height < 6 {
		height = 6
	}
	title = " " + title + " "
	topFill := width - lipgloss.Width(title) - 2
	if topFill < 1 {
		topFill = 1
	}
	top := s.popupFrame.Render("╭" + title + strings.Repeat("─", topFill) + "╮")
	bottomLabelWidth := lipgloss.Width(bottomLabel)
	bottomFill := width - bottomLabelWidth - 2
	if bottomFill < 1 {
		bottomFill = 1
		bottomLabel = ""
	}
	bottom := s.popupFrame.Render("╰" + strings.Repeat("─", bottomFill) + bottomLabel + "╯")
	lines := strings.Split(content, "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	contentHeight := height - 2
	if contentHeight < len(lines) {
		contentHeight = len(lines)
	}
	pad := strings.Repeat(" ", horizontalPadding)
	var b strings.Builder
	b.WriteString(top)
	b.WriteString("\n")
	for i := 0; i < contentHeight; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		paddedLine := pad + line + pad
		padding := width - 2 - lipgloss.Width(paddedLine)
		if padding < 0 {
			padding = 0
		}
		b.WriteString(s.popupFrame.Render("│"))
		b.WriteString(paddedLine)
		b.WriteString(s.root.Render(strings.Repeat(" ", padding)))
		b.WriteString(s.popupFrame.Render("│"))
		b.WriteString("\n")
	}
	b.WriteString(bottom)
	return b.String()
}

func renderCenteredTitledBox(title string, bottomLabel string, content string, width int, height int, horizontalPadding int, s styles) string {
	if width < 20 {
		width = 20
	}
	if height < 6 {
		height = 6
	}
	title = " " + title + " "
	topFill := width - lipgloss.Width(title) - 2
	if topFill < 1 {
		topFill = 1
	}
	top := s.popupFrame.Render("╭" + title + strings.Repeat("─", topFill) + "╮")
	bottomLabelWidth := lipgloss.Width(bottomLabel)
	bottomFill := width - bottomLabelWidth - 2
	if bottomFill < 1 {
		bottomFill = 1
		bottomLabel = ""
	}
	bottom := s.popupFrame.Render("╰" + strings.Repeat("─", bottomFill) + bottomLabel + "╯")
	lines := strings.Split(content, "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	contentHeight := height - 2
	if contentHeight < len(lines) {
		contentHeight = len(lines)
	}
	topPadding := 0
	if contentHeight > len(lines) {
		topPadding = (contentHeight - len(lines)) / 2
	}
	bottomPadding := contentHeight - len(lines) - topPadding
	pad := strings.Repeat(" ", horizontalPadding)
	innerWidth := width - 2 - horizontalPadding*2
	if innerWidth < 1 {
		innerWidth = 1
	}
	sidePad := s.popupBody.Render(pad)
	bodyFill := s.popupBody.Render(strings.Repeat(" ", innerWidth))
	var b strings.Builder
	b.WriteString(top)
	b.WriteString("\n")
	for i := 0; i < topPadding; i++ {
		b.WriteString(s.popupFrame.Render("│"))
		b.WriteString(sidePad)
		b.WriteString(bodyFill)
		b.WriteString(sidePad)
		b.WriteString(s.popupFrame.Render("│"))
		b.WriteString("\n")
	}
	for _, line := range lines {
		visibleWidth := lipgloss.Width(line)
		if visibleWidth > innerWidth {
			line = truncate(line, innerWidth)
			visibleWidth = lipgloss.Width(line)
		}
		leftPadding := (innerWidth - visibleWidth) / 2
		rightPadding := innerWidth - visibleWidth - leftPadding
		if strings.TrimSpace(ansi.Strip(line)) != "" && !strings.Contains(line, "\x1b[") {
			line = s.popupBody.Render(line)
		}
		b.WriteString(s.popupFrame.Render("│"))
		b.WriteString(sidePad)
		b.WriteString(s.popupBody.Render(strings.Repeat(" ", leftPadding)))
		b.WriteString(line)
		b.WriteString(s.popupBody.Render(strings.Repeat(" ", rightPadding)))
		b.WriteString(sidePad)
		b.WriteString(s.popupFrame.Render("│"))
		b.WriteString("\n")
	}
	for i := 0; i < bottomPadding; i++ {
		b.WriteString(s.popupFrame.Render("│"))
		b.WriteString(sidePad)
		b.WriteString(bodyFill)
		b.WriteString(sidePad)
		b.WriteString(s.popupFrame.Render("│"))
		b.WriteString("\n")
	}
	b.WriteString(bottom)
	return b.String()
}

func helpPanelWidth(appWidth int) int {
	if appWidth <= 0 {
		return 72
	}
	width := appWidth - 8
	if width > 88 {
		width = 88
	}
	if width < 62 {
		width = appWidth - 4
	}
	if width < 30 {
		width = 30
	}
	return width
}

func helpPanelHeight(appHeight int) int {
	if appHeight <= 0 {
		return 18
	}
	height := appHeight - 4
	if height < 10 {
		height = appHeight - 2
	}
	if height < 6 {
		height = 6
	}
	return height
}

func helpMainBoxHeight(appHeight int) int {
	height := helpPanelHeight(appHeight) - 3
	if height < 6 {
		height = 6
	}
	return height
}

func helpTitle(previous mode) string {
	if previous == modePathSearch {
		return "Path Search Help"
	}
	if previous == modeCommands {
		return "Command Palette Help"
	}
	return "Help"
}

func helpListHeight(appHeight int) int {
	height := helpMainBoxHeight(appHeight) - 4
	if height < 4 {
		height = 4
	}
	return height
}

func commandListHeight(appHeight int) int {
	height := helpListHeight(appHeight) - 3
	if height < 3 {
		height = 3
	}
	return height
}

func helpItemsForMode(previous mode) []helpItem {
	if previous == modePathSearch {
		return []helpItem{
			{Key: "type", Action: "edit path prompt", Description: "Type a path-like prompt; the text after the last slash is used as the fuzzy query."},
			{Key: "backspace", Action: "remove character", Description: "Remove one character from the path prompt and reparse the root/query."},
			{Key: "<up>/<down>", Action: "move selection", Description: "Move through fuzzy results without changing the typed path."},
			{Key: "<tab>", Action: "complete/narrow", Description: "Complete the current path segment, or narrow into the selected fuzzy result."},
			{Key: "<s-tab>", Action: "previous completion", Description: "Cycle backward through the current completion candidates."},
			{Key: "<left>/<right>", Action: "accept completion cycle", Description: "Clear the current completion cycle so the next Tab completes the next path level."},
			{Key: "<enter>", Action: "open selected result", Description: "Create or switch to a tmux session for the selected fuzzy result."},
			{Key: "<c-p>", Action: "open typed path", Description: "Open the exact typed prompt path when it exists as a directory."},
			{Key: "<c-o>", Action: "cycle prompt root", Description: "Cycle the prompt through ~/ / ./ ../."},
			{Key: "<c-`>", Action: "open last session", Description: "Switch to tmux's last active session."},
			{Key: "<meta-h>", Action: "toggle hidden dirs", Description: "Toggle whether hidden directories are skipped in the current path search."},
			{Key: "<meta-i>", Action: "toggle ignored dirs", Description: "Toggle whether gitignored directories are skipped in the current path search."},
			{Key: "<c-r>", Action: "reload search", Description: "Restart the current streamed path search."},
			{Key: "<c-g>", Action: "command palette", Description: "Open the command palette for path-search actions."},
			{Key: "?/<esc>", Action: "close help", Description: "Close this help popup and return to path search."},
		}
	}
	if previous == modeCommands {
		return []helpItem{
			{Key: "type", Action: "filter commands", Description: "Fuzzy-search commands by title, key, and description."},
			{Key: "backspace", Action: "remove character", Description: "Remove one character from the command search prompt."},
			{Key: "<up>/<down>", Action: "move selection", Description: "Move through available commands."},
			{Key: "<enter>", Action: "run selected command", Description: "Run the selected command when it is available in the current context."},
			{Key: "<c-g>", Action: "close palette", Description: "Close the command palette and return to the previous mode."},
			{Key: "<esc>", Action: "close palette/help", Description: "Close help, then close the command palette when pressed again."},
			{Key: "?", Action: "toggle help", Description: "Show or close this help popup."},
			{Key: "Quit command", Action: "quit app", Description: "Run the Quit command from the palette to exit tmux-parator."},
		}
	}
	return []helpItem{
		{Key: "type", Action: "filter sessions and roots", Description: "Filter open tmux sessions and configured root candidates."},
		{Key: "<enter>", Action: "open selected", Description: "Switch to an existing session or create one for the selected root."},
		{Key: "<up>/<down>", Action: "move selection", Description: "Move through matching sessions and root candidates."},
		{Key: "<tab>/<s-tab>", Action: "jump sections", Description: "Jump between open sessions and available workspaces."},
		{Key: "<c-g>", Action: "command overlay", Description: "Open the command overlay for less frequent actions."},
		{Key: "<c-`>", Action: "open last session", Description: "Switch to tmux's last active session."},
		{Key: "<c-n>", Action: "create session in path", Description: "Create a path session in the current working directory."},
		{Key: "<c-t>", Action: "create session from path", Description: "Open filesystem path search, then create or switch to a session for the selected path."},
		{Key: "<c-r>", Action: "reload", Description: "Reload tmux sessions and configured root candidates."},
		{Key: "<meta-h>", Action: "toggle hidden roots", Description: "Toggle whether hidden directories are skipped for configured repos and subdirs."},
		{Key: "<meta-i>", Action: "toggle ignored roots", Description: "Toggle whether gitignored directories are skipped for configured repos and subdirs."},
		{Key: "<c-k>", Action: "kill session", Description: "Ask for confirmation before killing the selected tmux session."},
		{Key: "?/<esc>", Action: "close help", Description: "Close this help popup and return to the main list."},
	}
}
