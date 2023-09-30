package rpc

type AccountData struct {
	Address       string
	AccountNumber uint64
	Sequence      uint64
}

type SimulationResult struct {
	GasRecommendation uint64
}
