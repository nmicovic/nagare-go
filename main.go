package main

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/nemke/nagare-go/internal/config"
	"github.com/nemke/nagare-go/internal/hooks"
	"github.com/nemke/nagare-go/internal/log"
	"github.com/nemke/nagare-go/internal/picker"
	"github.com/nemke/nagare-go/internal/setup"
	"github.com/nemke/nagare-go/internal/theme"
)

func main() {
	log.Init()
	defer log.Close()

	// Load theme from config
	cfg, _ := config.Load()
	theme.Set(cfg.Appearance.Theme)
	log.Info("starting nagare-go, theme=%s", cfg.Appearance.Theme)

	rootCmd := &cobra.Command{
		Use:   "nagare-go",
		Short: "tmux session manager for AI coding agents",
	}

	pickCmd := &cobra.Command{
		Use:   "pick",
		Short: "Launch session picker TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := tea.NewProgram(picker.New(), tea.WithAltScreen())
			_, err := p.Run()
			return err
		},
	}

	hookStateCmd := &cobra.Command{
		Use:   "hook-state",
		Short: "Handle Claude Code hook events from stdin",
		Run: func(cmd *cobra.Command, args []string) {
			hooks.Handle()
		},
	}

	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Install hooks to ~/.claude/settings.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setup.Run()
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
