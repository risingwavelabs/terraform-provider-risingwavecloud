package fake

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk"
	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk/apigen/mgmt"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/utils/ptr"
)

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

type GlobalState struct {
	regionStates map[string]*RegionState
}

func (g *GlobalState) GetRegionState(region string) *RegionState {
	if _, ok := g.regionStates[region]; !ok {
		g.regionStates[region] = &RegionState{}
	}
	return g.regionStates[region]
}

func (g *GlobalState) GetClusterByNsID(nsID uuid.UUID) (apigen_mgmt.Tenant, error) {
	for _, r := range g.regionStates {
		cluster, err := r.GetClusterByNsID(nsID)
		if err == nil {
			return cluster, nil
		}
	}
	return apigen_mgmt.Tenant{}, cloudsdk.ErrClusterNotFound
}

func (g *GlobalState) DeleteClusterByNsID(nsID uuid.UUID) error {
	for _, r := range g.regionStates {
		r.DeleteCluster(nsID)
	}
	return nil
}

func (g *GlobalState) GetNsIDByRegionAndName(region, name string) uuid.UUID {
	r := g.GetRegionState(region)
	for _, c := range r.GetClusters() {
		if c.TenantName == name {
			return c.NsId
		}
	}
	return uuid.UUID{}
}

var state GlobalState

func init() {
	state = GlobalState{
		regionStates: map[string]*RegionState{},
	}
}

func GetFakerState() *GlobalState {
	return &state
}

type RegionState struct {
	clusters []apigen_mgmt.Tenant
	mu       sync.RWMutex
}

func (r *RegionState) GetClusters() []apigen_mgmt.Tenant {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.clusters
}

func (r *RegionState) GetClusterByNsID(nsID uuid.UUID) (apigen_mgmt.Tenant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.clusters {
		if c.NsId == nsID {
			return c, nil
		}
	}
	return apigen_mgmt.Tenant{}, cloudsdk.ErrClusterNotFound
}
func (s *RegionState) AddCluster(cluster apigen_mgmt.Tenant) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clusters = append(s.clusters, cluster)
}

func (s *RegionState) DeleteCluster(nsID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.clusters {
		if c.NsId == nsID {
			s.clusters = append(s.clusters[:i], s.clusters[i+1:]...)
			return
		}
	}
}

func (s *RegionState) ReplaceCluster(nsID uuid.UUID, cluster apigen_mgmt.Tenant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.clusters {
		if c.NsId == nsID {
			s.clusters[i] = cluster
			return
		}
	}
}

func NewCloudClient() *FakeCloudClient {
	return &FakeCloudClient{}
}

type FakeCloudClient struct {
}

func (acc *FakeCloudClient) Ping(context.Context) error {
	return nil
}

func (acc *FakeCloudClient) GetClusterByNsID(ctx context.Context, nsID uuid.UUID) (*apigen_mgmt.Tenant, error) {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(nsID)
	if err != nil {
		return nil, err
	}
	return &cluster, nil
}

func (acc *FakeCloudClient) IsTenantNameExist(ctx context.Context, region string, tenantName string) (bool, error) {
	debugFuncCaller()

	r := state.GetRegionState(region)
	for _, c := range r.GetClusters() {
		if c.TenantName == tenantName {
			return true, nil
		}
	}
	return false, nil
}

func (acc *FakeCloudClient) CreateClusterAwait(ctx context.Context, region string, req apigen_mgmt.TenantRequestRequestBody) (*apigen_mgmt.Tenant, error) {
	debugFuncCaller()

	r := state.GetRegionState(region)
	cluster := apigen_mgmt.Tenant{
		Id:         uint64(len(r.GetClusters()) + 1),
		TenantName: req.TenantName,
		ImageTag:   *req.ImageTag,
		Region:     region,
		RwConfig:   *req.RwConfig,
		EtcdConfig: *req.EtcdConfig,
		Resources:  reqResouceToClusterResource(req.Resources),
		NsId:       uuid.New(),
	}
	r.AddCluster(cluster)
	return &cluster, nil
}

