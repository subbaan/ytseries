package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/subbass/ytseries/internal/cache"
	"github.com/subbass/ytseries/internal/config"
	"github.com/subbass/ytseries/internal/player"
	"github.com/subbass/ytseries/internal/sync"
	"github.com/subbass/ytseries/internal/tui"
)

const version = "2.5.3"

func main() {
	// Parse flags
	versionFlag := flag.Bool("v", false, "print version and exit")
	flag.BoolVar(versionFlag, "version", false, "print version and exit")
	syncFlag := flag.Bool("s", false, "sync all channels and exit")
	flag.BoolVar(syncFlag, "sync", false, "sync all channels and exit")
	flag.Parse()

	// Handle version flag
	if *versionFlag {
		fmt.Printf("ytseries %s\n", version)
		return
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize cache
	c := cache.New(cfg.ConfigDir, cfg.StateDir)

	// Initialize syncer
	s := sync.New(cfg, c)

	// If sync flag is set, sync all channels and exit
	if *syncFlag {
		syncAll(c, s)
		return
	}

	// Initialize player
	p := player.New(cfg, c)

	// Create TUI model
	model, err := tui.New(cfg, c, s, p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing TUI: %v\n", err)
		os.Exit(1)
	}

	// Run TUI
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func syncAll(c *cache.Cache, s *sync.Syncer) {
	// Read aliases
	aliases, err := c.ReadAliases()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading aliases: %v\n", err)
		os.Exit(1)
	}

	if len(aliases) == 0 {
		fmt.Println("No channels configured")
		return
	}

	// Read keys
	keys, err := c.ReadKeys()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading keys: %v\n", err)
		os.Exit(1)
	}

	// Sync each channel
	successCount := 0
	errorCount := 0

	for name, url := range aliases {
		fmt.Printf("Syncing %s...\n", name)

		// Get or create channel key
		key, ok := keys[sync.NormalizeURL(url)]
		if !ok {
			// Resolve key
			key, err = s.GetChannelKey(url)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Error resolving key: %v\n", err)
				errorCount++
				continue
			}
		}

		// Sync channel
		progressFunc := func(msg string) {
			fmt.Printf("  %s\n", msg)
		}

		if err := s.SyncChannel(url, key, progressFunc); err != nil {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			errorCount++
		} else {
			successCount++
		}
	}

	fmt.Printf("\nSync complete: %d succeeded, %d failed\n", successCount, errorCount)

	if errorCount > 0 {
		os.Exit(1)
	}
}
