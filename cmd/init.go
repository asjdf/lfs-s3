package cmd

import (
	"os"

	"github.com/asjdf/lfs-s3/cmd/config"
	"github.com/asjdf/lfs-s3/cmd/server"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use: "git-cdn",
}

func init() {
	rootCmd.AddCommand(config.StartCmd)
	rootCmd.AddCommand(server.StartCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
