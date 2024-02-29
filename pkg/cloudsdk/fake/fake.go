package fake

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk"
	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/cloudsdk/apigen/mgmt"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/pkg/utils/ptr"
)

type GlobalState struct {
	regionStates map[string]map[string]*RegionState
}

func (g *GlobalState) GetRegionState(platform, region string) *RegionState {
	if _, ok := g.regionStates[platform]; !ok {
		g.regionStates[platform] = map[string]*RegionState{}
	}
	if _, ok := g.regionStates[platform][region]; !ok {
		g.regionStates[platform][region] = &RegionState{}
	}
	return g.regionStates[platform][region]
}

var state = GlobalState{
	regionStates: map[string]map[string]*RegionState{},
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

func (r *RegionState) GetClusterByName(name string) (apigen_mgmt.Tenant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.clusters {
		if c.TenantName == name {
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

func (s *RegionState) DeleteCluster(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.clusters {
		if c.TenantName == name {
			s.clusters = append(s.clusters[:i], s.clusters[i+1:]...)
			return
		}
	}
}

func (s *RegionState) ReplaceCluster(name string, cluster apigen_mgmt.Tenant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.clusters {
		if c.TenantName == name {
			s.clusters[i] = cluster
			return
		}
	}
}

func NewFakeAccountServiceClient() *FakeAccountServiceClient {
	return &FakeAccountServiceClient{}
}

type FakeAccountServiceClient struct {
}

func (acc *FakeAccountServiceClient) Ping(context.Context) error {
	return nil
}

func (acc *FakeAccountServiceClient) GetRegionServiceClient(platform, region string) (cloudsdk.RegionServiceClientInterface, error) {
	return &FakeRegionServiceClient{
		region:   region,
		platform: platform,
	}, nil
}

type FakeRegionServiceClient struct {
	region   string
	platform string
}

func (f *FakeRegionServiceClient) GetRegionState() *RegionState {
	return state.GetRegionState(f.platform, f.region)
}

func (f *FakeRegionServiceClient) GetClusterByName(ctx context.Context, name string) (*apigen_mgmt.Tenant, error) {
	c, err := f.GetRegionState().GetClusterByName(name)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func reqResouceToClusterResource(reqResource *apigen_mgmt.TenantResourceRequest) apigen_mgmt.TenantResource {
	return apigen_mgmt.TenantResource{
		Components: apigen_mgmt.TenantResourceComponents{
			Compute: &apigen_mgmt.ComponentResource{
				ComponentTypeId: reqResource.Components.Compute.ComponentTypeId,
				Replica:         reqResource.Components.Compute.Replica,
			},
			Compactor: &apigen_mgmt.ComponentResource{
				ComponentTypeId: reqResource.Components.Compactor.ComponentTypeId,
				Replica:         reqResource.Components.Compactor.Replica,
			},
			Frontend: &apigen_mgmt.ComponentResource{
				ComponentTypeId: reqResource.Components.Frontend.ComponentTypeId,
				Replica:         reqResource.Components.Frontend.Replica,
			},
			Meta: &apigen_mgmt.ComponentResource{
				ComponentTypeId: reqResource.Components.Meta.ComponentTypeId,
				Replica:         reqResource.Components.Meta.Replica,
			},
			Etcd: apigen_mgmt.ComponentResource{
				ComponentTypeId: reqResource.Components.Etcd.ComponentTypeId,
				Replica:         reqResource.Components.Etcd.Replica,
			},
		},
		EnableComputeFileCache:  reqResource.EnableComputeFileCache,
		EtcdVolumeSizeGiB:       reqResource.EtcdVolumeSizeGiB,
		ComputeFileCacheSizeGiB: reqResource.ComputeFileCacheSizeGiB,
	}

}

// Create a RisingWave cluster and wait for it to be ready.
func (f *FakeRegionServiceClient) CreateClusterAwait(ctx context.Context, req apigen_mgmt.TenantRequestRequestBody) (*apigen_mgmt.Tenant, error) {
	cluster := apigen_mgmt.Tenant{
		Id:         uint64(len(f.GetRegionState().GetClusters()) + 1),
		TenantName: req.TenantName,
		ImageTag:   *req.ImageTag,
		Region:     f.region,
		RwConfig:   *req.RwConfig,
		EtcdConfig: *req.EtcdConfig,
		Resources:  reqResouceToClusterResource(req.Resources),
	}
	f.GetRegionState().AddCluster(cluster)
	return &cluster, nil
}

// Delete a RisingWave cluster by its name and wait for it to be ready.
func (f *FakeRegionServiceClient) DeleteClusterAwait(ctx context.Context, name string) error {
	f.GetRegionState().DeleteCluster(name)
	return nil
}

// Update the version of a RisingWave cluster by its name.
func (f *FakeRegionServiceClient) UpdateClusterImageAwait(ctx context.Context, name string, version string) error {
	cluster, err := f.GetClusterByName(ctx, name)
	if err != nil {
		return err
	}
	cluster.ImageTag = version
	f.GetRegionState().ReplaceCluster(name, *cluster)
	return nil
}

func componentReqToComponent(req *apigen_mgmt.ComponentResourceRequest) *apigen_mgmt.ComponentResource {
	return &apigen_mgmt.ComponentResource{
		ComponentTypeId: req.ComponentTypeId,
		Replica:         req.Replica,
	}
}

// Update the resources of a RisinGWave cluster by its name.
func (f *FakeRegionServiceClient) UpdateClusterResourcesAwait(ctx context.Context, name string, req apigen_mgmt.PostTenantResourcesRequestBody) error {
	cluster, err := f.GetClusterByName(ctx, name)
	if err != nil {
		return err
	}
	cluster.Resources.Components.Compactor = componentReqToComponent(req.Compactor)
	cluster.Resources.Components.Compute = componentReqToComponent(req.Compute)
	cluster.Resources.Components.Frontend = componentReqToComponent(req.Frontend)
	cluster.Resources.Components.Meta = componentReqToComponent(req.Meta)
	f.GetRegionState().ReplaceCluster(name, *cluster)
	return nil
}

func (f *FakeRegionServiceClient) GetTiers(ctx context.Context) ([]apigen_mgmt.Tier, error) {
	nodes := []apigen_mgmt.AvailableComponentType{
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
	return []apigen_mgmt.Tier{
		{
			Id:                              ptr.Ptr(apigen_mgmt.Standard),
			AvailableMetaNodes:              nodes,
			AvailableComputeNodes:           nodes,
			AvailableCompactorNodes:         nodes,
			AvailableEtcdNodes:              nodes,
			AvailableFrontendNodes:          nodes,
			AllowEnableComputeNodeFileCache: true,
			MaximumEtcdSizeGiB:              20,
		},
	}, nil
}

func (f *FakeRegionServiceClient) GetAvailableComponentTypes(ctx context.Context, targetTier apigen_mgmt.TierId, component string) ([]apigen_mgmt.AvailableComponentType, error) {
	tiers, err := f.GetTiers(ctx)
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

func (f *FakeRegionServiceClient) UpdateRisingWaveConfigAwait(ctx context.Context, name string, rwConfig string) error {
	cluster, err := f.GetClusterByName(ctx, name)
	if err != nil {
		return err
	}
	cluster.RwConfig = rwConfig
	f.GetRegionState().ReplaceCluster(cluster.TenantName, *cluster)
	return nil
}

func (f *FakeRegionServiceClient) UpdateEtcdConfigAwait(ctx context.Context, name string, etcdConfig string) error {
	cluster, err := f.GetClusterByName(ctx, name)
	if err != nil {
		return err
	}
	cluster.EtcdConfig = etcdConfig
	f.GetRegionState().ReplaceCluster(cluster.TenantName, *cluster)
	return nil
}
