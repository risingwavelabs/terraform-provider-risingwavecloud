package cloudsdk

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen"
	apigen_acc "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/acc/v1"
	apigen_mgmtv1 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v1"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/ptr"
)

var (
	ErrInvalidCredential = errors.New("invalid credential")
)

const (
	headerUserAgent     = "User-Agent"
	headerAPIKey        = "X-API-KEY"
	headerAuthorization = "Authorization"

	userAgentProductName = "terraform-provider-risingwavecloud"
)

type JSON = map[string]any

type CloudClientInterface interface {
	// Check the connection of the endpoint and validate the API key provided.
	Ping(context.Context) error

	/* Cluster */

	GetClusterByNsID(ctx context.Context, nsID uuid.UUID) (*apigen_mgmtv2.Tenant, error)

	GetClusterByRegionAndName(ctx context.Context, region string, name string) (*apigen_mgmtv2.Tenant, error)

	CreateClusterAwait(ctx context.Context, region string, req apigen_mgmtv2.TenantRequestRequestBody) (*apigen_mgmtv2.Tenant, error)

	GetTiers(ctx context.Context, region string) ([]apigen_mgmtv1.Tier, error)

	GetAvailableComponentTypes(ctx context.Context, region string, targetTier apigen_mgmtv1.TierId, component string) ([]apigen_mgmtv1.AvailableComponentType, error)

	DeleteClusterByNsIDAwait(ctx context.Context, nsID uuid.UUID) error

	UpdateClusterImageByNsIDAwait(ctx context.Context, nsID uuid.UUID, version string) error

	UpdateClusterResourcesByNsIDAwait(ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.PostTenantResourcesRequestBody) error

	UpdateRisingWaveConfigByNsIDAwait(ctx context.Context, nsID uuid.UUID, rwConfig string) error

	/* Cluster User */

	GetClusterUser(ctx context.Context, clusterNsID uuid.UUID, username string) (*apigen_mgmtv2.DBUser, error)

	CreateClusterUser(ctx context.Context, clusterNsID uuid.UUID, username, password string, createDB, superUser, createUser bool) (*apigen_mgmtv2.DBUser, error)

	UpdateClusterUserPassword(ctx context.Context, clusterNsID uuid.UUID, username, password string) error

	DeleteClusterUser(ctx context.Context, clusterNsID uuid.UUID, username string) error

	/* Private Link */

	GetPrivateLinks(ctx context.Context) ([]PrivateLinkInfo, error)

	// GetPrivateLink returns the private link and its cluster ID by the given private link ID.
	GetPrivateLink(ctx context.Context, privateLinkID uuid.UUID) (*PrivateLinkInfo, error)

	// GetPrivateLink returns the private link and its cluster ID by the given connection name.
	GetPrivateLinkByName(ctx context.Context, connectionName string) (*PrivateLinkInfo, error)

	// CreatePrivateLinkAwait creates the private link and waits for the creation to complete.
	CreatePrivateLinkAwait(ctx context.Context, clusterNsID uuid.UUID, req apigen_mgmtv2.PostPrivateLinkRequestBody) (*PrivateLinkInfo, error)

	// DeletePrivateLinkAwait deletes the private link and waits for the deletion to complete. it
	// returns nil if the private link is deleted successfully or not found.
	DeletePrivateLinkAwait(ctx context.Context, clusterNsID uuid.UUID, privateLinkID uuid.UUID) error

	GetBYOCCluster(ctx context.Context, region string, name string) (*apigen_mgmtv2.ManagedCluster, error)

	/* Resource Group */

	// GetResourceGroup returns the resource group of the given name in the cluster.
	GetResourceGroup(ctx context.Context, clusterNsID uuid.UUID, name string) (*apigen_mgmtv2.ResourceGroupDetails, error)

	// CreateResourceGroupAwait creates a resource group and waits for the cluster rescale to complete.
	CreateResourceGroupAwait(ctx context.Context, clusterNsID uuid.UUID, req apigen_mgmtv2.CreateResourceGroupsRequestBody) (*apigen_mgmtv2.ResourceGroupDetails, error)

	// UpdateResourceGroupAwait rescales a resource group and waits for the cluster rescale to complete.
	UpdateResourceGroupAwait(ctx context.Context, clusterNsID uuid.UUID, resourceGroup string, req apigen_mgmtv2.UpdateResourceGroupsRequestBody) (*apigen_mgmtv2.ResourceGroupDetails, error)

	// DeleteResourceGroupAwait deletes a resource group and waits for the deletion to complete. it
	// returns nil if the resource group is deleted successfully or not found.
	DeleteResourceGroupAwait(ctx context.Context, clusterNsID uuid.UUID, resourceGroup string) error

	/* Allowed IAM Roles */

	// GetAllowedIamRoles returns the IAM role ARNs allowed to access the cluster's resources
	// through the assume role mechanism.
	GetAllowedIamRoles(ctx context.Context, clusterNsID uuid.UUID) ([]string, error)

	// AddAllowedIamRoleAwait allows an IAM role and waits for the change to be applied. it
	// returns nil if the role is already allowed.
	AddAllowedIamRoleAwait(ctx context.Context, clusterNsID uuid.UUID, roleArn string) error

	// RemoveAllowedIamRoleAwait disallows an IAM role and waits for the change to be applied.
	// it returns nil if the role is not allowed in the first place.
	RemoveAllowedIamRoleAwait(ctx context.Context, clusterNsID uuid.UUID, roleArn string) error
}

