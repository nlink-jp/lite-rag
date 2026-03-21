package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"lite-rag/internal/config"
	"lite-rag/internal/database"
)

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Manage indexed documents",
}

var docsListJSON bool

var docsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all indexed documents",
	Args:  cobra.NoArgs,
	RunE:  runDocsList,
}

var docsDeleteCmd = &cobra.Command{
	Use:   "delete <document-id>",
	Short: "Delete a document and all its chunks from the database",
	Args:  cobra.ExactArgs(1),
	RunE:  runDocsDelete,
}

var docsShowCmd = &cobra.Command{
	Use:   "show <document-id>",
	Short: "Reconstruct and print the stored text of a document",
	Args:  cobra.ExactArgs(1),
	RunE:  runDocsShow,
}

func init() {
	docsListCmd.Flags().BoolVar(&docsListJSON, "json", false, "output as JSON")
	docsCmd.AddCommand(docsListCmd, docsDeleteCmd, docsShowCmd)
	rootCmd.AddCommand(docsCmd)
}

func openDB() (*database.DB, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return database.Open(cfg.Database.Path)
}

func runDocsList(_ *cobra.Command, _ []string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	docs, err := db.ListDocuments()
	if err != nil {
		return err
	}

	if docsListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		type jsonDoc struct {
			ID             string `json:"id"`
			FilePath       string `json:"file_path"`
			FileHash       string `json:"file_hash"`
			TotalChunks    int    `json:"total_chunks"`
			IndexedAt      string `json:"indexed_at"`
			EmbeddingModel string `json:"embedding_model"`
		}
		out := make([]jsonDoc, len(docs))
		for i, d := range docs {
			out[i] = jsonDoc{d.ID, d.FilePath, d.FileHash, d.TotalChunks, d.IndexedAt, d.EmbeddingModel}
		}
		return enc.Encode(out)
	}

	if len(docs) == 0 {
		fmt.Println("No documents indexed.")
		return nil
	}

	// ID is a SHA-256 hex (64 chars).
	fmt.Printf("%-64s  %-6s  %-19s  %s\n", "ID", "CHUNKS", "INDEXED AT", "FILE PATH")
	fmt.Println(strings.Repeat("-", 120))
	for _, d := range docs {
		fmt.Printf("%-64s  %-6d  %-19s  %s\n",
			d.ID, d.TotalChunks, d.IndexedAt, d.FilePath)
	}
	fmt.Printf("\n%d document(s)\n", len(docs))
	return nil
}

func runDocsDelete(_ *cobra.Command, args []string) error {
	id := args[0]
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.DeleteDocument(id); err != nil {
		return err
	}
	fmt.Printf("Deleted document %s\n", id)
	return nil
}

func runDocsShow(_ *cobra.Command, args []string) error {
	id := args[0]
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	chunks, err := db.DocumentChunks(id)
	if err != nil {
		return err
	}

	var lastHeading string
	for _, c := range chunks {
		// Print heading path when it changes.
		if c.HeadingPath != "" && c.HeadingPath != lastHeading {
			fmt.Printf("\n<!-- %s -->\n\n", c.HeadingPath)
			lastHeading = c.HeadingPath
		}
		fmt.Println(c.Content)
	}
	return nil
}
