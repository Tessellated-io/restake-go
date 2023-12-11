package restake

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"cosmossdk.io/math"
	"github.com/tessellated-io/healthchecks/health"
	"github.com/tessellated-io/pickaxe/cosmos/tx"

	"github.com/cosmos/cosmos-sdk/codec"
	txauth "github.com/cosmos/cosmos-sdk/x/auth/tx"
	chainregistry "github.com/tessellated-io/pickaxe/cosmos/chain-registry"
	"github.com/tessellated-io/pickaxe/cosmos/rpc"
	"github.com/tessellated-io/pickaxe/log"
	routertypes "github.com/tessellated-io/router/types"
)

var wg sync.WaitGroup
var results RestakeResults = []*RestakeResult{}

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
}

func NewRestakeManager(
	version string,

	configurationLoader *configurationLoader,
	logger *log.Logger,

	// NOTE: Chain router is injected from cmd so that we can hot swap if needed.
	chainRouter routertypes.Router,

	gasManager tx.GasManager,
) (*RestakeManager, error) {
	cdc := getCodec()

	restakeManager := &RestakeManager{
		version: version,

		cdc:         cdc,
		chainRouter: chainRouter,
		gasManager:  gasManager,
		logger:      logger,

		configurationLoader: configurationLoader,
	}

	return restakeManager, nil
}

func (rm *RestakeManager) Start(ctx context.Context) {
	firstRunCompleted := false

	// Loop indefinitely
	for {
		// Load localConfiguration from disk so we can configure things like run time and retries
		localConfiguration, err := rm.configurationLoader.LoadConfiguration()
		if err != nil {
			rm.logger.Error().Err(err).Msg("failed to load configuration from file")
			continue
		}

		if firstRunCompleted {
			time.Sleep(localConfiguration.RunEvery())
		} else {
			firstRunCompleted = true
		}

		// Retryable chain registry client
		rawChainClient := chainregistry.NewChainRegistryClient(rm.logger)
		chainRegistryClient, err := chainregistry.NewRetryableChainRegistryClient(localConfiguration.NetworkRetryAttempts, localConfiguration.NetworkRetryDelay(), rawChainClient)
		if err != nil {
			panic(fmt.Sprintf("unable to create a chain registry client: %s", err))
		}

		// Load Restake Configuration from registry
		registryConfiguration, err := chainRegistryClient.Validator(ctx, localConfiguration.TargetValidator)
		if err != nil {
			rm.logger.Error().Err(err).Msg("failed to load configuration from Restake's validator registry")
			continue
		}

		// Filter networks marked to ignore
		ignoreChainNames := localConfiguration.Ignores
		restakeChains := []chainregistry.RestakeInfo{}
		for _, chain := range registryConfiguration.Chains {
			for _, ignore := range ignoreChainNames {
				if strings.EqualFold(chain.Name, ignore) {
					rm.logger.Info().Str("chain_id", chain.Name).Msg("ignoring chain due to configuration file")
					continue
				}
			}

			restakeChains = append(restakeChains, chain)
		}

		// Run for each network
		results = []*RestakeResult{}
		for _, restakeChain := range restakeChains {
			wg.Add(1)

			go rm.runRestakeForNetwork(
				localConfiguration,
				restakeChain,
				chainRegistryClient,
			)

			// Print results whenever they all finish
			go func() {
				wg.Wait()
				printResults(results, rm.logger)
			}()
		}
	}
}

