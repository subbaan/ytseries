package tui

import (
	"fmt"
	"os"
	"sort"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/subbass/ytseries/internal/cache"
	"github.com/subbass/ytseries/internal/config"
	"github.com/subbass/ytseries/internal/player"
	"github.com/subbass/ytseries/internal/sync"
)

const version = "2.5.9"

// Model represents the application state
type Model struct {
	cfg             *config.Config
	cache           *cache.Cache
	syncer          *sync.Syncer
	player          *player.Player
	channels        []*cache.Channel
	cursor          int
	err             error
	spinner         spinner.Model
	loading         bool
	message         string
	width           int
	height          int
	settingsMode    bool
	settings        *settingsModel
	browseMode      bool
	browse          *browseModel
	manageMode      bool
	manage          *manageChannelsModel
	confirmSkip     bool         // waiting for y/n after members-only failure
	skipChannelKey  string
	skipChannelName string
	skipVideo       *cache.Video
}

// New creates a new TUI model
func New(cfg *config.Config, c *cache.Cache, s *sync.Syncer, p *player.Player) (*Model, error) {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	m := &Model{
		cfg:      cfg,
		cache:    c,
		syncer:   s,
		player:   p,
		spinner:  sp,
		settings: newSettingsModel(cfg),
	}

	// Load channels
	if err := m.loadChannels(); err != nil {
		return nil, err
	}

	return m, nil
}

// loadChannels loads all channels from cache
func (m *Model) loadChannels() error {
	aliases, err := m.cache.ReadAliases()
	if err != nil {
		return err
	}

	keys, err := m.cache.ReadKeys()
	if err != nil {
		return err
	}

	channels := make([]*cache.Channel, 0, len(aliases))

	for name, url := range aliases {
		// Get channel key
		key, ok := keys[sync.NormalizeURL(url)]
		if !ok {
			// Skip channels without keys
			continue
		}

		// Rebuild recent cache with current config (auto-updates when config changes)
		if err := m.cache.RebuildRecent(key, m.cfg.RecentHours); err != nil {
			// Log error but continue - will use existing cache if available
			fmt.Fprintf(os.Stderr, "Warning: Failed to rebuild recent cache for %s: %v\n", name, err)
		}

		// Load channel info
		ch, err := m.cache.GetChannelInfo(name, key, m.cfg.RecentHours)
		if err != nil {
			continue
		}

		ch.URL = url
		channels = append(channels, ch)
	}

	// Sort channels:
	// 1. Channels with recent episodes (top, alphabetically)
	// 2. Channels with unwatched but no recent (middle, alphabetically)
	// 3. All watched channels (bottom, alphabetically)
	sort.Slice(channels, func(i, j int) bool {
		a, b := channels[i], channels[j]

		// Check if has recent
		aHasRecent := a.HasRecent
		bHasRecent := b.HasRecent

		// Check if has unwatched
		aHasUnwatched := a.UnwatchedCount > 0
		bHasUnwatched := b.UnwatchedCount > 0

		// Priority: recent > unwatched > alphabetical
		if aHasRecent != bHasRecent {
			return aHasRecent // true comes first
		}

		if aHasUnwatched != bHasUnwatched {
			return aHasUnwatched // true comes first
		}

		// Same category, sort alphabetically
		return a.Name < b.Name
	})

	m.channels = channels
	return nil
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update handles messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.browse != nil {
			m.browse.height = msg.Height
			m.browse.width = msg.Width
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case syncCompleteMsg:
		m.loading = false
		m.message = "Sync complete!"
		m.loadChannels()
		return m, nil

	case syncErrorMsg:
		m.loading = false
		m.err = msg.err
		m.message = fmt.Sprintf("Error: %v", msg.err)
		return m, nil

	case playCompleteMsg:
		m.message = "" // Don't show "playback complete" - user closed it themselves
		m.loadChannels() // Reload to update watched status
		return m, nil

	case videoMarkedMsg:
		m.message = fmt.Sprintf("✓ Marked as watched: %s", msg.title)
		m.loadChannels() // Reload to update watched status
		return m, nil

	case playErrorMsg:
		m.err = msg.err
		m.message = fmt.Sprintf("Error: %v", msg.err)
		return m, nil

	case membersOnlyMsg:
		m.confirmSkip = true
		m.skipChannelKey = msg.channelKey
		m.skipChannelName = msg.channelName
		m.skipVideo = msg.video
		m.message = ""
		return m, nil
	}

	return m, nil
}

