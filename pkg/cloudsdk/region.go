package cloudsdk

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk/apigen"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/utils/wait"

	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk/apigen/mgmt"
)

var (
	ErrClusterNotFound = errors.New("cluster not found")
)

const (
	ComponentCompute   = "compute"
	ComponentCompactor = "compactor"
	ComponentFrontend  = "frontend"
	ComponentMeta      = "meta"
	ComponentEtcd      = "etcd"
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
	GetClusterByID(ctx context.Context, id uint64) (*apigen_mgmt.Tenant, error)

	IsTenantNameExist(ctx context.Context, tenantName string) (bool, error)

	CreateClusterAwait(ctx context.Context, req apigen_mgmt.TenantRequestRequestBody) (*apigen_mgmt.Tenant, error)

	DeleteClusterAwait(ctx context.Context, id uint64) error

	UpdateClusterImageAwait(ctx context.Context, id uint64, version string) error

	UpdateClusterResourcesAwait(ctx context.Context, id uint64, req apigen_mgmt.PostTenantResourcesRequestBody) error

	GetTiers(ctx context.Context) ([]apigen_mgmt.Tier, error)

	GetAvailableComponentTypes(ctx context.Context, targetTier apigen_mgmt.TierId, component string) ([]apigen_mgmt.AvailableComponentType, error)

	UpdateRisingWaveConfigAwait(ctx context.Context, id uint64, rwConfig string) error

	UpdateEtcdConfigAwait(ctx context.Context, id uint64, etcdConfig string) error
}

type RegionServiceClient struct {
	mgmtClient *apigen_mgmt.ClientWithResponses
}

func (c *RegionServiceClient) IsTenantNameExist(ctx context.Context, tenantName string) (bool, error) {
	_, err := c.getClusterByName(ctx, tenantName)
	if err != nil {
		if err == ErrClusterNotFound {
			return false, nil
		}
		return false, errors.Wrap(err, "failed to get cluster info")
	}
	return true, nil
}

