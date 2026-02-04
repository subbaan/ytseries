package tui

import (
	"fmt"
	"os"
	"sort"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/subbass/ytseries/internal/cache"
	"github.com/subbass/ytseries/internal/config"
	"github.com/subbass/ytseries/internal/player"
	"github.com/subbass/ytseries/internal/sync"
)

const version = "2.5.3"

// Model represents the application state
type Model struct {
	cfg          *config.Config
	cache        *cache.Cache
	syncer       *sync.Syncer
	player       *player.Player
	channels     []*cache.Channel
	cursor       int
	err          error
	spinner      spinner.Model
	loading      bool
	message      string
	width        int
	height       int
	settingsMode bool
	settings     *settingsModel
	browseMode   bool
	browse       *browseModel
	manageMode   bool
	manage       *manageChannelsModel
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
	}

	return m, nil
}

// handleKeyPress handles keyboard input
func (m *Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	}

	return m, nil
}

// View renders the UI
func (m *Model) View() string {
	// Settings mode
	if m.settingsMode {
		return m.settings.render()
	}

	// Browse mode
	if m.browseMode {
		return m.browse.render()
	}

	// Manage mode
	if m.manageMode {
		return m.manage.render()
	}

	if len(m.channels) == 0 {
		return "No channels configured. Press 'm' to manage channels\n\nPress q to quit"
	}

	// Title with version, inset by 2 chars
	s := "  " + lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorTitle))).
		Render(fmt.Sprintf("ytseries v%s", version)) + "\n\n"

	// Render channel list
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

		s += fmt.Sprintf("%s%s %s%s%s\n", cursor, name, count, divider, nextInfo)
	}

	// Help text
	help := "n:next  l:latest  b:browse  m:manage  s:sync  S:sync-all  c:settings  q:quit"
	helpLen := len(help)

	// Divider - 4 chars longer than help text (2 for inset on each side)
	divider := ""
	for i := 0; i < helpLen+4; i++ {
		divider += "─"
	}

	s += "\n" + lipgloss.NewStyle().
		Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorHelp))).
		Render(divider) + "\n"

	// Help bar inset by 2 chars
	s += "  " + lipgloss.NewStyle().
		Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorHelp))).
		Render(help) + "\n"

	// Message/error
	if m.loading {
		s += "\n" + m.spinner.View() + " " + m.message
	} else if m.message != "" {
		s += "\n" + m.message
	}

	return s
}

// Messages
type syncCompleteMsg struct{}
type syncErrorMsg struct{ err error }
type playCompleteMsg struct{}
type playErrorMsg struct{ err error }
type videoMarkedMsg struct{ title string }
