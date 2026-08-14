package main

import (
	"fmt"
	"os"

	"github.com/ai-code-review/aicr/internal/config"
	"github.com/ai-code-review/aicr/internal/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log, err := logger.New(cfg.LogLevel)
	if err != nil {
		return err
	}
	defer log.Sync()

	app, err := newApp(cfg, log)
	if err != nil {
		return err
	}
	return app.Run()
}
