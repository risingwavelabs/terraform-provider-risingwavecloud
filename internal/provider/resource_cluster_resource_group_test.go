package provider

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseClusterResourceGroupIdentifier(t *testing.T) {
	nsID := uuid.Must(uuid.NewRandom())

	tests := []struct {
		name         string
		id           string
		expectErr    bool
		expectedName string
	}{
		{
			name:         "valid",
			id:           nsID.String() + ".streaming-rg",
			expectedName: "streaming-rg",
		},
		{
			name:      "missing resource group name",
			id:        nsID.String() + ".",
			expectErr: true,
		},
		{
			name:      "missing separator",
			id:        nsID.String(),
			expectErr: true,
		},
		{
			name:      "invalid cluster ID",
			id:        "not-a-uuid.streaming-rg",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diags diag.Diagnostics
			gotNsID, gotName := parseClusterResourceGroupIdentifier(tt.id, &diags)

			if tt.expectErr {
				assert.True(t, diags.HasError())
				return
			}
			assert.False(t, diags.HasError())
			assert.Equal(t, nsID, gotNsID)
			assert.Equal(t, tt.expectedName, gotName)
		})
	}
}

func TestClusterResourceGroupToDataModel(t *testing.T) {
	nsID := uuid.Must(uuid.NewRandom())

	var data ClusterResourceGroupModel
	clusterResourceGroupToDataModel(nsID, &apigen_mgmtv2.ResourceGroupDetails{
		Name: "streaming-rg",
		Resource: apigen_mgmtv2.ComponentResource{
			ComponentTypeId: "p-1c4g",
			Replica:         2,
		},
		ComputeCache: apigen_mgmtv2.TenantResourceComputeCache{SizeGb: 20},
	}, &data)

	assert.Equal(t, nsID.String()+".streaming-rg", data.ID.ValueString())
	assert.Equal(t, nsID.String(), data.ClusterID.ValueString())
	assert.Equal(t, "streaming-rg", data.Name.ValueString())
	assert.Equal(t, "p-1c4g", data.ComponentTypeID.ValueString())
	assert.Equal(t, int64(2), data.Replica.ValueInt64())
	assert.Equal(t, int64(20), data.ComputeCacheSizeGB.ValueInt64())
}

// the default resource group belongs to the cluster resource: managing it here would give it
// two owners and a destroy would try to remove a resource group the cluster requires.
func TestCheckNotDefaultResourceGroup(t *testing.T) {
	var diags diag.Diagnostics
	checkNotDefaultResourceGroup("streaming-rg", &diags)
	assert.False(t, diags.HasError())

	checkNotDefaultResourceGroup(defaultResourceGroup, &diags)
	assert.True(t, diags.HasError())
}

func clusterResourceGroupSchema(ctx context.Context, t *testing.T) schema.Schema {
	t.Helper()

	resp := &resource.SchemaResponse{}
	(&ClusterResourceGroupResource{}).Schema(ctx, resource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError())
	return resp.Schema
}

// stateCacheSizeGB is the compute cache size the platform resolved for the component type
// currently in the state.
const stateCacheSizeGB = 20

func clusterResourceGroupValue(t *testing.T, objType tftypes.Object, componentTypeID string, replica int64) tftypes.Value {
	t.Helper()

	return tftypes.NewValue(objType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "cluster.streaming-rg"),
		"cluster_id":            tftypes.NewValue(tftypes.String, "cluster"),
		"name":                  tftypes.NewValue(tftypes.String, "streaming-rg"),
		"component_type_id":     tftypes.NewValue(tftypes.String, componentTypeID),
		"replica":               tftypes.NewValue(tftypes.Number, replica),
		"compute_cache_size_gb": tftypes.NewValue(tftypes.Number, stateCacheSizeGB),
	})
}

// The platform resolves the compute cache size from the component type. Terraform carries the
// prior value of a computed attribute into the plan, so without the plan modifier a component
// type change would end with an applied value that differs from the planned one.
func TestComputeCacheSizePlanModifier(t *testing.T) {
	ctx := context.Background()
	sch := clusterResourceGroupSchema(ctx, t)
	objType, ok := sch.Type().TerraformType(ctx).(tftypes.Object)
	require.True(t, ok)

	newRequest := func(state, plan tftypes.Value) planmodifier.Int64Request {
		return planmodifier.Int64Request{
			Path:        path.Root("compute_cache_size_gb"),
			State:       tfsdk.State{Raw: state, Schema: sch},
			Plan:        tfsdk.Plan{Raw: plan, Schema: sch},
			StateValue:  types.Int64Value(stateCacheSizeGB),
			PlanValue:   types.Int64Value(stateCacheSizeGB),
			ConfigValue: types.Int64Null(),
		}
	}

	t.Run("component type changed", func(t *testing.T) {
		req := newRequest(
			clusterResourceGroupValue(t, objType, "p-1c4g", 1),
			clusterResourceGroupValue(t, objType, "p-2c8g", 1),
		)
		resp := &planmodifier.Int64Response{PlanValue: req.PlanValue}

		unknownOnComponentTypeChange{}.PlanModifyInt64(ctx, req, resp)

		assert.False(t, resp.Diagnostics.HasError())
		assert.True(t, resp.PlanValue.IsUnknown())
	})

	t.Run("only the replica changed", func(t *testing.T) {
		req := newRequest(
			clusterResourceGroupValue(t, objType, "p-1c4g", 1),
			clusterResourceGroupValue(t, objType, "p-1c4g", 2),
		)
		resp := &planmodifier.Int64Response{PlanValue: req.PlanValue}

		unknownOnComponentTypeChange{}.PlanModifyInt64(ctx, req, resp)

		assert.False(t, resp.Diagnostics.HasError())
		assert.Equal(t, int64(stateCacheSizeGB), resp.PlanValue.ValueInt64())
	})

	t.Run("create keeps the unknown value", func(t *testing.T) {
		req := planmodifier.Int64Request{
			Path:        path.Root("compute_cache_size_gb"),
			State:       tfsdk.State{Raw: tftypes.NewValue(objType, nil), Schema: sch},
			Plan:        tfsdk.Plan{Raw: clusterResourceGroupValue(t, objType, "p-1c4g", 1), Schema: sch},
			StateValue:  types.Int64Null(),
			PlanValue:   types.Int64Unknown(),
			ConfigValue: types.Int64Null(),
		}
		resp := &planmodifier.Int64Response{PlanValue: req.PlanValue}

		unknownOnComponentTypeChange{}.PlanModifyInt64(ctx, req, resp)

		assert.False(t, resp.Diagnostics.HasError())
		assert.True(t, resp.PlanValue.IsUnknown())
	})

	t.Run("destroy is not modified", func(t *testing.T) {
		req := newRequest(
			clusterResourceGroupValue(t, objType, "p-1c4g", 1),
			tftypes.NewValue(objType, nil),
		)
		resp := &planmodifier.Int64Response{PlanValue: req.PlanValue}

		unknownOnComponentTypeChange{}.PlanModifyInt64(ctx, req, resp)

		assert.False(t, resp.Diagnostics.HasError())
		assert.Equal(t, int64(stateCacheSizeGB), resp.PlanValue.ValueInt64())
	})
}
