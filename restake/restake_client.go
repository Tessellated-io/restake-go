package restake

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cosmossdk.io/math"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/tessellated-io/pickaxe/cosmos/rpc"
	"github.com/tessellated-io/pickaxe/cosmos/tx"
	"github.com/tessellated-io/pickaxe/crypto"
	"github.com/tessellated-io/pickaxe/log"
)

// A restake client performs a one time restake
type restakeClient struct {
	// Chain specific information
	chainID       string
	addressPrefix string
	feeDenom      string
	stakingDenom  string

	// Restake information
	batchSize                 uint
	botAddress                string
	validatorAddress          string
	minimumRequiredReward     math.LegacyDec
	minimumRequiredBotBalance *sdk.Coin

	// Transaction configuration
	pollDelay    time.Duration
	pollAttempts uint

	// Restake services
	gasManager   tx.GasManager
	grantManager *grantManager
	txProvider   TxProvider

	// Core services
	logger                  *log.Logger
	rpcClient               rpc.RpcClient
	signingMetadataProvider *tx.SigningMetadataProvider

	// Signer
	signer crypto.BytesSigner
}

func NewRestakeClient(
	addressPrefix string,
	chainID string,
	feeDenom string,
	stakingDenom string,

	validatorAddress string,
	botAddress string,

	minimumRequiredReward math.LegacyDec,
	minimumRequiredBotBalance *sdk.Coin,

	pollDelay time.Duration,
	pollAttempts uint,

	gasManager tx.GasManager,
	grantManager *grantManager,
	txProvider TxProvider,

	logger *log.Logger,
	rpcClient rpc.RpcClient,
	signingMetadataProvider *tx.SigningMetadataProvider,

	signer crypto.BytesSigner,
) (*restakeClient, error) {

	return &restakeClient{
		addressPrefix: addressPrefix,
		chainID:       chainID,
		feeDenom:      feeDenom,
		stakingDenom:  stakingDenom,

		botAddress:                botAddress,
		validatorAddress:          validatorAddress,
		minimumRequiredReward:     minimumRequiredReward,
		minimumRequiredBotBalance: minimumRequiredBotBalance,

		pollDelay:    pollDelay,
		pollAttempts: pollAttempts,

		gasManager:   gasManager,
		grantManager: grantManager,
		txProvider:   txProvider,

		logger:                  logger,
		rpcClient:               rpcClient,
		signingMetadataProvider: signingMetadataProvider,

		signer: signer,
	}, nil
}

// TODO: Consider how retryability should work
// TODO: Support batch sizes
// Run runs the restake functionality for this client. It wraps internal calls with a healthcheck.
func (rm *restakeClient) Run(ctx context.Context) (string, error) {
	txHash, err := rm.restake(ctx)
	if err != nil {
		rm.logger.Error().Err(err).Str("chain_id", rm.chainID).Msg("failed sanity checks")
		return "", err
	}

	rm.logger.Info().Str("chain_id", rm.chainID).Str("tx_hash", txHash).Msg("successfully restaked")

	return txHash, nil
}

