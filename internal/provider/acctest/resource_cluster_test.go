package acctest

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/fake"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/provider"
	"github.com/stretchr/testify/require"
)

func getTestNamespace(t *testing.T) string {
	t.Helper()

	r, err := regexp.Compile("[^a-zA-Z0-9]")
	require.NoError(t, err)

	return r.ReplaceAllString(os.Getenv("TEST_NAMESPACE"), "-")
}

func getPrivateLinkTarget(t *testing.T) string {
	t.Helper()

	target := os.Getenv("TEST_PRIVATE_LINK_TARGET")
	require.NotEmpty(t, target, "TEST_PRIVATE_LINK_TARGET must be set")
	return target
}

func initCloudSDK(t *testing.T) cloudsdk.CloudClientInterface {
	t.Helper()

	if fake.UseFakeBackend() {
		return fake.NewCloudClient()
	}
	endpoint := os.Getenv(provider.EnvNameEndpoint)
	require.NotEmpty(t, endpoint)

	apiKey := os.Getenv(provider.EnvNameAPIKey)
	require.NotEmpty(t, apiKey)

	apiSecret := os.Getenv(provider.EnvNameAPISecret)
	require.NotEmpty(t, apiSecret)

	client, err := cloudsdk.NewCloudClient(context.Background(), endpoint, apiKey, apiSecret, "acctest")
	require.NoError(t, err)

	return client
}

func TestClusterResource_Standard(t *testing.T) {
	clusterName := fmt.Sprintf("tf%sacc", getTestNamespace(t))
	cloud := initCloudSDK(t)
	spec := testClusterSpec(t, cloud)

	privateLinkTarget := getPrivateLinkTarget(t)

	var clusterID uuid.UUID

	// The upgrade steps need a version to start from, which the environment cannot be asked
	// for. Without one the cluster is created with no version at all and the platform picks.
	upgradeFrom, upgradeTo, canUpgrade := testUpgradeVersions()
	if !canUpgrade {
		t.Logf("TEST_VERSION_FROM and TEST_VERSION_TO are not both set, skipping the version upgrade")
	}

	config := func(o clusterOptions) string {
		o.Name = clusterName
		return spec.render(o)
	}
	// the version every step after the upgrade runs at
	settled := clusterOptions{Version: upgradeTo}
	if !canUpgrade {
		settled = clusterOptions{}
	}

	// update: compactor replica 1 -> 2 and a risingwave_config. A standalone tier has no
	// compactor to scale, so only the configuration changes there.
	updated := settled
	updated.CompactorReplica = 2
	updated.RisingWaveConfig = "[server]\nheartbeat_interval_ms = 997\n"

	initial := settled
	if canUpgrade {
		initial = clusterOptions{Version: upgradeFrom}
	}

	versionCheck := func(want string) resource.TestCheckFunc {
		if want == "" {
			// chosen by the platform, so only its presence can be asserted
			return resource.TestCheckResourceAttrSet("risingwavecloud_cluster.test", "version")
		}
		return resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "version", want)
	}

	steps := []resource.TestStep{
		// Create and Read testing
		{
			Config: config(initial),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("risingwavecloud_cluster.test", "id"),
				resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "tier", string(testTier())),
				versionCheck(initial.Version),
				func(s *terraform.State) error {
					cluster, err := cloud.GetClusterByRegionAndName(context.Background(), testRegion(), clusterName)
					if err != nil {
						return err
					}
					clusterID = cluster.NsId
					return nil
				},
			),
		},
		// ImportState testing
		{
			Config:       config(initial),
			ResourceName: "risingwavecloud_cluster.test",
			ImportStateIdFunc: func(s *terraform.State) (string, error) {
				return clusterID.String(), nil
			},
			ImportState:       true,
			ImportStateVerify: true,
		},
	}

	// Update and Read: version
	if canUpgrade {
		steps = append(steps, resource.TestStep{
			Config: config(settled),
			Check:  resource.ComposeAggregateTestCheckFunc(versionCheck(upgradeTo)),
		})
	}

	// Update and Read: compactor replica, risingwave_config
	updateChecks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "spec.risingwave_config", updated.RisingWaveConfig),
	}
	if !spec.IsStandalone() {
		updateChecks = append(updateChecks,
			resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "spec.compactor.default_node_group.replica", "2"))
	}
	steps = append(steps, resource.TestStep{
		Config: config(updated),
		Check:  resource.ComposeAggregateTestCheckFunc(updateChecks...),
	})

	steps = append(steps,
		// Create and Read testing: user
		resource.TestStep{
			Config: config(updated) + testClusterUser("test-password"),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("risingwavecloud_cluster_user.test", "id"),
				resource.TestCheckResourceAttr("risingwavecloud_cluster_user.test", "username", "test-user"),
				resource.TestCheckResourceAttr("risingwavecloud_cluster_user.test", "password", "test-password"),
				resource.TestCheckResourceAttr("risingwavecloud_cluster_user.test", "create_db", "true"),
				resource.TestCheckResourceAttr("risingwavecloud_cluster_user.test", "super_user", "true"),
				resource.TestCheckResourceAttr("risingwavecloud_cluster_user.test", "create_user", "true"),
				// reported by the platform, never configured: CREATE USER implies LOGIN.
				resource.TestCheckResourceAttr("risingwavecloud_cluster_user.test", "can_login", "true"),
			),
		},
		// import user
		resource.TestStep{
			Config:       config(updated) + testClusterUser("test-password"),
			ResourceName: "risingwavecloud_cluster_user.test",
			ImportStateIdFunc: func(s *terraform.State) (string, error) {
				return fmt.Sprintf("%s.test-user", clusterID.String()), nil
			},
			ImportState: true,
		},
		// update user
		resource.TestStep{
			Config: config(updated) + testClusterUser("new-password"),
		},
		// Create and read testing: private link
		resource.TestStep{
			Config: config(updated) + testPrivateLink(privateLinkTarget),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("risingwavecloud_privatelink.test", "id"),
				resource.TestCheckResourceAttrSet("risingwavecloud_privatelink.test", "endpoint"),
			),
		},
		// import private link
		resource.TestStep{
			Config:       config(updated) + testPrivateLink(privateLinkTarget),
			ResourceName: "risingwavecloud_privatelink.test",
			ImportStateIdFunc: func(s *terraform.State) (string, error) {
				pls, err := cloud.GetPrivateLinks(context.Background())
				if err != nil {
					return "", err
				}
				for _, pl := range pls {
					if pl.PrivateLink.ConnectionName == "test-connection" {
						return pl.PrivateLink.Id.String(), nil
					}
				}
				return "", fmt.Errorf("private link not found")
			},
			ImportState: true,
		},
		// Delete testing automatically occurs in TestCase
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

func testClusterUser(password string) string {
	return fmt.Sprintf(`
resource "risingwavecloud_cluster_user" "test" {
	cluster_id  = risingwavecloud_cluster.test.id
	username    = "test-user"
	password    = "%s"
	super_user  = true
	create_db   = true
	create_user = true
}
`, password)
}

func testPrivateLink(target string) string {
	return fmt.Sprintf(`
resource "risingwavecloud_privatelink" "test" {
	cluster_id = risingwavecloud_cluster.test.id
	connection_name = "test-connection"
	target = "%s"
}`, target)
}
