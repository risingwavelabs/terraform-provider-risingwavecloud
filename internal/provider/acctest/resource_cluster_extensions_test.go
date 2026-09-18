package acctest

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	"github.com/stretchr/testify/require"
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
	t.Parallel()

	clusterName := fmt.Sprintf("tf%sext", getTestNamespace(t))
	spec := testClusterSpec(t, initCloudSDK(t))

	// Extensions run their own nodes, which a standalone cluster has no compute component for;
	// the platform refuses every one of them there.
	if spec.IsStandalone() {
		t.Skipf("tier %s in %s runs a standalone cluster, which cannot have extensions",
			testTier(), testRegion())
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with no extensions, so the compactor is the cluster's own
			{
				Config: testClusterWithExtensions(spec, clusterName, ""),
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
				Config: testClusterWithExtensions(spec, clusterName, testExtensionsBlock(2, 1), withComputeReplica(2)),
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
			// Changing the compactor while the extension runs is refused while planning, whether
			// the change is the count or the size: the platform records the whole component when
			// the extension is enabled and will not revise it, so either would be kept by
			// terraform and ignored by the platform.
			{
				Config:      testClusterWithExtensions(spec, clusterName, testExtensionsBlock(2, 1), withCompactorReplica(2)),
				ExpectError: regexp.MustCompile("cannot be changed while serverless compaction is enabled"),
				PlanOnly:    true,
			},
			{
				Config:      testClusterWithExtensions(spec, clusterName, testExtensionsBlock(2, 1), withCompactorSize("2", "8 GB")),
				ExpectError: regexp.MustCompile("cannot be changed while serverless compaction is enabled"),
				PlanOnly:    true,
			},
			// Change a different component while the extensions are on. The compactor is at zero
			// replicas here, and restating it would be refused, so this only passes if the
			// provider sends the components that actually changed.
			{
				Config: testClusterWithExtensions(spec, clusterName, testExtensionsBlock(2, 1), withComputeReplica(2)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"spec.compute.default_node_group.replica", "2"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"spec.compactor.default_node_group.replica", "1"),
				),
			},
			// Change both extensions at once, and give iceberg a config on the way. The platform
			// parses it as TOML and stores the string; if it rewrote it instead, the plan check
			// after this step would catch the difference.
			{
				Config: testClusterWithExtensions(spec, clusterName,
					testExtensionsBlock(4, 2, withIcebergConfig("max_task_parallelism = 1\n")),
					withComputeReplica(2)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"extensions.serverless_compaction.maximum_compaction_concurrency", "4"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"extensions.serverless_backfill.replica", "2"),
				),
			},
			// An explicitly empty config is a value, not an absence. The platform reports the
			// empty string for both, so this and the step above -- which omits the config
			// entirely -- have to end differently: `""` here, null there.
			{
				Config: testClusterWithExtensions(spec, clusterName,
					testExtensionsBlock(4, 2, withIcebergConfig("")), withComputeReplica(2)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test",
						"extensions.iceberg_compaction.config", ""),
				),
			},
			// Clear the last extension by emptying the block rather than deleting it. Terraform
			// tells a known object with null children apart from no object at all, and an apply
			// has to end with the shape the plan had, so this is not the same as the step below.
			{
				Config: testClusterWithExtensions(spec, clusterName, "\textensions = {}\n", withComputeReplica(2)),
			},
			// Remove them: the cluster stays and the platform gives the compactor back
			{
				Config: testClusterWithExtensions(spec, clusterName, "", withComputeReplica(2)),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// testExtensionsBlock renders the `extensions` attribute.
//
// Iceberg compaction is enabled without a `config` unless one is asked for. The test has no
// Iceberg tables for it to compact, so this only shows that the extension comes up and reports
// itself running -- which is what the provider is responsible for. Leaving the config out is the
// case worth covering by default: the platform stores an empty string for a request that omits
// one and always answers with a pointer, so recording that against a configuration which said
// nothing would end the apply with an inconsistent result. `withIcebergConfig` covers the other
// half, a config that makes the round trip and is compared verbatim.
func testExtensionsBlock(concurrency, backfillReplica int, opts ...func(*string)) string {
	// No config by default: the platform stores an empty string for a request that omits one and
	// always answers with a pointer, so an absent config is the case that has to read back as
	// absent rather than as "".
	config := ""
	for _, o := range opts {
		o(&config)
	}
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
%s		}
	}
`, concurrency, testExtensionComponentType, backfillReplica, testExtensionComponentType, config)
}

// withIcebergConfig gives the iceberg extension a configuration, which the platform parses as
// TOML and stores verbatim.
func withIcebergConfig(toml string) func(*string) {
	return func(c *string) { *c = fmt.Sprintf("\t\t\tconfig = %q\n", toml) }
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

// withCompactorSize changes the compactor's node size, which the extension takes over just as
// completely as it does the count.
func withCompactorSize(cpu, memory string) func(*clusterShape) {
	return func(c *clusterShape) { c.compactorCPU, c.compactorMemory = cpu, memory }
}

// clusterShape is what the rendered cluster asks for, beyond the extensions. An empty size
// means the tier's own, which is what every step but the one about resizing wants.
type clusterShape struct {
	compute         int
	compactor       int
	compactorCPU    string
	compactorMemory string
}

func testClusterWithExtensions(spec clusterSpec, name, extensions string, opts ...func(*clusterShape)) string {
	shape := clusterShape{compute: 1, compactor: 1}
	for _, o := range opts {
		o(&shape)
	}

	// The compactor is declared like any other component. Serverless compaction takes it away
	// while it runs and the platform gives it back, which is the extension's business rather
	// than a change to what the cluster was asked for.
	options := clusterOptions{
		Name:             name,
		ComputeReplica:   shape.compute,
		CompactorReplica: shape.compactor,
		Extensions:       extensions,
	}
	if shape.compactorCPU != "" {
		options.CompactorNode = &nodeSpec{CPU: shape.compactorCPU, Memory: shape.compactorMemory}
	}
	return spec.render(options)
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
	spec := standaloneSpec(t, initCloudSDK(t))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: spec.render(clusterOptions{Name: clusterName}),
			},
			{
				Config:   spec.render(clusterOptions{Name: clusterName}),
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
	spec := standaloneSpec(t, initCloudSDK(t))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: spec.render(clusterOptions{
					Name: clusterName,
					Extensions: "\textensions = {\n\t\tserverless_compaction = {\n" +
						"\t\t\tmaximum_compaction_concurrency = 2\n\t\t}\n\t}\n",
				}),
				ExpectError: regexp.MustCompile("not available on a standalone cluster"),
				PlanOnly:    true,
			},
		},
	})
}

// standaloneSpec reads the sizes of a tier that runs a standalone cluster. These two tests are
// about that shape rather than about the configured tier, so they ask for it by name; a tier
// that turns out not to be standalone in the target environment makes them skip rather than
// assert something else.
func standaloneSpec(t *testing.T, cloud cloudsdk.CloudClientInterface) clusterSpec {
	t.Helper()

	spec, summary, err := resolveClusterSpecOfTier(cloud, standaloneTestTier)
	require.NoErrorf(t, err, "cannot read the shape of tier %s in %s", standaloneTestTier, testRegion())
	t.Log(summary)

	if !spec.IsStandalone() {
		t.Skipf("tier %s in %s does not run a standalone cluster", standaloneTestTier, testRegion())
	}
	return spec
}

// standaloneTestTier is the tier these tests expect to be standalone.
const standaloneTestTier = "Standard"
