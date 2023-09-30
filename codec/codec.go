package codec

import (
	"sync"

	cryptocodec "github.com/evmos/evmos/v14/crypto/codec"
	"github.com/evmos/evmos/v14/types"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// Provides a singleton codec that can be used across the application

// Mutex to initialize the codec exactly once
var initRelayerOnce sync.Once

// The codec
var cdc *codec.ProtoCodec = nil

func GetCodec() *codec.ProtoCodec {
	initRelayerOnce.Do(func() {
		interfaceRegistry := codectypes.NewInterfaceRegistry()

		authtypes.RegisterInterfaces(interfaceRegistry)
		cryptotypes.RegisterInterfaces(interfaceRegistry)
		stakingtypes.RegisterInterfaces(interfaceRegistry)
		cryptocodec.RegisterInterfaces(interfaceRegistry)
		types.RegisterInterfaces(interfaceRegistry)

		cdc = codec.NewProtoCodec(interfaceRegistry)
	})

	return cdc
}
