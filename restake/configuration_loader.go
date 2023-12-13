package restake

import (
	"fmt"
	"os"
	"time"

	"github.com/tessellated-io/pickaxe/config"
	"gopkg.in/yaml.v2"
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
	HealthChecksPingKey      string   `yaml:"health_checks_ping_key" comment:"A ping API key for healthchecks.io. If empty, no pings will be delivered."`
	RunIntervalSeconds       uint     `yaml:"run_interval_seconds" comment:"How many seconds to wait in between restake runs"`
	BatchSize                uint     `yaml:"batch_size" comment:"What size batches of transactions should be sent in"`
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
	configurationFile string
}

func NewConfigurationLoader(configurationDirectory string) (*configurationLoader, error) {
	configurationFile := config.ExpandHomeDir(fmt.Sprintf("%s/%s", configurationDirectory, RestakeConfigFilename))

	loader := &configurationLoader{
		configurationFile: configurationFile,
	}

	return loader, nil
}

func (cl *configurationLoader) LoadConfiguration() (*Configuration, error) {
	data, err := os.ReadFile(cl.configurationFile)
	if err != nil {
		return nil, err
	}

	loaded := &Configuration{}
	err = yaml.Unmarshal(data, loaded)
	if err != nil {
		return nil, err
	}

	return loaded, nil
}
