package player

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/subbass/ytseries/internal/cache"
	"github.com/subbass/ytseries/internal/config"
)

// Player handles video playback
type Player struct {
	cfg   *config.Config
	cache *cache.Cache
}

// New creates a new player
func New(cfg *config.Config, c *cache.Cache) *Player {
	return &Player{
		cfg:   cfg,
		cache: c,
	}
}

// Play plays a video and handles auto-marking
func (p *Player) Play(channelName, channelKey string, video *cache.Video) error {
	videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", video.ID)

	// Add to history
	if err := p.cache.AddHistory(cache.HistoryEntry{
		Timestamp: time.Now(),
		Name:      channelName,
		VideoID:   video.ID,
		Date:      video.Date,
		Title:     video.Title,
	}); err != nil {
		// Non-fatal, just log
		fmt.Fprintf(os.Stderr, "Warning: failed to add history: %v\n", err)
	}

	// Play based on configured player
	if p.cfg.Player == "mpv" {
		return p.playMPV(channelKey, video, videoURL)
	}

	return p.playVLC(channelKey, video, videoURL)
}

// playMPV plays video with mpv and auto-mark support
// Uses try-then-retry: first attempts without cookies, retries with cookies if age-restricted
func (p *Player) playMPV(channelKey string, video *cache.Video, url string) error {
	// Create Lua script for auto-marking
	luaScript, err := p.createMPVLuaScript(channelKey, video.ID)
	if err != nil {
		return err
	}
	defer os.Remove(luaScript)

	// Try without cookies first (avoids YouTube watch history resume position)
	err = p.executeMPV(luaScript, url, false)

	// If playback failed, check if video requires authentication
	if err != nil {
		// Run diagnostic to check why it failed
		isRestricted, _ := p.checkVideoRestriction(url)

		// If restricted, retry with cookies
		if isRestricted {
			fmt.Fprintf(os.Stderr, "\nRestricted video detected, retrying with browser cookies...\n")
			time.Sleep(1 * time.Second) // Brief pause so user sees message
			err = p.executeMPV(luaScript, url, true)
		}
	}

	return err
}

// executeMPV executes mpv with the given parameters
func (p *Player) executeMPV(luaScript, url string, useCookies bool) error {
	// Clear screen before playback for clean display
	fmt.Print("\033[2J\033[H")

	// Build mpv command
	// Note: Don't use android client for playback - it can cause quality issues
	// Only use it for metadata fetching (sync operations)

	// Combine all ytdl-raw-options (including force-ipv4 from mpv.conf)
	ytdlOpts := "force-ipv4=,playlist-start=1"

	// Add cookies if configured and useCookies is true
	if useCookies && p.cfg.Browser != "" && p.cfg.Browser != "none" {
		if p.cfg.Browser == "custom" {
			// Use custom cookies file (via ytdl-raw-options)
			if p.cfg.CookiesFile != "" {
				ytdlOpts += fmt.Sprintf(",cookies=%s", p.cfg.CookiesFile)
			}
		} else {
			// Use browser cookies
			ytdlOpts += fmt.Sprintf(",cookies-from-browser=%s", p.cfg.Browser)
		}
	}

	args := []string{
		fmt.Sprintf("--ytdl-format=%s", p.cfg.YtdlFormat),
		fmt.Sprintf("--ytdl-raw-options=%s", ytdlOpts),
		fmt.Sprintf("--script=%s", luaScript),
		"--no-resume-playback", // Always start from beginning, ignore watch_later
		"--start=0",            // Explicitly start at 0 seconds
	}

	args = append(args, url)

	cmd := exec.Command("mpv", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()

	// Clear screen after playback before returning to TUI
	fmt.Print("\033[2J\033[H")

	return err
}

// checkVideoRestriction checks if a video requires authentication by running yt-dlp diagnostic
func (p *Player) checkVideoRestriction(url string) (bool, string) {
	// Run yt-dlp to check if video is accessible
	args := []string{
		"--simulate",
		"--no-warnings",
		url,
	}

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// If yt-dlp fails, check the error message
	if err != nil {
		return isAgeRestrictionError(outputStr), outputStr
	}

	return false, outputStr
}

// isAgeRestrictionError checks if the error output indicates age restriction or login requirement
func isAgeRestrictionError(stderr string) bool {
	// Patterns for various YouTube restrictions that require authentication
	ageErrorPatterns := []string{
		"Sign in to confirm your age",
		"age-restricted",
		"age restricted",
		"inappropriate for some users",
		"error code: 152",    // Login required
		"sign in to confirm", // Generic sign-in requirement
		"members-only",       // Members-only content
	}

	stderrLower := strings.ToLower(stderr)
	for _, pattern := range ageErrorPatterns {
		if strings.Contains(stderrLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// playVLC plays video with VLC (simple, marks on exit)
func (p *Player) playVLC(channelKey string, video *cache.Video, url string) error {
	// Note: VLC doesn't support yt-dlp cookies directly
	// For age-restricted videos with VLC, users need to use mpv instead
	// VLC doesn't support auto-mark percentage, so we just mark on completion
	args := []string{
		"--play-and-exit",
		url,
	}

	cmd := exec.Command("vlc", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	// Mark as watched after exit
	return p.cache.MarkWatched(channelKey, video.ID)
}

// createMPVLuaScript creates a temporary Lua script for mpv auto-marking
func (p *Player) createMPVLuaScript(channelKey, videoID string) (string, error) {
	tmpDir := os.TempDir()
	scriptPath := filepath.Join(tmpDir, fmt.Sprintf("ytseries-automark-%d.lua", time.Now().UnixNano()))

	// Add ch_ prefix if not already present (v1 compatibility)
	stateKey := channelKey
	if !strings.HasPrefix(channelKey, "pl_") && !strings.HasPrefix(channelKey, "ch_") {
		stateKey = "ch_" + channelKey
	}

	script := fmt.Sprintf(`
-- ytseries auto-mark script
local max_percent = 0
local channel_key = "%s"
local video_id = "%s"
local mark_threshold = %d
local state_dir = "%s"

function update_max_percent()
    local percent = mp.get_property_number("percent-pos", 0)
    if percent > max_percent then
        max_percent = percent
    end
end

function mark_watched()
    if max_percent >= mark_threshold then
        -- Mark video as watched
        local watched_file = state_dir .. "/" .. channel_key .. "/watched.txt"
        local file = io.open(watched_file, "a")
        if file then
            file:write(video_id .. "\n")
            file:close()
        end
    end
end

mp.observe_property("percent-pos", "number", update_max_percent)
mp.register_event("end-file", mark_watched)
`, stateKey, videoID, p.cfg.MarkThreshold, p.cfg.StateDir)

	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		return "", err
	}

	return scriptPath, nil
}
