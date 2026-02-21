package player

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/subbass/ytseries/internal/cache"
	"github.com/subbass/ytseries/internal/config"
)

// ErrMembersOnly is returned when a video is members-only and cannot be played
// (either no cookies configured or user is not a member of the channel)
var ErrMembersOnly = errors.New("members-only content")

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

// restrictionType categorises why a video requires authentication
type restrictionType int

const (
	restrictionNone       restrictionType = iota
	restrictionAgeGated                   // age-restricted – cookies let YouTube verify age
	restrictionMembersOnly                // members-only – cookies only help if user is a member
)

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
		restriction, _ := p.checkVideoRestriction(url)

		switch restriction {
		case restrictionAgeGated:
			fmt.Fprintf(os.Stderr, "\nAge-restricted video detected, retrying with browser cookies...\n")
			time.Sleep(1 * time.Second)
			err = p.executeMPV(luaScript, url, true)

		case restrictionMembersOnly:
			fmt.Fprintf(os.Stderr, "\nMembers-only video detected, trying with browser cookies...\n")
			time.Sleep(1 * time.Second)
			err = p.executeMPV(luaScript, url, true)
			if err != nil {
				return fmt.Errorf("%w: you must be a channel member with valid browser cookies configured", ErrMembersOnly)
			}
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
	ytdlOpts := "force-ipv4=,playlist-start=1,js-runtimes=node,format-sort=proto,extractor-args=youtube:skip=hls"

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

// checkVideoRestriction checks why a video failed by running a yt-dlp diagnostic
func (p *Player) checkVideoRestriction(url string) (restrictionType, string) {
	args := []string{
		"--simulate",
		"--no-warnings",
		"--js-runtimes=node",
		url,
	}

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		if isMembersOnlyError(outputStr) {
			return restrictionMembersOnly, outputStr
		}
		if isAgeRestrictionError(outputStr) {
			return restrictionAgeGated, outputStr
		}
	}

	return restrictionNone, outputStr
}

// isAgeRestrictionError checks if the output indicates an age-gate or generic login requirement
func isAgeRestrictionError(output string) bool {
	patterns := []string{
		"sign in to confirm your age",
		"age-restricted",
		"age restricted",
		"inappropriate for some users",
		"error code: 152",
		"sign in to confirm",
	}
	lower := strings.ToLower(output)
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// isMembersOnlyError checks if the output indicates members-only content
func isMembersOnlyError(output string) bool {
	patterns := []string{
		"members-only",
		"members only",
		"join this channel",
		"channel membership",
	}
	lower := strings.ToLower(output)
	for _, p := range patterns {
		if strings.Contains(lower, p) {
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
