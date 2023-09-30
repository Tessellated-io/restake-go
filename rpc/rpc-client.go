package rpc

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
)

// Handles RPCs for Restake
type RpcClient interface {
	BroadcastTxAndWait(ctx context.Context, txBytes []byte) (*txtypes.BroadcastTxResponse, error)

	SimulateTx(ctx context.Context, tx authsigning.Tx, txConfig client.TxConfig, gasFactor float64) (*SimulationResult, error)

	GetAccountData(ctx context.Context, address string) (*AccountData, error)
	GetDelegators(ctx context.Context, validatorAddress string) []string
	GetGrants(ctx context.Context, botAddress string) []*authztypes.GrantAuthorization
	GetPendingRewards(ctx context.Context, delegator, validator, stakingDenom string) sdk.Dec
}
