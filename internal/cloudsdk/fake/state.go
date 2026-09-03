package fake

import (
	"sort"
	"sync"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
)

type ClusterState struct {
	mu sync.RWMutex

	tenant *apigen_mgmtv2.Tenant

	// username -> user
	users map[string]*apigen_mgmtv2.DBUser

	// private link ID -> private link
	privateLinks map[string]*apigen_mgmtv2.PrivateLink

	// resource group name -> resource group
	resourceGroups map[string]*apigen_mgmtv2.ResourceGroupDetails

	// IAM role ARNs allowed to assume a role into the customer's account
	allowedIamRoles map[string]bool

	// The tenant extensions. A nil pointer is an extension that was never enabled, which the
	// platform reports as `Disabled` rather than as a missing object.
	// compactorReplicaBeforeCompaction remembers what the compactor was before serverless
	// compaction took it away, so disabling the extension can put it back the way the platform
	// does.
	compactorReplicaBeforeCompaction int

	serverlessCompaction *apigen_mgmtv2.GetTenantExtensionCompactionResponseBody
	serverlessBackfill   *apigen_mgmtv2.GetTenantExtensionServerlessBackfillResponseBody
	icebergCompaction    *apigen_mgmtv2.IcebergCompaction
}

// GetServerlessCompaction returns the extension, or nil when it was never enabled.
func (c *ClusterState) GetServerlessCompaction() *apigen_mgmtv2.GetTenantExtensionCompactionResponseBody {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.serverlessCompaction
}

// SetServerlessCompaction records the extension and moves the cluster's own compactor the way
// the platform does: enabling scales it to zero, since that is what the extension is for, and
// disabling restores what it was.
func (c *ClusterState) SetServerlessCompaction(ext *apigen_mgmtv2.GetTenantExtensionCompactionResponseBody) {
	c.mu.Lock()
	defer c.mu.Unlock()

	enabling := ext != nil && c.serverlessCompaction == nil
	disabling := ext == nil && c.serverlessCompaction != nil
	c.serverlessCompaction = ext

	compactor := c.tenant.Resources.Components.Compactor
	if compactor == nil {
		return
	}
	switch {
	case enabling:
		c.compactorReplicaBeforeCompaction = compactor.Replica
		compactor.Replica = 0
	case disabling:
		compactor.Replica = c.compactorReplicaBeforeCompaction
	}
}

// GetServerlessBackfill returns the extension, or nil when it was never enabled.
func (c *ClusterState) GetServerlessBackfill() *apigen_mgmtv2.GetTenantExtensionServerlessBackfillResponseBody {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.serverlessBackfill
}

func (c *ClusterState) SetServerlessBackfill(ext *apigen_mgmtv2.GetTenantExtensionServerlessBackfillResponseBody) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.serverlessBackfill = ext
}

// GetIcebergCompaction returns the extension, or nil when it was never enabled.
func (c *ClusterState) GetIcebergCompaction() *apigen_mgmtv2.IcebergCompaction {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.icebergCompaction
}

func (c *ClusterState) SetIcebergCompaction(ext *apigen_mgmtv2.IcebergCompaction) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.icebergCompaction = ext
}

func NewClusterState(tenant *apigen_mgmtv2.Tenant) *ClusterState {
	return &ClusterState{
		tenant:          tenant,
		users:           map[string]*apigen_mgmtv2.DBUser{},
		privateLinks:    map[string]*apigen_mgmtv2.PrivateLink{},
		resourceGroups:  map[string]*apigen_mgmtv2.ResourceGroupDetails{},
		allowedIamRoles: map[string]bool{},
	}
}

func (c *ClusterState) GetPrivateLinks() map[string]*apigen_mgmtv2.PrivateLink {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.privateLinks
}

func (c *ClusterState) AddClusterUser(user *apigen_mgmtv2.DBUser) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.users[user.Username] = user
}

func (c *ClusterState) DeleteClusterUser(username string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.users, username)
}

func (c *ClusterState) GetClusterUser(username string) (*apigen_mgmtv2.DBUser, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	u, ok := c.users[username]
	if !ok {
		return nil, errors.Wrapf(cloudsdk.ErrClusterUserNotFound, "username: %s", username)
	}
	return u, nil
}

func (c *ClusterState) GetTenant() *apigen_mgmtv2.Tenant {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.tenant
}

func (c *ClusterState) AddPrivateLink(privateLink *apigen_mgmtv2.PrivateLink) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.privateLinks[privateLink.Id.String()] = privateLink
}

func (c *ClusterState) DeletePrivateLink(id uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.privateLinks, id.String())
}

func (c *ClusterState) GetPrivateLink(id uuid.UUID) (*apigen_mgmtv2.PrivateLink, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	pl, ok := c.privateLinks[id.String()]
	if !ok {
		return nil, errors.Wrapf(cloudsdk.ErrPrivateLinkNotFound, "id: %s", id.String())
	}
	return pl, nil
}

func (c *ClusterState) GetResourceGroups() []*apigen_mgmtv2.ResourceGroupDetails {
	c.mu.RLock()
	defer c.mu.RUnlock()

	rtn := make([]*apigen_mgmtv2.ResourceGroupDetails, 0, len(c.resourceGroups))
	for _, g := range c.resourceGroups {
		rtn = append(rtn, g)
	}
	return rtn
}

func (c *ClusterState) GetResourceGroup(name string) (*apigen_mgmtv2.ResourceGroupDetails, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	g, ok := c.resourceGroups[name]
	if !ok {
		return nil, errors.Wrapf(cloudsdk.ErrResourceGroupNotFound, "name: %s", name)
	}
	return g, nil
}

func (c *ClusterState) AddResourceGroup(group *apigen_mgmtv2.ResourceGroupDetails) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.resourceGroups[group.Name] = group
}

func (c *ClusterState) DeleteResourceGroup(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.resourceGroups, name)
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

func (c *ClusterState) GetAllowedIamRoles() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	rtn := make([]string, 0, len(c.allowedIamRoles))
	for arn := range c.allowedIamRoles {
		rtn = append(rtn, arn)
	}
	sort.Strings(rtn)
	return rtn
}

func (c *ClusterState) AddAllowedIamRole(roleArn string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.allowedIamRoles[roleArn] = true
}

func (c *ClusterState) RemoveAllowedIamRole(roleArn string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.allowedIamRoles, roleArn)
}