func (rm *restakeClient) restake(ctx context.Context) (string, error) {
	// 1. Perform sanity checks
	err := rm.performSanityChecks(ctx)
	if err != nil {
		rm.logger.Error().Err(err).Str("chain_id", rm.chainID).Msg("failed sanity checks")
		return "", err
	}

	// 2. Get all valid grants
	restakeDelegators, err := rm.grantManager.getRestakeDelegators(ctx, rm.minimumRequiredReward)
	if err != nil {
		rm.logger.Error().Err(err).Str("chain_id", rm.chainID).Msg("failed to fetch grants")
		return "", err
	}
	if len(restakeDelegators) == 0 {
		rm.logger.Warn().Str("chain_id", rm.chainID).Msg("no grants above minimum found, no restaking will be processed")
	}

	// 3. Create restake messages
	restakeMessages, err := rm.createRestakeMessages(ctx, restakeDelegators)
	if err != nil {
		rm.logger.Error().Err(err).Str("chain_id", rm.chainID).Msg("failed to generate restake messages")
		return "", err
	}

	// 4. Sign the message
	gasPrice, err := rm.gasManager.GetGasPrice(ctx, rm.chainID)
	signingMetadata, err := rm.signingMetadataProvider.SigningMetadataForAccount(ctx, rm.botAddress)
	if signingMetadata != nil {
		rm.logger.Error().Err(err).Str("chain_id", rm.chainID).Msg("failed to get signing metadata for bot")
		return "", err
	}

	signedMessage, err := rm.txProvider.ProvideTx(ctx, gasPrice, restakeMessages, signingMetadata)
	if err != nil {
		rm.logger.Error().Err(err).Str("chain_id", rm.chainID).Msg("failed to generate restake messages")
		return "", err
	}

	// TODO: Retryability for this tx?
	// 5. Broadcast and update the gas
	broadcastResult, err := rm.rpcClient.Broadcast(ctx, signedMessage)
	if err != nil {
		rm.logger.Error().Err(err).Str("chain_id", rm.chainID).Msg("failed to manage gas updates for broadcast result")
		return "", err
	}
	err = rm.gasManager.ManageBroadcastResult(ctx, rm.chainID, broadcastResult)
	if err != nil {
		rm.logger.Error().Err(err).Str("chain_id", rm.chainID).Msg("failed to manage gas updates for broadcast result")
		return "", err
	}
	txHash := broadcastResult.TxResponse.TxHash
	rm.logger.Info().Str("chain_id", rm.chainID).Str("tx_hash", txHash).Uint32("code", broadcastResult.TxResponse.Code).Str("logs", broadcastResult.TxResponse.RawLog).Msg("broadcasted restake transaction")

	// 6. Poll for tx inclusion
	var pollAttempt uint
	for pollAttempt = 0; pollAttempt < rm.pollAttempts; pollAttempt++ {
		// Sleep. Do this first so the tx has some time to settle.
		time.Sleep(rm.pollDelay)

		// Check for confirmation
		included, err := rm.rpcClient.CheckIncluded(ctx, txHash)
		if err != nil {
			rm.logger.Error().Err(err).Str("chain_id", rm.chainID).Str("tx_hash", txHash).Msg("failed to check tx inclusion, will retry")
			continue
		}

		// Return success if included
		if included {
			rm.logger.Info().Str("tx_hash", txHash).Str("chain_id", rm.chainID).Msg("found included transaction")

			err := rm.gasManager.ManageInclusionResult(ctx, rm.chainID, true)
			if err != nil {
				rm.logger.Error().Err(err).Str("chain_id", rm.chainID).Msg("failed to manage gas updates for tx inclusion")
			}

			// Hurrah!
			return txHash, nil
		}

		rm.logger.Info().Str("tx_hash", txHash).Str("chain_id", rm.chainID).Uint("attempt", pollAttempt).Uint("poll_attempts", rm.pollAttempts).Msg("still waiting for inclusion")
	}

	// Create and log an error
	rm.logger.Error().Str("tx_hash", txHash).Str("chain_id", rm.chainID).Uint("poll_attempts", rm.pollAttempts).Dur("poll_delay", rm.pollDelay).Msg("restake transaction not included")
	err = rm.gasManager.ManageInclusionResult(ctx, rm.chainID, false)
	if err != nil {
		rm.logger.Error().Err(err).Str("chain_id", rm.chainID).Msg("failed to manage gas updates for tx inclusion")
	}

	return "", fmt.Errorf("polling for tx hash %s on %s failed")
}

func (rm *restakeClient) performSanityChecks(ctx context.Context) error {
	// Get the bot address and verify it matches what we think it should
	derivedBotAddress := rm.signer.GetAddress(rm.addressPrefix)
	if !strings.EqualFold(derivedBotAddress, rm.botAddress) {
		return fmt.Errorf("Unexpected bot adddress! Expected: %s, Got: %s", rm.botAddress, derivedBotAddress)
	}

	// Get the robot's balance
	balance, err := rm.rpcClient.GetBalance(ctx, rm.botAddress, rm.feeDenom)
	if err != nil {
		return err
	}

	// Ensure the minimum balance
	if rm.minimumRequiredBotBalance.Amount.GT(balance.Amount) {
		return fmt.Errorf("need a higher balance for %s. got: %s, want: %s", rm.botAddress, balance.String(), rm.minimumRequiredBotBalance.String())
	}

	return nil
}

// TODO: Support batching
func (rm *restakeClient) createRestakeMessages(ctx context.Context, delegators []*restakeDelegator) ([]sdk.Msg, error) {
	delegateMsgs := []sdk.Msg{}
	for _, delegator := range delegators {
		// Form our messages
		delegateAmount := sdk.Coin{
			Denom:  rm.stakingDenom,
			Amount: delegator.amount.TruncateInt(),
		}
		delegateMessage := &stakingtypes.MsgDelegate{
			DelegatorAddress: delegator.address,
			ValidatorAddress: rm.validatorAddress,
			Amount:           delegateAmount,
		}

		delegateMsgs = append(delegateMsgs, delegateMessage)
	}

	msgsAny := make([]*cdctypes.Any, len(delegateMsgs))
	for i, msg := range delegateMsgs {
		any, err := cdctypes.NewAnyWithValue(msg)
		if err != nil {
			return nil, err
		}

		msgsAny[i] = any
	}

	execMessage := &authztypes.MsgExec{
		Grantee: rm.botAddress,
		Msgs:    msgsAny,
	}

	return []sdk.Msg{execMessage}, nil
}
