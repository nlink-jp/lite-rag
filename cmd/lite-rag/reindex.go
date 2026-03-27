package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"lite-rag/internal/database"
	"lite-rag/internal/indexer"
	"lite-rag/internal/llm"
)

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Re-embed documents indexed with a different embedding model",
	Long: `Re-embed all documents whose stored embedding model differs from the
current config. Run this after changing models.embedding in the config.

Chunk text is read from the database — source files do not need to exist on
disk. This means documents remain searchable even if their source files have
been moved or deleted.`,
	Args: cobra.NoArgs,
	RunE: runReindex,
}

func init() {
	rootCmd.AddCommand(reindexCmd)
}

func runReindex(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Check how many stale documents exist before starting.
	stale, err := db.ListStaleDocuments(cfg.Models.Embedding)
	if err != nil {
		return fmt.Errorf("list stale documents: %w", err)
	}
	if len(stale) == 0 {
		fmt.Fprintln(os.Stderr, "All documents are already indexed with the current embedding model.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Found %d document(s) with stale embeddings. Re-indexing with model %q...\n",
		len(stale), cfg.Models.Embedding)

	client := llm.New(cfg.API.BaseURL, cfg.API.APIKey, cfg.Models.Embedding, cfg.Models.Chat)
	idx := indexer.New(db, client, cfg.Models.Embedding, cfg.Retrieval)

	stats, err := idx.Reindex(cmd.Context())
	if err != nil {
		return fmt.Errorf("reindex: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Done. Re-indexed: %d", stats.Reindexed)
	if stats.Errors > 0 {
		fmt.Fprintf(os.Stderr, ", Errors: %d", stats.Errors)
	}
	fmt.Fprintln(os.Stderr)
	return nil
}
