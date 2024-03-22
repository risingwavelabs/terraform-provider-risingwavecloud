package fake

import (
	"sync"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt"
)

type ClusterState struct {
	mu           sync.RWMutex
	tenant       *apigen_mgmt.Tenant
	users        map[string]*apigen_mgmt.DBUser
	privateLinks map[string]*apigen_mgmt.PrivateLink
}

func NewClusterState(tenant *apigen_mgmt.Tenant) *ClusterState {
	return &ClusterState{
		tenant: tenant,
		users:  map[string]*apigen_mgmt.DBUser{},
	}
}

func (c *ClusterState) AddClusterUser(user *apigen_mgmt.DBUser) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.users[user.Username] = user
}

func (c *ClusterState) DeleteClusterUser(username string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.users, username)
}

func (c *ClusterState) GetClusterUser(username string) (*apigen_mgmt.DBUser, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	u, ok := c.users[username]
	if !ok {
		return nil, errors.Wrapf(cloudsdk.ErrClusterUserNotFound, "username: %s", username)
	}
	return u, nil
}

func (c *ClusterState) GetTenant() *apigen_mgmt.Tenant {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.tenant
}

func (c *ClusterState) AddPrivateLink(privateLink *apigen_mgmt.PrivateLink) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.privateLinks[privateLink.Id.String()] = privateLink
}

func (c *ClusterState) DeletePrivateLink(id uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.privateLinks, id.String())
}

func (c *ClusterState) GetPrivateLink(id uuid.UUID) (*apigen_mgmt.PrivateLink, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	pl, ok := c.privateLinks[id.String()]
	if !ok {
		return nil, errors.Wrapf(cloudsdk.ErrPrivateLinkNotFound, "id: %s", id.String())
	}
	return pl, nil
}

type RegionState struct {
	clusters map[string]*ClusterState
	mu       sync.RWMutex
}

func NewRegionState() *RegionState {
	return &RegionState{
		clusters: map[string]*ClusterState{},
	}
}

func (r *RegionState) GetClusters() map[string]*ClusterState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.clusters
}

func (r *RegionState) GetClusterByNsID(nsID uuid.UUID) (*ClusterState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.clusters[nsID.String()]
	if !ok {
		return nil, errors.Wrapf(cloudsdk.ErrClusterNotFound, "nsID: %s", nsID.String())
	}

	return c, nil
}

func (s *RegionState) AddCluster(cluster *ClusterState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clusters[cluster.tenant.NsId.String()] = cluster
}

func (s *RegionState) DeleteCluster(nsID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.clusters, nsID.String())
}

func (s *RegionState) ReplaceCluster(nsID uuid.UUID, cluster *ClusterState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clusters[nsID.String()] = cluster
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

func (g *GlobalState) GetClusterByNsID(nsID uuid.UUID) (*ClusterState, error) {
	for _, r := range g.regionStates {
		cluster, err := r.GetClusterByNsID(nsID)
		if err == nil {
			return cluster, nil
		}
	}
	return nil, errors.Wrapf(cloudsdk.ErrClusterNotFound, "nsID: %s", nsID.String())
}

func (g *GlobalState) GetNsIDByRegionAndName(region, name string) (uuid.UUID, error) {
	r := g.GetRegionState(region)
	for _, c := range r.GetClusters() {
		if c.tenant.TenantName == name {
			return c.tenant.NsId, nil
		}
	}
	return uuid.UUID{}, errors.Wrapf(cloudsdk.ErrClusterNotFound, "region: %s, name: %s", region, name)
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
