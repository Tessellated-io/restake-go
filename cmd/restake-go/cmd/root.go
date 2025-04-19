/*
Copyright © 2023 Tessellated <tessellated.io>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/tessellated-io/pickaxe/config"
	"github.com/tessellated-io/pickaxe/log"
)

const fileRouterConfigFilename = "chains.yml"

var (
	rawLogLevel string
	logger      *log.Logger

	configurationDirectory string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "restake",
	Short: "Restake-Go implements the Restake protocol.",
	Long: `Restake-Go is an alternative implementation of the Restake protocol by Tessellated.
	
See also: https://github.com/eco-stake/restake.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Get a logger
		logger = log.NewLogger(rawLogLevel)

		configurationDirectory = config.ExpandHomeDir(configurationDirectory)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configurationDirectory, "config-directory", "c", "~/.restake", "Where to store Restake's configuration")
	rootCmd.PersistentFlags().StringVarP(&rawLogLevel, "log-level", "l", "info", "Logging level")
}
