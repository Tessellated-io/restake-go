package restake

import (
	"context"
	"time"

	"github.com/tessellated-io/pickaxe/arrays"
	"github.com/tessellated-io/pickaxe/cosmos/rpc"
	"github.com/tessellated-io/pickaxe/log"

	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
)

// restakeDelegator defines data about a restake delegator using Restake
type restakeDelegator struct {
	address string
	amount  sdk.Dec
}

// Grant manager gets grants for Restaking
type grantManager struct {
	botAddress       string
	chainID          string
	logger           *log.Logger
	rpcClient        rpc.RpcClient
	stakingDenom     string
	validatorAddress string
}

func NewGrantManager(botAddress, chainID string, logger *log.Logger, rpcClient rpc.RpcClient, stakingToken, validatorAddress string) (*grantManager, error) {
	return &grantManager{
		botAddress:       botAddress,
		chainID:          chainID,
		logger:           logger,
		rpcClient:        rpcClient,
		stakingDenom:     stakingToken,
		validatorAddress: validatorAddress,
	}, nil
}

func (gm *grantManager) getRestakeDelegators(ctx context.Context, minimumReward math.LegacyDec) ([]*restakeDelegator, error) {
	logger := gm.logger.With("chain_id", gm.chainID, "bot_address", gm.botAddress, "staking_token", gm.stakingDenom)
	logger.Debug("fetching grants")

	// Get all grants to the bot
	allGrants, err := gm.rpcClient.GetGrants(ctx, gm.botAddress)
	if err != nil {
		return nil, err
	}

	// Filter for grants that can be restaked
	grants := arrays.Filter(allGrants, isValidGrant)
	logger.Debug("Found valid grants", "grants", len(grants))
	if len(grants) == 0 {
		// NOTE: We log a warning for this in `restakeManager`, so we silently return here.
		return []*restakeDelegator{}, nil
	}

	// Convert into the addresses of delegators
	delegatorAddresses := arrays.Map(grants, func(input *authztypes.GrantAuthorization) string { return input.Granter })

	// For each delegator, fetch their rewards and add them to get restaked if they're above the threshold
	restakeDelegators := []*restakeDelegator{}
	for _, delegatorAddress := range delegatorAddresses {
		// Fetch total rewards
		totalRewards, err := gm.rpcClient.GetPendingRewards(ctx, delegatorAddress, gm.validatorAddress, gm.stakingDenom)
		if err != nil {
			return nil, err
		}
		logger = logger.With("delegator", delegatorAddress, "total_rewards", totalRewards.String())
		logger.Debug("fetched delegation rewards")

		// Determine if they are above the minimum
		if totalRewards.LT(minimumReward) {
			logger.Debug("skipping because below award threshold", "minimum_rewards", minimumReward.String())
			continue
		}

		// Add to grants
		restakeDelegator := &restakeDelegator{
			address: delegatorAddress,
			amount:  totalRewards,
		}
		restakeDelegators = append(restakeDelegators, restakeDelegator)
	}

	logger.Debug("fetched valid grants above minimum", "eligible_delegators", len(restakeDelegators), "minimum", minimumReward.String())
	return restakeDelegators, nil
}

// Helper functions

// Docs on unpacking any: https://docs.cosmos.network/main/core/encoding.html#interface-encoding-and-usage-of-any
// NOTE: The above doesn't seem to work, perhaps I need to register a codec somewhere. I gave up since using a type-url seems good enough.

func isValidGrant(grant *authztypes.GrantAuthorization) bool {
	// Ensure grant hasn't expired
	expiration := grant.Expiration
	now := time.Now()
	if expiration.Before(now) {
		return false
	}

	if grant.Authorization.TypeUrl == "/cosmos.staking.v1beta1.StakeAuthorization" {
		return true
	}

	if grant.Authorization.TypeUrl == "/cosmos.authz.v1beta1.GenericAuthorization" {
		return true
	}

	return false
}
