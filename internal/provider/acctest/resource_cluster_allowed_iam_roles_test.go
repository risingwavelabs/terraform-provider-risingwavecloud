package acctest

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	roleArnA = "arn:aws:iam::123456789012:role/tf-acctest-a"
	roleArnB = "arn:aws:iam::123456789012:role/tf-acctest-b"
	roleArnC = "arn:aws:iam::123456789012:role/tf-acctest-c"
)

// TestClusterAllowedIamRolesResource covers the list a cluster owns: setting it, growing and
// shrinking it, importing it, and emptying it.
func TestClusterAllowedIamRolesResource(t *testing.T) {
	// Each test builds its own cluster, and the cluster is almost all of the runtime: about
	// thirteen of the twenty-odd minutes a test takes go into provisioning one and tearing it
	// down, because this tier's meta store is a dedicated RDS instance. Run them at the same
	// time and the suite costs the slowest test rather than the sum of all of them.
	t.Parallel()

	clusterName := fmt.Sprintf("tf%siam", getTestNamespace(t))
	spec := testClusterSpec(t, initCloudSDK(t))

	config := func(arns ...string) string {
		return spec.terraform(clusterName, 1) + testAllowedIamRoles(arns...)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with two principals
			{
				Config: config(roleArnA, roleArnB),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"risingwavecloud_cluster_allowed_iam_roles.test", "cluster_id",
						"risingwavecloud_cluster.test", "id",
					),
					resource.TestCheckResourceAttr("risingwavecloud_cluster_allowed_iam_roles.test", "role_arns.#", "2"),
					resource.TestCheckTypeSetElemAttr("risingwavecloud_cluster_allowed_iam_roles.test", "role_arns.*", roleArnA),
					resource.TestCheckTypeSetElemAttr("risingwavecloud_cluster_allowed_iam_roles.test", "role_arns.*", roleArnB),
				),
			},
			// Import: the cluster owns exactly one list, so its id is the cluster's
			{
				Config:       config(roleArnA, roleArnB),
				ResourceName: "risingwavecloud_cluster_allowed_iam_roles.test",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["risingwavecloud_cluster.test"].Primary.ID, nil
				},
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Replace one and add another: one removal and two additions in a single apply
			{
				Config: config(roleArnA, roleArnC),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster_allowed_iam_roles.test", "role_arns.#", "2"),
					resource.TestCheckTypeSetElemAttr("risingwavecloud_cluster_allowed_iam_roles.test", "role_arns.*", roleArnA),
					resource.TestCheckTypeSetElemAttr("risingwavecloud_cluster_allowed_iam_roles.test", "role_arns.*", roleArnC),
				),
			},
			// An empty list is a legitimate state: nobody may assume a role
			{
				Config: config(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("risingwavecloud_cluster_allowed_iam_roles.test", "role_arns.#", "0"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAllowedIamRoles(arns ...string) string {
	list := ""
	for _, arn := range arns {
		list += fmt.Sprintf("\t\t%q,\n", arn)
	}
	return fmt.Sprintf(`
resource "risingwavecloud_cluster_allowed_iam_roles" "test" {
	cluster_id = risingwavecloud_cluster.test.id
	role_arns = [
%s	]
}
`, list)
}
