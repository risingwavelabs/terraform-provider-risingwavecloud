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

	apigen_mgmtv1 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v1"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
)

var (
	ErrClusterNotFound     = errors.New("cluster not found")
	ErrBYOCClusterNotFound = errors.New("BYOC cluster not found")
	ErrClusterUserNotFound = errors.New("cluster user not found")
	ErrPrivateLinkNotFound = errors.New("private link not found")
)

const (
	ComponentCompute    = "compute"
	ComponentCompactor  = "compactor"
	ComponentFrontend   = "frontend"
	ComponentMeta       = "meta"
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
	GetClusterByNsID(ctx context.Context, nsID uuid.UUID) (*apigen_mgmtv2.Tenant, error)

	GetClusterByName(ctx context.Context, name string) (*apigen_mgmtv1.Tenant, error)

	CreateClusterAwait(ctx context.Context, req apigen_mgmtv2.TenantRequestRequestBody) (*apigen_mgmtv2.Tenant, error)

	DeleteClusterAwait(ctx context.Context, nsID uuid.UUID) error

	UpdateClusterImageAwait(ctx context.Context, nsID uuid.UUID, version string) error

	UpdateClusterResourcesAwait(ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.PostTenantResourcesRequestBody) error

	GetTiers(ctx context.Context) ([]apigen_mgmtv1.Tier, error)

	GetAvailableComponentTypes(ctx context.Context, targetTier apigen_mgmtv1.TierId, component string) ([]apigen_mgmtv1.AvailableComponentType, error)

	UpdateRisingWaveConfigAwait(ctx context.Context, nsID uuid.UUID, rwConfig string) error

	GetClusterUsers(ctx context.Context, nsID uuid.UUID) ([]apigen_mgmtv2.DBUser, error)

	CreateCluserUser(ctx context.Context, nsID uuid.UUID, params apigen_mgmtv2.CreateDBUserRequestBody) (*apigen_mgmtv2.DBUser, error)

	UpdateClusterUserPassword(ctx context.Context, nsID uuid.UUID, username, password string) error

	DeleteClusterUser(ctx context.Context, nsID uuid.UUID, username string) error

	GetPrivateLink(ctx context.Context, nsID, privateLinkID uuid.UUID) (*apigen_mgmtv2.PrivateLink, error)

	CreatePrivateLinkAwait(ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.PostPrivateLinkRequestBody) (*apigen_mgmtv2.PrivateLink, error)

	DeletePrivateLinkAwait(ctx context.Context, nsID, privateLinkID uuid.UUID) error

	GetBYOCCluster(ctx context.Context, name string) (*apigen_mgmtv2.ManagedCluster, error)
}

type RegionServiceClient struct {
	mgmtV1Client *apigen_mgmtv1.ClientWithResponses
	mgmtV2Client *apigen_mgmtv2.ClientWithResponses
}

func (c *RegionServiceClient) IsTenantExist(ctx context.Context, nsID uuid.UUID) (bool, error) {
	_, err := c.GetClusterByNsID(ctx, nsID)
	if err != nil {
		if errors.Is(err, ErrClusterNotFound) {
			return false, nil
		}
		return false, errors.Wrap(err, "failed to get cluster info")
	}
	return true, nil
}

func (c *RegionServiceClient) waitClusterHealthy(ctx context.Context, nsID uuid.UUID) error {
	var currHealth apigen_mgmtv2.TenantHealthStatus
	if err := wait.Poll(ctx, func() (bool, error) {
		cluster, err := c.GetClusterByNsID(ctx, nsID)
		if err != nil {
			return false, errors.Wrap(err, "failed to get the cluster info")
		}
		currHealth = cluster.HealthStatus
		return currHealth == apigen_mgmtv2.TenantHealthStatusHealthy, nil
	}, PollingTenantCreation); err != nil {
		return errors.Wrapf(err, "failed to wait for the cluster, current health status: %s, target health status: %s", currHealth, apigen_mgmtv2.TenantHealthStatusHealthy)
	}
	return nil
}

