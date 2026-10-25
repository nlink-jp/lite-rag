package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"lite-rag/internal/database"
	"lite-rag/internal/llm"
	"lite-rag/internal/server"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the HTTP API server with embedded Web UI",
	RunE:  runServe,
}

var serveAddr string

func init() {
	serveCmd.Flags().StringVar(&serveAddr, "addr", "", "listen address (host:port); overrides config server.addr")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) (err error) {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Structured JSON logging to stderr.
	// Log level from config controls whether sensitive content (query text) appears.
	// Default "info" omits query content; "debug" includes it.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.Server.LogLevel),
	})))

	// CLI flag overrides config; fall back to config value (default: 127.0.0.1:8080).
	addr := cfg.Server.Addr
	if cmd.Flags().Changed("addr") {
		addr = serveAddr
	}

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer closeDB(db, &err)

	client := llm.New(cfg.API.BaseURL, cfg.API.APIKey, cfg.Models.Embedding, cfg.Models.Chat)

	srv := server.New(db, client, cfg, version)
	if err := srv.Start(addr); err != nil {
		slog.Error("server error", "error", err)
		return err
	}
	return nil
}

// parseLogLevel converts a config string to a slog.Level.
// Unknown values fall back to INFO.
func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
