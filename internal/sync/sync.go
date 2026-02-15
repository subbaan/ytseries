package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/subbass/ytseries/internal/cache"
	"github.com/subbass/ytseries/internal/config"
)

// Syncer handles yt-dlp operations
type Syncer struct {
	cfg   *config.Config
	cache *cache.Cache
}

// New creates a new syncer
func New(cfg *config.Config, c *cache.Cache) *Syncer {
	return &Syncer{
		cfg:   cfg,
		cache: c,
	}
}

// PlaylistEntry represents a video in a playlist
type PlaylistEntry struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// PlaylistInfo represents playlist metadata from yt-dlp
type PlaylistInfo struct {
	ChannelID  string          `json:"channel_id"`
	UploaderID string          `json:"uploader_id"`
	Entries    []PlaylistEntry `json:"entries"`
}

// VideoInfo represents detailed video info from yt-dlp
type VideoInfo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	UploadDate  string `json:"upload_date"`
	ReleaseDate string `json:"release_date"`
}

// NormalizeURL normalizes a YouTube URL
// For channel URLs, ensures they end with /videos (to exclude shorts/streams)
func NormalizeURL(url string) string {
	// Remove trailing slashes
	url = strings.TrimRight(url, "/")

	// Convert old formats to @handle format
	url = strings.Replace(url, "youtube.com/c/", "youtube.com/@", 1)

	// Check if it's a channel URL without /videos or /streams
	// Patterns: youtube.com/@handle, youtube.com/channel/UC..., youtube.com/c/name
	isChannel := strings.Contains(url, "youtube.com/@") ||
		strings.Contains(url, "youtube.com/channel/") ||
		strings.Contains(url, "youtube.com/c/")

	hasVideos := strings.HasSuffix(url, "/videos")
	hasStreams := strings.HasSuffix(url, "/streams")

	// If it's a channel URL without /videos or /streams, add /videos
	if isChannel && !hasVideos && !hasStreams {
		url = url + "/videos"
	}

	return url
}

// GetChannelKey resolves a channel URL to a unique key
func (s *Syncer) GetChannelKey(url string) (string, error) {
	url = NormalizeURL(url)

	// Check cache first
	keys, err := s.cache.ReadKeys()
	if err != nil {
		return "", err
	}

	if key, ok := keys[url]; ok {
		return key, nil
	}

	// Build yt-dlp command
	args := []string{"-J", "--flat-playlist",
		"--extractor-args", "youtube:player_client=android",
		"--js-runtimes=node"}

	// Add cookies if configured
	if s.cfg.Browser != "" && s.cfg.Browser != "none" {
		if s.cfg.Browser == "custom" {
			// Use custom cookies file
			if s.cfg.CookiesFile != "" {
				args = append(args, "--cookies", s.cfg.CookiesFile)
			}
		} else {
			// Use browser cookies
			args = append(args, "--cookies-from-browser", s.cfg.Browser)
		}
	}

	args = append(args, url)

	// Fetch from yt-dlp
	cmd := exec.Command("yt-dlp", args...)

	output, err := cmd.Output()
	if err != nil {
		// Fallback: use URL hash as key
		hash := sha256.Sum256([]byte(url))
		return hex.EncodeToString(hash[:])[:16], nil
	}

	var info PlaylistInfo
	if err := json.Unmarshal(output, &info); err != nil {
		// Fallback: use URL hash as key
		hash := sha256.Sum256([]byte(url))
		return hex.EncodeToString(hash[:])[:16], nil
	}

	// Prefer channel_id, fallback to uploader_id
	key := info.ChannelID
	if key == "" {
		key = info.UploaderID
	}
	if key == "" {
		// Fallback: use URL hash as key
		hash := sha256.Sum256([]byte(url))
		key = hex.EncodeToString(hash[:])[:16]
	}

	// Cache the key
	if err := s.cache.WriteKey(url, key); err != nil {
		return "", err
	}

	return key, nil
}

