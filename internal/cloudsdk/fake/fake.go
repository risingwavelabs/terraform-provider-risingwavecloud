package fake

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen_mgmtv1 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v1"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/ptr"
)

// defaultResourceGroup is the resource group that every cluster has, it is not tracked in
// the fake resource group state because its lifecycle is bound to the cluster.
const defaultResourceGroup = "default"

// allowedIamRoleArnPattern mirrors the format the platform enforces.
var allowedIamRoleArnPattern = regexp.MustCompile(`^arn:aws:iam::\d{12}:role/\S+$`)

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
			Id:                       ptr.Ptr(apigen_mgmtv1.Standard),
			AvailableStandaloneNodes: availableComponentTypes,
			AvailableMetaStore:       availableMetaStore,
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
	case cloudsdk.ComponentStandalone:
		return tier.AvailableStandaloneNodes, nil
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
	// The platform wants at least one component in a resource update, so a caller that filtered
	// every component out -- because nothing but an extension changed -- is refused rather than
	// quietly accepted.
	if req.Compute == nil && req.Compactor == nil && req.Frontend == nil &&
		req.Meta == nil && req.Standalone == nil {
		return errors.New("at least one resource must be provided")
	}

	// The resource endpoint owns serverless compaction as well: it reads the concurrency out of
	// the request, compares it with what the extension has, and a request that says nothing
	// about the extension counts as asking for zero -- which disables it. Reproducing that is
	// what lets a mock run catch a rescale that forgets to restate the extension.
	requested := 0
	if req.Extensions != nil && req.Extensions.ServerlessCompaction != nil {
		requested = req.Extensions.ServerlessCompaction.MaximumCompactionConcurrency
	}
	switch current := cluster.GetServerlessCompaction(); {
	case current == nil && requested > 0:
		// Enabling through this endpoint is the case that hid a real defect: a rescale carrying
		// the planned concurrency enabled the extension here, and the explicit enable that
		// followed was refused because it was already running.
		cluster.SetServerlessCompaction(&apigen_mgmtv2.GetTenantExtensionCompactionResponseBody{
			MaximumCompactionConcurrency: &requested,
			Status:                       cloudsdk.ExtensionStatusRunning,
		})
	case current == nil:
		// nothing enabled, nothing asked for
	case requested == 0:
		cluster.SetServerlessCompaction(nil)
	case current.MaximumCompactionConcurrency == nil || *current.MaximumCompactionConcurrency != requested:
		updated := *current
		updated.MaximumCompactionConcurrency = &requested
		cluster.SetServerlessCompaction(&updated)
	}

	// The platform rejects a component with no replicas, so the fake does too: a caller that
	// restates an unchanged compactor while serverless compaction holds it at zero would be
	// refused there, and a mock run that accepted it would hide that.
	for name, comp := range map[string]*apigen_mgmtv2.ComponentResourceRequest{
		"compactor": req.Compactor, "compute": req.Compute,
		"frontend": req.Frontend, "meta": req.Meta, "standalone": req.Standalone,
	} {
		if comp != nil && comp.Replica <= 0 {
			return errors.Errorf("request %d replica(s) not valid for %s. Exceeds the maximum allowed value or <= 0",
				comp.Replica, name)
		}
	}

	// A component the request leaves out keeps what it had. That is what the platform does --
	// `update_resources.go` starts from the tenant's spec and assigns only the components it was
	// given -- and it is what lets a caller change one component without restating the others,
	// which matters for a cluster whose compactor the serverless compaction extension holds at
	// zero replicas: restating that would be refused.
	components := &cluster.GetTenant().Resources.Components
	assign := func(dst **apigen_mgmtv2.ComponentResource, req *apigen_mgmtv2.ComponentResourceRequest) {
		if req == nil {
			return
		}
		*dst = componentReqToComponent(req)
	}
	assign(&components.Compactor, req.Compactor)
	assign(&components.Compute, req.Compute)
	assign(&components.Frontend, req.Frontend)
	assign(&components.Meta, req.Meta)
	assign(&components.Standalone, req.Standalone)
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

