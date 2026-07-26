package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is set at link time via -ldflags "-X main.version=<tag>".
var version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "lite-rag %s\n", version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)

	// Every tool in the org answers `--version`, and the shared homebrew
	// formula template tests for it — without the flag `brew test` fails on
	// "unknown flag". Setting Version here makes cobra add the flag. The
	// `version` subcommand stays for compatibility, and the template matches
	// its output exactly (cobra's default would print "lite-rag version X").
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("lite-rag {{.Version}}\n")
}
