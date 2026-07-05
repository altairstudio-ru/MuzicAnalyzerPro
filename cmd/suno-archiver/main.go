package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/library"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/web"
)

func main() {
	var basePath string

	rootCmd := &cobra.Command{
		Use:   "suno-archiver",
		Short: "Suno Archiver — archive and manage Suno AI music tracks",
		Long: `Suno Archiver is a CLI tool for archiving Suno AI music tracks.
It downloads tracks with full metadata (prompts, lyrics, tags) to your local library
and provides a web UI for browsing, searching, and organizing them.`,
	}

	rootCmd.PersistentFlags().StringVar(&basePath, "path", "~/.muzicanalyzer", "Path to the library directory")

	authCmd := &cobra.Command{
		Use:   "auth <token>",
		Short: "Save your Suno auth token",
		Long: `Save your Suno Clerk JWT auth token.

To get the token:
1. Open suno.com in your browser
2. Open DevTools (F12) → Application → Local Storage
3. Find the Clerk session token (starts with "ey...")
4. Copy it and run: suno-archiver auth <token>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := library.LoadConfig(basePath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			cfg.Suno.AuthToken = args[0]

			if err := library.SaveConfig(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			fmt.Println("✓ Auth token saved.")
			return nil
		},
	}

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync tracks from Suno to local library",
		Long: `Fetch all tracks from your Suno account, download audio files,
and store everything in the local library.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := library.LoadConfig(basePath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			mgr, err := library.NewManager(cfg)
			if err != nil {
				return fmt.Errorf("init library: %w", err)
			}
			defer mgr.Close()

			fmt.Println("🔄 Syncing tracks from Suno...")
			stats, err := mgr.Sync()
			if err != nil {
				return fmt.Errorf("sync: %w", err)
			}

			fmt.Printf("\n✓ Sync complete!\n")
			fmt.Printf("  Total tracks found: %d\n", stats.TotalTracks)
			fmt.Printf("  New tracks:         %d\n", stats.NewTracks)
			fmt.Printf("  Updated tracks:     %d\n", stats.UpdatedTracks)
			fmt.Printf("  Downloaded:         %d\n", stats.Downloaded)
			if stats.Errors > 0 {
				fmt.Printf("  Errors:             %d\n", stats.Errors)
			}
			return nil
		},
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the web UI server",
		Long: `Start the web interface for browsing and managing your Suno library.
Opens on http://localhost:8080 by default.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := library.LoadConfig(basePath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			mgr, err := library.NewManager(cfg)
			if err != nil {
				return fmt.Errorf("init library: %w", err)
			}
			defer mgr.Close()

			srv, err := web.NewServer(mgr)
			if err != nil {
				return fmt.Errorf("init web server: %w", err)
			}

			port, _ := cmd.Flags().GetString("port")
			fmt.Printf("🌐 Starting web UI on http://localhost%s\n", port)
			return srv.ListenAndServe(port)
		},
	}
	serveCmd.Flags().StringP("port", "p", ":8080", "Port to listen on")

	rootCmd.AddCommand(authCmd, syncCmd, serveCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
