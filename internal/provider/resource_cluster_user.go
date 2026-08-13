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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
)

// Assert provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ClusterUserResource{}
var _ resource.ResourceWithImportState = &ClusterUserResource{}
var _ resource.ResourceWithValidateConfig = &ClusterUserResource{}

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
	// PasswordWO is always null here: terraform never puts a write-only value in the plan or
	// the state. The field exists because the model has to mirror the schema; the value is
	// read from the configuration in Create and Update. Never assign to it.
	PasswordWO        types.String `tfsdk:"password_wo"`
	PasswordWOVersion types.Int64  `tfsdk:"password_wo_version"`
	CreateDB          types.Bool   `tfsdk:"create_db"`
	SuperUser         types.Bool   `tfsdk:"super_user"`
	CreateUser        types.Bool   `tfsdk:"create_user"`
	CanLogin          types.Bool   `tfsdk:"can_login"`
}

// readWriteOnlyPassword returns the write-only password from the configuration. Write-only
// values only exist there, never in the plan or the state.
func readWriteOnlyPassword(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) string {
	var password types.String
	diags.Append(config.GetAttribute(ctx, path.Root("password_wo"), &password)...)
	return password.ValueString()
}

func (r *ClusterUserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_user"
}

func (r *ClusterUserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "A cluster user is a database user that can connect to a cluster.",
		MarkdownDescription: clusterUserMarkdownDescription,
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
				MarkdownDescription: "The password for connecting to the cluster. This value is stored in the Terraform " +
					"state in plain text; use `password_wo` instead if the state must not hold the secret.",
				Optional:  true,
				Sensitive: true,
			},
			"password_wo": schema.StringAttribute{
				MarkdownDescription: "The password for connecting to the cluster, as a " +
					"[write-only argument](https://developer.hashicorp.com/terraform/language/manage-sensitive-data/write-only): " +
					"Terraform sends it to the provider but stores it in neither the plan nor the state. Requires Terraform " +
					"1.11 or later, and must be set together with `password_wo_version`. Conflicts with `password`.",
				Optional:  true,
				Sensitive: true,
				WriteOnly: true,
			},
			"password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "The version of `password_wo`. Terraform cannot detect a change in a value it does " +
					"not store, so increment this whenever `password_wo` changes to have the new password applied. Must be " +
					"set together with `password_wo`.",
				Optional: true,
			},
			"super_user": schema.BoolAttribute{
				MarkdownDescription: "Whether the user is a superuser (`SUPERUSER`). Cannot be changed after the user is created.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"create_db": schema.BoolAttribute{
				MarkdownDescription: "Whether the user may create databases (`CREATEDB`). Cannot be changed after the user is created.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"create_user": schema.BoolAttribute{
				MarkdownDescription: "Whether the user may create other users and roles (`CREATEUSER`, the legacy spelling of " +
					"`CREATEROLE`). This is the \"Allow creating roles\" option in the RisingWave Cloud portal. Cannot be " +
					"changed after the user is created.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"can_login": schema.BoolAttribute{
				MarkdownDescription: "Whether the user may log in (`LOGIN`). Users created here can always log in, so this " +
					"is reported by the platform rather than configured.",
				Computed: true,
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

// ValidateConfig enforces the relationship between the two ways of supplying a password. The
// framework has no declarative equivalent without pulling in terraform-plugin-framework-validators.
func (r *ClusterUserResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data ClusterUserModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A value that is unknown at this point still counts as supplied: it comes from a variable
	// or from another resource, and rejecting it here would reject a valid configuration.
	var (
		hasPassword  = !data.Password.IsNull()
		hasWriteOnly = !data.PasswordWO.IsNull()
		hasVersion   = !data.PasswordWOVersion.IsNull()
	)

	switch {
	case hasPassword && hasWriteOnly:
		resp.Diagnostics.AddError(
			"Conflicting password arguments",
			"Only one of \"password\" and \"password_wo\" can be set. \"password_wo\" keeps the password out of the "+
				"Terraform state and requires Terraform 1.11 or later.",
		)
	case !hasPassword && !hasWriteOnly:
		resp.Diagnostics.AddError(
			"Missing password",
			"One of \"password\" or \"password_wo\" must be set to create a cluster user.",
		)
	}

	if hasWriteOnly != hasVersion {
		resp.Diagnostics.AddAttributeError(
			path.Root("password_wo_version"),
			"Incomplete write-only password",
			"\"password_wo\" and \"password_wo_version\" must be set together. Terraform cannot detect a change in a "+
				"write-only value, so the version is what tells the provider to apply a new password.",
		)
	}
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
		// the role flags all default to false in the schema, and ValueBool reports false for a
		// null or unknown value anyway, so they can be read directly.
		createDB   = data.CreateDB.ValueBool()
		superUser  = data.SuperUser.ValueBool()
		createUser = data.CreateUser.ValueBool()
	)

	// the write-only password never reaches the plan, only the configuration holds it.
	if data.Password.IsNull() {
		password = readWriteOnlyPassword(ctx, req.Config, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if len(username) == 0 {
		resp.Diagnostics.AddError("Username is required", "Username is required")
		return
	}

	if len(password) == 0 {
		resp.Diagnostics.AddError("Password is required", "A password is required to create a cluster user")
		return
	}

	nsID, err := uuid.Parse(clusterID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid cluster ID", fmt.Sprintf("Cannot parse cluster ID: %s", clusterID))
		return
	}

	createdUser, err := r.client.CreateClusterUser(ctx, nsID, username, password, createDB, superUser, createUser)
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

// clusterUserToDataModel converts the user from the API to the data model.
// it does not overwrite the password as we cannot know the password through API.
func clusterUserToDataModel(clusterNsID uuid.UUID, user *apigen_mgmtv2.DBUser, data *ClusterUserModel) {
	data.ID = types.StringValue(fmt.Sprintf("%s.%s", clusterNsID.String(), user.Username))
	data.ClusterID = types.StringValue(clusterNsID.String())
	data.CreateDB = types.BoolValue(user.Usecreatedb)
	data.SuperUser = types.BoolValue(user.Usesuper)
	data.CreateUser = types.BoolValue(user.Usecreateuser)
	data.CanLogin = types.BoolValue(user.Canlogin)
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

	// the API can only update a password, so the role flags are fixed once the user exists.
	// Recreating the user would drop it along with everything granted to it, so this is
	// reported as an error rather than planned as a replacement.
	if data.CreateUser != state.CreateUser {
		resp.Diagnostics.AddError(
			"Cannot update immutable field",
			fmt.Sprintf("CreateUser cannot be updated, previous: %s, new: %s", state.CreateUser, data.CreateUser),
		)
		return
	}

	// Which password to send, if any. A nil result means there is nothing to send, which is
	// not the same as an empty one.
	//
	//   - the write-only value is invisible to terraform, so its version argument is the only
	//     signal that it changed
	//   - the plain attribute changing to null means the practitioner moved to password_wo,
	//     not that they want an empty password
	var newPassword *string

	switch {
	case !data.PasswordWOVersion.Equal(state.PasswordWOVersion):
		password := readWriteOnlyPassword(ctx, req.Config, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		newPassword = &password
	case !data.Password.IsNull() && !data.Password.Equal(state.Password):
		password := data.Password.ValueString()
		newPassword = &password
	}

	if newPassword != nil {
		if len(*newPassword) == 0 {
			resp.Diagnostics.AddError("Password is required", "The new password cannot be empty")
			return
		}
		if err := r.client.UpdateClusterUserPassword(ctx, stateNsID, stateUsername, *newPassword); err != nil {
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
