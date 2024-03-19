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
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/ptr"
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
		g.regionStates[region] = NewRegionState()
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
	return apigen_mgmt.Tenant{}, errors.Wrapf(cloudsdk.ErrClusterNotFound, "nsID: %s", nsID.String())
}

func (g *GlobalState) GetClusterUser(nsID uuid.UUID, username string) (*apigen_mgmt.DBUser, error) {
	for _, r := range g.regionStates {
		_, err := r.GetClusterByNsID(nsID)
		if err == nil {
			if u := r.GetClusterUser(nsID, username); u != nil {
				return u, nil
			}
			return nil, errors.Errorf("user %s not found in cluster %s", username, nsID.String())
		}
	}

	return nil, errors.Wrapf(cloudsdk.ErrClusterNotFound, "nsID: %s", nsID.String())
}

func (g *GlobalState) CreateClusterUser(nsID uuid.UUID, username, password string, createDB, superUser bool) (*apigen_mgmt.DBUser, error) {
	for _, r := range g.regionStates {
		_, err := r.GetClusterByNsID(nsID)
		if err == nil {
			u := apigen_mgmt.DBUser{
				Usecreatedb: createDB,
				Username:    username,
				Usesysid:    uint64(len(r.clusterUsers) + 1),
				Usesuper:    superUser,
			}
			r.CreateClusterUser(nsID, u)
			return &u, nil
		}
	}

	return nil, errors.Wrapf(cloudsdk.ErrClusterNotFound, "nsID: %s", nsID.String())
}

func (g *GlobalState) DeleteClusterUser(nsID uuid.UUID, username string) {
	for _, r := range g.regionStates {
		_, err := r.GetClusterByNsID(nsID)
		if err == nil {
			r.DeleteClusterUser(nsID, username)
		}
	}
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
	clusters     []apigen_mgmt.Tenant
	clusterUsers map[string][]apigen_mgmt.DBUser
	mu           sync.RWMutex
}

func NewRegionState() *RegionState {
	return &RegionState{
		clusterUsers: map[string][]apigen_mgmt.DBUser{},
	}
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
	return apigen_mgmt.Tenant{}, errors.Wrapf(cloudsdk.ErrClusterNotFound, "nsID: %s", nsID.String())
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

func (s *RegionState) CreateClusterUser(nsID uuid.UUID, user apigen_mgmt.DBUser) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.clusterUsers[nsID.String()]; !ok {
		s.clusterUsers[nsID.String()] = []apigen_mgmt.DBUser{}
	}

	s.clusterUsers[nsID.String()] = append(s.clusterUsers[nsID.String()], user)
}

func (s *RegionState) GetClusterUser(nsID uuid.UUID, username string) *apigen_mgmt.DBUser {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, u := range s.clusterUsers[nsID.String()] {
		if u.Username == username {
			return ptr.Ptr(u)
		}
	}
	return nil
}

func (s *RegionState) DeleteClusterUser(nsID uuid.UUID, username string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, u := range s.clusterUsers[nsID.String()] {
		if u.Username == username {
			s.clusterUsers[nsID.String()] = append(
				s.clusterUsers[nsID.String()][:i],
				s.clusterUsers[nsID.String()][i+1:]...,
			)
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

func (acc *FakeCloudClient) GetClusterUser(ctx context.Context, nsID uuid.UUID, username string) (*apigen_mgmt.DBUser, error) {
	debugFuncCaller()

	return state.GetClusterUser(nsID, username)
}

func (acc *FakeCloudClient) CreateCluserUser(ctx context.Context, nsID uuid.UUID, username, password string, createDB, superUser bool) (*apigen_mgmt.DBUser, error) {
	debugFuncCaller()

	return state.CreateClusterUser(nsID, username, password, createDB, superUser)
}

func (acc *FakeCloudClient) UpdateClusterUserPassword(ctx context.Context, nsID uuid.UUID, username, password string) error {
	debugFuncCaller()

	_, err := state.GetClusterUser(nsID, username)
	return err
}

func (acc *FakeCloudClient) DeleteClusterUser(ctx context.Context, nsID uuid.UUID, username string) error {
	debugFuncCaller()

	state.DeleteClusterUser(nsID, username)

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
