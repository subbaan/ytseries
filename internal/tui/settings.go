package tui

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/subbass/ytseries/internal/config"
)

// settingsModel represents the settings UI
type settingsModel struct {
	cfg                *config.Config
	cursor             int
	settingNames       []string
	previewColors      bool
	editingCookiesPath bool
	cookiesPathInput   string
}

// newSettingsModel creates a new settings model
func newSettingsModel(cfg *config.Config) *settingsModel {
	return &settingsModel{
		cfg: cfg,
		settingNames: []string{
			"Player",
			"Quality",
			"Browser",
			"Cookies File Path",
			"Mark Threshold",
			"Recent Hours",
			"Color: Title",
			"Color: Selected",
			"Color: Channel Name",
			"Color: Unwatched",
			"Color: Recent",
			"Color: Date",
			"Color: Divider",
			"Color: Help",
		},
		previewColors: true, // Show preview by default
	}
}

// handleSettingsKey handles key presses in settings mode
func (m *settingsModel) handleKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	// Handle cookies path editing mode
	if m.editingCookiesPath {
		switch msg.String() {
		case "esc":
			// Cancel editing
			m.editingCookiesPath = false
			m.cookiesPathInput = ""
		case "enter":
			// Save the path
			m.cfg.CookiesFile = m.cookiesPathInput
			m.cfg.Save()
			m.editingCookiesPath = false
			m.cookiesPathInput = ""
		case "backspace":
			if len(m.cookiesPathInput) > 0 {
				m.cookiesPathInput = m.cookiesPathInput[:len(m.cookiesPathInput)-1]
			}
		default:
			// Add character to input
			if len(msg.String()) == 1 {
				m.cookiesPathInput += msg.String()
			}
		}
		return false, nil
	}

	// Normal settings navigation
	switch msg.String() {
	case "q", "esc", "c":
		// Exit settings mode
		return true, nil

	case "j", "down":
		if m.cursor < len(m.settingNames)-1 {
			m.cursor++
		}

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}

	case "g":
		m.cursor = 0

	case "G":
		m.cursor = len(m.settingNames) - 1

	case "enter", "e":
		// Enter edit mode for Cookies File Path if browser is custom
		if m.cursor == 3 && m.cfg.Browser == "custom" {
			m.editingCookiesPath = true
			m.cookiesPathInput = m.cfg.CookiesFile
		}

	case "left", "h":
		m.decrementSetting()

	case "right", "l":
		m.incrementSetting()

	case "p":
		// Toggle color preview
		m.previewColors = !m.previewColors
	}

	return false, nil
}

// incrementSetting increases the current setting value
func (m *settingsModel) incrementSetting() {
	switch m.cursor {
	case 0: // Player
		players := config.PlayerOptions()
		for i, p := range players {
			if p == m.cfg.Player {
				m.cfg.Player = players[(i+1)%len(players)]
				break
			}
		}

	case 1: // Quality
		qualities := config.QualityOptions()
		for i, q := range qualities {
			if q == m.cfg.Quality {
				m.cfg.Quality = qualities[(i+1)%len(qualities)]
				m.cfg.YtdlFormat = m.cfg.ComputeYtdlFormat()
				break
			}
		}

	case 2: // Browser
		browsers := config.BrowserOptions()
		for i, b := range browsers {
			if b == m.cfg.Browser {
				m.cfg.Browser = browsers[(i+1)%len(browsers)]
				break
			}
		}

	case 3: // Cookies File Path - handled by text input, skip
		return

	case 4: // Mark Threshold
		if m.cfg.MarkThreshold < 100 {
			m.cfg.MarkThreshold += 5
		}

	case 5: // Recent Hours
		if m.cfg.RecentHours < 168 {
			m.cfg.RecentHours += 12
		}

	case 6: // Color: Title
		m.cfg.ColorTitle = (m.cfg.ColorTitle + 1) % 256

	case 7: // Color: Selected
		m.cfg.ColorSelected = (m.cfg.ColorSelected + 1) % 256

	case 8: // Color: Channel Name
		m.cfg.ColorChannelName = (m.cfg.ColorChannelName + 1) % 256

	case 9: // Color: Unwatched
		m.cfg.ColorUnwatched = (m.cfg.ColorUnwatched + 1) % 256

	case 10: // Color: Recent
		m.cfg.ColorRecent = (m.cfg.ColorRecent + 1) % 256

	case 11: // Color: Date
		m.cfg.ColorDate = (m.cfg.ColorDate + 1) % 256

	case 12: // Color: Divider
		m.cfg.ColorDivider = (m.cfg.ColorDivider + 1) % 256

	case 13: // Color: Help
		m.cfg.ColorHelp = (m.cfg.ColorHelp + 1) % 256
	}

	// Save config
	m.cfg.Save()
}

