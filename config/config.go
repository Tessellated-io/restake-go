package config

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/tessellated-io/pickaxe/arrays"
	"github.com/tessellated-io/restake-go/log"
	"github.com/tessellated-io/restake-go/registry"
	"github.com/tessellated-io/router/router"
)

func GetRestakeConfig(ctx context.Context, filename string, log *log.Logger) (*RestakeConfig, error) {
	// Get data from the file
	fileConfig, err := parseConfig(filename)
	if err != nil {
		return nil, err
	}

	// Request network data for the validator
	log.Info().Msg("Loading configs...")
	registryClient := registry.NewRegistryClient()
	restakeInfos, err := registryClient.GetRestakeChains(ctx, fileConfig.Moniker)
	if err != nil {
		return nil, err
	}

	// Filter ignores
	filteredRestakeInfos := arrays.Filter(restakeInfos, func(input registry.RestakeInfo) bool {
		log.Info().Str("network", input.Name).Msg("Found config")

		for _, ignore := range fileConfig.Ignores {
			if strings.EqualFold(input.Name, ignore) {
				log.Info().Str("network", ignore).Msg("		/ Ignoring network marked to ignore")
				return false
			}
		}
		return true
	})

	// Loop through each restake chain, resolving the data
	chainRouter, err := router.NewRouter(nil)
	if err != nil {
		return nil, err
	}
	configs := []*ChainConfig{}
	for _, restakeInfo := range filteredRestakeInfos {
		// Fetch chain info
		registryChainInfo, err := registryClient.GetChainInfo(ctx, restakeInfo.Name)
		if err != nil {
			return nil, err
		}

		// Extract the relevant file config
		fileChainInfo, err := extractFileConfig(registryChainInfo.ChainID, fileConfig.Chains)
		if err != nil {
			return nil, err
		}

		// Create a chain object for the router
		chain, err := router.NewChain(restakeInfo.Name, registryChainInfo.ChainID, &fileChainInfo.Grpc)
		if err != nil {
			return nil, err
		}
		err = chainRouter.AddChain(chain)
		if err != nil {
			return nil, err
		}

		mergedConfig := &MergedConfig{
			ChainInfo:       registryChainInfo,
			UserChainConfig: fileChainInfo,
			RestakeInfo:     restakeInfo,
		}

		config, err := newChainConfig(
			mergedConfig,
			chainRouter,
		)
		if err != nil {
			return nil, err
		}
		configs = append(configs, config)
	}

	return &RestakeConfig{
		Mnemonic:         fileConfig.Mnemonic,
		Memo:             fileConfig.Memo,
		RunIntervalHours: fileConfig.RunIntervalHours,
		Chains:           configs,
	}, nil
}

func extractFileConfig(needle string, haystack []UserChainConfig) (*UserChainConfig, error) {
	for _, candidate := range haystack {
		if strings.EqualFold(candidate.ChainID, needle) {
			return &candidate, nil
		}
	}
	return nil, fmt.Errorf("failed to find a network for %s in the config file", needle)
}

func newChainConfig(
	config *MergedConfig,
	router router.Router,
) (*ChainConfig, error) {
	// Pull gRPC and chain name from router so that clients can shim in.
	grpc, err := router.GetGrpcEndpoint(config.UserChainConfig.ChainID)
	if err != nil {
		return nil, err
	}
	network, err := router.GetHumanReadableName(config.UserChainConfig.ChainID)
	if err != nil {
		return nil, err
	}

	// Extract the gas price
	stakingTokens := config.ChainInfo.Staking.StakingTokens
	if len(stakingTokens) > 1 {
		panic(fmt.Errorf("found too many staking tokens in chain registry for %s", config.ChainInfo.ChainName))
	}
	stakingDenom := stakingTokens[0].Denom

	feeTokens := config.ChainInfo.Fees.FeeTokens
	feeToken, err := extractFeeToken(stakingDenom, feeTokens)
	if err != nil {
		panic(fmt.Errorf("found too many staking tokens in chain registry for %s", config.ChainInfo.ChainName))
	}

	gasPrice := feeToken.FixedMinGasPrice

	return &ChainConfig{
		network:            network,
		HealthcheckId:      config.UserChainConfig.HealthCheckID,
		ValidatorAddress:   config.RestakeInfo.Address,
		FeeDenom:           config.ChainInfo.Fees.FeeTokens[0].Denom,
		ExpectedBotAddress: config.RestakeInfo.Restake.Address,
		MinRestakeAmount:   big.NewInt(0), //big.NewInt(int64(config.RestakeInfo.Restake.MinimumReward)),
		AddressPrefix:      config.ChainInfo.Bech32Prefix,
		chainID:            config.ChainInfo.ChainID,
		CoinType:           config.ChainInfo.Slip44,
		nodeGrpcURI:        grpc,
		GasPrice:           gasPrice,
	}, nil
}

func extractFeeToken(needle string, haystack []registry.FeeToken) (*registry.FeeToken, error) {
	for _, candidate := range haystack {
		if strings.EqualFold(candidate.Denom, needle) {
			return &candidate, nil
		}
	}
	return nil, fmt.Errorf("failed to find a fee token for %s in the registry response", needle)
}
