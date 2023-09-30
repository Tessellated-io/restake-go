package registry

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tessellated-io/pickaxe/arrays"
)

type RegistryClient struct{}

func NewRegistryClient() *RegistryClient {
	return &RegistryClient{}
}

func (rc *RegistryClient) GetRestakeChains(targetValidator string) ([]Chain, error) {
	validators, err := rc.getValidators()
	if err != nil {
		return nil, err
	}

	validator, err := rc.extractValidator(targetValidator, validators)
	if err != nil {
		return nil, err
	}

	validChains := arrays.Filter(validator.Chains, func(input Chain) bool {
		return input.Restake.Address != ""
	})

	return validChains, nil
}

func (rc *RegistryClient) GetChainInfo(chainName string) (*ChainInfo, error) {
	url := fmt.Sprintf("https://proxy.atomscan.com/directory/%s/chain.json", chainName)
	bytes, err := rc.makeRequest(url)
	if err != nil {
		return nil, err
	}

	chainInfo, err := parseChainResponse(bytes)
	if err != nil {
		return nil, err
	}
	return chainInfo, nil
}

func (rc *RegistryClient) extractValidator(targetValidator string, validators *[]Validator) (*Validator, error) {
	for _, validator := range *validators {
		if strings.EqualFold(targetValidator, validator.Name) {
			return &validator, nil
		}
	}
	return nil, fmt.Errorf("unable to find a validator with name \"%s\"", targetValidator)
}

func (rc *RegistryClient) getValidators() (*[]Validator, error) {
	bytes, err := rc.makeRequest("https://validators.cosmos.directory/")
	if err != nil {
		return nil, err
	}

	response, err := parseRegistryResponse(bytes)
	if err != nil {
		return nil, err
	}
	return &(response.Validators), nil
}

func (rc *RegistryClient) makeRequest(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return data, nil
	} else {
		return nil, fmt.Errorf("received non-OK HTTP status: %d", resp.StatusCode)
	}
}
