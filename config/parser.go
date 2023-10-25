package config

import (
	"os"

	"gopkg.in/yaml.v2"
)

// Parse a file into a UserConfig
func parseConfig(filename string) (*UserConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	config := &UserConfig{}
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}
