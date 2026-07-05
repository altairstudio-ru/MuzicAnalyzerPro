package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "suno-archiver",
		Short: "Suno Archiver — archive and analyze Suno tracks",
		Long:  `Suno Archiver is a CLI tool for archiving, analyzing, and managing Suno AI music tracks.`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Suno Archiver v0.1.0")
		},
	}

	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate with Suno",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("not implemented yet")
		},
	}

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync tracks from Suno to local library",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("not implemented yet")
		},
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the web server",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("not implemented yet")
		},
	}

	rootCmd.AddCommand(authCmd, syncCmd, serveCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