func (c *RegionServiceClient) waitClusterByID(ctx context.Context, id uint64, target apigen_mgmt.TenantStatus) error {
	var currentStatus apigen_mgmt.TenantStatus
	if err := wait.Poll(ctx, func() (bool, error) {
		cluster, err := c.GetClusterByID(ctx, id)
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

// this is used only when the cluster ID is unknown.
func (c *RegionServiceClient) waitClusterByName(ctx context.Context, name string, target apigen_mgmt.TenantStatus) error {
	var currentStatus apigen_mgmt.TenantStatus
	if err := wait.Poll(ctx, func() (bool, error) {
		cluster, err := c.getClusterByName(ctx, name)
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

// this is used only when the cluster ID is unknown.
func (c *RegionServiceClient) getClusterByName(ctx context.Context, name string) (*apigen_mgmt.Tenant, error) {
	res, err := c.mgmtClient.GetTenantWithResponse(ctx, &apigen_mgmt.GetTenantParams{
		TenantName: &name,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed call API to to get cluster")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, ErrClusterNotFound
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK); err != nil {
		return nil, err
	}
	return res.JSON200, nil
}

func (c *RegionServiceClient) GetClusterByID(ctx context.Context, id uint64) (*apigen_mgmt.Tenant, error) {
	res, err := c.mgmtClient.GetTenantWithResponse(ctx, &apigen_mgmt.GetTenantParams{
		TenantId: &id,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed call API to to get cluster")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, ErrClusterNotFound
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK); err != nil {
		return nil, errors.Wrapf(err, "message %s", string(res.Body))
	}
	return res.JSON200, nil
}

func (c *RegionServiceClient) CreateClusterAwait(ctx context.Context, req apigen_mgmt.TenantRequestRequestBody) (*apigen_mgmt.Tenant, error) {
	// create cluster
	createRes, err := c.mgmtClient.PostTenantsWithResponse(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed call API to to create cluster")
	}
	if err := apigen.ExpectStatusCodeWithMessage(createRes, http.StatusAccepted); err != nil {
		return nil, errors.Wrapf(err, "message %s", string(createRes.Body))
	}

	// wait for the tenant to be ready
	if err := c.waitClusterByName(ctx, req.TenantName, apigen_mgmt.Running); err != nil {
		return nil, err
	}

	cluster, err := c.getClusterByName(ctx, req.TenantName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cluster info")
	}
	return cluster, nil
}

func (c *RegionServiceClient) DeleteClusterAwait(ctx context.Context, id uint64) error {
	// delete the cluster
	deleteRes, err := c.mgmtClient.DeleteTenantWithResponse(ctx, &apigen_mgmt.DeleteTenantParams{
		TenantId: &id,
	})
	if err != nil {
		return errors.Wrap(err, "failed call API to to delete cluster")
	}
	if deleteRes.StatusCode() == http.StatusNotFound {
		return nil
	}
	if err := apigen.ExpectStatusCodeWithMessage(deleteRes, http.StatusAccepted); err != nil {
		return errors.Wrapf(err, "message %s", string(deleteRes.Body))
	}

	// wait for the tenant to be deleted
	return wait.Poll(ctx, func() (bool, error) {
		getRes, err := c.mgmtClient.GetTenantWithResponse(ctx, &apigen_mgmt.GetTenantParams{
			TenantId: &id,
		})
		if err != nil {
			return false, errors.Wrap(err, "failed to call API to get the latest tenant status")
		}
		return getRes.StatusCode() == http.StatusNotFound, nil
	}, PollingTenantDeletion)
}

func (c *RegionServiceClient) UpdateClusterImageAwait(ctx context.Context, id uint64, version string) error {
	cluster, err := c.GetClusterByID(ctx, id)
	if err != nil {
		return errors.Wrap(err, "failed to get cluster info")
	}
	// update cluster image
	res, err := c.mgmtClient.PostTenantTenantIdUpdateVersionWithResponse(ctx, cluster.Id, apigen_mgmt.PostTenantTenantIdUpdateVersionJSONRequestBody{
		Version: &version,
	})
	if err != nil {
		return errors.Wrap(err, "failed to call API to udpate cluster image")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted); err != nil {
		return errors.Wrapf(err, "message %s", string(res.Body))
	}

	// wait for the tenant to be ready
	return c.waitClusterByID(ctx, id, apigen_mgmt.Running)
}

func (c *RegionServiceClient) UpdateClusterResourcesAwait(ctx context.Context, id uint64, req apigen_mgmt.PostTenantResourcesRequestBody) error {
	cluster, err := c.GetClusterByID(ctx, id)
	if err != nil {
		return errors.Wrap(err, "failed to get cluster info")
	}
	res, err := c.mgmtClient.PostTenantTenantIdResourceWithResponse(ctx, cluster.Id, req)
	if err != nil {
		return errors.Wrap(err, "failed to call API to udpate cluster resource")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted); err != nil {
		return errors.Wrapf(err, "message %s", string(res.Body))
	}

	// wait for the tenant resource udpated
	return c.waitClusterByID(ctx, id, apigen_mgmt.Running)
}

func (c *RegionServiceClient) GetTiers(ctx context.Context) ([]apigen_mgmt.Tier, error) {
	res, err := c.mgmtClient.GetTiersWithResponse(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to retrieve information of all tiers")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK); err != nil {
		return nil, errors.Wrapf(err, "message %s", string(res.Body))
	}

	return res.JSON200.Tiers, nil
}

func (c *RegionServiceClient) GetAvailableComponentTypes(ctx context.Context, targetTier apigen_mgmt.TierId, component string) ([]apigen_mgmt.AvailableComponentType, error) {
	tiers, err := c.GetTiers(ctx)
	if err != nil {
		return nil, err
	}
	var tier *apigen_mgmt.Tier
	for _, t := range tiers {
		if t.Id == nil {
			continue
		}
		if *t.Id == targetTier {
			tier = &t
			break
		}
	}
	if tier == nil {
		return nil, errors.Errorf("tier %s not found", targetTier)
	}
	switch component {
	case ComponentCompute:
		return tier.AvailableComputeNodes, nil
	case ComponentCompactor:
		return tier.AvailableCompactorNodes, nil
	case ComponentFrontend:
		return tier.AvailableFrontendNodes, nil
	case ComponentMeta:
		return tier.AvailableMetaNodes, nil
	case ComponentEtcd:
		return tier.AvailableEtcdNodes, nil
	}
	return nil, errors.Errorf("component %s not found", component)
}

func (c *RegionServiceClient) UpdateRisingWaveConfigAwait(ctx context.Context, id uint64, rwConfig string) error {
	cluster, err := c.GetClusterByID(ctx, id)
	if err != nil {
		return errors.Wrap(err, "failed to get cluster info")
	}
	res, err := c.mgmtClient.PutTenantTenantIdConfigRisingwaveWithBodyWithResponse(ctx, cluster.Id, "text/plain", strings.NewReader(rwConfig))
	if err != nil {
		return errors.Wrap(err, "failed to call API to update cluster config")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted); err != nil {
		return errors.Wrapf(err, "message %s", string(res.Body))
	}

	// wait for the tenant to be ready
	return c.waitClusterByID(ctx, id, apigen_mgmt.Running)
}

func (c *RegionServiceClient) UpdateEtcdConfigAwait(ctx context.Context, id uint64, etcdConfig string) error {
	cluster, err := c.GetClusterByID(ctx, id)
	if err != nil {
		return errors.Wrap(err, "failed to get cluster info")
	}
	res, err := c.mgmtClient.PutTenantTenantIdConfigEtcdWithBodyWithResponse(ctx, cluster.Id, "text/plain", strings.NewReader(etcdConfig))
	if err != nil {
		return errors.Wrap(err, "failed to call API to update cluster config")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted); err != nil {
		return errors.Wrapf(err, "message %s", string(res.Body))
	}

	// wait for the tenant to be ready
	return c.waitClusterByID(ctx, id, apigen_mgmt.Running)
}
