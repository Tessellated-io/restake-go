/*
Copyright © 2023 Tessellated <tessellated.io>
*/
package cmd

import (
	"github.com/spf13/cobra"
	file "github.com/tessellated-io/pickaxe/config"
	"github.com/tessellated-io/restake-go/restake"
	filerouter "github.com/tessellated-io/router/file"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a configuration directory",
	Run: func(cmd *cobra.Command, args []string) {
		logger.Info().Str("configuration_directory", configurationDirectory).Msg("initializing treasurer configuration directory")

		// Create folder if needed
		err := file.CreateDirectoryIfNeeded(configurationDirectory, logger)
		if err != nil {
			logger.Error().Err(err).Msg("error writing file")
			return
		}

		// Write Treasurer configuration
		configLoader, err := restake.NewConfigurationLoader(configurationDirectory, logger)
		if err != nil {
			logger.Error().Err(err).Msg("error writing config")
			return
		}

		err = configLoader.Initialize()
		if err != nil {
			logger.Error().Err(err).Msg("error writing config")
			return
		}

		// Write router's configuration
		err = filerouter.InitializeConfigFile(fileRouterConfigFilename, configurationDirectory, logger)
		if err != nil {
			logger.Error().Err(err).Msg("error writing file")
			return
		}
		logger.Info().Str("configuration_directory", configurationDirectory).Msg("finished initializing configuration directory for treasurer")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
