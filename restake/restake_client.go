package restake

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	chainID       string
	chainName     string
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
	txPollDelay          time.Duration
	txPollAttempts       uint
	networkRetryDelay    time.Duration
	networkRetryAttempts uint

	// Restake services
	gasManager   tx.GasManager
	grantManager *grantManager
	txProvider   tx.TxProvider

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
	chainName string,
	feeDenom string,
	stakingDenom string,

	batchSize uint,
	validatorAddress string,
	botAddress string,
	minimumRequiredReward math.LegacyDec,
	minimumRequiredBotBalance *sdk.Coin,

	txPollDelay time.Duration,
	txPollAttempts uint,
	networkRetryDelay time.Duration,
	networkRetryAttempts uint,

	broadcaster tx.Broadcaster

	gasManager tx.GasManager,
	grantManager *grantManager,
	txProvider tx.TxProvider,

	logger *log.Logger,
	rpcClient rpc.RpcClient,
	signingMetadataProvider *tx.SigningMetadataProvider,

	signer crypto.BytesSigner,
) (*restakeClient, error) {
	return &restakeClient{
		addressPrefix: addressPrefix,
		chainID:       chainID,
		chainName:     chainName,
		feeDenom:      feeDenom,
		stakingDenom:  stakingDenom,

		batchSize:                 batchSize,
		botAddress:                botAddress,
		validatorAddress:          validatorAddress,
		minimumRequiredReward:     minimumRequiredReward,
		minimumRequiredBotBalance: minimumRequiredBotBalance,

		txPollDelay:          txPollDelay,
		txPollAttempts:       txPollAttempts,
		networkRetryDelay:    networkRetryDelay,
		networkRetryAttempts: networkRetryAttempts,

		gasManager:   gasManager,
		grantManager: grantManager,
		txProvider:   txProvider,

		logger:                  logger,
		rpcClient:               rpcClient,
		signingMetadataProvider: signingMetadataProvider,

		signer: signer,
	}, nil
}

func (rc *restakeClient) restake(ctx context.Context) ([]string, error) {
	// 1. Perform sanity checks
	err := rc.performSanityChecks(ctx)
	if err != nil {
		rc.logger.Error().Err(err).Str("chain_id", rc.chainID).Msg("failed sanity checks")
		return nil, err
	}

	// 2. Get all valid grants
	restakeDelegators, err := rc.grantManager.getRestakeDelegators(ctx, rc.minimumRequiredReward)
	if err != nil {
		rc.logger.Error().Err(err).Str("chain_id", rc.chainID).Msg("failed to fetch grants")
		return nil, err
	}
	if len(restakeDelegators) == 0 {
		rc.logger.Warn().Str("chain_id", rc.chainID).Msg("no grants above minimum found, no restaking will be processed")
	}

	// 3. Create restake messages
	batches, err := rc.createRestakeMessages(ctx, restakeDelegators)
	if err != nil {
		rc.logger.Error().Err(err).Str("chain_id", rc.chainID).Msg("failed to generate restake messages")
		return nil, err
	}

	// 4. Send in batches
	txHashes := []string{}
	for batchNum, batch := range batches {
		rc.logger.Info().Uint("batch_size", rc.batchSize).Int("batch", batchNum+1).Int("total_batches", len(batches)).Msg("📬 sending a batch of messages")

		txHash, err := rc.broadcaster.SignAndBroadcast(ctx, []sdk.Msg{batch})
		if err != nil {
			return nil, err
		}
		txHashes = append(txHashes, txHash)
		rc.logger.Info().Uint("batch_size", rc.batchSize).Int("batch", batchNum+1).Int("total_batches", len(batches)).Msg("📭 batch sent successfully")
	}

	rc.logger.Info().Str("chain_id", rc.chainID).Msg("🙌 successfully restaked")
	return txHashes, nil
}

// Wrap `sendBatch` so we can retry in case there are hiccups around gas or nonces
func (rc *restakeClient) sendBatchWithRetries(ctx context.Context, batch sdk.Msg) (string, error) {
	var err error
	var txHash string

	var i uint
	for i = 0; i < rc.networkRetryAttempts; i++ {
		// Return early if the context has expired
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		txHash, err = rc.sendBatch(ctx, batch)
		if err == nil {
			return txHash, err
		}

		if i+1 != rc.networkRetryAttempts {
			rc.logger.Error().Err(err).Str("chain_id", rc.chainID).Msg("failed to send batch, will retry")
			time.Sleep(rc.networkRetryDelay)
		}
	}

	rc.logger.Error().Str("chain_id", rc.chainID).Msg("all attempts to send batch failed.")
	return txHash, err
}

