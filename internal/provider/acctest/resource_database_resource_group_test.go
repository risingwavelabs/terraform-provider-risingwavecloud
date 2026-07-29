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
// resources on top of a managed cluster: create, read, import, and (for the
// resource group) rescale.
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

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read: cluster + resource group + database
			{
				Config: testClusterResourceConfig_newVersion(clusterName) +
					testResourceGroup(1) +
					testDatabase(),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureClusterID,
					resource.TestCheckResourceAttrSet("risingwavecloud_resource_group.test", "id"),
					resource.TestCheckResourceAttr("risingwavecloud_resource_group.test", "name", "streaming-rg"),
					resource.TestCheckResourceAttr("risingwavecloud_resource_group.test", "component_type_id", "p-1c4g"),
					resource.TestCheckResourceAttr("risingwavecloud_resource_group.test", "replica", "1"),
					resource.TestCheckResourceAttrSet("risingwavecloud_resource_group.test", "compute_cache_size_gb"),
					resource.TestCheckResourceAttrSet("risingwavecloud_database.test", "id"),
					resource.TestCheckResourceAttr("risingwavecloud_database.test", "name", "test_db"),
					resource.TestCheckResourceAttr("risingwavecloud_database.test", "resource_group", "default"),
				),
			},
			// Import resource group
			{
				Config: testClusterResourceConfig_newVersion(clusterName) +
					testResourceGroup(1) +
					testDatabase(),
				ResourceName: "risingwavecloud_resource_group.test",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return fmt.Sprintf("%s.streaming-rg", clusterID.String()), nil
				},
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Import database
			{
				Config: testClusterResourceConfig_newVersion(clusterName) +
					testResourceGroup(1) +
					testDatabase(),
				ResourceName: "risingwavecloud_database.test",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return fmt.Sprintf("%s.test_db", clusterID.String()), nil
				},
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update resource group: rescale replica 1 -> 2
			{
				Config: testClusterResourceConfig_newVersion(clusterName) +
					testResourceGroup(2) +
					testDatabase(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_resource_group.test", "replica", "2"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testResourceGroup(replica int) string {
	return fmt.Sprintf(`
resource "risingwavecloud_resource_group" "test" {
	cluster_id        = risingwavecloud_cluster.test.id
	name              = "streaming-rg"
	component_type_id = "p-1c4g"
	replica           = %d
}
`, replica)
}

func testDatabase() string {
	return `
resource "risingwavecloud_database" "test" {
	cluster_id     = risingwavecloud_cluster.test.id
	name           = "test_db"
	resource_group = "default"
}
`
}
