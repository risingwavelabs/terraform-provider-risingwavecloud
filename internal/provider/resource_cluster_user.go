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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt"
)

var passwordMask = "******"

// Assert provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ClusterResource{}
var _ resource.ResourceWithImportState = &ClusterResource{}

func NewClusterUserResource() resource.Resource {
	return &ClusterUserResource{}
}

type ClusterUserResource struct {
	client cloudsdk.CloudClientInterface
}

type ClusterUserModel struct {
	// [cluster ID].[username]
	ID        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	Username  types.String `tfsdk:"username"`
	Password  types.String `tfsdk:"password"`
	CreateDB  types.Bool   `tfsdk:"create_db"`
	SuperUser types.Bool   `tfsdk:"super_user"`
}

func (r *ClusterUserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_user"
}

func (r *ClusterUserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A RisingWave Cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The global identifier for the resource: [cluster ID].[username]",
				Computed:            true,
			},
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "The NsID (namespace id) of the cluster.",
				Required:            true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "The username for connecting to the cluster. The username is unique within the cluster.",
				Required:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "The password for connecting to the cluster",
				Required:            true,
				Sensitive:           true,
			},
			"super_user": schema.BoolAttribute{
				MarkdownDescription: "The super user flag for the user",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"create_db": schema.BoolAttribute{
				MarkdownDescription: "The create db flag for the user",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (r *ClusterUserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ClusterUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterUserModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		username  = data.Username.ValueString()
		password  = data.Password.ValueString()
		clusterID = data.ClusterID.ValueString()
		createDB  = false
		superUser = false
	)

	if !data.CreateDB.IsUnknown() {
		createDB = data.CreateDB.ValueBool()
	}

	if !data.SuperUser.IsUnknown() {
		superUser = data.SuperUser.ValueBool()
	}

	if len(username) == 0 {
		resp.Diagnostics.AddError("Username is required", "Username is required")
		return
	}

	if len(password) == 0 {
		resp.Diagnostics.AddError("Password is required", "Username is required")
		return
	}

	nsID, err := uuid.Parse(clusterID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid cluster ID", fmt.Sprintf("Cannot parse cluster ID: %s", clusterID))
		return
	}

	createdUser, err := r.client.CreateCluserUser(ctx, nsID, username, password, createDB, superUser)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create cluster user", err.Error())
		return
	}

	// the password is stored in the state to avoid inconsistency error.
	clusterUserToDataModel(nsID, createdUser, &data)

	tflog.Info(ctx, fmt.Sprintf("cluster user created, username: %s", username))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func parseClusterUserIdentifier(clusterUserResourceID string, diags *diag.Diagnostics) (nsID uuid.UUID, username string) {
	arr := strings.Split(clusterUserResourceID, ".")
	if len(arr) != 2 {
		diags.AddError("Invalid ID", fmt.Sprintf("Cannot parse cluster user ID: %s", clusterUserResourceID))
		return
	}
	var err error
	nsID, err = uuid.Parse(arr[0])
	if err != nil {
		diags.AddError("Invalid ID", fmt.Sprintf("Cannot extract cluster ID from cluster user ID: %s", clusterUserResourceID))
		return
	}
	username = arr[1]
	return
}

type DataGetter interface {
	Get(ctx context.Context, target interface{}) diag.Diagnostics
}

// clusterUserToDataModel converts the user from the API to the data model.
// it does not overwrite the password as we cannot know the password through API.
func clusterUserToDataModel(clusterNsID uuid.UUID, user *apigen.DBUser, data *ClusterUserModel) {
	data.ID = types.StringValue(fmt.Sprintf("%s.%s", clusterNsID.String(), user.Username))
	data.ClusterID = types.StringValue(clusterNsID.String())
	data.CreateDB = types.BoolValue(user.Usecreatedb)
	data.SuperUser = types.BoolValue(user.Usesuper)
	data.Username = types.StringValue(user.Username)
}

func (r *ClusterUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClusterUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsUnknown() || data.ID.IsNull() {
		resp.Diagnostics.AddError("ID is missing", "ID is required to read the resource")
		return
	}

	nsID, username := parseClusterUserIdentifier(data.ID.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	user, err := r.client.GetClusterUser(ctx, nsID, username)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cluster user", err.Error())
		return
	}

	// it uses password stored in the state
	clusterUserToDataModel(nsID, user, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClusterUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var (
		data  ClusterUserModel
		state ClusterUserModel
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

	stateNsID, stateUsername := parseClusterUserIdentifier(state.ID.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.Username != state.Username {
		resp.Diagnostics.AddError(
			"Cannot update immutable field",
			fmt.Sprintf("Username cannot be updated, previous: %s, new: %s", state.Username, data.Username),
		)
		return
	}

	if data.CreateDB != state.CreateDB {
		resp.Diagnostics.AddError(
			"Cannot update immutable field",
			fmt.Sprintf("CreateDB cannot be updated, previous: %s, new: %s", state.CreateDB, data.CreateDB),
		)
		return
	}

	if data.SuperUser != state.SuperUser {
		resp.Diagnostics.AddError(
			"Cannot update immutable field",
			fmt.Sprintf("SuperUser cannot be updated, previous: %s, new: %s", state.SuperUser, data.SuperUser),
		)
		return
	}

	if data.Password != state.Password {
		if err := r.client.UpdateClusterUserPassword(ctx, stateNsID, stateUsername, data.Password.ValueString()); err != nil {
			resp.Diagnostics.AddError("Unable to update cluster user password", err.Error())
			return
		}
	}

	user, err := r.client.GetClusterUser(ctx, stateNsID, state.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cluster user", err.Error())
		return
	}

	// the password is stored in the state to avoid inconsistency error.
	clusterUserToDataModel(stateNsID, user, &data)

	// Directly save the plan to the state since we cannot know the password through API.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClusterUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ClusterUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsUnknown() || data.ID.IsNull() {
		resp.Diagnostics.AddError("ID is missing", "ID is required to delete the resource")
		return
	}

	nsID, username := parseClusterUserIdentifier(data.ID.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteClusterUser(ctx, nsID, username); err != nil {
		resp.Diagnostics.AddError("Unable to delete cluster user", err.Error())
		return
	}
}

func (r *ClusterUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nsID, username := parseClusterUserIdentifier(req.ID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.GetClusterUser(ctx, nsID, username); err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to import cluster user with ID: %s", req.ID), err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
