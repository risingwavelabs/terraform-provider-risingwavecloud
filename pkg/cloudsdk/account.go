package cloudsdk

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pkg/errors"

	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk/apigen"
	apigen_acc "github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk/apigen/acc"
	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk/apigen/mgmt"
)

var (
	ErrInvalidCredential = errors.New("invalid credential")
)

type JSON = map[string]any

type AccountServiceClientInterface interface {
	// Check the connection of the endpoint and validate the API key provided.
	Ping(context.Context) error

	GetRegionServiceClient(platform, region string) (RegionServiceClientInterface, error)
}

type AccountServiceClient struct {
	Endpoint string

	accClient *apigen_acc.ClientWithResponses

	apiKeyPair string

	// platform -> region -> info
	regions map[string]map[string]apigen_acc.Region
}

func NewAccountServiceClient(ctx context.Context, endpoint, apiKey, apiSecret string) (AccountServiceClientInterface, error) {
	apiKeyPair := fmt.Sprintf("%s:%s", apiKey, apiSecret)
	accClient, err := apigen_acc.NewClientWithResponses(endpoint, apigen_acc.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		req.Header.Set("X-API-KEY", apiKeyPair)
		return nil
	}))
	if err != nil {
		return nil, err
	}

	// get regions
	res, err := accClient.GetRegionsWithResponse(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get regions")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, "failed to get regions"); err != nil {
		return nil, err
	}
	if res.JSON200 == nil {
		return nil, errors.New("unexpected error, region array is nil")
	}
	regions := *res.JSON200
	if len(regions) == 0 {
		return nil, errors.New("unexpected error, region array is empty")
	}

	regionMap := make(map[string]map[string]apigen_acc.Region)
	for _, region := range regions {
		if regionMap[region.Platform] == nil {
			regionMap[region.Platform] = make(map[string]apigen_acc.Region)
		}
		regionMap[region.Platform][region.RegionName] = region
	}

	return &AccountServiceClient{
		Endpoint:   endpoint,
		accClient:  accClient,
		regions:    regionMap,
		apiKeyPair: apiKeyPair,
	}, nil
}

func (c *AccountServiceClient) Ping(ctx context.Context) error {
	res, err := c.accClient.GetAuthPingWithResponse(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to ping endpoint")
	}
	if res.StatusCode() == http.StatusForbidden {
		return ErrInvalidCredential
	}
	return apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, "failed to ping endpoint")
}

func (c *AccountServiceClient) GetRegionServiceClient(platform, region string) (RegionServiceClientInterface, error) {
	regionInfo := c.regions[platform][region]
	mgmtClient, err := apigen_mgmt.NewClientWithResponses(regionInfo.Url, apigen_mgmt.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		req.Header.Set("X-API-KEY", c.apiKeyPair)
		return nil
	}))
	if err != nil {
		return nil, err
	}
	return &RegionServiceClient{
		mgmtClient,
	}, nil
}
