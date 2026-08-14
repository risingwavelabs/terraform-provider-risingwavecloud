package provider

import (
	"context"
	"fmt"
	"regexp"
	"sort"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/pkg/errors"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
)

// Assert provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ClusterAllowedIamRolesResource{}
var _ resource.ResourceWithImportState = &ClusterAllowedIamRolesResource{}

func NewClusterAllowedIamRolesResource() resource.Resource {
	return &ClusterAllowedIamRolesResource{}
}

type ClusterAllowedIamRolesResource struct {
	client cloudsdk.CloudClientInterface
}

type ClusterAllowedIamRolesModel struct {
	// the cluster's NsID: a cluster has exactly one set of allowed principals
	ID        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	RoleArns  types.Set    `tfsdk:"role_arns"`
}

// roleArnPattern mirrors the format the platform states when it rejects one:
// `validate arn failed: invalid target format, expected format:
// arn:aws:iam::{account}:role/{role_name}`. Checking it here turns an apply-time API error
// into a plan-time one.
var roleArnPattern = regexp.MustCompile(`^arn:aws:iam::\d{12}:role/\S+$`)

type roleArnValidator struct{}

func (v roleArnValidator) Description(ctx context.Context) string {
	return "value must be an AWS IAM role ARN"
}

func (v roleArnValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v roleArnValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if arn := req.ConfigValue.ValueString(); !roleArnPattern.MatchString(arn) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid IAM role ARN",
			fmt.Sprintf("Expected an ARN of the form arn:aws:iam::{account}:role/{role_name}, got: %q", arn),
		)
	}
}

func (r *ClusterAllowedIamRolesResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_allowed_iam_roles"
}

func (r *ClusterAllowedIamRolesResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "The IAM roles allowed to reach a RisingWave cluster's resources through AWS assume role.",
		MarkdownDescription: clusterAllowedIamRolesMarkdownDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The global identifier for the resource, which is the cluster's NsID: a cluster has one set of allowed principals.",
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
			"role_arns": schema.SetAttribute{
				MarkdownDescription: "The IAM role ARNs allowed to access this cluster's resources, each of the form " +
					"`arn:aws:iam::{account}:role/{role_name}`. This resource owns the whole list: an ARN added " +
					"elsewhere, in the RisingWave Cloud console for instance, is removed on the next apply.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setValidatorEach{roleArnValidator{}},
				},
			},
		},
	}
}

// setValidatorEach applies a string validator to every element of a set. The framework has no
// built-in for this without terraform-plugin-framework-validators.
type setValidatorEach struct {
	element validator.String
}

func (v setValidatorEach) Description(ctx context.Context) string {
	return v.element.Description(ctx)
}

func (v setValidatorEach) MarkdownDescription(ctx context.Context) string {
	return v.element.MarkdownDescription(ctx)
}

func (v setValidatorEach) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	for _, element := range req.ConfigValue.Elements() {
		value, ok := element.(types.String)
		if !ok {
			continue
		}
		elementResp := &validator.StringResponse{}
		v.element.ValidateString(ctx, validator.StringRequest{
			Path:        req.Path,
			ConfigValue: value,
		}, elementResp)
		resp.Diagnostics.Append(elementResp.Diagnostics...)
	}
}

func (r *ClusterAllowedIamRolesResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// roleArnsOf reads the set into a sorted slice, so that the calls the provider makes are in a
// predictable order.
func roleArnsOf(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	var arns []string
	diags.Append(set.ElementsAs(ctx, &arns, false)...)
	sort.Strings(arns)
	return arns
}

// applyAllowedIamRoles brings the cluster's list to `desired`, one call at a time.
//
// The platform answers a request that overlaps another with a 500, so these are deliberately
// sequential rather than concurrent.
//
// Whatever happens, the state is set from what the platform reports afterwards: a failure
// halfway through leaves some of the changes applied, and recording the desired list instead
// would make the next plan wrong.
func (r *ClusterAllowedIamRolesResource) applyAllowedIamRoles(
	ctx context.Context, nsID uuid.UUID, current, desired []string,
) error {
	inDesired := make(map[string]bool, len(desired))
	for _, arn := range desired {
		inDesired[arn] = true
	}
	inCurrent := make(map[string]bool, len(current))
	for _, arn := range current {
		inCurrent[arn] = true
	}

	for _, arn := range desired {
		if inCurrent[arn] {
			continue
		}
		if err := r.client.AddAllowedIamRoleAwait(ctx, nsID, arn); err != nil {
			return errors.Wrapf(err, "failed to allow the IAM role %s", arn)
		}
	}
	for _, arn := range current {
		if inDesired[arn] {
			continue
		}
		if err := r.client.RemoveAllowedIamRoleAwait(ctx, nsID, arn); err != nil {
			return errors.Wrapf(err, "failed to remove the IAM role %s", arn)
		}
	}
	return nil
}

// setStateFromPlatform records the list the platform actually has. It is called after a
// successful change and after a failed one, since in both cases it is the only truthful answer.
func (r *ClusterAllowedIamRolesResource) setStateFromPlatform(
	ctx context.Context, nsID uuid.UUID, state *tfsdk.State, diags *diag.Diagnostics,
) {
	arns, err := r.client.GetAllowedIamRoles(ctx, nsID)
	if err != nil {
		diags.AddError("Unable to read the allowed IAM roles", err.Error())
		return
	}
	sort.Strings(arns)

	value, valueDiags := types.SetValueFrom(ctx, types.StringType, arns)
	diags.Append(valueDiags...)
	if diags.HasError() {
		return
	}

	diags.Append(state.SetAttribute(ctx, path.Root("id"), nsID.String())...)
	diags.Append(state.SetAttribute(ctx, path.Root("cluster_id"), nsID.String())...)
	diags.Append(state.SetAttribute(ctx, path.Root("role_arns"), value)...)
}

func (r *ClusterAllowedIamRolesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterAllowedIamRolesModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nsID, err := uuid.Parse(data.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("cluster_id is invalid", fmt.Sprintf("Cannot parse cluster ID %s", data.ClusterID.String()))
		return
	}

	desired := roleArnsOf(ctx, data.RoleArns, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.client.GetAllowedIamRoles(ctx, nsID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the allowed IAM roles", err.Error())
		return
	}

	applyErr := r.applyAllowedIamRoles(ctx, nsID, current, desired)

	// recorded even when the change failed halfway: some of it may have been applied
	r.setStateFromPlatform(ctx, nsID, &resp.State, &resp.Diagnostics)
	if applyErr != nil {
		resp.Diagnostics.AddError("Unable to set the allowed IAM roles", applyErr.Error())
		return
	}

	tflog.Info(ctx, fmt.Sprintf("allowed IAM roles set on cluster %s", nsID))
}

func (r *ClusterAllowedIamRolesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClusterAllowedIamRolesModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nsID, err := uuid.Parse(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("ID is invalid", fmt.Sprintf("Cannot parse cluster ID %s", data.ID.String()))
		return
	}

	arns, err := r.client.GetAllowedIamRoles(ctx, nsID)
	if err != nil {
		// the cluster is gone, and with it the list: report it as deleted rather than failing
		// every future plan.
		if errors.Is(err, cloudsdk.ErrClusterNotFound) {
			tflog.Info(ctx, fmt.Sprintf("cluster %s not found, removing the allowed IAM roles from the state", nsID))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read the allowed IAM roles", err.Error())
		return
	}
	sort.Strings(arns)

	value, valueDiags := types.SetValueFrom(ctx, types.StringType, arns)
	resp.Diagnostics.Append(valueDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = types.StringValue(nsID.String())
	data.ClusterID = types.StringValue(nsID.String())
	data.RoleArns = value

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClusterAllowedIamRolesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state ClusterAllowedIamRolesModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nsID, err := uuid.Parse(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("ID is invalid", fmt.Sprintf("Cannot parse cluster ID %s", state.ID.String()))
		return
	}

	desired := roleArnsOf(ctx, data.RoleArns, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// the platform is asked rather than trusting the state, so that an ARN added elsewhere is
	// removed as the authoritative list demands
	current, err := r.client.GetAllowedIamRoles(ctx, nsID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the allowed IAM roles", err.Error())
		return
	}

	applyErr := r.applyAllowedIamRoles(ctx, nsID, current, desired)

	r.setStateFromPlatform(ctx, nsID, &resp.State, &resp.Diagnostics)
	if applyErr != nil {
		resp.Diagnostics.AddError("Unable to update the allowed IAM roles", applyErr.Error())
		return
	}
}

func (r *ClusterAllowedIamRolesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ClusterAllowedIamRolesModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nsID, err := uuid.Parse(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("ID is invalid", fmt.Sprintf("Cannot parse cluster ID %s", data.ID.String()))
		return
	}

	current, err := r.client.GetAllowedIamRoles(ctx, nsID)
	if err != nil {
		// the cluster is already gone, so is its list
		if errors.Is(err, cloudsdk.ErrClusterNotFound) {
			return
		}
		resp.Diagnostics.AddError("Unable to read the allowed IAM roles", err.Error())
		return
	}

	if err := r.applyAllowedIamRoles(ctx, nsID, current, nil); err != nil {
		resp.Diagnostics.AddError("Unable to remove the allowed IAM roles", err.Error())
		return
	}
}

func (r *ClusterAllowedIamRolesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nsID, err := uuid.Parse(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid ID",
			fmt.Sprintf("Expected the cluster ID, got: %s", req.ID),
		)
		return
	}

	if _, err := r.client.GetAllowedIamRoles(ctx, nsID); err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to import the allowed IAM roles of cluster %s", req.ID), err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), nsID.String())...)
}
