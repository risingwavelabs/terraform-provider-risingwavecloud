package acc

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/fake"
	"github.com/stretchr/testify/require"
)

func getTestNamespace(t *testing.T) string {
	t.Helper()

	r, err := regexp.Compile("[^a-zA-Z0-9]")
	require.NoError(t, err)

	return r.ReplaceAllString(os.Getenv("TEST_NAMESPACE"), "_")
}

func TestClusterResource(t *testing.T) {

	clusterName := fmt.Sprintf("tf-test%s", getTestNamespace(t))

	var id string
	var userID string
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
						nsID, err := fake.GetFakerState().GetNsIDByRegionAndName("us-east-1", clusterName)
						if err != nil {
							return err
						}
						id = nsID.String()
						userID = fmt.Sprintf("%s.test-user", id)
						return nil
					},
				),
			},
			// ImportState testing
			{
				Config:            testClusterResourceConfig("v1.5.0", clusterName),
				ResourceName:      "risingwavecloud_cluster.test",
				ImportStateId:     id,
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
					resource.TestCheckResourceAttr("risingwavecloud_cluster_user.test", "create_db", "false"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster_user.test", "super_user", "false"),
				),
			},
			// import user
			{
				Config:        testClusterResourceUpdateConfig(clusterName) + testClusterUser("test-password"),
				ResourceName:  "risingwavecloud_cluster_user.test",
				ImportStateId: userID,
				ImportState:   true,
			},
			// update user
			{
				Config: testClusterResourceUpdateConfig(clusterName) + testClusterUser("new-password"),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testClusterResourceConfig(version, name string) string {
	return fmt.Sprintf(`
resource "risingwavecloud_cluster" "test" {
	region   = "us-east-1"
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
`, name, version)
}

// update: compactor replica 1 -> 2, etcd_config, risingwave_config
func testClusterResourceUpdateConfig(name string) string {
	return fmt.Sprintf(`
resource "risingwavecloud_cluster" "test" {
	region   = "us-east-1"
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
`, name)
}

func testClusterUser(password string) string {
	return fmt.Sprintf(`
resource "risingwavecloud_cluster_user" "test" {
	cluster_id = risingwavecloud_cluster.test.id
	username   = "test-user"
	password   = "%s"
}	
`, password)
}
