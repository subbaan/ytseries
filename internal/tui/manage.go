package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/subbass/ytseries/internal/cache"
	"github.com/subbass/ytseries/internal/config"
)

// manageChannelsModel represents the channel management UI
type manageChannelsModel struct {
	cfg              *config.Config
	cache            *cache.Cache
	channels         []*cache.Channel
	cursor           int
	addMode          bool
	addModel         *addChannelModel
	deleteConfirm    bool
	deleteConfModel  *deleteConfirmModel
	deleteConfirm2   bool
	deleteConfModel2 *deleteConfirmModel
}

// newManageChannelsModel creates a new manage channels model
func newManageChannelsModel(cfg *config.Config, c *cache.Cache, channels []*cache.Channel) *manageChannelsModel {
	return &manageChannelsModel{
		cfg:      cfg,
		cache:    c,
		channels: channels,
	}
}

// handleKey handles key presses in manage channels mode
func (m *manageChannelsModel) handleKey(msg tea.KeyMsg) (bool, tea.Cmd, string, string, bool) {
	// Add channel mode
	if m.addMode {
		exit, cmd, name, url := m.addModel.handleKey(msg)
		if exit {
			m.addMode = false
			if name != "" && url != "" {
				// Return to add channel
				return false, cmd, name, url, false
			}
		}
		return false, cmd, "", "", false
	}

	// First delete confirmation
	if m.deleteConfirm {
		exit, confirm := m.deleteConfModel.handleKey(msg)
		if exit {
			m.deleteConfirm = false
			if confirm {
				// Show second confirmation
				m.deleteConfModel2 = newDeleteConfirmModel(m.cfg, m.channels[m.cursor].Name)
				m.deleteConfirm2 = true
			}
		}
		return false, nil, "", "", false
	}

	// Second delete confirmation
	if m.deleteConfirm2 {
		exit, confirm := m.deleteConfModel2.handleKey(msg)
		if exit {
			m.deleteConfirm2 = false
			if confirm {
				// Confirmed twice - delete the channel
				return false, nil, "", "", true
			}
		}
		return false, nil, "", "", false
	}

	switch msg.String() {
	case "q", "esc", "m":
		// Exit manage mode
		return true, nil, "", "", false

	case "a":
		// Enter add channel mode
		m.addModel = newAddChannelModel(m.cfg)
		m.addMode = true
		return false, nil, "", "", false

	case "d":
		// Delete selected channel (first confirmation)
		if m.cursor < len(m.channels) {
			m.deleteConfModel = newDeleteConfirmModel(m.cfg, m.channels[m.cursor].Name)
			m.deleteConfirm = true
		}
		return false, nil, "", "", false

	case "j", "down":
		if m.cursor < len(m.channels)-1 {
			m.cursor++
		} else {
			m.cursor = 0
		}

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		} else {
			m.cursor = len(m.channels) - 1
		}

	case "g":
		m.cursor = 0

	case "G":
		m.cursor = len(m.channels) - 1
	}

	return false, nil, "", "", false
}

// render renders the manage channels UI
func (m *manageChannelsModel) render() string {
	// Add channel mode
	if m.addMode {
		return m.addModel.render()
	}

	// Delete confirmations
	if m.deleteConfirm {
		return m.deleteConfModel.render()
	}
	if m.deleteConfirm2 {
		s := m.deleteConfModel2.render()
		warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
		return s + "\n" + warningStyle.Render("⚠ SECOND CONFIRMATION - This action cannot be undone!")
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorTitle)))

	s := titleStyle.Render("Manage Channels") + "\n\n"

	if len(m.channels) == 0 {
		s += "No channels configured.\n\n"
	} else {
		// Render channel list
		for i, ch := range m.channels {
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
			if i == m.cursor {
				nameStyle = nameStyle.Bold(true).Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorSelected)))
			}

			s += fmt.Sprintf("%s%s (%d unwatched)\n",
				cursor,
				nameStyle.Render(ch.Name),
				ch.UnwatchedCount)
		}
		s += "\n"
	}

	// Help
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorHelp)))
	s += helpStyle.Render("a:add  d:delete  j/k:navigate  q/esc/m:back") + "\n"

	return s
}


