package config

import (
	"os"

	"gopkg.in/yaml.v2"
)

type RestakeChain struct {
	Name             string  `yaml:"name"`
	HealthCheckID    string  `yaml:"healthCheckId"`
	MinRestakeAmount int     `yaml:"minRestakeAmount"`
	Grpc             string  `yaml:"grpc"`
	GasPrice         float64 `yaml:"gasPrice"`
}

type Config struct {
	Moniker  string         `yaml:"moniker"`
	Mnemonic string         `yaml:"mnemonic"`
	Memo     string         `yaml:"memo"`
	Chains   []RestakeChain `yaml:"chains"`
	Ignores  []string       `yaml:"ignores"`
	SleepTimeHours int `yaml:"sleepTime"`
}

func parseConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	config := &Config{}
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}
