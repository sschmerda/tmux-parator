package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sschmerda/tmux-parator/internal/config"
	"github.com/sschmerda/tmux-parator/internal/discovery"
	"github.com/sschmerda/tmux-parator/internal/fuzzy"
	"github.com/sschmerda/tmux-parator/internal/pathsearch"
	"github.com/sschmerda/tmux-parator/internal/sessionconfig"
	"github.com/sschmerda/tmux-parator/internal/theme"
	"github.com/sschmerda/tmux-parator/internal/tmux"
)

type sessionClient interface {
	ListSessions(context.Context) ([]tmux.Session, error)
	SwitchSession(context.Context, string) error
	SwitchLastSession(context.Context) error
	KillSession(context.Context, string) error
	RenameSession(context.Context, string, string) error
	NewSession(context.Context, string, string, tmux.SessionMetadata) error
	NewSessionWithLayout(context.Context, string, string, tmux.SessionMetadata, sessionconfig.Template) error
	NewSessionWithLayoutAndSwitch(context.Context, string, string, tmux.SessionMetadata, sessionconfig.Template) error
	TagSession(context.Context, string, tmux.SessionMetadata) error
}

type mode int

const (
	modeBrowse mode = iota
	modeConfirmKill
	modeCommands
	modeHelp
	modeCreateSession
	modeRenameSession
	modePathSearch
	modeConfirmCreatePath
	modeTemplatePicker
	modeTemplateParameter
)

type confirmChoice int

const (
	confirmCancel confirmChoice = iota
	confirmYes
)

type Model struct {
	client                sessionClient
	roots                 []config.Root
	discovery             discovery.Options
	pathConfig            pathsearchConfig
	glyphs                config.Glyphs
	glyphColors           config.GlyphColors
	columns               config.Columns
	dialogs               config.Dialogs
	keys                  config.KeyBindings
	templates             []sessionconfig.Template
	matchingTemplates     []sessionconfig.Template
	sessions              []tmux.Session
	rootItems             []discovery.Candidate
	candidates            []candidate
	filtered              []candidate
	pathItems             []candidate
	pathResult            []candidate
	pathCompletions       []candidate
	filter                string
	commandInput          string
	pathInput             string
	pathRoot              string
	createPathInput       string
	createText            string
	createPath            string
	createMetadata        tmux.SessionMetadata
	templatePath          string
	templateMetadata      tmux.SessionMetadata
	templateName          string
	templateAvailable     []sessionconfig.Template
	templateFilter        string
	templateFiltered      []sessionconfig.Template
	pendingTemplateName   string
	pendingTemplatePath   string
	pendingTemplateMeta   tmux.SessionMetadata
	pendingTemplate       sessionconfig.Template
	renameText            string
	renameOriginal        string
	cursor                int
	scroll                int
	filterRestoreCursor   int
	filterRestoreScroll   int
	filterRestoreActive   bool
	pathCursor            int
	pathScroll            int
	helpCursor            int
	helpScroll            int
	helpInput             string
	helpRestore           int
	commandCursor         int
	commandScroll         int
	commandRestore        int
	templateCursor        int
	templateScroll        int
	templateRestore       int
	parameterTemplate     sessionconfig.Template
	parameterPath         string
	parameterFallback     string
	parameterMetadata     tmux.SessionMetadata
	parameterValues       map[string]string
	parameterIndex        int
	parameterCursor       int
	parameterPreviousMode mode
	confirmChoice         confirmChoice
	pathCompletionCursor  int
	pathCompletionInput   string
	pathCompletionRoot    string
	pathCompletionQuery   string
	pathStream            <-chan pathsearch.Batch
	pathCancel            context.CancelFunc
	width                 int
	height                int
	err                   error
	rootErr               error
	pathErr               error
	notice                error
	pathNotice            error
	mode                  mode
	previousMode          mode
	commandPreviousMode   mode
	templatePreviousMode  mode
	loading               bool
	pathBusy              bool
	styles                styles
}

const noTemplatePickerName = "No template"

func noTemplatePickerItem() sessionconfig.Template {
	return sessionconfig.Template{
		Name:        noTemplatePickerName,
		Description: "Create a normal tmux session without applying a template.",
		Chip:        "nt",
	}
}

