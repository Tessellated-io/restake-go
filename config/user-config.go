package config

// UserConfig is the top level config provided by a user of restake
type UserConfig struct {
	Moniker          string            `yaml:"moniker"`
	Mnemonic         string            `yaml:"mnemonic"`
	Memo             string            `yaml:"memo"`
	Chains           []UserChainConfig `yaml:"chains"`
	Ignores          []string          `yaml:"ignores"`
	RunIntervalHours int               `yaml:"runIntervalHours"`
}

// User chain config is the config for a chain provided by a user
type UserChainConfig struct {
	ChainID       string `yaml:"chainID"`
	HealthCheckID string `yaml:"healthCheckId"`
	// TODO: Pull from registry?
	MinRestakeAmount int     `yaml:"minRestakeAmount"`
	Grpc             string  `yaml:"grpc"`
	GasPrice         float64 `yaml:"gasPrice"`
}
