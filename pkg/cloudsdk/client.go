package cloudsdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/httpx"
)

const (
	PollingTenantCreationTimeout  = 15 * time.Minute
	PollingTenantCreationInterval = 3 * time.Second

	PollingTenantDeletionTimeout  = 15 * time.Minute
	PollingTenantDeletionInterval = 3 * time.Second
)

type CloudClientInterface interface {

	/* risingwavecloud_cluster resource */

	// Create a RisingWave cluster.
	CreateCluster(context.Context, *CreateClusterPayload) error
	// Delete a RisingWave cluster by its name.
	DeleteCluster(ctx context.Context, name string) error
	// Update the version of a RisingWave cluster by its name.
	UpdateClusterImage(ctx context.Context, name string, version string) error
	// Update the resources of a RisinGWave cluster by its name.
	UpdateClusterResources(ctx context.Context, name string) error

	/* risingwave_component_type data source */

	// Get all component types available.
	GetComponentTypes(ctx context.Context) error

	/* Others */

	// Check the connection of the endpoint and validate the API key provided.
	Ping(context.Context) error
}

var _ CloudClientInterface = &CloudClient{}

type ClusterModel struct {
	Name       string
	Platform   string
	Region     string
	Version    string
	ResourceV1 ResourceV1
}

type ResourceV1 struct {
	Compute                ComponentTypeV1
	Compactor              ComponentTypeV1
	Frontend               ComponentTypeV1
	Etcd                   ComponentTypeV1
	Meta                   ComponentTypeV1
	EtcdDiskSizeGB         int64
	ComputeFileCacheSizeGB int64
}

type ComponentTypeV1 struct {
	Type    string
	Replica int64
}

type CloudClient struct {
	Endpoint   string
	APIKey     string
	httpClient *httpx.HTTPClient
}

func NewCloudClient(endpoint, apiKey string) *CloudClient {
	httpClient := httpx.NewHTTPClient(endpoint)
	httpClient.SetHeader("Authentication", "Bearer "+apiKey)

	return &CloudClient{
		Endpoint:   endpoint,
		APIKey:     apiKey,
		httpClient: httpClient,
	}
}

func (c *CloudClient) Ping(ctx context.Context) error {
	res, err := c.httpClient.Get(ctx, "/auth/ping").Do()
	if err != nil {
		return errors.Wrap(err, "failed to ping endpoint")
	}
	return res.ExpectStatusWithMessage("failed to connect to the RisingWave Cloud control plane", http.StatusOK)
}

type CreateClusterPayload struct {
	TenantName string                        `json:"tenantName"`
	Resources  *CreateClusterResourcePayload `json:"resources"`
}

type CreateClusterResourcePayload struct {
	EtcdVolumnesSizeGiB     int                        `json:"etcdVolumeSizeGiB"`
	EnableComputeFileCache  bool                       `json:"enableComputeFileCache"`
	ComputeFileCacheSizeGiB int                        `json:"computeFileCacheSizeGiB"`
	Components              *ResourceComponentsPayload `json:"components"`
}

type ResourceComponentsPayload struct {
	Compute   *ComponentTypePayload `json:"compute"`
	Compactor *ComponentTypePayload `json:"compactor"`
	Frontend  *ComponentTypePayload `json:"frontend"`
	Etcd      *ComponentTypePayload `json:"etcd"`
	Meta      *ComponentTypePayload `json:"meta"`
}

type ComponentTypePayload struct {
	Replica         int    `json:"replica"`
	ComponentTypeID string `json:"componentTypeId"`
}

func (c *CloudClient) CreateCluster(ctx context.Context, payload *CreateClusterPayload) error {
	// create cluster
	res, err := c.httpClient.
		Post(ctx, "/tenants").
		WithJSON(payload).
		Do()
	if err != nil {
		return errors.Wrap(err, "failed to create cluster")
	}
	if err := res.ExpectStatusWithMessage("failed to create cluster", http.StatusAccepted); err != nil {
		return err
	}

	// wait for the tenant to be ready
	err = c.httpClient.
		Get(ctx, "/tenant").
		WithQuery("tenantName", payload.TenantName).
		Poll(
			func(rh *httpx.ResponseHelper) (bool, error) {
				if rh.StatusCode == http.StatusOK {
					return true, nil
				}
				if rh.StatusCode == http.StatusNotFound {
					return false, nil
				}
				return false, fmt.Errorf("unexpected status code: %d", rh.StatusCode)
			},
			PollingTenantCreationInterval,
			PollingTenantCreationTimeout,
		)
	if err != nil {
		return errors.Wrap(err, "failed to wait for the cluster ready")
	}
	return nil
}

func (c *CloudClient) DeleteCluster(ctx context.Context, name string) error {
	// delete the cluster
	res, err := c.httpClient.
		Delete(ctx, "/tenant").
		WithQuery("tenantName", name).
		Do()
	if err != nil {
		return errors.Wrap(err, "failed to delete cluster")
	}
	if err := res.ExpectStatusWithMessage("failed to delete cluster", http.StatusAccepted); err != nil {
		return err
	}

	// wait for the tenant to be deleted
	err = c.httpClient.
		Get(ctx, "/tenant").
		WithQuery("tenantName", name).
		Poll(
			func(rh *httpx.ResponseHelper) (bool, error) {
				if rh.StatusCode == http.StatusOK {
					return false, nil
				}
				if rh.StatusCode == http.StatusNotFound {
					return true, nil
				}
				return false, fmt.Errorf("unexpected status code: %d", rh.StatusCode)
			},
			PollingTenantDeletionInterval,
			PollingTenantDeletionTimeout,
		)
	if err != nil {
		return errors.Wrap(err, "failed to wait for the cluster ready")
	}

	return nil
}

func (c *CloudClient) UpdateClusterImage(ctx context.Context, name string, version string) error {
	//
	return errors.New("Unimplemented")
}

func (c *CloudClient) UpdateClusterResources(ctx context.Context, name string) error {
	return errors.New("Unimplemented")
}

func (c *CloudClient) GetComponentTypes(ctx context.Context) error {
	return errors.New("Unimplemented")
}
