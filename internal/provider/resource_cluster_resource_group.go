package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/pkg/errors"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
)

// defaultResourceGroup is the resource group that always exists in a cluster and is
// used when a database is created without specifying a resource group. Its lifecycle is
// bound to the cluster, so it is managed by the cluster resource.
const defaultResourceGroup = "default"

// resourceGroupNamePattern mirrors what the platform accepts, so that an invalid name is
// reported at plan time instead of coming back as an API error halfway through an apply.
// Note that it allows no dots, which is what makes the `[cluster ID].[name]` resource id
// unambiguous.
var resourceGroupNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,18}[a-z0-9])?$`)

// resourceGroupNameValidator checks a resource group name against the platform's rules.
type resourceGroupNameValidator struct{}

func (v resourceGroupNameValidator) Description(ctx context.Context) string {
	return fmt.Sprintf("value must match %s", resourceGroupNamePattern)
}

func (v resourceGroupNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v resourceGroupNameValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if name := req.ConfigValue.ValueString(); !resourceGroupNamePattern.MatchString(name) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid resource group name",
			fmt.Sprintf(
				"A resource group name must match %s: lower case letters, digits and dashes, "+
					"1 to 20 characters, starting and ending with a letter or a digit. Got: %q",
				resourceGroupNamePattern, name,
			),
		)
	}
}

// positiveReplicaValidator rejects replica counts the platform cannot serve. The upper bound
// depends on the component type and is left to the platform.
type positiveReplicaValidator struct{}

func (v positiveReplicaValidator) Description(ctx context.Context) string {
	return "value must be at least 1"
}

func (v positiveReplicaValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v positiveReplicaValidator) ValidateInt64(ctx context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if replica := req.ConfigValue.ValueInt64(); replica < 1 {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid replica count",
			fmt.Sprintf("A resource group needs at least one compute node replica, got: %d", replica),
		)
	}
}

// Assert provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ClusterResourceGroupResource{}
var _ resource.ResourceWithImportState = &ClusterResourceGroupResource{}

func NewResourceGroupResource() resource.Resource {
	return &ClusterResourceGroupResource{}
}

type ClusterResourceGroupResource struct {
	client cloudsdk.CloudClientInterface
}

type ClusterResourceGroupModel struct {
	// [cluster ID].[resource group name]
	ID                 types.String `tfsdk:"id"`
	ClusterID          types.String `tfsdk:"cluster_id"`
	Name               types.String `tfsdk:"name"`
	ComponentTypeID    types.String `tfsdk:"component_type_id"`
	Replica            types.Int64  `tfsdk:"replica"`
	ComputeCacheSizeGB types.Int64  `tfsdk:"compute_cache_size_gb"`
}

// unknownOnComponentTypeChange marks a computed attribute as unknown when the component
// type changes. Terraform carries the prior value of a computed attribute into the plan,
// so if the platform re-resolves the value during an update, the applied state differs from
// the plan and terraform fails with "provider produced inconsistent result after apply".
//
// This is a precaution rather than a fix for an observed failure: the platform currently
// returns the same compute cache size for every component type, so the value happens not to
// change today. The cost is a "(known after apply)" on that attribute when the component type
// changes; the alternative is a hard error the user cannot work around if that ever changes.
type unknownOnComponentTypeChange struct{}

func (m unknownOnComponentTypeChange) Description(ctx context.Context) string {
	return "The value is resolved by the platform again once the component type changes."
}

func (m unknownOnComponentTypeChange) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m unknownOnComponentTypeChange) PlanModifyInt64(ctx context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	// nothing to carry over on create, and nothing to plan on destroy.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var stateComponentType, planComponentType types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("component_type_id"), &stateComponentType)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("component_type_id"), &planComponentType)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !stateComponentType.Equal(planComponentType) {
		resp.PlanValue = types.Int64Unknown()
	}
}

// checkNotDefaultResourceGroup rejects the default resource group: it is created and
// rescaled through the cluster resource, managing it here would mean two resources own the
// same object and a destroy would try to remove a resource group the cluster requires.
func checkNotDefaultResourceGroup(name string, diags *diag.Diagnostics) {
	if name != defaultResourceGroup {
		return
	}
	diags.AddError(
		"Cannot manage the default resource group",
		fmt.Sprintf(
			"The %q resource group is managed by the risingwavecloud_cluster resource. Use the cluster's spec to rescale it.",
			defaultResourceGroup,
		),
	)
}

func (r *ClusterResourceGroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_resource_group"
}

func (r *ClusterResourceGroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "An additional (non-default) resource group in a RisingWave cluster used to isolate streaming workloads on their own compute nodes.",
		MarkdownDescription: clusterResourceGroupMarkdownDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The global identifier for the resource: [cluster ID].[resource group name]",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "The NsID (namespace id) of the cluster.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the resource group, unique within the cluster. It must be 1 to 20 " +
					"characters of lower case letters, digits and dashes, starting and ending with a letter or a digit. " +
					"The \"default\" resource group is managed by the cluster resource and cannot be managed here.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					resourceGroupNameValidator{},
				},
			},
			"component_type_id": schema.StringAttribute{
				MarkdownDescription: "The compute node component type ID (e.g. \"p-1c4g\") used by the resource group. Available component types depend on the cluster tier.",
				Required:            true,
			},
			"replica": schema.Int64Attribute{
				MarkdownDescription: "The number of compute node replicas in the resource group. At least 1; the " +
					"maximum depends on the component type.",
				Required: true,
				Validators: []validator.Int64{
					positiveReplicaValidator{},
				},
			},
			"compute_cache_size_gb": schema.Int64Attribute{
				MarkdownDescription: "The compute cache size in GB. It is resolved by the platform and cannot be set.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
					unknownOnComponentTypeChange{},
				},
			},
		},
	}
}

func (r *ClusterResourceGroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(cloudsdk.CloudClientInterface)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected cloudsdk.CloudClientInterface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func (r *ClusterResourceGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterResourceGroupModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	// the name format and the replica range are enforced by the schema validators.
	checkNotDefaultResourceGroup(name, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	nsID, err := uuid.Parse(data.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("cluster_id is invalid", fmt.Sprintf("Cannot parse cluster ID %s", data.ClusterID.String()))
		return
	}

	// Record the identifier before waiting for the rescale. The request that follows is
	// accepted long before the compute nodes are up, so if the wait fails or times out the
	// resource group exists on the platform. Without an id in the state terraform would forget
	// about it and the next apply would fail trying to create it again; with one, the resource
	// is tainted and the next apply can replace or destroy it.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), fmt.Sprintf("%s.%s", nsID.String(), name))...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := r.client.CreateResourceGroupAwait(ctx, nsID, apigen_mgmtv2.CreateResourceGroupsRequestBody{
		Name: name,
		Resource: apigen_mgmtv2.ComponentResourceRequest{
			ComponentTypeId: data.ComponentTypeID.ValueString(),
			Replica:         int(data.Replica.ValueInt64()),
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create resource group", err.Error())
		return
	}

	clusterResourceGroupToDataModel(nsID, group, &data)

	tflog.Info(ctx, fmt.Sprintf("resource group created, name: %s", name))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func clusterResourceGroupToDataModel(clusterNsID uuid.UUID, group *apigen_mgmtv2.ResourceGroupDetails, data *ClusterResourceGroupModel) {
	data.ID = types.StringValue(fmt.Sprintf("%s.%s", clusterNsID.String(), group.Name))
	data.ClusterID = types.StringValue(clusterNsID.String())
	data.Name = types.StringValue(group.Name)
	data.ComponentTypeID = types.StringValue(group.Resource.ComponentTypeId)
	data.Replica = types.Int64Value(int64(group.Resource.Replica))
	data.ComputeCacheSizeGB = types.Int64Value(int64(group.ComputeCache.SizeGb))
}

// parseClusterResourceGroupIdentifier splits `[cluster ID].[resource group name]`. The split is
// unambiguous because a resource group name cannot contain a dot, see resourceGroupNamePattern.
func parseClusterResourceGroupIdentifier(resourceGroupResourceID string, diags *diag.Diagnostics) (nsID uuid.UUID, name string) {
	arr := strings.SplitN(resourceGroupResourceID, ".", 2)
	if len(arr) != 2 || len(arr[1]) == 0 {
		diags.AddError("Invalid ID", fmt.Sprintf("Cannot parse resource group ID: %s, expected format: [cluster ID].[resource group name]", resourceGroupResourceID))
		return
	}
	var err error
	nsID, err = uuid.Parse(arr[0])
	if err != nil {
		diags.AddError("Invalid ID", fmt.Sprintf("Cannot extract cluster ID from resource group ID: %s", resourceGroupResourceID))
		return
	}
	name = arr[1]
	return
}

func (r *ClusterResourceGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClusterResourceGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsUnknown() || data.ID.IsNull() {
		resp.Diagnostics.AddError("ID is missing", "ID is required to read the resource")
		return
	}

	nsID, name := parseClusterResourceGroupIdentifier(data.ID.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := r.client.GetResourceGroup(ctx, nsID, name)
	if err != nil {
		// the resource group (or the whole cluster) is gone: report it as deleted instead of
		// failing every future plan and forcing the user to remove it from the state manually.
		if errors.Is(err, cloudsdk.ErrResourceGroupNotFound) || errors.Is(err, cloudsdk.ErrClusterNotFound) {
			tflog.Info(ctx, fmt.Sprintf("resource group %s not found, removing it from the state", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read resource group", err.Error())
		return
	}

	clusterResourceGroupToDataModel(nsID, group, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClusterResourceGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var (
		data  ClusterResourceGroupModel
		state ClusterResourceGroupModel
	)

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nsID, name := parseClusterResourceGroupIdentifier(state.ID.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := r.client.UpdateResourceGroupAwait(ctx, nsID, name, apigen_mgmtv2.UpdateResourceGroupsRequestBody{
		Resource: apigen_mgmtv2.ComponentResourceRequest{
			ComponentTypeId: data.ComponentTypeID.ValueString(),
			Replica:         int(data.Replica.ValueInt64()),
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update resource group", err.Error())
		return
	}

	clusterResourceGroupToDataModel(nsID, group, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClusterResourceGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ClusterResourceGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsUnknown() || data.ID.IsNull() {
		resp.Diagnostics.AddError("ID is missing", "ID is required to delete the resource")
		return
	}

	nsID, name := parseClusterResourceGroupIdentifier(data.ID.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteResourceGroupAwait(ctx, nsID, name); err != nil {
		// the cluster is already gone, so is the resource group. This happens when the cluster
		// is not deleted through terraform, or when the configuration does not let terraform
		// know that this resource group belongs to the cluster.
		if errors.Is(err, cloudsdk.ErrClusterNotFound) {
			tflog.Info(ctx, fmt.Sprintf("cluster %s not found, the resource group is already deleted", nsID.String()))
			return
		}
		resp.Diagnostics.AddError("Unable to delete resource group", err.Error())
		return
	}
}

func (r *ClusterResourceGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nsID, name := parseClusterResourceGroupIdentifier(req.ID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	checkNotDefaultResourceGroup(name, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.GetResourceGroup(ctx, nsID, name); err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to import resource group with ID: %s", req.ID), err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
