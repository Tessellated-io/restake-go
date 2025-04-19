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
		logger = logger.With("configuration_directory", configurationDirectory)
		logger.Info("initializing treasurer configuration directory")

		// Create folder if needed
		err := file.CreateDirectoryIfNeeded(configurationDirectory, logger)
		if err != nil {
			logger.Error("error writing file", "error", err.Error())
			return
		}

		// Write Treasurer configuration
		configLoader, err := restake.NewConfigurationLoader(configurationDirectory, logger)
		if err != nil {
			logger.Error("error writing config", "error", err.Error())
			return
		}

		err = configLoader.Initialize()
		if err != nil {
			logger.Error("error writing config", "error", err.Error())
			return
		}

		// Write router's configuration
		err = filerouter.InitializeConfigFile(fileRouterConfigFilename, configurationDirectory, logger)
		if err != nil {
			logger.Error("error writing file", "error", err.Error())
			return
		}
		logger.Info("finished initializing configuration directory for treasurer")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
