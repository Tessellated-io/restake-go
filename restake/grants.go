package restake

import (
	"time"

	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
)

// Docs on unpacking any: https://docs.cosmos.network/main/core/encoding.html#interface-encoding-and-usage-of-any
// NOTE: The above doesn't seem to work, perhaps I need to register a codec somewhere. I gave up since using a type-url seems good enough.

func isValidGrant(grant *authztypes.GrantAuthorization) bool {
	// Ensure grant hasn't expired
	expiration := grant.Expiration
	now := time.Now()
	if expiration.Before(now) {
		return false
	}

	if grant.Authorization.TypeUrl == "/cosmos.staking.v1beta1.StakeAuthorization" {
		return true
	}

	if grant.Authorization.TypeUrl == "/cosmos.authz.v1beta1.GenericAuthorization" {
		return true
	}

	return false
}
