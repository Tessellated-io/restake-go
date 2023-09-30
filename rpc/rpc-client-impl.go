package rpc

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/restake-go/sleep"
	"github.com/tessellated-io/pickaxe/arrays"
	"github.com/tessellated-io/pickaxe/grpc"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// Page size to use
const pageSize = 100

// rpcClientImpl is the private and default implementation for restake.
type rpcClientImpl struct {
	cdc *codec.ProtoCodec

	authClient         authtypes.QueryClient
	authzClient        authztypes.QueryClient
	distributionClient distributiontypes.QueryClient
	stakingClient      stakingtypes.QueryClient
	txClient           txtypes.ServiceClient
}

// A struct that came back from an RPC query
type paginatedRpcResponse[dataType any] struct {
	data    []dataType
	nextKey []byte
}

// Ensure that rpcClientImpl implements RpcClient
var _ RpcClient = (*rpcClientImpl)(nil)

// NewRpcClient makes a new RpcClient for the Restake Go App.
func NewRpcClient(nodeGrpcUri string, cdc *codec.ProtoCodec) (RpcClient, error) {
	conn, err := grpc.GetGrpcConnection(nodeGrpcUri)
	if err != nil {
		fmt.Printf("Unable to connect to gRPC at URL %s: %s\n", nodeGrpcUri, err)
		return nil, err
	}

	authClient := authtypes.NewQueryClient(conn)
	authzClient := authztypes.NewQueryClient(conn)
	distributionClient := distributiontypes.NewQueryClient(conn)
	stakingClient := stakingtypes.NewQueryClient(conn)
	txClient := txtypes.NewServiceClient(conn)

	return &rpcClientImpl{
		cdc: cdc,

		authClient:         authClient,
		authzClient:        authzClient,
		distributionClient: distributionClient,
		stakingClient:      stakingClient,
		txClient:           txClient,
	}, nil
}

func (r *rpcClientImpl) GetPendingRewards(ctx context.Context, delegator, validator, stakingDenom string) sdk.Dec {
	fmt.Printf("...Fetching pending rewards for %s\n", delegator)

	request := &distributiontypes.QueryDelegationTotalRewardsRequest{
		DelegatorAddress: delegator,
	}
	for {
		response, err := r.distributionClient.DelegationTotalRewards(ctx, request)
		if err != nil {
			fmt.Printf("Error fetching balances for %s: %s\n", delegator, err)
			sleep.Sleep()
			continue
		}

		for _, reward := range response.Rewards {
			if strings.EqualFold(validator, reward.ValidatorAddress) {
				for _, coin := range reward.Reward {
					if strings.EqualFold(coin.Denom, stakingDenom) {
						return coin.Amount
					}
				}
			}
		}

		fmt.Printf("Failed to find a reward denom matching %s for validator %s \n", stakingDenom, validator)
		return sdk.NewDec(0)
	}
}

func (r *rpcClientImpl) BroadcastTxAndWait(
	ctx context.Context,
	txBytes []byte,
) (*txtypes.BroadcastTxResponse, error) {
	// Form a query
	query := &txtypes.BroadcastTxRequest{
		Mode:    txtypes.BroadcastMode_BROADCAST_MODE_SYNC,
		TxBytes: txBytes,
	}

	// Send tx
	response, err := r.txClient.BroadcastTx(
		ctx,
		query,
	)
	if err != nil {
		return nil, err
	}

	// If successful, return
	if response.TxResponse.Code == 0 {
		r.waitForConfirmation(ctx, response.TxResponse.TxHash)
	}
	return response, nil
}

func (r *rpcClientImpl) waitForConfirmation(ctx context.Context, txHash string) {
	fmt.Printf("Polling for inclusion of tx %s\n", txHash)
	for {
		time.Sleep(5 * time.Second)
		status, err := r.getTxStatus(ctx, txHash)
		if err != nil {
			fmt.Printf("Error querying tx status: %s\n", err)
			continue
		} else {
			height := status.TxResponse.Height
			if height != 0 {
				fmt.Printf("Transaction with hash %s confirmed at height %d\n", txHash, height)
				return
			} else {
				fmt.Printf("Transaction still not confirmed, still waiting...\n")
			}
		}
	}
}

func (r *rpcClientImpl) getTxStatus(ctx context.Context, txHash string) (*txtypes.GetTxResponse, error) {
	request := &txtypes.GetTxRequest{Hash: txHash}
	return r.txClient.GetTx(ctx, request)
}

