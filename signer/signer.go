package signer

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/restake-go/log"
	"github.com/restake-go/rpc"
	"github.com/tessellated-io/pickaxe/crypto"

	"github.com/cosmos/cosmos-sdk/client"
	cosmostx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	txauth "github.com/cosmos/cosmos-sdk/x/auth/tx"
)

// TODO: Support increasing fees

// TODO: Determine what this should be?
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
) error {
	var err error

	// Try to send a few times
	for i := 0; i < 5; i++ {
		// TODO: if the tx broadcast but did not confirm, let's increase gas then try agin. otherwise, try again.
		// TODO: After fixing above, make sure that we're not throwing retries into the rpc client
		// TODO: Clean up this text
		// TODO: mess with these return codes from RPC Client
		var result *txtypes.BroadcastTxResponse
		var gasWanted uint64
		result, gasWanted, err = s.sendMessages(ctx, msgs)
		if err != nil {
			s.log.Error().Err(err).Msg("Error broadcasting transaction")
			continue
		}

		code := result.TxResponse.Code
		logs := result.TxResponse.RawLog
		if code == 13 {
			maybeNewMinFee, err := s.extractMinGlobalFee(logs)
			if err == nil {
				s.log.Info().Msg("Adjusting gas price due to Evmos/EVM error")
				s.gasPrice = float64(maybeNewMinFee/int(gasWanted)) + 1
				continue
			} else {
				s.log.Info().Msg(fmt.Sprintf("Need more gas, increasing gas price. Code: %d, Logs: %s", code, logs))
				s.gasPrice += feeIncrement
				time.Sleep(30 * time.Second)
				continue
			}
		} else if code != 0 {
			s.log.Info().Msg(fmt.Sprintf("Failed to apply transaction batch after broadcast. Code %d, Logs: %s", code, logs))
			time.Sleep(30 * time.Second)
			continue
		}

		hash := result.TxResponse.TxHash
		s.log.Info().Str("tx hash", hash).Msg("Transaction sent and included in block")
		return nil
	}
	return fmt.Errorf("error broadcasting tx: %s", err.Error())
}

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
		panic(err)
	}

	result, err := s.rpcClient.BroadcastTxAndWait(ctx, signedTx)
	return result, simulationResult.GasRecommendation, err
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
	pattern := `provided fee < minimum global fee \((\d+)aevmos < (\d+)aevmos\). Please increase the gas price.: insufficient fee`
	re := regexp.MustCompile(pattern)

	matches := re.FindStringSubmatch(errMsg)
	if len(matches) == 0 && len(matches) > 2 {
		converted, err := strconv.Atoi(matches[2])
		if err != nil {
			s.log.Error().Err(err).Msg("Found a matching eth / evmos error, but failed to atoi it")
			return 0, nil
		}
		return converted, nil
	}

	return 0, fmt.Errorf("unrecognized error format")
}
