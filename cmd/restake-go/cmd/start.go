/*
Copyright © 2023 Tessellated <tessellated.io>
*/
package cmd

import (
	"context"
	"fmt"
	"os/user"
	"sort"
	"strings"
	"sync"
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

type RestakeResult struct {
	network string
	txHash  string
	err     error
}

type RestakeResults []*RestakeResult

func (rr RestakeResults) Len() int           { return len(rr) }
func (rr RestakeResults) Swap(i, j int)      { rr[i], rr[j] = rr[j], rr[i] }
func (rr RestakeResults) Less(i, j int) bool { return rr[i].network < rr[j].network }

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

		runInterval := time.Duration(config.RunIntervalHours) * time.Hour
		for {
			var wg sync.WaitGroup
			var results RestakeResults = []*RestakeResult{}

			for idx, restakeManager := range restakeManagers {
				wg.Add(1)

				go func(ctx context.Context, restakeManager *restake.RestakeManager, healthClient *health.HealthCheckClient) {
					defer wg.Done()

					timeoutContext, cancelFunc := context.WithTimeout(ctx, runInterval)
					defer cancelFunc()

					_ = healthClient.Start()
					txHash, err := restakeManager.Restake(timeoutContext)
					if err != nil {
						_ = healthClient.Failed(err)
					} else {
						_ = healthClient.Success("Hooray!")
					}

					result := &RestakeResult{
						network: restakeManager.Network(),
						txHash:  txHash,
						err:     err,
					}
					results = append(results, result)
				}(ctx, restakeManager, healthClients[idx])
			}

			// Print results whenever they all finish
			go func() {
				wg.Wait()
				printResults(results, log)
			}()

			log.Info().Dur("next run in hours", runInterval).Msg("Finished restaking. Sleeping until next round")
			time.Sleep(runInterval)
		}
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	startCmd.Flags().StringVarP(&configFile, "config-file", "c", "~/.restake/config.yml", "A path to the configuration file")
	startCmd.Flags().Float64VarP(&gasMultiplier, "gas-multiplier", "g", 1.2, "The multiplier to use for gas")
}

// TODO: Move to pickaxe here
func expandHomeDir(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}

	usr, err := user.Current()
	if err != nil {
		panic(fmt.Errorf("failed to get user's home directory: %v", err))
	}
	return strings.Replace(path, "~", usr.HomeDir, 1)
}

func printResults(results RestakeResults, log *log.Logger) {
	sort.Sort(results)

	log.Info().Msg("Restake Results:")
	for _, result := range results {
		if result.err == nil {
			log.Info().Str("tx_hash", result.txHash).Msg(fmt.Sprintf("✅ %s: Success", result.network))
		} else {
			log.Error().Err(result.err).Msg(fmt.Sprintf("❌ %s: Failure", result.network))
		}
	}
}
