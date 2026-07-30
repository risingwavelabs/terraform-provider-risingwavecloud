package acctest

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	testResourceGroupRegion  = "us-east-1"
	testResourceGroupVersion = "v3.0.1"
)

// TestClusterResourceGroupResource exercises the cluster_resource_group resource on top of a
// managed cluster: create, read, import, rescaling it and moving it to another component type.
func TestClusterResourceGroupResource(t *testing.T) {
	clusterName := fmt.Sprintf("tf%srg", getTestNamespace(t))
	cloud := initCloudSDK(t)

	var clusterID uuid.UUID

	captureClusterID := func(s *terraform.State) error {
		cluster, err := cloud.GetClusterByRegionAndName(context.Background(), testResourceGroupRegion, clusterName)
		if err != nil {
			return err
		}
		clusterID = cluster.NsId
		return nil
	}

	config := func(componentTypeID string, replica int) string {
		return testResourceGroupCluster(clusterName) +
			testResourceGroup(componentTypeID, replica)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read: cluster + resource group
			{
				Config: config("p-1c4g", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureClusterID,
					resource.TestCheckResourceAttrSet("risingwavecloud_cluster_resource_group.test", "id"),
					resource.TestCheckResourceAttrPair(
						"risingwavecloud_cluster_resource_group.test", "cluster_id",
						"risingwavecloud_cluster.test", "id",
					),
					resource.TestCheckResourceAttr("risingwavecloud_cluster_resource_group.test", "name", "streaming-rg"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster_resource_group.test", "component_type_id", "p-1c4g"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster_resource_group.test", "replica", "1"),
					resource.TestCheckResourceAttrSet("risingwavecloud_cluster_resource_group.test", "compute_cache_size_gb"),
				),
			},
			// Import
			{
				Config:       config("p-1c4g", 1),
				ResourceName: "risingwavecloud_cluster_resource_group.test",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return fmt.Sprintf("%s.streaming-rg", clusterID.String()), nil
				},
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update: rescale replica 1 -> 2
			{
				Config: config("p-1c4g", 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster_resource_group.test", "replica", "2"),
				),
			},
			// Update: move to another component type. The compute cache size is resolved by the
			// platform from the component type, so it must be re-planned as unknown instead of
			// keeping the value of the previous component type.
			{
				Config: config("p-2c8g", 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster_resource_group.test", "component_type_id", "p-2c8g"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster_resource_group.test", "replica", "2"),
					resource.TestCheckResourceAttrSet("risingwavecloud_cluster_resource_group.test", "compute_cache_size_gb"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// testResourceGroupCluster is the smallest cluster a resource group can live on: resource
// groups add compute nodes, so the cluster needs a tier with a separate compute component
// (a Standard tier cluster runs standalone and has none).
//
// The values below have to exist in the target environment. To refresh them, check
// `GET <region mgmt url>/api/v1/tiers` for the tiers, component types and meta store types,
// and `GET <region mgmt url>/api/v1/tenant/tags` for the current RisingWave version.
func testResourceGroupCluster(name string) string {
	return fmt.Sprintf(`
resource "risingwavecloud_cluster" "test" {
	region   = "%s"
	name     = "%s"
	version  = "%s"
	tier     = "Invited"
	spec     = {
		compute = {
			default_node_group = {
				cpu     = "1"
				memory  = "4 GB"
				replica = 1
			}
		}
		compactor = {
			default_node_group = {
				cpu     = "1"
				memory  = "4 GB"
				replica = 1
			}
		}
		frontend = {
			default_node_group = {
				cpu     = "1"
				memory  = "4 GB"
				replica = 1
			}
		}
		meta = {
			default_node_group = {
				cpu     = "1"
				memory  = "4 GB"
				replica = 1
			}
		}
	}
}
`, testResourceGroupRegion, name, testResourceGroupVersion)
}

func testResourceGroup(componentTypeID string, replica int) string {
	return fmt.Sprintf(`
resource "risingwavecloud_cluster_resource_group" "test" {
	cluster_id        = risingwavecloud_cluster.test.id
	name              = "streaming-rg"
	component_type_id = "%s"
	replica           = %d
}
`, componentTypeID, replica)
}
