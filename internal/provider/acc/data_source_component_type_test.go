// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestComponentTypeDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testComponentTypeDataSourceConfig("compute"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.risingwavecloud_component_type.test", "id", "p-2c8g"),
				),
			},
		},
	})
}

func testComponentTypeDataSourceConfig(component string) string {
	return fmt.Sprintf(`data "risingwavecloud_component_type" "test" {
	platform   = "aws"
	region     = "us-east-1"
	vcpu       = 2
	memory_gib = 8
	component  = "%s"
}`, component)
}
