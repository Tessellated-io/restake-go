/*
Copyright © 2023 Tessellated <tessellated.io>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tessellated-io/pickaxe/config"
	"github.com/tessellated-io/restake-go/restake"
	filerouter "github.com/tessellated-io/router/file"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a configuration directory",
	Run: func(cmd *cobra.Command, args []string) {
		logger.Info().Str("configuration_directory", configurationDirectory).Msg("initializing restake-go configuration")

		// Create folder if needed
		config.CreateDirectoryIfNeeded(configurationDirectory, logger)

		// Write Restake configuration
		restakeConfigFile := fmt.Sprintf("%s/%s", configurationDirectory, restake.RestakeConfigFilename)
		header := "This is the configuration file for Restake"
		restakeConfig := restake.Configuration{}
		err := config.WriteYamlWithComments(restakeConfig, header, restakeConfigFile, logger)
		if err != nil {
			logger.Error().Err(err).Msg("error writing file")
			return
		}

		// Write router's configuration
		err = filerouter.InitializeConfigFile(fileRouterConfigFilename, configurationDirectory, logger)
		if err != nil {
			logger.Error().Err(err).Msg("error writing file")
			return
		}
		logger.Info().Str("configuration_directory", configurationDirectory).Msg("finished initializing configuration for restake-go")

	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
