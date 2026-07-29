package cloudsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRegionServiceClient(t *testing.T, handler http.Handler) *RegionServiceClient {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := apigen_mgmtv2.NewClientWithResponses(server.URL)
	require.NoError(t, err)

	return &RegionServiceClient{mgmtV2Client: client}
}

// Databases are paginated: the loop must keep requesting until every page is consumed,
// otherwise a cluster with more databases than the page size silently reports a database as
// missing and terraform deletes it from the state.
func TestGetDatabasesPagination(t *testing.T) {
	var (
		nsID  = uuid.Must(uuid.NewRandom())
		pages = []apigen_mgmtv2.DatabasesPagination{
			{
				Databases: []apigen_mgmtv2.Database{
					{Name: "db1", ResourceGroup: "default"},
					{Name: "db2", ResourceGroup: "streaming-rg"},
				},
				Pagination: &apigen_mgmtv2.Pagination{Offset: 0, Limit: 2, Size: 3},
			},
			{
				Databases: []apigen_mgmtv2.Database{
					{Name: "db3", ResourceGroup: "default"},
				},
				Pagination: &apigen_mgmtv2.Pagination{Offset: 2, Limit: 2, Size: 3},
			},
		}
		requestedOffsets []string
	)

	client := newTestRegionServiceClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/tenants/"+nsID.String()+"/databases", r.URL.Path)

		offset := r.URL.Query().Get("offset")
		requestedOffsets = append(requestedOffsets, offset)

		page := pages[0]
		if offset != "0" {
			page = pages[1]
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(page))
	}))

	databases, err := client.GetDatabases(context.Background(), nsID)
	require.NoError(t, err)

	assert.Equal(t, []string{"0", "2"}, requestedOffsets)
	require.Len(t, databases, 3)
	assert.Equal(t, "db1", databases[0].Name)
	assert.Equal(t, "db2", databases[1].Name)
	assert.Equal(t, "db3", databases[2].Name)
}

func TestGetDatabasesSinglePage(t *testing.T) {
	nsID := uuid.Must(uuid.NewRandom())

	var requests int
	client := newTestRegionServiceClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(apigen_mgmtv2.DatabasesPagination{
			Databases: []apigen_mgmtv2.Database{{Name: "db1", ResourceGroup: "default"}},
		}))
	}))

	databases, err := client.GetDatabases(context.Background(), nsID)
	require.NoError(t, err)

	assert.Equal(t, 1, requests)
	require.Len(t, databases, 1)
	assert.Equal(t, "db1", databases[0].Name)
}

func TestGetDatabasesClusterNotFound(t *testing.T) {
	nsID := uuid.Must(uuid.NewRandom())

	client := newTestRegionServiceClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := client.GetDatabases(context.Background(), nsID)
	assert.True(t, errors.Is(err, ErrClusterNotFound))
}

func TestGetResourceGroupsClusterNotFound(t *testing.T) {
	nsID := uuid.Must(uuid.NewRandom())

	client := newTestRegionServiceClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := client.GetResourceGroups(context.Background(), nsID)
	assert.True(t, errors.Is(err, ErrClusterNotFound))
}

func TestGetResourceGroups(t *testing.T) {
	nsID := uuid.Must(uuid.NewRandom())

	client := newTestRegionServiceClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/tenants/"+nsID.String()+"/resourceGroups", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(apigen_mgmtv2.GetResourceGroupsResponseBody{
			ResourceGroups: []apigen_mgmtv2.ResourceGroupDetails{
				{
					Name:         "streaming-rg",
					Resource:     apigen_mgmtv2.ComponentResource{ComponentTypeId: "p-1c4g", Replica: 2},
					ComputeCache: apigen_mgmtv2.TenantResourceComputeCache{SizeGb: 20},
				},
			},
		}))
	}))

	groups, err := client.GetResourceGroups(context.Background(), nsID)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "streaming-rg", groups[0].Name)

	group, err := client.getResourceGroup(context.Background(), nsID, "streaming-rg")
	require.NoError(t, err)
	assert.Equal(t, 2, group.Resource.Replica)

	_, err = client.getResourceGroup(context.Background(), nsID, "missing-rg")
	assert.True(t, errors.Is(err, ErrResourceGroupNotFound))
}
