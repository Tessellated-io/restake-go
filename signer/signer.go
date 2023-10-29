package signer

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/tessellated-io/pickaxe/cosmos/rpc"
	"github.com/tessellated-io/pickaxe/crypto"
	"github.com/tessellated-io/pickaxe/log"

	"github.com/cosmos/cosmos-sdk/client"
	cosmostx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	txauth "github.com/cosmos/cosmos-sdk/x/auth/tx"
)

const feeIncrement float64 = 0.01

type Signer struct {
	cdc *codec.ProtoCodec

	rpcClient rpc.RpcClient

	feeDenom  string
	gasFactor float64
	gasPrice  float64

	chainID       string
	addressPrefix string
	memo          string

	bytesSigner crypto.BytesSigner

	log *log.Logger
}

func NewSigner(
	gasFactor float64,
	cdc *codec.ProtoCodec,
	chainID string,
	bytesSigner crypto.BytesSigner,
	gasPrice float64,
	addressPrefix string,
	rpcClient rpc.RpcClient,
	memo string,
	feeDenom string,
	log *log.Logger,
) *Signer {
	return &Signer{
		cdc: cdc,

		rpcClient: rpcClient,

		feeDenom:  feeDenom,
		gasFactor: gasFactor,
		gasPrice:  gasPrice,

		chainID:       chainID,
		addressPrefix: addressPrefix,
		memo:          memo,

		bytesSigner: bytesSigner,

		log: log,
	}
}

func (s *Signer) SendMessages(
	ctx context.Context,
	msgs []sdk.Msg,
) (string, error) {
	var err error

	// Attempt to send a transaction a few times.
	for i := 0; i < 5; i++ {
		// Attempt to broadcast
		var result *txtypes.BroadcastTxResponse
		var gasWanted uint64
		result, gasWanted, err = s.sendMessages(ctx, msgs)

		// Compose for logging
		gasPrice := fmt.Sprintf("%f%s", s.gasPrice, s.feeDenom)

		// Give up if we get an error attempting to broadcast.
		if err != nil {
			s.log.Error().Err(err).Str("gas_price", gasPrice).Float64("gas_factor", s.gasFactor).Msg("Error broadcasting transaction")
			continue
		}

		// Extract codes
		code := result.TxResponse.Code
		logs := result.TxResponse.RawLog

		if code == 0 {
			// Code 0 indicates a successful broadcast.
			txHash := result.TxResponse.TxHash

			// Wait for the transaction to hit the chain.
			pollDelay := 30 * time.Second
			s.log.Info().Str("tx_hash", txHash).Str("gas_price", gasPrice).Float64("gas_factor", s.gasFactor).Msg("💤 Transaction sent, waiting for inclusion...")
			time.Sleep(pollDelay)

			// 1. Try to get a confirmation on the first try. If not, increass the gas.
			// Check that it confirmed.
			// If it failed, that likely means we should use more gas.
			err := s.rpcClient.CheckConfirmed(ctx, txHash)
			if err == nil {
				// Hurrah, things worked out!
				s.log.Info().Str("tx_hash", txHash).Str("gas_price", gasPrice).Float64("gas_factor", s.gasFactor).Msg("✅ Transaction confirmed. Success.")
				return txHash, nil
			}

			// 2a. If the tx broadcast did not error, but it hasn't landed, then we can likely affor more in gas.
			s.gasPrice += feeIncrement
			newGasPrice := fmt.Sprintf("%f%s", s.gasPrice, s.feeDenom)
			s.log.Info().Str("new_gas_price", newGasPrice).Float64("gas_factor", s.gasFactor).Msg(fmt.Sprintf("⛽ Transaction broadcasted but failed to confirm. Likely need more gas. Increasing gas price. Code: %d, Logs: %s", code, logs))

			// Failing for gas seems silly, so let's go ahead and retry.
			i--

			// 3. Eventually it might show up though, so to prevent nonce conflicts go ahead and search
			maxPollAttempts := 10
			for j := 0; j < maxPollAttempts; j++ {
				// Sleep and wait
				time.Sleep(pollDelay)
				s.log.Info().Str("tx_hash", txHash).Str("gas_price", gasPrice).Float64("gas_factor", s.gasFactor).Int("attempt", j).Int("max_attempts", maxPollAttempts).Msg("still waiting for tx to land")

				// Re-poll
				err = s.rpcClient.CheckConfirmed(ctx, txHash)
				if err == nil {
					// Hurrah, things worked out!
					s.log.Info().Str("tx_hash", txHash).Str("gas_price", gasPrice).Float64("gas_factor", s.gasFactor).Msg("✅ Transaction confirmed. Success.")
					return txHash, nil
				}
			}
		} else if code == 13 {
			// Code 13 indicates too little gas

			// First, figure out if the network told us the minimum fee
			maybeNewMinFee, err := s.extractMinGlobalFee(logs)
			if err == nil {
				s.gasPrice += feeIncrement
				s.gasPrice = float64(maybeNewMinFee/int(gasWanted)) + 1
				newGasPrice := fmt.Sprintf("%f%s", s.gasPrice, s.feeDenom)
				s.log.Info().Str("new_gas_price", newGasPrice).Float64("gas_factor", s.gasFactor).Msg("⛽ Adjusting gas price due to a minimum global fee error")
				continue
			} else {
				// Otherwise, use normal increment logic.
				s.gasPrice += feeIncrement
				newGasPrice := fmt.Sprintf("%f%s", s.gasPrice, s.feeDenom)
				s.log.Info().Str("new_gas_price", newGasPrice).Float64("gas_factor", s.gasFactor).Msg(fmt.Sprintf("⛽ Transaction failed to broadcast with gas error. Likely need more gas. Increasing gas price. Code: %d, Logs: %s", code, logs))
			}

			// Failing for gas seems silly, so let's go ahead and retry.
			i--
		} else if code != 0 {
			s.log.Info().Str("gas_price", gasPrice).Float64("gas_factor", s.gasFactor).Msg(fmt.Sprintf("⚠️ Failed to apply transaction batch after broadcast. Code %d, Logs: %s", code, logs))
			time.Sleep(5 * time.Second)
			continue
		}
	}

	// Log that we're giving up and what price we gave up at.
	gasPrice := fmt.Sprintf("%f%s", s.gasPrice, s.feeDenom)
	s.log.Info().Str("gas_price", gasPrice).Float64("gas_factor", s.gasFactor).Msg("❌ failed in all attempts to broadcast transaction")

	if err != nil {
		// All tries exhausted, give up and return an error.
		return "", fmt.Errorf("error broadcasting tx: %s", err.Error())
	} else {
		// Broadcasted but could not confirm
		return "", fmt.Errorf("tx broadcasted but was never confirmed. need higher gas prices?")
	}
}