func (rc *restakeClient) sendBatch(ctx context.Context, batch sdk.Msg) (string, error) {
	// Sign the message
	gasPrice, err := rc.gasManager.GetGasPrice(ctx, rc.chainName)
	if err != nil {
		rc.logger.Error().Err(err).Str("chain_id", rc.chainID).Msg("failed to get gas price")
		return "", err
	}

	signingMetadata, err := rc.signingMetadataProvider.SigningMetadataForAccount(ctx, rc.botAddress)
	if err != nil {
		rc.logger.Error().Err(err).Str("chain_id", rc.chainID).Msg("failed to get signing metadata for bot")
		return "", err
	}

	signedMessage, gasWanted, err := rc.txProvider.ProvideTx(ctx, gasPrice, []sdk.Msg{batch}, signingMetadata)
	if err != nil {
		rc.logger.Error().Err(err).Str("chain_id", rc.chainID).Msg("failed to generate restake messages")
		return "", err
	}
	rc.logger.Debug().Str("chain_id", rc.chainID).Msg("signed transaction")

	// Broadcast and update the gas
	broadcastResult, broadcastErr := rc.rpcClient.Broadcast(ctx, signedMessage)

	// Not what you would expect. Some chains, for instance, Agoric, will return an error *AND* a broadcast result.
	// To work around this
	// 1. always try to manage a non-nil broadcast result for gas.
	//		a. Always print any information we got about the broadcast
	//		b. Print any errors about gas management, but don't return it. These errors are non-fatal error
	// 2. ditch if error is non-nil, since it didn't broadacst
	// 3. ditch if code is non-zero, since that also symbolizes it didn't broadcast. Turn the rawlogs into an error in this case
	if broadcastResult != nil {
		txHash := broadcastResult.TxResponse.TxHash
		codespace := broadcastResult.TxResponse.Codespace
		broadcastResponseCode := broadcastResult.TxResponse.Code
		logs := broadcastResult.TxResponse.RawLog
		rc.logger.Info().Str("chain_id", rc.chainID).Str("tx_hash", txHash).Uint32("code", broadcastResponseCode).Str("codespace", codespace).Str("logs", logs).Msg("📣 attempted to broadcast restake transaction")

		err = rc.gasManager.ManageBroadcastResult(ctx, rc.chainName, broadcastResult, gasWanted)
		if err != nil {
			rc.logger.Error().Err(err).Str("chain_id", rc.chainID).Msg("failed to manage gas updates for broadcast result")
		}
	}

	// 2.
	if broadcastErr != nil {
		rc.logger.Error().Err(broadcastErr).Str("chain_id", rc.chainID).Msg("failed to manage gas updates for broadcast result")
		return "", broadcastErr
	}

	// 3.
	// NOTE: Presume that if `broadcastErr` is nil (check #2) then broadcastResult is populated. There's fancier and more succinct
	//			 ways to write this code, but the code loses readability fast.
	txHash := broadcastResult.TxResponse.TxHash
	broadcastResponseCode := broadcastResult.TxResponse.Code
	logs := broadcastResult.TxResponse.RawLog
	if broadcastResponseCode != 0 {
		return "", fmt.Errorf(logs)
	}

	// Poll for tx inclusion
	var pollAttempt uint
	for pollAttempt = 0; pollAttempt < rc.txPollAttempts; pollAttempt++ {
		// Return early if the context has expired
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		// Sleep. Do this first so the tx has some time to settle.
		time.Sleep(rc.txPollDelay)

		// Check for confirmation
		included, err := rc.rpcClient.CheckIncluded(ctx, txHash)
		if err != nil {
			rc.logger.Error().Err(err).Str("chain_id", rc.chainID).Str("tx_hash", txHash).Msg("failed to check tx inclusion, will retry")
			continue
		}

		// Return success if included
		if included {
			rc.logger.Info().Str("tx_hash", txHash).Str("chain_id", rc.chainID).Msg("🔍 found included transaction")

			err := rc.gasManager.ManageInclusionResult(ctx, rc.chainName, true)
			if err != nil {
				rc.logger.Error().Err(err).Str("chain_id", rc.chainID).Msg("failed to manage gas updates for tx inclusion")
			}

			// Hurrah!
			return txHash, nil
		}

		rc.logger.Info().Str("tx_hash", txHash).Str("chain_id", rc.chainID).Uint("attempt", pollAttempt).Uint("poll_attempts", rc.txPollAttempts).Msg("⌛ still waiting for inclusion")
	}

	// Create and log an error
	rc.logger.Error().Str("tx_hash", txHash).Str("chain_id", rc.chainID).Uint("poll_attempts", rc.txPollAttempts).Dur("poll_delay", rc.txPollDelay).Msg("restake transaction not included")
	err = rc.gasManager.ManageInclusionResult(ctx, rc.chainName, false)
	if err != nil {
		rc.logger.Error().Err(err).Str("chain_id", rc.chainID).Msg("failed to manage gas updates for tx inclusion")
	}

	return "", fmt.Errorf("polling for tx hash %s on %s failed", txHash, rc.chainID)
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
