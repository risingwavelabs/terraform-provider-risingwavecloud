package cloudsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/wait"
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

func TestAllowedIamRoleMutationWaitsForReady(t *testing.T) {
	previousPolling := PollingAllowedIamRoleOperation
	PollingAllowedIamRoleOperation = wait.PollingParams{
		Timeout:  time.Second,
		Interval: time.Millisecond,
	}
	t.Cleanup(func() {
		PollingAllowedIamRoleOperation = previousPolling
	})

	tests := []struct {
		name   string
		method string
		mutate func(context.Context, *RegionServiceClient, uuid.UUID) error
	}{
		{
			name:   "add",
			method: http.MethodPost,
			mutate: func(ctx context.Context, client *RegionServiceClient, nsID uuid.UUID) error {
				return client.AddAllowedIamRoleAwait(ctx, nsID, "arn:aws:iam::123456789012:role/test")
			},
		},
		{
			name:   "remove",
			method: http.MethodDelete,
			mutate: func(ctx context.Context, client *RegionServiceClient, nsID uuid.UUID) error {
				return client.RemoveAllowedIamRoleAwait(ctx, nsID, "arn:aws:iam::123456789012:role/test")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nsID := uuid.Must(uuid.NewRandom())
			var methods []string
			getCalls := 0

			client := newTestRegionServiceClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/tenants/"+nsID.String()+"/allowedIamRoles", r.URL.Path)
				methods = append(methods, r.Method)

				if r.Method == http.MethodGet {
					getCalls++
					status := apigen_mgmtv2.GetTenantAllowedIamRolesResponseBodyStatusReady
					if getCalls == 1 {
						status = apigen_mgmtv2.GetTenantAllowedIamRolesResponseBodyStatusPending
					}
					w.Header().Set("Content-Type", "application/json")
					require.NoError(t, json.NewEncoder(w).Encode(apigen_mgmtv2.GetTenantAllowedIamRolesResponseBody{
						RoleArns: []string{},
						Status:   status,
					}))
					return
				}

				assert.Equal(t, tt.method, r.Method)
				assert.GreaterOrEqual(t, getCalls, 2, "the policy must be ready before the mutation")
				w.WriteHeader(http.StatusAccepted)
			}))

			require.NoError(t, tt.mutate(context.Background(), client, nsID))
			assert.Equal(t, []string{http.MethodGet, http.MethodGet, tt.method, http.MethodGet}, methods)
		})
	}
}

func TestAllowedIamRoleMutationStopsOnFailedStatus(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *RegionServiceClient, uuid.UUID) error
	}{
		{
			name: "add",
			mutate: func(ctx context.Context, client *RegionServiceClient, nsID uuid.UUID) error {
				return client.AddAllowedIamRoleAwait(ctx, nsID, "arn:aws:iam::123456789012:role/test")
			},
		},
		{
			name: "remove",
			mutate: func(ctx context.Context, client *RegionServiceClient, nsID uuid.UUID) error {
				return client.RemoveAllowedIamRoleAwait(ctx, nsID, "arn:aws:iam::123456789012:role/test")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nsID := uuid.Must(uuid.NewRandom())
			client := newTestRegionServiceClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method, "a failed policy must stop before the mutation")
				w.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(w).Encode(apigen_mgmtv2.GetTenantAllowedIamRolesResponseBody{
					RoleArns: []string{},
					Status:   apigen_mgmtv2.GetTenantAllowedIamRolesResponseBodyStatusFailed,
				}))
			}))

			err := tt.mutate(context.Background(), client, nsID)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "the platform failed to apply the allowed IAM roles")
		})
	}
}
