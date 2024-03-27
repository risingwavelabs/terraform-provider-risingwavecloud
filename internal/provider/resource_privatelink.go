package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/defaults"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/wait"
)

// Assert provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &PrivateLinkResource{}
var _ resource.ResourceWithImportState = &PrivateLinkResource{}

func NewPrivateLinkResource() resource.Resource {
	return &PrivateLinkResource{}
}

type PrivateLinkResource struct {
	client cloudsdk.CloudClientInterface
}

type PrivateLinkModel struct {
	ID             types.String `tfsdk:"id"`
	ClusterID      types.String `tfsdk:"cluster_id"`
	ConnectionName types.String `tfsdk:"connection_name"`
	Target         types.String `tfsdk:"target"`
	Endpoint       types.String `tfsdk:"endpoint"`
}

func (r *PrivateLinkResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_privatelink"
}

func (r *PrivateLinkResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "A Private Link connection on the RisingWave Cloud platform.",
		MarkdownDescription: privateLinkMarkdownDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The global identifier for the resource in format of UUID.",
				Computed:            true,
			},
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "The NsID (namespace id) of the cluster in format of UUID.",
				Required:            true,
			},
			"connection_name": schema.StringAttribute{
				MarkdownDescription: "The name of the Private Link connection, just for display purpose.",
				Required:            true,
			},
			"target": schema.StringAttribute{
				MarkdownDescription: "The target of the Private Link connection. In AWS, it is the service name of the VPC endpoint service. In GCP, it is the service attachment in Private Service Connect.",
				Required:            true,
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "The endpoint of the Private Link to connect to. This has different format for different platforms.",
				Computed:            true,
			},
		},
	}
}

func (r *PrivateLinkResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(cloudsdk.CloudClientInterface)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected cloudsdk.AccountServiceClientInterface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func (r *PrivateLinkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PrivateLinkModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nsID, err := uuid.Parse(data.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("ClusterID is invalid", fmt.Sprintf("Cannot parse cluster ID %s", data.ClusterID.String()))
		return
	}

	if len(data.ConnectionName.ValueString()) == 0 {
		resp.Diagnostics.AddError("connection_name is missing", "connection_name is required to create the private link resource")
		return
	}

	if len(data.Target.ValueString()) == 0 {
		resp.Diagnostics.AddError("target is missing", "target is required to create the private link resource")
		return
	}

	pl, err := r.client.CreatePrivateLinkAwait(ctx, nsID, apigen.PostPrivateLinkRequestBody{
		ConnectionName: data.ConnectionName.ValueString(),
		Target:         data.Target.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Create failed", err.Error())
		return
	}

	privateLinkToDataModel(pl, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func privateLinkToDataModel(plInfo *cloudsdk.PrivateLinkInfo, data *PrivateLinkModel) {
	data.ID = types.StringValue(plInfo.PrivateLink.Id.String())
	data.ClusterID = types.StringValue(plInfo.ClusterNsID.String())
	data.ConnectionName = types.StringValue(plInfo.PrivateLink.ConnectionName)
	data.Target = types.StringValue(defaults.UnwrapOr(plInfo.PrivateLink.Target, ""))
	data.Endpoint = types.StringValue(defaults.UnwrapOr(plInfo.PrivateLink.Endpoint, ""))
}

func (r *PrivateLinkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PrivateLinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsUnknown() || data.ID.IsNull() {
		resp.Diagnostics.AddError("ID is missing", "ID is required to read the resource")
		return
	}

	privateLinkID, err := uuid.Parse(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("ID is invalid", fmt.Sprintf("Cannot parse private link ID %s", data.ID.String()))
		return
	}

	plInfo, err := r.client.GetPrivateLink(ctx, privateLinkID)
	if err != nil {
		resp.Diagnostics.AddError("Read failed", err.Error())
		return
	}
	privateLinkToDataModel(plInfo, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PrivateLinkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var (
		data  PrivateLinkModel
		state PrivateLinkModel
	)

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// all field are immutable
	if data.ConnectionName.ValueString() != state.ConnectionName.ValueString() {
		resp.Diagnostics.AddError("connection_name is immutable", "connection_name cannot be changed")
		return
	}
	if data.Endpoint.ValueString() != state.Endpoint.ValueString() {
		resp.Diagnostics.AddError("endpoint is immutable after creation", "endpoint cannot be changed")
		return
	}
	if data.Target.ValueString() != state.Target.ValueString() {
		resp.Diagnostics.AddError("target is immutable", "target cannot be changed")
		return
	}
	if data.ClusterID.ValueString() != state.ClusterID.ValueString() {
		resp.Diagnostics.AddError("cluster_id is immutable", "cluster_id cannot be changed")
		return
	}

	data.ID = state.ID

	// Directly save the plan to the state since we cannot know the password through API.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PrivateLinkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PrivateLinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsUnknown() || data.ID.IsNull() {
		resp.Diagnostics.AddError("ID is missing", "ID is required to delete the resource")
		return
	}

	clusterNsID, err := uuid.Parse(data.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("ClusterID is invalid", fmt.Sprintf("Cannot parse cluster ID %s", data.ClusterID.String()))
		return
	}

	privateLinkID, err := uuid.Parse(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("ID is invalid", fmt.Sprintf("Cannot parse private link ID %s", data.ID.String()))
		return
	}

	if err := r.client.DeletePrivateLinkAwait(ctx, clusterNsID, privateLinkID); err != nil {
		if errors.Is(err, wait.ErrWaitTimeout) {
			resp.Diagnostics.AddError(
				"Timeout waiting for privatelink to be deleted",
				err.Error(),
			)
			return
		} else {
			resp.Diagnostics.AddError(
				"Delete failed",
				err.Error(),
			)
			return
		}
	}
}

func (r *PrivateLinkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := uuid.Parse(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("ID is invalid", fmt.Sprintf("Cannot parse private link ID %s", req.ID))
		return
	}

	_, err = r.client.GetPrivateLink(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Import failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