// decrementSetting decreases the current setting value
func (m *settingsModel) decrementSetting() {
	switch m.cursor {
	case 0: // Player
		players := config.PlayerOptions()
		for i, p := range players {
			if p == m.cfg.Player {
				idx := (i - 1 + len(players)) % len(players)
				m.cfg.Player = players[idx]
				break
			}
		}

	case 1: // Quality
		qualities := config.QualityOptions()
		for i, q := range qualities {
			if q == m.cfg.Quality {
				idx := (i - 1 + len(qualities)) % len(qualities)
				m.cfg.Quality = qualities[idx]
				m.cfg.YtdlFormat = m.cfg.ComputeYtdlFormat()
				break
			}
		}

	case 2: // Browser
		browsers := config.BrowserOptions()
		for i, b := range browsers {
			if b == m.cfg.Browser {
				idx := (i - 1 + len(browsers)) % len(browsers)
				m.cfg.Browser = browsers[idx]
				break
			}
		}

	case 3: // Cookies File Path - handled by text input, skip
		return

	case 4: // Mark Threshold
		if m.cfg.MarkThreshold > 0 {
			m.cfg.MarkThreshold -= 5
		}

	case 5: // Recent Hours
		if m.cfg.RecentHours > 0 {
			m.cfg.RecentHours -= 12
		}

	case 6: // Color: Title
		m.cfg.ColorTitle = (m.cfg.ColorTitle - 1 + 256) % 256

	case 7: // Color: Selected
		m.cfg.ColorSelected = (m.cfg.ColorSelected - 1 + 256) % 256

	case 8: // Color: Channel Name
		m.cfg.ColorChannelName = (m.cfg.ColorChannelName - 1 + 256) % 256

	case 9: // Color: Unwatched
		m.cfg.ColorUnwatched = (m.cfg.ColorUnwatched - 1 + 256) % 256

	case 10: // Color: Recent
		m.cfg.ColorRecent = (m.cfg.ColorRecent - 1 + 256) % 256

	case 11: // Color: Date
		m.cfg.ColorDate = (m.cfg.ColorDate - 1 + 256) % 256

	case 12: // Color: Divider
		m.cfg.ColorDivider = (m.cfg.ColorDivider - 1 + 256) % 256

	case 13: // Color: Help
		m.cfg.ColorHelp = (m.cfg.ColorHelp - 1 + 256) % 256
	}

	// Save config
	m.cfg.Save()
}

