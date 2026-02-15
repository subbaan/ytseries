# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ytseries is a TUI application for watching YouTube channels/playlists chronologically with watch tracking. Built in Go using the Bubble Tea framework, it provides instant startup by caching video metadata locally and syncing via yt-dlp.


## Build & Development Commands

```bash
# Build the main binary
make build

# Install to ~/bin
make install

# Run tests
make test

# Format code
make fmt

# Vet code
make vet

# Clean build artifacts
rm -rf bin/

# Run directly without installing
go run ./cmd/ytseries

# Test with a specific channel (if configured)
./bin/ytseries
```

## Version Updates

When updating the version, it must be changed in **3 locations**:
1. `Makefile` line 3: `VERSION := X.Y.Z`
2. `cmd/ytseries/main.go` line 16: `const version = "X.Y.Z"`
3. `internal/tui/model.go` line 17: `const version = "X.Y.Z"`

## Architecture

### Core Components

The application follows a clean architecture with distinct layers:

**Main Entry Point:**
- `cmd/ytseries/main.go` - TUI application with flag parsing (-s/--sync for sync-only mode)

**Internal Packages:**

1. **config** (`internal/config/config.go`)
   - XDG-compliant directory handling (`~/.config/ytseries`, `~/.local/state/ytseries`)
   - Key-value config file parsing (`key=value` format)
   - Supports both v1 `quality` format and explicit `ytdl_format` strings
   - Browser cookie support (vivaldi, chrome, firefox, brave, edge, safari, opera, custom, none)
   - Custom cookies file support (Netscape format) when `browser=custom` - editable in settings TUI (v2.3.2+)
   - Try-then-retry cookie usage (v2.4.0): cookies only used when video requires authentication
   - UI color configuration (256-color palette indices 0-255)

2. **cache** (`internal/cache/`)
   - TSV file operations for all persistent data
   - V1 compatibility: auto-prefixes channel keys with `ch_` if not already prefixed
   - Files managed:
     - `aliases.tsv`: name → URL mapping
     - `keys.tsv`: normalized URL → channel_key mapping
     - `videos.tsv`: index, video_id, date (YYYYMMDD), title
     - `watched.txt`: newline-separated video IDs
     - `recent.tsv`: recent video IDs (within recent_hours threshold)
     - `history.tsv`: unix_timestamp, name, video_id, date, title

3. **sync** (`internal/sync/sync.go`)
   - yt-dlp wrapper with `--extractor-args youtube:player_client=android` for metadata
   - Uses `--cookies-from-browser` flag if browser config is set (not "none")
   - URL normalization ensures channel URLs end with `/videos` (excludes shorts/streams)
   - Batch date fetching (200 videos at a time) for new videos
   - Channel key resolution via yt-dlp JSON output (channel_id or uploader_id)
   - Falls back to URL hash if yt-dlp fails
   - Recent video detection based on `recent_hours` config

4. **player** (`internal/player/player.go`)
   - mpv integration with Lua script for auto-marking at threshold (default 90%)
   - Try-then-retry with diagnostic approach (v2.4.0):
     - First attempt without cookies (avoids YouTube resume position)
     - On failure: runs `yt-dlp --simulate` diagnostic
     - Retries with cookies only if age-restricted/login-required detected
   - Uses `--ytdl-raw-options=cookies-from-browser=<browser>` only when needed
   - VLC fallback (marks on completion only, no cookie support)
   - **Critical**: Does NOT use `player_client=android` for playback (quality issues)
   - Generates temporary Lua script per playback session
   - Screen clearing before/after playback for clean TUI transitions
   - No output capture during playback (allows proper mpv operation)

5. **tui** (`internal/tui/`)
   - Bubble Tea model implementation
   - Three modes: main list, browse (per-channel episodes), manage (add/delete channels)
   - Channel sorting: recent unwatched → unwatched → all watched (alphabetical within each group)
   - Settings mode for changing player/quality/colors/cookies
   - Text input for cookies file path when browser=custom (v2.3.2+)
   - Real-time config changes (recent_hours rebuilds cache automatically)

### Data Flow

**Sync Operation:**
1. Fetch playlist with `yt-dlp -J --flat-playlist` (gets video IDs, titles)
2. Compare with existing videos.tsv to find new videos
3. Batch fetch upload dates for new videos only
4. Sort all videos chronologically by date
5. Build recent.tsv based on recent_hours threshold
6. Write videos.tsv (reindexed after sorting)