type CloudClient struct {
	Endpoint   string
	accClient  *apigen_acc.ClientWithResponses
	apiKeyPair string
	regions    map[string]RegionServiceClientInterface

	// rescaleLocks holds one mutex per cluster NsID (uuid.UUID -> *sync.Mutex).
	rescaleLocks sync.Map
}

// lockClusterRescale serializes the operations that rescale a cluster. Rescaling is
// exclusive on the platform side and every request waits for the whole cluster to become
// healthy again, while terraform applies independent resources concurrently: without this
// lock two resource groups of the same cluster (or a resource group and the cluster spec
// itself) would trigger overlapping rescales. The returned function releases the lock.
func (c *CloudClient) lockClusterRescale(nsID uuid.UUID) func() {
	v, _ := c.rescaleLocks.LoadOrStore(nsID, &sync.Mutex{})
	mu, ok := v.(*sync.Mutex)
	if !ok {
		// unreachable: nothing else is ever stored in this map.
		return func() {}
	}
	mu.Lock()
	return mu.Unlock
}

func NewCloudClient(ctx context.Context, endpoint, apiKey, apiSecret, tfPluginVersion string) (CloudClientInterface, error) {
	apiKeyPair := fmt.Sprintf("%s:%s", apiKey, apiSecret)

	requestEditor := func(ctx context.Context, req *http.Request) error {
		req.Header.Set(headerAPIKey, apiKeyPair) // deprecated: keep it to support old version
		req.Header.Set(headerAuthorization, fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(apiKeyPair))))
		req.Header.Set(headerUserAgent, fmt.Sprintf("%s/%s", userAgentProductName, tfPluginVersion))
		return nil
	}

	accClient, err := apigen_acc.NewClientWithResponses(endpoint, apigen_acc.WithRequestEditorFn(requestEditor))
	if err != nil {
		return nil, err
	}

	// get regions
	res, err := accClient.GetRegionsWithResponse(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get regions")
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return nil, err
	}
	if res.JSON200 == nil {
		return nil, errors.New("unexpected error, region array is nil")
	}
	regions := *res.JSON200
	if len(regions) == 0 {
		return nil, errors.New("unexpected error, region array is empty")
	}

	regionMap := make(map[string]RegionServiceClientInterface)
	for _, region := range regions {
		rs, err := createRegionServiceClient(region.Url, region.UrlV2, requestEditor)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get region service client")
		}
		regionMap[region.RegionName] = rs
	}

	return &CloudClient{
		Endpoint:   endpoint,
		accClient:  accClient,
		regions:    regionMap,
		apiKeyPair: apiKeyPair,
	}, nil
}

func createRegionServiceClient(urlV1, urlV2 string, reqEditor func(ctx context.Context, req *http.Request) error) (RegionServiceClientInterface, error) {
	mgmtV1Client, err := apigen_mgmtv1.NewClientWithResponses(urlV1, apigen_mgmtv1.WithRequestEditorFn(reqEditor))
	if err != nil {
		return nil, err
	}

	mgmtV2Client, err := apigen_mgmtv2.NewClientWithResponses(urlV2, apigen_mgmtv2.WithRequestEditorFn(reqEditor))
	if err != nil {
		return nil, err
	}

	return &RegionServiceClient{
		mgmtV1Client: mgmtV1Client,
		mgmtV2Client: mgmtV2Client,
	}, nil
}