// renderSettings renders the settings UI
func (m *settingsModel) render() string {
	s := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(strconv.Itoa(m.cfg.ColorTitle))).
		Render("Settings") + "\n\n"

	// Build cookies file path value
	cookiesValue := ""
	if m.editingCookiesPath {
		cookiesValue = m.cookiesPathInput + "█" // Show cursor
	} else if m.cfg.Browser == "custom" {
		if m.cfg.CookiesFile == "" {
			cookiesValue = "(not set)"
		} else {
			cookiesValue = m.cfg.CookiesFile
		}
	} else {
		cookiesValue = "(disabled)"
	}

	settings := []struct {
		name  string
		value string
	}{
		{"Player", m.cfg.Player},
		{"Quality", m.cfg.Quality},
		{"Browser", m.cfg.Browser},
		{"Cookies File Path", cookiesValue},
		{"Mark Threshold", fmt.Sprintf("%d%%", m.cfg.MarkThreshold)},
		{"Recent Hours", fmt.Sprintf("%dh", m.cfg.RecentHours)},
		{"Color: Title", fmt.Sprintf("%d", m.cfg.ColorTitle)},
		{"Color: Selected", fmt.Sprintf("%d", m.cfg.ColorSelected)},
		{"Color: Channel Name", fmt.Sprintf("%d", m.cfg.ColorChannelName)},
		{"Color: Unwatched", fmt.Sprintf("%d", m.cfg.ColorUnwatched)},
		{"Color: Recent", fmt.Sprintf("%d", m.cfg.ColorRecent)},
		{"Color: Date", fmt.Sprintf("%d", m.cfg.ColorDate)},
		{"Color: Divider", fmt.Sprintf("%d", m.cfg.ColorDivider)},
		{"Color: Help", fmt.Sprintf("%d", m.cfg.ColorHelp)},
	}

	for i, setting := range settings {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		nameStyle := lipgloss.NewStyle().
			Bold(i == m.cursor).
			Foreground(lipgloss.Color("7")).
			Width(20)

		// Special handling for Cookies File Path (can be long)
		valueWidth := 8
		if i == 3 {
			valueWidth = 50 // Allow longer paths
		}

		valueStyle := lipgloss.NewStyle().
			Bold(i == m.cursor).
			Foreground(lipgloss.Color("11"))

		// Gray out disabled cookies path
		if i == 3 && m.cfg.Browser != "custom" && !m.editingCookiesPath {
			valueStyle = valueStyle.Foreground(lipgloss.Color("8"))
		}

		// For Cookies File Path, add inline note when browser is custom
		if i == 3 {
			noteText := ""
			if m.cfg.Browser == "custom" {
				noteStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("8")).
					Italic(true)
				if m.editingCookiesPath {
					noteText = " " + noteStyle.Render("[editing, Enter to save, Esc to cancel]")
				} else {
					noteText = " " + noteStyle.Render("[Press Enter/e to edit]")
				}
			}
			s += fmt.Sprintf("%s%s %s%s\n",
				cursor,
				nameStyle.Render(setting.name),
				valueStyle.Width(valueWidth).Render(setting.value),
				noteText)
			continue
		}

		// For color settings, show preview
		if i >= 6 && m.previewColors {
			colorIdx := 0
			switch i {
			case 6:
				colorIdx = m.cfg.ColorTitle
			case 7:
				colorIdx = m.cfg.ColorSelected
			case 8:
				colorIdx = m.cfg.ColorChannelName
			case 9:
				colorIdx = m.cfg.ColorUnwatched
			case 10:
				colorIdx = m.cfg.ColorRecent
			case 11:
				colorIdx = m.cfg.ColorDate
			case 12:
				colorIdx = m.cfg.ColorDivider
			case 13:
				colorIdx = m.cfg.ColorHelp
			}

			preview := lipgloss.NewStyle().
				Foreground(lipgloss.Color(strconv.Itoa(colorIdx))).
				Render("◼ Preview Text")

			s += fmt.Sprintf("%s%s %s %s\n",
				cursor,
				nameStyle.Render(setting.name),
				valueStyle.Width(valueWidth).Render(setting.value),
				preview)
		} else {
			s += fmt.Sprintf("%s%s %s\n",
				cursor,
				nameStyle.Render(setting.name),
				valueStyle.Width(valueWidth).Render(setting.value))
		}
	}

	// Live preview section
	if m.previewColors {
		s += "\n" + lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(strconv.Itoa(m.cfg.ColorTitle))).
			Render("Preview") + "\n\n"

		// Sample channel 1: Selected with recent episode
		nameStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(strconv.Itoa(m.cfg.ColorSelected))).
			Width(10)
		countStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(strconv.Itoa(m.cfg.ColorUnwatched)))
		dateStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(strconv.Itoa(m.cfg.ColorRecent)))
		titleStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(strconv.Itoa(m.cfg.ColorRecent)))
		dividerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(strconv.Itoa(m.cfg.ColorDivider)))

		s += fmt.Sprintf("> %s %s %s %s %s\n",
			nameStyle.Render("channel1"),
			countStyle.Render("  12 unwatched"),
			dividerStyle.Render("|"),
			dateStyle.Render("20241120"),
			titleStyle.Render("Recent Episode Title"))

		// Sample channel 2: Not selected, no recent
		nameStyle2 := lipgloss.NewStyle().
			Foreground(lipgloss.Color(strconv.Itoa(m.cfg.ColorChannelName))).
			Width(10)
		dateStyle2 := lipgloss.NewStyle().
			Foreground(lipgloss.Color(strconv.Itoa(m.cfg.ColorDate)))
		titleStyle2 := lipgloss.NewStyle().
			Foreground(lipgloss.Color(strconv.Itoa(m.cfg.ColorDate)))

		s += fmt.Sprintf("  %s %s %s %s %s\n",
			nameStyle2.Render("channel2"),
			countStyle.Render("   5 unwatched"),
			dividerStyle.Render("|"),
			dateStyle2.Render("20241015"),
			titleStyle2.Render("Older Episode Title"))

		// Sample channel 3: All watched
		s += fmt.Sprintf("  %s %s %s %s\n\n",
			nameStyle2.Render("channel3"),
			countStyle.Render("   0 unwatched"),
			dividerStyle.Render("|"),
			dateStyle2.Render("All watched"))
	}

	// Help text
	helpText := ""
	if m.editingCookiesPath {
		helpText = "Type path  enter: save  esc: cancel"
	} else {
		helpText = "h/l or ←/→: adjust  j/k: navigate  p: toggle preview  c/q/esc: back"
	}

	s += "\n" + lipgloss.NewStyle().
		Foreground(lipgloss.Color(strconv.Itoa(m.cfg.ColorHelp))).
		Render(helpText) + "\n"

	return s
}
