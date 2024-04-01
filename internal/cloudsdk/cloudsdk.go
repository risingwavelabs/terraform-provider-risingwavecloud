package cloudsdk

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen"
	apigen_acc "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/acc"
	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/ptr"
)

var (
	ErrInvalidCredential = errors.New("invalid credential")
)

type JSON = map[string]any

type CloudClientInterface interface {
	// Check the connection of the endpoint and validate the API key provided.
	Ping(context.Context) error

	/* Cluster */

	GetClusterByNsID(ctx context.Context, nsID uuid.UUID) (*apigen_mgmt.Tenant, error)

	IsTenantNameExist(ctx context.Context, region string, tenantName string) (bool, error)

	CreateClusterAwait(ctx context.Context, region string, req apigen_mgmt.TenantRequestRequestBody) (*apigen_mgmt.Tenant, error)

	GetTiers(ctx context.Context, region string) ([]apigen_mgmt.Tier, error)

	GetAvailableComponentTypes(ctx context.Context, region string, targetTier apigen_mgmt.TierId, component string) ([]apigen_mgmt.AvailableComponentType, error)

	DeleteClusterByNsIDAwait(ctx context.Context, nsID uuid.UUID) error

	UpdateClusterImageByNsIDAwait(ctx context.Context, nsID uuid.UUID, version string) error

	UpdateClusterResourcesByNsIDAwait(ctx context.Context, nsID uuid.UUID, req apigen_mgmt.PostTenantResourcesRequestBody) error

	UpdateRisingWaveConfigByNsIDAwait(ctx context.Context, nsID uuid.UUID, rwConfig string) error

	UpdateEtcdConfigByNsIDAwait(ctx context.Context, nsID uuid.UUID, etcdConfig string) error

	GetClusterByRegionAndName(ctx context.Context, region, name string) (*apigen_mgmt.Tenant, error)

	/* Cluster User */

	GetClusterUser(ctx context.Context, clusterNsID uuid.UUID, username string) (*apigen_mgmt.DBUser, error)

	CreateCluserUser(ctx context.Context, clusterNsID uuid.UUID, username, password string, createDB, superUser bool) (*apigen_mgmt.DBUser, error)

	UpdateClusterUserPassword(ctx context.Context, clusterNsID uuid.UUID, username, password string) error

	DeleteClusterUser(ctx context.Context, clusterNsID uuid.UUID, username string) error

	/* Private Link */

	GetPrivateLinks(ctx context.Context) ([]PrivateLinkInfo, error)

	// GetPrivateLink returns the private link and its cluster ID by the given private link ID.
	GetPrivateLink(ctx context.Context, privateLinkID uuid.UUID) (*PrivateLinkInfo, error)

	// CreatePrivateLinkAwait creates the private link and waits for the creation to complete.
	CreatePrivateLinkAwait(ctx context.Context, clusterNsID uuid.UUID, req apigen_mgmt.PostPrivateLinkRequestBody) (*PrivateLinkInfo, error)

	// DeletePrivateLinkAwait deletes the private link and waits for the deletion to complete. it
	// returns nil if the private link is deleted successfully or not found.
	DeletePrivateLinkAwait(ctx context.Context, clusterNsID uuid.UUID, privateLinkID uuid.UUID) error
}

type CloudClient struct {
	Endpoint   string
	accClient  *apigen_acc.ClientWithResponses
	apiKeyPair string
	regions    map[string]RegionServiceClientInterface
}

