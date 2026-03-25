package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/safedoor/ostui/internal/bus"
	"github.com/safedoor/ostui/internal/config"
	"github.com/safedoor/ostui/internal/db"
	"github.com/safedoor/ostui/internal/server"
	"github.com/safedoor/ostui/internal/tui"
)

func main() {
	cfg := config.Load()

	// Set up file logging — never write to stderr once TUI starts.
	logPath := cfg.LogFile
	if logPath == "" {
		dataDir, err := os.UserConfigDir()
		if err != nil {
			dataDir = filepath.Join(os.Getenv("HOME"), ".config")
		}
		logPath = filepath.Join(dataDir, "ostui", "ostui.log")
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open log file %s: %v\n", logPath, err)
		os.Exit(1)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	log.Printf("ostui starting: %s", cfg)
	log.Printf("logging to %s", logPath)

	// Open database.
	database, err := db.Open(cfg.DBFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// Create event bus.
	eventBus := bus.New()

	// Create and start gRPC server.
	srv, err := server.New(cfg, eventBus, database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create server: %v\n", err)
		os.Exit(1)
	}
	go func() {
		if err := srv.Start(); err != nil {
			log.Printf("FATAL grpc server error: %v", err)
			close(eventBus.Done)
		}
	}()
	go srv.RouteNotifications()

	// Handle signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		close(eventBus.Done)
	}()

	// Run TUI (blocks on main goroutine).
	app := tui.New(cfg, eventBus, database)
	if err := app.Run(); err != nil {
		log.Printf("FATAL tui error: %v", err)
		fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
	}

	log.Printf("ostui shutting down")
	srv.Stop()
}
