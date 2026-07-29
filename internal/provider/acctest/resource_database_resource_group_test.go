package acctest

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestDatabaseAndResourceGroupResource exercises the database and resource_group
// resources on top of a managed cluster: create, read, import, rescaling the resource
// group and moving it to another component type.
//
// The database runs in the resource group created by the same configuration and
// references its name, so the test also covers the create and destroy ordering:
// the resource group has to exist before the database, and the database has to be
// dropped before the resource group it runs on.
func TestDatabaseAndResourceGroupResource(t *testing.T) {
	clusterName := fmt.Sprintf("tf%sdbrg", getTestNamespace(t))
	cloud := initCloudSDK(t)

	var clusterID uuid.UUID

	captureClusterID := func(s *terraform.State) error {
		cluster, err := cloud.GetClusterByRegionAndName(context.Background(), "us-east-1", clusterName)
		if err != nil {
			return err
		}
		clusterID = cluster.NsId
		return nil
	}

	config := func(componentTypeID string, replica int) string {
		return testClusterResourceConfig_newVersion(clusterName) +
			testResourceGroup(componentTypeID, replica) +
			testDatabase()
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read: cluster + resource group + database
			{
				Config: config("p-1c4g", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureClusterID,
					resource.TestCheckResourceAttrSet("risingwavecloud_resource_group.test", "id"),
					resource.TestCheckResourceAttr("risingwavecloud_resource_group.test", "name", "streaming-rg"),
					resource.TestCheckResourceAttr("risingwavecloud_resource_group.test", "component_type_id", "p-1c4g"),
					resource.TestCheckResourceAttr("risingwavecloud_resource_group.test", "replica", "1"),
					resource.TestCheckResourceAttrSet("risingwavecloud_resource_group.test", "compute_cache_size_gb"),
					resource.TestCheckResourceAttrSet("risingwavecloud_database.test", "id"),
					resource.TestCheckResourceAttr("risingwavecloud_database.test", "name", "test_db"),
					resource.TestCheckResourceAttrPair(
						"risingwavecloud_database.test", "resource_group",
						"risingwavecloud_resource_group.test", "name",
					),
				),
			},
			// Import resource group
			{
				Config:       config("p-1c4g", 1),
				ResourceName: "risingwavecloud_resource_group.test",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return fmt.Sprintf("%s.streaming-rg", clusterID.String()), nil
				},
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Import database
			{
				Config:       config("p-1c4g", 1),
				ResourceName: "risingwavecloud_database.test",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return fmt.Sprintf("%s.test_db", clusterID.String()), nil
				},
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update resource group: rescale replica 1 -> 2
			{
				Config: config("p-1c4g", 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_resource_group.test", "replica", "2"),
				),
			},
			// Update resource group: move to another component type. The compute cache size is
			// resolved by the platform from the component type, so it must be re-planned as
			// unknown instead of keeping the value of the previous component type.
			{
				Config: config("p-2c8g", 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_resource_group.test", "component_type_id", "p-2c8g"),
					resource.TestCheckResourceAttr("risingwavecloud_resource_group.test", "replica", "2"),
					resource.TestCheckResourceAttrSet("risingwavecloud_resource_group.test", "compute_cache_size_gb"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testResourceGroup(componentTypeID string, replica int) string {
	return fmt.Sprintf(`
resource "risingwavecloud_resource_group" "test" {
	cluster_id        = risingwavecloud_cluster.test.id
	name              = "streaming-rg"
	component_type_id = "%s"
	replica           = %d
}
`, componentTypeID, replica)
}

func testDatabase() string {
	return `
resource "risingwavecloud_database" "test" {
	cluster_id     = risingwavecloud_cluster.test.id
	name           = "test_db"
	resource_group = risingwavecloud_resource_group.test.name
}
`
}