func isNoTemplatePickerItem(template sessionconfig.Template) bool {
	return template.ID == "" && template.Name == noTemplatePickerName
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

type renamedMsg struct {
	err error
}

type createdMsg struct {
	name     string
	switched bool
	err      error
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

func NewModel(client sessionClient, activeTheme theme.Theme, roots []config.Root, discoveryOptions discovery.Options, pathSearch config.PathSearch, glyphs config.Glyphs, glyphColors config.GlyphColors, columns config.Columns, dialogs ...config.Dialogs) Model {
	return NewModelWithKeys(client, activeTheme, roots, discoveryOptions, pathSearch, glyphs, glyphColors, columns, config.Default().UI.Keys, dialogs...)
}

func NewModelWithKeys(client sessionClient, activeTheme theme.Theme, roots []config.Root, discoveryOptions discovery.Options, pathSearch config.PathSearch, glyphs config.Glyphs, glyphColors config.GlyphColors, columns config.Columns, keys config.KeyBindings, dialogs ...config.Dialogs) Model {
	return NewModelWithTemplates(client, activeTheme, roots, discoveryOptions, pathSearch, glyphs, glyphColors, columns, keys, nil, dialogs...)
}

func NewModelWithTemplates(client sessionClient, activeTheme theme.Theme, roots []config.Root, discoveryOptions discovery.Options, pathSearch config.PathSearch, glyphs config.Glyphs, glyphColors config.GlyphColors, columns config.Columns, keys config.KeyBindings, templates []sessionconfig.Template, dialogs ...config.Dialogs) Model {
	glyphs = normalizeUIGlyphs(glyphs)
	glyphColors = normalizeUIGlyphColors(glyphColors)
	columns = normalizeUIColumns(columns)
	if keyBindingsEmpty(keys) {
		keys = config.Default().UI.Keys
	}
	dialogConfig := config.Dialogs{}
	if len(dialogs) > 0 {
		dialogConfig = dialogs[0]
	}
	dialogConfig = normalizeUIDialogs(dialogConfig)
	return Model{
		client:            client,
		roots:             roots,
		discovery:         discoveryOptions,
		glyphs:            glyphs,
		glyphColors:       glyphColors,
		columns:           columns,
		dialogs:           dialogConfig,
		keys:              keys,
		templates:         sessionconfig.EnabledTemplates(templates),
		matchingTemplates: sessionconfig.EnabledTemplatesInOrder(templates),
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

func keyBindingsEmpty(keys config.KeyBindings) bool {
	return len(keys.Browse.Quit) == 0 &&
		len(keys.PathSearch.Close) == 0 &&
		len(keys.Commands.Close) == 0 &&
		len(keys.Help.Close) == 0 &&
		len(keys.Confirm.Yes) == 0
}

func keyMatches(msg tea.KeyMsg, keys []string) bool {
	value := msg.String()
	for _, key := range keys {
		if value == key {
			return true
		}
	}
	return false
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
	case renamedMsg:
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
		if msg.switched {
			return m, tea.Quit
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
	if keyMatches(msg, dismissKeys(m.keys.Browse.Quit)) || keyMatches(msg, dismissKeys(m.keys.PathSearch.Close)) || keyMatches(msg, dismissKeys(m.keys.Commands.Close)) || keyMatches(msg, dismissKeys(m.keys.Help.Close)) || keyMatches(msg, dismissKeys(m.keys.Confirm.No)) {
		if m.clearVisibleError() {
			return m, nil
		}
	}
	if m.notice != nil {
		if keyMatches(msg, m.keys.Browse.OpenSelected) || keyMatches(msg, dismissKeys(m.keys.Browse.Quit)) {
			m.notice = nil
			if keyMatches(msg, m.keys.Browse.OpenSelected) && strings.TrimSpace(m.pendingTemplatePath) != "" {
				fallbackName := m.pendingTemplateName
				path := m.pendingTemplatePath
				metadata := m.pendingTemplateMeta
				template := m.pendingTemplate
				m.clearPendingTemplate()
				return m.beginTemplateCreation(template, fallbackName, path, metadata, modeBrowse)
			}
			m.clearPendingTemplate()
		}
		return m, nil
	}
	if m.showsPathSearchBase() && m.pathNotice != nil {
		if keyMatches(msg, m.keys.PathSearch.OpenSelected) || keyMatches(msg, dismissKeys(m.keys.PathSearch.Close)) {
			m.pathNotice = nil
		}
		return m, nil
	}
	if m.mode == modeCommands {
		return m.updateCommandKey(msg)
	}
	if m.mode == modeHelp {
		items := m.helpMatches()
		switch {
		case keyMatches(msg, m.keys.Help.Close):
			m.mode = m.previousMode
		case keyMatches(msg, m.keys.Browse.Up):
			if m.helpCursor > 0 {
				m.helpCursor--
				m.ensureHelpCursorVisible()
			}
		case keyMatches(msg, m.keys.Browse.Down):
			if m.helpCursor < len(items)-1 {
				m.helpCursor++
				m.ensureHelpCursorVisible()
			}
		case keyMatches(msg, m.keys.Browse.PageUp):
			m.moveHelpCursor(-halfPageStep(commandListHeightForFrame(panelDialogFrame(m.dialogs, m.innerWidth(), m.innerHeight()))))
		case keyMatches(msg, m.keys.Browse.PageDown):
			m.moveHelpCursor(halfPageStep(commandListHeightForFrame(panelDialogFrame(m.dialogs, m.innerWidth(), m.innerHeight()))))
		case keyMatches(msg, m.keys.Browse.ScrollUp):
			m.scrollHelpViewport(-1)
		case keyMatches(msg, m.keys.Browse.ScrollDown):
			m.scrollHelpViewport(1)
		case keyMatches(msg, m.keys.Browse.DeleteChar):
			m.setHelpInput(deleteLastRune(m.helpInput))
		case keyMatches(msg, m.keys.Browse.DeleteWord):
			m.setHelpInput(deleteLastShellWord(m.helpInput))
		case keyMatches(msg, m.keys.Browse.ClearInput):
			m.setHelpInput("")
		default:
			if len(msg.Runes) > 0 && !msg.Alt {
				m.setHelpInput(m.helpInput + string(msg.Runes))
			}
		}
		return m, nil
	}
	if m.mode == modeCreateSession {
		return m.updateCreateKey(msg)
	}
	if m.mode == modeRenameSession {
		return m.updateRenameKey(msg)
	}
	if m.mode == modeConfirmCreatePath {
		switch {
		case keyMatches(msg, m.keys.Confirm.Yes):
			return m.confirmCreateTypedPath()
		case keyMatches(msg, m.keys.Confirm.No):
			m.mode = modePathSearch
			m.confirmChoice = confirmCancel
			return m, nil
		case keyMatches(msg, m.keys.Confirm.Left):
			m.confirmChoice = confirmCancel
		case keyMatches(msg, m.keys.Confirm.Right):
			m.confirmChoice = confirmYes
		case keyMatches(msg, m.keys.Confirm.Submit):
			if m.confirmChoice == confirmYes {
				return m.confirmCreateTypedPath()
			}
			m.mode = modePathSearch
			m.confirmChoice = confirmCancel
			return m, nil
		}
		return m, nil
	}
	if m.mode == modePathSearch {
		return m.updatePathSearchKey(msg)
	}
	if m.mode == modeTemplatePicker {
		return m.updateTemplateKey(msg)
	}
	if m.mode == modeTemplateParameter {
		return m.updateTemplateParameterKey(msg)
	}
	if m.mode == modeConfirmKill {
		switch {
		case keyMatches(msg, m.keys.Confirm.Yes):
			return m.confirmKill()
		case keyMatches(msg, m.keys.Confirm.No):
			m.mode = modeBrowse
			return m, nil
		case keyMatches(msg, m.keys.Confirm.Left):
			m.confirmChoice = confirmCancel
		case keyMatches(msg, m.keys.Confirm.Right):
			m.confirmChoice = confirmYes
		case keyMatches(msg, m.keys.Confirm.Submit):
			if m.confirmChoice == confirmYes {
				return m.confirmKill()
			}
			m.mode = modeBrowse
			return m, nil
		}
		return m, nil
	}

	switch {
	case keyMatches(msg, m.keys.Browse.Quit):
		return m, tea.Quit
	case keyMatches(msg, m.keys.Browse.CommandPalette):
		m.openCommands(m.mode)
	case keyMatches(msg, m.keys.Browse.OpenLastSession):
		return m, m.switchLastSession()
	case keyMatches(msg, m.keys.Browse.Help):
		m.openHelp(m.mode)
	case keyMatches(msg, m.keys.Browse.Reload):
		m.loading = true
		return m, tea.Batch(m.loadSessions(), m.loadRoots())
	case keyMatches(msg, m.keys.Browse.RenameSession):
		return m.openRenameSession()
	case keyMatches(msg, m.keys.Browse.NewSession):
		m.openCreateSession()
	case keyMatches(msg, m.keys.Browse.TemplateSession):
		return m.openTemplatePicker()
	case keyMatches(msg, m.keys.Browse.PathSearch):
		return m.openPathSearch()
	case keyMatches(msg, m.keys.Browse.ToggleHidden):
		m.toggleDiscoveryHidden()
		m.loading = true
		return m, m.loadRoots()
	case keyMatches(msg, m.keys.Browse.ToggleIgnored):
		m.toggleDiscoveryGitignored()
		m.loading = true
		return m, m.loadRoots()
	case keyMatches(msg, m.keys.Browse.Up):
		if m.cursor > 0 {
			m.cursor--
			m.ensureCursorVisible()
		}
	case keyMatches(msg, m.keys.Browse.Down):
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.ensureCursorVisible()
		}
	case keyMatches(msg, m.keys.Browse.PageUp):
		m.moveBrowseCursor(-halfPageStep(m.listLimit()))
	case keyMatches(msg, m.keys.Browse.PageDown):
		m.moveBrowseCursor(halfPageStep(m.listLimit()))
	case keyMatches(msg, m.keys.Browse.ScrollUp):
		m.scrollBrowseViewport(-1)
	case keyMatches(msg, m.keys.Browse.ScrollDown):
		m.scrollBrowseViewport(1)
	case keyMatches(msg, m.keys.Browse.JumpNextSection):
		m.jumpBrowseSection(1)
	case keyMatches(msg, m.keys.Browse.JumpPreviousSection):
		m.jumpBrowseSection(-1)
	case keyMatches(msg, m.keys.Browse.OpenSelected):
		selected, ok := m.selected()
		if !ok {
			return m, nil
		}
		return m.openCandidate(selected)
	case keyMatches(msg, m.keys.Browse.KillSession):
		if selected, ok := m.selected(); ok && selected.kind == candidateSession {
			m.mode = modeConfirmKill
			m.confirmChoice = confirmCancel
		}
	case keyMatches(msg, m.keys.Browse.DeleteChar):
		m.removeBrowseFilterRune()
	case keyMatches(msg, m.keys.Browse.DeleteWord):
		m.removeBrowseFilterWord()
	case keyMatches(msg, m.keys.Browse.ClearInput):
		m.clearBrowseFilter()
	default:
		if len(msg.String()) == 1 && !msg.Alt {
			m.addBrowseFilterText(msg.String())
		}
	}

	return m, nil
}

func (m Model) updateCommandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.commandMatches()
	switch {
	case keyMatches(msg, m.keys.Commands.Close):
		m.mode = m.commandPreviousMode
	case keyMatches(msg, m.keys.Commands.Help):
		m.openHelp(m.mode)
	case keyMatches(msg, m.keys.Browse.Up):
		if m.commandCursor > 0 {
			m.commandCursor--
			m.ensureCommandCursorVisible()
		}
	case keyMatches(msg, m.keys.Browse.Down):
		if m.commandCursor < len(items)-1 {
			m.commandCursor++
			m.ensureCommandCursorVisible()
		}
	case keyMatches(msg, m.keys.Browse.PageUp):
		m.moveCommandCursor(-halfPageStep(commandListHeightForFrame(panelDialogFrame(m.dialogs, m.innerWidth(), m.innerHeight()))))
	case keyMatches(msg, m.keys.Browse.PageDown):
		m.moveCommandCursor(halfPageStep(commandListHeightForFrame(panelDialogFrame(m.dialogs, m.innerWidth(), m.innerHeight()))))
	case keyMatches(msg, m.keys.Browse.ScrollUp):
		m.scrollCommandViewport(-1)
	case keyMatches(msg, m.keys.Browse.ScrollDown):
		m.scrollCommandViewport(1)
	case keyMatches(msg, m.keys.Commands.RunSelected):
		if len(items) == 0 || m.commandCursor < 0 || m.commandCursor >= len(items) {
			return m, nil
		}
		return m.runCommand(items[m.commandCursor].item)
	case keyMatches(msg, m.keys.Browse.DeleteChar):
		m.setCommandInput(deleteLastRune(m.commandInput))
	case keyMatches(msg, m.keys.Browse.DeleteWord):
		m.setCommandInput(deleteLastShellWord(m.commandInput))
	case keyMatches(msg, m.keys.Browse.ClearInput):
		m.setCommandInput("")
	default:
		if len(msg.Runes) > 0 && !msg.Alt {
			m.setCommandInput(m.commandInput + string(msg.Runes))
		}
	}
	return m, nil
}

func (m Model) updateCreateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatches(msg, m.keys.Browse.Quit):
		m.mode = modeBrowse
		m.createText = ""
		m.createPath = ""
		m.createMetadata = tmux.SessionMetadata{}
	case keyMatches(msg, m.keys.Browse.OpenSelected):
		name := strings.TrimSpace(m.createText)
		if name == "" {
			m.notice = fmt.Errorf("session name is empty")
			return m, nil
		}
		if m.hasSession(name) {
			m.notice = fmt.Errorf("session name already exists: %s", name)
			return m, nil
		}
		path := strings.TrimSpace(m.createPath)
		if path == "" {
			m.notice = fmt.Errorf("selected item has no path")
			return m, nil
		}
		metadata := m.createMetadata
		metadata.BaseName = sanitizeSessionName(name)
		m.loading = true
		m.mode = modeBrowse
		m.createText = ""
		m.createPath = ""
		m.createMetadata = tmux.SessionMetadata{}
		return m, m.createSessionWithMetadata(name, path, metadata)
	case keyMatches(msg, m.keys.Browse.DeleteChar):
		m.createText = deleteLastRune(m.createText)
	case keyMatches(msg, m.keys.Browse.DeleteWord):
		m.createText = deleteLastShellWord(m.createText)
	case keyMatches(msg, m.keys.Browse.ClearInput):
		m.createText = ""
	default:
		if len(msg.Runes) > 0 {
			m.createText += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) updateRenameKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatches(msg, m.keys.Browse.Quit):
		m.mode = modeBrowse
		m.renameText = ""
		m.renameOriginal = ""
	case keyMatches(msg, m.keys.Browse.OpenSelected):
		name := strings.TrimSpace(m.renameText)
		if name == "" {
			m.notice = fmt.Errorf("session name is empty")
			return m, nil
		}
		if name == m.renameOriginal {
			m.mode = modeBrowse
			m.renameText = ""
			m.renameOriginal = ""
			return m, nil
		}
		if m.hasSessionExcept(name, m.renameOriginal) {
			m.notice = fmt.Errorf("session name already exists: %s", name)
			return m, nil
		}
		m.loading = true
		m.mode = modeBrowse
		original := m.renameOriginal
		m.renameText = ""
		m.renameOriginal = ""
		return m, m.renameSession(original, name)
	case keyMatches(msg, m.keys.Browse.DeleteChar):
		m.renameText = deleteLastRune(m.renameText)
	case keyMatches(msg, m.keys.Browse.DeleteWord):
		m.renameText = deleteLastShellWord(m.renameText)
	case keyMatches(msg, m.keys.Browse.ClearInput):
		m.renameText = ""
	default:
		if len(msg.Runes) > 0 {
			m.renameText += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) updateTemplateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.templateFiltered
	switch {
	case keyMatches(msg, m.keys.Browse.Quit):
		m.closeTemplatePicker()
	case keyMatches(msg, m.keys.Browse.OpenSelected):
		if len(items) == 0 || m.templateCursor < 0 || m.templateCursor >= len(items) {
			return m, nil
		}
		template := items[m.templateCursor]
		metadata := m.templateMetadata
		path := m.templatePath
		fallbackName := m.templateName
		previous := m.templatePreviousMode
		if isNoTemplatePickerItem(template) {
			m.closeTemplatePicker()
			m.loading = true
			if previous == modePathSearch {
				m.stopPathStream()
				m.mode = modeBrowse
			}
			name := m.availableSessionName(fallbackName)
			metadata.BaseName = fallbackName
			return m, m.createSessionWithMetadata(name, path, metadata)
		}
		if len(template.Parameters) > 0 {
			return m.beginTemplateCreation(template, fallbackName, path, metadata, modeTemplatePicker)
		}
		m.closeTemplatePicker()
		return m.beginTemplateCreation(template, fallbackName, path, metadata, previous)
	case keyMatches(msg, m.keys.Browse.Up):
		if m.templateCursor > 0 {
			m.templateCursor--
			m.ensureTemplateCursorVisible()
		}
	case keyMatches(msg, m.keys.Browse.Down):
		if m.templateCursor < len(items)-1 {
			m.templateCursor++
			m.ensureTemplateCursorVisible()
		}
	case keyMatches(msg, m.keys.Browse.PageUp):
		m.moveTemplateCursor(-halfPageStep(templateListHeightForFrame(panelDialogFrame(m.dialogs, m.innerWidth(), m.innerHeight()))))
	case keyMatches(msg, m.keys.Browse.PageDown):
		m.moveTemplateCursor(halfPageStep(templateListHeightForFrame(panelDialogFrame(m.dialogs, m.innerWidth(), m.innerHeight()))))
	case keyMatches(msg, m.keys.Browse.ScrollUp):
		m.scrollTemplateViewport(-1)
	case keyMatches(msg, m.keys.Browse.ScrollDown):
		m.scrollTemplateViewport(1)
	case keyMatches(msg, m.keys.Browse.JumpNextSection):
		m.jumpTemplateSection(1)
	case keyMatches(msg, m.keys.Browse.JumpPreviousSection):
		m.jumpTemplateSection(-1)
	case keyMatches(msg, m.keys.Browse.DeleteChar):
		m.setTemplateFilter(deleteLastRune(m.templateFilter))
	case keyMatches(msg, m.keys.Browse.DeleteWord):
		m.setTemplateFilter(deleteLastShellWord(m.templateFilter))
	case keyMatches(msg, m.keys.Browse.ClearInput):
		m.setTemplateFilter("")
	default:
		if len(msg.Runes) > 0 && !msg.Alt {
			m.setTemplateFilter(m.templateFilter + string(msg.Runes))
		}
	}
	return m, nil
}

func (m *Model) closeTemplatePicker() {
	if m.templatePreviousMode == modePathSearch {
		m.mode = modePathSearch
	} else {
		m.mode = modeBrowse
	}
	m.templatePath = ""
	m.templateName = ""
	m.templateMetadata = tmux.SessionMetadata{}
	m.templateAvailable = nil
	m.templateFilter = ""
	m.templateFiltered = nil
	m.templateCursor = 0
	m.templateScroll = 0
	m.templateRestore = 0
	m.templatePreviousMode = modeBrowse
}

func (m Model) openTemplatePicker() (tea.Model, tea.Cmd) {
	selected, ok := m.selected()
	if !ok {
		m.notice = fmt.Errorf("there is no selected candidate")
		return m, nil
	}
	return m.openTemplatePickerForCandidate(selected, modeBrowse)
}

func (m Model) openPathTemplatePicker() (tea.Model, tea.Cmd) {
	selected, ok := m.selectedPath()
	if !ok {
		m.pathNotice = fmt.Errorf("there is no selected path result")
		return m, nil
	}
	return m.openTemplatePickerForCandidate(selected, modePathSearch)
}

func (m Model) openTemplatePickerForCandidate(selected candidate, previous mode) (tea.Model, tea.Cmd) {
	if selected.kind == candidateSession {
		m.notice = fmt.Errorf("selected item is already an open tmux session")
		return m, nil
	}
	path, metadata, ok := m.namedSessionTarget(selected)
	if !ok {
		m.notice = fmt.Errorf("selected item has no path")
		return m, nil
	}
	if session, ok := m.sessionForPath(path); ok {
		m.notice = fmt.Errorf("tmux session already exists for %s: %s", path, session.Name)
		return m, nil
	}
	m.mode = modeTemplatePicker
	m.templatePreviousMode = previous
	m.templatePath = path
	m.templateName = selected.sessionName()
	m.templateMetadata = metadata
	m.templateFilter = ""
	templates, err := m.templatesForPath(path)
	if err != nil {
		m.notice = err
		return m, nil
	}
	m.templateAvailable = templates
	m.templateFiltered = templatePickerItems(m.templateAvailable)
	m.templateCursor = 0
	m.templateScroll = 0
	m.templateRestore = 0
	m.applyTemplateFilter()
	return m, nil
}

func (m *Model) ensureTemplateCursorVisible() {
	limit := templateListHeightForFrame(panelDialogFrame(m.dialogs, m.innerWidth(), m.innerHeight()))
	templates := m.templateFiltered
	cursorRow := templateDisplayRow(templates, m.templateCursor)
	scrollRow := templateScrollDisplayRow(templates, m.templateScroll)
	if cursorRow < scrollRow {
		m.templateScroll = m.templateCursor
		return
	}
	for cursorRow-scrollRow+1 > limit && m.templateScroll < m.templateCursor && m.templateScroll < len(templates) {
		m.templateScroll++
		scrollRow = templateScrollDisplayRow(templates, m.templateScroll)
	}
}

func (m *Model) jumpTemplateSection(direction int) {
	starts := templateSectionStartIndexes(m.templateFiltered)
	if len(starts) == 0 {
		return
	}
	if direction >= 0 {
		for _, start := range starts {
			if start > m.templateCursor {
				m.templateCursor = start
				m.ensureTemplateCursorVisible()
				return
			}
		}
		m.templateCursor = starts[0]
		m.ensureTemplateCursorVisible()
		return
	}
	for i := len(starts) - 1; i >= 0; i-- {
		if starts[i] < m.templateCursor {
			m.templateCursor = starts[i]
			m.ensureTemplateCursorVisible()
			return
		}
	}
	m.templateCursor = starts[len(starts)-1]
	m.ensureTemplateCursorVisible()
}

func templateSectionStartIndexes(templates []sessionconfig.Template) []int {
	starts := make([]int, 0, 2)
	for i := range templates {
		if templateStartsSection(templates, i) {
			starts = append(starts, i)
		}
	}
	return starts
}

func (m *Model) moveTemplateCursor(delta int) {
	if len(m.templateFiltered) == 0 {
		m.templateCursor = 0
		m.templateScroll = 0
		return
	}
	m.templateCursor += delta
	if m.templateCursor < 0 {
		m.templateCursor = 0
	}
	if m.templateCursor >= len(m.templateFiltered) {
		m.templateCursor = len(m.templateFiltered) - 1
	}
	m.ensureTemplateCursorVisible()
}

func (m *Model) scrollTemplateViewport(delta int) {
	if len(m.templateFiltered) == 0 {
		return
	}
	m.templateScroll += delta
	if m.templateScroll < 0 {
		m.templateScroll = 0
	}
	maxScroll := len(m.templateFiltered) - 1
	if m.templateScroll > maxScroll {
		m.templateScroll = maxScroll
	}
	if m.templateCursor < m.templateScroll {
		m.templateCursor = m.templateScroll
	}
	limit := templateListHeightForFrame(panelDialogFrame(m.dialogs, m.innerWidth(), m.innerHeight()))
	if m.templateCursor >= m.templateScroll+limit {
		m.templateCursor = m.templateScroll + limit - 1
	}
}

func (m Model) updatePathSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatches(msg, m.keys.PathSearch.Close):
		m.stopPathStream()
		m.mode = modeBrowse
		return m, nil
	case keyMatches(msg, m.keys.PathSearch.CommandPalette):
		m.openCommands(modePathSearch)
		return m, nil
	case keyMatches(msg, m.keys.PathSearch.OpenLastSession):
		m.stopPathStream()
		m.mode = modeBrowse
		return m, m.switchLastSession()
	case keyMatches(msg, m.keys.PathSearch.TemplateSession):
		return m.openPathTemplatePicker()
	case keyMatches(msg, m.keys.PathSearch.Help):
		m.openHelp(modePathSearch)
		return m, nil
	case keyMatches(msg, m.keys.PathSearch.Reload):
		m.pathBusy = true
		m.clearPathCompletion()
		return m, m.startPathStream(m.pathRoot)
	case keyMatches(msg, m.keys.PathSearch.CycleRoot):
		m.cyclePathRoot()
		return m, m.startPathStream(m.pathRoot)
	case keyMatches(msg, m.keys.PathSearch.ToggleHidden):
		m.pathConfig.options.SkipHidden = !m.pathConfig.options.SkipHidden
		m.pathBusy = true
		m.clearPathCompletion()
		return m, m.startPathStream(m.pathRoot)
	case keyMatches(msg, m.keys.PathSearch.ToggleIgnored):
		m.pathConfig.options.SkipGitignored = !m.pathConfig.options.SkipGitignored
		m.pathBusy = true
		m.clearPathCompletion()
		return m, m.startPathStream(m.pathRoot)
	case keyMatches(msg, m.keys.PathSearch.OpenTyped):
		selected, ok := m.typedPathCandidate()
		if !ok {
			m.pathErr = errPathInputUnavailable(m.pathInput)
			m.pathNotice = m.pathErr
			return m, nil
		}
		m.loading = true
		m.stopPathStream()
		m.mode = modeBrowse
		return m.openCandidate(selected)
	case keyMatches(msg, m.keys.PathSearch.CreateTyped):
		return m.openCreateTypedPathConfirmation()
	case keyMatches(msg, m.keys.PathSearch.CompleteNext):
		if m.hasPathCompletionCycle() {
			m.advancePathCompletion(1)
			return m, m.startPathStreamPreserveCompletion(m.pathRoot)
		}
		return m, m.loadPathCompletions(1)
	case keyMatches(msg, m.keys.PathSearch.CompletePrevious):
		if m.hasPathCompletionCycle() {
			m.advancePathCompletion(-1)
			return m, m.startPathStreamPreserveCompletion(m.pathRoot)
		}
		return m, m.loadPathCompletions(-1)
	case keyMatches(msg, m.keys.PathSearch.AcceptCompletion):
		if m.hasPathCompletionCycle() {
			m.clearPathCompletion()
		}
	case keyMatches(msg, m.keys.PathSearch.Up):
		if m.pathCursor > 0 {
			m.pathCursor--
			m.ensurePathCursorVisible()
		}
	case keyMatches(msg, m.keys.PathSearch.Down):
		if m.pathCursor < len(m.pathResult)-1 {
			m.pathCursor++
			m.ensurePathCursorVisible()
		}
	case keyMatches(msg, m.keys.PathSearch.PageUp):
		m.movePathCursor(-halfPageStep(m.pathListLimit()))
	case keyMatches(msg, m.keys.PathSearch.PageDown):
		m.movePathCursor(halfPageStep(m.pathListLimit()))
	case keyMatches(msg, m.keys.PathSearch.ScrollUp):
		m.scrollPathViewport(-1)
	case keyMatches(msg, m.keys.PathSearch.ScrollDown):
		m.scrollPathViewport(1)
	case keyMatches(msg, m.keys.PathSearch.OpenSelected):
		selected, ok := m.selectedPath()
		if !ok {
			return m, nil
		}
		m.loading = true
		m.stopPathStream()
		m.mode = modeBrowse
		return m.openCandidate(selected)
	case keyMatches(msg, m.keys.PathSearch.DeleteChar):
		return m.updatePathInput(deleteLastRune(m.pathInput))
	case keyMatches(msg, m.keys.PathSearch.DeleteWord):
		return m.updatePathInput(deleteLastShellWord(m.pathInput))
	case keyMatches(msg, m.keys.PathSearch.ClearInput):
		return m.updatePathInput("")
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

	appendAnchoredFooter(&b, renderInsetRow(renderFooterAlignedRow(renderStatusFooter(false, m.discovery.SkipHidden, m.discovery.SkipGitignored, "", contentWidth-searchBoxInnerOffset, s), s), m.innerWidth(), s), m.innerHeight())
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

	footerHelp := ""
	if m.pathErr != nil {
		footerHelp = "error: " + m.pathErr.Error()
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
		base = placeCenteredOverlay(base, renderConfirmKillWithKeys(m.styles, m.dialogs, m.keys, m.confirmKillSessionName(), m.confirmChoice, innerWidth, innerHeight), innerWidth, innerHeight)
	}
	if m.mode == modeConfirmCreatePath {
		base = placeCenteredOverlay(base, renderConfirmCreatePathWithKeys(m.styles, m.dialogs, m.keys, m.createPathInput, m.confirmChoice, innerWidth, innerHeight), innerWidth, innerHeight)
	}
	if m.mode == modeCreateSession {
		base = placeCenteredOverlay(base, renderCreateSessionWithKeys(m.styles, m.dialogs, m.keys, m.createText, innerWidth, innerHeight), innerWidth, innerHeight)
	}
	if m.mode == modeRenameSession {
		base = placeCenteredOverlay(base, renderRenameSessionWithKeys(m.styles, m.dialogs, m.keys, m.renameText, innerWidth, innerHeight), innerWidth, innerHeight)
	}
	if m.mode == modeTemplatePicker {
		base = placeOverlay(base, renderTemplatePicker(m.styles, m.dialogs, m.keys, m.templateFiltered, m.templateFilter, m.templateCursor, m.templateScroll, innerWidth, innerHeight), innerWidth, innerHeight)
	}
	if m.mode == modeTemplateParameter {
		if parameter, ok := m.currentTemplateParameter(); ok {
			base = placeOverlay(base, renderTemplateParameterPicker(
				m.styles,
				m.dialogs,
				parameter,
				m.parameterIndex,
				len(m.parameterTemplate.Parameters),
				m.parameterCursor,
				keyListLabel(m.keys.Browse.OpenSelected),
				keyListLabel(m.keys.Browse.Quit),
				m.parameterPreviousMode == modeTemplatePicker,
				innerWidth,
				innerHeight,
			), innerWidth, innerHeight)
		}
	}
	if m.mode == modeCommands || (m.mode == modeHelp && m.previousMode == modeCommands) {
		base = placeOverlay(base, renderCommandPalette(m.styles, m.dialogs, m.commandMatches(), m.commandInput, m.commandCursor, m.commandScroll, innerWidth, innerHeight), innerWidth, innerHeight)
	}
	if m.mode == modeHelp {
		base = placeOverlay(base, renderHelpPanelWithKeys(m.styles, m.dialogs, m.keys, m.previousMode, m.helpInput, m.helpCursor, m.helpScroll, innerWidth, innerHeight), innerWidth, innerHeight)
	}
	if m.notice != nil {
		base = placeCenteredOverlay(base, renderPathNoticePopupWithKeys(m.styles, m.dialogs, m.keys.Browse.OpenSelected, dismissKeys(m.keys.Browse.Quit), m.notice.Error(), innerWidth, innerHeight), innerWidth, innerHeight)
	}
	if m.showsPathSearchBase() && m.pathNotice != nil {
		base = placeCenteredOverlay(base, renderPathNoticePopupWithKeys(m.styles, m.dialogs, m.keys.PathSearch.OpenSelected, dismissKeys(m.keys.PathSearch.Close), m.pathNotice.Error(), innerWidth, innerHeight), innerWidth, innerHeight)
	}
	if message, ok := m.errorMessage(); ok {
		base = placeCenteredOverlay(base, renderErrorPopupWithKeys(m.styles, m.dialogs, dismissKeys(m.keys.Browse.Quit), m.keys.Browse.Reload, message, innerWidth, innerHeight), innerWidth, innerHeight)
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
	m.updateBrowseFilter(deleteLastRune(m.filter))
}

func (m *Model) removeBrowseFilterWord() {
	m.updateBrowseFilter(deleteLastShellWord(m.filter))
}

func (m *Model) clearBrowseFilter() {
	m.updateBrowseFilter("")
}

func (m *Model) updateBrowseFilter(value string) {
	if value == m.filter {
		return
	}
	m.filter = value
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

func (m Model) updatePathInput(value string) (tea.Model, tea.Cmd) {
	if value == m.pathInput {
		return m, nil
	}
	m.pathInput = value
	m.clearPathCompletion()
	return m, m.applyPathInputChange()
}

func deleteLastRune(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	return string(runes[:len(runes)-1])
}

func deleteLastShellWord(value string) string {
	runes := []rune(value)
	end := len(runes)
	if end == 0 {
		return ""
	}
	for end > 0 && unicode.IsSpace(runes[end-1]) {
		end--
	}
	if end == 0 {
		return ""
	}
	if isShellWordSeparator(runes[end-1]) {
		return string(runes[:end-1])
	}
	for end > 0 && !isShellWordSeparator(runes[end-1]) {
		end--
	}
	return string(runes[:end])
}

func isShellWordSeparator(value rune) bool {
	switch value {
	case '/', '\\', '.', '-', '_', ':':
		return true
	default:
		return unicode.IsSpace(value)
	}
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
	m.pathNotice = nil
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

func (m Model) missingTypedPathCandidate() (candidate, bool, error) {
	return typedMissingPathCandidate(m.pathInput)
}

func errPathInputUnavailable(input string) error {
	if strings.TrimSpace(input) == "" {
		return fmt.Errorf("typed path is empty")
	}
	return fmt.Errorf("typed path is not an available directory: %s", input)
}

func errPathCreateUnavailable(input string) error {
	if strings.TrimSpace(input) == "" {
		return fmt.Errorf("typed path is empty")
	}
	return fmt.Errorf("typed path cannot be created: %s", input)
}

func errPathAlreadyExists(input string) error {
	return fmt.Errorf("typed path already exists: %s", input)
}

func errPathAlreadyExistsAsFile(input string) error {
	return fmt.Errorf("typed path already exists and is not a directory: %s", input)
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

func (m Model) openRenameSession() (tea.Model, tea.Cmd) {
	selected, ok := m.selected()
	if !ok || selected.kind != candidateSession {
		m.notice = fmt.Errorf("selected item is not an open tmux session")
		return m, nil
	}
	m.mode = modeRenameSession
	m.renameOriginal = selected.session.Name
	m.renameText = selected.session.Name
	return m, nil
}

func (m *Model) openCreateSession() {
	selected, ok := m.selected()
	if !ok {
		return
	}
	path, metadata, ok := m.namedSessionTarget(selected)
	if !ok {
		m.notice = fmt.Errorf("selected item has no path")
		return
	}
	m.mode = modeCreateSession
	m.createText = selected.sessionName()
	m.createPath = path
	m.createMetadata = metadata
}

func (m Model) namedSessionTarget(selected candidate) (string, tmux.SessionMetadata, bool) {
	switch selected.kind {
	case candidateSession:
		path := sessionDisplayPath(selected.session)
		if strings.TrimSpace(path) == "" {
			return "", tmux.SessionMetadata{}, false
		}
		metadata := selected.session.Metadata
		metadata.CreatedByParator = true
		metadata.Kind = originLabel(candidateOrigin(selected))
		if strings.TrimSpace(metadata.Path) == "" {
			metadata.Path = path
		}
		if strings.TrimSpace(metadata.Glyph) == "" {
			metadata.Glyph = originGlyph(metadata.Kind, m.glyphs)
		}
		if strings.TrimSpace(metadata.GlyphColor) == "" {
			metadata.GlyphColor = string(originGlyphColor(metadata.Kind, false, m.glyphColors))
		}
		return path, metadata, true
	case candidateRoot, candidatePath:
		path := selected.path()
		if strings.TrimSpace(path) == "" {
			return "", tmux.SessionMetadata{}, false
		}
		metadata := selected.sessionMetadata()
		metadata.CreatedByParator = true
		if strings.TrimSpace(metadata.Glyph) == "" {
			metadata.Glyph = originGlyph(metadata.Kind, m.glyphs)
		}
		if strings.TrimSpace(metadata.GlyphColor) == "" {
			metadata.GlyphColor = string(originGlyphColor(metadata.Kind, false, m.glyphColors))
		}
		return path, metadata, true
	default:
		return "", tmux.SessionMetadata{}, false
	}
}

func (m Model) openCreateTypedPathConfirmation() (tea.Model, tea.Cmd) {
	if _, ok, err := m.missingTypedPathCandidate(); !ok {
		m.pathErr = err
		m.pathNotice = err
		return m, nil
	}
	m.mode = modeConfirmCreatePath
	m.createPathInput = strings.TrimSpace(m.pathInput)
	m.confirmChoice = confirmCancel
	return m, nil
}

func (m Model) confirmCreateTypedPath() (tea.Model, tea.Cmd) {
	pathInput := strings.TrimSpace(m.createPathInput)
	if pathInput == "" {
		pathInput = strings.TrimSpace(m.pathInput)
	}
	candidate, ok, err := typedMissingPathCandidate(pathInput)
	if !ok {
		m.mode = modePathSearch
		m.confirmChoice = confirmCancel
		m.pathErr = err
		m.pathNotice = err
		return m, nil
	}
	if err := os.MkdirAll(candidate.path(), 0o755); err != nil {
		m.mode = modePathSearch
		m.confirmChoice = confirmCancel
		m.pathErr = fmt.Errorf("create typed path %s: %w", pathInput, err)
		m.pathNotice = m.pathErr
		return m, nil
	}
	m.loading = true
	m.confirmChoice = confirmCancel
	m.stopPathStream()
	m.mode = modeBrowse
	return m.openCandidate(candidate)
}

func typedMissingPathCandidate(pathInput string) (candidate, bool, error) {
	pathInput = strings.TrimSpace(pathInput)
	if pathInput == "" {
		return candidate{}, false, errPathCreateUnavailable(pathInput)
	}
	expanded, err := pathsearch.ExpandRoot(pathInput)
	if err != nil {
		return candidate{}, false, err
	}
	info, err := os.Stat(expanded)
	if err == nil {
		if info.IsDir() {
			return candidate{}, false, errPathAlreadyExists(pathInput)
		}
		return candidate{}, false, errPathAlreadyExistsAsFile(pathInput)
	}
	if !os.IsNotExist(err) {
		if !errors.Is(err, syscall.ENOTDIR) {
			return candidate{}, false, err
		}
	}
	return candidate{kind: candidatePath, fsPath: pathsearch.Candidate{Name: filepath.Base(expanded), Path: expanded}}, true, nil
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
	if m.notice != nil {
		m.notice = nil
		return true
	}
	if m.showsPathSearchBase() && m.pathNotice != nil {
		m.pathNotice = nil
		return true
	}
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

func halfPageStep(limit int) int {
	step := limit / 2
	if step < 1 {
		return 1
	}
	return step
}

func clampIndex(value int, total int) int {
	if total <= 0 {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value >= total {
		return total - 1
	}
	return value
}

func maxSimpleScroll(total int, visible int) int {
	if total <= 0 || visible >= total {
		return 0
	}
	return total - visible
}

func adjustCursorToSimpleViewport(cursor int, scroll int, visible int, total int) int {
	cursor = clampIndex(cursor, total)
	if total == 0 {
		return 0
	}
	if visible < 1 {
		visible = 1
	}
	if cursor < scroll {
		return scroll
	}
	lastVisible := scroll + visible - 1
	if cursor > lastVisible {
		return clampIndex(lastVisible, total)
	}
	return cursor
}

func (m *Model) moveBrowseCursor(delta int) {
	if len(m.filtered) == 0 {
		m.cursor = 0
		m.scroll = 0
		return
	}
	m.cursor = clampIndex(m.cursor+delta, len(m.filtered))
	m.ensureCursorVisible()
}

func (m *Model) movePathCursor(delta int) {
	if len(m.pathResult) == 0 {
		m.pathCursor = 0
		m.pathScroll = 0
		return
	}
	m.pathCursor = clampIndex(m.pathCursor+delta, len(m.pathResult))
	m.ensurePathCursorVisible()
}

func (m *Model) moveCommandCursor(delta int) {
	items := m.commandMatches()
	if len(items) == 0 {
		m.commandCursor = 0
		m.commandScroll = 0
		return
	}
	m.commandCursor = clampIndex(m.commandCursor+delta, len(items))
	m.ensureCommandCursorVisible()
}

func (m *Model) moveHelpCursor(delta int) {
	items := m.helpMatches()
	if len(items) == 0 {
		m.helpCursor = 0
		m.helpScroll = 0
		return
	}
	m.helpCursor = clampIndex(m.helpCursor+delta, len(items))
	m.ensureHelpCursorVisible()
}

func (m *Model) scrollBrowseViewport(direction int) {
	if len(m.filtered) == 0 {
		m.cursor = 0
		m.scroll = 0
		return
	}
	if direction < 0 && m.scroll > 0 {
		m.scroll--
	}
	if direction > 0 && m.scroll < len(m.filtered)-1 {
		m.scroll++
	}
	m.cursor = clampIndex(m.cursor, len(m.filtered))
	if m.cursor < m.scroll {
		m.cursor = m.scroll
	}
	limit := m.listLimit()
	for m.cursorRowsFromScroll(limit) > limit && m.cursor > m.scroll {
		m.cursor--
	}
}

func (m *Model) scrollPathViewport(direction int) {
	visible := m.pathListLimit()
	total := len(m.pathResult)
	if total == 0 {
		m.pathCursor = 0
		m.pathScroll = 0
		return
	}
	m.pathScroll = clampIndex(m.pathScroll+direction, maxSimpleScroll(total, visible)+1)
	m.pathCursor = adjustCursorToSimpleViewport(m.pathCursor, m.pathScroll, visible, total)
}

func (m *Model) scrollCommandViewport(direction int) {
	items := m.commandMatches()
	visible := commandListHeightForFrame(panelDialogFrame(m.dialogs, m.innerWidth(), m.innerHeight()))
	total := len(items)
	if total == 0 {
		m.commandCursor = 0
		m.commandScroll = 0
		return
	}
	m.commandScroll = clampIndex(m.commandScroll+direction, maxSimpleScroll(total, visible)+1)
	m.commandCursor = adjustCursorToSimpleViewport(m.commandCursor, m.commandScroll, visible, total)
}

func (m *Model) scrollHelpViewport(direction int) {
	items := m.helpMatches()
	visible := commandListHeightForFrame(panelDialogFrame(m.dialogs, m.innerWidth(), m.innerHeight()))
	total := len(items)
	if total == 0 {
		m.helpCursor = 0
		m.helpScroll = 0
		return
	}
	m.helpScroll = clampIndex(m.helpScroll+direction, maxSimpleScroll(total, visible)+1)
	m.helpCursor = adjustCursorToSimpleViewport(m.helpCursor, m.helpScroll, visible, total)
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
	for m.pathCursorRowsFromScroll(limit) > limit && m.pathScroll < m.pathCursor {
		m.pathScroll++
	}
}

func (m Model) cursorRowsFromScroll(limit int) int {
	rows := m.browseFixedRows()
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

func (m Model) pathCursorRowsFromScroll(limit int) int {
	rows := m.pathFixedRows()
	if m.showPathTopOverflow(limit) {
		rows++
	}
	return rows + m.pathCursor - m.pathScroll + 1
}

func (m Model) browseFixedRows() int {
	if hasVisibleBrowseColumns(m.renderColumns(m.filtered)) {
		return 1
	}
	return 0
}

func (m Model) pathFixedRows() int {
	rows := 1
	if hasVisibleBrowseColumns(m.renderColumns(m.pathResult)) {
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
	if m.mode == modeConfirmCreatePath {
		return true
	}
	if m.mode == modeTemplateParameter && m.parameterPreviousMode == modePathSearch {
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
	m.commandRestore = 0
}

func (m *Model) ensureCommandCursorVisible() {
	limit := commandListHeightForFrame(panelDialogFrame(m.dialogs, m.innerWidth(), m.innerHeight()))
	if m.commandCursor < m.commandScroll {
		m.commandScroll = m.commandCursor
		return
	}
	for m.commandCursor-m.commandScroll+1 > limit && m.commandScroll < m.commandCursor {
		m.commandScroll++
	}
}

func (m *Model) setCommandInput(input string) {
	previous := m.commandInput
	m.commandInput = input
	m.commandCursor, m.commandScroll, m.commandRestore = popupFilterSelection(
		previous,
		input,
		m.commandCursor,
		m.commandScroll,
		m.commandRestore,
		len(m.commandMatches()),
	)
	m.ensureCommandCursorVisible()
}

func (m *Model) openHelp(previous mode) {
	m.previousMode = previous
	m.mode = modeHelp
	m.helpCursor = 0
	m.helpScroll = 0
	m.helpInput = ""
	m.helpRestore = 0
}

func (m *Model) ensureHelpCursorVisible() {
	limit := commandListHeightForFrame(panelDialogFrame(m.dialogs, m.innerWidth(), m.innerHeight()))
	if m.helpCursor < m.helpScroll {
		m.helpScroll = m.helpCursor
		return
	}
	for m.helpCursor-m.helpScroll+1 > limit && m.helpScroll < m.helpCursor {
		m.helpScroll++
	}
}

func (m *Model) setHelpInput(input string) {
	previous := m.helpInput
	m.helpInput = input
	m.helpCursor, m.helpScroll, m.helpRestore = popupFilterSelection(
		previous,
		input,
		m.helpCursor,
		m.helpScroll,
		m.helpRestore,
		len(m.helpMatches()),
	)
	m.ensureHelpCursorVisible()
}

func popupFilterSelection(previous string, next string, cursor int, scroll int, restore int, total int) (int, int, int) {
	wasFiltering := strings.TrimSpace(previous) != ""
	isFiltering := strings.TrimSpace(next) != ""
	if !wasFiltering && isFiltering {
		restore = cursor
	}
	if isFiltering {
		return 0, 0, restore
	}
	if wasFiltering {
		return clampIndex(restore, total), 0, restore
	}
	return clampIndex(cursor, total), clampIndex(scroll, total), restore
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

func (m Model) renameSession(oldName string, newName string) tea.Cmd {
	return func() tea.Msg {
		return renamedMsg{err: m.client.RenameSession(context.Background(), oldName, newName)}
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

func (m Model) createSessionWithTemplate(name string, path string, metadata tmux.SessionMetadata, template sessionconfig.Template) tea.Cmd {
	return func() tea.Msg {
		return createdMsg{name: name, switched: true, err: m.client.NewSessionWithLayoutAndSwitch(context.Background(), name, path, metadata, template)}
	}
}

func (m *Model) clearPendingTemplate() {
	m.pendingTemplateName = ""
	m.pendingTemplatePath = ""
	m.pendingTemplateMeta = tmux.SessionMetadata{}
	m.pendingTemplate = sessionconfig.Template{}
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
		m.pathNotice = nil
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
	m.pathNotice = nil
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
	m.pathNotice = nil
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
	m.pathNotice = nil
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

func (m Model) openCandidate(selected candidate) (tea.Model, tea.Cmd) {
	if selected.kind == candidateRoot || selected.kind == candidatePath {
		if session, ok := m.sessionForPath(selected.path()); ok {
			return m, m.tagAndSwitchSession(session.Name, selected.sessionMetadata())
		}
		metadata := selected.sessionMetadata()
		if selected.kind == candidatePath && metadata.Glyph == "" {
			metadata.Glyph = m.glyphs.Path
		}
		if selected.kind == candidatePath && metadata.GlyphColor == "" {
			metadata.GlyphColor = m.glyphColors.Path
		}
		if template, templatePath, ok, err := m.localTemplateForPath(selected.path()); err != nil {
			m.notice = err
			return m, nil
		} else if ok {
			m.pendingTemplateName = selected.sessionName()
			m.pendingTemplatePath = selected.path()
			m.pendingTemplateMeta = metadata
			m.pendingTemplate = template
			m.notice = fmt.Errorf("local tmux-parator template found: %s", templatePath)
			return m, nil
		}
		if template, ok := sessionconfig.MatchingTemplate(m.matchingTemplates, selected.path()); ok {
			return m.beginTemplateCreation(template, selected.sessionName(), selected.path(), metadata, modeBrowse)
		}
		name := m.availableSessionName(selected.sessionName())
		metadata.BaseName = selected.sessionName()
		return m, m.createSessionWithMetadata(name, selected.path(), metadata)
	}
	return m, m.switchSession(selected.session.Name)
}

func (m Model) templatesForPath(path string) ([]sessionconfig.Template, error) {
	templates := make([]sessionconfig.Template, 0, len(m.templates)+1)
	if template, _, ok, err := m.localTemplateForPath(path); err != nil {
		return nil, err
	} else if ok {
		templates = append(templates, template)
	}
	templates = append(templates, m.templates...)
	return templates, nil
}

func (m Model) localTemplateForPath(path string) (sessionconfig.Template, string, bool, error) {
	template, templatePath, ok, err := sessionconfig.LoadLocal(path)
	if err != nil {
		return sessionconfig.Template{}, templatePath, ok, fmt.Errorf("load local tmux-parator template: %w", err)
	}
	return template, templatePath, ok, nil
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

func (m Model) availableTemplateSessionName(template sessionconfig.Template, fallback string, path string, metadata tmux.SessionMetadata) (string, string, error) {
	base, err := sessionconfig.ResolveSessionName(template, sessionconfig.RenderContext{
		WorkspacePath: path,
		RepoRoot:      metadata.Root,
		SessionKind:   metadata.Kind,
	})
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(base) == "" {
		base = fallback
	}
	base = sanitizeSessionName(base)
	return m.availableSessionName(base), base, nil
}

func (m Model) beginTemplateCreation(template sessionconfig.Template, fallback string, path string, metadata tmux.SessionMetadata, previous mode) (tea.Model, tea.Cmd) {
	if len(template.Parameters) > 0 {
		m.mode = modeTemplateParameter
		m.loading = false
		m.parameterTemplate = template
		m.parameterPath = path
		m.parameterFallback = fallback
		m.parameterMetadata = metadata
		m.parameterValues = make(map[string]string, len(template.Parameters))
		m.parameterIndex = 0
		m.parameterCursor = parameterDefaultIndex(template.Parameters[0])
		m.parameterPreviousMode = previous
		return m, nil
	}
	return m.finishTemplateCreation(template, fallback, path, metadata, previous)
}

func (m Model) finishTemplateCreation(template sessionconfig.Template, fallback string, path string, metadata tmux.SessionMetadata, previous mode) (tea.Model, tea.Cmd) {
	name, baseName, err := m.availableTemplateSessionName(template, fallback, path, metadata)
	if err != nil {
		m.loading = false
		if previous == modePathSearch {
			m.mode = modePathSearch
		} else {
			m.mode = modeBrowse
		}
		m.notice = err
		return m, nil
	}
	metadata.BaseName = baseName
	m.loading = true
	m.mode = modeBrowse
	if previous == modePathSearch {
		m.stopPathStream()
	}
	return m, m.createSessionWithTemplate(name, path, metadata, template)
}

func (m Model) updateTemplateParameterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	parameter, ok := m.currentTemplateParameter()
	if !ok {
		m.clearTemplateParameters()
		m.mode = modeBrowse
		return m, nil
	}
	switch {
	case keyMatches(msg, m.keys.Browse.Quit):
		if m.parameterIndex > 0 {
			delete(m.parameterValues, parameter.Name)
			m.parameterIndex--
			previous := m.parameterTemplate.Parameters[m.parameterIndex]
			m.parameterCursor = parameterSelectedIndex(previous, m.parameterValues)
			return m, nil
		}
		if m.parameterPreviousMode == modeTemplatePicker {
			m.clearTemplateParameters()
			m.mode = modeTemplatePicker
			return m, nil
		}
		previous := m.parameterPreviousMode
		m.clearTemplateParameters()
		if previous == modePathSearch {
			m.mode = modePathSearch
		} else {
			m.mode = modeBrowse
		}
	case keyMatches(msg, m.keys.Browse.Up):
		if m.parameterCursor > 0 {
			m.parameterCursor--
		}
	case keyMatches(msg, m.keys.Browse.Down):
		if m.parameterCursor < len(parameter.Options)-1 {
			m.parameterCursor++
		}
	case keyMatches(msg, m.keys.Browse.PageUp):
		m.parameterCursor -= halfPageStep(commandListHeightForFrame(panelDialogFrame(m.dialogs, m.innerWidth(), m.innerHeight())))
		if m.parameterCursor < 0 {
			m.parameterCursor = 0
		}
	case keyMatches(msg, m.keys.Browse.PageDown):
		m.parameterCursor += halfPageStep(commandListHeightForFrame(panelDialogFrame(m.dialogs, m.innerWidth(), m.innerHeight())))
		if m.parameterCursor >= len(parameter.Options) {
			m.parameterCursor = len(parameter.Options) - 1
		}
	case keyMatches(msg, m.keys.Browse.OpenSelected):
		if m.parameterCursor < 0 || m.parameterCursor >= len(parameter.Options) {
			return m, nil
		}
		m.parameterValues[parameter.Name] = parameter.Options[m.parameterCursor]
		m.parameterIndex++
		if m.parameterIndex < len(m.parameterTemplate.Parameters) {
			m.parameterCursor = parameterSelectedIndex(m.parameterTemplate.Parameters[m.parameterIndex], m.parameterValues)
			return m, nil
		}
		template, err := sessionconfig.WithParameterValues(m.parameterTemplate, m.parameterValues)
		if err != nil {
			m.notice = err
			previous := m.parameterPreviousMode
			m.clearTemplateParameters()
			if previous == modeTemplatePicker {
				m.mode = modeTemplatePicker
			} else if previous == modePathSearch {
				m.mode = modePathSearch
			} else {
				m.mode = modeBrowse
			}
			return m, nil
		}
		fallback := m.parameterFallback
		path := m.parameterPath
		metadata := m.parameterMetadata
		previous := m.parameterPreviousMode
		m.clearTemplateParameters()
		if previous == modeTemplatePicker {
			previous = m.templatePreviousMode
			m.closeTemplatePicker()
		}
		return m.finishTemplateCreation(template, fallback, path, metadata, previous)
	}
	return m, nil
}

func (m Model) currentTemplateParameter() (sessionconfig.Parameter, bool) {
	if m.parameterIndex < 0 || m.parameterIndex >= len(m.parameterTemplate.Parameters) {
		return sessionconfig.Parameter{}, false
	}
	return m.parameterTemplate.Parameters[m.parameterIndex], true
}

func (m *Model) clearTemplateParameters() {
	m.parameterTemplate = sessionconfig.Template{}
	m.parameterPath = ""
	m.parameterFallback = ""
	m.parameterMetadata = tmux.SessionMetadata{}
	m.parameterValues = nil
	m.parameterIndex = 0
	m.parameterCursor = 0
	m.parameterPreviousMode = modeBrowse
}

func parameterDefaultIndex(parameter sessionconfig.Parameter) int {
	for i, option := range parameter.Options {
		if option == parameter.Default {
			return i
		}
	}
	return 0
}

func parameterSelectedIndex(parameter sessionconfig.Parameter, values map[string]string) int {
	if value, ok := values[parameter.Name]; ok {
		for i, option := range parameter.Options {
			if option == value {
				return i
			}
		}
	}
	return parameterDefaultIndex(parameter)
}

func (m Model) hasSession(name string) bool {
	for _, session := range m.sessions {
		if session.Name == name {
			return true
		}
	}
	return false
}

func (m Model) hasSessionExcept(name string, except string) bool {
	for _, session := range m.sessions {
		if session.Name == except {
			continue
		}
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
	title = strings.ToUpper(title)
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
		chipStyle = s.selected
		chipGlyphBackground = s.selected.GetBackground()
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

type helpMatch struct {
	item          helpItem
	actionIndexes []int
	keyIndexes    []int
}

type commandID string

const (
	commandOpenSelected    commandID = "open-selected"
	commandOpenLast        commandID = "open-last-session"
	commandOpenTyped       commandID = "open-typed"
	commandCreateTyped     commandID = "create-typed-path"
	commandKillSession     commandID = "kill-session"
	commandRenameSession   commandID = "rename-session"
	commandNewSession      commandID = "new-session"
	commandTemplateSession commandID = "template-session"
	commandPathSearch      commandID = "path-search"
	commandCycleRoot       commandID = "cycle-root"
	commandReload          commandID = "reload"
	commandToggleHidden    commandID = "toggle-hidden"
	commandToggleIgnored   commandID = "toggle-ignored"
	commandHelp            commandID = "help"
	commandQuit            commandID = "quit"
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

type commandSpec struct {
	ID          commandID
	Title       string
	Key         string
	Description string
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
		specs := m.pathSearchCommandSpecs()
		templateReason := ""
		if len(m.pathResult) == 0 {
			templateReason = "There is no selected path result."
		} else if selected, ok := m.selectedPath(); ok {
			if path, _, targetOK := m.namedSessionTarget(selected); !targetOK {
				templateReason = "The selected path result has no path."
			} else if _, exists := m.sessionForPath(path); exists {
				templateReason = "A tmux session already exists for the selected path."
			}
		}
		typedReason := ""
		if _, ok := m.typedPathCandidate(); !ok {
			typedReason = "The typed prompt is not an existing directory."
		}
		createReason := ""
		if _, ok, err := m.missingTypedPathCandidate(); !ok {
			createReason = err.Error()
		}
		return []commandItem{
			commandItemFromSpec(specs[0], len(m.pathResult) > 0, "There is no selected path result."),
			commandItemFromSpec(specs[1], templateReason == "", templateReason),
			commandItemFromSpec(specs[2], typedReason == "", typedReason),
			commandItemFromSpec(specs[3], createReason == "", createReason),
			commandItemFromSpec(specs[4], true, ""),
			commandItemFromSpec(specs[5], true, ""),
			commandItemFromSpec(specs[6], true, ""),
			commandItemFromSpec(specs[7], true, ""),
			commandItemFromSpec(specs[8], true, ""),
			commandItemFromSpec(specs[9], true, ""),
		}
	}
	specs := m.browseCommandSpecs()
	selected, ok := m.selected()
	killReason := ""
	sessionReason := ""
	createNamedReason := ""
	templateReason := ""
	if !ok {
		killReason = "There is no selected candidate."
		sessionReason = "There is no selected candidate."
		createNamedReason = "There is no selected candidate."
		templateReason = "There is no selected candidate."
	} else if selected.kind != candidateSession {
		killReason = "The selected candidate is not an open tmux session."
		sessionReason = "The selected candidate is not an open tmux session."
	} else {
		templateReason = "The selected candidate is already an open tmux session."
	}
	if ok {
		if path, _, targetOK := m.namedSessionTarget(selected); !targetOK {
			createNamedReason = "The selected candidate has no path."
			templateReason = "The selected candidate has no path."
		} else if _, exists := m.sessionForPath(path); exists {
			templateReason = "A tmux session already exists for the selected path."
		}
	}
	return []commandItem{
		commandItemFromSpec(specs[0], ok, "There is no selected candidate."),
		commandItemFromSpec(specs[1], true, ""),
		commandItemFromSpec(specs[2], killReason == "", killReason),
		commandItemFromSpec(specs[3], sessionReason == "", sessionReason),
		commandItemFromSpec(specs[4], createNamedReason == "", createNamedReason),
		commandItemFromSpec(specs[5], templateReason == "", templateReason),
		commandItemFromSpec(specs[6], m.pathConfig.enabled, "Path search is disabled in config."),
		commandItemFromSpec(specs[7], true, ""),
		commandItemFromSpec(specs[8], true, ""),
		commandItemFromSpec(specs[9], true, ""),
		commandItemFromSpec(specs[10], true, ""),
		commandItemFromSpec(specs[11], true, ""),
	}
}

func commandItemFromSpec(spec commandSpec, enabled bool, disabledReason string) commandItem {
	return commandItemFromSpecWithTitle(spec, spec.Title, enabled, disabledReason)
}

func commandItemFromSpecWithTitle(spec commandSpec, title string, enabled bool, disabledReason string) commandItem {
	return commandItem{
		ID:             spec.ID,
		Title:          title,
		Key:            spec.Key,
		Description:    spec.Description,
		Enabled:        enabled,
		DisabledReason: disabledReason,
	}
}

func helpItemFromSpec(spec commandSpec) helpItem {
	return helpItem{Key: spec.Key, Action: spec.Title, Description: spec.Description}
}

func pathSearchCommandSpecs() []commandSpec {
	return pathSearchCommandSpecsFor(config.Default().UI.Keys)
}

func (m Model) pathSearchCommandSpecs() []commandSpec {
	return pathSearchCommandSpecsFor(m.keys)
}

func pathSearchCommandSpecsFor(keys config.KeyBindings) []commandSpec {
	return []commandSpec{
		{ID: commandOpenSelected, Title: "Open selected result", Key: keyListLabel(keys.PathSearch.OpenSelected), Description: "Create or switch to a tmux session for the selected fuzzy path result."},
		{ID: commandTemplateSession, Title: "Create selected result from template", Key: keyListLabel(keys.PathSearch.TemplateSession), Description: "Choose a configured session template for the selected fuzzy path result."},
		{ID: commandOpenTyped, Title: "Open typed path", Key: keyListLabel(keys.PathSearch.OpenTyped), Description: "Open the exact typed prompt path when it exists as a directory."},
		{ID: commandCreateTyped, Title: "Add typed path", Key: keyListLabel(keys.PathSearch.CreateTyped), Description: "Create the exact typed prompt path after confirmation, then create or switch to its tmux session."},
		{ID: commandCycleRoot, Title: "Cycle prompt root", Key: keyListLabel(keys.PathSearch.CycleRoot), Description: "Cycle the path prompt through ~/ / ./ ../."},
		{ID: commandToggleHidden, Title: "Toggle hidden path results", Key: keyListLabel(keys.PathSearch.ToggleHidden), Description: "Toggle whether hidden directories are skipped in the current path search."},
		{ID: commandToggleIgnored, Title: "Toggle gitignored path results", Key: keyListLabel(keys.PathSearch.ToggleIgnored), Description: "Toggle whether gitignored directories are skipped in the current path search."},
		{ID: commandReload, Title: "Reload path search", Key: keyListLabel(keys.PathSearch.Reload), Description: "Restart the current streamed path search."},
		{ID: commandHelp, Title: "Show help", Key: keyListLabel(keys.PathSearch.Help), Description: "Show help for the command palette."},
		{ID: commandQuit, Title: "Quit", Key: keyListLabel(keys.PathSearch.Close), Description: "Quit tmux-parator."},
	}
}

func browseCommandSpecs() []commandSpec {
	return browseCommandSpecsFor(config.Default().UI.Keys)
}

func (m Model) browseCommandSpecs() []commandSpec {
	return browseCommandSpecsFor(m.keys)
}

func browseCommandSpecsFor(keys config.KeyBindings) []commandSpec {
	return []commandSpec{
		{ID: commandOpenSelected, Title: "Open selected", Key: keyListLabel(keys.Browse.OpenSelected), Description: "Switch to an existing session or create one for the selected root."},
		{ID: commandOpenLast, Title: "Open last session", Key: keyListLabel(keys.Browse.OpenLastSession), Description: "Switch to tmux's last active session."},
		{ID: commandKillSession, Title: "Kill selected session", Key: keyListLabel(keys.Browse.KillSession), Description: "Ask for confirmation before killing the selected tmux session."},
		{ID: commandRenameSession, Title: "Rename selected session", Key: keyListLabel(keys.Browse.RenameSession), Description: "Rename the selected open tmux session."},
		{ID: commandNewSession, Title: "Create named session", Key: keyListLabel(keys.Browse.NewSession), Description: "Create a new named tmux session from the selected row's path and kind."},
		{ID: commandTemplateSession, Title: "Create session from template", Key: keyListLabel(keys.Browse.TemplateSession), Description: "Choose a configured session template for the selected workspace."},
		{ID: commandPathSearch, Title: "Create session from path", Key: keyListLabel(keys.Browse.PathSearch), Description: "Open filesystem path search, then create or switch to a session for the selected path."},
		{ID: commandToggleHidden, Title: "Toggle hidden configured paths", Key: keyListLabel(keys.Browse.ToggleHidden), Description: "Toggle whether hidden directories are skipped for configured repos and subdirs."},
		{ID: commandToggleIgnored, Title: "Toggle gitignored configured paths", Key: keyListLabel(keys.Browse.ToggleIgnored), Description: "Toggle whether gitignored directories are skipped for configured repos and subdirs."},
		{ID: commandReload, Title: "Reload", Key: keyListLabel(keys.Browse.Reload), Description: "Reload tmux sessions and configured root candidates."},
		{ID: commandHelp, Title: "Show help", Key: keyListLabel(keys.Browse.Help), Description: "Show help for the command palette."},
		{ID: commandQuit, Title: "Quit", Key: keyListLabel(keys.Browse.Quit), Description: "Quit tmux-parator."},
	}
}

func combinedKeyLabel(first []string, second []string) string {
	if len(first) == 1 && len(second) == 1 {
		return keyLabel(first[0]) + "/" + keyLabel(second[0])
	}
	return keyListLabel(append(append([]string(nil), first...), second...))
}

func keyListLabel(keys []string) string {
	labels := make([]string, 0, len(keys))
	for _, key := range keys {
		labels = append(labels, keyLabel(key))
	}
	return strings.Join(labels, "/")
}

func keyLabel(key string) string {
	switch key {
	case "ctrl+@":
		return "<c-`>"
	case "ctrl+_":
		return "<c-?>"
	case "shift+tab":
		return "<s-tab>"
	case "esc", "enter", "tab", "backspace", "alt+backspace", "meta+backspace", "up", "down", "left", "right":
		return "<" + key + ">"
	}
	for _, prefix := range []struct {
		raw   string
		label string
	}{
		{raw: "ctrl+", label: "c-"},
		{raw: "alt+", label: "alt-"},
		{raw: "meta+", label: "meta-"},
	} {
		if strings.HasPrefix(key, prefix.raw) {
			return "<" + prefix.label + strings.TrimPrefix(key, prefix.raw) + ">"
		}
	}
	return key
}

func dismissKeys(keys []string) []string {
	var filtered []string
	for _, key := range keys {
		if key == "esc" {
			filtered = append(filtered, key)
		}
	}
	if len(filtered) > 0 {
		return filtered
	}
	return keys
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

func (m Model) helpMatches() []helpMatch {
	items := m.helpItemsForMode(m.previousMode)
	if len(items) == 0 {
		items = m.helpItemsForMode(modeBrowse)
	}
	return filterHelpItems(items, m.helpInput)
}

func filterHelpItems(items []helpItem, query string) []helpMatch {
	if strings.TrimSpace(query) == "" {
		matches := make([]helpMatch, 0, len(items))
		for _, item := range items {
			matches = append(matches, helpMatch{item: item})
		}
		return matches
	}
	candidates := make([]fuzzy.Candidate, 0, len(items))
	for _, item := range items {
		candidates = append(candidates, fuzzy.Candidate{
			Title:   item.Action,
			Aliases: []string{item.Key},
			Fields: []fuzzy.Field{
				{Name: "description", Value: item.Description, Weight: 500},
			},
			Value: item,
		})
	}
	filtered := fuzzy.Filter(candidates, query)
	matches := make([]helpMatch, 0, len(filtered))
	for _, match := range filtered {
		item, ok := match.Candidate.Value.(helpItem)
		if !ok {
			continue
		}
		matches = append(matches, helpMatch{
			item:          item,
			actionIndexes: match.TitleIndexes,
			keyIndexes:    match.AliasIndexes[item.Key],
		})
	}
	return matches
}

func (m *Model) applyTemplateFilter() {
	items := templatePickerItems(m.templateAvailable)
	if strings.TrimSpace(m.templateFilter) == "" {
		m.templateFiltered = append(m.templateFiltered[:0], items...)
	} else {
		fuzzyCandidates := make([]fuzzy.Candidate, 0, len(items))
		for _, template := range items {
			fuzzyCandidates = append(fuzzyCandidates, templateFuzzyCandidate(template))
		}
		matches := fuzzy.Filter(fuzzyCandidates, m.templateFilter)
		filtered := make([]sessionconfig.Template, 0, len(matches))
		for _, match := range matches {
			template, ok := match.Candidate.Value.(sessionconfig.Template)
			if !ok {
				continue
			}
			filtered = append(filtered, template)
		}
		sort.SliceStable(filtered, func(i, j int) bool {
			return templateSourceRank(filtered[i]) < templateSourceRank(filtered[j])
		})
		m.templateFiltered = filtered
	}
	if len(m.templateFiltered) == 0 {
		m.templateCursor = 0
		m.templateScroll = 0
		return
	}
	if m.templateCursor >= len(m.templateFiltered) {
		m.templateCursor = len(m.templateFiltered) - 1
	}
	if m.templateScroll >= len(m.templateFiltered) {
		m.templateScroll = len(m.templateFiltered) - 1
	}
	m.ensureTemplateCursorVisible()
}

func (m *Model) setTemplateFilter(filter string) {
	previous := m.templateFilter
	cursor := m.templateCursor
	scroll := m.templateScroll
	m.templateFilter = filter
	m.applyTemplateFilter()
	m.templateCursor, m.templateScroll, m.templateRestore = popupFilterSelection(
		previous,
		filter,
		cursor,
		scroll,
		m.templateRestore,
		len(m.templateFiltered),
	)
	m.ensureTemplateCursorVisible()
}

func templatePickerItems(templates []sessionconfig.Template) []sessionconfig.Template {
	items := make([]sessionconfig.Template, 0, len(templates)+1)
	items = append(items, templates...)
	items = append(items, noTemplatePickerItem())
	return items
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
			return m.openCandidate(selected)
		}
		selected, ok := m.selected()
		if !ok {
			return m.notifyCommandUnavailable(item), nil
		}
		m.mode = modeBrowse
		return m.openCandidate(selected)
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
		return m.openCandidate(selected)
	case commandCreateTyped:
		return m.openCreateTypedPathConfirmation()
	case commandKillSession:
		m.mode = modeConfirmKill
		m.confirmChoice = confirmCancel
	case commandRenameSession:
		return m.openRenameSession()
	case commandNewSession:
		m.openCreateSession()
	case commandTemplateSession:
		if m.commandPreviousMode == modePathSearch {
			return m.openPathTemplatePicker()
		}
		return m.openTemplatePicker()
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
		m.pathNotice = m.pathErr
		return m
	}
	m.mode = modeBrowse
	m.notice = fmt.Errorf("%s: %s", item.Title, reason)
	return m
}

type dialogFrame struct {
	width  int
	height int
}

func renderCommandPalette(s styles, dialogs config.Dialogs, matches []commandMatch, query string, cursor int, scroll int, appWidth int, appHeight int) string {
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(matches) {
		cursor = len(matches) - 1
	}
	frame := panelDialogFrame(dialogs, appWidth, appHeight)
	width := frame.width
	description := "Type to fuzzy-search commands."
	if len(matches) > 0 && cursor >= 0 {
		description = matches[cursor].item.Description
		if !matches[cursor].item.Enabled && strings.TrimSpace(matches[cursor].item.DisabledReason) != "" {
			description = matches[cursor].item.DisabledReason
		}
	}
	descriptionLines := wrapHelpDescription(description, helpDescriptionTextWidth(width))
	height := commandListHeightForFrame(frame)
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

	panelHeight := panelMainBoxHeight(frame)
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
		selected := itemIndex < len(matches) && itemIndex == cursor
		row := strings.Repeat(" ", rowWidth)
		if itemIndex < len(matches) {
			row = renderCommandRow(matches[itemIndex], selected, s, rowWidth)
		} else if rowIndex == 0 && len(matches) == 0 {
			row = s.muted.Render(truncate("No matching commands", rowWidth))
		}
		if rowWidth != bodyWidth {
			row = renderPopupSelectionMarker(selected, s) + row + renderHelpScrollbar(rowIndex, scroll, height, len(matches), s)
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

func renderPopupSelectionMarker(selected bool, s styles) string {
	if selected {
		return s.glyph.Render("▌")
	}
	return s.root.Render(" ")
}

func renderCommandRow(match commandMatch, selected bool, s styles, width int) string {
	item := match.item
	const (
		keyColumnWidth = 26
		columnGap      = 2
	)
	key := truncate(item.Key, keyColumnWidth-2)
	titleBudget := width - keyColumnWidth - columnGap
	if titleBudget < 1 {
		titleBudget = 1
	}
	title := truncate(item.Title, titleBudget)
	titleIndexes := match.titleIndexes
	if !item.Enabled {
		title = truncate(item.Title+" (disabled)", titleBudget)
		titleIndexes = nil
	}
	return renderPopupLabeledRow(
		key,
		nil,
		title,
		titleIndexes,
		selected,
		!item.Enabled,
		s,
		width,
		keyColumnWidth,
		columnGap,
	)
}

func renderTemplatePicker(s styles, dialogs config.Dialogs, keys config.KeyBindings, templates []sessionconfig.Template, query string, cursor int, scroll int, appWidth int, appHeight int) string {
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(templates) {
		cursor = len(templates) - 1
	}
	frame := panelDialogFrame(dialogs, appWidth, appHeight)
	width := frame.width
	description := "Select a session template."
	if len(templates) > 0 && cursor >= 0 {
		description = templates[cursor].Description
		if strings.TrimSpace(description) == "" {
			description = templates[cursor].Name
		}
	}
	descriptionLines := wrapHelpDescription(description, helpDescriptionTextWidth(width))
	height := templateListHeightForFrame(frame)
	if scroll < 0 {
		scroll = 0
	}
	if scroll > cursor {
		scroll = cursor
	}
	totalRows := templateDisplayRowCount(templates)
	for scroll > 0 && totalRows-templateScrollDisplayRow(templates, scroll) < height {
		scroll--
	}

	panelHeight := panelMainBoxHeight(frame)
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
	startRow := templateScrollDisplayRow(templates, scroll)
	activeSection := ""
	if cursor >= 0 && cursor < len(templates) {
		activeSection = templateSource(templates[cursor])
	}
	for rowIndex := 0; rowIndex < height; rowIndex++ {
		displayRow := startRow + rowIndex
		row := strings.Repeat(" ", rowWidth)
		itemIndex, section := templateIndexAtDisplayRow(templates, displayRow)
		selected := itemIndex >= 0 && itemIndex < len(templates) && itemIndex == cursor
		fullWidthRow := false
		if section != "" {
			row = renderTemplateSectionHeader(section, section == activeSection, s, bodyWidth)
			fullWidthRow = true
		} else if itemIndex >= 0 && itemIndex < len(templates) {
			row = renderTemplateRow(templates[itemIndex], query, selected, s, rowWidth)
		} else if rowIndex == 0 && len(templates) == 0 {
			row = s.muted.Render(truncate("No matching templates", rowWidth))
		}
		if rowWidth != bodyWidth && !fullWidthRow {
			row = renderPopupSelectionMarker(selected, s) + row + renderHelpScrollbar(rowIndex, startRow, height, totalRows, s)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	mainBox := renderTitledBox("Templates", helpPositionLabel(cursor, len(templates)), b.String(), width, panelHeight, horizontalPadding, s)
	descriptionBox := renderHelpDescription(descriptionLines, s, width)
	footer := renderHelpDescription([]string{s.popupAccent.Render(keyListLabel(keys.Browse.OpenSelected)) + s.popupMuted.Render(" create") + s.popupBody.Render("   ") + s.popupAccent.Render(keyListLabel(keys.Browse.Quit)) + s.popupMuted.Render(" cancel")}, s, width)
	return lipgloss.JoinVertical(lipgloss.Left, mainBox, descriptionBox, footer)
}

func renderTemplateParameterPicker(s styles, dialogs config.Dialogs, parameter sessionconfig.Parameter, index int, total int, cursor int, selectKey string, backKey string, returnsToTemplatePicker bool, appWidth int, appHeight int) string {
	frame := panelDialogFrame(dialogs, appWidth, appHeight)
	width := frame.width
	contentWidth := width - 2
	horizontalPadding := 3
	bodyWidth := contentWidth - horizontalPadding*2
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	rowWidth := bodyWidth
	if rowWidth < 1 {
		rowWidth = 1
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(parameter.Options) {
		cursor = len(parameter.Options) - 1
	}
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(s.popupAccent.Render(truncate(parameter.Prompt, bodyWidth)))
	b.WriteString("\n\n")
	for optionIndex, option := range parameter.Options {
		selected := optionIndex == cursor
		const (
			chipWidth = 3
			gapWidth  = 2
		)
		optionWidth := rowWidth - chipWidth - gapWidth
		if optionWidth < 1 {
			optionWidth = 1
		}
		option = truncate(option, optionWidth)
		textStyle := s.muted
		chipStyle := s.chip
		bullet := "○"
		if selected {
			textStyle = s.selected
			chipStyle = s.selectedChip
			bullet = "●"
		}
		chip := chipStyle.Render(" " + bullet + " ")
		row := chip + textStyle.Render(strings.Repeat(" ", gapWidth)+option)
		if selected {
			row = padSelectedRow(row, rowWidth, s)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	position := fmt.Sprintf("%d/%d", index+1, total)
	mainBox := renderTitledBox("Template Parameter", position, b.String(), width, panelMainBoxHeight(frame), horizontalPadding, s)
	backAction := "cancel"
	if index > 0 || returnsToTemplatePicker {
		backAction = "back"
	}
	footer := renderHelpDescription([]string{
		s.popupAccent.Render(selectKey) + s.popupMuted.Render(" select") +
			s.popupBody.Render("   ") +
			s.popupAccent.Render(backKey) + s.popupMuted.Render(" "+backAction),
	}, s, width)
	return lipgloss.JoinVertical(lipgloss.Left, mainBox, footer)
}

func renderTemplateRow(template sessionconfig.Template, query string, selected bool, s styles, width int) string {
	match := templateFuzzyMatch(template, query)
	rowStyles := popupRowStyles(selected, false, s)

	const (
		chipColumnWidth   = 6
		nameColumnWidth   = 24
		templateColumnGap = 2
	)

	chip := strings.TrimSpace(template.Chip)
	chipColumn := renderPopupChipColumn(chip, match.AliasIndexes[chip], chipColumnWidth, rowStyles)
	name := truncate(template.Name, nameColumnWidth)
	nameColumn := fitStyledColumn(renderMatchedText(name, rowStyles.text, rowStyles.match, match.TitleIndexes), nameColumnWidth, rowStyles.text)
	indicatorStyle := lipgloss.NewStyle().
		Foreground(s.glyph.GetForeground()).
		Background(rowStyles.text.GetBackground())
	if selected {
		indicatorStyle = indicatorStyle.Foreground(s.selectedMatch.GetForeground())
	}
	gap := rowStyles.text.Render(strings.Repeat(" ", templateColumnGap))
	indicatorColumnWidth := max(0, width-chipColumnWidth-nameColumnWidth-(templateColumnGap*2))
	indicators := strings.Join(template.WindowIndicators, " · ")
	indicatorColumn := fitStyledColumn(
		renderMatchedText(indicators, indicatorStyle, rowStyles.match, match.FieldIndexes["window_indicators"]),
		indicatorColumnWidth,
		rowStyles.text,
	)
	line := chipColumn + gap + nameColumn + gap + indicatorColumn
	if selected {
		return padSelectedRow(line, width, s)
	}
	return fitRow(line, width)
}

func templateFuzzyMatch(template sessionconfig.Template, query string) fuzzy.Match {
	matches := fuzzy.Filter([]fuzzy.Candidate{templateFuzzyCandidate(template)}, query)
	if len(matches) == 0 {
		return fuzzy.Match{}
	}
	return matches[0]
}

func renderPopupChip(chip string, style lipgloss.Style, matchStyle lipgloss.Style, indexes []int) string {
	return style.Render(" ") + renderMatchedText(chip, style, matchStyle, indexes) + style.Render(" ")
}

type popupRowStyleSet struct {
	text      lipgloss.Style
	match     lipgloss.Style
	chip      lipgloss.Style
	chipMatch lipgloss.Style
}

func popupRowStyles(selected bool, mutedChip bool, s styles) popupRowStyleSet {
	if selected {
		return popupRowStyleSet{
			text:      s.selected,
			match:     s.selectedMatch,
			chip:      s.selected,
			chipMatch: s.selectedMatch,
		}
	}
	chipStyle := s.chip
	chipMatchStyle := s.match.Copy().Background(s.chip.GetBackground())
	if mutedChip {
		chipStyle = s.chip.Copy().Foreground(s.muted.GetForeground())
		chipMatchStyle = chipStyle
	}
	return popupRowStyleSet{
		text:      s.muted,
		match:     s.match,
		chip:      chipStyle,
		chipMatch: chipMatchStyle,
	}
}

func renderPopupChipColumn(chip string, indexes []int, width int, rowStyles popupRowStyleSet) string {
	renderedChip := ""
	if chip = strings.TrimSpace(chip); chip != "" {
		renderedChip = renderPopupChip(chip, rowStyles.chip, rowStyles.chipMatch, indexes)
	}
	return fitStyledColumn(renderedChip, width, rowStyles.text)
}

func renderPopupLabeledRow(chip string, chipIndexes []int, label string, labelIndexes []int, selected bool, mutedChip bool, s styles, width int, chipColumnWidth int, columnGap int) string {
	rowStyles := popupRowStyles(selected, mutedChip, s)
	chipColumn := renderPopupChipColumn(chip, chipIndexes, chipColumnWidth, rowStyles)
	gap := rowStyles.text.Render(strings.Repeat(" ", columnGap))
	renderedLabel := renderMatchedText(label, rowStyles.text, rowStyles.match, labelIndexes)
	line := chipColumn + gap + renderedLabel
	if selected {
		return padSelectedRow(line, width, s)
	}
	return fitRow(line, width)
}

func fitStyledColumn(value string, width int, fillStyle lipgloss.Style) string {
	if lipgloss.Width(value) > width {
		return ansi.Cut(value, 0, width)
	}
	return value + fillStyle.Render(strings.Repeat(" ", width-lipgloss.Width(value)))
}

func renderTemplateSectionHeader(section string, active bool, s styles, width int) string {
	label := strings.ToUpper(section) + " "
	labelStyle, lineStyle := templateSectionStyles(active, s)
	prefix := labelStyle.Render(label)
	lineWidth := width - lipgloss.Width(prefix)
	if lineWidth < 1 {
		lineWidth = 1
	}
	return prefix + lineStyle.Render(strings.Repeat("─", lineWidth))
}

func templateSectionStyles(active bool, s styles) (lipgloss.Style, lipgloss.Style) {
	if !active {
		return s.muted, s.muted
	}
	return lipgloss.NewStyle().
			Foreground(s.popupAccent.GetForeground()).
			Bold(true),
		s.filterLabel
}

func templateSource(template sessionconfig.Template) string {
	if isNoTemplatePickerItem(template) {
		return sessionconfig.SourceGlobal
	}
	source := strings.TrimSpace(template.Source)
	if source == "" {
		return sessionconfig.SourceGlobal
	}
	return source
}

func templateSourceRank(template sessionconfig.Template) int {
	switch templateSource(template) {
	case sessionconfig.SourceLocal:
		return 0
	case sessionconfig.SourceGlobal:
		return 1
	default:
		return 2
	}
}

func templateDisplayRow(templates []sessionconfig.Template, templateIndex int) int {
	if templateIndex < 0 {
		return 0
	}
	row := templateIndex
	for i := 0; i <= templateIndex && i < len(templates); i++ {
		if templateStartsSection(templates, i) {
			row++
		}
	}
	return row
}

func templateScrollDisplayRow(templates []sessionconfig.Template, templateIndex int) int {
	row := templateDisplayRow(templates, templateIndex)
	if templateStartsSection(templates, templateIndex) && row > 0 {
		return row - 1
	}
	return row
}

func templateDisplayRowCount(templates []sessionconfig.Template) int {
	count := len(templates)
	for i := range templates {
		if templateStartsSection(templates, i) {
			count++
		}
	}
	return count
}

func templateStartsSection(templates []sessionconfig.Template, index int) bool {
	if index < 0 || index >= len(templates) {
		return false
	}
	return index == 0 || templateSource(templates[index-1]) != templateSource(templates[index])
}

func templateIndexAtDisplayRow(templates []sessionconfig.Template, displayRow int) (int, string) {
	row := 0
	for i, template := range templates {
		if templateStartsSection(templates, i) {
			if row == displayRow {
				return -1, templateSource(template)
			}
			row++
		}
		if row == displayRow {
			return i, ""
		}
		row++
	}
	return -1, ""
}

func renderConfirmKill(s styles, dialogs config.Dialogs, sessionName string, choice confirmChoice, appWidth int, appHeight int) string {
	return renderConfirmKillWithKeys(s, dialogs, config.Default().UI.Keys, sessionName, choice, appWidth, appHeight)
}

func renderConfirmKillWithKeys(s styles, dialogs config.Dialogs, keys config.KeyBindings, sessionName string, choice confirmChoice, appWidth int, appHeight int) string {
	width := smallDialogWidth(dialogs, appWidth)
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
	lines = append(lines, renderConfirmAction("Cancel", keyListLabel(keys.Confirm.No), choice == confirmCancel, s)+s.popupBody.Render("   ")+renderConfirmAction("Confirm", keyListLabel(keys.Confirm.Yes), choice == confirmYes, s))
	return renderCenteredTitledBox("Confirm", "", strings.Join(lines, "\n"), width, smallDialogHeight(dialogs, appHeight, len(lines)+4), 3, s)
}

func renderConfirmCreatePath(s styles, dialogs config.Dialogs, pathInput string, choice confirmChoice, appWidth int, appHeight int) string {
	return renderConfirmCreatePathWithKeys(s, dialogs, config.Default().UI.Keys, pathInput, choice, appWidth, appHeight)
}

func renderConfirmCreatePathWithKeys(s styles, dialogs config.Dialogs, keys config.KeyBindings, pathInput string, choice confirmChoice, appWidth int, appHeight int) string {
	width := smallDialogWidth(dialogs, appWidth)
	bodyWidth := width - 8
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	if strings.TrimSpace(pathInput) == "" {
		pathInput = "typed path"
	}
	message := "Create directory " + strconv.Quote(pathInput) + "?"
	lines := wrapHelpDescription(message, bodyWidth)
	lines = append(lines, "")
	lines = append(lines, renderConfirmAction("Cancel", keyListLabel(keys.Confirm.No), choice == confirmCancel, s)+s.popupBody.Render("   ")+renderConfirmAction("Create", keyListLabel(keys.Confirm.Yes), choice == confirmYes, s))
	return renderCenteredTitledBox("Confirm", "", strings.Join(lines, "\n"), width, smallDialogHeight(dialogs, appHeight, len(lines)+4), 3, s)
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

func renderCreateSession(s styles, dialogs config.Dialogs, value string, appWidth int, appHeight int) string {
	return renderCreateSessionWithKeys(s, dialogs, config.Default().UI.Keys, value, appWidth, appHeight)
}

func renderCreateSessionWithKeys(s styles, dialogs config.Dialogs, keys config.KeyBindings, value string, appWidth int, appHeight int) string {
	return renderNamePrompt(s, dialogs, keys, "Create Named Session", "Create a tmux session from the selected row.", "create", value, appWidth, appHeight)
}

func renderRenameSession(s styles, dialogs config.Dialogs, value string, appWidth int, appHeight int) string {
	return renderRenameSessionWithKeys(s, dialogs, config.Default().UI.Keys, value, appWidth, appHeight)
}

func renderRenameSessionWithKeys(s styles, dialogs config.Dialogs, keys config.KeyBindings, value string, appWidth int, appHeight int) string {
	return renderNamePrompt(s, dialogs, keys, "Rename Session", "Rename the selected tmux session.", "rename", value, appWidth, appHeight)
}

func renderNamePrompt(s styles, dialogs config.Dialogs, keys config.KeyBindings, title string, message string, action string, value string, appWidth int, appHeight int) string {
	width := smallDialogWidth(dialogs, appWidth)
	bodyWidth := width - 8
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	lines := []string{
		s.popupMuted.Render(message),
		"",
		renderSearchBox("name ❯ ", value, bodyWidth, s),
		"",
		s.popupAccent.Render(keyListLabel(keys.Browse.OpenSelected)) + s.popupMuted.Render(" "+action) + s.popupBody.Render("   ") + s.popupAccent.Render(keyListLabel(keys.Browse.Quit)) + s.popupMuted.Render(" cancel"),
	}
	return renderCenteredTitledBox(title, "", strings.Join(lines, "\n"), width, smallDialogHeight(dialogs, appHeight, len(lines)+4), 3, s)
}

func renderErrorPopup(s styles, dialogs config.Dialogs, message string, appWidth int, appHeight int) string {
	defaults := config.Default().UI.Keys
	return renderErrorPopupWithKeys(s, dialogs, dismissKeys(defaults.Browse.Quit), defaults.Browse.Reload, message, appWidth, appHeight)
}

func renderErrorPopupWithKeys(s styles, dialogs config.Dialogs, dismissKeys []string, retryKeys []string, message string, appWidth int, appHeight int) string {
	width := smallDialogWidth(dialogs, appWidth)
	bodyWidth := width - 8
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	lines := wrapHelpDescription(message, bodyWidth)
	lines = append(lines, "")
	lines = append(lines, s.popupAccent.Render(keyListLabel(dismissKeys))+s.popupMuted.Render(" dismiss")+s.popupBody.Render("   ")+s.popupAccent.Render(keyListLabel(retryKeys))+s.popupMuted.Render(" retry/reload"))
	return renderCenteredTitledBox("Error", "", strings.Join(lines, "\n"), width, smallDialogHeight(dialogs, appHeight, len(lines)+4), 3, s)
}

func renderPathNoticePopup(s styles, dialogs config.Dialogs, message string, appWidth int, appHeight int) string {
	defaults := config.Default().UI.Keys
	return renderPathNoticePopupWithKeys(s, dialogs, defaults.Browse.OpenSelected, dismissKeys(defaults.Browse.Quit), message, appWidth, appHeight)
}

func renderPathNoticePopupWithKeys(s styles, dialogs config.Dialogs, acceptKeys []string, dismissKeys []string, message string, appWidth int, appHeight int) string {
	width := smallDialogWidth(dialogs, appWidth)
	bodyWidth := width - 8
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	lines := wrapHelpDescription(message, bodyWidth)
	lines = append(lines, "")
	lines = append(lines, s.popupAccent.Render(keyListLabel(append(append([]string(nil), acceptKeys...), dismissKeys...)))+s.popupMuted.Render(" dismiss"))
	return renderCenteredTitledBox("Notice", "", strings.Join(lines, "\n"), width, smallDialogHeight(dialogs, appHeight, len(lines)+4), 3, s)
}

func renderHelpPanel(s styles, dialogs config.Dialogs, previous mode, cursor int, scroll int, appWidth int, appHeight int) string {
	return renderHelpPanelWithKeys(s, dialogs, config.Default().UI.Keys, previous, "", cursor, scroll, appWidth, appHeight)
}

func renderHelpPanelWithKeys(s styles, dialogs config.Dialogs, keys config.KeyBindings, previous mode, query string, cursor int, scroll int, appWidth int, appHeight int) string {
	items := helpItemsForModeWithKeys(previous, keys)
	if len(items) == 0 {
		items = helpItemsForModeWithKeys(modeBrowse, keys)
	}
	matches := filterHelpItems(items, query)
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(matches) {
		cursor = len(matches) - 1
	}
	frame := panelDialogFrame(dialogs, appWidth, appHeight)
	width := frame.width
	description := "Type to fuzzy-search help."
	if len(matches) > 0 && cursor >= 0 {
		description = helpDescription(matches[cursor].item)
	}
	descriptionLines := wrapHelpDescription(description, helpDescriptionTextWidth(width))
	height := commandListHeightForFrame(frame)
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

	panelHeight := panelMainBoxHeight(frame)
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
		selected := itemIndex < len(matches) && itemIndex == cursor
		row := strings.Repeat(" ", rowWidth)
		if itemIndex < len(matches) {
			row = renderHelpRow(matches[itemIndex], selected, s, rowWidth)
		} else if rowIndex == 0 && len(matches) == 0 {
			row = s.muted.Render(truncate("No matching help entries", rowWidth))
		}
		if rowWidth != bodyWidth {
			row = renderPopupSelectionMarker(selected, s) + row + renderHelpScrollbar(rowIndex, scroll, height, len(matches), s)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	mainBox := renderTitledBox(helpTitle(previous), helpPositionLabel(cursor, len(matches)), b.String(), width, panelHeight, horizontalPadding, s)
	descriptionBox := renderHelpDescription(descriptionLines, s, width)
	return lipgloss.JoinVertical(lipgloss.Left, mainBox, descriptionBox)
}

func renderHelpRow(match helpMatch, selected bool, s styles, width int) string {
	item := match.item
	const (
		keyColumnWidth = 26
		columnGap      = 2
	)
	key := truncate(item.Key, keyColumnWidth-2)
	actionBudget := width - keyColumnWidth - columnGap
	if actionBudget < 1 {
		actionBudget = 1
	}
	action := truncate(item.Action, actionBudget)
	return renderPopupLabeledRow(
		key,
		match.keyIndexes,
		action,
		match.actionIndexes,
		selected,
		false,
		s,
		width,
		keyColumnWidth,
		columnGap,
	)
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

func normalizeUIDialogs(dialogs config.Dialogs) config.Dialogs {
	if dialogs.Small.Width <= 0 {
		dialogs.Small.Width = 72
	}
	if dialogs.Small.Height < 0 {
		dialogs.Small.Height = 9
	}
	if dialogs.Panel.Width <= 0 {
		dialogs.Panel.Width = 88
	}
	if dialogs.Panel.Height < 0 {
		dialogs.Panel.Height = 0
	}
	return dialogs
}

func smallDialogWidth(dialogs config.Dialogs, appWidth int) int {
	dialogs = normalizeUIDialogs(dialogs)
	return dialogWidth(dialogs.Small.Width, appWidth, 20)
}

func smallDialogHeight(dialogs config.Dialogs, appHeight int, contentHeight int) int {
	dialogs = normalizeUIDialogs(dialogs)
	if dialogs.Small.Height == 0 {
		return contentHeight
	}
	return dialogHeight(dialogs.Small.Height, appHeight, contentHeight)
}

func panelDialogFrame(dialogs config.Dialogs, appWidth int, appHeight int) dialogFrame {
	dialogs = normalizeUIDialogs(dialogs)
	return dialogFrame{
		width:  panelDialogWidth(dialogs.Panel.Width, appWidth),
		height: panelDialogHeight(dialogs.Panel.Height, appHeight),
	}
}

func dialogWidth(target int, appWidth int, minWidth int) int {
	if target <= 0 {
		target = minWidth
	}
	width := target
	if appWidth > 0 {
		maxWidth := appWidth - 4
		if maxWidth < minWidth {
			maxWidth = appWidth
		}
		if maxWidth > 0 && width > maxWidth {
			width = maxWidth
		}
	}
	if width < minWidth {
		width = minWidth
	}
	if appWidth > 0 && width > appWidth {
		width = appWidth
	}
	return width
}

func dialogHeight(target int, appHeight int, contentHeight int) int {
	height := target
	if height < contentHeight {
		height = contentHeight
	}
	if appHeight > 0 && height > appHeight {
		height = appHeight
	}
	if height < 6 {
		height = 6
	}
	return height
}

func panelDialogWidth(target int, appWidth int) int {
	if appWidth <= 0 {
		return target
	}
	width := appWidth - 8
	if width > target {
		width = target
	}
	if width < 62 {
		width = appWidth - 4
	}
	if width < 30 {
		width = appWidth
	}
	if width < 20 {
		width = 20
	}
	if width > appWidth {
		width = appWidth
	}
	return width
}

func panelDialogHeight(target int, appHeight int) int {
	if target > 0 {
		return dialogHeight(target, appHeight, 6)
	}
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

func panelMainBoxHeight(frame dialogFrame) int {
	height := frame.height - 3
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

func helpListHeightForFrame(frame dialogFrame) int {
	height := panelMainBoxHeight(frame) - 4
	if height < 4 {
		height = 4
	}
	return height
}

func commandListHeightForFrame(frame dialogFrame) int {
	height := helpListHeightForFrame(frame) - 3
	if height < 3 {
		height = 3
	}
	return height
}

func templateListHeightForFrame(frame dialogFrame) int {
	height := helpListHeightForFrame(frame) - 3
	if height < 3 {
		height = 3
	}
	return height
}

func helpItemsForMode(previous mode) []helpItem {
	return helpItemsForModeWithKeys(previous, config.Default().UI.Keys)
}

func (m Model) helpItemsForMode(previous mode) []helpItem {
	return helpItemsForModeWithKeys(previous, m.keys)
}

func helpItemsForModeWithKeys(previous mode, keys config.KeyBindings) []helpItem {
	if previous == modePathSearch {
		specs := pathSearchCommandSpecsFor(keys)
		return []helpItem{
			{Key: "type", Action: "edit path prompt", Description: "Type a path-like prompt; the text after the last slash is used as the fuzzy query."},
			{Key: keyListLabel(keys.PathSearch.DeleteChar), Action: "remove character", Description: "Remove one character from the path prompt and reparse the root/query."},
			{Key: keyListLabel(keys.PathSearch.DeleteWord), Action: "remove word", Description: "Remove one path or word segment from the prompt and reparse the root/query."},
			{Key: keyListLabel(keys.PathSearch.ClearInput), Action: "clear prompt", Description: "Clear the path prompt and reparse the root/query."},
			{Key: combinedKeyLabel(keys.PathSearch.Up, keys.PathSearch.Down), Action: "move selection", Description: "Move through fuzzy results without changing the typed path."},
			{Key: combinedKeyLabel(keys.PathSearch.ScrollUp, keys.PathSearch.ScrollDown), Action: "scroll results", Description: "Scroll the result viewport one row while keeping the selection visible."},
			{Key: combinedKeyLabel(keys.PathSearch.PageUp, keys.PathSearch.PageDown), Action: "page results", Description: "Move the selection by half a page through fuzzy results."},
			{Key: keyListLabel(keys.PathSearch.CompleteNext), Action: "complete/narrow", Description: "Complete the current path segment, or narrow into the selected fuzzy result."},
			{Key: keyListLabel(keys.PathSearch.CompletePrevious), Action: "previous completion", Description: "Cycle backward through the current completion candidates."},
			{Key: keyListLabel(keys.PathSearch.AcceptCompletion), Action: "accept completion cycle", Description: "Clear the current completion cycle so the next Tab completes the next path level."},
			helpItemFromSpec(specs[0]),
			helpItemFromSpec(specs[1]),
			helpItemFromSpec(specs[2]),
			helpItemFromSpec(specs[3]),
			{Key: keyListLabel(keys.PathSearch.OpenLastSession), Action: "open last session", Description: "Switch to tmux's last active session."},
			helpItemFromSpec(specs[4]),
			helpItemFromSpec(specs[5]),
			helpItemFromSpec(specs[6]),
			helpItemFromSpec(specs[7]),
			{Key: keyListLabel(keys.PathSearch.CommandPalette), Action: "command palette", Description: "Open the command palette for path-search actions."},
			helpItemFromSpec(specs[8]),
			helpItemFromSpec(specs[9]),
		}
	}
	if previous == modeCommands {
		specs := browseCommandSpecsFor(keys)
		return []helpItem{
			{Key: "type", Action: "filter commands", Description: "Fuzzy-search commands by title, key, and description."},
			{Key: keyListLabel(keys.Browse.DeleteChar), Action: "remove character", Description: "Remove one character from the command search prompt."},
			{Key: keyListLabel(keys.Browse.DeleteWord), Action: "remove word", Description: "Remove one word from the command search prompt."},
			{Key: keyListLabel(keys.Browse.ClearInput), Action: "clear prompt", Description: "Clear the command search prompt."},
			{Key: combinedKeyLabel(keys.Browse.Up, keys.Browse.Down), Action: "move selection", Description: "Move through available commands."},
			{Key: combinedKeyLabel(keys.Browse.ScrollUp, keys.Browse.ScrollDown), Action: "scroll commands", Description: "Scroll the command viewport one row while keeping the selection visible."},
			{Key: combinedKeyLabel(keys.Browse.PageUp, keys.Browse.PageDown), Action: "page commands", Description: "Move the selection by half a page through available commands."},
			{Key: keyListLabel(keys.Commands.RunSelected), Action: "run selected command", Description: "Run the selected command when it is available in the current context."},
			{Key: keyListLabel(keys.Commands.Close), Action: "close palette", Description: "Close the command palette and return to the previous mode."},
			{Key: keyListLabel(keys.Help.Close), Action: "close help", Description: "Close help and return to the command palette."},
			{Key: keyListLabel(keys.Commands.Help), Action: "toggle help", Description: "Show or close this help popup."},
			{Key: "Quit command", Action: specs[11].Title, Description: specs[11].Description},
		}
	}
	specs := browseCommandSpecsFor(keys)
	return []helpItem{
		{Key: "type", Action: "filter sessions and roots", Description: "Filter open tmux sessions and configured root candidates."},
		{Key: keyListLabel(keys.Browse.DeleteChar), Action: "remove character", Description: "Remove one character from the filter prompt."},
		{Key: keyListLabel(keys.Browse.DeleteWord), Action: "remove word", Description: "Remove one word from the filter prompt."},
		{Key: keyListLabel(keys.Browse.ClearInput), Action: "clear prompt", Description: "Clear the filter prompt."},
		helpItemFromSpec(specs[0]),
		{Key: combinedKeyLabel(keys.Browse.Up, keys.Browse.Down), Action: "move selection", Description: "Move through matching sessions and root candidates."},
		{Key: combinedKeyLabel(keys.Browse.ScrollUp, keys.Browse.ScrollDown), Action: "scroll results", Description: "Scroll the result viewport one row while keeping the selection visible."},
		{Key: combinedKeyLabel(keys.Browse.PageUp, keys.Browse.PageDown), Action: "page results", Description: "Move the selection by half a page through matching sessions and root candidates."},
		{Key: combinedKeyLabel(keys.Browse.JumpNextSection, keys.Browse.JumpPreviousSection), Action: "jump sections", Description: "Jump between open sessions and available workspaces."},
		{Key: keyListLabel(keys.Browse.CommandPalette), Action: "command overlay", Description: "Open the command overlay for less frequent actions."},
		helpItemFromSpec(specs[1]),
		helpItemFromSpec(specs[3]),
		helpItemFromSpec(specs[4]),
		helpItemFromSpec(specs[5]),
		helpItemFromSpec(specs[8]),
		helpItemFromSpec(specs[6]),
		helpItemFromSpec(specs[7]),
		helpItemFromSpec(specs[2]),
		helpItemFromSpec(specs[9]),
		helpItemFromSpec(specs[10]),
		helpItemFromSpec(specs[11]),
	}
}
