package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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

// The pattern mirrors what the platform enforces, checked against prod us-east-1:
// "resource group name should match '[a-z0-9]([a-z0-9-]{0,18}[a-z0-9])?'". Validating it here
// turns an apply-time API error into a plan-time one.
func TestResourceGroupNameValidator(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{name: "streaming-rg", valid: true},
		{name: "rg1", valid: true},
		{name: "a", valid: true},
		{name: "12345678901234567890", valid: true}, // 20 characters, the maximum
		{name: "", valid: false},
		{name: "Verify-MixedCase", valid: false}, // rejected by the platform
		{name: "verify.dot", valid: false},       // rejected by the platform, and a dot would break the id
		{name: "-leading-dash", valid: false},
		{name: "trailing-dash-", valid: false},
		{name: "under_score", valid: false},
		{name: "123456789012345678901", valid: false}, // 21 characters
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			resourceGroupNameValidator{}.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("name"),
				ConfigValue: types.StringValue(tt.name),
			}, resp)

			assert.Equal(t, tt.valid, !resp.Diagnostics.HasError())
		})
	}
}

// The platform rejects a resource group with no replica: "request 0 replica(s) not valid for
// resource group. Exceeds the maximum allowed value or less than or equal to 0". The upper
// bound depends on the component type and stays with the platform.
func TestPositiveReplicaValidator(t *testing.T) {
	tests := []struct {
		replica int64
		valid   bool
	}{
		{replica: 1, valid: true},
		{replica: 8, valid: true},
		{replica: 0, valid: false},
		{replica: -1, valid: false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("replica=%d", tt.replica), func(t *testing.T) {
			resp := &validator.Int64Response{}
			positiveReplicaValidator{}.ValidateInt64(context.Background(), validator.Int64Request{
				Path:        path.Root("replica"),
				ConfigValue: types.Int64Value(tt.replica),
			}, resp)

			assert.Equal(t, tt.valid, !resp.Diagnostics.HasError())
		})
	}
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
