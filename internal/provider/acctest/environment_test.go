package acctest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen_mgmtv1 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v1"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/fake"
	"github.com/stretchr/testify/require"
)

// The acceptance tests are meant to run against whatever environment the credentials point
// at, so nothing about that environment is hardcoded:
//
//   - the region and the tier come from the environment
//   - the node sizes are asked of the tier, the way the rwc CLI's e2e test does, instead of
//     being copied into the configuration where they go stale
//   - the RisingWave version is never pinned. `version` is optional on the cluster resource
//     and the platform then picks the newest stable release
//
// Every component runs at the smallest size the tier offers, 0.5 RWU, and so does the resource
// group, because these tests exercise the provider rather than the cluster: nothing here needs
// a node that can do work.
const (
	defaultTestRegion = "us-east-1"
	defaultTestTier   = "Invited"

	// The fake accepts any version, so the upgrade step has a pair to move between there.
	fakeUpgradeFrom = "v2.0.5"
	fakeUpgradeTo   = "v2.1.2"

	// smallestNodeCPU is 0.5 RWU. Only the CPU is named, and the memory that the tier pairs
	// with it is taken as given, so this does not have to track how the platform sizes memory.
	smallestNodeCPU = "0.5"
)

func testRegion() string {
	if r := os.Getenv("TEST_REGION"); r != "" {
		return r
	}
	return defaultTestRegion
}

func testTier() apigen_mgmtv1.TierId {
	if t := os.Getenv("TEST_TIER"); t != "" {
		return apigen_mgmtv1.TierId(t)
	}
	return defaultTestTier
}

// nodeSpec is the part of a component type the cluster resource takes: it asks for a size,
// not for a component type ID, and resolves the ID itself.
type nodeSpec struct {
	ID     string
	CPU    string
	Memory string
}

// clusterSpec holds the smallest usable size of every component of a tier.
type clusterSpec struct {
	Compute   nodeSpec
	Compactor nodeSpec
	Frontend  nodeSpec
	Meta      nodeSpec

	// Standalone is set instead of the four components above when the tier runs everything in
	// a single node, which is how the `Standard` tier works. Resource groups are not available
	// on such a cluster: the platform rejects them for want of a compute component.
	Standalone   *nodeSpec
	ComputeTypes []apigen_mgmtv1.AvailableComponentType
}

// NextComputeTypeAfter returns the compute size the tier offers above the given one, which the
// resource group test moves to. A tier meant for testing may offer a single size, in which case
// there is nothing to move to.
func (s clusterSpec) NextComputeTypeAfter(id string) (string, bool) {
	for i, t := range s.ComputeTypes {
		if t.Id == id && i+1 < len(s.ComputeTypes) {
			return s.ComputeTypes[i+1].Id, true
		}
	}
	return "", false
}

// IsStandalone reports whether the tier runs a single node rather than separate components.
func (s clusterSpec) IsStandalone() bool { return s.Standalone != nil }

// testClusterSpec reads the sizes of the target tier. It fails the test rather than falling
// back to a guess, because a guess would fail later with a much less obvious message.
func testClusterSpec(t *testing.T, cloud cloudsdk.CloudClientInterface) clusterSpec {
	t.Helper()

	ctx := context.Background()
	region, tier := testRegion(), testTier()

	sizes := func(component string) []apigen_mgmtv1.AvailableComponentType {
		types, err := cloud.GetAvailableComponentTypes(ctx, region, tier, component)
		require.NoErrorf(t, err, "cannot list %s types of tier %s in %s", component, tier, region)
		return types
	}

	// The tier is asked for the 0.5 RWU size by name rather than for whatever happens to come
	// first, so the tests cost the same everywhere and a reordered list cannot silently make
	// them run on a larger node.
	smallest := func(component string) nodeSpec {
		types := sizes(component)
		require.NotEmptyf(t, types, "tier %s offers no %s component in %s", tier, component, region)
		for _, n := range types {
			if n.Cpu == smallestNodeCPU {
				return nodeSpec{ID: n.Id, CPU: n.Cpu, Memory: n.Memory}
			}
		}
		require.FailNowf(t, "no smallest node",
			"tier %s offers no %s CPU %s component in %s, only %v",
			tier, component, smallestNodeCPU, region, types)
		return nodeSpec{}
	}

	// The meta store is deliberately not requested. A tier that offers the shared postgres
	// instance gets it by default, and asking for it explicitly is rejected with
	// `400 don't allow specify SharingPg meta store in request` — the tiers endpoint lists
	// what a cluster can end up with, not what a request may name.
	var spec clusterSpec

	// A tier offers either separate components or a single standalone node, never both.
	if computeTypes := sizes(cloudsdk.ComponentCompute); len(computeTypes) > 0 {
		spec.ComputeTypes = computeTypes
		spec.Compute = smallest(cloudsdk.ComponentCompute)
		spec.Compactor = smallest(cloudsdk.ComponentCompactor)
		spec.Frontend = smallest(cloudsdk.ComponentFrontend)
		spec.Meta = smallest(cloudsdk.ComponentMeta)
		t.Logf("tier %s in %s: compute=%s (%s/%s), %d sizes offered",
			tier, region, spec.Compute.ID, spec.Compute.CPU, spec.Compute.Memory, len(computeTypes))
	} else {
		// A standalone node runs every component at once, so tiers do not offer it at 0.5 RWU.
		standaloneTypes := sizes(cloudsdk.ComponentStandalone)
		require.NotEmptyf(t, standaloneTypes, "tier %s offers no standalone component in %s", tier, region)
		first := standaloneTypes[0]
		standalone := nodeSpec{ID: first.Id, CPU: first.Cpu, Memory: first.Memory}
		spec.Standalone = &standalone
		t.Logf("tier %s in %s: standalone=%s (%s/%s)",
			tier, region, standalone.ID, standalone.CPU, standalone.Memory)
	}

	return spec
}

