package fake

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen_mgmtv1 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v1"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/ptr"
)

func UseFakeBackend() bool {
	return len(os.Getenv("RWC_MOCK")) != 0
}

func debugFuncCaller() {
	for _, stack := range []int{1, 2} {
		stmt := "faker stack trace: "
		pc, file, line, ok := runtime.Caller(stack)
		if ok {
			if fn := runtime.FuncForPC(pc); fn != nil {
				tmp := strings.Split(fn.Name(), "/")
				stmt += tmp[len(tmp)-1]
			} else {
				stmt += "<unknown function>"
			}
			stmt += fmt.Sprintf(", %s:%d", file, line)
		}
		log.Default().Println(stmt)
	}
	log.Default().Println()
}

func NewCloudClient() *FakeCloudClient {
	return &FakeCloudClient{}
}

type FakeCloudClient struct {
}

func (acc *FakeCloudClient) Ping(context.Context) error {
	return nil
}

func (acc *FakeCloudClient) GetClusterByRegionAndName(ctx context.Context, region, name string) (*apigen_mgmtv2.Tenant, error) {
	debugFuncCaller()

	r := state.GetRegionState(region)
	for _, c := range r.clusters {
		if c.tenant.TenantName == name {
			return c.tenant, nil
		}
	}
	return nil, errors.Wrapf(cloudsdk.ErrClusterNotFound, "cluster %s not found", name)
}

func (acc *FakeCloudClient) GetClusterByNsID(ctx context.Context, nsID uuid.UUID) (*apigen_mgmtv2.Tenant, error) {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(nsID)
	if err != nil {
		return nil, err
	}
	return cluster.tenant, nil
}

func (acc *FakeCloudClient) CreateClusterAwait(ctx context.Context, region string, req apigen_mgmtv2.TenantRequestRequestBody) (*apigen_mgmtv2.Tenant, error) {
	debugFuncCaller()

	clusterName := req.ClusterName
	if clusterName == nil {
		clusterName = ptr.Ptr("default-control-plane")
	}

	r := state.GetRegionState(region)
	t := &apigen_mgmtv2.Tenant{
		Id:          uint64(len(r.GetClusters()) + 1),
		TenantName:  req.TenantName,
		ImageTag:    *req.ImageTag,
		Region:      region,
		RwConfig:    *req.RwConfig,
		Resources:   reqResouceToClusterResource(req.Resources),
		NsId:        uuid.New(),
		Tier:        *req.Tier,
		ClusterName: *clusterName,
	}

	cluster := NewClusterState(t)
	r.AddCluster(cluster)
	return t, nil
}

var availableComponentTypes = []apigen_mgmtv1.AvailableComponentType{
	{
		Id:      "p-1c4g",
		Cpu:     "1",
		Memory:  "4 GB",
		Maximum: 3,
	},
	{
		Id:      "p-2c8g",
		Cpu:     "2",
		Memory:  "8 GB",
		Maximum: 3,
	},
}

var availableMetaStore = &apigen_mgmtv1.AvailableMetaStore{
	Postgresql: &apigen_mgmtv1.AvailableMetaStorePostgreSql{
		MaximumSizeGiB: 20,
		Nodes:          availableComponentTypes,
	},
	SharingPg: &apigen_mgmtv1.AvailableMetaStoreSharingPg{
		Enabled: ptr.Ptr(true),
	},
	AwsRds: &apigen_mgmtv1.AvailableMetaStoreAwsRds{
		Enabled: ptr.Ptr(true),
	},
	AzrPostgres: &apigen_mgmtv1.AvailableMetaStoreAzrPostgres{
		Enabled: ptr.Ptr(true),
	},
	GcpCloudsql: &apigen_mgmtv1.AvailableMetaStoreGcpCloudSql{
		Enabled: ptr.Ptr(true),
	},
}

func (acc *FakeCloudClient) GetTiers(ctx context.Context, _ string) ([]apigen_mgmtv1.Tier, error) {
	return []apigen_mgmtv1.Tier{
		{
			Id:                      ptr.Ptr(apigen_mgmtv1.Standard),
			AvailableMetaNodes:      availableComponentTypes,
			AvailableComputeNodes:   availableComponentTypes,
			AvailableCompactorNodes: availableComponentTypes,
			AvailableFrontendNodes:  availableComponentTypes,
			AvailableMetaStore:      availableMetaStore,
		},
		{
			Id:                      ptr.Ptr(apigen_mgmtv1.BYOC),
			AvailableMetaNodes:      availableComponentTypes,
			AvailableComputeNodes:   availableComponentTypes,
			AvailableCompactorNodes: availableComponentTypes,
			AvailableFrontendNodes:  availableComponentTypes,
			AvailableMetaStore:      availableMetaStore,
		},
		{
			Id:                      ptr.Ptr(apigen_mgmtv1.Invited),
			AvailableMetaNodes:      availableComponentTypes,
			AvailableComputeNodes:   availableComponentTypes,
			AvailableCompactorNodes: availableComponentTypes,
			AvailableFrontendNodes:  availableComponentTypes,
			AvailableMetaStore:      availableMetaStore,
		},
	}, nil
}

