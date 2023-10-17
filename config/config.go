package config

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/tessellated-io/pickaxe/arrays"
	"github.com/tessellated-io/restake-go/log"
	"github.com/tessellated-io/restake-go/registry"
)

type RestakeConfig struct {
	Memo           string
	Mnemonic       string
	SleepTimeHours int
	Chains         []*ChainConfig
}

type ChainConfig struct {
	Network            string
	HealthcheckId      string
	ValidatorAddress   string
	FeeDenom           string
	ExpectedBotAddress string
	MinRestakeAmount   *big.Int
	AddressPrefix      string
	ChainId            string
	NodeGrpcURI        string
	GasPrice           float64

	CoinType int
}

func GetRestakeConfig(ctx context.Context, filename string, log *log.Logger) (*RestakeConfig, error) {
	// Get data from the file
	fileConfig, err := parseConfig(filename)
	if err != nil {
		return nil, err
	}

	// Request network data for the validator
	registryClient := registry.NewRegistryClient()
	restakeChains, err := registryClient.GetRestakeChains(ctx, fileConfig.Moniker)
	if err != nil {
		return nil, err
	}

	// Filter ignores
	log.Info().Msg("Loading configs...")
	filtered := arrays.Filter(restakeChains, func(input registry.Chain) bool {
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
	configs := []*ChainConfig{}
	for _, restakeChain := range filtered {
		// Extract the relevant file config
		fileChainInfo, err := extractFileConfig(restakeChain.Name, fileConfig.Chains)
		if err != nil {
			return nil, err
		}

		// Fetch chain info
		registryChainInfo, err := registryClient.GetChainInfo(ctx, restakeChain.Name)
		if err != nil {
			return nil, err
		}

		// Find the staking token
		stakingTokens := registryChainInfo.Staking.StakingTokens
		if len(stakingTokens) > 1 {
			panic(fmt.Errorf("found too many staking tokens in chain registry for %s", restakeChain.Name))
		}
		stakingDenom := stakingTokens[0].Denom

		// Use that to pay fees
		feeTokens := registryChainInfo.Fees.FeeTokens
		feeToken, err := extractFeeToken(stakingDenom, feeTokens)
		if err != nil {
			panic(fmt.Errorf("found too many staking tokens in chain registry for %s", restakeChain.Name))
		}

		config := newChainConfig(
			restakeChain.Name,
			fileChainInfo.HealthCheckID,
			restakeChain.Address,
			registryChainInfo.Fees.FeeTokens[0].Denom,
			restakeChain.Restake.Address,
			fileChainInfo.MinRestakeAmount,
			registryChainInfo.Bech32Prefix,
			registryChainInfo.ChainID,
			registryChainInfo.Slip44,
			fileChainInfo.Grpc,
			feeToken.FixedMinGasPrice,
		)
		configs = append(configs, config)
	}

	return &RestakeConfig{
		Mnemonic:       fileConfig.Mnemonic,
		Memo:           fileConfig.Memo,
		SleepTimeHours: fileConfig.SleepTimeHours,
		Chains:         configs,
	}, nil
}

func extractFileConfig(needle string, haystack []RestakeChain) (*RestakeChain, error) {
	for _, candidate := range haystack {
		if strings.EqualFold(candidate.Name, needle) {
			return &candidate, nil
		}
	}
	return nil, fmt.Errorf("failed to find a network for %s in the config file", needle)
}

func newChainConfig(
	network string,
	healthcheckId string,
	validatorAddress string,
	feeDenom string,
	expectedBotAddress string,
	minRestakeAmount int,
	addressPrefix string,
	chainId string,
	coinType int,
	grpc string,
	gasPrice float64,
) *ChainConfig {
	return &ChainConfig{
		Network:            network,
		HealthcheckId:      healthcheckId,
		ValidatorAddress:   validatorAddress,
		FeeDenom:           feeDenom,
		ExpectedBotAddress: expectedBotAddress,
		MinRestakeAmount:   big.NewInt(int64(minRestakeAmount)),
		AddressPrefix:      addressPrefix,
		ChainId:            chainId,
		CoinType:           coinType,
		NodeGrpcURI:        grpc,
		GasPrice:           gasPrice,
	}
}

func extractFeeToken(needle string, haystack []registry.FeeToken) (*registry.FeeToken, error) {
	for _, candidate := range haystack {
		if strings.EqualFold(candidate.Denom, needle) {
			return &candidate, nil
		}
	}
	return nil, fmt.Errorf("failed to find a network for %s in the config file", needle)
}
