package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
)

// defaultResourceGroup is the resource group that always exists in a cluster and is
// used when a database is created without specifying a resource group.
const defaultResourceGroup = "default"

// Assert provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &DatabaseResource{}
var _ resource.ResourceWithImportState = &DatabaseResource{}

func NewDatabaseResource() resource.Resource {
	return &DatabaseResource{}
}

type DatabaseResource struct {
	client cloudsdk.CloudClientInterface
}

type DatabaseModel struct {
	// [cluster ID].[database name]
	ID            types.String `tfsdk:"id"`
	ClusterID     types.String `tfsdk:"cluster_id"`
	Name          types.String `tfsdk:"name"`
	ResourceGroup types.String `tfsdk:"resource_group"`
}

func (r *DatabaseResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (r *DatabaseResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "A database in a RisingWave cluster.",
		MarkdownDescription: databaseMarkdownDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The global identifier for the resource: [cluster ID].[database name]",
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
				MarkdownDescription: "The name of the database. The name is unique within the cluster.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"resource_group": schema.StringAttribute{
				MarkdownDescription: "The resource group that the database's streaming jobs run in. Defaults to \"default\". Changing it re-creates the database.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(defaultResourceGroup),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *DatabaseResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatabaseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DatabaseModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		name          = data.Name.ValueString()
		resourceGroup = data.ResourceGroup.ValueString()
	)

	if len(name) == 0 {
		resp.Diagnostics.AddError("name is missing", "name is required to create the database resource")
		return
	}
	if len(resourceGroup) == 0 {
		resourceGroup = defaultResourceGroup
	}

	nsID, err := uuid.Parse(data.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("cluster_id is invalid", fmt.Sprintf("Cannot parse cluster ID %s", data.ClusterID.String()))
		return
	}

	database, err := r.client.CreateDatabase(ctx, nsID, name, resourceGroup)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create database", err.Error())
		return
	}

	databaseToDataModel(nsID, database, &data)

	tflog.Info(ctx, fmt.Sprintf("database created, name: %s", name))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func databaseToDataModel(clusterNsID uuid.UUID, database *apigen_mgmtv2.Database, data *DatabaseModel) {
	data.ID = types.StringValue(fmt.Sprintf("%s.%s", clusterNsID.String(), database.Name))
	data.ClusterID = types.StringValue(clusterNsID.String())
	data.Name = types.StringValue(database.Name)
	resourceGroup := database.ResourceGroup
	if len(resourceGroup) == 0 {
		resourceGroup = defaultResourceGroup
	}
	data.ResourceGroup = types.StringValue(resourceGroup)
}

func parseDatabaseIdentifier(databaseResourceID string, diags *diag.Diagnostics) (nsID uuid.UUID, name string) {
	arr := strings.SplitN(databaseResourceID, ".", 2)
	if len(arr) != 2 {
		diags.AddError("Invalid ID", fmt.Sprintf("Cannot parse database ID: %s", databaseResourceID))
		return
	}
	var err error
	nsID, err = uuid.Parse(arr[0])
	if err != nil {
		diags.AddError("Invalid ID", fmt.Sprintf("Cannot extract cluster ID from database ID: %s", databaseResourceID))
		return
	}
	name = arr[1]
	return
}

func (r *DatabaseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DatabaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsUnknown() || data.ID.IsNull() {
		resp.Diagnostics.AddError("ID is missing", "ID is required to read the resource")
		return
	}

	nsID, name := parseDatabaseIdentifier(data.ID.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	database, err := r.client.GetDatabase(ctx, nsID, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read database", err.Error())
		return
	}

	databaseToDataModel(nsID, database, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatabaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All attributes are immutable (RequiresReplace), so a real change re-creates the
	// resource instead of reaching Update. Persist the plan to keep the state consistent.
	var data DatabaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatabaseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DatabaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsUnknown() || data.ID.IsNull() {
		resp.Diagnostics.AddError("ID is missing", "ID is required to delete the resource")
		return
	}

	nsID, name := parseDatabaseIdentifier(data.ID.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteDatabase(ctx, nsID, name); err != nil {
		resp.Diagnostics.AddError("Unable to delete database", err.Error())
		return
	}
}

func (r *DatabaseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nsID, name := parseDatabaseIdentifier(req.ID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.GetDatabase(ctx, nsID, name); err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to import database with ID: %s", req.ID), err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
