package main

import (
	"fmt"
	"github.com/subbass/ytseries/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Quality: %s\n", cfg.Quality)
	fmt.Printf("YtdlFormat: %s\n", cfg.YtdlFormat)
	fmt.Printf("Player: %s\n", cfg.Player)
}
