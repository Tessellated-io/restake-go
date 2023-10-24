package config

import "math/big"

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