func (acc *FakeCloudClient) CreateClusterUser(ctx context.Context, nsID uuid.UUID, username, password string, createDB, superUser, createUser bool) (*apigen_mgmtv2.DBUser, error) {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(nsID)
	if err != nil {
		return nil, err
	}

	dbuser := &apigen_mgmtv2.DBUser{
		Usecreatedb:   createDB,
		Username:      username,
		Usesysid:      uint64((time.Now().Unix() << 10) + int64(rand.Int31n(1024))),
		Usesuper:      superUser,
		Usecreateuser: createUser,
		// the platform creates users with CREATE USER, which implies LOGIN.
		Canlogin: true,
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
			Compute:    componentReqToComponent(reqResource.Components.Compute),
			Compactor:  componentReqToComponent(reqResource.Components.Compactor),
			Frontend:   componentReqToComponent(reqResource.Components.Frontend),
			Meta:       componentReqToComponent(reqResource.Components.Meta),
			Standalone: componentReqToComponent(reqResource.Components.Standalone),
		},
		ComputeCache: func() apigen_mgmtv2.TenantResourceComputeCache {
			if reqResource.ComputeCache != nil {
				return *reqResource.ComputeCache
			}
			return apigen_mgmtv2.TenantResourceComputeCache{}
		}(),
	}
	if reqResource.MetaStore != nil {
		ret.MetaStore = &apigen_mgmtv2.TenantResourceMetaStore{
			Type: reqResource.MetaStore.Type,
		}
	} else {
		// Real API always assigns a default metastore
		ret.MetaStore = &apigen_mgmtv2.TenantResourceMetaStore{
			Type: apigen_mgmtv2.Postgresql,
		}
	}

	return ret
}

func componentReqToComponent(req *apigen_mgmtv2.ComponentResourceRequest) *apigen_mgmtv2.ComponentResource {
	if req == nil {
		return nil
	}
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

func (acc *FakeCloudClient) GetResourceGroup(ctx context.Context, clusterNsID uuid.UUID, name string) (*apigen_mgmtv2.ResourceGroupDetails, error) {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return nil, err
	}
	return c.GetResourceGroup(name)
}

// resolveComputeCache resolves the compute cache size from the component type.
//
// This deliberately diverges from the platform, which currently returns a constant 100 GB
// regardless of the component type (verified against prod us-east-1 for p-1c4g and p-2c8g).
// Varying it here is what gives the acceptance test a component type change that also changes
// a computed attribute, which is the case the compute_cache_size_gb plan modifier handles.
func resolveComputeCache(componentTypeID string, requested *apigen_mgmtv2.TenantResourceComputeCache) apigen_mgmtv2.TenantResourceComputeCache {
	if requested != nil {
		return *requested
	}
	for _, c := range availableComponentTypes {
		if c.Id == componentTypeID {
			cpu, err := strconv.Atoi(c.Cpu)
			if err != nil {
				break
			}
			return apigen_mgmtv2.TenantResourceComputeCache{SizeGb: cpu * 20}
		}
	}
	return apigen_mgmtv2.TenantResourceComputeCache{SizeGb: 20}
}

func reqResourceGroupToDetails(req apigen_mgmtv2.ComponentResourceRequest, computeCache *apigen_mgmtv2.TenantResourceComputeCache, name string) *apigen_mgmtv2.ResourceGroupDetails {
	var resource apigen_mgmtv2.ComponentResource
	if comp := componentReqToComponent(&req); comp != nil {
		resource = *comp
	} else {
		resource = apigen_mgmtv2.ComponentResource{
			ComponentTypeId: req.ComponentTypeId,
			Replica:         req.Replica,
		}
	}
	return &apigen_mgmtv2.ResourceGroupDetails{
		Name:         name,
		Resource:     resource,
		ComputeCache: resolveComputeCache(req.ComponentTypeId, computeCache),
	}
}

func (acc *FakeCloudClient) CreateResourceGroupAwait(ctx context.Context, clusterNsID uuid.UUID, req apigen_mgmtv2.CreateResourceGroupsRequestBody) (*apigen_mgmtv2.ResourceGroupDetails, error) {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return nil, err
	}
	if req.Name == defaultResourceGroup {
		return nil, errors.Errorf("the %s resource group already exists", defaultResourceGroup)
	}
	// the platform reserves this prefix for its serverless backfill extension.
	if strings.HasPrefix(req.Name, "backfill") {
		return nil, errors.New("Invalid name, resource group name cannot start with 'backfill'")
	}
	if _, err := c.GetResourceGroup(req.Name); err == nil {
		return nil, errors.Errorf("resource group %s already exists", req.Name)
	}
	g := reqResourceGroupToDetails(req.Resource, req.ComputeCache, req.Name)
	c.AddResourceGroup(g)
	return g, nil
}

