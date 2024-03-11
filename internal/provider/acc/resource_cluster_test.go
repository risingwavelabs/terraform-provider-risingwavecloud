// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestClusterResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testClusterResourceConfig("v1.5.0"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("risingwavecloud_cluster.test", "id"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "version", "v1.5.0"),
				),
			},
			// ImportState testing
			{
				Config:            testClusterResourceConfig("v1.5.0"),
				ResourceName:      "risingwavecloud_cluster.test",
				ImportStateId:     "aws.us-east-1.tf-test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read: version
			{
				Config: testClusterResourceConfig("v1.6.0"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "version", "v1.6.0"),
				),
			},
			// Update and Read: compactor replica, risingwave_config, etcd_config
			{
				Config: testClusterResourceUpdateConfig("v1.6.0"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "spec.compactor.resource.replica", "2"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "spec.risingwave_config", "[server]\nheartbeat_interval_ms = 997\n"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster.test", "spec.meta.etcd_meta_store.etcd_config", "ETCD_MAX_REQUEST_BYTES: '100000000'\n"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testClusterResourceConfig(version string) string {
	return fmt.Sprintf(`
resource "risingwavecloud_cluster" "test" {
	platform = "aws"
	region   = "us-east-1"
	name     = "tf-test"
	version  = "%s"
	spec     = {
		compute = {
			resource = {
				id      = "p-2c8g"
				replica = 1
			}
		}
		compactor = {
			resource = {
				id      = "p-1c4g"
				replica = 1
			}
		}
		frontend = {
			resource = {
				id      = "p-1c4g"
				replica = 1
			}
		}
		meta = {
			resource = {
				id      = "p-1c4g"
				replica = 1
			}
			etcd_meta_store = {
				resource = {
					id      = "p-1c4g"
					replica = 1
				}
			}
		}
	}
}
`, version)
}

// update: compactor replica 1 -> 2, etcd_config, risingwave_config
func testClusterResourceUpdateConfig(version string) string {
	return fmt.Sprintf(`
resource "risingwavecloud_cluster" "test" {
	platform = "aws"
	region   = "us-east-1"
	name     = "tf-test"
	version  = "%s"
	spec     = {
		compute = {
			resource = {
				id      = "p-2c8g"
				replica = 1
			}
		}
		compactor = {
			resource = {
				id      = "p-1c4g"
				replica = 2
			}
		}
		frontend = {
			resource = {
				id      = "p-1c4g"
				replica = 1
			}
		}
		meta = {
			resource = {
				id      = "p-1c4g"
				replica = 1
			}
			etcd_meta_store = {
				resource = {
					id      = "p-1c4g"
					replica = 1
				}
				etcd_config = <<-EOT
				ETCD_MAX_REQUEST_BYTES: '100000000'
				EOT
			}
		}
		risingwave_config = <<-EOT
		[server]
		heartbeat_interval_ms = 997
		EOT
	}
}
`, version)
}
