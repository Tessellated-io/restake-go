/*
Copyright © 2023 Tessellated <tessellated.io>
*/
package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/cometbft/cometbft/libs/os"
	"github.com/restake-go/codec"
	"github.com/restake-go/config"
	"github.com/restake-go/health"
	"github.com/restake-go/restake"
	"github.com/restake-go/rpc"
	"github.com/spf13/cobra"
)

var (
	configFile    string
	gasMultiplier float64
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Restake Service",
	Long:  `Starts the Restake Service with the given configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println()
		fmt.Println("====================")
		fmt.Println("Go Go... Restake-Go!")
		fmt.Println()
		fmt.Println("A Product Of Tessellatd / tessellated.io")
		fmt.Println("====================")
		fmt.Println("")

		configOk := os.FileExists(configFile)
		if !configOk {
			panic(fmt.Sprintf("Failed to load config file at: %s", configFile))
		}
		fmt.Printf("Loading config from file: %s\n", configFile)

		// Parse config
		config, err := config.GetRestakeConfig(configFile)
		if err != nil {
			panic(err)
		}

		cdc := codec.GetCodec()

		// Make restake clients
		restakeManagers := []*restake.RestakeManager{}
		for _, chain := range config.Chains {
			rpcClient, err := rpc.NewRpcClient(chain.NodeGrpcURI, cdc)
			if err != nil {
				panic(err)
			}

			healthcheckId := chain.HealthcheckId
			if healthcheckId == "" {
				panic(fmt.Sprintf("No health check id found for network %s", chain.Network))
			}
			healthClient := health.NewHealthCheckClient(chain.Network, healthcheckId)

			restakeManager, err := restake.NewRestakeManager(rpcClient, cdc, config.Mnemonic, config.Memo, gasMultiplier, *chain, healthClient)
			if err != nil {
				panic(err)
			}
			restakeManagers = append(restakeManagers, restakeManager)
		}

		for {
			for _, restakeClient := range restakeManagers {
				fmt.Printf("\n✨ Starting Restake on %s\n", restakeClient.Network())
				restakeClient.Restake(context.Background())
			}
			var sleepTimeHours uint64 = 3
			fmt.Printf("Finished restaking. Will start the next round in %d hours\n", sleepTimeHours)
			time.Sleep(3 * time.Hour)
		}
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	startCmd.Flags().StringVarP(&configFile, "configFile", "c", "~/.restake/config.yml", "A path to the configuration file")
	startCmd.Flags().Float64VarP(&gasMultiplier, "gasMultipler", "g", 1.2, "The multiplier to use for gas")
}
