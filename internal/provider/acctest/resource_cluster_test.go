//go:build !ut
// +build !ut

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
	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/fake"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/provider"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils"
	"github.com/stretchr/testify/require"
)

func getTestNamespace(t *testing.T) string {
	t.Helper()

	r, err := regexp.Compile("[^a-zA-Z0-9-]")
	require.NoError(t, err)

	return r.ReplaceAllString(os.Getenv("TEST_NAMESPACE"), "-")
}

var region = utils.IfElse(len(os.Getenv("TEST_REGION")) != 0, os.Getenv("TEST_REGION"), "us-east-1")

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

func TestClusterResource(t *testing.T) {

	clusterName := fmt.Sprintf("tf-test%s", getTestNamespace(t))
	cloud := initCloudSDK(t)

	var clusterID uuid.UUID

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testClusterResourceConfig("v1.5.0", clusterName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("risingwavecloud_cluster.test", "id"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "tier", string(apigen_mgmt.Standard)),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "version", "v1.5.0"),
					func(s *terraform.State) error {
						cluster, err := cloud.GetClusterByRegionAndName(context.Background(), region, clusterName)
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
				Config:       testClusterResourceConfig("v1.5.0", clusterName),
				ResourceName: "risingwavecloud_cluster.test",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return clusterID.String(), nil
				},
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read: version
			{
				Config: testClusterResourceConfig("v1.6.0", clusterName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "version", "v1.6.0"),
				),
			},
			// Update and Read: compactor replica, risingwave_config, etcd_config
			{
				Config: testClusterResourceUpdateConfig(clusterName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "spec.compactor.default_node_group.replica", "2"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "spec.risingwave_config", "[server]\nheartbeat_interval_ms = 997\n"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "spec.meta.etcd_meta_store.etcd_config", "ETCD_MAX_REQUEST_BYTES: \"100000000\"\n"),
				),
			},
			// Create and Read testing: user
			{
				Config: testClusterResourceUpdateConfig(clusterName) + testClusterUser("test-password"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("risingwavecloud_cluster_user.test", "id"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster_user.test", "username", "test-user"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster_user.test", "password", "test-password"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster_user.test", "create_db", "true"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster_user.test", "super_user", "true"),
				),
			},
			// import user
			{
				Config:       testClusterResourceUpdateConfig(clusterName) + testClusterUser("test-password"),
				ResourceName: "risingwavecloud_cluster_user.test",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return fmt.Sprintf("%s.test-user", clusterID.String()), nil
				},
				ImportState: true,
			},
			// update user
			{
				Config: testClusterResourceUpdateConfig(clusterName) + testClusterUser("new-password"),
			},
			// Create and read testing: private link
			{
				Config: testClusterResourceUpdateConfig(clusterName) + testPrivateLink(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("risingwavecloud_privatelink.test", "id"),
					resource.TestCheckResourceAttrSet("risingwavecloud_privatelink.test", "endpoint"),
				),
			},
			// import private link
			{
				Config:       testClusterResourceUpdateConfig(clusterName) + testPrivateLink(),
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
		},
	})
}

func testClusterResourceConfig(version, name string) string {
	return fmt.Sprintf(`
resource "risingwavecloud_cluster" "test" {
	region   = "%s"
	name     = "%s"
	version  = "%s"
	spec     = {
		compute = {
			default_node_group = {
				cpu     = "2"
				memory  = "8 GB"
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
			etcd_meta_store = {
				default_node_group = {
					cpu     = "1"
					memory  = "4 GB"
					replica = 1
				}
			}
		}
	}
}
`, region, name, version)
}

// update: compactor replica 1 -> 2, etcd_config, risingwave_config
func testClusterResourceUpdateConfig(name string) string {
	return fmt.Sprintf(`
resource "risingwavecloud_cluster" "test" {
	region   = "%s"
	name     = "%s"
	version  = "v1.6.0"
	spec     = {
		compute = {
			default_node_group = {
				cpu     = "2"
				memory  = "8 GB"
				replica = 1
			}
		}
		compactor = {
			default_node_group = {
				cpu     = "1"
				memory  = "4 GB"
				replica = 2
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
			etcd_meta_store = {
				default_node_group = {
					cpu     = "1"
					memory  = "4 GB"
					replica = 1
				}
				etcd_config = <<-EOT
				ETCD_MAX_REQUEST_BYTES: "100000000"
				EOT
			}
		}
		risingwave_config = <<-EOT
		[server]
		heartbeat_interval_ms = 997
		EOT
	}
}
`, region, name)
}

func testClusterUser(password string) string {
	return fmt.Sprintf(`
resource "risingwavecloud_cluster_user" "test" {
	cluster_id = risingwavecloud_cluster.test.id
	username   = "test-user"
	password   = "%s"
	super_user = true
	create_db  = true
}	
`, password)
}

func testPrivateLink() string {
	return `
resource "risingwavecloud_privatelink" "test" {
	cluster_id = risingwavecloud_cluster.test.id
	connection_name = "test-connection"
	target = "test-target"
}`
}
