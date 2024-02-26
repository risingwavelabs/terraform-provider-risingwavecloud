package cloudsdk

import (
	"context"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk/apigen"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/utils/wait"

	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk/apigen/mgmt"
)

var (
	PollingTenantCreation = wait.PollingParams{
		Timeout:  15 * time.Minute,
		Interval: 3 * time.Second,
	}

	PollingTenantDeletion = wait.PollingParams{
		Timeout:  15 * time.Minute,
		Interval: 3 * time.Second,
	}
)

type RegionServiceClientInterface interface {
	/* risingwavecloud_cluster resource */

	// Create a RisingWave cluster and wait for it to be ready.
	CreateClusterAwait(ctx context.Context, req apigen_mgmt.TenantRequestRequestBody) error

	// Delete a RisingWave cluster by its name and wait for it to be ready.
	DeleteClusterAwait(ctx context.Context, name string) error

	// Update the version of a RisingWave cluster by its name.
	UpdateClusterImageAwait(ctx context.Context, name string, version string) error

	// Update the resources of a RisinGWave cluster by its name.
	UpdateClusterResourcesAwait(ctx context.Context, name string, req apigen_mgmt.PostTenantResourcesRequestBody) error
}

var _ RegionServiceClientInterface = &RegionServiceClient{}

type RegionServiceClient struct {
	mgmtClient *apigen_mgmt.ClientWithResponses
}

func (c *RegionServiceClient) WaitCluster(ctx context.Context, name string, target apigen_mgmt.TenantStatus) error {
	var currentStatus apigen_mgmt.TenantStatus
	if err := wait.Poll(ctx, func() (bool, error) {
		cluster, err := c.GetClusterByName(ctx, name)
		if err != nil {
			return false, errors.Wrap(err, "failed to get the cluster info")
		}
		currentStatus = cluster.Status
		return currentStatus == target, nil
	}, PollingTenantCreation); err != nil {
		return errors.Wrapf(err, "failed to wait for the cluster, current status: %s, target status: %s", currentStatus, target)
	}
	return nil
}

func (c *RegionServiceClient) GetClusterByName(ctx context.Context, name string) (*apigen_mgmt.Tenant, error) {
	res, err := c.mgmtClient.GetTenantWithResponse(ctx, &apigen_mgmt.GetTenantParams{
		TenantName: &name,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cluster")
	}
	if apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, "failed to get cluster: %s", err.Error()); err != nil {
		return nil, err
	}
	return res.JSON200, nil
}

func (c *RegionServiceClient) CreateClusterAwait(ctx context.Context, req apigen_mgmt.TenantRequestRequestBody) error {
	// create cluster
	createRes, err := c.mgmtClient.PostTenantsWithResponse(ctx, req)
	if err != nil {
		return errors.Wrap(err, "failed to create cluster")
	}
	if err := apigen.ExpectStatusCodeWithMessage(createRes, http.StatusAccepted, "failed to create cluster: %s", err.Error()); err != nil {
		return err
	}

	// wait for the tenant to be ready
	return c.WaitCluster(ctx, req.TenantName, apigen_mgmt.Running)
}

func (c *RegionServiceClient) DeleteClusterAwait(ctx context.Context, name string) error {
	// delete the cluster
	deleteRes, err := c.mgmtClient.DeleteTenantWithResponse(ctx, &apigen_mgmt.DeleteTenantParams{
		TenantName: &name,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create cluster")
	}
	if err := apigen.ExpectStatusCodeWithMessage(deleteRes, http.StatusAccepted, "failed to create cluster: %s", err.Error()); err != nil {
		return err
	}

	// wait for the tenant to be deleted
	return wait.Poll(ctx, func() (bool, error) {
		getRes, err := c.mgmtClient.GetTenantWithResponse(ctx, &apigen_mgmt.GetTenantParams{
			TenantName: &name,
		})
		if err != nil {
			return false, errors.Wrap(err, "failed to get the latest tenant status")
		}
		return getRes.StatusCode() == http.StatusNotFound, nil
	}, PollingTenantDeletion)
}

func (c *RegionServiceClient) UpdateClusterImageAwait(ctx context.Context, name string, version string) error {
	cluster, err := c.GetClusterByName(ctx, name)
	if err != nil {
		return errors.Wrap(err, "failed to get cluster info")
	}
	// update cluster image
	res, err := c.mgmtClient.PostTenantTenantIdUpdateVersionWithResponse(ctx, cluster.Id, apigen_mgmt.PostTenantTenantIdUpdateVersionJSONRequestBody{
		Version: &version,
	})
	if err != nil {
		return errors.Wrap(err, "failed to udpate cluster image")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, "failed to update cluster image"); err != nil {
		return err
	}

	// wait for the tenant to be ready
	return c.WaitCluster(ctx, name, apigen_mgmt.Running)
}

func (c *RegionServiceClient) UpdateClusterResourcesAwait(ctx context.Context, name string, req apigen_mgmt.PostTenantResourcesRequestBody) error {
	cluster, err := c.GetClusterByName(ctx, name)
	if err != nil {
		return errors.Wrap(err, "failed to get cluster info")
	}
	res, err := c.mgmtClient.PostTenantTenantIdResourceWithResponse(ctx, cluster.Id, req)
	if err != nil {
		return errors.Wrap(err, "failed to udpate cluster resource")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, "failed to update cluster resource"); err != nil {
		return err
	}

	// wait for the tenant resource udpated
	return c.WaitCluster(ctx, name, apigen_mgmt.Running)
}