func (c *CloudClient) getRegionClient(region string) (RegionServiceClientInterface, error) {
	rs, ok := c.regions[region]
	if !ok {
		return nil, fmt.Errorf("region %s is not found", region)
	}
	return rs, nil
}

func (c *CloudClient) getClusterInfo(ctx context.Context, nsID uuid.UUID) (*apigen_acc.Tenant, error) {
	res, err := c.accClient.GetTenantNsIDWithResponse(ctx, nsID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call API to get cluster info")
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, errors.Wrapf(ErrClusterNotFound, "cluster %s", nsID.String())
	}
	return res.JSON200, nil
}

func (c *CloudClient) getClusterInfoAndRegionClient(ctx context.Context, nsID uuid.UUID) (*apigen_acc.Tenant, RegionServiceClientInterface, error) {
	cluster, err := c.getClusterInfo(ctx, nsID)
	if err != nil {
		return nil, nil, err
	}
	rs, err := c.getRegionClient(cluster.Region)
	if err != nil {
		return nil, nil, err
	}
	return cluster, rs, nil
}

func (c *CloudClient) GetClusterByNsID(ctx context.Context, nsID uuid.UUID) (*apigen_mgmtv2.Tenant, error) {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, nsID)
	if err != nil {
		return nil, err
	}

	return rs.GetClusterByNsID(ctx, info.NsId)
}

func (c *CloudClient) Ping(ctx context.Context) error {
	res, err := c.accClient.GetAuthPingWithResponse(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to ping endpoint")
	}
	if res.StatusCode() == http.StatusForbidden {
		return ErrInvalidCredential
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
		return err
	}
	return nil
}

func (c *CloudClient) CreateClusterAwait(ctx context.Context, region string, req apigen_mgmtv2.TenantRequestRequestBody) (*apigen_mgmtv2.Tenant, error) {
	rs, err := c.getRegionClient(region)
	if err != nil {
		return nil, err
	}

	return rs.CreateClusterAwait(ctx, req)
}

func (c *CloudClient) GetTiers(ctx context.Context, region string) ([]apigen_mgmtv1.Tier, error) {
	rs, err := c.getRegionClient(region)
	if err != nil {
		return nil, err
	}

	return rs.GetTiers(ctx)
}

func (c *CloudClient) GetAvailableComponentTypes(ctx context.Context, region string, targetTier apigen_mgmtv1.TierId, component string) ([]apigen_mgmtv1.AvailableComponentType, error) {
	rs, err := c.getRegionClient(region)
	if err != nil {
		return nil, err
	}

	return rs.GetAvailableComponentTypes(ctx, targetTier, component)
}

func (c *CloudClient) DeleteClusterByNsIDAwait(ctx context.Context, nsID uuid.UUID) error {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, nsID)
	if err != nil {
		return err
	}

	return rs.DeleteClusterAwait(ctx, info.NsId)
}

func (c *CloudClient) UpdateClusterImageByNsIDAwait(ctx context.Context, nsID uuid.UUID, version string) error {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, nsID)
	if err != nil {
		return err
	}

	return rs.UpdateClusterImageAwait(ctx, info.NsId, version)
}

func (c *CloudClient) UpdateClusterResourcesByNsIDAwait(ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.PostTenantResourcesRequestBody) error {
	defer c.lockClusterRescale(nsID)()

	info, rs, err := c.getClusterInfoAndRegionClient(ctx, nsID)
	if err != nil {
		return err
	}

	return rs.UpdateClusterResourcesAwait(ctx, info.NsId, req)
}

func (c *CloudClient) UpdateRisingWaveConfigByNsIDAwait(ctx context.Context, nsID uuid.UUID, rwConfig string) error {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, nsID)
	if err != nil {
		return err
	}

	return rs.UpdateRisingWaveConfigAwait(ctx, info.NsId, rwConfig)
}

