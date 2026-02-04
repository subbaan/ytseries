package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config represents the ytseries configuration
type Config struct {
	Player        string
	Quality       string // v1 format: "1080", "720", etc.
	YtdlFormat    string // Computed from Quality
	MarkThreshold int
	RecentHours   int
	Browser       string // Browser to use for cookies (for age-restricted videos)
	CookiesFile   string // Path to custom cookies.txt file (when browser="custom")
	ConfigDir     string
	StateDir      string
	// UI Colors (color indices 0-255)
	ColorTitle       int
	ColorSelected    int
	ColorChannelName int
	ColorUnwatched   int
	ColorRecent      int
	ColorDate        int
	ColorDivider     int
	ColorHelp        int
}

// Load reads configuration from file and environment
func Load() (*Config, error) {
	// Get XDG directories
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	configDir = filepath.Join(configDir, "ytseries")

	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		stateDir = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	stateDir = filepath.Join(stateDir, "ytseries")

	// Default config
	cfg := &Config{
		Player:        "mpv",
		Quality:       "1080",
		YtdlFormat:    "",
		MarkThreshold: 90,
		RecentHours:   48,
		Browser:       "vivaldi",
		ConfigDir:     configDir,
		StateDir:      stateDir,
		// Default UI colors
		ColorTitle:       12, // Bright blue
		ColorSelected:    15, // Bright white
		ColorChannelName: 7,  // Light gray
		ColorUnwatched:   11, // Bright yellow
		ColorRecent:      14, // Bright cyan
		ColorDate:        8,  // Dark gray
		ColorDivider:     8,  // Dark gray
		ColorHelp:        8,  // Dark gray
	}

	// Read config file
	configFile := filepath.Join(configDir, "config")
	file, err := os.Open(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Config file doesn't exist, use defaults
			return cfg, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "player":
			cfg.Player = value
		case "quality":
			cfg.Quality = value
		case "ytdl_format":
			cfg.YtdlFormat = value
		case "mark_threshold":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.MarkThreshold = v
			}
		case "recent_hours":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.RecentHours = v
			}
		case "browser":
			cfg.Browser = value
		case "cookies_file":
			cfg.CookiesFile = value
		case "color_title":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.ColorTitle = v
			}
		case "color_selected":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.ColorSelected = v
			}
		case "color_channel_name":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.ColorChannelName = v
			}
		case "color_unwatched":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.ColorUnwatched = v
			}
		case "color_recent":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.ColorRecent = v
			}
		case "color_date":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.ColorDate = v
			}
		case "color_divider":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.ColorDivider = v
			}
		case "color_help":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.ColorHelp = v
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Compute ytdl_format from quality if not set
	if cfg.YtdlFormat == "" {
		cfg.YtdlFormat = cfg.computeYtdlFormat()
	}

	// Ensure directories exist
	if err := os.MkdirAll(cfg.ConfigDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.StateDir, 0755); err != nil {
		return nil, err
	}

	return cfg, nil
}

// computeYtdlFormat converts quality setting to ytdl format string
func (c *Config) computeYtdlFormat() string {
	if c.Quality == "best" {
		return "bestvideo[height<=?1080]+bestaudio/best"
	}
	// Use bestvideo+bestaudio format for better quality
	// This works with DASH/HLS streams which are more common now
	return fmt.Sprintf("bestvideo[height<=?%s]+bestaudio/best", c.Quality)
}

// ComputeYtdlFormat is the public version for external callers
func (c *Config) ComputeYtdlFormat() string {
	return c.computeYtdlFormat()
}

// Save writes the configuration to file
func (c *Config) Save() error {
	configFile := filepath.Join(c.ConfigDir, "config")

	if err := os.MkdirAll(c.ConfigDir, 0755); err != nil {
		return err
	}

	file, err := os.Create(configFile)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Fprintf(file, "player=%s\n", c.Player)
	fmt.Fprintf(file, "quality=%s\n", c.Quality)
	fmt.Fprintf(file, "mark_threshold=%d\n", c.MarkThreshold)
	fmt.Fprintf(file, "recent_hours=%d\n", c.RecentHours)
	fmt.Fprintf(file, "browser=%s\n", c.Browser)
	if c.CookiesFile != "" {
		fmt.Fprintf(file, "cookies_file=%s\n", c.CookiesFile)
	}
	fmt.Fprintf(file, "color_title=%d\n", c.ColorTitle)
	fmt.Fprintf(file, "color_selected=%d\n", c.ColorSelected)
	fmt.Fprintf(file, "color_channel_name=%d\n", c.ColorChannelName)
	fmt.Fprintf(file, "color_unwatched=%d\n", c.ColorUnwatched)
	fmt.Fprintf(file, "color_recent=%d\n", c.ColorRecent)
	fmt.Fprintf(file, "color_date=%d\n", c.ColorDate)
	fmt.Fprintf(file, "color_divider=%d\n", c.ColorDivider)
	fmt.Fprintf(file, "color_help=%d\n", c.ColorHelp)

	return nil
}

// QualityOptions returns available quality options
func QualityOptions() []string {
	return []string{"480", "720", "1080", "1440", "best"}
}

// PlayerOptions returns available player options
func PlayerOptions() []string {
	return []string{"mpv", "vlc"}
}

// BrowserOptions returns available browser options for cookies
func BrowserOptions() []string {
	return []string{"vivaldi", "chrome", "firefox", "brave", "edge", "safari", "opera", "custom", "none"}
}