var availableComponentTypes = []apigen_mgmt.AvailableComponentType{
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

func (acc *FakeCloudClient) GetTiers(ctx context.Context, _ string) ([]apigen_mgmt.Tier, error) {
	return []apigen_mgmt.Tier{
		{
			Id:                              ptr.Ptr(apigen_mgmt.Standard),
			AvailableMetaNodes:              availableComponentTypes,
			AvailableComputeNodes:           availableComponentTypes,
			AvailableCompactorNodes:         availableComponentTypes,
			AvailableEtcdNodes:              availableComponentTypes,
			AvailableFrontendNodes:          availableComponentTypes,
			AllowEnableComputeNodeFileCache: true,
			MaximumEtcdSizeGiB:              20,
		},
	}, nil
}

func (acc *FakeCloudClient) GetAvailableComponentTypes(ctx context.Context, region string, targetTier apigen_mgmt.TierId, component string) ([]apigen_mgmt.AvailableComponentType, error) {
	tiers, err := acc.GetTiers(ctx, region)
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
	case cloudsdk.ComponentCompute:
		return tier.AvailableComputeNodes, nil
	case cloudsdk.ComponentCompactor:
		return tier.AvailableCompactorNodes, nil
	case cloudsdk.ComponentFrontend:
		return tier.AvailableFrontendNodes, nil
	case cloudsdk.ComponentMeta:
		return tier.AvailableMetaNodes, nil
	case cloudsdk.ComponentEtcd:
		return tier.AvailableEtcdNodes, nil
	}
	return nil, errors.Errorf("component %s not found", component)
}

func (acc *FakeCloudClient) DeleteClusterByNsIDAwait(ctx context.Context, nsID uuid.UUID) error {
	debugFuncCaller()

	return state.DeleteClusterByNsID(nsID)
}

func (acc *FakeCloudClient) UpdateClusterImageByNsIDAwait(ctx context.Context, nsID uuid.UUID, version string) error {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(nsID)
	if err != nil {
		return err
	}
	cluster.ImageTag = version
	r := state.GetRegionState(cluster.Region)
	r.ReplaceCluster(nsID, cluster)
	return nil
}

func (acc *FakeCloudClient) UpdateClusterResourcesByNsIDAwait(ctx context.Context, nsID uuid.UUID, req apigen_mgmt.PostTenantResourcesRequestBody) error {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(nsID)
	if err != nil {
		return err
	}
	cluster.Resources.Components.Compactor = componentReqToComponent(req.Compactor)
	cluster.Resources.Components.Compute = componentReqToComponent(req.Compute)
	cluster.Resources.Components.Frontend = componentReqToComponent(req.Frontend)
	cluster.Resources.Components.Meta = componentReqToComponent(req.Meta)
	r := state.GetRegionState(cluster.Region)
	r.ReplaceCluster(nsID, cluster)
	return nil
}

func (acc *FakeCloudClient) UpdateRisingWaveConfigByNsIDAwait(ctx context.Context, nsID uuid.UUID, rwConfig string) error {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(nsID)
	if err != nil {
		return err
	}
	cluster.RwConfig = rwConfig
	r := state.GetRegionState(cluster.Region)
	r.ReplaceCluster(nsID, cluster)
	return nil
}

func (acc *FakeCloudClient) UpdateEtcdConfigByNsIDAwait(ctx context.Context, nsID uuid.UUID, etcdConfig string) error {
	debugFuncCaller()

	cluster, err := state.GetClusterByNsID(nsID)
	if err != nil {
		return err
	}
	cluster.EtcdConfig = etcdConfig
	r := state.GetRegionState(cluster.Region)
	r.ReplaceCluster(nsID, cluster)
	return nil
}

func reqResouceToClusterResource(reqResource *apigen_mgmt.TenantResourceRequest) apigen_mgmt.TenantResource {
	return apigen_mgmt.TenantResource{
		Components: apigen_mgmt.TenantResourceComponents{
			Compute:   componentReqToComponent(reqResource.Components.Compute),
			Compactor: componentReqToComponent(reqResource.Components.Compactor),
			Frontend:  componentReqToComponent(reqResource.Components.Frontend),
			Meta:      componentReqToComponent(reqResource.Components.Meta),
			Etcd:      *componentReqToComponent(&reqResource.Components.Etcd),
		},
		EnableComputeFileCache:  reqResource.EnableComputeFileCache,
		EtcdVolumeSizeGiB:       reqResource.EtcdVolumeSizeGiB,
		ComputeFileCacheSizeGiB: reqResource.ComputeFileCacheSizeGiB,
	}

}

func componentReqToComponent(req *apigen_mgmt.ComponentResourceRequest) *apigen_mgmt.ComponentResource {
	for _, c := range availableComponentTypes {
		if c.Id == req.ComponentTypeId {
			return &apigen_mgmt.ComponentResource{
				ComponentTypeId: req.ComponentTypeId,
				Replica:         req.Replica,
				Cpu:             c.Cpu,
				Memory:          c.Memory,
			}
		}
	}
	return nil
}