// addChannelModel represents the add channel UI
type addChannelModel struct {
	cfg          *config.Config
	nameInput    textinput.Model
	urlInput     textinput.Model
	focusIndex   int
	err          string
	submitting   bool
}

// newAddChannelModel creates a new add channel model
func newAddChannelModel(cfg *config.Config) *addChannelModel {
	nameInput := textinput.New()
	nameInput.Placeholder = "channel name"
	nameInput.Focus()
	nameInput.CharLimit = 50
	nameInput.Width = 50

	urlInput := textinput.New()
	urlInput.Placeholder = "https://youtube.com/@channelname"
	urlInput.CharLimit = 200
	urlInput.Width = 50

	return &addChannelModel{
		cfg:       cfg,
		nameInput: nameInput,
		urlInput:  urlInput,
	}
}

// handleKey handles key presses in add channel mode
func (m *addChannelModel) handleKey(msg tea.KeyMsg) (bool, tea.Cmd, string, string) {
	switch msg.String() {
	case "esc", "ctrl+c":
		// Cancel
		return true, nil, "", ""

	case "tab", "shift+tab":
		// Switch focus
		if m.focusIndex == 0 {
			m.focusIndex = 1
			m.nameInput.Blur()
			m.urlInput.Focus()
		} else {
			m.focusIndex = 0
			m.urlInput.Blur()
			m.nameInput.Focus()
		}
		return false, nil, "", ""

	case "enter":
		// Submit
		name := strings.TrimSpace(m.nameInput.Value())
		url := strings.TrimSpace(m.urlInput.Value())

		if name == "" {
			m.err = "Name cannot be empty"
			return false, nil, "", ""
		}

		if url == "" {
			m.err = "URL cannot be empty"
			return false, nil, "", ""
		}

		// Return name and url to be processed
		return true, nil, name, url
	}

	// Update focused input
	var cmd tea.Cmd
	if m.focusIndex == 0 {
		m.nameInput, cmd = m.nameInput.Update(msg)
	} else {
		m.urlInput, cmd = m.urlInput.Update(msg)
	}

	return false, cmd, "", ""
}

// render renders the add channel UI
func (m *addChannelModel) render() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorTitle)))

	s := titleStyle.Render("Add Channel") + "\n\n"

	// Name input
	s += "Name: " + m.nameInput.View() + "\n\n"

	// URL input
	s += "URL:  " + m.urlInput.View() + "\n\n"

	// Help
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorHelp)))
	s += helpStyle.Render("tab:switch  enter:submit  esc:cancel") + "\n"

	// Error
	if m.err != "" {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		s += "\n" + errorStyle.Render(fmt.Sprintf("Error: %s", m.err))
	}

	return s
}

// deleteConfirmModel represents the delete confirmation dialog
type deleteConfirmModel struct {
	cfg         *config.Config
	channelName string
}

// newDeleteConfirmModel creates a new delete confirmation model
func newDeleteConfirmModel(cfg *config.Config, channelName string) *deleteConfirmModel {
	return &deleteConfirmModel{
		cfg:         cfg,
		channelName: channelName,
	}
}

// handleKey handles key presses in delete confirmation mode
func (m *deleteConfirmModel) handleKey(msg tea.KeyMsg) (bool, bool) {
	switch msg.String() {
	case "y", "Y":
		// Confirm delete
		return true, true
	case "n", "N", "esc", "q":
		// Cancel delete
		return true, false
	}
	return false, false
}

// render renders the delete confirmation dialog
func (m *deleteConfirmModel) render() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorTitle)))

	s := titleStyle.Render("Delete Channel") + "\n\n"

	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	s += warningStyle.Render(fmt.Sprintf("Delete channel '%s'?", m.channelName)) + "\n\n"

	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorHelp)))
	s += helpStyle.Render("y:yes  n:no") + "\n"

	return s
}
