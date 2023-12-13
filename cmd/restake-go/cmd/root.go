/*
Copyright © 2023 Tessellated <tessellated.io>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tessellated-io/pickaxe/log"
)

const fileRouterConfigFilename = "chains.yaml"

var (
	logger                 *log.Logger
	configurationDirectory string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "restake",
	Short: "Restake-Go implements the Restake protocol.",
	Long: `Restake-Go is an alternative implementation of the Restake protocol by Tessellated.
	
See also: https://github.com/eco-stake/restake.`,
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
	var rawLogLevel string

	rootCmd.PersistentFlags().StringVarP(&configurationDirectory, "config-directory", "c", "~/.restake", "Where to store Restake-Go's configuration")
	rootCmd.PersistentFlags().StringVarP(&rawLogLevel, "log-level", "l", "info", "Logging level")

	// TODO
	fmt.Println(rawLogLevel)

	// Get a logger
	logLevel := log.ParseLogLevel(rawLogLevel)

	fmt.Println(logLevel)
	logger = log.NewLogger(logLevel)
	// logger = logger.ApplyPrefix(" restake")
}