func (acc *FakeCloudClient) GetAvailableComponentTypes(ctx context.Context, region string, targetTier apigen_mgmtv1.TierId, component string) ([]apigen_mgmtv1.AvailableComponentType, error) {
	tiers, err := acc.GetTiers(ctx, region)
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
	case cloudsdk.ComponentCompute:
		return tier.AvailableComputeNodes, nil
	case cloudsdk.ComponentCompactor:
		return tier.AvailableCompactorNodes, nil
	case cloudsdk.ComponentFrontend:
		return tier.AvailableFrontendNodes, nil
	case cloudsdk.ComponentMeta:
		return tier.AvailableMetaNodes, nil
	}
	return nil, errors.Errorf("component %s not found", component)
}

func (acc *FakeCloudClient) DeleteClusterByNsIDAwait(ctx context.Context, nsID uuid.UUID) error {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(nsID)
	if err != nil {
		if errors.Is(err, cloudsdk.ErrClusterNotFound) {
			return nil
		}
	}

	state.GetRegionState(c.tenant.Region).DeleteCluster(nsID)

	return nil
}

func (acc *FakeCloudClient) UpdateClusterImageByNsIDAwait(ctx context.Context, nsID uuid.UUID, version string) error {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(nsID)
	if err != nil {
		return err
	}
	cluster.GetTenant().ImageTag = version
	r := state.GetRegionState(cluster.GetTenant().Region)

	r.ReplaceCluster(nsID, cluster)
	return nil
}

func (acc *FakeCloudClient) UpdateClusterResourcesByNsIDAwait(ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.PostTenantResourcesRequestBody) error {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(nsID)
	if err != nil {
		return err
	}
	cluster.GetTenant().Resources.Components.Compactor = componentReqToComponent(req.Compactor)
	cluster.GetTenant().Resources.Components.Compute = componentReqToComponent(req.Compute)
	cluster.GetTenant().Resources.Components.Frontend = componentReqToComponent(req.Frontend)
	cluster.GetTenant().Resources.Components.Meta = componentReqToComponent(req.Meta)
	r := state.GetRegionState(cluster.GetTenant().Region)

	r.ReplaceCluster(nsID, cluster)
	return nil
}

func (acc *FakeCloudClient) UpdateRisingWaveConfigByNsIDAwait(ctx context.Context, nsID uuid.UUID, rwConfig string) error {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(nsID)
	if err != nil {
		return err
	}
	cluster.GetTenant().RwConfig = rwConfig
	r := state.GetRegionState(cluster.GetTenant().Region)
	r.ReplaceCluster(nsID, cluster)
	return nil
}

func (acc *FakeCloudClient) GetClusterUser(ctx context.Context, nsID uuid.UUID, username string) (*apigen_mgmtv2.DBUser, error) {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(nsID)
	if err != nil {
		return nil, err
	}

	return c.GetClusterUser(username)
}

func (acc *FakeCloudClient) CreateCluserUser(ctx context.Context, nsID uuid.UUID, username, password string, createDB, superUser bool) (*apigen_mgmtv2.DBUser, error) {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(nsID)
	if err != nil {
		return nil, err
	}

	dbuser := &apigen_mgmtv2.DBUser{
		Usecreatedb: createDB,
		Username:    username,
		Usesysid:    uint64((time.Now().Unix() << 10) + int64(rand.Int31n(1024))),
		Usesuper:    superUser,
	}

	c.AddClusterUser(dbuser)

	return dbuser, nil
}

func (acc *FakeCloudClient) UpdateClusterUserPassword(ctx context.Context, nsID uuid.UUID, username, password string) error {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(nsID)
	if err != nil {
		return err
	}

	_, err = c.GetClusterUser(username)
	return err
}

func (acc *FakeCloudClient) DeleteClusterUser(ctx context.Context, nsID uuid.UUID, username string) error {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(nsID)
	if err != nil {
		return err
	}

	c.DeleteClusterUser(username)

	return nil
}

