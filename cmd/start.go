/*
Copyright © 2023 Tessellated <tessellated.io>
*/
package cmd

import (
	"context"
	"fmt"
	"sync"
	"time"

	os2 "github.com/cometbft/cometbft/libs/os"
	"github.com/restake-go/codec"
	"github.com/restake-go/config"
	"github.com/restake-go/health"
	"github.com/restake-go/log"
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
		fmt.Println("============================================================")
		fmt.Println("Go Go... Restake-Go!")
		fmt.Println()
		fmt.Println("A Product Of Tessellatd / tessellated.io")
		fmt.Println("============================================================")
		fmt.Println("")

		// Configure a logger
		log := log.NewLogger()

		// Load config
		configOk := os2.FileExists(configFile)
		if !configOk {
			panic(fmt.Sprintf("Failed to load config file at: %s", configFile))
		}
		log.Info().Str("config file", configFile).Msg("Loading config from file")

		// Parse config
		config, err := config.GetRestakeConfig(configFile, log)
		if err != nil {
			panic(err)
		}

		cdc := codec.GetCodec()

		// Make restake clients
		restakeManagers := []*restake.RestakeManager{}
		healthClients := []*health.HealthCheckClient{}
		for _, chain := range config.Chains {
			prefixedLogger := log.ApplyPrefix(fmt.Sprintf(" [%s]", chain.Network))

			rpcClient, err := rpc.NewRpcClient(chain.NodeGrpcURI, cdc, prefixedLogger)
			if err != nil {
				panic(err)
			}

			healthcheckId := chain.HealthcheckId
			if healthcheckId == "" {
				panic(fmt.Sprintf("No health check id found for network %s", chain.Network))
			}
			healthClient := health.NewHealthCheckClient(chain.Network, healthcheckId, prefixedLogger)
			healthClients = append(healthClients, healthClient)

			restakeManager, err := restake.NewRestakeManager(rpcClient, cdc, config.Mnemonic, config.Memo, gasMultiplier, *chain, prefixedLogger)
			if err != nil {
				panic(err)
			}
			restakeManagers = append(restakeManagers, restakeManager)
		}

		// TODO: Use a context with a deadline.
		for {
			// Wait group entered once by each network
			var restakeSyncGroup sync.WaitGroup

			for idx, restakeClient := range restakeManagers {
				restakeSyncGroup.Add(1)

				go func(restakeClient *restake.RestakeManager, healthClient *health.HealthCheckClient) {
					defer restakeSyncGroup.Done()

					// TODO: better message
					healthClient.Start("start")
					err := restakeClient.Restake(context.Background())
					if err != nil {
						healthClient.Failed(err.Error())
					} else {
						healthClient.Success("Hooray!")
					}
				}(restakeClient, healthClients[idx])
			}

			// Wait for all networks to finish
			restakeSyncGroup.Wait()

			log.Info().Int("sleep hours", config.SleepTimeHours).Msg("Finished restaking. Sleeping until next round")
			time.Sleep(time.Duration(config.SleepTimeHours) * time.Hour)
		}
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	startCmd.Flags().StringVarP(&configFile, "configFile", "c", "~/.restake/config.yml", "A path to the configuration file")
	startCmd.Flags().Float64VarP(&gasMultiplier, "gasMultipler", "g", 1.2, "The multiplier to use for gas")
}