**Playback Flow:**
1. User selects action (n=next, l=latest)
2. Retrieve video from cache
3. Create mpv Lua script with channel key, video ID, threshold
4. Launch mpv WITHOUT cookies (avoids YouTube resume position, full terminal access)
5. If playback fails:
   a. Run `yt-dlp --simulate` diagnostic to check restriction status
   b. If age-restricted/login-required detected, retry WITH cookies
6. Lua script observes percent-pos property during playback
7. On end-file event, append to watched.txt if threshold reached
8. Return to TUI, reload channels to reflect new state

**Channel Loading:**
1. Read aliases.tsv and keys.tsv
2. For each channel, rebuild recent.tsv if recent_hours changed
3. Load videos.tsv, watched.txt, recent.tsv
4. Calculate unwatched count and find next unwatched episode
5. Sort channels by category (recent/unwatched/watched) then alphabetically

### TSV File Format Details

All data files use tab-separated values for simplicity and awk-compatibility:

- **videos.tsv**: Chronologically sorted (oldest first), reindexed after each sync
- **watched.txt**: Appended by mpv Lua script, deduplicated on read
- **recent.tsv**: Format is `video_id\t1` (historical, second column unused)
- **dates**: YYYYMMDD format, "99999999" for unknown/unavailable dates

### Key Design Decisions

1. **Android client for metadata only**: Sync uses `player_client=android` for reliable metadata, but playback uses default client for better quality
2. **Browser cookies for age-restricted videos**: Uses `--cookies-from-browser` flag when browser is configured (default: vivaldi). Set to "none" to disable
3. **Custom cookies file support**: When `browser=custom`, uses a custom Netscape-format cookies.txt file specified in `cookies_file` config (v2.3.2+)
4. **Try-then-retry with diagnostic approach** (v2.4.0):
   - Always attempt playback WITHOUT cookies first (avoids YouTube resume position)
   - mpv gets full terminal access (no output capture) for proper operation
   - On failure: run `yt-dlp --simulate` to diagnose the cause
   - Only retry WITH cookies if restriction detected (age/login/members-only)
   - ~2-3 second delay only for restricted videos, immediate start for normal videos
5. **Lazy date fetching**: Only fetch upload dates for new videos, cache all dates persistently
6. **Auto-rebuild recent cache**: When recent_hours config changes, recent.tsv is rebuilt on next load
7. **V1 compatibility**: Channel keys auto-prefixed with `ch_` to work with existing v1 cache

## Known Issues & Solutions

### yt-dlp JavaScript Runtime Requirement (2026-02)

**Issue**: yt-dlp requires a JavaScript runtime (Node.js or Deno) for proper YouTube extraction. Without one, it falls back to the "android vr player API" which provides throttled/inferior stream URLs, causing severe buffering during playback.

**Solution**: ytseries v2.5.4+ passes `--js-runtimes=node` to all yt-dlp invocations (sync and playback). Users must have Node.js (or Deno) installed on their system.

**Affected locations**:
- `internal/player/player.go`: `executeMPV()` via ytdl-raw-options, `checkVideoRestriction()` via direct arg
- `internal/sync/sync.go`: `GetChannelKey()`, `SyncChannel()`, `fetchDatesBatch()` via direct arg

### YouTube Watch History Resume Position (2026-01)

**Issue**: When using browser cookies for authentication, videos would often start 1-2 minutes (or more) into playback instead of at 0 seconds. This affected ALL videos when cookies were enabled, not just age-restricted ones.

**Root Cause**: When authenticated via browser cookies, YouTube's watch history resume feature embeds the last-watched position into playback, overriding mpv's `--start=0` and `--no-resume-playback` flags.

**Attempted Solutions**:
1. **Added `&t=0s` to YouTube URL** (`internal/player/player.go`)
   - Result: ❌ Did not work - YouTube still resumed from watch history position
   - The URL parameter was insufficient to override the authenticated session's resume data

2. **Pre-fetched direct stream URLs** with `yt-dlp -g`
   - Approach: Get direct stream URL(s) first, pass to mpv to bypass YouTube watch page entirely
   - Result: ❌ Broke mpv functionality - timebar showed as fully filled, seeking became impossible
   - Direct stream URLs lack proper duration/seeking metadata

**Working Solution**: **Try-then-retry with diagnostic approach** (implemented in v2.3.1, refined in v2.4.0)
- First attempt: Play WITHOUT cookies (no resume position issue)
- On playback failure: Run `yt-dlp --simulate` diagnostic to check why it failed
- Detection: Pattern matching on yt-dlp output for restriction indicators
- If restricted: Automatically retry WITH cookies
- Result: ✅ 95%+ of videos start at 0 seconds, only age-restricted videos may have resume position issue

