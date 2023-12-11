package restake

import (
	"fmt"
	"time"
)

const RestakeConfigFilename = "restake.yaml"

// Configuration is configuration for restake
type Configuration struct {
	TargetValidator          string   `yaml:"target_validator" comment:"The name of the validator to Restake for. Ex. 'Tessellated'"`
	Ignores                  []string `yaml:"ignores" comment:"A list of network names to ignore. Ex. 'cosmoshub' or 'osmosis'"`
	BotMnemonic              string   `yaml:"mnemonic" comment:"The mnemonic to use for Restaking"`
	Memo                     string   `yaml:"memo" comment:"An optional memo to include in Restake transactions"`
	GasFactor                float64  `yaml:"gas_factor" comment:"A factor to multiply gas estimates by"`
	TxPollDelaySeconds       uint     `yaml:"tx_poll_delay_seconds" comment:"How long to delay between attempts to poll for a tx being included in a block"`
	TxPollAttempts           uint     `yaml:"tx_poll_attempts" comment:"How many attempts to poll for a tx being included before failing."`
	NetworkRetryDelaySeconds uint     `yaml:"network_retry_delay_seconds" comment:"How long to delay between retries due to RPC failures"`
	NetworkRetryAttempts     uint     `yaml:"network_retry_attempts" comment:"How many attempts to retry due to network errors before failing."`
	DisableHealthChecks      bool     `yaml:"disable_health_checks" comment:"Whether status should be reported to healthchecks.io"`
	RunIntervalSeconds       uint     `yaml:"run_interval_seconds" comment:"How many seconds to wait in between restake runs"`
}

func (c *Configuration) VersionedMemo(version string) string {
	return fmt.Sprintf("%s | restake-go %s", c.Memo, version)
}

func (c *Configuration) RunInterval() time.Duration {
	return time.Duration(c.RunIntervalSeconds) * time.Second
}

func (c *Configuration) NetworkRetryDelay() time.Duration {
	return time.Duration(c.NetworkRetryDelaySeconds) * time.Second
}

func (c *Configuration) TxPollDelay() time.Duration {
	return time.Duration(c.TxPollDelaySeconds) * time.Second
}

// configurationLoader loads configuration
type configurationLoader struct {
}

func NewConfigurationLoader() (*configurationLoader, error) {
	loader := &configurationLoader{}

	return loader, nil
}

func (cl *configurationLoader) LoadConfiguration() (*Configuration, error) {
	return &Configuration{}, nil
}
