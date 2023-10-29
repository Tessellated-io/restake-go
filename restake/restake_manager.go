package restake

import (
	"context"
	"fmt"

	"github.com/tessellated-io/pickaxe/arrays"
	"github.com/tessellated-io/pickaxe/cosmos/rpc"
	"github.com/tessellated-io/pickaxe/crypto"
	"github.com/tessellated-io/pickaxe/log"
	"github.com/tessellated-io/restake-go/config"
	"github.com/tessellated-io/restake-go/signer"

	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

type RestakeManager struct {
	network string

	rpcClient        rpc.RpcClient
	validatorAddress string
	botAddress       string

	stakingToken string
	minRewards   sdk.Dec

	addressPrefix string
	signer        *signer.Signer

	log *log.Logger
}

func NewRestakeManager(
	rpcClient rpc.RpcClient,
	cdc *codec.ProtoCodec,
	mnemonic string,
	memo string,
	version string,
	gasFactor float64,
	config config.ChainConfig,
	log *log.Logger,
) (*RestakeManager, error) {
	var keyPair crypto.BytesSigner = crypto.NewKeyPairFromMnemonic(mnemonic, uint32(config.CoinType))
	if config.CoinType == 60 {
		keyPair = crypto.NewEthermintKeyPairFromMnemonic(mnemonic)
	}

	versionedMemo := fmt.Sprintf("%s | restake-go %s", memo, version)

	signer := signer.NewSigner(
		gasFactor,
		cdc,
		config.ChainID(),
		keyPair,
		config.GasPrice,
		config.AddressPrefix,
		rpcClient,
		versionedMemo,
		config.FeeDenom,
		log,
	)

	address := keyPair.GetAddress(config.AddressPrefix)
	if address != config.ExpectedBotAddress {
		return nil, fmt.Errorf("unexpected address: %s (expected: %s)", address, config.ExpectedBotAddress)
	}

	return &RestakeManager{
		rpcClient:        rpcClient,
		validatorAddress: config.ValidatorAddress,
		botAddress:       config.ExpectedBotAddress,
		network:          config.Network(),

		stakingToken: config.FeeDenom,
		minRewards:   sdk.NewDec(config.MinRestakeAmount.Int64()),

		addressPrefix: config.AddressPrefix,
		signer:        signer,

		log: log,
	}, nil
}

type restakeTarget struct {
	delegator string
	amount    sdk.Dec
}

func (r *RestakeManager) Restake(ctx context.Context) (txHash string, err error) {
	startMessage := fmt.Sprintf("♻️  Starting Restake on %s", r.Network())
	r.log.Debug().Msg(startMessage)

	// Get all delegators and grants to the bot
	allGrants, err := r.rpcClient.GetGrants(ctx, r.botAddress)
	if err != nil {
		return "", err
	}

	validGrants := arrays.Filter(allGrants, isValidGrant)
	r.log.Debug().Int("valid grants", len(validGrants)).Msg("Found valid grants")
	if len(validGrants) == 0 {
		return "", fmt.Errorf("no valid grants found")
	}

	// Map to balances and then filter by rewards
	validDelegators := arrays.Map(validGrants, func(input *authztypes.GrantAuthorization) string { return input.Granter })
	restakeTargets := []*restakeTarget{}
	for _, validDelegator := range validDelegators {
		// Fetch total rewards
		totalRewards, err := r.rpcClient.GetPendingRewards(ctx, validDelegator, r.validatorAddress, r.stakingToken)
		if err != nil {
			return "", err
		}
		r.log.Debug().Str("delegator", validDelegator).Str("total rewards", totalRewards.String()).Str("staking token", r.stakingToken).Msg("Fetched delegation rewards")

		restakeTarget := &restakeTarget{
			delegator: validDelegator,
			amount:    totalRewards,
		}
		restakeTargets = append(restakeTargets, restakeTarget)
	}
	targetsAboveMinimum := arrays.Filter(restakeTargets, func(input *restakeTarget) bool {
		return input.amount.GTE(r.minRewards)
	})
	r.log.Debug().Int("grants_above_min", len(targetsAboveMinimum)).Str("minimum", r.minRewards.String()).Msg("fetched grants above minimum")

	// Restake all delegators
	if len(targetsAboveMinimum) > 0 {
		return r.restakeDelegators(ctx, targetsAboveMinimum)
	}

	return "", fmt.Errorf("no grants above minimum found")
}

func (r *RestakeManager) restakeDelegators(ctx context.Context, targets []*restakeTarget) (txHash string, err error) {
	delegateMsgs := []sdk.Msg{}
	for _, target := range targets {
		// Form our messages
		delegateAmount := sdk.Coin{
			Denom:  r.stakingToken,
			Amount: target.amount.TruncateInt(),
		}
		delegateMessage := &stakingtypes.MsgDelegate{
			DelegatorAddress: target.delegator,
			ValidatorAddress: r.validatorAddress,
			Amount:           delegateAmount,
		}

		delegateMsgs = append(delegateMsgs, delegateMessage)
	}

	msgsAny := make([]*cdctypes.Any, len(delegateMsgs))
	for i, msg := range delegateMsgs {
		any, err := cdctypes.NewAnyWithValue(msg)
		if err != nil {
			return "", err
		}

		msgsAny[i] = any
	}

	execMessage := authztypes.MsgExec{
		Grantee: r.botAddress,
		Msgs:    msgsAny,
	}

	return r.signer.SendMessages(ctx, []sdk.Msg{&execMessage})
}

func (r *RestakeManager) Network() string {
	return r.network
}
