package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "nagare-go",
		Short: "tmux session manager for AI coding agents",
	}

	pickCmd := &cobra.Command{
		Use:   "pick",
		Short: "Launch session picker TUI",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("picker: not implemented yet")
		},
	}

	hookStateCmd := &cobra.Command{
		Use:   "hook-state",
		Short: "Handle Claude Code hook events from stdin",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("hook-state: not implemented yet")
		},
	}

	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Install hooks to ~/.claude/settings.json",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("setup: not implemented yet")
		},
	}

	rootCmd.AddCommand(pickCmd, hookStateCmd, setupCmd)

	// Default to "pick" when no subcommand given
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return pickCmd.RunE(pickCmd, args)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
