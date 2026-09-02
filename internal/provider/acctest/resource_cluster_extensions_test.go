package acctest

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testExtensionComponentType is the size the extensions' nodes run on. Like the rest of the
// values these tests hardcode, it has to exist in the target environment: check
// `GET <region mgmt url>/api/v1/tiers` for the component types a tier offers.
const testExtensionComponentType = "p-1c4g"

// TestClusterExtensionsResource covers the extensions of a cluster: enabling them, changing
// them, and removing them.
//
// The compactor is what to watch. Enabling serverless compaction makes the platform hold the
// cluster's compactor at zero, and restore it when the extension is removed. The declared count
// is left alone throughout -- the platform treats it as the value to come back to -- so every
// step asserts it is still one, and the framework's check that each step's plan is empty is
// what proves the two stay decoupled.
func TestClusterExtensionsResource(t *testing.T) {
	clusterName := fmt.Sprintf("tf%sext", getTestNamespace(t))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with no extensions, so the compactor is the cluster's own
			{
				Config: testClusterWithExtensions(clusterName, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					// defaulted by the provider when the configuration leaves it out
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "spec.compactor.default_node_group.replica", "1"),
				),
			},
			// Enable the extensions *and* change a component in the same plan. The rescale and the
			// enable travel to the platform through different endpoints, and the resource endpoint
			// acts on the extension too: carrying the planned concurrency there enabled the
			// extension early and the explicit enable that followed was refused with
			// `Illegal status: Running, cannot enable extensions compaction`.
			{
				Config: testClusterWithExtensions(clusterName, testExtensionsBlock(2, 1), withComputeReplica(2)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"extensions.serverless_compaction.maximum_compaction_concurrency", "2"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"extensions.serverless_compaction.status", "Running"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"extensions.serverless_backfill.replica", "1"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"extensions.serverless_backfill.status", "Running"),
					// resolved by the platform from the component type, not configured
					resource.TestCheckResourceAttrSet("risingwavecloud_cluster.test",
						"extensions.serverless_backfill.cpu"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"extensions.iceberg_compaction.status", "Running"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"extensions.iceberg_compaction.replica", "1"),
					resource.TestCheckResourceAttrSet("risingwavecloud_cluster.test",
						"extensions.iceberg_compaction.cpu"),
					// the extension holds the compactor at zero, but the declared count stands
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"spec.compactor.default_node_group.replica", "1"),
				),
			},
			// Resizing the compactor while the extension runs is refused while planning. The
			// platform records the count to restore when the extension is enabled and will not
			// revise it, so a new count would be kept by terraform and ignored by the platform.
			{
				Config:      testClusterWithExtensions(clusterName, testExtensionsBlock(2, 1), withCompactorReplica(2)),
				ExpectError: regexp.MustCompile("cannot be resized while serverless compaction is enabled"),
				PlanOnly:    true,
			},
			// Change a different component while the extensions are on. The compactor is at zero
			// replicas here, and restating it would be refused, so this only passes if the
			// provider sends the components that actually changed.
			{
				Config: testClusterWithExtensions(clusterName, testExtensionsBlock(2, 1), withComputeReplica(2)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"spec.compute.default_node_group.replica", "2"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"spec.compactor.default_node_group.replica", "1"),
				),
			},
			// Change both extensions at once
			{
				Config: testClusterWithExtensions(clusterName, testExtensionsBlock(4, 2), withComputeReplica(2)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"extensions.serverless_compaction.maximum_compaction_concurrency", "4"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"extensions.serverless_backfill.replica", "2"),
				),
			},
			// Remove them: the cluster stays and the platform gives the compactor back
			{
				Config: testClusterWithExtensions(clusterName, "", withComputeReplica(2)),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// testExtensionsBlock renders the `extensions` attribute.
//
// Iceberg compaction is enabled with a small configuration. The test has no Iceberg tables for
// it to compact, so this only shows that the extension comes up and reports itself running --
// which is what the provider is responsible for. It also puts `config` through a round trip: the
// platform parses it as TOML and stores the string, and if it rewrote it instead, the plan check
// after this step would catch the difference.
func testExtensionsBlock(concurrency, backfillReplica int) string {
	return fmt.Sprintf(`
	extensions = {
		serverless_compaction = {
			maximum_compaction_concurrency = %d
		}
		serverless_backfill = {
			component_type_id = %q
			replica           = %d
		}
		iceberg_compaction = {
			component_type_id = %q
			replica           = 1
			config            = "max_task_parallelism = 1\n"
		}
	}
`, concurrency, testExtensionComponentType, backfillReplica, testExtensionComponentType)
}

// testClusterWithExtensions is the cluster the extensions hang off. It declares one compactor,
// which is what the platform demands at creation -- a component with no replicas is rejected --
// and keeps saying one even after the extension takes it away.
// withComputeReplica changes how many compute nodes the rendered cluster asks for.
func withComputeReplica(n int) func(*clusterShape) { //nolint:unparam // only one size is needed today
	return func(c *clusterShape) { c.compute = n }
}

// withCompactorReplica changes how many compactor nodes it asks for.
func withCompactorReplica(n int) func(*clusterShape) {
	return func(c *clusterShape) { c.compactor = n }
}

// clusterShape is what the rendered cluster asks for, beyond the extensions.
type clusterShape struct {
	compute   int
	compactor int
}

func testClusterWithExtensions(name, extensions string, opts ...func(*clusterShape)) string {
	shape := clusterShape{compute: 1, compactor: 1}
	for _, o := range opts {
		o(&shape)
	}
	// The compactor is declared like any other component. Serverless compaction takes it away
	// while it runs and the platform gives it back, which is the extension's business rather
	// than a change to what the cluster was asked for.
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
				replica = %d
			}
		}
		compactor = {
			default_node_group = {
				cpu     = "1"
				memory  = "4 GB"
				replica = %d
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
%s}
`, testResourceGroupRegion, name, testResourceGroupVersion, shape.compute, shape.compactor, extensions)
}

// TestClusterStandaloneIgnoresExtensions covers the clusters that have nothing to do with this
// feature, which is where the risk of it lies: the platform refuses even to *report* the
// extensions of a standalone cluster, answering 412 rather than "disabled". Reading them
// unconditionally turned every plan of every standalone cluster into an error -- for users who
// had never enabled an extension and would have had no idea what to do about it.
//
// The second step is the point: it is a plan, not an apply, and it has to be empty.
func TestClusterStandaloneIgnoresExtensions(t *testing.T) {
	clusterName := fmt.Sprintf("tf%ssa", getTestNamespace(t))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testStandaloneCluster(clusterName),
			},
			{
				Config:   testStandaloneCluster(clusterName),
				PlanOnly: true,
			},
		},
	})
}

// TestClusterStandaloneRejectsExtensions covers the other half: asking for an extension on a
// standalone cluster is refused while planning, with a message that says why, rather than
// halfway through an apply with the platform's status code.
func TestClusterStandaloneRejectsExtensions(t *testing.T) {
	clusterName := fmt.Sprintf("tf%ssax", getTestNamespace(t))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testStandaloneClusterWithExtension(clusterName),
				ExpectError: regexp.MustCompile("not available on a standalone cluster"),
				PlanOnly:    true,
			},
		},
	})
}

// testStandaloneClusterWithExtension is the combination the platform cannot honour.
func testStandaloneClusterWithExtension(name string) string {
	cluster := testStandaloneCluster(name)
	closing := "}\n"
	return cluster[:len(cluster)-len(closing)] + `
	extensions = {
		serverless_compaction = {
			maximum_compaction_concurrency = 2
		}
	}
` + closing
}

func testStandaloneCluster(name string) string {
	return fmt.Sprintf(`
resource "risingwavecloud_cluster" "test" {
	region  = "%s"
	name    = "%s"
	version = "%s"
	tier    = "Standard"
	spec = {
		standalone = {
			default_node_group = {
				cpu     = "2"
				memory  = "8 GB"
				replica = 1
			}
		}
	}
}
`, testResourceGroupRegion, name, testResourceGroupVersion)
}