// handleKeyPress handles keyboard input
func (m *Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// confirmSkip takes priority over all other modes
	if m.confirmSkip {
		switch msg.String() {
		case "y", "Y":
			// Mark the members-only video as watched and play the next one
			key := m.skipChannelKey
			name := m.skipChannelName
			vid := m.skipVideo
			m.confirmSkip = false
			m.skipChannelKey = ""
			m.skipChannelName = ""
			m.skipVideo = nil
			m.message = ""
			// Exit browse mode if it was active when the error occurred
			m.browseMode = false
			m.browse = nil
			if err := m.cache.MarkWatched(key, vid.ID); err != nil {
				m.message = fmt.Sprintf("Error marking watched: %v", err)
				return m, nil
			}
			m.loadChannels()
			// Restore cursor to the same channel by name
			for i, ch := range m.channels {
				if ch.Name == name {
					m.cursor = i
					break
				}
			}
			return m, m.playNext()
		case "n", "N", "esc":
			m.confirmSkip = false
			m.skipChannelKey = ""
			m.skipChannelName = ""
			m.skipVideo = nil
			m.message = ""
		}
		return m, nil
	}

	// Settings mode handling
	if m.settingsMode {
		exit, cmd := m.settings.handleKey(msg)
		if exit {
			m.settingsMode = false
			m.loadChannels() // Reload in case colors changed
		}
		return m, cmd
	}

	// Browse mode handling
	if m.browseMode {
		exit, cmd := m.browse.handleKey(msg)
		if exit {
			m.browseMode = false
			m.loadChannels() // Reload to update watched counts
		}
		return m, cmd
	}

	// Manage mode handling
	if m.manageMode {
		exit, cmd, name, url, confirmDelete := m.manage.handleKey(msg)
		if exit {
			m.manageMode = false
			m.loadChannels() // Reload channel list
		} else if name != "" && url != "" {
			// Add channel
			return m, m.addChannelFunc(name, url)
		} else if confirmDelete {
			// Delete channel at manage cursor position
			if m.manage.cursor < len(m.manage.channels) {
				ch := m.manage.channels[m.manage.cursor]
				// Set main cursor to match the channel being deleted
				for i, mainCh := range m.channels {
					if mainCh.Name == ch.Name {
						m.cursor = i
						break
					}
				}
				return m, m.deleteChannelFunc()
			}
		}
		return m, cmd
	}

	if m.loading {
		return m, nil // Ignore input while loading
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "m":
		// Enter manage mode
		m.manage = newManageChannelsModel(m.cfg, m.cache, m.channels)
		m.manageMode = true
		return m, nil

	case "c":
		m.settingsMode = true
		return m, nil

	case "b":
		// Enter browse mode for current channel
		if m.cursor < len(m.channels) {
			ch := m.channels[m.cursor]
			browse, err := newBrowseModel(m.cfg, m.cache, m.player, ch)
			if err != nil {
				m.message = fmt.Sprintf("Error: %v", err)
				return m, nil
			}
			browse.height = m.height
			browse.width = m.width
			m.browse = browse
			m.browseMode = true
		}
		return m, nil

	case "j", "down":
		if m.cursor < len(m.channels)-1 {
			m.cursor++
			m.message = ""
		}

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			m.message = ""
		}

	case "g":
		m.cursor = 0
		m.message = ""

	case "G":
		m.cursor = len(m.channels) - 1
		m.message = ""

	case "n", "enter":
		return m, m.playNext()

	case "l":
		return m, m.playLatest()

	case "s":
		return m, m.syncChannel()

	case "S":
		return m, m.syncAllChannels()

	case "u":
		if m.cursor < len(m.channels) {
			ch := m.channels[m.cursor]
			if ch.NextEpisode != nil {
				url := "https://www.youtube.com/watch?v=" + ch.NextEpisode.ID
				if err := clipboard.WriteAll(url); err != nil {
					m.message = fmt.Sprintf("Clipboard error: %v", err)
				} else {
					m.message = "Copied: " + url
				}
			} else {
				m.message = "No video URL to copy"
			}
		}
	}

	return m, nil
}

