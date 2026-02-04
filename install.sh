#!/bin/bash
# ytseries v2 installer

set -e

echo "ytseries v2.0.0 - Installer"
echo "============================"
echo

# Check dependencies
echo "Checking dependencies..."
command -v go >/dev/null 2>&1 || { echo "Error: go is required but not installed."; exit 1; }
command -v yt-dlp >/dev/null 2>&1 || { echo "Error: yt-dlp is required but not installed."; exit 1; }
command -v mpv >/dev/null 2>&1 || command -v vlc >/dev/null 2>&1 || { echo "Error: mpv or vlc is required but not installed."; exit 1; }

echo "✓ All dependencies found"
echo

# Build
echo "Building binaries..."
make build
echo "✓ Build complete"
echo

# Setup instructions
echo "Setup Instructions"
echo "=================="
echo
echo "1. If you have existing ytseries config, it will work automatically"
echo "2. add channels by launching 'ytseries' and using the TUI"
echo "3. Run 'ytseries -sync' to populate cache or press 's' in the TUI"
echo
echo "Optional: Add to crontab for automatic syncing:"
echo "  0 */2 * * * $HOME/bin/ytseries -s >> /tmp/ytseries.log 2>&1"
echo
echo "Done! Run 'ytseries' to get started."