func (rm *RestakeManager) runRestakeForNetwork(
	ctx context.Context,
	localConfiguration *Configuration,
	restakeChain chainregistry.RestakeInfo,
	chainRegistryClient chainregistry.ChainRegistryClient,
) {
	timeoutContext, cancelFunc := context.WithTimeout(ctx, localConfiguration.RunInterval())
	defer cancelFunc()

	// Results from the run
	var txHash string
	var err error

	defer func() {
		if err != nil {
			rm.logger.Error().Err(err).Str("chain_id", restakeChain.Name).Msg("restake failed with error")
		}

		// Send health check if enabled
		if !localConfiguration.DisableHealthChecks {
			healthClient := health.NewHealthClient(rm.logger, restakeChain.Name, true)

			if err == nil {
				err = healthClient.SendSuccess(restakeChain.Name)
			} else {
				err = healthClient.SendFailure(restakeChain.Name)
			}
		} else {
			rm.logger.Info().Str("chain_id", restakeChain.Name).Msg("not sending healthchecks.io pings as they are disabled in config.")
		}

		// Add results
		result := &RestakeResult{
			network: restakeChain.Name,
			txHash:  txHash,
			err:     err,
		}
		results = append(results, result)

		// Leave wait group
		defer wg.Done()
	}()
	prefixedLogger := rm.logger.ApplyPrefix(fmt.Sprintf(" [%s]", restakeChain.Name))

	// Get the chain info
	chainInfo, err := chainRegistryClient.ChainInfo(ctx, restakeChain.Name)
	if err == nil {
		return
	}

	// Derive info needed about the chain
	chainID := chainInfo.ChainID

	stakingDenom, err := chainInfo.StakingDenom()
	if err != nil {
		prefixedLogger.Error().Err(err).Str("chain_id", chainID).Msg("failed to get staking denom")
		return
	}

	minimumRequiredReward, err := math.LegacyNewDecFromStr(restakeChain.Restake.MinimumReward.String())
	if err != nil {
		prefixedLogger.Error().Err(err).Str("chain_id", chainID).Str("minimum_reward", restakeChain.Restake.MinimumReward.String()).Msg("failed to parse minimum reward")
		return
	}

	// Derive info needed about Restake
	validatorAddress := restakeChain.Address
	botAddress := restakeChain.Restake.Address

	// Get an endpoint from the Router
	grpcEndpoint, err := rm.chainRouter.GrpcEndpoint(chainInfo.ChainID)
	if err != nil {
		return
	}

	// Bind up an RPC Client with some retries
	rawRpcClient, err := rpc.NewGrpcClient(grpcEndpoint, cdc, prefixedLogger)
	if err != nil {
		return
	}

	rpcClient, err := rpc.NewRetryableRpcClient(localConfiguration.NetworkRetryAttempts, localConfiguration.NetworkRetryDelay(), rawRpcClient)
	if err != nil {
		return
	}

	// Set minimum bot balance to be 1 fee token.
	minimumRequiredBotBalance, err := chainInfo.OneFeeToken(ctx, rpcClient)
	if err != nil {
		return
	}

	// Create a Grant manager
	grantManager, err := NewGrantManager(restakeChain.Restake.Address, chainID, prefixedLogger, rpcClient, restakeChain.Address)
	if err != nil {
		return
	}

	// Create a tx provider
	feeDenom, err := chainInfo.FeeDenom()
	if err != nil {
		return
	}

	slip44 := uint(chainInfo.Slip44)
	signer, err := tx.GetSoftSigner(slip44, localConfiguration.BotMnemonic)
	if err != nil {
		return
	}

	txConfig := txauth.NewTxConfig(cdc, txauth.DefaultSignModes)

	simulationManager, err := tx.NewSimulationManager(localConfiguration.GasFactor, rpcClient, txConfig)
	if err != nil {
		return
	}

	txProvider, err := tx.NewTxProvider(signer, chainID, feeDenom, localConfiguration.VersionedMemo(rm.version), prefixedLogger, simulationManager, txConfig)
	if err != nil {
		return
	}

	// Create a signing metadata provider
	signingMetadataProvider, err := tx.NewSigningMetadataProvider(chainID, rpcClient)
	if err != nil {
		return
	}

	restakeClient, err := NewRestakeClient(
		chainInfo.Bech32Prefix,
		chainID,
		feeDenom,
		stakingDenom,
		validatorAddress,
		botAddress,
		minimumRequiredReward,
		minimumRequiredBotBalance,
		localConfiguration.TxPollDelay(),
		localConfiguration.TxPollAttempts,
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

	txHash, err = restakeClient.Run(timeoutContext)
}

// Results of running Restake on a given network
type RestakeResult struct {
	network string
	txHash  string
	err     error
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
			log.Info().Str("tx_hash", result.txHash).Msg(fmt.Sprintf("✅ %s: Success", result.network))
		} else {
			log.Error().Err(result.err).Msg(fmt.Sprintf("❌ %s: Failure", result.network))
		}
	}
}