func reqResouceToClusterResource(reqResource *apigen_mgmtv2.TenantResourceRequest) apigen_mgmtv2.TenantResource {
	ret := apigen_mgmtv2.TenantResource{
		Components: apigen_mgmtv2.TenantResourceComponents{
			Compute:   componentReqToComponent(reqResource.Components.Compute),
			Compactor: componentReqToComponent(reqResource.Components.Compactor),
			Frontend:  componentReqToComponent(reqResource.Components.Frontend),
			Meta:      componentReqToComponent(reqResource.Components.Meta),
		},
		ComputeCache: apigen_mgmtv2.TenantResourceComputeCache{
			SizeGb: reqResource.ComputeFileCacheSizeGiB,
		},
	}

	return ret
}

func componentReqToComponent(req *apigen_mgmtv2.ComponentResourceRequest) *apigen_mgmtv2.ComponentResource {
	for _, c := range availableComponentTypes {
		if c.Id == req.ComponentTypeId {
			return &apigen_mgmtv2.ComponentResource{
				ComponentTypeId: req.ComponentTypeId,
				Replica:         req.Replica,
				Cpu:             c.Cpu,
				Memory:          c.Memory,
			}
		}
	}
	return nil
}

func (acc *FakeCloudClient) GetPrivateLinks(ctx context.Context) ([]cloudsdk.PrivateLinkInfo, error) {
	debugFuncCaller()

	var plis []cloudsdk.PrivateLinkInfo
	for _, r := range state.regionStates {
		for _, c := range r.GetClusters() {
			for _, pl := range c.GetPrivateLinks() {
				plis = append(plis, cloudsdk.PrivateLinkInfo{
					PrivateLink: pl,
					ClusterNsID: c.GetTenant().NsId,
				})
			}
		}
	}
	return plis, nil
}

func (acc *FakeCloudClient) GetPrivateLink(ctx context.Context, privateLinkID uuid.UUID) (*cloudsdk.PrivateLinkInfo, error) {
	debugFuncCaller()

	for _, r := range state.regionStates {
		for _, c := range r.GetClusters() {
			pl, err := c.GetPrivateLink(privateLinkID)
			if err == nil {
				return &cloudsdk.PrivateLinkInfo{
					PrivateLink: pl,
					ClusterNsID: c.GetTenant().NsId,
				}, nil
			}
		}
	}

	return nil, errors.Wrapf(cloudsdk.ErrPrivateLinkNotFound, "private link %s not found", privateLinkID)
}

func (acc *FakeCloudClient) CreatePrivateLinkAwait(ctx context.Context, clusterNsID uuid.UUID, req apigen_mgmtv2.PostPrivateLinkRequestBody) (*cloudsdk.PrivateLinkInfo, error) {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return nil, err
	}

	pl := &apigen_mgmtv2.PrivateLink{
		Id:              uuid.New(),
		ConnectionName:  req.ConnectionName,
		Target:          &req.Target,
		Endpoint:        ptr.Ptr("vpce-fakestatetest"),
		Status:          apigen_mgmtv2.CREATED,
		ConnectionState: apigen_mgmtv2.ACCEPTED,
		TenantId:        int64(c.GetTenant().Id),
	}

	c.AddPrivateLink(pl)

	return &cloudsdk.PrivateLinkInfo{
		PrivateLink: pl,
		ClusterNsID: clusterNsID,
	}, nil
}

func (acc *FakeCloudClient) DeletePrivateLinkAwait(ctx context.Context, clusterNsID uuid.UUID, privateLinkID uuid.UUID) error {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return err
	}

	c.DeletePrivateLink(privateLinkID)

	return nil
}

func (acc *FakeCloudClient) GetPrivateLinkByName(ctx context.Context, connectionName string) (*cloudsdk.PrivateLinkInfo, error) {
	debugFuncCaller()

	for _, r := range state.regionStates {
		for _, c := range r.GetClusters() {
			for _, pl := range c.GetPrivateLinks() {
				if pl.ConnectionName == connectionName {
					return &cloudsdk.PrivateLinkInfo{
						PrivateLink: pl,
						ClusterNsID: c.GetTenant().NsId,
					}, nil
				}
			}
		}
	}

	return nil, errors.Wrapf(cloudsdk.ErrPrivateLinkNotFound, "private link %s not found", connectionName)
}

func (acc *FakeCloudClient) GetBYOCCluster(ctx context.Context, region string, name string) (*apigen_mgmtv2.ManagedCluster, error) {
	debugFuncCaller()

	return &apigen_mgmtv2.ManagedCluster{
		Id:   101,
		Name: name,
		Settings: map[string]string{
			"uuid": uuid.Nil.String(),
		},
	}, nil
}