// View renders the UI
func (m *Model) View() string {
	// Modal for members-only skip confirmation (takes priority over all modes)
	if m.confirmSkip {
		return m.renderModal()
	}

	// Settings mode
	if m.settingsMode {
		return m.settings.render(m.width)
	}

	// Browse mode
	if m.browseMode {
		return m.browse.render()
	}

	// Manage mode
	if m.manageMode {
		return m.manage.render(m.width)
	}

	if len(m.channels) == 0 {
		return renderTitleBar("ytseries v"+version, m.width, m.cfg.ColorTitle) +
			"\nNo channels configured. Press 'm' to manage channels\n\nPress q to quit"
	}

	// Title bar (line 1)
	s := renderTitleBar("ytseries v"+version, m.width, m.cfg.ColorTitle)
	// Blank line after title bar (line 2)
	s += "\n"

	// Build channel rows
	var channelRows []string
	for i, ch := range m.channels {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		// Channel name (fixed width: 10 chars)
		nameColor := m.cfg.ColorSelected
		if i != m.cursor {
			nameColor = m.cfg.ColorChannelName
		}
		nameStr := ch.Name
		if len(nameStr) > 10 {
			nameStr = nameStr[:10]
		}
		name := lipgloss.NewStyle().
			Bold(i == m.cursor).
			Foreground(lipgloss.Color(fmt.Sprintf("%d", nameColor))).
			Width(10).
			Render(nameStr)

		// Unwatched count (fixed width: "nnnn unwatched")
		countStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorUnwatched)))
		countStr := fmt.Sprintf("%4d unwatched", ch.UnwatchedCount)
		count := countStyle.Render(countStr)

		// Divider
		divider := lipgloss.NewStyle().
			Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorDivider))).
			Render(" | ")

		// Next episode info
		nextInfo := ""
		if ch.NextEpisode != nil {
			dateColor := m.cfg.ColorDate
			titleColor := m.cfg.ColorDate

			// Check if channel has any recent episodes (not just if next episode is recent)
			if ch.HasRecent {
				dateColor = m.cfg.ColorRecent
				titleColor = m.cfg.ColorRecent
			}

			dateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("%d", dateColor)))
			titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("%d", titleColor)))
			dividerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorDivider)))

			nextInfo = fmt.Sprintf("%s%s%s",
				dateStyle.Render(formatDate(ch.NextEpisode.Date)),
				dividerStyle.Render(" | "),
				titleStyle.Render(ch.NextEpisode.Title))
		} else {
			nextInfo = lipgloss.NewStyle().
				Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorDate))).
				Render("All watched")
		}

		channelRows = append(channelRows, fmt.Sprintf("%s%s %s%s%s", cursor, name, count, divider, nextInfo))
	}

	// Fixed overhead: title(1) + blank(1) + blank-before-divider(1) + divider(1) + help(1) + status(1) = 6
	const fixedLines = 6
	availableLines := m.height - fixedLines
	if m.height == 0 || availableLines < len(channelRows) {
		availableLines = len(channelRows)
	}

	for _, row := range channelRows {
		s += row + "\n"
	}

	// Padding to keep footer at a fixed vertical position
	padding := availableLines - len(channelRows)
	for i := 0; i < padding; i++ {
		s += "\n"
	}

	// Help text
	help := "n:next  l:latest  b:browse  u:copy-url  m:manage  s:sync  S:sync-all  c:settings  q:quit"
	helpLen := len(help)

	// Divider - 4 chars longer than help text (2 inset on each side)
	dividerStr := ""
	for i := 0; i < helpLen+4; i++ {
		dividerStr += "─"
	}

	// Blank line before divider
	s += "\n"
	s += lipgloss.NewStyle().
		Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorHelp))).
		Render(dividerStr) + "\n"

	// Help bar inset by 2 chars
	s += "  " + lipgloss.NewStyle().
		Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorHelp))).
		Render(help) + "\n"

	// Status bar (always 1 line, no trailing newline)
	s += m.renderStatusBar()

	return s
}

// renderTitleBar renders a full-width reverse-colour title bar using the configured
// title colour as the bar background. When width is 0 falls back to a simple inset title.
func renderTitleBar(title string, width int, colorTitle int) string {
	if width == 0 {
		return lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(fmt.Sprintf("%d", colorTitle))).
			Render("  "+title) + "\n"
	}
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(fmt.Sprintf("%d", colorTitle))).
		Reverse(true).
		Width(width).
		Padding(0, 1).
		Render(title) + "\n"
}

// renderStatusBar renders a fixed one-line reverse-colour status bar at the bottom.
// It always shows "Status:" so it is permanently reserved screen space.
func (m *Model) renderStatusBar() string {
	var content string
	if m.loading {
		content = "Status: " + m.spinner.View() + "  " + m.message
	} else if m.message != "" {
		content = "Status: " + m.message
	} else {
		content = "Status:"
	}
	if m.width == 0 {
		return lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorTitle))).
			Render("  " + content)
	}
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorTitle))).
		Reverse(true).
		Width(m.width).
		Padding(0, 1).
		Render(content)
}

// renderModal renders the centred bordered members-only skip confirmation popup.
func (m *Model) renderModal() string {
	title := m.skipVideo.Title
	if len(title) > 52 {
		title = title[:49] + "..."
	}

	heading := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorTitle))).
		Render("Members-Only Video")

	body := lipgloss.JoinVertical(lipgloss.Center,
		heading,
		"",
		title,
		"",
		"[y] Skip & mark as watched    [n] Cancel",
	)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorTitle))).
		Padding(1, 4).
		Render(body)

	titleBar := renderTitleBar("ytseries v"+version, m.width, m.cfg.ColorTitle)
	remaining := m.height - 1
	if remaining < 1 {
		remaining = 24
	}
	placed := lipgloss.Place(m.width, remaining, lipgloss.Center, lipgloss.Center, box)
	return titleBar + placed
}

// Messages
type syncCompleteMsg struct{}
type syncErrorMsg struct{ err error }
type playCompleteMsg struct{}
type playErrorMsg struct{ err error }
type videoMarkedMsg struct{ title string }
type membersOnlyMsg struct {
	channelKey  string
	channelName string
	video       *cache.Video
}