// SyncChannel syncs a channel's video list
func (s *Syncer) SyncChannel(url, channelKey string, progressFunc func(string)) error {
	url = NormalizeURL(url)

	if progressFunc != nil {
		progressFunc("Fetching video list...")
	}

	// Build yt-dlp command
	args := []string{"-J", "--flat-playlist",
		"--extractor-args", "youtube:player_client=android",
		"--js-runtimes=node"}

	// Add cookies if configured
	if s.cfg.Browser != "" && s.cfg.Browser != "none" {
		if s.cfg.Browser == "custom" {
			// Use custom cookies file
			if s.cfg.CookiesFile != "" {
				args = append(args, "--cookies", s.cfg.CookiesFile)
			}
		} else {
			// Use browser cookies
			args = append(args, "--cookies-from-browser", s.cfg.Browser)
		}
	}

	args = append(args, url)

	// Get playlist entries
	cmd := exec.Command("yt-dlp", args...)

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to fetch playlist: %w", err)
	}

	var info PlaylistInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return fmt.Errorf("failed to parse playlist: %w", err)
	}

	if len(info.Entries) == 0 {
		return fmt.Errorf("no videos found in playlist")
	}

	// Read existing videos and dates
	existingVideos, _ := s.cache.ReadVideos(channelKey)
	existingDates := make(map[string]string)
	for _, v := range existingVideos {
		existingDates[v.ID] = v.Date
	}

	// Build new video list
	videos := make([]cache.Video, 0, len(info.Entries))
	needDates := make([]string, 0)

	for i, entry := range info.Entries {
		date, ok := existingDates[entry.ID]
		if !ok {
			date = "99999999" // Unknown date marker
			needDates = append(needDates, entry.ID)
		}

		videos = append(videos, cache.Video{
			Index: i + 1,
			ID:    entry.ID,
			Date:  date,
			Title: entry.Title,
		})
	}

	// Fetch dates for new videos in batches
	if len(needDates) > 0 {
		if progressFunc != nil {
			progressFunc(fmt.Sprintf("Fetching dates for %d new videos...", len(needDates)))
		}

		dates := s.fetchDatesBatch(needDates, progressFunc)

		// Update videos with fetched dates
		for i := range videos {
			if date, ok := dates[videos[i].ID]; ok {
				videos[i].Date = date
			}
		}
	}

	// Sort videos by date (chronological order)
	sort.Slice(videos, func(i, j int) bool {
		return videos[i].Date < videos[j].Date
	})

	// Reindex after sorting
	for i := range videos {
		videos[i].Index = i + 1
	}

	// Write videos.tsv
	if err := s.cache.WriteVideos(channelKey, videos); err != nil {
		return err
	}

	// Build recent.tsv
	if progressFunc != nil {
		progressFunc("Building recent video cache...")
	}

	recentIDs := s.findRecentVideos(videos, s.cfg.RecentHours)
	if err := s.cache.WriteRecent(channelKey, recentIDs); err != nil {
		return err
	}

	if progressFunc != nil {
		progressFunc("Sync complete!")
	}

	return nil
}

// fetchDatesBatch fetches upload dates for multiple videos in batches
func (s *Syncer) fetchDatesBatch(videoIDs []string, progressFunc func(string)) map[string]string {
	dates := make(map[string]string)
	batchSize := 200

	for i := 0; i < len(videoIDs); i += batchSize {
		end := i + batchSize
		if end > len(videoIDs) {
			end = len(videoIDs)
		}

		batch := videoIDs[i:end]

		if progressFunc != nil {
			progressFunc(fmt.Sprintf("Fetching dates %d-%d of %d...", i+1, end, len(videoIDs)))
		}

		// Build URL list
		urls := make([]string, len(batch))
		for j, id := range batch {
			urls[j] = fmt.Sprintf("https://www.youtube.com/watch?v=%s", id)
		}

		// Fetch batch using --print like v1 does
		args := []string{
			"-q", "--no-warnings",
			"--extractor-args", "youtube:player_client=android",
			"--js-runtimes=node",
			"--ignore-no-formats-error",
			"--skip-download", "--no-playlist",
			"--print", "%(id)s\t%(upload_date)s",
		}

		// Add cookies if configured
		if s.cfg.Browser != "" && s.cfg.Browser != "none" {
			if s.cfg.Browser == "custom" {
				// Use custom cookies file
				if s.cfg.CookiesFile != "" {
					args = append(args, "--cookies", s.cfg.CookiesFile)
				}
			} else {
				// Use browser cookies
				args = append(args, "--cookies-from-browser", s.cfg.Browser)
			}
		}

		args = append(args, urls...)

		cmd := exec.Command("yt-dlp", args...)
		output, err := cmd.Output()
		if err != nil {
			continue
		}

		// Parse tab-delimited output: id<TAB>date
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}

			id := parts[0]
			date := parts[1]

			// Validate date (must be 8 digits)
			if date == "" || date == "NA" || len(date) != 8 {
				date = "99999999"
			}
			dates[id] = date
		}
	}

	return dates
}

// findRecentVideos returns video IDs uploaded within the recent hours threshold
func (s *Syncer) findRecentVideos(videos []cache.Video, recentHours int) []string {
	if recentHours <= 0 {
		return nil
	}

	now := time.Now()
	threshold := now.Add(-time.Duration(recentHours) * time.Hour)

	var recentIDs []string
	for _, v := range videos {
		if v.Date == "99999999" {
			continue
		}

		// Parse date YYYYMMDD
		year, _ := strconv.Atoi(v.Date[0:4])
		month, _ := strconv.Atoi(v.Date[4:6])
		day, _ := strconv.Atoi(v.Date[6:8])

		videoDate := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)

		// Include videos at or after threshold (not just after)
		if !videoDate.Before(threshold) {
			recentIDs = append(recentIDs, v.ID)
		}
	}

	return recentIDs
}
