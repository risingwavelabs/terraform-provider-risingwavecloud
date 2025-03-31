package cloudsdk

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/ptr"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/wait"

	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt"
)

var (
	ErrClusterNotFound     = errors.New("cluster not found")
	ErrClusterUserNotFound = errors.New("cluster user not found")
	ErrPrivateLinkNotFound = errors.New("private link not found")
)

const (
	ComponentCompute    = "compute"
	ComponentCompactor  = "compactor"
	ComponentFrontend   = "frontend"
	ComponentMeta       = "meta"
	ComponentEtcd       = "etcd"
	ComponentPostgresql = "postgresql"
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

	PollingPrivateLinkCreation = wait.PollingParams{
		Timeout:  5 * time.Minute,
		Interval: 3 * time.Second,
	}

	PollingPrivateLinkDeletion = wait.PollingParams{
		Timeout:  5 * time.Minute,
		Interval: 3 * time.Second,
	}
)

type RegionServiceClientInterface interface {
	GetClusterByName(ctx context.Context, name string) (*apigen_mgmt.Tenant, error)

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

	GetClusterUsers(ctx context.Context, id uint64) ([]apigen_mgmt.DBUser, error)

	CreateCluserUser(ctx context.Context, params apigen_mgmt.CreateDBUserRequestBody) (*apigen_mgmt.DBUser, error)

	UpdateClusterUserPassword(ctx context.Context, id uint64, username, password string) error

	DeleteClusterUser(ctx context.Context, id uint64, username string) error

	GetPrivateLink(ctx context.Context, id uint64, privateLinkID uuid.UUID) (*apigen_mgmt.PrivateLink, error)

	CreatePrivateLinkAwait(ctx context.Context, id uint64, req apigen_mgmt.PostPrivateLinkRequestBody) (*apigen_mgmt.PrivateLink, error)

	DeletePrivateLinkAwait(ctx context.Context, id uint64, privateLinkID uuid.UUID) error
}

type RegionServiceClient struct {
	mgmtClient *apigen_mgmt.ClientWithResponses
}

func (c *RegionServiceClient) IsTenantNameExist(ctx context.Context, tenantName string) (bool, error) {
	_, err := c.GetClusterByName(ctx, tenantName)
	if err != nil {
		if errors.Is(err, ErrClusterNotFound) {
			return false, nil
		}
		return false, errors.Wrap(err, "failed to get cluster info")
	}
	return true, nil
}

func (c *RegionServiceClient) waitClusterRunning(ctx context.Context, id uint64) error {
	var currentStatus apigen_mgmt.TenantStatus
	if err := wait.Poll(ctx, func() (bool, error) {
		cluster, err := c.GetClusterByID(ctx, id)
		if err != nil {
			return false, errors.Wrap(err, "failed to get the cluster info")
		}
		currentStatus = cluster.Status
		return currentStatus == apigen_mgmt.Running, nil
	}, PollingTenantCreation); err != nil {
		return errors.Wrapf(err, "failed to wait for the cluster, current status: %s, target status: %s", currentStatus, apigen_mgmt.Running)
	}
	return nil
}

// this is used only when the cluster ID is unknown.
func (c *RegionServiceClient) waitClusterStatusByName(ctx context.Context, name string, target apigen_mgmt.TenantStatus) error {
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

// this is used only when the cluster ID is unknown.
func (c *RegionServiceClient) waitClusterHealthStatusByName(ctx context.Context, name string, target apigen_mgmt.TenantHealthStatus) error {
	var currentStatus apigen_mgmt.TenantHealthStatus
	if err := wait.Poll(ctx, func() (bool, error) {
		cluster, err := c.GetClusterByName(ctx, name)
		if err != nil {
			return false, errors.Wrap(err, "failed to get the cluster info")
		}
		currentStatus = cluster.HealthStatus
		return currentStatus == target, nil
	}, PollingTenantCreation); err != nil {
		return errors.Wrapf(err, "failed to wait for the cluster, current health status: %s, target health status: %s", currentStatus, target)
	}
	return nil
}

// this is used only when the cluster ID is unknown.
func (c *RegionServiceClient) GetClusterByName(ctx context.Context, name string) (*apigen_mgmt.Tenant, error) {
	res, err := c.mgmtClient.GetTenantWithResponse(ctx, &apigen_mgmt.GetTenantParams{
		TenantName: &name,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed call API to to get cluster")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, errors.Wrapf(ErrClusterNotFound, "cluster %s not found", name)
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
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
		return nil, errors.Wrapf(ErrClusterNotFound, "cluster %d not found", id)
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return nil, err
	}
	return res.JSON200, nil
}

func (c *RegionServiceClient) CreateClusterAwait(ctx context.Context, req apigen_mgmt.TenantRequestRequestBody) (*apigen_mgmt.Tenant, error) {
	// create cluster
	createRes, err := c.mgmtClient.PostTenantsWithResponse(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed call API to to create cluster")
	}
	if err := apigen.ExpectStatusCodeWithMessage(createRes, http.StatusAccepted, string(createRes.Body)); err != nil {
		return nil, err
	}

	// wait for the tenant to be ready
	if err := c.waitClusterStatusByName(ctx, req.TenantName, apigen_mgmt.Running); err != nil {
		return nil, err
	}
	if err := c.waitClusterHealthStatusByName(ctx, req.TenantName, apigen_mgmt.Healthy); err != nil {
		return nil, err
	}

	cluster, err := c.GetClusterByName(ctx, req.TenantName)
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
	if err := apigen.ExpectStatusCodeWithMessage(deleteRes, http.StatusAccepted, string(deleteRes.Body)); err != nil {
		return err
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
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return err
	}

	// wait for the tenant to be ready
	return c.waitClusterRunning(ctx, id)
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
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return err
	}

	// wait for the tenant resource updated.
	return c.waitClusterRunning(ctx, id)
}

func (c *RegionServiceClient) GetTiers(ctx context.Context) ([]apigen_mgmt.Tier, error) {
	res, err := c.mgmtClient.GetTiersWithResponse(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to retrieve information of all tiers")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return nil, err
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
			tier = ptr.Ptr(t)
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
		return tier.AvailableMetaStore.Etcd.Nodes, nil
	case ComponentPostgresql:
		return tier.AvailableMetaStore.Postgresql.Nodes, nil
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
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return err
	}

	// wait for the tenant to be ready
	return c.waitClusterRunning(ctx, id)
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
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return err
	}

	// wait for the tenant to be ready
	return c.waitClusterRunning(ctx, id)
}

