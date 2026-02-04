package cache

import "time"

// Video represents a single video entry
type Video struct {
	Index int
	ID    string
	Date  string // YYYYMMDD format
	Title string
}

// Channel represents a channel with its metadata
type Channel struct {
	Name          string
	URL           string
	Key           string
	NextEpisode   *Video
	LatestVideo   *Video
	UnwatchedCount int
	HasRecent     bool
}

// Config represents the ytseries configuration
type Config struct {
	Player       string
	YtdlFormat   string
	MarkThreshold int
	RecentHours  int
	ConfigDir    string
	StateDir     string
}

// HistoryEntry represents a watch history entry
type HistoryEntry struct {
	Timestamp time.Time
	Name      string
	VideoID   string
	Date      string
	Title     string
}
