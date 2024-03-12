# terraform-provider-risingwavecloud

This is the official repository of the terraform provider for [RisingWave Cloud](https://cloud.risingwave.com/). It is used to manage resources in RisingWave Cloud in a declarative way. More information about Terrafom can be found [here](https://www.terraform.io/).

Documentation: *TODO*

## Example Usage


### Create RisingWave Clusters

1. Follow the *instruction TODO* to get the API Key and Secret from the RisingWave Cloud console.

2. Add the provider to your Terraform configuration.

    ```hcl
    provider "risingwavecloud" {
      api_key = ""
      api_secret = ""
    }
    ```

3. Get the list of all available components:

    ```hcl
    data "risingwavecloud_component_type" "compute" {
      platform   = "aws"
      region     = "us-east-1"
      vcpu       = 2
      memory_gib = 8
      component  = "compute"
    }
    ```

4. Create a RisingWave cluster with the component:

    ```hcl
    resource "risingwavecloud_cluster" "test" {
      region   = "us-east-1"
      name     = "tf-test"
      version  = "%s"
      spec     = {
        compute = {
          resource = {
            id      = data.risingwavecloud_component_type.compute.id
            replica = 1
          }
        }
        compactor = {
          resource = {
            id      = data.risingwavecloud_component_type.compactor.id
            replica = 2
          }
        }
        frontend = {
          resource = {
            id      = data.risingwavecloud_component_type.frontend.id
            replica = 1
          }
        }
        meta = {
          resource = {
            id      = data.risingwavecloud_component_type.meta.id
            replica = 1
          }
          etcd_meta_store = {
            resource = {
              id      = data.risingwavecloud_component_type.etcd.id
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
    ```

### Import an existing RisingWave Cluster

*TODO*


## Development

### Terraform Plugin Framework

_This template repository is built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework). The template repository built on the [Terraform Plugin SDK](https://github.com/hashicorp/terraform-plugin-sdk) can be found at [terraform-provider-scaffolding](https://github.com/hashicorp/terraform-provider-scaffolding). See [Which SDK Should I Use?](https://developer.hashicorp.com/terraform/plugin/framework-benefits) in the Terraform documentation for additional information.*

This repository is a *template* for a [Terraform](https://www.terraform.io) provider. It is intended as a starting point for creating Terraform providers, containing:

- A resource and a data source (`internal/provider/`),
- Examples (`examples/`) and generated documentation (`docs/`),
- Miscellaneous meta files.

These files contain boilerplate code that you will need to edit to create your own Terraform provider. Tutorials for creating Terraform providers can be found on the [HashiCorp Developer](https://developer.hashicorp.com/terraform/tutorials/providers-plugin-framework) platform. *Terraform Plugin Framework specific guides are titled accordingly.*

Please see the [GitHub template repository documentation](https://help.github.com/en/github/creating-cloning-and-archiving-repositories/creating-a-repository-from-a-template) for how to create a new repository from this template on GitHub.

Once you've written your provider, you'll want to [publish it on the Terraform Registry](https://developer.hashicorp.com/terraform/registry/providers/publishing) so that others can use it.

### Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up to date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```shell
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

### Developing the Provider

1. Generate code.
  
    a. Generate API client code from the OpenAPI spec:

      ```shell
      make gen-spec
      ```
    
    b. Generate mock client code:

      ```shell
      make gen-mock
      ```

    *Note: The `make codegen` command runs both `gen-spec` and `gen-mock`.*

2. Generate or update documentation:

    ```shell
    go generate
    ```

3. Run acceptance tests:

    ```shell
    make testacc
    ```
    Note: Acceptance tests create real resources, and often cost money to run.

    You can also run with a stateful mocking cloud client to test the provider with the acceptance test suites. This is only used for ease of development. The acceptance test above should be used to verify the provider's functionality before releasing.

    ```shell
    make mockacc
    ```
