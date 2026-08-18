package acctest

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// Write-only arguments need Terraform 1.11, while CI still runs the acceptance tests against
// 1.5 to 1.7, so this lives in its own test case with a version gate instead of as extra steps
// on the main one.
func TestClusterUserWriteOnlyPassword(t *testing.T) {
	clusterName := fmt.Sprintf("tf%swopw", getTestNamespace(t))
	spec := testClusterSpec(t, initCloudSDK(t))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			// Create with a write-only password
			{
				Config: spec.terraform(clusterName, 1) + testClusterUserWriteOnly("first-password", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("risingwavecloud_cluster_user.wo", "id"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster_user.wo", "username", "wo-user"),
					resource.TestCheckResourceAttr("risingwavecloud_cluster_user.wo", "password_wo_version", "1"),
					// the point of the feature: neither the password nor its write-only twin is
					// recorded, and the version is all that remains in the state.
					resource.TestCheckNoResourceAttr("risingwavecloud_cluster_user.wo", "password"),
					resource.TestCheckNoResourceAttr("risingwavecloud_cluster_user.wo", "password_wo"),
				),
			},
			// Rotating the password means changing the value and its version together
			{
				Config: spec.terraform(clusterName, 1) + testClusterUserWriteOnly("second-password", 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster_user.wo", "password_wo_version", "2"),
					resource.TestCheckNoResourceAttr("risingwavecloud_cluster_user.wo", "password_wo"),
				),
			},
			// A new value without a new version is deliberately ignored: terraform cannot see
			// the change, so there is nothing to plan and the step must stay empty.
			{
				Config:   spec.terraform(clusterName, 1) + testClusterUserWriteOnly("third-password", 2),
				PlanOnly: true,
			},
		},
	})
}

func testClusterUserWriteOnly(password string, version int) string {
	return fmt.Sprintf(`
resource "risingwavecloud_cluster_user" "wo" {
	cluster_id          = risingwavecloud_cluster.test.id
	username            = "wo-user"
	password_wo         = "%s"
	password_wo_version = %d
}
`, password, version)
}
