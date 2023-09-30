package restake

import (
	"context"
	"fmt"

	"github.com/restake-go/config"
	"github.com/restake-go/health"
	"github.com/restake-go/rpc"
	"github.com/restake-go/signer"
	"github.com/tessellated-io/pickaxe/arrays"
	"github.com/tessellated-io/pickaxe/crypto"

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

	healthClient *health.HealthCheckClient
}

func NewRestakeManager(
	rpcClient rpc.RpcClient,
	cdc *codec.ProtoCodec,
	mnemonic string,
	memo string,
	gasFactor float64,
	config config.ChainConfig,
	healthClient *health.HealthCheckClient,
) (*RestakeManager, error) {
	var keyPair crypto.BytesSigner = crypto.NewKeyPairFromMnemonic(mnemonic, uint32(config.CoinType))
	if config.CoinType == 60 {
		keyPair = crypto.NewEthermintKeyPairFromMnemonic(mnemonic)
	}
	signer := signer.NewSigner(
		gasFactor,
		cdc,
		config.ChainId,
		keyPair,
		config.GasPrice,
		config.AddressPrefix,
		rpcClient,
		memo,
		config.FeeDenom,
	)

	address := keyPair.GetAddress(config.AddressPrefix)
	if address != config.ExpectedBotAddress {
		return nil, fmt.Errorf("unexpected address: %s (expected: %s)", address, config.ExpectedBotAddress)
	}

	return &RestakeManager{
		rpcClient:        rpcClient,
		validatorAddress: config.ValidatorAddress,
		botAddress:       config.ExpectedBotAddress,
		network:          config.Network,

		stakingToken: config.FeeDenom,
		minRewards:   sdk.NewDec(config.MinRestakeAmount.Int64()),

		addressPrefix: config.AddressPrefix,
		signer:        signer,

		healthClient: healthClient,
	}, nil
}

type restakeTarget struct {
	delegator string
	amount    sdk.Dec
}

// TODO: Rip out unused rpc methods
func (r *RestakeManager) Restake(ctx context.Context) {
	r.healthClient.Start("restaking")

	// Get all delegators and grants to the bot
	// allDelegators := r.rpcClient.GetDelegators(ctx, r.validatorAddress)
	allGrants := r.rpcClient.GetGrants(ctx, r.botAddress)
	validGrants := arrays.Filter(allGrants, isValidGrant)
	fmt.Printf("	...Found %d valid grants\n", len(validGrants))

	// Map to balances and then filter by rewards
	validDelegators := arrays.Map(validGrants, func(input *authztypes.GrantAuthorization) string { return input.Granter })
	restakeTargets := arrays.Map(validDelegators, func(input string) *restakeTarget {
		// Fetch total rewards
		totalRewards := r.rpcClient.GetPendingRewards(ctx, input, r.validatorAddress, r.stakingToken)
		fmt.Printf("	...Delegator %s has %s %s in delegation rewards\n", input, totalRewards, r.stakingToken)

		return &restakeTarget{
			delegator: input,
			amount:    totalRewards,
		}
	})
	targetsAboveMinimum := arrays.Filter(restakeTargets, func(input *restakeTarget) bool {
		return input.amount.GTE(r.minRewards)
	})
	fmt.Printf("...Found %d grants above the reward threshold\n", len(targetsAboveMinimum))

	// Restake all delegators
	if len(targetsAboveMinimum) > 0 {
		r.restakeDelegators(ctx, targetsAboveMinimum)
		r.healthClient.Success("hurrah")
	} else {
		r.healthClient.Failed("no valid grants found")
	}
}

func (r *RestakeManager) restakeDelegators(ctx context.Context, targets []*restakeTarget) {
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
			panic(err)
		}

		msgsAny[i] = any
	}

	execMessage := authztypes.MsgExec{
		Grantee: r.botAddress,
		Msgs:    msgsAny,
	}

	r.signer.SendMessages(ctx, []sdk.Msg{&execMessage})
}

func (r *RestakeManager) Network() string {
	return r.network
}
