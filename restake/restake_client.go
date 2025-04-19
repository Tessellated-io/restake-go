package restake

import (
	"context"
	"fmt"
	"strings"

	"github.com/tessellated-io/pickaxe/arrays"
	"github.com/tessellated-io/pickaxe/cosmos/rpc"
	"github.com/tessellated-io/pickaxe/cosmos/tx"
	"github.com/tessellated-io/pickaxe/crypto"
	"github.com/tessellated-io/pickaxe/log"

	"cosmossdk.io/math"

	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// A restake client performs a one time restake
type restakeClient struct {
	// Chain specific information
	addressPrefix string
	chainID       string
	feeDenom      string
	stakingDenom  string

	// Restake information
	batchSize                 uint
	botAddress                string
	validatorAddress          string
	minimumRequiredReward     math.LegacyDec
	minimumRequiredBotBalance *sdk.Coin
	markEmptyRestakeAsFailed  bool

	// Restake services
	grantManager *grantManager

	// Core services
	broadcaster *tx.Broadcaster
	logger      *log.Logger
	rpcClient   rpc.RpcClient

	// Signer
	signer crypto.BytesSigner
}

func NewRestakeClient(
	addressPrefix string,
	chainID string,
	feeDenom string,
	stakingDenom string,

	batchSize uint,
	validatorAddress string,
	botAddress string,
	minimumRequiredReward math.LegacyDec,
	minimumRequiredBotBalance *sdk.Coin,
	markEmptyRestakeAsFailed bool,

	broadcaster *tx.Broadcaster,

	grantManager *grantManager,

	logger *log.Logger,
	rpcClient rpc.RpcClient,

	signer crypto.BytesSigner,
) (*restakeClient, error) {
	return &restakeClient{
		addressPrefix: addressPrefix,
		chainID:       chainID,
		feeDenom:      feeDenom,
		stakingDenom:  stakingDenom,

		batchSize:                 batchSize,
		botAddress:                botAddress,
		validatorAddress:          validatorAddress,
		minimumRequiredReward:     minimumRequiredReward,
		minimumRequiredBotBalance: minimumRequiredBotBalance,
		markEmptyRestakeAsFailed:  markEmptyRestakeAsFailed,

		grantManager: grantManager,

		broadcaster: broadcaster,
		logger:      logger,
		rpcClient:   rpcClient,

		signer: signer,
	}, nil
}

func (rc *restakeClient) restake(ctx context.Context) ([]string, error) {
	logger := rc.logger.With("chain_id", rc.chainID)

	logger.Info("performing pre-flight sanity checks")
	// 1. Perform sanity checks
	err := rc.performSanityChecks(ctx)
	if err != nil {
		logger.Error("failed sanity checks", "error", err.Error())
		return nil, err
	}
	logger.Info("starting to restake")

	// 2. Get all valid grants
	logger.Info("fetching delegators and grants. this might take a while if there are many delegators...")
	restakeDelegators, err := rc.grantManager.getRestakeDelegators(ctx, rc.minimumRequiredReward)
	if err != nil {
		logger.Error("failed to fetch grants", "error", err.Error())
		return nil, err
	}
	if len(restakeDelegators) == 0 {
		remarks := "no grants above minimum found, no restaking will be processed"
		logger.Warn(remarks)

		if rc.markEmptyRestakeAsFailed {
			return nil, fmt.Errorf(remarks)
		}
	}
	logger.Info("finished fetching delegators and grants")

	// 3. Create restake messages
	logger.Info("creating restake messages")
	batches, err := rc.createRestakeMessages(ctx, restakeDelegators)
	if err != nil {
		logger.Error("failed to generate restake messages", "error", err)
		return nil, err
	}
	logger.Info("finished creating messages")

	// 4. Send in batches
	logger.Info("sending restake batches")
	txHashes := []string{}
	for batchNum, batch := range batches {
		logger.Info("📬 sending a batch of messages", "batch_size", rc.batchSize, "batch", batchNum+1, "total_batches", len(batches))

		txHash, err := rc.broadcaster.SignAndBroadcast(ctx, []sdk.Msg{batch})
		logger = logger.With("tx_hash", txHash)

		logger.Debug("restake_client::got result from signAndBroadcast", "error", err.Error())
		if err != nil {
			return nil, err
		}
		txHashes = append(txHashes, txHash)
		logger.Info("📭 batch sent successfully", "batch_size", rc.batchSize, "batch", batchNum+1, "total_batches", len(batches))
	}

	logger.Info("🙌 successfully restaked")
	return txHashes, nil
}

func (rc *restakeClient) performSanityChecks(ctx context.Context) error {
	// Get the bot address and verify it matches what we think it should
	derivedBotAddress := rc.signer.GetAddress(rc.addressPrefix)
	if !strings.EqualFold(derivedBotAddress, rc.botAddress) {
		return fmt.Errorf("unexpected bot adddress! expected: %s, got: %s", rc.botAddress, derivedBotAddress)
	}

	// Get the robot's balance
	balance, err := rc.rpcClient.GetBalance(ctx, rc.botAddress, rc.feeDenom)
	if err != nil {
		return err
	}

	// Ensure the minimum balance
	if rc.minimumRequiredBotBalance.Amount.GT(balance.Amount) {
		return fmt.Errorf("need a higher balance for %s. got: %s, want: %s", rc.botAddress, balance.String(), rc.minimumRequiredBotBalance.String())
	}

	return nil
}

func (rc *restakeClient) createRestakeMessages(ctx context.Context, delegators []*restakeDelegator) ([]sdk.Msg, error) {
	// Form delegate messages
	delegateMsgs := []sdk.Msg{}
	for _, delegator := range delegators {
		delegateAmount := sdk.Coin{
			Denom:  rc.stakingDenom,
			Amount: delegator.amount.TruncateInt(),
		}
		delegateMessage := &stakingtypes.MsgDelegate{
			DelegatorAddress: delegator.address,
			ValidatorAddress: rc.validatorAddress,
			Amount:           delegateAmount,
		}

		delegateMsgs = append(delegateMsgs, delegateMessage)
	}

	// Batch them
	batches := arrays.Batch(delegateMsgs, int(rc.batchSize))

	// Map them into an exec message
	restakeMessages := []sdk.Msg{}
	for _, batch := range batches {
		msgsAny := []*cdctypes.Any{}

		for _, msg := range batch {
			any, err := cdctypes.NewAnyWithValue(msg)
			if err != nil {
				return nil, err
			}

			msgsAny = append(msgsAny, any)
		}

		execMessage := &authztypes.MsgExec{
			Grantee: rc.botAddress,
			Msgs:    msgsAny,
		}

		restakeMessages = append(restakeMessages, execMessage)
	}

	return restakeMessages, nil
}
