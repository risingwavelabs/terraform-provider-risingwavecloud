package provider

import (
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/types"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
	"github.com/stretchr/testify/assert"
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

// The password never comes back from the API, so the model conversion has to leave whatever
// the state already holds untouched.
func TestClusterUserToDataModelKeepsPassword(t *testing.T) {
	nsID := uuid.Must(uuid.NewRandom())

	data := ClusterUserModel{Password: types.StringValue("secret")}
	clusterUserToDataModel(nsID, &apigen_mgmtv2.DBUser{Username: "test-user"}, &data)

	assert.Equal(t, "secret", data.Password.ValueString())
}
