/*
Copyright © 2023 Tessellated <tessellated.io>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tessellated-io/pickaxe/cosmos/tx"
	"github.com/tessellated-io/restake-go/restake"
	filerouter "github.com/tessellated-io/router/file"
)

var ()

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Restake Service",
	Long:  `Starts the Restake Service with the given configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println()
		fmt.Println("============================================================")
		fmt.Println("Restake Go")
		fmt.Println()
		fmt.Println("A Product Of Tessellated // tessellated.io")
		fmt.Println("============================================================")
		fmt.Println("")

		ctx := cmd.Context()

		// Configuration loader
		configurationLoader, err := restake.NewConfigurationLoader()
		if err != nil {
			logger.Error().Err(err).Msg("unable to create a configuration loader")
			return
		}

		// Create Gas Manager
		gasPriceProvider, err := tx.NewInMemoryGasProvider()
		if err != nil {
			logger.Error().Err(err).Msg("unable to create a gas price provider")
			return
		}
		gasManager, err := tx.NewDefaultGasManager(0.01, gasPriceProvider, logger)
		if err != nil {
			logger.Error().Err(err).Msg("unable to create a gas manager")
			return
		}

		// Create a file based router.
		fileRouterConfig := fmt.Sprintf("%s/%s", configurationDirectory, fileRouterConfigFilename)
		router, err := filerouter.NewRouter(fileRouterConfig)
		if err != nil {
			logger.Error().Err(err).Msg("unable to create a chain router")
			return
		}

		// Bundle up a Restake Manager
		restakeManager, err := restake.NewRestakeManager(configurationLoader, logger, router, gasManager)
		if err != nil {
			logger.Error().Err(err).Msg("unable to create a restake manager")
			return
		}

		// Run!
		restakeManager.Start(ctx)
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
