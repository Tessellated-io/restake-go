package registry

import (
	"encoding/json"
)

type Token struct {
	Denom string `json:"denom"`
}

type FeeToken struct {
	Denom string `json:"denom"`

	FixedMinGasPrice float64 `json:"fixed_min_gas_price"`
	LowGasPrice      float64 `json:"low_gas_price"`
	AverageGasPrice  float64 `json:"average_gas_price"`
	HighGasPrice     float64 `json:"high_gas_price"`
}

type Fee struct {
	FeeTokens []FeeToken `json:"fee_tokens"`
}

type Staking struct {
	StakingTokens []Token `json:"staking_tokens"`
}

type Binaries struct {
	LinuxAmd64   string `json:"linux/amd64"`
	LinuxArm64   string `json:"linux/arm64"`
	DarwinAmd64  string `json:"darwin/amd64"`
	DarwinArm64  string `json:"darwin/arm64"`
	WindowsAmd64 string `json:"windows/amd64"`
}

type Genesis struct {
	GenesisURL string `json:"genesis_url"`
}

type Version struct {
	Name               string   `json:"name"`
	RecommendedVersion string   `json:"recommended_version"`
	CompatibleVersions []string `json:"compatible_versions"`
	Binaries           Binaries `json:"binaries"`
}

type Codebase struct {
	GitRepo            string    `json:"git_repo"`
	RecommendedVersion string    `json:"recommended_version"`
	CompatibleVersions []string  `json:"compatible_versions"`
	Binaries           Binaries  `json:"binaries"`
	Genesis            Genesis   `json:"genesis"`
	Versions           []Version `json:"versions"`
}

type LogoURIs struct {
	PNG string `json:"png"`
	SVG string `json:"svg"`
}

type Seed struct {
	ID       string `json:"id"`
	Address  string `json:"address"`
	Provider string `json:"provider,omitempty"`
}

type Peer struct {
	ID       string `json:"id"`
	Address  string `json:"address"`
	Provider string `json:"provider,omitempty"`
}

type Peers struct {
	Seeds           []Seed `json:"seeds"`
	PersistentPeers []Peer `json:"persistent_peers"`
}

type APIAddress struct {
	Address  string `json:"address"`
	Provider string `json:"provider"`
}

type APIs struct {
	RPC  []APIAddress `json:"rpc"`
	Rest []APIAddress `json:"rest"`
	GRPC []APIAddress `json:"grpc"`
}

type Explorer struct {
	Kind        string `json:"kind"`
	URL         string `json:"url"`
	TxPage      string `json:"tx_page"`
	AccountPage string `json:"account_page,omitempty"`
}

type ChainInfo struct {
	ChainName    string     `json:"chain_name"`
	Status       string     `json:"status"`
	NetworkType  string     `json:"network_type"`
	Website      string     `json:"website"`
	PrettyName   string     `json:"pretty_name"`
	ChainID      string     `json:"chain_id"`
	Bech32Prefix string     `json:"bech32_prefix"`
	DaemonName   string     `json:"daemon_name"`
	NodeHome     string     `json:"node_home"`
	KeyAlgos     []string   `json:"key_algos"`
	Slip44       int        `json:"slip44"`
	Fees         Fee        `json:"fees"`
	Staking      Staking    `json:"staking"`
	Codebase     Codebase   `json:"codebase"`
	LogoURIs     LogoURIs   `json:"logo_URIs"`
	Peers        Peers      `json:"peers"`
	APIs         APIs       `json:"apis"`
	Explorers    []Explorer `json:"explorers"`
}

func parseChainResponse(responseBytes []byte) (*ChainInfo, error) {
	// Unmarshal the JSON data into the ChainInfo struct
	var chainInfo ChainInfo
	err := json.Unmarshal(responseBytes, &chainInfo)
	if err != nil {
		return nil, err
	}
	return &chainInfo, nil
}
