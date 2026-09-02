package cloudsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/wait"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fastExtensionPolling shortens the waits so the tests do not sit through real intervals.
// Both budgets matter: every extension mutation waits for the cluster to be idle first, and
// that wait is on the resource group budget.
func fastExtensionPolling(t *testing.T) {
	t.Helper()

	fast := wait.PollingParams{
		Timeout:  time.Second,
		Interval: time.Millisecond,
	}

	previousExtension, previousCluster := PollingExtensionOperation, PollingResourceGroupOperation
	PollingExtensionOperation, PollingResourceGroupOperation = fast, fast
	t.Cleanup(func() {
		PollingExtensionOperation, PollingResourceGroupOperation = previousExtension, previousCluster
	})
}

// compactionHandler answers the compaction endpoint with a status per GET, walking the given
// sequence and repeating its last entry.
func compactionHandler(t *testing.T, nsID uuid.UUID, statuses []string, methods *[]string) http.Handler {
	t.Helper()

	gets := 0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Every extension mutation asks whether the cluster is idle first, since the platform
		// will not touch an extension of a cluster that is busy.
		if r.URL.Path == "/tenants/"+nsID.String() {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(apigen_mgmtv2.Tenant{
				NsId:         nsID,
				Status:       apigen_mgmtv2.Running,
				HealthStatus: apigen_mgmtv2.Healthy,
			}))
			return
		}

		assert.Equal(t, "/tenants/"+nsID.String()+"/extensions/compaction", r.URL.Path)
		*methods = append(*methods, r.Method)

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		status := statuses[min(gets, len(statuses)-1)]
		gets++
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(apigen_mgmtv2.GetTenantExtensionCompactionResponseBody{
			Status: status,
		}))
	})
}

// A `Disabled` extension is not a missing one as far as the API is concerned: it answers 200
// with that status, and the resources rely on the SDK turning it into ErrExtensionDisabled so
// they can drop themselves from state.
func TestGetExtensionReportsDisabled(t *testing.T) {
	nsID := uuid.Must(uuid.NewRandom())
	var methods []string

	client := newTestRegionServiceClient(t, compactionHandler(t, nsID, []string{ExtensionStatusDisabled}, &methods))

	_, err := client.GetServerlessCompaction(context.Background(), nsID)
	assert.True(t, errors.Is(err, ErrExtensionDisabled))
}

// A standalone cluster is refused by the platform, and the message should say what the tier has
// to do with it rather than passing the bare status code along.
func TestExtensionOnStandaloneClusterIsExplained(t *testing.T) {
	nsID := uuid.Must(uuid.NewRandom())

	client := newTestRegionServiceClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte(`{"msg":"extension compaction isn't supported for standalone deployment"}`))
	}))

	_, err := client.GetServerlessCompaction(context.Background(), nsID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "standalone")
	assert.Contains(t, err.Error(), "separate compute component")
}

// The wait that runs before a mutation must let a `Failed` extension through. Refusing there
// would leave a failed extension impossible to disable, which is the one thing that clears it.
func TestExtensionMutationProceedsWhenFailed(t *testing.T) {
	fastExtensionPolling(t)

	nsID := uuid.Must(uuid.NewRandom())
	var methods []string

	// Failed while settling, then Disabled once the delete has been accepted.
	client := newTestRegionServiceClient(t, compactionHandler(t, nsID,
		[]string{ExtensionStatusFailed, ExtensionStatusDisabled}, &methods))

	require.NoError(t, client.DisableServerlessCompactionAwait(context.Background(), nsID))
	assert.Contains(t, methods, http.MethodDelete, "a failed extension must still be deletable")
}

// The wait that runs after a mutation must give up on `Failed` instead of polling until the
// budget runs out, so the error names the failure the platform already knows about.
func TestExtensionApplyStopsOnFailedStatus(t *testing.T) {
	fastExtensionPolling(t)

	nsID := uuid.Must(uuid.NewRandom())
	var methods []string

	// Settled before the request, then failing afterwards.
	client := newTestRegionServiceClient(t, compactionHandler(t, nsID,
		[]string{ExtensionStatusDisabled, ExtensionStatusFailed}, &methods))

	err := client.EnableServerlessCompactionAwait(context.Background(), nsID, apigen_mgmtv2.TenantExtensionServerlessCompactionRequest{
		MaximumCompactionConcurrency: 4,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the platform failed to apply the serverless compaction extension")
	assert.NotContains(t, err.Error(), "timeout", "a known failure must not be reported as a timeout")
}

// A mutation waits for the extension to settle before it is sent: the platform refuses a
// request that overlaps a workflow already running on the tenant.
func TestExtensionMutationWaitsForTheExtensionToSettle(t *testing.T) {
	fastExtensionPolling(t)

	nsID := uuid.Must(uuid.NewRandom())
	var methods []string

	client := newTestRegionServiceClient(t, compactionHandler(t, nsID,
		[]string{ExtensionStatusUpdating, ExtensionStatusRunning, ExtensionStatusDisabled}, &methods))

	require.NoError(t, client.DisableServerlessCompactionAwait(context.Background(), nsID))

	// two reads to get past `Updating`, then the delete, then the read that sees `Disabled`
	assert.Equal(t, []string{http.MethodGet, http.MethodGet, http.MethodDelete, http.MethodGet}, methods)
}
