package config

import "math/big"

// Top level config for Restake
type RestakeConfig struct {
	Memo             string
	Mnemonic         string
	RunIntervalHours int
	GasMultiplier    float64

	Chains []*ChainConfig
}

// A chain config for Restake to run on a chain
type ChainConfig struct {
	network            string
	HealthcheckId      string
	ValidatorAddress   string
	FeeDenom           string
	ExpectedBotAddress string
	MinRestakeAmount   *big.Int
	AddressPrefix      string
	chainID            string
	nodeGrpcURI        string
	GasPrice           float64

	CoinType int
}

func (cc *ChainConfig) Network() string {
	return cc.network
}

func (cc *ChainConfig) ChainID() string {
	return cc.chainID
}

func (cc *ChainConfig) ChainGrpcURI() string {
	return cc.nodeGrpcURI
}
