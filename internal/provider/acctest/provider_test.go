package acctest

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/provider"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
// A fresh provider is built per call rather than one shared by every test. The provider holds
// only its version today, so sharing one happens to be safe, but that is a property of the
// current struct and not something a reader of this file can rely on: the moment a field is set
// in Configure, tests running in parallel would race on it.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"risingwavecloud": func() (tfprotov6.ProviderServer, error) {
		return providerserver.NewProtocol6WithError(provider.New("test")())()
	},
}

func testAccPreCheck(t *testing.T) {
	// You can add code here to run prior to any test case execution, for example assertions
	// about the appropriate environment variables being set are common to see in a pre-check
	// function.
}
