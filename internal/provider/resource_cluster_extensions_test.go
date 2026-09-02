package provider

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"

	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/mock"
)

func compactionObject(t *testing.T, concurrency int64) types.Object {
	t.Helper()

	obj, diags := types.ObjectValue(serverlessCompactionAttrTypes, map[string]attr.Value{
		"maximum_compaction_concurrency": types.Int64Value(concurrency),
		"version":                        types.StringNull(),
		"status":                         types.StringValue("Running"),
	})
	require.False(t, diags.HasError())
	return obj
}

func extensionsWith(compaction types.Object) ClusterExtensionsModel {
	return ClusterExtensionsModel{
		ServerlessCompaction: compaction,
		ServerlessBackfill:   types.ObjectNull(extensionNodesAttrTypes),
		IcebergCompaction:    types.ObjectNull(icebergCompactionAttrTypes),
	}
}

// An extension the plan leaves exactly as it is must not be sent again. Every mutation starts a
// platform workflow, so without this a change to anything else about the cluster -- a version
// bump, a line of risingwave_config -- would restart every extension the cluster has.
func TestApplyExtensionsSkipsUnchanged(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mock.NewMockCloudClientInterface(ctrl)

	// No call is expected: the controller fails the test if one is made.
	current := extensionsWith(compactionObject(t, 4))
	planned := extensionsWith(compactionObject(t, 4))

	var diags diag.Diagnostics
	require.NoError(t, applyExtensions(context.Background(), client, uuid.Must(uuid.NewRandom()), planned, current, &diags))
	require.False(t, diags.HasError())
}

// A changed extension is still sent, so the skip above cannot be achieved by never sending
// anything.
func TestApplyExtensionsSendsChanged(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mock.NewMockCloudClientInterface(ctrl)
	nsID := uuid.Must(uuid.NewRandom())

	client.EXPECT().
		UpdateServerlessCompactionAwait(gomock.Any(), nsID, gomock.Any()).
		Return(nil).
		Times(1)

	current := extensionsWith(compactionObject(t, 4))
	planned := extensionsWith(compactionObject(t, 8))

	var diags diag.Diagnostics
	require.NoError(t, applyExtensions(context.Background(), client, nsID, planned, current, &diags))
	require.False(t, diags.HasError())
}
