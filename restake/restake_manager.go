package restake

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/tessellated-io/healthchecks/health"
	chainregistry "github.com/tessellated-io/pickaxe/cosmos/chain-registry"
	"github.com/tessellated-io/pickaxe/cosmos/rpc"
	"github.com/tessellated-io/pickaxe/cosmos/tx"
	"github.com/tessellated-io/pickaxe/crypto"
	"github.com/tessellated-io/pickaxe/log"
	routertypes "github.com/tessellated-io/router/types"

	"cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txauth "github.com/cosmos/cosmos-sdk/x/auth/tx"
)

var (
	wg      sync.WaitGroup
	results RestakeResults = []*RestakeResult{}
)

// A restake client runs restake clients based on configurations in the Restake registry.
type RestakeManager struct {
	version string

	// Core services
	cdc         *codec.ProtoCodec
	chainRouter routertypes.Router
	gasManager  tx.GasManager
	logger      *log.Logger

	// Restake services
	configurationLoader *configurationLoader

	debug_runSync bool
}

func NewRestakeManager(
	version string,

	configurationLoader *configurationLoader,
	logger *log.Logger,

	// NOTE: Chain router is injected from cmd so that we can hot swap if needed.
	chainRouter routertypes.Router,

	// If `true`, run restake synchronously across networks to aid in debugging.
	gasManager tx.GasManager,

	debug_runSync bool,
) (*RestakeManager, error) {
	cdc := getCodec()

	restakeManager := &RestakeManager{
		version: version,

		cdc:         cdc,
		chainRouter: chainRouter,
		gasManager:  gasManager,
		logger:      logger,

		configurationLoader: configurationLoader,

		debug_runSync: debug_runSync,
	}

	return restakeManager, nil
}

func (rm *RestakeManager) Start(mainContext context.Context) {
	rm.logger.Debug().Msg("starting restake")

	// Loop indefinitely
	for {
		rm.logger.Debug().Msg("starting core restake loop")

		// Load localConfiguration from disk so we can configure things like run time and retries
		localConfiguration, err := rm.configurationLoader.LoadConfiguration()
		if err != nil {
			rm.logger.Error().Err(err).Msg("failed to load configuration from file")
			continue
		}
		rm.logger.Debug().Msg("reloaded config from file")

		// Fork the passed context to account for timeout
		now := time.Now()
		nextRunTime := now.Add(localConfiguration.RunInterval())
		rm.logger.Debug().Time("deadline", nextRunTime).Msg("calculated deadline for restake run")

		// NOTE: This context is explicitly canceled at the end of the for loop. We can't use `defer` because this
		//			 is an infinite loop.
		deadlineContext, cancelDeadlineFunc := context.WithDeadline(mainContext, nextRunTime)

		// Retryable chain registry client
		rawChainClient := chainregistry.NewChainRegistryClient(rm.logger)
		chainRegistryClient, err := chainregistry.NewRetryableChainRegistryClient(
			localConfiguration.NetworkRetryAttempts,
			localConfiguration.NetworkRetryDelay(),
			rawChainClient,
			rm.logger,
		)
		if err != nil {
			rm.logger.Error().Err(err).Msg("unable to create a chain registry client")
			continue
		}
		rm.logger.Debug().Msg("created chain registry client")

		// Hotswap client in gas manager
		rm.gasManager.SetChainRegistryClient(chainRegistryClient)

		// Load Restake Configuration from registry
		registryConfiguration, err := chainRegistryClient.Validator(deadlineContext, localConfiguration.TargetValidator)
		if err != nil {
			rm.logger.Error().Err(err).Msg("failed to load configuration from Restake's validator registry")
			continue
		}
		rm.logger.Debug().Int("registry_count", len(registryConfiguration.Chains)).Msg("loaded restake registry data")

		// Filter networks marked to ignore
		ignoreChainNames := localConfiguration.Ignores
		restakeChains := []chainregistry.RestakeInfo{}
		for _, chain := range registryConfiguration.Chains {
			shouldIgnore := false
			for _, ignore := range ignoreChainNames {
				if strings.EqualFold(chain.Name, ignore) {
					rm.logger.Info().Str("chain_id", chain.Name).Msg("ignoring chain due to configuration file")
					shouldIgnore = true
				}
			}

			if !shouldIgnore {
				restakeChains = append(restakeChains, chain)
			}
		}
		rm.logger.Debug().Int("chain_count", len(restakeChains)).Msg("determined chains to restake")

		// Run for each network
		results = []*RestakeResult{}
		for _, restakeChain := range restakeChains {
			wg.Add(1)

			// Print results whenever they all finish
			// Do this async so that the time.sleep() below tracks time correctly
			if rm.debug_runSync {
				rm.logger.Debug().Msg("running restake synchronously for debugging")
				rm.runRestakeForNetwork(
					deadlineContext,
					localConfiguration,
					restakeChain,
					chainRegistryClient,
				)
				rm.logger.Debug().Msg("synchronously running runRestakeForNetwork has finished")

			} else {
				go rm.runRestakeForNetwork(
					deadlineContext,
					localConfiguration,
					restakeChain,
					chainRegistryClient,
				)

			}

		}

		// Wait until all jobs time out, or finish
		rm.logger.Debug().Msg("all jobs kicked off")
		wg.Wait()
		rm.logger.Debug().Msg("all jobs finished")

		// Print results whenever they all finish
		// Do this async so that the time.sleep() below tracks time correctly
		if rm.debug_runSync {
			rm.logger.Debug().Msg("waiting and printing results synchronously for debugging")
			rm.waitForAndPrintResults()
		} else {
			go rm.waitForAndPrintResults()
		}

		// Sleep until the next run
		timeToNextRun := time.Until(nextRunTime)
		rm.logger.Debug().Time("next_run_time", nextRunTime).Str("time_to_next_run", timeToNextRun.String()).Msg("waiting for next run")
		time.Sleep(timeToNextRun)

		// Cancel the context
		cancelDeadlineFunc()
	}
}

