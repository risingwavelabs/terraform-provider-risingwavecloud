package cloudsdk

import (
	"context"
	"net/http"
	"slices"
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
	ErrClusterNotFound       = errors.New("cluster not found")
	ErrBYOCClusterNotFound   = errors.New("BYOC cluster not found")
	ErrClusterUserNotFound   = errors.New("cluster user not found")
	ErrPrivateLinkNotFound   = errors.New("private link not found")
	ErrResourceGroupNotFound = errors.New("resource group not found")
)

const (
	ComponentCompute    = "compute"
	ComponentCompactor  = "compactor"
	ComponentFrontend   = "frontend"
	ComponentMeta       = "meta"
	ComponentStandalone = "standalone"
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

	// Resource group create/update/delete trigger a cluster rescale, so reuse the
	// tenant-scale timeout budget.
	PollingResourceGroupOperation = wait.PollingParams{
		Timeout:  15 * time.Minute,
		Interval: 3 * time.Second,
	}

	// Changing the allowed IAM roles does not rescale anything, it only reconfigures access,
	// so it settles in seconds rather than minutes.
	PollingAllowedIamRoleOperation = wait.PollingParams{
		Timeout:  5 * time.Minute,
		Interval: 2 * time.Second,
	}

	// A rescale request is accepted asynchronously: the cluster keeps reporting the healthy
	// status for a short while before the rescale actually starts. Wait for that transition
	// so that waiting for "healthy" does not return before the rescale even began.
	PollingRescaleStart = wait.PollingParams{
		Timeout:  30 * time.Second,
		Interval: 2 * time.Second,
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

	CreateClusterUser(ctx context.Context, nsID uuid.UUID, params apigen_mgmtv2.CreateDBUserRequestBody) (*apigen_mgmtv2.DBUser, error)

	UpdateClusterUserPassword(ctx context.Context, nsID uuid.UUID, username, password string) error

	DeleteClusterUser(ctx context.Context, nsID uuid.UUID, username string) error

	GetPrivateLink(ctx context.Context, nsID, privateLinkID uuid.UUID) (*apigen_mgmtv2.PrivateLink, error)

	CreatePrivateLinkAwait(ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.PostPrivateLinkRequestBody) (*apigen_mgmtv2.PrivateLink, error)

	DeletePrivateLinkAwait(ctx context.Context, nsID, privateLinkID uuid.UUID) error

	GetBYOCCluster(ctx context.Context, name string) (*apigen_mgmtv2.ManagedCluster, error)

	GetResourceGroups(ctx context.Context, nsID uuid.UUID) ([]apigen_mgmtv2.ResourceGroupDetails, error)

	CreateResourceGroupAwait(ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.CreateResourceGroupsRequestBody) (*apigen_mgmtv2.ResourceGroupDetails, error)

	UpdateResourceGroupAwait(ctx context.Context, nsID uuid.UUID, resourceGroup string, req apigen_mgmtv2.UpdateResourceGroupsRequestBody) (*apigen_mgmtv2.ResourceGroupDetails, error)

	DeleteResourceGroupAwait(ctx context.Context, nsID uuid.UUID, resourceGroup string) error

	GetAllowedIamRoles(ctx context.Context, nsID uuid.UUID) ([]string, error)

	AddAllowedIamRoleAwait(ctx context.Context, nsID uuid.UUID, roleArn string) error

	RemoveAllowedIamRoleAwait(ctx context.Context, nsID uuid.UUID, roleArn string) error
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
		return currHealth == apigen_mgmtv2.Healthy, nil
	}, PollingTenantCreation); err != nil {
		return errors.Wrapf(err, "failed to wait for the cluster, current health status: %s, target health status: %s", currHealth, apigen_mgmtv2.Healthy)
	}
	return nil
}