func (c *CloudClient) GetClusterUser(ctx context.Context, clusterNsID uuid.UUID, username string) (*apigen_mgmtv2.DBUser, error) {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return nil, err
	}
	users, err := rs.GetClusterUsers(ctx, info.NsId)
	if err != nil {
		if errors.Is(err, ErrClusterNotFound) {
			return nil, errors.Wrapf(ErrClusterNotFound, "cluster %s", clusterNsID.String())
		}
		return nil, err
	}
	for _, user := range users {
		if user.Username == username {
			return ptr.Ptr(user), nil
		}
	}
	return nil, errors.Errorf("user %s not found in cluster %s", username, clusterNsID.String())
}

func (c *CloudClient) CreateClusterUser(ctx context.Context, clusterNsID uuid.UUID, username, password string, createDB, superUser, createUser bool) (*apigen_mgmtv2.DBUser, error) {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return nil, err
	}
	u, err := rs.CreateClusterUser(ctx, info.NsId, apigen_mgmtv2.CreateDBUserRequestBody{
		Username:   username,
		Password:   password,
		Createdb:   createDB,
		Superuser:  superUser,
		Createuser: ptr.Ptr(createUser),
	})
	if err != nil {
		if errors.Is(err, ErrClusterNotFound) {
			return nil, errors.Wrapf(ErrClusterNotFound, "cluster %s", clusterNsID.String())
		}
		return nil, err
	}
	return u, err
}

func (c *CloudClient) UpdateClusterUserPassword(ctx context.Context, clusterNsID uuid.UUID, username, password string) error {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return err
	}
	return rs.UpdateClusterUserPassword(ctx, info.NsId, username, password)
}

func (c *CloudClient) DeleteClusterUser(ctx context.Context, clusterNsID uuid.UUID, username string) error {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return err
	}
	return rs.DeleteClusterUser(ctx, info.NsId, username)
}

type PrivateLinkInfo struct {
	ClusterNsID uuid.UUID
	PrivateLink *apigen_mgmtv2.PrivateLink
}

func (c *CloudClient) GetPrivateLinks(ctx context.Context) ([]PrivateLinkInfo, error) {
	var (
		offset uint64 = 0
		limit  uint64 = 10
	)
	var privateLinks []apigen_acc.PrivateLink
	for {
		res, err := c.accClient.GetPrivatelinksWithResponse(ctx, &apigen_acc.GetPrivatelinksParams{
			Offset: &offset,
			Limit:  &limit,
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to call API get private links")
		}
		if err = apigen.ExpectStatusCodeWithMessage(res, http.StatusOK, string(res.Body)); err != nil {
			return nil, err
		}
		offset = res.JSON200.Offset
		limit = res.JSON200.Limit
		privateLinks = append(privateLinks, res.JSON200.PrivateLinks...)
		if limit*(offset+1) >= res.JSON200.Size {
			break
		}
		offset++
	}
	var plInfos []PrivateLinkInfo
	for _, accpl := range privateLinks {
		rs, err := c.getRegionClient(accpl.Region)
		if err != nil {
			return nil, err
		}
		pl, err := rs.GetPrivateLink(ctx, accpl.TenantNsId, accpl.Id)
		if err != nil {
			return nil, err
		}
		cluster, err := rs.GetClusterByNsID(ctx, accpl.TenantNsId)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get cluster through id %d provided by private link with ID %s", accpl.TenantId, pl.Id.String())
		}
		plInfos = append(plInfos, PrivateLinkInfo{
			ClusterNsID: cluster.NsId,
			PrivateLink: pl,
		})
	}
	return plInfos, nil
}

func (c *CloudClient) GetPrivateLink(ctx context.Context, privateLinkID uuid.UUID) (*PrivateLinkInfo, error) {
	pls, err := c.GetPrivateLinks(ctx)
	if err != nil {
		return nil, err
	}
	for _, pl := range pls {
		if pl.PrivateLink.Id == privateLinkID {
			return &pl, nil
		}
	}

	return nil, errors.Wrapf(ErrPrivateLinkNotFound, "private link %s", privateLinkID.String())
}

