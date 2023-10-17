package restake

import (
	"context"
	"fmt"

	"github.com/restake-go/config"
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
}

func NewRestakeManager(
	rpcClient rpc.RpcClient,
	cdc *codec.ProtoCodec,
	mnemonic string,
	memo string,
	gasFactor float64,
	config config.ChainConfig,
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
	}, nil
}

type restakeTarget struct {
	delegator string
	amount    sdk.Dec
}

// TODO: Rip out unused rpc methods
func (r *RestakeManager) Restake(ctx context.Context) error {
	// Get all delegators and grants to the bot
	allGrants, err := r.rpcClient.GetGrants(ctx, r.botAddress)
	if err != nil {
		return err
	}

	validGrants := arrays.Filter(allGrants, isValidGrant)
	fmt.Printf("	...Found %d valid grants\n", len(validGrants))

	// Map to balances and then filter by rewards
	validDelegators := arrays.Map(validGrants, func(input *authztypes.GrantAuthorization) string { return input.Granter })
	restakeTargets := []*restakeTarget{}
	for _, validDelegator := range validDelegators {
		// Fetch total rewards
		totalRewards, err := r.rpcClient.GetPendingRewards(ctx, validDelegator, r.validatorAddress, r.stakingToken)
		if err != nil {
			return err
		}
		fmt.Printf("	...Delegator %s has %s %s in delegation rewards\n", validDelegator, totalRewards, r.stakingToken)

		restakeTarget := &restakeTarget{
			delegator: validDelegator,
			amount:    totalRewards,
		}
		restakeTargets = append(restakeTargets, restakeTarget)
	}
	targetsAboveMinimum := arrays.Filter(restakeTargets, func(input *restakeTarget) bool {
		return input.amount.GTE(r.minRewards)
	})
	fmt.Printf("...Found %d grants above the reward threshold\n", len(targetsAboveMinimum))

	// Restake all delegators
	if len(targetsAboveMinimum) > 0 {
		return r.restakeDelegators(ctx, targetsAboveMinimum)
	}

	return fmt.Errorf("no valid grants found")
}

func (r *RestakeManager) restakeDelegators(ctx context.Context, targets []*restakeTarget) error {
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

	return r.signer.SendMessages(ctx, []sdk.Msg{&execMessage})
}

func (r *RestakeManager) Network() string {
	return r.network
}
