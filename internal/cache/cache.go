package cache

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Cache manages all TSV file operations
type Cache struct {
	configDir string
	stateDir  string
}

// channelStateDir returns the state directory for a channel key
// v1 compatibility: prefix with ch_ if not already prefixed
func (c *Cache) channelStateDir(channelKey string) string {
	// If key already has prefix (pl_ or ch_), use as-is
	if strings.HasPrefix(channelKey, "pl_") || strings.HasPrefix(channelKey, "ch_") {
		return filepath.Join(c.stateDir, channelKey)
	}
	// Otherwise, prefix with ch_ for v1 compatibility
	return filepath.Join(c.stateDir, "ch_"+channelKey)
}

// New creates a new cache instance
func New(configDir, stateDir string) *Cache {
	return &Cache{
		configDir: configDir,
		stateDir:  stateDir,
	}
}

// ReadAliases reads the aliases.tsv file
func (c *Cache) ReadAliases() (map[string]string, error) {
	path := filepath.Join(c.configDir, "aliases.tsv")
	aliases := make(map[string]string)

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return aliases, nil // Return empty map if file doesn't exist
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) >= 2 {
			aliases[parts[0]] = parts[1]
		}
	}

	return aliases, scanner.Err()
}

// WriteAliases writes the aliases.tsv file
func (c *Cache) WriteAliases(aliases map[string]string) error {
	path := filepath.Join(c.configDir, "aliases.tsv")

	// Ensure directory exists
	if err := os.MkdirAll(c.configDir, 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Sort keys for consistent output
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Fprintf(file, "%s\t%s\n", k, aliases[k])
	}

	return nil
}

// ReadKeys reads the keys.tsv file
func (c *Cache) ReadKeys() (map[string]string, error) {
	path := filepath.Join(c.configDir, "keys.tsv")
	keys := make(map[string]string)

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return keys, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) >= 2 {
			keys[parts[0]] = parts[1]
		}
	}

	return keys, scanner.Err()
}

// WriteKey writes a single key to keys.tsv
func (c *Cache) WriteKey(url, key string) error {
	keys, err := c.ReadKeys()
	if err != nil {
		return err
	}

	keys[url] = key

	path := filepath.Join(c.configDir, "keys.tsv")
	if err := os.MkdirAll(c.configDir, 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	for u, k := range keys {
		fmt.Fprintf(file, "%s\t%s\n", u, k)
	}

	return nil
}

// ReadVideos reads the videos.tsv file for a channel
func (c *Cache) ReadVideos(channelKey string) ([]Video, error) {
	path := filepath.Join(c.channelStateDir(channelKey), "videos.tsv")
	var videos []Video

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return videos, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) >= 4 {
			index, _ := strconv.Atoi(parts[0])
			videos = append(videos, Video{
				Index: index,
				ID:    parts[1],
				Date:  parts[2],
				Title: parts[3],
			})
		}
	}

	return videos, scanner.Err()
}

// WriteVideos writes the videos.tsv file for a channel
func (c *Cache) WriteVideos(channelKey string, videos []Video) error {
	path := filepath.Join(c.channelStateDir(channelKey), "videos.tsv")

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, v := range videos {
		fmt.Fprintf(file, "%d\t%s\t%s\t%s\n", v.Index, v.ID, v.Date, v.Title)
	}

	return nil
}

// ReadWatched reads the watched.txt file for a channel
func (c *Cache) ReadWatched(channelKey string) (map[string]bool, error) {
	path := filepath.Join(c.channelStateDir(channelKey), "watched.txt")
	watched := make(map[string]bool)

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return watched, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			watched[line] = true
		}
	}

	return watched, scanner.Err()
}

// WriteWatched writes the watched.txt file for a channel
func (c *Cache) WriteWatched(channelKey string, watched map[string]bool) error {
	path := filepath.Join(c.channelStateDir(channelKey), "watched.txt")

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Sort for consistent output
	ids := make([]string, 0, len(watched))
	for id := range watched {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		fmt.Fprintln(file, id)
	}

	return nil
}

// MarkWatched adds a video ID to the watched list
func (c *Cache) MarkWatched(channelKey, videoID string) error {
	watched, err := c.ReadWatched(channelKey)
	if err != nil {
		return err
	}

	watched[videoID] = true
	return c.WriteWatched(channelKey, watched)
}

// UnmarkWatched removes a video ID from the watched list
func (c *Cache) UnmarkWatched(channelKey, videoID string) error {
	watched, err := c.ReadWatched(channelKey)
	if err != nil {
		return err
	}

	delete(watched, videoID)
	return c.WriteWatched(channelKey, watched)
}

