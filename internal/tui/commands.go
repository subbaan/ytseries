package tui

import (
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/subbass/ytseries/internal/cache"
	"github.com/subbass/ytseries/internal/player"
	"github.com/subbass/ytseries/internal/sync"
)

// playNext plays the next unwatched episode for the selected channel
func (m *Model) playNext() tea.Cmd {
	if m.cursor >= len(m.channels) {
		return nil
	}

	ch := m.channels[m.cursor]
	m.message = "Loading video..."

	return tea.Sequence(
		tea.ExitAltScreen,
		func() tea.Msg {
			video, err := m.cache.GetNextUnwatched(ch.Key)
			if err != nil {
				return playErrorMsg{err}
			}

			if video == nil {
				return playErrorMsg{fmt.Errorf("no unwatched episodes")}
			}

			// Check if already watched before playback
			watchedBefore, _ := m.cache.ReadWatched(ch.Key)
			wasWatched := watchedBefore[video.ID]

			// Play the video (this will block until mpv exits)
			if err := m.player.Play(ch.Name, ch.Key, video); err != nil {
				if errors.Is(err, player.ErrMembersOnly) {
					return membersOnlyMsg{channelKey: ch.Key, channelName: ch.Name, video: video}
				}
				return playErrorMsg{err}
			}

			// Check if marked after playback
			watchedAfter, _ := m.cache.ReadWatched(ch.Key)
			nowWatched := watchedAfter[video.ID]

			if !wasWatched && nowWatched {
				return videoMarkedMsg{title: video.Title}
			}

			return playCompleteMsg{}
		},
		tea.EnterAltScreen,
	)
}

// playLatest plays the latest (most recent) episode for the selected channel
func (m *Model) playLatest() tea.Cmd {
	if m.cursor >= len(m.channels) {
		return nil
	}

	ch := m.channels[m.cursor]
	m.message = "Loading video..."

	return tea.Sequence(
		tea.ExitAltScreen,
		func() tea.Msg {
			video, err := m.cache.GetLatestVideo(ch.Key)
			if err != nil {
				return playErrorMsg{err}
			}

			if video == nil {
				return playErrorMsg{fmt.Errorf("no videos found")}
			}

			// Check if already watched before playback
			watchedBefore, _ := m.cache.ReadWatched(ch.Key)
			wasWatched := watchedBefore[video.ID]

			// Play the video (this will block until mpv exits)
			if err := m.player.Play(ch.Name, ch.Key, video); err != nil {
				if errors.Is(err, player.ErrMembersOnly) {
					return membersOnlyMsg{channelKey: ch.Key, channelName: ch.Name, video: video}
				}
				return playErrorMsg{err}
			}

			// Check if marked after playback
			watchedAfter, _ := m.cache.ReadWatched(ch.Key)
			nowWatched := watchedAfter[video.ID]

			if !wasWatched && nowWatched {
				return videoMarkedMsg{title: video.Title}
			}

			return playCompleteMsg{}
		},
		tea.EnterAltScreen,
	)
}

// syncChannel syncs the selected channel
func (m *Model) syncChannel() tea.Cmd {
	if m.cursor >= len(m.channels) {
		return nil
	}

	ch := m.channels[m.cursor]
	m.loading = true
	m.message = "Syncing channel..."

	return func() tea.Msg {
		progressFunc := func(msg string) {
			m.message = msg
		}

		if err := m.syncer.SyncChannel(ch.URL, ch.Key, progressFunc); err != nil {
			return syncErrorMsg{err}
		}

		return syncCompleteMsg{}
	}
}

// syncAllChannels syncs all channels
func (m *Model) syncAllChannels() tea.Cmd {
	m.loading = true
	m.message = "Syncing all channels..."

	return func() tea.Msg {
		for i, ch := range m.channels {
			progressFunc := func(msg string) {
				m.message = msg
			}

			progressFunc("Syncing " + ch.Name + " (" + string(rune(i+1)) + "/" + string(rune(len(m.channels))) + ")...")

			if err := m.syncer.SyncChannel(ch.URL, ch.Key, progressFunc); err != nil {
				// Continue on error
				continue
			}
		}

		return syncCompleteMsg{}
	}
}

// addChannel adds a new channel
func (m *Model) addChannel(name, url string) tea.Cmd {
	m.loading = true
	m.message = "Adding channel..."

	return func() tea.Msg {
		// Get channel key
		key, err := m.syncer.GetChannelKey(url)
		if err != nil {
			return syncErrorMsg{err}
		}

		// Add to aliases
		aliases, err := m.cache.ReadAliases()
		if err != nil {
			return syncErrorMsg{err}
		}

		aliases[name] = url
		if err := m.cache.WriteAliases(aliases); err != nil {
			return syncErrorMsg{err}
		}

		// Sync channel
		progressFunc := func(msg string) {
			m.message = msg
		}

		if err := m.syncer.SyncChannel(url, key, progressFunc); err != nil {
			return syncErrorMsg{err}
		}

		return syncCompleteMsg{}
	}
}

// deleteChannel removes a channel
func (m *Model) deleteChannel() tea.Cmd {
	if m.cursor >= len(m.channels) {
		return nil
	}

	ch := m.channels[m.cursor]

	return func() tea.Msg {
		// Remove from aliases
		aliases, err := m.cache.ReadAliases()
		if err != nil {
			return syncErrorMsg{err}
		}

		delete(aliases, ch.Name)
		if err := m.cache.WriteAliases(aliases); err != nil {
			return syncErrorMsg{err}
		}

		// Note: We don't delete the cache data, just remove from aliases
		// User can manually clean up state directory if desired

		return syncCompleteMsg{}
	}
}

// markVideo marks a video as watched
func (m *Model) markVideo(channelKey string, video *cache.Video) error {
	return m.cache.MarkWatched(channelKey, video.ID)
}

// unmarkVideo unmarks a video as watched
func (m *Model) unmarkVideo(channelKey string, video *cache.Video) error {
	return m.cache.UnmarkWatched(channelKey, video.ID)
}

// addChannelFunc adds a channel with URL normalization
func (m *Model) addChannelFunc(name, url string) tea.Cmd {
	// Normalize URL to ensure /videos suffix
	url = sync.NormalizeURL(url)
	return m.addChannel(name, url)
}

// deleteChannelFunc deletes the currently selected channel
func (m *Model) deleteChannelFunc() tea.Cmd {
	return m.deleteChannel()
}
