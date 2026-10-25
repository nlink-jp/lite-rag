package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"lite-rag/internal/database"
	"lite-rag/internal/indexer"
	"lite-rag/internal/llm"
)

var (
	indexDir  string
	indexFile string
)

var indexCmd = &cobra.Command{
	Use:   "index (--dir <directory> | --file <file>)",
	Short: "Index Markdown documents",
	Long: `Index Markdown documents into the database.

  --dir  <directory>  Recursively index all *.md files under a directory.
  --file <file>       Index a single file (any extension).

Exactly one of --dir or --file must be specified.`,
	Args: cobra.NoArgs,
	RunE: runIndex,
}

func init() {
	indexCmd.Flags().StringVar(&indexDir, "dir", "", "directory to index recursively")
	indexCmd.Flags().StringVar(&indexFile, "file", "", "single file to index")
	rootCmd.AddCommand(indexCmd)
}

func runIndex(cmd *cobra.Command, _ []string) (err error) {
	if indexDir == "" && indexFile == "" {
		return fmt.Errorf("one of --dir or --file is required")
	}
	if indexDir != "" && indexFile != "" {
		return fmt.Errorf("--dir and --file are mutually exclusive")
	}

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer closeDB(db, &err)

	client := llm.New(cfg.API.BaseURL, cfg.API.APIKey, cfg.Models.Embedding, cfg.Models.Chat)
	idx := indexer.New(db, client, cfg.Models.Embedding, cfg.Retrieval)

	if indexFile != "" {
		if _, err := os.Stat(indexFile); err != nil {
			return fmt.Errorf("file %s: %w", indexFile, err)
		}
		fmt.Fprintf(os.Stderr, "Indexing %s...\n", indexFile)
		if err := idx.IndexFile(context.Background(), indexFile); err != nil {
			return err
		}
	} else {
		if _, err := os.Stat(indexDir); err != nil {
			return fmt.Errorf("directory %s: %w", indexDir, err)
		}
		fmt.Fprintf(os.Stderr, "Indexing %s...\n", indexDir)
		if err := idx.IndexDir(context.Background(), indexDir); err != nil {
			return err
		}
	}

	fmt.Fprintln(os.Stderr, "Done.")
	return nil
}
