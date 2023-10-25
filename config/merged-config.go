package config

import "github.com/tessellated-io/restake-go/registry"

// MergedConfig is a merged object of all configs are gathered in the configuration step, prior to creating the final Restake Config object
type MergedConfig struct {
	// Config from the user in the config file
	*UserChainConfig

	// Config from the Chain registry about a chain
	*registry.ChainInfo

	// Config about Restake on the chain from the Restake registry
	*registry.RestakeInfo
}
