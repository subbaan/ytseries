# Changelog

All notable changes to ytseries will be documented in this file.

## [2.2.0] - 2025-11-18

### Added
- **Browse/Mark mode** (press `b` on any channel)
  - View all videos in chronological order
  - Toggle mark/unmark with `m` or `space`
  - Auto-advance to next video after marking
  - Hold key down to quickly mark multiple videos
  - Shows ✓ for watched, □ for unwatched
  - Recent videos highlighted in cyan
  - Navigate with j/k, g/G for top/bottom
  - Exit with q/esc/p

## [2.1.1] - 2025-11-18

### Fixed
- **CRITICAL:** URL normalization was removing `/videos` suffix instead of adding it
  - This caused syncs to fetch ALL content (videos + shorts + streams) instead of just videos
  - Now correctly ensures channel URLs end with `/videos` to exclude shorts
  - Re-sync affected channels to get correct video counts

## [2.1.0] - 2025-11-18

### Added
- Settings menu (press `c`) with left/right navigation
- Customizable UI colors (6 configurable color indices for wpgtk compatibility)
- Video marked feedback: Shows "✓ Marked as watched: [title]" when auto-mark triggers
- Color preview mode in settings (press `p`)

### Changed
- Video quality format now uses `bestvideo[height<=?N]+bestaudio/best` for better DASH/HLS support
- Removed "Playback complete" message (silent return to TUI)
- Extended divider line (4 chars longer than help text)
- Inset title and help bar by 2 chars for cleaner layout
- Fixed-width columns: 10 chars for channel name, "nnnn unwatched" format

### Fixed
- v1 cache compatibility: Added `ch_` prefix for state directories
- Video quality issues: Removed incorrect android client args from mpv playback
- Alt-screen handling: Clean suspend/resume when launching mpv
- Screen clearing before/after playback to prevent visual noise
- Hanging on early mpv exit (Ctrl+C, 'q')

## [2.0.0] - 2025-11-18

### Added
- Complete rewrite in Go using Bubble Tea TUI framework
- Direct key bindings (n/l/s/S/c/q) - no nested fzf menus
- Instant startup - all operations use cached data
- Background sync via `-s` flag (for cron)
- Auto-mark at 90% with mpv Lua script
- Recent video highlighting (configurable hours)
- Chronological playback + latest video support
- Watch history tracking

### Changed
- Architecture: Single-file bash → Go modules with clean separation
- UI: fzf-based → Bubble Tea TUI with vim-style navigation
- Sync: Runtime → Background (cron-based)
- Config format: Added color customization fields
- Quality setting: Simplified to 480/720/1080/1440/best options

### Maintained
- 100% v1 cache compatibility (TSV format)
- Same directory structure: `~/.config/ytseries`, `~/.local/state/ytseries`
- mpv auto-mark functionality
- Watch state tracking
- Quality caps

---

## Version History

- **v0.83.0** - bash implementation (v1)
- **v2.0.0** - Go + Bubble Tea rewrite
- **v2.1.0** - Settings menu + polish