// testUpgradeVersions returns the pair of RisingWave versions the upgrade step moves between.
//
// The platform reports a single image tag -- the version it hands out -- and has no endpoint
// that lists the older ones it would still accept, so a version to upgrade *from* cannot be
// discovered. It therefore has to be named: setting both TEST_VERSION_FROM and TEST_VERSION_TO
// to versions the target environment accepts turns the upgrade step on, and without them the
// step is skipped. The fake accepts any version, so a mock run keeps covering the upgrade.
func testUpgradeVersions() (from, to string, ok bool) {
	if fake.UseFakeBackend() {
		return fakeUpgradeFrom, fakeUpgradeTo, true
	}
	from, to = os.Getenv("TEST_VERSION_FROM"), os.Getenv("TEST_VERSION_TO")
	return from, to, from != "" && to != ""
}

// clusterOptions is what the acceptance tests vary about the cluster they run on. Everything
// else -- region, tier, node sizes -- comes from the environment.
type clusterOptions struct {
	Name string

	// Version is left empty unless a test is about versions, so the platform picks the newest
	// stable release rather than the test pinning one that ages out of the environment.
	Version string

	ComputeReplica   int
	CompactorReplica int

	// RisingWaveConfig is the cluster's TOML configuration, rendered as a heredoc.
	RisingWaveConfig string
}

// render writes the cluster resource. A tier that runs a single standalone node has none of the
// separate components, so the replica counts do not apply there.
func (s clusterSpec) render(o clusterOptions) string {
	nodeGroup := func(component string, n nodeSpec, replica int) string {
		if replica < 1 {
			replica = 1
		}
		return fmt.Sprintf(`		%s = {
			default_node_group = {
				cpu     = %q
				memory  = %q
				replica = %d
			}
		}
`, component, n.CPU, n.Memory, replica)
	}

	var b strings.Builder
	b.WriteString("\nresource \"risingwavecloud_cluster\" \"test\" {\n")
	fmt.Fprintf(&b, "\tregion = %q\n", testRegion())
	fmt.Fprintf(&b, "\tname   = %q\n", o.Name)
	fmt.Fprintf(&b, "\ttier   = %q\n", string(testTier()))
	if o.Version != "" {
		fmt.Fprintf(&b, "\tversion = %q\n", o.Version)
	}
	b.WriteString("\tspec = {\n")

	if s.IsStandalone() {
		b.WriteString(nodeGroup("standalone", *s.Standalone, 1))
	} else {
		b.WriteString(nodeGroup("compute", s.Compute, o.ComputeReplica))
		b.WriteString(nodeGroup("compactor", s.Compactor, o.CompactorReplica))
		b.WriteString(nodeGroup("frontend", s.Frontend, 1))
		b.WriteString(nodeGroup("meta", s.Meta, 1))
	}

	// The meta store is deliberately not requested; see testClusterSpec.
	if o.RisingWaveConfig != "" {
		b.WriteString("\t\trisingwave_config = <<-EOT\n")
		for _, line := range strings.Split(strings.TrimRight(o.RisingWaveConfig, "\n"), "\n") {
			fmt.Fprintf(&b, "\t\t%s\n", line)
		}
		b.WriteString("\t\tEOT\n")
	}

	b.WriteString("\t}\n}\n")
	return b.String()
}

// terraform renders a cluster with nothing but the compute replica varied, which is all most
// tests need.
func (s clusterSpec) terraform(name string, computeReplica int) string {
	return s.render(clusterOptions{Name: name, ComputeReplica: computeReplica})
}