func (c *RegionServiceClient) GetClusterUsers(ctx context.Context, id uint64) ([]apigen_mgmt.DBUser, error) {
	res, err := c.mgmtClient.GetTenantDbusersWithResponse(ctx, &apigen_mgmt.GetTenantDbusersParams{
		TenantId: id,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to get cluster user")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, errors.Wrapf(ErrClusterNotFound, "cluster %d not found", id)
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return nil, err
	}
	var rtn []apigen_mgmt.DBUser
	if res.JSON200.Dbusers != nil {
		rtn = *res.JSON200.Dbusers
	}
	return rtn, nil
}

func (c *RegionServiceClient) CreateCluserUser(ctx context.Context, params apigen_mgmt.CreateDBUserRequestBody) (*apigen_mgmt.DBUser, error) {
	res, err := c.mgmtClient.PostTenantDbusersWithResponse(ctx, params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to create cluster user")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, errors.Wrapf(ErrClusterNotFound, "cluster %d not found", params.TenantId)
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return nil, err
	}
	return res.JSON200, nil
}

func (c *RegionServiceClient) UpdateClusterUserPassword(ctx context.Context, id uint64, username, password string) error {
	res, err := c.mgmtClient.PutTenantDbusersWithResponse(ctx, apigen_mgmt.UpdateDBUserRequestBody{
		TenantId: id,
		Username: username,
		Password: password,
	})
	if err != nil {
		return errors.Wrap(err, "failed to call API to update cluster user password")
	}
	return apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body))
}

func (c *RegionServiceClient) DeleteClusterUser(ctx context.Context, id uint64, username string) error {
	res, err := c.mgmtClient.DeleteTenantDbusersWithResponse(ctx, &apigen_mgmt.DeleteTenantDbusersParams{
		TenantId: id,
		Username: username,
	})
	if err != nil {
		return errors.Wrap(err, "failed to call API to delete cluster user")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil
	}
	return apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body))
}

func (c *RegionServiceClient) GetPrivateLink(ctx context.Context, id uint64, privateLinkID uuid.UUID) (*apigen_mgmt.PrivateLink, error) {
	res, err := c.mgmtClient.GetTenantTenantIdPrivatelinkPrivateLinkIdWithResponse(ctx, id, privateLinkID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to get private link")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, ErrPrivateLinkNotFound
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return nil, err
	}
	return res.JSON200, nil
}

func (c *RegionServiceClient) CreatePrivateLinkAwait(ctx context.Context, id uint64, req apigen_mgmt.PostPrivateLinkRequestBody) (*apigen_mgmt.PrivateLink, error) {
	res, err := c.mgmtClient.PostTenantTenantIdPrivatelinksWithResponse(ctx, id, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to create private link")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return nil, err
	}
	var info = res.JSON202
	var rtn *apigen_mgmt.PrivateLink
	err = wait.Poll(ctx, func() (bool, error) {
		link, err := c.GetPrivateLink(ctx, id, info.Id)
		if err != nil {
			if errors.Is(err, ErrPrivateLinkNotFound) {
				return false, nil
			}
			return false, err
		}
		rtn = link
		if link.Status == apigen_mgmt.CREATED {
			return true, nil
		}
		return false, nil
	}, PollingPrivateLinkCreation)

	if err != nil {
		lastStatus := "<nil>"
		if rtn != nil {
			lastStatus = string(rtn.Status)
		}
		return nil, errors.Wrapf(err, "failed to wait for the private link to be created, last status is %s", lastStatus)
	}
	return rtn, nil
}

func (c *RegionServiceClient) DeletePrivateLinkAwait(ctx context.Context, id uint64, privateLinkID uuid.UUID) error {
	res, err := c.mgmtClient.DeleteTenantTenantIdPrivatelinkPrivateLinkIdWithResponse(ctx, id, privateLinkID)
	if err != nil {
		return errors.Wrap(err, "failed to call API to delete private link")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return err
	}
	return wait.Poll(ctx, func() (bool, error) {
		_, err := c.GetPrivateLink(ctx, id, privateLinkID)
		if err != nil {
			if errors.Is(err, ErrPrivateLinkNotFound) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}, PollingPrivateLinkDeletion)
}