**Why This Approach**:
- mpv needs full terminal access during playback (can't capture output without breaking functionality)
- When playback fails, we run a quick diagnostic with yt-dlp to determine the cause
- Only adds ~2-3 second delay for restricted videos that need authentication
- Non-restricted videos play immediately with no overhead

**Detection Patterns** (case-insensitive):
- `"Sign in to confirm your age"` - Explicit age verification
- `"age-restricted"` / `"age restricted"` - Community Guidelines restrictions
- `"inappropriate for some users"` - Age restriction variant
- `"error code: 152"` - Generic login requirement (Error 152-4)
- `"sign in to confirm"` - Generic sign-in requirement
- `"members-only"` - Members-only content

**Implementation Details**:
- `playMPV()`: Main function with try-retry logic
- `executeMPV()`: Executes mpv with/without cookies, no output capture (allows normal mpv operation)
- `checkVideoRestriction()`: Runs `yt-dlp --simulate` to diagnose failure, returns restriction status
- `isAgeRestrictionError()`: Pattern matching for various YouTube restrictions
- User sees: "Restricted video detected, retrying with browser cookies..." on retry (1 second pause)

This approach balances functionality (age-restricted videos still play) with user experience (most videos start at correct position) and reliability (proper error detection via yt-dlp diagnostics).

## Cookie Configuration

### Browser Cookie Support (v2.4.0)

ytseries uses a **try-then-retry approach** for cookies to minimize YouTube watch history resume position issues:

**How It Works**:
1. Videos are **always attempted first WITHOUT cookies** (avoids resume position issue)
2. If playback fails, yt-dlp diagnostic checks for authentication requirements
3. **Only if age-restricted/login-required**, retry WITH browser cookies
4. Result: Most videos start at 0 seconds, minimal delay for restricted content

**Supported Browsers** (via `--cookies-from-browser`):
- vivaldi (default)
- chrome
- firefox
- brave
- edge
- safari
- opera
- custom (see below)
- none (disable cookie support)

**Configuration**: Set `browser=<name>` in `~/.config/ytseries/config` or use settings TUI (press `c`, navigate to Browser)

### Using Custom Cookies File (v2.3.2+)

For browsers not in the built-in list, or for more control over cookie handling:

1. **Set browser to custom** in settings (press `c` in main menu, navigate to Browser, cycle to "custom")
2. **Set cookies file path** in settings:
   - Navigate to "Cookies File Path"
   - Press Enter or `e` to edit
   - Type the full path to your cookies.txt file (e.g., `/home/user/.config/ytseries/cookies.txt`)
   - Press Enter to save, or Esc to cancel
3. **Cookie file format**: Must be Netscape format (also called cookies.txt format)

**Exporting cookies**:
- Browser extensions: "Get cookies.txt LOCALLY", "EditThisCookie", etc.
- Command-line tools: Many cookie export utilities available
- Format requirements: Plain text, tab-separated, Netscape/cookies.txt format

**Use cases**:
- Unsupported browsers (Opera GX, Librewolf, Waterfox, etc.)
- Browser installed via Flatpak/Snap (isolation issues with `--cookies-from-browser`)
- Portable cookies (same file across machines)
- Privacy-focused (dedicated profile/temporary cookies)
- Manual cookie management

**Note**: Custom cookies are used in the same try-then-retry approach (first attempt without, retry with if needed).

## Common Patterns

### Adding a new config option

1. Add field to `config.Config` struct
2. Add parsing in `config.Load()` switch statement
3. Add writing in `config.Save()`
4. Update settings TUI if user-modifiable (`internal/tui/settings.go`)

### Adding a new TUI key binding

1. Add case in `tui.handleKeyPress()` switch
2. Create command function (returns `tea.Cmd`)
3. Handle result message in `tui.Update()`
4. Update help text in `tui.View()`

### Working with cache files

Always use cache package methods, never write files directly:
- Use `cache.ReadX()` / `cache.WriteX()` for atomic operations
- Use `cache.GetChannelInfo()` for consolidated channel data
- Use `cache.MarkWatched()` for adding to watched list

## Testing

The codebase currently has minimal test coverage. When adding tests:
- Use `go test ./...` to run all tests
- Mock external commands (yt-dlp, mpv) for unit tests
- Use `test_config.go` for test configuration setup

## Dependencies

- **Bubble Tea**: TUI framework (github.com/charmbracelet/bubbletea)
- **Bubbles**: TUI components like spinner (github.com/charmbracelet/bubbles)
- **Lipgloss**: Styling and colors (github.com/charmbracelet/lipgloss)
- **yt-dlp**: External binary for YouTube metadata and streaming
- **mpv/vlc**: External video players

Run `go mod tidy` after adding new dependencies.
