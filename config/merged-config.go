package config

import (
	cregistry "github.com/tessellated-io/pickaxe/cosmos/chain-registry"
	"github.com/tessellated-io/restake-go/registry"
)

// MergedConfig is a merged object of all configs are gathered in the configuration step, prior to creating the final Restake Config object
type MergedConfig struct {
	// Config from the user in the config file
	*UserChainConfig

	// Config from the Chain registry about a chain
	*cregistry.ChainInfo

	// Config about Restake on the chain from the Restake registry
	registry.RestakeInfo
}