func (r *rpcClientImpl) GetAccountData(ctx context.Context, address string) (*AccountData, error) {
	// Make a query
	query := &authtypes.QueryAccountRequest{Address: address}
	res, err := r.authClient.Account(
		ctx,
		query,
	)
	if err != nil {
		return nil, err
	}

	// Deserialize response
	var account authtypes.AccountI
	if err := r.cdc.UnpackAny(res.Account, &account); err != nil {
		return nil, err
	}

	return &AccountData{
		Address:       address,
		AccountNumber: account.GetAccountNumber(),
		Sequence:      account.GetSequence(),
	}, nil
}

func (r *rpcClientImpl) SimulateTx(
	ctx context.Context,
	tx authsigning.Tx,
	txConfig client.TxConfig,
	gasFactor float64,
) (*SimulationResult, error) {
	// Form a query
	encoder := txConfig.TxEncoder()
	txBytes, err := encoder(tx)
	if err != nil {
		return nil, err
	}

	query := &txtypes.SimulateRequest{
		TxBytes: txBytes,
	}
	simulationResponse, err := r.txClient.Simulate(ctx, query)
	if err != nil {
		return nil, err
	}

	return &SimulationResult{
		GasRecommendation: uint64(math.Ceil(float64(simulationResponse.GasInfo.GasUsed) * gasFactor)),
	}, nil
}

func (r *rpcClientImpl) GetGrants(ctx context.Context, botAddress string) []*authztypes.GrantAuthorization {
	fmt.Printf("...Fetching grants for bot %s\n", botAddress)

	getGrantsFunc := func(ctx context.Context, pageKey []byte) (*paginatedRpcResponse[*authztypes.GrantAuthorization], error) {
		pagination := &query.PageRequest{
			Key:   pageKey,
			Limit: pageSize,
		}

		request := &authztypes.QueryGranteeGrantsRequest{
			Grantee:    botAddress,
			Pagination: pagination,
		}

		response, err := r.authzClient.GranteeGrants(ctx, request)
		if err != nil {
			return nil, err
		}

		return &paginatedRpcResponse[*authztypes.GrantAuthorization]{
			data:    response.Grants,
			nextKey: response.Pagination.NextKey,
		}, nil
	}

	grants := retrievePaginatedData(ctx, r, "grants", getGrantsFunc)
	fmt.Printf("	...Retrieved %d grants for %s\n", len(grants), botAddress)

	return grants
}

func (r *rpcClientImpl) GetDelegators(ctx context.Context, validatorAddress string) []string {
	transformFunc := func(input stakingtypes.DelegationResponse) string { return input.Delegation.DelegatorAddress }

	fetchDelegatorPageFunc := func(ctx context.Context, pageKey []byte) (*paginatedRpcResponse[string], error) {
		pagination := &query.PageRequest{
			Key:   pageKey,
			Limit: pageSize,
		}

		request := &stakingtypes.QueryValidatorDelegationsRequest{
			ValidatorAddr: validatorAddress,
			Pagination:    pagination,
		}
		response, err := r.stakingClient.ValidatorDelegations(ctx, request)
		if err != nil {
			return nil, err
		}
		delegators := arrays.Map(response.DelegationResponses, transformFunc)

		return &paginatedRpcResponse[string]{
			data:    delegators,
			nextKey: response.Pagination.NextKey,
		}, nil
	}

	delegators := retrievePaginatedData(ctx, r, "delegations", fetchDelegatorPageFunc)
	fmt.Printf("	...Retrieved %d delegations for %s\n", len(delegators), validatorAddress)

	return delegators
}

// Pagination
// NOTE: Implemented as a private standalone func since go doesn't seem to support generics on struct methods.

func retrievePaginatedData[DataType any](
	ctx context.Context,
	r *rpcClientImpl,
	noun string,
	retrievePageFn func(
		ctx context.Context,
		nextKey []byte,
	) (*paginatedRpcResponse[DataType], error),
) []DataType {
	// Running list of data
	data := []DataType{}

	// Loop through all pages
	var nextKey []byte
	for {
		// Query the page, ditching if we fail to retry
		rpcResponse, err := retrievePageFn(ctx, nextKey)
		if err != nil {
			fmt.Printf("Error fetching %s: %s\n", noun, err)
			sleep.Sleep()
			continue
		}

		// Append the data
		data = append(data, rpcResponse.data...)
		fmt.Printf("	...Fetched a page of %d %s (total: %d)\n", len(rpcResponse.data), noun, len(data))

		// Update next key or break out of loop if we have finished
		if len(rpcResponse.nextKey) == 0 {
			break
		}
		nextKey = rpcResponse.nextKey
	}

	return data
}