// Try to broadcast a transaction containing the given messages.
// Returns:
// - a broadcast tx response if it was broadcasted successfully
// - a gas estimate if one was able to be made
// - an error if broadcasting couldn't occur
func (s *Signer) sendMessages(
	ctx context.Context,
	msgs []sdk.Msg,
) (*txtypes.BroadcastTxResponse, uint64, error) {
	// Get account data
	address := s.bytesSigner.GetAddress(s.addressPrefix)
	accountData, err := s.rpcClient.GetAccountData(ctx, address)
	if err != nil {
		s.log.Error().Err(err).Str("signer address", s.bytesSigner.GetAddress(s.addressPrefix)).Msg("Error getting account data")
		return nil, 0, err
	}

	// Start building a tx
	txConfig := txauth.NewTxConfig(s.cdc, txauth.DefaultSignModes)
	factory := cosmostx.Factory{}.WithChainID(s.chainID).WithTxConfig(txConfig)
	txb, err := factory.BuildUnsignedTx(msgs...)
	if err != nil {
		return nil, 0, err
	}

	txb.SetMemo(s.memo)

	signatureProto := signing.SignatureV2{
		PubKey: s.bytesSigner.GetPublicKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  signing.SignMode_SIGN_MODE_DIRECT,
			Signature: nil,
		},
		Sequence: accountData.Sequence,
	}
	err = txb.SetSignatures(signatureProto)
	if err != nil {
		return nil, 0, err
	}

	// Simulate the tx
	simulationResult, err := s.rpcClient.SimulateTx(ctx, txb.GetTx(), txConfig, s.gasFactor)
	if err != nil {
		return nil, 0, err
	}
	txb.SetGasLimit(simulationResult.GasRecommendation)

	fee := []sdk.Coin{
		{
			Denom:  s.feeDenom,
			Amount: sdk.NewInt(int64(s.gasPrice*float64(simulationResult.GasRecommendation)) + 1),
		},
	}
	txb.SetFeeAmount(fee)

	// Sign the tx
	signedTx, err := s.signTx(txb, accountData, txConfig)
	if err != nil {
		return nil, 0, err
	}

	response, err := s.rpcClient.Broadcast(ctx, signedTx)
	return response, simulationResult.GasRecommendation, err
}

func (s *Signer) signTx(
	txb client.TxBuilder,
	accountData *rpc.AccountData,
	txConfig client.TxConfig,
) ([]byte, error) {
	// Form signing data
	signerData := authsigning.SignerData{
		ChainID:       s.chainID,
		Sequence:      accountData.Sequence,
		AccountNumber: accountData.AccountNumber,
	}

	// Encode to bytes to sign
	signMode := signing.SignMode_SIGN_MODE_DIRECT
	unsignedTxBytes, err := txConfig.SignModeHandler().GetSignBytes(signMode, signerData, txb.GetTx())
	if err != nil {
		return []byte{}, err
	}

	// Sign the bytes
	signatureBytes, err := s.bytesSigner.SignBytes(unsignedTxBytes)
	if err != nil {
		return []byte{}, err
	}

	// Reconstruct the signature proto
	signatureData := &signing.SingleSignatureData{
		SignMode:  signMode,
		Signature: signatureBytes,
	}
	signatureProto := signing.SignatureV2{
		PubKey:   s.bytesSigner.GetPublicKey(),
		Data:     signatureData,
		Sequence: accountData.Sequence,
	}
	err = txb.SetSignatures(signatureProto)
	if err != nil {
		return []byte{}, err
	}

	// Encode to bytes
	encoder := txConfig.TxEncoder()
	return encoder(txb.GetTx())
}

// extractMinGlobalFee is useful for evmos, or other EVMs in the Tendermint space
func (s *Signer) extractMinGlobalFee(errMsg string) (int, error) {
	// Regular expression to match the desired number
	pattern := `(\d+)\w+\)\. Please increase`
	re := regexp.MustCompile(pattern)

	matches := re.FindStringSubmatch(errMsg)
	if len(matches) > 1 {
		converted, err := strconv.Atoi(matches[1])
		if err != nil {
			s.log.Error().Err(err).Msg("Found a matching min global fee error, but failed to atoi it")
			return 0, nil
		}
		return converted, nil

	}
	return 0, fmt.Errorf("unrecognized error format")
}
