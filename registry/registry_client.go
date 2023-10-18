package registry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	retry "github.com/avast/retry-go/v4"
	"github.com/tessellated-io/pickaxe/arrays"
)

type RegistryClient struct {
	attempts retry.Option
	delay    retry.Option
}

func NewRegistryClient() *RegistryClient {
	return &RegistryClient{
		attempts: retry.Attempts(5),
		delay:    retry.Delay(1 * time.Second),
	}
}

func (rc *RegistryClient) GetRestakeChains(ctx context.Context, targetValidator string) ([]Chain, error) {
	var chains []Chain
	var err error

	err = retry.Do(func() error {
		chains, err = rc.getRestakeChains(ctx, targetValidator)
		return err
	}, rc.delay, rc.attempts, retry.Context(ctx))
	if err != nil {
		err = errors.Unwrap(err)
	}

	return chains, err
}

// Internal method without retries
func (rc *RegistryClient) getRestakeChains(ctx context.Context, targetValidator string) ([]Chain, error) {
	validators, err := rc.getValidatorsWithRetries(ctx)
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

func (rc *RegistryClient) GetChainInfo(ctx context.Context, chainName string) (*ChainInfo, error) {
	var chainInfo *ChainInfo
	var err error

	err = retry.Do(func() error {
		chainInfo, err = rc.getChainInfo(ctx, chainName)
		return err
	}, rc.delay, rc.attempts, retry.Context(ctx))
	if err != nil {
		err = errors.Unwrap(err)
	}

	return chainInfo, err
}

// Internal method without retries
func (rc *RegistryClient) getChainInfo(ctx context.Context, chainName string) (*ChainInfo, error) {
	url := fmt.Sprintf("https://proxy.atomscan.com/directory/%s/chain.json", chainName)
	bytes, err := rc.makeRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	chainInfo, err := parseChainResponse(bytes)
	if err != nil {
		return nil, err
	}
	return chainInfo, nil
}

func (rc *RegistryClient) extractValidator(targetValidator string, validators []Validator) (*Validator, error) {
	for _, validator := range validators {
		if strings.EqualFold(targetValidator, validator.Name) {
			return &validator, nil
		}
	}
	return nil, fmt.Errorf("unable to find a validator with name \"%s\"", targetValidator)
}

func (rc *RegistryClient) getValidatorsWithRetries(ctx context.Context) ([]Validator, error) {
	var validators []Validator
	var err error

	err = retry.Do(func() error {
		validators, err = rc.getValidators(ctx)
		return err
	}, rc.delay, rc.attempts, retry.Context(ctx))
	if err != nil {
		err = errors.Unwrap(err)
	}

	return validators, err
}

func (rc *RegistryClient) getValidators(ctx context.Context) ([]Validator, error) {
	bytes, err := rc.makeRequest(ctx, "https://validators.cosmos.directory/")
	if err != nil {
		return nil, err
	}

	response, err := parseRegistryResponse(bytes)
	if err != nil {
		return nil, err
	}
	return response.Validators, nil
}

func (rc *RegistryClient) makeRequest(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		log.Fatalf("Error making request: %v", err)
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