func NewCloudClient(ctx context.Context, endpoint, apiKey, apiSecret, tfPluginVersion string) (CloudClientInterface, error) {
	apiKeyPair := fmt.Sprintf("%s:%s", apiKey, apiSecret)

	requestEditor := func(ctx context.Context, req *http.Request) error {
		req.Header.Set("X-API-KEY", apiKeyPair)
		req.Header.Set("User-Agent", fmt.Sprintf("terraform-provider-risingwavecloud/%s", tfPluginVersion))
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
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK); err != nil {
		return nil, errors.Wrapf(err, "message %s", string(res.Body))
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
		rs, err := createRegionServiceClient(region.Url, requestEditor)
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

func createRegionServiceClient(url string, reqEditor func(ctx context.Context, req *http.Request) error) (RegionServiceClientInterface, error) {
	mgmtClient, err := apigen_mgmt.NewClientWithResponses(url, apigen_mgmt.WithRequestEditorFn(reqEditor))
	if err != nil {
		return nil, err
	}
	return &RegionServiceClient{
		mgmtClient,
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

func (c *CloudClient) GetClusterByNsID(ctx context.Context, nsID uuid.UUID) (*apigen_mgmt.Tenant, error) {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, nsID)
	if err != nil {
		return nil, err
	}

	return rs.GetClusterByID(ctx, info.Id)
}

func (c *CloudClient) IsTenantNameExist(ctx context.Context, region string, tenantName string) (bool, error) {
	rs, err := c.getRegionClient(region)
	if err != nil {
		return false, err
	}

	return rs.IsTenantNameExist(ctx, tenantName)
}

func (c *CloudClient) Ping(ctx context.Context) error {
	res, err := c.accClient.GetAuthPingWithResponse(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to ping endpoint")
	}
	if res.StatusCode() == http.StatusForbidden {
		return ErrInvalidCredential
	}
	if err := apigen.ExpectStatusCodeWithMessage(res, http.StatusOK); err != nil {
		return errors.Wrapf(err, "message %s", string(res.Body))
	}
	return nil
}

func (c *CloudClient) CreateClusterAwait(ctx context.Context, region string, req apigen_mgmt.TenantRequestRequestBody) (*apigen_mgmt.Tenant, error) {
	rs, err := c.getRegionClient(region)
	if err != nil {
		return nil, err
	}

	return rs.CreateClusterAwait(ctx, req)
}

func (c *CloudClient) GetTiers(ctx context.Context, region string) ([]apigen_mgmt.Tier, error) {
	rs, err := c.getRegionClient(region)
	if err != nil {
		return nil, err
	}

	return rs.GetTiers(ctx)
}

func (c *CloudClient) GetAvailableComponentTypes(ctx context.Context, region string, targetTier apigen_mgmt.TierId, component string) ([]apigen_mgmt.AvailableComponentType, error) {
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

	return rs.DeleteClusterAwait(ctx, info.Id)
}

func (c *CloudClient) UpdateClusterImageByNsIDAwait(ctx context.Context, nsID uuid.UUID, version string) error {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, nsID)
	if err != nil {
		return err
	}

	return rs.UpdateClusterImageAwait(ctx, info.Id, version)
}

func (c *CloudClient) UpdateClusterResourcesByNsIDAwait(ctx context.Context, nsID uuid.UUID, req apigen_mgmt.PostTenantResourcesRequestBody) error {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, nsID)
	if err != nil {
		return err
	}

	return rs.UpdateClusterResourcesAwait(ctx, info.Id, req)
}

func (c *CloudClient) UpdateRisingWaveConfigByNsIDAwait(ctx context.Context, nsID uuid.UUID, rwConfig string) error {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, nsID)
	if err != nil {
		return err
	}

	return rs.UpdateRisingWaveConfigAwait(ctx, info.Id, rwConfig)
}

func (c *CloudClient) UpdateEtcdConfigByNsIDAwait(ctx context.Context, nsID uuid.UUID, etcdConfig string) error {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, nsID)
	if err != nil {
		return err
	}

	return rs.UpdateEtcdConfigAwait(ctx, info.Id, etcdConfig)
}

func (c *CloudClient) GetClusterUser(ctx context.Context, clusterNsID uuid.UUID, username string) (*apigen_mgmt.DBUser, error) {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return nil, err
	}
	users, err := rs.GetClusterUsers(ctx, info.Id)
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

func (c *CloudClient) CreateCluserUser(ctx context.Context, clusterNsID uuid.UUID, username, password string, createDB, superUser bool) (*apigen_mgmt.DBUser, error) {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return nil, err
	}
	u, err := rs.CreateCluserUser(ctx, apigen_mgmt.CreateDBUserRequestBody{
		TenantId:  info.Id,
		Username:  username,
		Password:  password,
		Createdb:  createDB,
		Superuser: superUser,
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
	return rs.UpdateClusterUserPassword(ctx, info.Id, username, password)
}

func (c *CloudClient) DeleteClusterUser(ctx context.Context, clusterNsID uuid.UUID, username string) error {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return err
	}
	return rs.DeleteClusterUser(ctx, info.Id, username)
}

type PrivateLinkInfo struct {
	ClusterNsID uuid.UUID
	PrivateLink *apigen_mgmt.PrivateLink
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
		if err = apigen.ExpectStatusCodeWithMessage(res, http.StatusOK); err != nil {
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
		pl, err := rs.GetPrivateLink(ctx, accpl.TenantId, accpl.Id)
		if err != nil {
			return nil, err
		}
		cluster, err := rs.GetClusterByID(ctx, accpl.TenantId)
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

func (c *CloudClient) CreatePrivateLinkAwait(ctx context.Context, clusterNsID uuid.UUID, req apigen_mgmt.PostPrivateLinkRequestBody) (*PrivateLinkInfo, error) {
	info, rs, err := c.getClusterInfoAndRegionClient(ctx, clusterNsID)
	if err != nil {
		return nil, err
	}
	pl, err := rs.CreatePrivateLinkAwait(ctx, info.Id, req)
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
	return rs.DeletePrivateLinkAwait(ctx, info.Id, privateLinkID)
}

func (c *CloudClient) GetClusterByRegionAndName(ctx context.Context, region, name string) (*apigen_mgmt.Tenant, error) {
	rs, err := c.getRegionClient(region)
	if err != nil {
		return nil, err
	}
	return rs.GetClusterByName(ctx, name)
}