func (c *CloudClient) CreatePrivateLinkAwait(ctx context.Context, clusterNsID uuid.UUID, req apigen_mgmtv2.PostPrivateLinkRequestBody) (*PrivateLinkInfo, error) {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return nil, err
	}
	pl, err := rs.CreatePrivateLinkAwait(ctx, info.NsId, req)
	if err != nil {
		return nil, err
	}
	return &PrivateLinkInfo{
		ClusterNsID: info.NsId,
		PrivateLink: pl,
	}, nil
}

func (c *CloudClient) DeletePrivateLinkAwait(ctx context.Context, clusterNsID uuid.UUID, privateLinkID uuid.UUID) error {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return err
	}
	return rs.DeletePrivateLinkAwait(ctx, info.NsId, privateLinkID)
}

func (c *CloudClient) GetClusterByRegionAndName(ctx context.Context, region string, name string) (*apigen_mgmtv2.Tenant, error) {
	rs, err := c.getRegionClient(region)
	if err != nil {
		return nil, err
	}

	tenantV1, err := rs.GetClusterByName(ctx, name)
	if err != nil {
		return nil, err
	}

	return rs.GetClusterByNsID(ctx, tenantV1.NsId)
}

func (c *CloudClient) GetPrivateLinkByName(ctx context.Context, connectionName string) (*PrivateLinkInfo, error) {
	pls, err := c.GetPrivateLinks(ctx)
	if err != nil {
		return nil, err
	}
	for _, pl := range pls {
		if pl.PrivateLink.ConnectionName == connectionName {
			return &pl, nil
		}
	}
	return nil, errors.Wrapf(ErrPrivateLinkNotFound, "private link %s", connectionName)
}

func (c *CloudClient) GetBYOCCluster(ctx context.Context, region string, name string) (*apigen_mgmtv2.ManagedCluster, error) {
	rs, err := c.getRegionClient(region)
	if err != nil {
		return nil, err
	}
	return rs.GetBYOCCluster(ctx, name)
}

func (c *CloudClient) GetResourceGroup(ctx context.Context, clusterNsID uuid.UUID, name string) (*apigen_mgmtv2.ResourceGroupDetails, error) {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return nil, err
	}
	groups, err := rs.GetResourceGroups(ctx, info.NsId)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if g.Name == name {
			return ptr.Ptr(g), nil
		}
	}
	return nil, errors.Wrapf(ErrResourceGroupNotFound, "resource group %s in cluster %s", name, clusterNsID.String())
}

func (c *CloudClient) CreateResourceGroupAwait(ctx context.Context, clusterNsID uuid.UUID, req apigen_mgmtv2.CreateResourceGroupsRequestBody) (*apigen_mgmtv2.ResourceGroupDetails, error) {
	defer c.lockClusterRescale(clusterNsID)()

	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return nil, err
	}
	return rs.CreateResourceGroupAwait(ctx, info.NsId, req)
}

func (c *CloudClient) UpdateResourceGroupAwait(ctx context.Context, clusterNsID uuid.UUID, resourceGroup string, req apigen_mgmtv2.UpdateResourceGroupsRequestBody) (*apigen_mgmtv2.ResourceGroupDetails, error) {
	defer c.lockClusterRescale(clusterNsID)()

	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return nil, err
	}
	return rs.UpdateResourceGroupAwait(ctx, info.NsId, resourceGroup, req)
}

func (c *CloudClient) DeleteResourceGroupAwait(ctx context.Context, clusterNsID uuid.UUID, resourceGroup string) error {
	defer c.lockClusterRescale(clusterNsID)()

	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return err
	}
	return rs.DeleteResourceGroupAwait(ctx, info.NsId, resourceGroup)
}

func (c *CloudClient) GetAllowedIamRoles(ctx context.Context, clusterNsID uuid.UUID) ([]string, error) {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return nil, err
	}
	return rs.GetAllowedIamRoles(ctx, info.NsId)
}

func (c *CloudClient) AddAllowedIamRoleAwait(ctx context.Context, clusterNsID uuid.UUID, roleArn string) error {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return err
	}
	return rs.AddAllowedIamRoleAwait(ctx, info.NsId, roleArn)
}

func (c *CloudClient) RemoveAllowedIamRoleAwait(ctx context.Context, clusterNsID uuid.UUID, roleArn string) error {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return err
	}
	return rs.RemoveAllowedIamRoleAwait(ctx, info.NsId, roleArn)
}