func (acc *FakeCloudClient) UpdateResourceGroupAwait(ctx context.Context, clusterNsID uuid.UUID, resourceGroup string, req apigen_mgmtv2.UpdateResourceGroupsRequestBody) (*apigen_mgmtv2.ResourceGroupDetails, error) {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return nil, err
	}
	previous, err := c.GetResourceGroup(resourceGroup)
	if err != nil {
		return nil, err
	}
	g := reqResourceGroupToDetails(req.Resource, nil, resourceGroup)
	g.DatabaseCount = previous.DatabaseCount
	c.AddResourceGroup(g)
	return g, nil
}

func (acc *FakeCloudClient) DeleteResourceGroupAwait(ctx context.Context, clusterNsID uuid.UUID, resourceGroup string) error {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return err
	}
	c.DeleteResourceGroup(resourceGroup)
	return nil
}

func (acc *FakeCloudClient) GetAllowedIamRoles(ctx context.Context, clusterNsID uuid.UUID) ([]string, error) {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return nil, err
	}
	return c.GetAllowedIamRoles(), nil
}

func (acc *FakeCloudClient) AddAllowedIamRoleAwait(ctx context.Context, clusterNsID uuid.UUID, roleArn string) error {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return err
	}
	// the platform validates the shape of the ARN before anything else
	if !allowedIamRoleArnPattern.MatchString(roleArn) {
		return errors.Errorf("validate arn failed: invalid target format, expected format: arn:aws:iam::{account}:role/{role_name}")
	}
	c.AddAllowedIamRole(roleArn)
	return nil
}

func (acc *FakeCloudClient) RemoveAllowedIamRoleAwait(ctx context.Context, clusterNsID uuid.UUID, roleArn string) error {
	debugFuncCaller()

	c, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return err
	}
	c.RemoveAllowedIamRole(roleArn)
	return nil
}

//
// Tenant extensions.
//
// The fake applies a change immediately rather than reporting the transient statuses the
// platform passes through, so a mock run exercises the resources rather than the waits. The
// one behaviour it does reproduce is the important one for the resources: an extension that
// was never enabled reads back as disabled instead of as a missing object.
//

// refuseOnStandalone mirrors the platform, which answers every extension request for a
// standalone cluster with a 412 before it looks at anything else -- including the reads. A fake
// that answered "disabled" instead would hide the one failure that reaches clusters nobody has
// enabled an extension on.
func refuseOnStandalone(cluster *ClusterState, extension string) error {
	if cluster.GetTenant().Resources.Components.Standalone == nil {
		return nil
	}
	return errors.Errorf(
		"the platform refused the %s extension: extensions need a cluster with a separate compute component, "+
			"which a standalone cluster does not have", extension)
}

func (acc *FakeCloudClient) GetServerlessCompaction(ctx context.Context, clusterNsID uuid.UUID) (*apigen_mgmtv2.GetTenantExtensionCompactionResponseBody, error) {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return nil, err
	}
	if err := refuseOnStandalone(cluster, "serverless compaction"); err != nil {
		return nil, err
	}
	ext := cluster.GetServerlessCompaction()
	if ext == nil {
		return nil, cloudsdk.ErrExtensionDisabled
	}
	return ext, nil
}

func (acc *FakeCloudClient) EnableServerlessCompactionAwait(ctx context.Context, clusterNsID uuid.UUID, req apigen_mgmtv2.TenantExtensionServerlessCompactionRequest) error {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return err
	}
	if err := refuseOnStandalone(cluster, "serverless_compaction"); err != nil {
		return err
	}
	// The platform refuses to enable an extension that is already running, which is how a caller
	// finds out it enabled it somewhere else -- through the resource endpoint, say.
	if current := cluster.GetServerlessCompaction(); current != nil {
		return errors.Errorf("Illegal status: %s, cannot enable extensions compaction", current.Status)
	}
	cluster.SetServerlessCompaction(&apigen_mgmtv2.GetTenantExtensionCompactionResponseBody{
		MaximumCompactionConcurrency: ptr.Ptr(req.MaximumCompactionConcurrency),
		Status:                       cloudsdk.ExtensionStatusRunning,
		Version:                      req.Version,
	})
	return nil
}

