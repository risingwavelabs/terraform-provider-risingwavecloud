package acctest

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestClusterResourceGroupResource exercises the cluster_resource_group resource on top of a
// managed cluster: create, read, import, rescaling it and moving it to another component type.
func TestClusterResourceGroupResource(t *testing.T) {
	// Each test builds its own cluster, and the cluster is almost all of the runtime: about
	// thirteen of the twenty-odd minutes a test takes go into provisioning one and tearing it
	// down, because this tier's meta store is a dedicated RDS instance. Run them at the same
	// time and the suite costs the slowest test rather than the sum of all of them.
	t.Parallel()

	clusterName := fmt.Sprintf("tf%srg", getTestNamespace(t))
	cloud := initCloudSDK(t)
	spec := testClusterSpec(t, cloud)

	// A resource group runs compute nodes, so it can only use a compute size of the tier. It
	// starts at the same 0.5 RWU the cluster's own components use.
	firstType := spec.Compute.ID

	var clusterID uuid.UUID

	captureClusterID := func(s *terraform.State) error {
		cluster, err := cloud.GetClusterByRegionAndName(context.Background(), testRegion(), clusterName)
		if err != nil {
			return err
		}
		clusterID = cluster.NsId
		return nil
	}

	config := func(clusterComputeReplica int, componentTypeID string, replica int) string {
		return spec.terraform(clusterName, clusterComputeReplica) +
			testResourceGroup(componentTypeID, replica)
	}

	steps := []resource.TestStep{
		// Create and Read: cluster + resource group
		{
			Config: config(1, firstType, 1),
			Check: resource.ComposeAggregateTestCheckFunc(
				captureClusterID,
				resource.TestCheckResourceAttrSet("risingwavecloud_cluster_resource_group.test", "id"),
				resource.TestCheckResourceAttrPair(
					"risingwavecloud_cluster_resource_group.test", "cluster_id",
					"risingwavecloud_cluster.test", "id",
				),
				resource.TestCheckResourceAttr("risingwavecloud_cluster_resource_group.test", "name", "streaming-rg"),
				resource.TestCheckResourceAttr("risingwavecloud_cluster_resource_group.test", "component_type_id", firstType),
				resource.TestCheckResourceAttr("risingwavecloud_cluster_resource_group.test", "replica", "1"),
				resource.TestCheckResourceAttrSet("risingwavecloud_cluster_resource_group.test", "compute_cache_size_gb"),
			),
		},
		// Import
		{
			Config:       config(1, firstType, 1),
			ResourceName: "risingwavecloud_cluster_resource_group.test",
			ImportStateIdFunc: func(s *terraform.State) (string, error) {
				return fmt.Sprintf("%s.streaming-rg", clusterID.String()), nil
			},
			ImportState:       true,
			ImportStateVerify: true,
		},
		// Update: rescale replica 1 -> 2
		{
			Config: config(1, firstType, 2),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("risingwavecloud_cluster_resource_group.test", "replica", "2"),
			),
		},
	}

	// Update: move to another component type. The compute cache size is resolved by the
	// platform from the component type, so it must be re-planned as unknown instead of keeping
	// the value of the previous one. Tiers meant for testing tend to offer a single compute
	// size, in which case there is nothing to move to.
	currentType := firstType
	if next, ok := spec.NextComputeTypeAfter(firstType); ok {
		// The only step that leaves 0.5 RWU, and the only way to cover this path at all: moving
		// to another component type is what makes the platform resolve a new cache size.
		currentType = next
		steps = append(steps, resource.TestStep{
			Config: config(1, currentType, 2),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("risingwavecloud_cluster_resource_group.test", "component_type_id", currentType),
				resource.TestCheckResourceAttr("risingwavecloud_cluster_resource_group.test", "replica", "2"),
				resource.TestCheckResourceAttrSet("risingwavecloud_cluster_resource_group.test", "compute_cache_size_gb"),
			),
		})
	} else {
		t.Logf("tier %s offers no compute size above %s, skipping the component type change",
			testTier(), firstType)
	}

	// Rescale the cluster itself while the resource group exists. The request that changes the
	// cluster's components carries no resource group at all, so this guards against the
	// platform rebuilding the tenant's resource spec from it and dropping the resource groups
	// it did not mention.
	steps = append(steps, resource.TestStep{
		Config: config(2, currentType, 2),
		Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "spec.compute.default_node_group.replica", "2"),
			resource.TestCheckResourceAttr("risingwavecloud_cluster_resource_group.test", "component_type_id", currentType),
			resource.TestCheckResourceAttr("risingwavecloud_cluster_resource_group.test", "replica", "2"),
		),
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Delete testing automatically occurs in TestCase
		Steps: steps,
	})
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
