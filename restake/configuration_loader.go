package restake

import (
	"fmt"
	"os"
	"time"

	file "github.com/tessellated-io/pickaxe/config"
	"github.com/tessellated-io/pickaxe/log"
	"gopkg.in/yaml.v2"
)

const RestakeConfigFilename = "restake.yml"

// Configuration is configuration for restake
type Configuration struct {
	TargetValidator          string   `yaml:"target_validator" comment:"The name of the validator to Restake for. Ex. 'Tessellated'"`
	Ignores                  []string `yaml:"ignores" comment:"A list of network names to ignore. Ex. 'cosmoshub' or 'osmosis'"`
	BotMnemonic              string   `yaml:"mnemonic" comment:"The mnemonic to use for Restaking"`
	Memo                     string   `yaml:"memo" comment:"An optional memo to include in Restake transactions"`
	TxPollDelaySeconds       uint     `yaml:"tx_poll_delay_seconds" comment:"How long to delay between attempts to poll for a tx being included in a block"`
	TxPollAttempts           uint     `yaml:"tx_poll_attempts" comment:"How many attempts to poll for a tx being included before failing."`
	NetworkRetryDelaySeconds uint     `yaml:"network_retry_delay_seconds" comment:"How long to delay between retries due to RPC failures"`
	NetworkRetryAttempts     uint     `yaml:"network_retry_attempts" comment:"How many attempts to retry due to network errors before failing."`
	HealthChecksPingKey      string   `yaml:"health_checks_ping_key" comment:"A ping API key for healthchecks.io. If empty, no pings will be delivered."`
	RunIntervalSeconds       uint     `yaml:"run_interval_seconds" comment:"How many seconds to wait in between restake runs"`
	BatchSize                uint     `yaml:"batch_size" comment:"What size batches of transactions should be sent in"`
	ChainRegistryBaseUrl     string   `yaml:"chain_registry_base_url" comment:"The base url for the chain registry"`
	ValidatorRegistryBaseUrl string   `yaml:"validator_registry_base_url" comment:"The base url for the validator registry"`
	MarkEmptyRestakeAsFailed bool     `yaml:"mark_empty_as_failed" comment:"If true and there are no valid restake messages, the run will be marked as a failure. This is helpful in case the restake website will mark you as offline."`
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
	configurationDirectory string
	logger                 *log.Logger
}

func NewConfigurationLoader(configurationDirectory string, logger *log.Logger) (*configurationLoader, error) {
	loader := &configurationLoader{
		configurationDirectory: configurationDirectory,
		logger:                 logger,
	}

	return loader, nil
}

func (cl *configurationLoader) LoadConfiguration() (*Configuration, error) {
	configurationFile := cl.getConfigFile()

	data, err := os.ReadFile(configurationFile)
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

func (cl *configurationLoader) Initialize() error {
	// Example config
	config := &Configuration{}

	configFile := cl.getConfigFile()
	header := "This is the configuration file for Treasurer"

	err := file.WriteYamlWithComments(config, header, configFile, cl.logger)
	if err != nil {
		cl.logger.Error("error writing file", "error", err.Error())
		return err
	}

	return nil
}

func (cl *configurationLoader) getConfigFile() string {
	return fmt.Sprintf("%s/%s", cl.configurationDirectory, RestakeConfigFilename)
}
