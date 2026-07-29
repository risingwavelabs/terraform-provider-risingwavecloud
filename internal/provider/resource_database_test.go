package provider

import (
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
	"github.com/stretchr/testify/assert"
)

func TestParseDatabaseIdentifier(t *testing.T) {
	nsID := uuid.Must(uuid.NewRandom())

	tests := []struct {
		name         string
		id           string
		expectErr    bool
		expectedName string
	}{
		{
			name:         "valid",
			id:           nsID.String() + ".test_db",
			expectedName: "test_db",
		},
		{
			name:         "database name containing a dot",
			id:           nsID.String() + ".test.db",
			expectedName: "test.db",
		},
		{
			name:      "missing database name",
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
			id:        "not-a-uuid.test_db",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diags diag.Diagnostics
			gotNsID, gotName := parseDatabaseIdentifier(tt.id, &diags)

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

func TestDatabaseToDataModel(t *testing.T) {
	nsID := uuid.Must(uuid.NewRandom())

	t.Run("resource group reported by the platform", func(t *testing.T) {
		var data DatabaseModel
		databaseToDataModel(nsID, &apigen_mgmtv2.Database{
			Name:          "test_db",
			ResourceGroup: "streaming-rg",
		}, &data)

		assert.Equal(t, nsID.String()+".test_db", data.ID.ValueString())
		assert.Equal(t, nsID.String(), data.ClusterID.ValueString())
		assert.Equal(t, "test_db", data.Name.ValueString())
		assert.Equal(t, "streaming-rg", data.ResourceGroup.ValueString())
	})

	// the resource group defaults to "default" in the schema, so an empty value reported by
	// the platform must be normalized, otherwise it shows up as a permanent diff.
	t.Run("empty resource group falls back to the default one", func(t *testing.T) {
		var data DatabaseModel
		databaseToDataModel(nsID, &apigen_mgmtv2.Database{Name: "test_db"}, &data)

		assert.Equal(t, defaultResourceGroup, data.ResourceGroup.ValueString())
	})
}