// ReadRecent reads the recent.tsv file for a channel
func (c *Cache) ReadRecent(channelKey string) (map[string]bool, error) {
	path := filepath.Join(c.channelStateDir(channelKey), "recent.tsv")
	recent := make(map[string]bool)

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return recent, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) >= 1 {
			recent[parts[0]] = true
		}
	}

	return recent, scanner.Err()
}

// WriteRecent writes the recent.tsv file for a channel
func (c *Cache) WriteRecent(channelKey string, recentIDs []string) error {
	path := filepath.Join(c.channelStateDir(channelKey), "recent.tsv")

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, id := range recentIDs {
		fmt.Fprintf(file, "%s\t1\n", id)
	}

	return nil
}

// RebuildRecent rebuilds the recent.tsv file from videos.tsv using the current threshold
func (c *Cache) RebuildRecent(channelKey string, recentHours int) error {
	if recentHours <= 0 {
		// Clear recent cache
		return c.WriteRecent(channelKey, nil)
	}

	// Read existing videos
	videos, err := c.ReadVideos(channelKey)
	if err != nil {
		return err
	}

	// Calculate recent videos using current threshold
	now := time.Now()
	threshold := now.Add(-time.Duration(recentHours) * time.Hour)

	var recentIDs []string
	for _, v := range videos {
		if v.Date == "99999999" {
			continue
		}

		// Parse date YYYYMMDD
		year, err := strconv.Atoi(v.Date[0:4])
		if err != nil {
			continue
		}
		month, err := strconv.Atoi(v.Date[4:6])
		if err != nil {
			continue
		}
		day, err := strconv.Atoi(v.Date[6:8])
		if err != nil {
			continue
		}

		videoDate := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)

		// Include videos at or after threshold
		if !videoDate.Before(threshold) {
			recentIDs = append(recentIDs, v.ID)
		}
	}

	return c.WriteRecent(channelKey, recentIDs)
}

// AddHistory appends an entry to history.tsv
func (c *Cache) AddHistory(entry HistoryEntry) error {
	path := filepath.Join(c.configDir, "history.tsv")

	if err := os.MkdirAll(c.configDir, 0755); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	timestamp := entry.Timestamp.Unix()
	fmt.Fprintf(file, "%d\t%s\t%s\t%s\t%s\n",
		timestamp, entry.Name, entry.VideoID, entry.Date, entry.Title)

	return nil
}

// GetChannelInfo returns information about a channel
func (c *Cache) GetChannelInfo(name, channelKey string, recentHours int) (*Channel, error) {
	videos, err := c.ReadVideos(channelKey)
	if err != nil {
		return nil, err
	}

	watched, err := c.ReadWatched(channelKey)
	if err != nil {
		return nil, err
	}

	recent, err := c.ReadRecent(channelKey)
	if err != nil {
		return nil, err
	}

	ch := &Channel{
		Name: name,
		Key:  channelKey,
	}

	// Find next unwatched and count unwatched
	for i := range videos {
		if !watched[videos[i].ID] {
			ch.UnwatchedCount++
			if ch.NextEpisode == nil {
				ch.NextEpisode = &videos[i]
			}
		}
	}

	// Latest video is last in list (newest)
	if len(videos) > 0 {
		ch.LatestVideo = &videos[len(videos)-1]
	}

	// Check if channel has recent unwatched videos
	ch.HasRecent = false
	for videoID := range recent {
		if !watched[videoID] {
			ch.HasRecent = true
			break
		}
	}

	return ch, nil
}

// GetNextUnwatched returns the next unwatched video for a channel
func (c *Cache) GetNextUnwatched(channelKey string) (*Video, error) {
	videos, err := c.ReadVideos(channelKey)
	if err != nil {
		return nil, err
	}

	watched, err := c.ReadWatched(channelKey)
	if err != nil {
		return nil, err
	}

	for i := range videos {
		if !watched[videos[i].ID] {
			return &videos[i], nil
		}
	}

	return nil, nil // All watched
}

// GetLatestVideo returns the latest (most recent) video for a channel
func (c *Cache) GetLatestVideo(channelKey string) (*Video, error) {
	videos, err := c.ReadVideos(channelKey)
	if err != nil {
		return nil, err
	}

	if len(videos) == 0 {
		return nil, nil
	}

	return &videos[len(videos)-1], nil
}