// UpdateServerlessCompactionAwait changes an extension that is already running. It is not the
// same call as enabling one -- the platform refuses each in the other's situation -- so it
// cannot delegate to it.
func (acc *FakeCloudClient) UpdateServerlessCompactionAwait(ctx context.Context, clusterNsID uuid.UUID, req apigen_mgmtv2.TenantExtensionServerlessCompactionRequest) error {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return err
	}
	if err := refuseOnStandalone(cluster, "serverless_compaction"); err != nil {
		return err
	}
	if cluster.GetServerlessCompaction() == nil {
		return errors.Wrapf(cloudsdk.ErrExtensionDisabled,
			"the serverless_compaction extension of cluster %s is not enabled", clusterNsID)
	}
	cluster.SetServerlessCompaction(&apigen_mgmtv2.GetTenantExtensionCompactionResponseBody{
		MaximumCompactionConcurrency: ptr.Ptr(req.MaximumCompactionConcurrency),
		Status:                       cloudsdk.ExtensionStatusRunning,
		Version:                      req.Version,
	})
	return nil
}

func (acc *FakeCloudClient) DisableServerlessCompactionAwait(ctx context.Context, clusterNsID uuid.UUID) error {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return err
	}
	cluster.SetServerlessCompaction(nil)
	return nil
}

func (acc *FakeCloudClient) GetServerlessBackfill(ctx context.Context, clusterNsID uuid.UUID) (*apigen_mgmtv2.GetTenantExtensionServerlessBackfillResponseBody, error) {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return nil, err
	}
	if err := refuseOnStandalone(cluster, "serverless backfill"); err != nil {
		return nil, err
	}
	ext := cluster.GetServerlessBackfill()
	if ext == nil {
		return nil, cloudsdk.ErrExtensionDisabled
	}
	return ext, nil
}

func (acc *FakeCloudClient) EnableServerlessBackfillAwait(ctx context.Context, clusterNsID uuid.UUID, req apigen_mgmtv2.TenantExtensionServerlessBackfillRequest) error {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return err
	}
	cluster.SetServerlessBackfill(&apigen_mgmtv2.GetTenantExtensionServerlessBackfillResponseBody{
		Resources: componentReqToComponent(&req.Resources),
		Status:    cloudsdk.ExtensionStatusRunning,
	})
	return nil
}

func (acc *FakeCloudClient) UpdateServerlessBackfillAwait(ctx context.Context, clusterNsID uuid.UUID, req apigen_mgmtv2.TenantExtensionServerlessBackfillRequest) error {
	debugFuncCaller()

	return acc.EnableServerlessBackfillAwait(ctx, clusterNsID, req)
}

func (acc *FakeCloudClient) DisableServerlessBackfillAwait(ctx context.Context, clusterNsID uuid.UUID) error {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return err
	}
	cluster.SetServerlessBackfill(nil)
	return nil
}

func (acc *FakeCloudClient) GetIcebergCompaction(ctx context.Context, clusterNsID uuid.UUID) (*apigen_mgmtv2.IcebergCompaction, error) {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return nil, err
	}
	if err := refuseOnStandalone(cluster, "iceberg compaction"); err != nil {
		return nil, err
	}
	ext := cluster.GetIcebergCompaction()
	if ext == nil {
		return nil, cloudsdk.ErrExtensionDisabled
	}
	return ext, nil
}

func (acc *FakeCloudClient) EnableIcebergCompactionAwait(ctx context.Context, clusterNsID uuid.UUID, req apigen_mgmtv2.PostTenantsNsIdExtensionsIcebergCompactionJSONRequestBody) error {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return err
	}
	cluster.SetIcebergCompaction(&apigen_mgmtv2.IcebergCompaction{
		Config:    req.Config,
		Resources: componentReqToComponent(req.Resources),
		Status:    cloudsdk.ExtensionStatusRunning,
	})
	return nil
}

func (acc *FakeCloudClient) UpdateIcebergCompactionAwait(ctx context.Context, clusterNsID uuid.UUID, req apigen_mgmtv2.PutTenantsNsIdExtensionsIcebergCompactionJSONRequestBody) error {
	debugFuncCaller()

	return acc.EnableIcebergCompactionAwait(ctx, clusterNsID, apigen_mgmtv2.PostTenantsNsIdExtensionsIcebergCompactionJSONRequestBody(req))
}

func (acc *FakeCloudClient) DisableIcebergCompactionAwait(ctx context.Context, clusterNsID uuid.UUID) error {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(clusterNsID)
	if err != nil {
		return err
	}
	cluster.SetIcebergCompaction(nil)
	return nil
}
