package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/subbass/ytseries/internal/cache"
	"github.com/subbass/ytseries/internal/config"
	"github.com/subbass/ytseries/internal/player"
)

// formatDate converts YYYYMMDD to "Jan 15 2024" format
func formatDate(dateStr string) string {
	// Handle special case for unknown dates
	if dateStr == "99999999" || dateStr == "" {
		return "Unknown"
	}

	// Parse YYYYMMDD format
	t, err := time.Parse("20060102", dateStr)
	if err != nil {
		return dateStr // Return original if parsing fails
	}

	return t.Format("Jan 02 2006")
}

// browseModel represents the browse/mark UI
type browseModel struct {
	cfg            *config.Config
	cache          *cache.Cache
	player         *player.Player
	channel        *cache.Channel
	videos         []cache.Video
	watched        map[string]bool
	recent         map[string]bool
	cursor         int
	offset         int
	height         int
	width          int
	searchMode     bool     // Whether in search input mode
	searchQuery    string   // Current search query
	filteredVideos []int    // Indices of videos matching search (nil = no filter)
	message        string   // Transient status message (cleared on next keypress)
}

// newBrowseModel creates a new browse model
func newBrowseModel(cfg *config.Config, c *cache.Cache, p *player.Player, ch *cache.Channel) (*browseModel, error) {
	videos, err := c.ReadVideos(ch.Key)
	if err != nil {
		return nil, err
	}

	watched, err := c.ReadWatched(ch.Key)
	if err != nil {
		return nil, err
	}

	recent, err := c.ReadRecent(ch.Key)
	if err != nil {
		return nil, err
	}

	// Reverse videos so latest are at the top
	for i := 0; i < len(videos)/2; i++ {
		j := len(videos) - 1 - i
		videos[i], videos[j] = videos[j], videos[i]
	}

	return &browseModel{
		cfg:     cfg,
		cache:   c,
		player:  p,
		channel: ch,
		videos:  videos,
		watched: watched,
		recent:  recent,
		height:  20, // Default height, will be updated
		width:   0,  // Default width, will be updated
	}, nil
}

// handleBrowseKey handles key presses in browse mode
func (m *browseModel) handleKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	// Handle search mode input
	if m.searchMode {
		switch msg.String() {
		case "esc":
			// Exit search mode and clear filter
			m.searchMode = false
			m.searchQuery = ""
			m.filteredVideos = nil
			m.cursor = 0
			m.offset = 0
			return false, nil
		case "enter":
			// Exit search mode but keep filter active
			m.searchMode = false
			return false, nil
		case "backspace":
			if len(m.searchQuery) > 0 {
				m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
				m.filterVideos()
			}
			return false, nil
		default:
			// Add character to search query
			if len(msg.String()) == 1 {
				m.searchQuery += msg.String()
				m.filterVideos()
			}
			return false, nil
		}
	}

	m.message = "" // Clear any transient message on keypress

	switch msg.String() {
	case "q", "esc", "b":
		// Exit browse mode
		return true, nil

	case "/":
		// Enter search mode
		m.searchMode = true
		m.searchQuery = ""
		m.filteredVideos = nil
		return false, nil

	case "enter":
		// Play selected video
		videoCount := m.getVideoCount()
		if m.cursor < videoCount {
			return false, m.playVideo()
		}

	case "j", "down":
		videoCount := m.getVideoCount()
		if m.cursor < videoCount-1 {
			m.cursor++
		} else {
			// Wrap to top
			m.cursor = 0
		}
		m.adjustOffset()

	case "k", "up":
		videoCount := m.getVideoCount()
		if m.cursor > 0 {
			m.cursor--
		} else {
			// Wrap to bottom
			m.cursor = videoCount - 1
		}
		m.adjustOffset()

	case "pgdown", "ctrl+f":
		videoCount := m.getVideoCount()
		visibleLines := m.height - 5
		m.cursor += visibleLines
		if m.cursor >= videoCount {
			m.cursor = videoCount - 1
		}
		m.adjustOffset()

	case "pgup", "ctrl+b":
		visibleLines := m.height - 5
		m.cursor -= visibleLines
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.adjustOffset()

	case "g":
		m.cursor = 0
		m.offset = 0

	case "G":
		videoCount := m.getVideoCount()
		m.cursor = videoCount - 1
		m.adjustOffset()

	case "m", " ":
		// Toggle mark/unmark
		videoCount := m.getVideoCount()
		if m.cursor < videoCount {
			videoIdx := m.getVideoIndex(m.cursor)
			videoID := m.videos[videoIdx].ID
			if m.watched[videoID] {
				// Unmark
				m.cache.UnmarkWatched(m.channel.Key, videoID)
				delete(m.watched, videoID)
			} else {
				// Mark
				m.cache.MarkWatched(m.channel.Key, videoID)
				m.watched[videoID] = true
			}

			// Auto-advance to next video
			if m.cursor < videoCount-1 {
				m.cursor++
				m.adjustOffset()
			}
		}

	case "u":
		// Copy video URL to clipboard
		videoCount := m.getVideoCount()
		if m.cursor < videoCount {
			videoIdx := m.getVideoIndex(m.cursor)
			video := m.videos[videoIdx]
			url := "https://www.youtube.com/watch?v=" + video.ID
			if err := clipboard.WriteAll(url); err != nil {
				m.message = fmt.Sprintf("Clipboard error: %v", err)
			} else {
				m.message = "Copied: " + url
			}
		}
	}

	return false, nil
}