// NOTE: This thread should be run from a goroutine, unless in debug mode.
func (rm *RestakeManager) waitForAndPrintResults() {
	wg.Wait()
	printResults(results, rm.logger)
}

// NOTE: This thread should be run from a goroutine, unless in debug mode.
func (rm *RestakeManager) runRestakeForNetwork(
	ctx context.Context,
	localConfiguration *Configuration,
	restakeChain chainregistry.RestakeInfo,
	chainRegistryClient chainregistry.ChainRegistryClient,
) {
	// Results from the run
	var txHashes []string
	var err error

	defer func() {
		rm.logger.Debug().Str("chain_id", restakeChain.Name).Msg("restake finished")
		if err != nil {
			rm.logger.Error().Err(err).Str("chain_id", restakeChain.Name).Msg("restake failed with error")
		}

		// Add results
		result := &RestakeResult{
			network:  restakeChain.Name,
			txHashes: txHashes,
			err:      err,
		}
		results = append(results, result)

		// Send health check if enabled
		if !strings.EqualFold(localConfiguration.HealthChecksPingKey, "") {
			healthClient := health.NewHealthClient(rm.logger, localConfiguration.HealthChecksPingKey, true)

			if err == nil {
				err = healthClient.SendSuccess(restakeChain.Name)
			} else {
				err = healthClient.SendFailure(restakeChain.Name)
			}
		} else {
			rm.logger.Info().Str("chain_id", restakeChain.Name).Msg("not sending healthchecks.io pings as they are disabled in config.")
		}

		// Leave wait group
		defer wg.Done()
	}()
	prefixedLogger := rm.logger.ApplyPrefix(fmt.Sprintf(" [%s]", restakeChain.Name))

	// Get the chain info
	var chainInfo *chainregistry.ChainInfo
	chainInfo, err = chainRegistryClient.ChainInfo(ctx, restakeChain.Name)
	if err != nil {
		return
	}

	// Derive info needed about the chain
	chainID := chainInfo.ChainID

	var stakingDenom string
	stakingDenom, err = chainInfo.StakingDenom()
	if err != nil {
		prefixedLogger.Error().Err(err).Str("chain_id", chainID).Msg("failed to get staking denom")
		return
	}

	var minimumRequiredReward math.LegacyDec
	minimumRequiredReward, err = math.LegacyNewDecFromStr(restakeChain.Restake.MinimumReward.String())
	if err != nil {
		prefixedLogger.Error().Err(err).Str("chain_id", chainID).Str("minimum_reward", restakeChain.Restake.MinimumReward.String()).Msg("failed to parse minimum reward")
		return
	}

	// Derive info needed about Restake
	validatorAddress := restakeChain.Address
	botAddress := restakeChain.Restake.Address

	// Get an endpoint from the Router
	var grpcEndpoint string
	grpcEndpoint, err = rm.chainRouter.GrpcEndpoint(chainInfo.ChainID)
	if err != nil {
		return
	}

	// Bind up an RPC Client with some retries
	var rawRpcClient rpc.RpcClient
	rawRpcClient, err = rpc.NewGrpcClient(grpcEndpoint, cdc, prefixedLogger)
	if err != nil {
		return
	}

	var rpcClient rpc.RpcClient
	rpcClient, err = rpc.NewRetryableRpcClient(
		localConfiguration.NetworkRetryAttempts,
		localConfiguration.NetworkRetryDelay(),
		rawRpcClient,
		prefixedLogger,
	)
	if err != nil {
		return
	}

	// Set minimum bot balance to be 1 fee token.
	var minimumRequiredBotBalance *sdk.Coin
	minimumRequiredBotBalance, err = chainInfo.OneFeeToken(ctx, rpcClient)
	if err != nil {
		return
	}

	// Create a Grant manager
	var grantManager *grantManager
	grantManager, err = NewGrantManager(restakeChain.Restake.Address, chainID, prefixedLogger, rpcClient, stakingDenom, validatorAddress)
	if err != nil {
		return
	}

	// Create a tx provider
	var feeDenom string
	feeDenom, err = chainInfo.FeeDenom()
	if err != nil {
		return
	}

	slip44 := uint(chainInfo.Slip44)
	var signer crypto.BytesSigner
	signer, err = tx.GetSoftSigner(slip44, localConfiguration.BotMnemonic)
	if err != nil {
		return
	}

	txConfig := txauth.NewTxConfig(cdc, txauth.DefaultSignModes)

	var simulationManager tx.SimulationManager
	simulationManager, err = tx.NewSimulationManager(localConfiguration.GasFactor, rpcClient, txConfig)
	if err != nil {
		return
	}

	var txProvider tx.TxProvider
	txProvider, err = tx.NewTxProvider(signer, chainID, feeDenom, localConfiguration.VersionedMemo(rm.version), prefixedLogger, simulationManager, txConfig)
	if err != nil {
		return
	}

	// Create a signing metadata provider
	var signingMetadataProvider *tx.SigningMetadataProvider
	signingMetadataProvider, err = tx.NewSigningMetadataProvider(chainID, rpcClient)
	if err != nil {
		return
	}

	var restakeClient *restakeClient
	restakeClient, err = NewRestakeClient(
		chainInfo.Bech32Prefix,
		chainID,
		restakeChain.Name,
		feeDenom,
		stakingDenom,
		localConfiguration.BatchSize,
		validatorAddress,
		botAddress,
		minimumRequiredReward,
		minimumRequiredBotBalance,
		localConfiguration.TxPollDelay(),
		localConfiguration.TxPollAttempts,
		localConfiguration.NetworkRetryDelay(),
		localConfiguration.NetworkRetryAttempts,
		rm.gasManager,
		grantManager,
		txProvider,
		prefixedLogger,
		rpcClient,
		signingMetadataProvider,
		signer,
	)
	if err != nil {
		return
	}

	txHashes, err = restakeClient.restake(ctx)
}

// Results of running Restake on a given network
type RestakeResult struct {
	network  string
	txHashes []string
	err      error
}

type RestakeResults []*RestakeResult

func (rr RestakeResults) Len() int           { return len(rr) }
func (rr RestakeResults) Swap(i, j int)      { rr[i], rr[j] = rr[j], rr[i] }
func (rr RestakeResults) Less(i, j int) bool { return rr[i].network < rr[j].network }

func printResults(results RestakeResults, log *log.Logger) {
	sort.Sort(results)

	log.Info().Msg("Restake Results:")
	for _, result := range results {
		if result.err == nil {
			txHashes := zerolog.Arr()
			for _, txHash := range result.txHashes {
				txHashes.Str(txHash)
			}

			msg := fmt.Sprintf("✅ %s: Success", result.network)
			if len(result.txHashes) == 0 {
				msg = fmt.Sprintf("%s | 😬 No transactions sent. The restake website may mark you as offline.", msg)
			}

			log.Info().Array("tx_hashes", txHashes).Msg(msg)
		} else {
			log.Error().Err(result.err).Msg(fmt.Sprintf("❌ %s: Failure", result.network))
		}
	}
}