// this is used only when the cluster ID is unknown.
func (c *RegionServiceClient) waitClusterStatusByNsID(ctx context.Context, nsID uuid.UUID, target apigen_mgmtv2.TenantStatus) error {
	var currentStatus apigen_mgmtv2.TenantStatus
	if err := wait.Poll(ctx, func() (bool, error) {
		cluster, err := c.GetClusterByNsID(ctx, nsID)
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
func (c *RegionServiceClient) waitClusterHealthStatusByNsID(ctx context.Context, nsID uuid.UUID, target apigen_mgmtv2.TenantHealthStatus) error {
	var currentStatus apigen_mgmtv2.TenantHealthStatus
	if err := wait.Poll(ctx, func() (bool, error) {
		cluster, err := c.GetClusterByNsID(ctx, nsID)
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
func (c *RegionServiceClient) GetClusterByNsID(ctx context.Context, nsID uuid.UUID) (*apigen_mgmtv2.Tenant, error) {
	res, err := c.mgmtV2Client.GetTenantsNsIdWithResponse(ctx, nsID)
	if err != nil {
		return nil, errors.Wrap(err, "failed call API to to get cluster")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, errors.Wrapf(ErrClusterNotFound, "cluster %s not found", nsID.String())
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return nil, err
	}
	return res.JSON200, nil
}

func (c *RegionServiceClient) GetClusterByName(ctx context.Context, name string) (*apigen_mgmtv1.Tenant, error) {
	res, err := c.mgmtV1Client.GetTenantWithResponse(ctx, &apigen_mgmtv1.GetTenantParams{
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

func (c *RegionServiceClient) CreateClusterAwait(ctx context.Context, req apigen_mgmtv2.TenantRequestRequestBody) (*apigen_mgmtv2.Tenant, error) {
	// create cluster
	createRes, err := c.mgmtV2Client.PostTenantsWithResponse(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed call API to to create cluster")
	}
	if err := apigen.ExpectStatusCodeWithMessage(createRes, http.StatusAccepted, string(createRes.Body)); err != nil {
		return nil, err
	}

	// wait for the tenant to be ready
	if err := c.waitClusterStatusByNsID(ctx, createRes.JSON202.NsId, apigen_mgmtv2.Running); err != nil {
		return nil, err
	}
	if err := c.waitClusterHealthStatusByNsID(ctx, createRes.JSON202.NsId, apigen_mgmtv2.TenantHealthStatusHealthy); err != nil {
		return nil, err
	}

	cluster, err := c.GetClusterByNsID(ctx, createRes.JSON202.NsId)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cluster info")
	}
	return cluster, nil
}

func (c *RegionServiceClient) DeleteClusterAwait(ctx context.Context, nsID uuid.UUID) error {
	// delete the cluster
	deleteRes, err := c.mgmtV2Client.DeleteTenantsNsIdWithResponse(ctx, nsID)
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
		getRes, err := c.mgmtV2Client.GetTenantsNsIdWithResponse(ctx, nsID)
		if err != nil {
			return false, errors.Wrap(err, "failed to call API to get the latest tenant status")
		}
		return getRes.StatusCode() == http.StatusNotFound, nil
	}, PollingTenantDeletion)
}

func (c *RegionServiceClient) UpdateClusterImageAwait(ctx context.Context, nsID uuid.UUID, version string) error {
	cluster, err := c.GetClusterByNsID(ctx, nsID)
	if err != nil {
		return errors.Wrap(err, "failed to get cluster info")
	}
	// update cluster image
	res, err := c.mgmtV2Client.PostTenantsNsIdUpdateVersionWithResponse(ctx, cluster.NsId, apigen_mgmtv2.PostTenantsNsIdUpdateVersionJSONRequestBody{
		Version: &version,
	})
	if err != nil {
		return errors.Wrap(err, "failed to call API to udpate cluster image")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return err
	}

	// wait for the tenant to be ready
	return c.waitClusterHealthy(ctx, cluster.NsId)
}

func (c *RegionServiceClient) UpdateClusterResourcesAwait(ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.PostTenantResourcesRequestBody) error {
	cluster, err := c.GetClusterByNsID(ctx, nsID)
	if err != nil {
		return errors.Wrap(err, "failed to get cluster info")
	}
	res, err := c.mgmtV2Client.PostTenantsNsIdUpdateResourceWithResponse(ctx, cluster.NsId, req)
	if err != nil {
		return errors.Wrap(err, "failed to call API to udpate cluster resource")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return err
	}

	// wait for the tenant resource updated.
	return c.waitClusterHealthy(ctx, nsID)
}

func (c *RegionServiceClient) GetTiers(ctx context.Context) ([]apigen_mgmtv1.Tier, error) {
	res, err := c.mgmtV1Client.GetTiersWithResponse(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to retrieve information of all tiers")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return nil, err
	}

	return res.JSON200.Tiers, nil
}

func (c *RegionServiceClient) GetAvailableComponentTypes(ctx context.Context, targetTier apigen_mgmtv1.TierId, component string) ([]apigen_mgmtv1.AvailableComponentType, error) {
	tiers, err := c.GetTiers(ctx)
	if err != nil {
		return nil, err
	}
	var tier *apigen_mgmtv1.Tier
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
	case ComponentPostgresql:
		return tier.AvailableMetaStore.Postgresql.Nodes, nil
	}
	return nil, errors.Errorf("component %s not found", component)
}

func (c *RegionServiceClient) UpdateRisingWaveConfigAwait(ctx context.Context, nsID uuid.UUID, rwConfig string) error {
	cluster, err := c.GetClusterByNsID(ctx, nsID)
	if err != nil {
		return errors.Wrap(err, "failed to get cluster info")
	}
	res, err := c.mgmtV1Client.PutTenantTenantIdConfigRisingwaveWithBodyWithResponse(ctx, cluster.Id, "text/plain", strings.NewReader(rwConfig))
	if err != nil {
		return errors.Wrap(err, "failed to call API to update cluster config")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return err
	}

	// wait for the tenant to be ready
	return c.waitClusterHealthy(ctx, nsID)
}

func (c *RegionServiceClient) GetClusterUsers(ctx context.Context, nsID uuid.UUID) ([]apigen_mgmtv2.DBUser, error) {
	res, err := c.mgmtV2Client.GetTenantsNsIdDatabaseUsersWithResponse(ctx, nsID, &apigen_mgmtv2.GetTenantsNsIdDatabaseUsersParams{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to get cluster user")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, errors.Wrapf(ErrClusterNotFound, "cluster %s not found", nsID)
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return nil, err
	}
	var rtn []apigen_mgmtv2.DBUser
	if res.JSON200.Dbusers != nil {
		rtn = res.JSON200.Dbusers
	}
	return rtn, nil
}

func (c *RegionServiceClient) CreateCluserUser(ctx context.Context, nsID uuid.UUID, params apigen_mgmtv2.CreateDBUserRequestBody) (*apigen_mgmtv2.DBUser, error) {
	res, err := c.mgmtV2Client.PostTenantsNsIdDatabaseUsersWithResponse(ctx, nsID, params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to create cluster user")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, errors.Wrapf(ErrClusterNotFound, "cluster %s not found", nsID)
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return nil, err
	}
	return res.JSON200, nil
}

func (c *RegionServiceClient) UpdateClusterUserPassword(ctx context.Context, nsID uuid.UUID, username, password string) error {
	res, err := c.mgmtV2Client.PutTenantsNsIdDatabaseUsersDbuserNameWithResponse(ctx, nsID, username, apigen_mgmtv2.UpdateDBUserRequestBody{
		Password: password,
	})
	if err != nil {
		return errors.Wrap(err, "failed to call API to update cluster user password")
	}
	return apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body))
}

func (c *RegionServiceClient) DeleteClusterUser(ctx context.Context, nsID uuid.UUID, username string) error {
	res, err := c.mgmtV2Client.DeleteTenantsNsIdDatabaseUsersDbuserNameWithResponse(ctx, nsID, username)
	if err != nil {
		return errors.Wrapf(err, "failed to call API to delete cluster user %s", username)
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil
	}
	return apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body))
}

func (c *RegionServiceClient) GetPrivateLink(ctx context.Context, nsID, privateLinkID uuid.UUID) (*apigen_mgmtv2.PrivateLink, error) {
	res, err := c.mgmtV2Client.GetTenantsNsIdPrivatelinksPrivateLinkIdWithResponse(ctx, nsID, privateLinkID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to call API to get private link %s", privateLinkID)
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, ErrPrivateLinkNotFound
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return nil, err
	}
	return res.JSON200, nil
}

func (c *RegionServiceClient) CreatePrivateLinkAwait(ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.PostPrivateLinkRequestBody) (*apigen_mgmtv2.PrivateLink, error) {
	res, err := c.mgmtV2Client.PostTenantsNsIdPrivatelinksWithResponse(ctx, nsID, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to create private link")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return nil, err
	}
	var info = res.JSON202
	var rtn *apigen_mgmtv2.PrivateLink
	err = wait.Poll(ctx, func() (bool, error) {
		link, err := c.GetPrivateLink(ctx, nsID, info.Id)
		if err != nil {
			if errors.Is(err, ErrPrivateLinkNotFound) {
				return false, nil
			}
			return false, err
		}
		rtn = link
		if link.Status == apigen_mgmtv2.CREATED {
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

func (c *RegionServiceClient) DeletePrivateLinkAwait(ctx context.Context, nsID, privateLinkID uuid.UUID) error {
	res, err := c.mgmtV2Client.DeleteTenantsNsIdPrivatelinksPrivateLinkIdWithResponse(ctx, nsID, privateLinkID)
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
		_, err := c.GetPrivateLink(ctx, nsID, privateLinkID)
		if err != nil {
			if errors.Is(err, ErrPrivateLinkNotFound) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}, PollingPrivateLinkDeletion)
}

func (c *RegionServiceClient) GetBYOCCluster(ctx context.Context, name string) (*apigen_mgmtv2.ManagedCluster, error) {
	res, err := c.mgmtV2Client.GetByocClustersNameWithResponse(ctx, name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to get BYOC cluster")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, errors.Wrapf(ErrBYOCClusterNotFound, "BYOC cluster %s not found", name)
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return nil, err
	}
	return res.JSON200, nil
}
