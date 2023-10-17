/*
Copyright © 2023 Tessellated <tessellated.io>
*/
package cmd

import (
	"context"
	"fmt"
	"os/user"
	"strings"
	"time"

	os2 "github.com/cometbft/cometbft/libs/os"
	"github.com/spf13/cobra"
	"github.com/tessellated-io/restake-go/codec"
	"github.com/tessellated-io/restake-go/config"
	"github.com/tessellated-io/restake-go/health"
	"github.com/tessellated-io/restake-go/log"
	"github.com/tessellated-io/restake-go/restake"
	"github.com/tessellated-io/restake-go/rpc"
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
		ctx := cmd.Context()

		fmt.Println()
		fmt.Println("============================================================")
		fmt.Println("Go Go... Restake-Go!")
		fmt.Println()
		fmt.Println("A Product Of Tessellated / tessellated.io")
		fmt.Println("============================================================")
		fmt.Println("")

		// Configure a logger
		log := log.NewLogger()

		// Load config
		expandedConfigFile := expandHomeDir(configFile)
		fmt.Printf("expanded: %s (%s)\n", expandedConfigFile, configFile)
		configOk := os2.FileExists(expandedConfigFile)
		if !configOk {
			panic(fmt.Sprintf("Failed to load config file at: %s", configFile))
		}
		log.Info().Str("config file", expandedConfigFile).Msg("Loading config from file")

		// Parse config
		config, err := config.GetRestakeConfig(ctx, expandedConfigFile, log)
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

		// TODO: rename sleep time hours
		runLoopTime := time.Duration(config.SleepTimeHours) * time.Hour
		for {
			// Wait group entered once by each network
			// var restakeSyncGroup sync.WaitGroup

			for idx, restakeManager := range restakeManagers {
				// restakeSyncGroup.Add(1)

				// go func(restakeClient *restake.RestakeManager, healthClient *health.HealthCheckClient) {
				func(ctx context.Context, restakeManager *restake.RestakeManager, healthClient *health.HealthCheckClient) {
					timeoutContext, cancelFunc := context.WithTimeout(ctx, runLoopTime)
					defer cancelFunc()

					// defer restakeSyncGroup.Done()

					// TODO: better message
					healthClient.Start("start")
					err := restakeManager.Restake(timeoutContext)
					if err != nil {
						fmt.Printf("FAILED on network %s: %s\n", restakeManager.Network(), err)

						healthClient.Failed(err.Error())
					} else {
						healthClient.Success("Hooray!")
					}
				}(ctx, restakeManager, healthClients[idx])
			}

			// // Wait for all networks to finish
			// restakeSyncGroup.Wait()

			log.Info().Int("sleep hours", config.SleepTimeHours).Msg("Finished restaking. Sleeping until next round")
			time.Sleep(time.Duration(config.SleepTimeHours) * time.Hour)
		}
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	startCmd.Flags().StringVarP(&configFile, "config-file", "c", "~/.restake/config.yml", "A path to the configuration file")
	startCmd.Flags().Float64VarP(&gasMultiplier, "gas-multiplier", "g", 1.2, "The multiplier to use for gas")
}

func expandHomeDir(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}

	usr, err := user.Current()
	if err != nil {
		panic(fmt.Errorf("Failed to get user's home directory: %v", err))
	}
	return strings.Replace(path, "~", usr.HomeDir, 1)
}
