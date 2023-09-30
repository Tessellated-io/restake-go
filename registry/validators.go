package registry

import (
	"encoding/json"
)

type Delegations struct {
	TotalTokens        string  `json:"total_tokens"`
	TotalCount         int     `json:"total_count"`
	TotalTokensDisplay float64 `json:"total_tokens_display"`
	TotalUSD           float64 `json:"total_usd"`
}

type Description struct {
	Moniker         string `json:"moniker"`
	Identity        string `json:"identity"`
	Website         string `json:"website"`
	SecurityContact string `json:"security_contact"`
	Details         string `json:"details"`
}

type Commission struct {
	Rate float64 `json:"rate"`
}

type Slashes struct {
	ValidatorPeriod string `json:"validator_period"`
	Fraction        string `json:"fraction"`
}

type Restake struct {
	Address string `json:"address"`
}

type MissedBlocksPeriods struct {
	Blocks int `json:"blocks"`
	Missed int `json:"missed"`
}

type Chain struct {
	Name        string      `json:"name"`
	Restake     Restake     `json:"restake"`
	Moniker     string      `json:"moniker"`
	Identity    string      `json:"identity"`
	Address     string      `json:"address"`
	Active      bool        `json:"active"`
	Jailed      bool        `json:"jailed"`
	Status      string      `json:"status"`
	Delegations Delegations `json:"delegations"`
	Description Description `json:"description"`
	Commission  Commission  `json:"commission"`
	Rank        int         `json:"rank"`
	Slashes     []Slashes   `json:"slashes"`
	Image       string      `json:"image"`
}

type Validator struct {
	Path       string  `json:"path"`
	Name       string  `json:"name"`
	Identity   string  `json:"identity"`
	TotalUSD   float64 `json:"total_usd"`
	TotalUsers int     `json:"total_users"`
	Chains     []Chain `json:"chains"`
}

type Response struct {
	Validators []Validator `json:"validators"`
}

func parseRegistryResponse(responseBytes []byte) (*Response, error) {
	// Unmarshal JSON data
	var response Response
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