// waitClusterIdle waits until the cluster can accept a rescale request. The platform rejects
// one while another is in flight, with `400 {"msg":"Cluster is not running"}`, and the caller's
// per-cluster lock only covers a single provider process: a second terraform run, an aliased
// provider or somebody working in the console can all put the cluster in that state.
func (c *RegionServiceClient) waitClusterIdle(ctx context.Context, nsID uuid.UUID) error {
	var current apigen_mgmtv2.TenantStatus
	if err := wait.Poll(ctx, func() (bool, error) {
		cluster, err := c.GetClusterByNsID(ctx, nsID)
		if err != nil {
			return false, errors.Wrap(err, "failed to get the cluster info")
		}
		current = cluster.Status
		return current == apigen_mgmtv2.Running && cluster.HealthStatus == apigen_mgmtv2.Healthy, nil
	}, PollingResourceGroupOperation); err != nil {
		return errors.Wrapf(
			err,
			"the cluster is not ready to be rescaled, current status: %s, target status: %s",
			current, apigen_mgmtv2.Running,
		)
	}
	return nil
}

// waitClusterRescaled waits for an accepted rescale request to be fully applied. Waiting
// for the healthy status alone is not enough: the cluster still reports itself as healthy
// for a short while after the request is accepted, so wait for it to leave the healthy
// status first. Not observing that transition is not an error, the rescale may already be
// done by the time we start polling.
func (c *RegionServiceClient) waitClusterRescaled(ctx context.Context, nsID uuid.UUID) error {
	if err := wait.Poll(ctx, func() (bool, error) {
		cluster, err := c.GetClusterByNsID(ctx, nsID)
		if err != nil {
			return false, errors.Wrap(err, "failed to get the cluster info")
		}
		return cluster.HealthStatus != apigen_mgmtv2.Healthy, nil
	}, PollingRescaleStart); err != nil && !errors.Is(err, wait.ErrWaitTimeout) {
		return err
	}
	return c.waitClusterHealthy(ctx, nsID)
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
	if err := c.waitClusterHealthStatusByNsID(ctx, createRes.JSON202.NsId, apigen_mgmtv2.Healthy); err != nil {
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
	case ComponentStandalone:
		return tier.AvailableStandaloneNodes, nil
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
	res, err := c.mgmtV1Client.PutTenantTenantIdConfigRisingwaveWithBodyWithResponse(ctx, cluster.Id, nil, "text/plain", strings.NewReader(rwConfig))
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

func (c *RegionServiceClient) CreateClusterUser(ctx context.Context, nsID uuid.UUID, params apigen_mgmtv2.CreateDBUserRequestBody) (*apigen_mgmtv2.DBUser, error) {
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

func (c *RegionServiceClient) GetResourceGroups(ctx context.Context, nsID uuid.UUID) ([]apigen_mgmtv2.ResourceGroupDetails, error) {
	res, err := c.mgmtV2Client.GetTenantsNsIdResourceGroupsWithResponse(ctx, nsID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to get resource groups")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, errors.Wrapf(ErrClusterNotFound, "cluster %s not found", nsID)
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return nil, err
	}
	return res.JSON200.ResourceGroups, nil
}

func (c *RegionServiceClient) getResourceGroup(ctx context.Context, nsID uuid.UUID, name string) (*apigen_mgmtv2.ResourceGroupDetails, error) {
	groups, err := c.GetResourceGroups(ctx, nsID)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if g.Name == name {
			return ptr.Ptr(g), nil
		}
	}
	return nil, errors.Wrapf(ErrResourceGroupNotFound, "resource group %s", name)
}

// waitResourceGroupResource waits for the resource group to report the requested resource
// spec. The returned details are written to the terraform state, so they must reflect the
// applied spec instead of the one the cluster is rescaling away from.
func (c *RegionServiceClient) waitResourceGroupResource(
	ctx context.Context, nsID uuid.UUID, name string, expected apigen_mgmtv2.ComponentResourceRequest,
) (*apigen_mgmtv2.ResourceGroupDetails, error) {
	var rtn *apigen_mgmtv2.ResourceGroupDetails
	if err := wait.Poll(ctx, func() (bool, error) {
		g, err := c.getResourceGroup(ctx, nsID, name)
		if err != nil {
			if errors.Is(err, ErrResourceGroupNotFound) {
				return false, nil
			}
			return false, err
		}
		if g.Resource.ComponentTypeId != expected.ComponentTypeId || g.Resource.Replica != expected.Replica {
			return false, nil
		}
		rtn = g
		return true, nil
	}, PollingResourceGroupOperation); err != nil {
		return nil, errors.Wrapf(
			err,
			"failed to wait for the resource group %s to report component type %s with %d replica(s)",
			name, expected.ComponentTypeId, expected.Replica,
		)
	}
	return rtn, nil
}

func (c *RegionServiceClient) CreateResourceGroupAwait(ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.CreateResourceGroupsRequestBody) (*apigen_mgmtv2.ResourceGroupDetails, error) {
	if err := c.waitClusterIdle(ctx, nsID); err != nil {
		return nil, err
	}
	res, err := c.mgmtV2Client.PostTenantsNsIdResourceGroupsWithResponse(ctx, nsID, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to create resource group")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, errors.Wrapf(ErrClusterNotFound, "cluster %s not found", nsID)
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return nil, err
	}
	if err := c.waitClusterRescaled(ctx, nsID); err != nil {
		return nil, err
	}
	return c.waitResourceGroupResource(ctx, nsID, req.Name, req.Resource)
}

func (c *RegionServiceClient) UpdateResourceGroupAwait(ctx context.Context, nsID uuid.UUID, resourceGroup string, req apigen_mgmtv2.UpdateResourceGroupsRequestBody) (*apigen_mgmtv2.ResourceGroupDetails, error) {
	if err := c.waitClusterIdle(ctx, nsID); err != nil {
		return nil, err
	}
	res, err := c.mgmtV2Client.PostTenantsNsIdResourceGroupsResourceGroupWithResponse(ctx, nsID, resourceGroup, req)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to call API to update resource group %s", resourceGroup)
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, errors.Wrapf(ErrResourceGroupNotFound, "resource group %s", resourceGroup)
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return nil, err
	}
	if err := c.waitClusterRescaled(ctx, nsID); err != nil {
		return nil, err
	}
	return c.waitResourceGroupResource(ctx, nsID, resourceGroup, req.Resource)
}

func (c *RegionServiceClient) DeleteResourceGroupAwait(ctx context.Context, nsID uuid.UUID, resourceGroup string) error {
	if err := c.waitClusterIdle(ctx, nsID); err != nil {
		return err
	}
	res, err := c.mgmtV2Client.DeleteTenantsNsIdResourceGroupsResourceGroupWithResponse(ctx, nsID, resourceGroup)
	if err != nil {
		return errors.Wrapf(err, "failed to call API to delete resource group %s", resourceGroup)
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil
	}
	// A rejected deletion (the databases running in the group have to be dropped first) comes
	// back as 400 with a message that already names them, so pass the body through as is.
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return err
	}
	if err := c.waitClusterRescaled(ctx, nsID); err != nil {
		return err
	}
	return wait.Poll(ctx, func() (bool, error) {
		_, err := c.getResourceGroup(ctx, nsID, resourceGroup)
		if err != nil {
			if errors.Is(err, ErrResourceGroupNotFound) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}, PollingResourceGroupOperation)
}

// allowedIamRoles reports the ARNs currently permitted to assume a role into the customer's
// account, together with the state the platform is in while applying a change.
func (c *RegionServiceClient) allowedIamRoles(ctx context.Context, nsID uuid.UUID) (*apigen_mgmtv2.GetTenantAllowedIamRolesResponseBody, error) {
	res, err := c.mgmtV2Client.GetTenantsNsIdAllowedIamRolesWithResponse(ctx, nsID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to get the allowed IAM roles")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, errors.Wrapf(ErrClusterNotFound, "cluster %s not found", nsID)
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return nil, err
	}
	return res.JSON200, nil
}

func (c *RegionServiceClient) GetAllowedIamRoles(ctx context.Context, nsID uuid.UUID) ([]string, error) {
	roles, err := c.allowedIamRoles(ctx, nsID)
	if err != nil {
		return nil, err
	}
	return roles.RoleArns, nil
}

// pollAllowedIamRoles checks once before polling, since wait.Poll sleeps for a whole interval
// before its first attempt and every mutation would otherwise pay for it twice.
func pollAllowedIamRoles(ctx context.Context, check func() (bool, error)) error {
	if done, err := check(); err != nil || done {
		return err
	}
	return wait.Poll(ctx, check, PollingAllowedIamRoleOperation)
}

// readyAllowedIamRoles reports whether the policy has settled, and gives up on `Failed`:
// polling on would only spend the whole budget to report a timeout instead of the failure the
// platform already knows about.
func (c *RegionServiceClient) readyAllowedIamRoles(ctx context.Context, nsID uuid.UUID) (*apigen_mgmtv2.GetTenantAllowedIamRolesResponseBody, bool, error) {
	roles, err := c.allowedIamRoles(ctx, nsID)
	if err != nil {
		return nil, false, err
	}
	switch roles.Status {
	case apigen_mgmtv2.GetTenantAllowedIamRolesResponseBodyStatusReady:
		return roles, true, nil
	case apigen_mgmtv2.GetTenantAllowedIamRolesResponseBodyStatusFailed:
		return nil, false, errors.Errorf("the platform failed to apply the allowed IAM roles of cluster %s", nsID)
	default:
		return roles, false, nil
	}
}

// waitAllowedIamRoles waits until the IAM policy can accept another change. The platform
// answers a request that overlaps another with a 500, so this runs before every mutation.
func (c *RegionServiceClient) waitAllowedIamRoles(ctx context.Context, nsID uuid.UUID) error {
	if err := pollAllowedIamRoles(ctx, func() (bool, error) {
		_, ready, err := c.readyAllowedIamRoles(ctx, nsID)
		return ready, err
	}); err != nil {
		return errors.Wrap(err, "failed to wait for the allowed IAM roles to settle")
	}
	return nil
}

// waitAllowedIamRoleApplied waits until the list itself shows the change, not merely until the
// status says the policy has settled.
//
// The two are not the same. A check that runs right after a request is accepted can read a
// status that has not moved yet, and answer with the list as it was. The platform updates the
// record before the status today, so this is a belt rather than a fix, but membership is the
// thing the caller actually asked about and it does not depend on that ordering holding.
func (c *RegionServiceClient) waitAllowedIamRoleApplied(ctx context.Context, nsID uuid.UUID, roleArn string, want bool) error {
	if err := pollAllowedIamRoles(ctx, func() (bool, error) {
		roles, ready, err := c.readyAllowedIamRoles(ctx, nsID)
		if err != nil || !ready {
			return false, err
		}
		return slices.Contains(roles.RoleArns, roleArn) == want, nil
	}); err != nil {
		verb := "allowed"
		if !want {
			verb = "removed"
		}
		return errors.Wrapf(err, "failed to wait for the IAM role %s to be %s", roleArn, verb)
	}
	return nil
}

func (c *RegionServiceClient) AddAllowedIamRoleAwait(ctx context.Context, nsID uuid.UUID, roleArn string) error {
	if err := c.waitAllowedIamRoles(ctx, nsID); err != nil {
		return err
	}
	res, err := c.mgmtV2Client.PostTenantsNsIdAllowedIamRolesWithResponse(ctx, nsID, apigen_mgmtv2.TenantAllowedIamRoleRequestBody{
		RoleArn: roleArn,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to call API to allow the IAM role %s", roleArn)
	}
	// the role is already allowed, which is the state the caller asked for
	if res.StatusCode() == http.StatusConflict {
		return nil
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return err
	}
	return c.waitAllowedIamRoleApplied(ctx, nsID, roleArn, true)
}

func (c *RegionServiceClient) RemoveAllowedIamRoleAwait(ctx context.Context, nsID uuid.UUID, roleArn string) error {
	if err := c.waitAllowedIamRoles(ctx, nsID); err != nil {
		return err
	}
	res, err := c.mgmtV2Client.DeleteTenantsNsIdAllowedIamRolesWithResponse(ctx, nsID, apigen_mgmtv2.TenantAllowedIamRoleRequestBody{
		RoleArn: roleArn,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to call API to remove the IAM role %s", roleArn)
	}
	// already gone, which is the state the caller asked for
	if res.StatusCode() == http.StatusNotFound {
		return nil
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusAccepted, string(res.Body)); err != nil {
		return err
	}
	return c.waitAllowedIamRoleApplied(ctx, nsID, roleArn, false)
}
