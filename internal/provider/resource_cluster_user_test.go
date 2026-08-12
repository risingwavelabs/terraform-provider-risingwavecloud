package provider

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The API reports four PostgreSQL role attributes and the resource has to carry all of them:
// `usecreateuser` is the "Allow creating roles" option in the portal, and `canlogin` is
// read-only because the platform creates users with CREATE USER, which implies LOGIN.
func TestClusterUserToDataModel(t *testing.T) {
	nsID := uuid.Must(uuid.NewRandom())

	tests := []struct {
		name string
		user apigen_mgmtv2.DBUser
	}{
		{
			name: "all roles granted",
			user: apigen_mgmtv2.DBUser{
				Username:      "test-user",
				Usesuper:      true,
				Usecreatedb:   true,
				Usecreateuser: true,
				Canlogin:      true,
			},
		},
		{
			name: "no roles granted",
			user: apigen_mgmtv2.DBUser{
				Username: "test-user",
				Canlogin: true,
			},
		},
		{
			name: "only the role the provider used to ignore",
			user: apigen_mgmtv2.DBUser{
				Username:      "test-user",
				Usecreateuser: true,
				Canlogin:      true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data ClusterUserModel
			clusterUserToDataModel(nsID, &tt.user, &data)

			assert.Equal(t, nsID.String()+".test-user", data.ID.ValueString())
			assert.Equal(t, nsID.String(), data.ClusterID.ValueString())
			assert.Equal(t, tt.user.Username, data.Username.ValueString())
			assert.Equal(t, tt.user.Usesuper, data.SuperUser.ValueBool())
			assert.Equal(t, tt.user.Usecreatedb, data.CreateDB.ValueBool())
			assert.Equal(t, tt.user.Usecreateuser, data.CreateUser.ValueBool())
			assert.Equal(t, tt.user.Canlogin, data.CanLogin.ValueBool())
		})
	}
}

// A cluster user needs exactly one of the two password arguments, and the write-only one is
// useless without its version, since that is the only thing terraform can see change.
func TestClusterUserValidateConfig(t *testing.T) {
	ctx := context.Background()

	resp := &resource.SchemaResponse{}
	(&ClusterUserResource{}).Schema(ctx, resource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError())
	sch := resp.Schema

	objType, ok := sch.Type().TerraformType(ctx).(tftypes.Object)
	require.True(t, ok)

	config := func(password, passwordWO tftypes.Value, version tftypes.Value) tftypes.Value {
		return tftypes.NewValue(objType, map[string]tftypes.Value{
			"id":                  tftypes.NewValue(tftypes.String, "cluster.user"),
			"cluster_id":          tftypes.NewValue(tftypes.String, "cluster"),
			"username":            tftypes.NewValue(tftypes.String, "test-user"),
			"password":            password,
			"password_wo":         passwordWO,
			"password_wo_version": version,
			"create_db":           tftypes.NewValue(tftypes.Bool, false),
			"super_user":          tftypes.NewValue(tftypes.Bool, false),
			"create_user":         tftypes.NewValue(tftypes.Bool, false),
			"can_login":           tftypes.NewValue(tftypes.Bool, true),
		})
	}

	var (
		str     = func(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
		nullStr = tftypes.NewValue(tftypes.String, nil)
		num     = func(i int64) tftypes.Value { return tftypes.NewValue(tftypes.Number, i) }
		nullNum = tftypes.NewValue(tftypes.Number, nil)
	)

	tests := []struct {
		name    string
		value   tftypes.Value
		wantErr bool
	}{
		{name: "password only", value: config(str("secret"), nullStr, nullNum)},
		{name: "write-only with version", value: config(nullStr, str("secret"), num(1))},
		{name: "both passwords", value: config(str("secret"), str("secret"), num(1)), wantErr: true},
		{name: "no password", value: config(nullStr, nullStr, nullNum), wantErr: true},
		{name: "write-only without version", value: config(nullStr, str("secret"), nullNum), wantErr: true},
		{name: "version without write-only", value: config(str("secret"), nullStr, num(1)), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validateResp := &resource.ValidateConfigResponse{}
			(&ClusterUserResource{}).ValidateConfig(ctx, resource.ValidateConfigRequest{
				Config: tfsdk.Config{Raw: tt.value, Schema: sch},
			}, validateResp)

			assert.Equal(t, tt.wantErr, validateResp.Diagnostics.HasError())
		})
	}
}

// The password never comes back from the API, so the model conversion has to leave whatever
// the state already holds untouched.
func TestClusterUserToDataModelKeepsPassword(t *testing.T) {
	nsID := uuid.Must(uuid.NewRandom())

	data := ClusterUserModel{Password: types.StringValue("secret")}
	clusterUserToDataModel(nsID, &apigen_mgmtv2.DBUser{Username: "test-user"}, &data)

	assert.Equal(t, "secret", data.Password.ValueString())
}