// getVideoCount returns the number of videos (filtered or total)
func (m *browseModel) getVideoCount() int {
	if m.filteredVideos != nil {
		return len(m.filteredVideos)
	}
	return len(m.videos)
}

// getVideoIndex converts cursor position to actual video index
func (m *browseModel) getVideoIndex(cursor int) int {
	if m.filteredVideos != nil {
		if cursor < len(m.filteredVideos) {
			return m.filteredVideos[cursor]
		}
		return 0
	}
	return cursor
}

// filterVideos filters videos based on search query
func (m *browseModel) filterVideos() {
	if m.searchQuery == "" {
		m.filteredVideos = nil
		m.cursor = 0
		m.offset = 0
		return
	}

	query := strings.ToLower(m.searchQuery)
	var matches []int
	for i, video := range m.videos {
		if strings.Contains(strings.ToLower(video.Title), query) {
			matches = append(matches, i)
		}
	}

	m.filteredVideos = matches

	// Reset cursor if out of bounds
	if m.cursor >= len(matches) {
		m.cursor = 0
		m.offset = 0
	}
}

// adjustOffset adjusts the scroll offset to keep cursor visible
func (m *browseModel) adjustOffset() {
	visibleLines := m.height - 5 // Reserve space for header/footer

	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+visibleLines {
		m.offset = m.cursor - visibleLines + 1
	}
}

// playVideo plays the currently selected video
func (m *browseModel) playVideo() tea.Cmd {
	videoCount := m.getVideoCount()
	if m.cursor >= videoCount {
		return nil
	}

	videoIdx := m.getVideoIndex(m.cursor)
	video := &m.videos[videoIdx]

	return tea.Sequence(
		tea.ExitAltScreen,
		func() tea.Msg {
			// Check if already watched before playback
			wasWatched := m.watched[video.ID]

			// Play the video (this will block until mpv exits)
			if err := m.player.Play(m.channel.Name, m.channel.Key, video); err != nil {
				if errors.Is(err, player.ErrMembersOnly) {
					return membersOnlyMsg{channelKey: m.channel.Key, channelName: m.channel.Name, video: video}
				}
				return playErrorMsg{err}
			}

			// Check if marked after playback
			watchedAfter, _ := m.cache.ReadWatched(m.channel.Key)
			nowWatched := watchedAfter[video.ID]

			// Update local state
			if nowWatched {
				m.watched[video.ID] = true
			}

			if !wasWatched && nowWatched {
				return videoMarkedMsg{title: video.Title}
			}

			return playCompleteMsg{}
		},
		tea.EnterAltScreen,
	)
}

// render renders the browse UI
func (m *browseModel) render() string {
	titleText := fmt.Sprintf("Browse: %s", m.channel.Name)
	if m.filteredVideos != nil {
		titleText += fmt.Sprintf(" [%d matches]", len(m.filteredVideos))
	}
	s := renderTitleBar(titleText, m.width, m.cfg.ColorTitle)
	s += "\n"

	videoCount := m.getVideoCount()
	visibleLines := m.height - 5
	if m.searchMode {
		visibleLines -= 1 // Reserve space for search input line
	}

	endIdx := m.offset + visibleLines
	if endIdx > videoCount {
		endIdx = videoCount
	}

	// Render visible videos
	for i := m.offset; i < endIdx; i++ {
		videoIdx := m.getVideoIndex(i)
		video := m.videos[videoIdx]
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		// Mark indicator
		mark := "□"
		if m.watched[video.ID] {
			mark = "✓"
		}

		markStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorUnwatched)))
		markStr := markStyle.Render(mark)

		// Date and title colors
		dateColor := m.cfg.ColorDate
		titleColor := m.cfg.ColorDate
		if m.recent[video.ID] {
			dateColor = m.cfg.ColorRecent
			titleColor = m.cfg.ColorRecent
		}

		// Bold if selected
		dateStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(fmt.Sprintf("%d", dateColor))).
			Bold(i == m.cursor)
		titleStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(fmt.Sprintf("%d", titleColor))).
			Bold(i == m.cursor)

		// Divider
		divider := lipgloss.NewStyle().
			Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorDivider))).
			Render(" | ")

		s += fmt.Sprintf("%s%s %4d %s%s%s\n",
			cursor,
			markStr,
			video.Index,
			dateStyle.Render(formatDate(video.Date)),
			divider,
			titleStyle.Render(video.Title))
	}

	// Search bar (if in search mode)
	if m.searchMode {
		s += "\n"
		searchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorHelp)))
		s += searchStyle.Render(fmt.Sprintf("Search: %s_", m.searchQuery)) + "\n"
	} else {
		s += "\n"
	}

	// Status info
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("%d", m.cfg.ColorHelp)))
	var status string
	if m.filteredVideos != nil {
		status = fmt.Sprintf("Showing %d-%d of %d matches (%d total) | %d/%d watched",
			m.offset+1, endIdx, videoCount, len(m.videos),
			len(m.watched), len(m.videos))
	} else {
		status = fmt.Sprintf("Showing %d-%d of %d videos | %d/%d watched",
			m.offset+1, endIdx, len(m.videos),
			len(m.watched), len(m.videos))
	}
	if m.message != "" {
		s += statusStyle.Render(m.message) + "\n"
	} else {
		s += statusStyle.Render(status) + "\n"
	}

	// Help
	help := "/:search  enter:play  m/space:toggle  u:copy-url  j/k:navigate  pgup/pgdn:page  g/G:top/bottom  q/esc/b:back"
	s += statusStyle.Render(help) + "\n"

	return s
}
