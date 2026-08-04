package provider

var providerMarkdownDescription = `
The Terraform plugin for [RisingWave Cloud](https://cloud.risingwave.com/) allows you to manage your resources 
on the RisingWave Cloud platform with Terraform. Please join our 
[Slack](https://join.slack.com/t/risingwave-community/shared_invite/zt-1jei7dk79-fguGadPI2KnhtWnnxBVGoA) to get the latest information.


## Authentication
Before using the provider, you need to create an API key and API secret at the RisingWave Cloud portal.
Please check the 
[documentation](https://docs.risingwave.com/cloud/service-account) for more information.

Note that you can also use environment variables to set the API key and API secret:
` + "```hcl" + `
RWC_API_KEY=myapikeyvalue
RWC_API_SECRET=myapisecretvalue
` + "```" + `
This allows you to manage your credentials in a more secure way.


## Quick Start

` + "```hcl" + `
# Install Terraform provider for RisingWave Cloud
terraform {
	required_providers {
	  risingwavecloud = {
		  source = "risingwavelabs/risingwavecloud"
		  version = <provider version>
	  }
	}
}

# Configure the RisingWave Cloud provider
provider "risingwavecloud" {
	api_key    = <API Key>    # or use RWC_API_KEY environment variable
	api_secret = <API Secret> # or use RWC_API_SECRET environment variable
}

# Create a RisingWave Cluster
resource "risingwavecloud_cluster" "mycluster" {
  name    = "mycluster"
  version = "v2.1.2"
  region  = "us-east-1"
  spec = {
    risingwave_config = ""
    compute = {
      default_node_group = {
        cpu    = "0.5"
        memory = "2 GB"
      }
    }
    compactor = {
      default_node_group = {
        cpu    = "1"
        memory = "4 GB"
      }
    }
    meta = {
      default_node_group = {
        cpu    = "0.5"
        memory = "2 GB"
      }
    }
    frontend = {
      default_node_group = {
        cpu    = "0.5"
        memory = "2 GB"
      }
    }
  }
}  
` + "```" + `


## Import Resources
You can import existing resources into Terraform using the ` + "`" + `terraform import` + "`" + ` command. 

To import a resource, you need to know the resource ID to let the provider know which resource to fetch from 
the RisingWave Cloud platform. Read the documentation of each resource to know how to get its ID.

For more details about this command, check the [Terraform documentation](https://developer.hashicorp.com/terraform/cli/import).


## Feature Requests
Please join our 
[Slack](https://join.slack.com/t/risingwave-community/shared_invite/zt-1jei7dk79-fguGadPI2KnhtWnnxBVGoA) to request new features.


## Reporting Issues
Please report any issues at the [GitHub repository](https://github.com/risingwavelabs/terraform-provider-risingwavecloud).
`

var clusterMarkdownDescription = `
A managed RisingWave Cluster on the RisingWave Cloud platform.

## Import a RisingWave Cluster

To import a RisingWave cluster, follow the steps below:

1. Get the UUID of the cluster from the RisingWave Cloud platform.

2. Write a resource definition to import the cluster. For example:

` + "  ```hcl" + `
  resource "risingwavecloud_cluster" "mycluster" {
    region  = "us-east-1"
    name    = "mycluster"
    version = "v1.8.0"
    spec = {
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
      }
    }
  }

` + "  ```" + `

Note that 1 RWU is equivalent to 1 vCPU and 4 GB of memory.

3. Run the import command:

` + "  ```shell" + `
  terraform import risingwavecloud_cluster.mycluster <cluster_id>
` + "  ```" + `
`

var privateLinkMarkdownDescription = `
A Private Link connection on the RisingWave Cloud platform.

In AWS, it is a configured endpoint to connect to a VPC endpoint service in your VPC.

In GCP, it is a endpoint to a service attachment in your private network.

Learn more details about this resource at [RisingWave Cloud Documentation](https://docs.risingwave.com/cloud/create-a-connection/).

## Import a Privatelink Resource

To import a Privatelink resource, follow the steps below:

1. Get the UUID of the privatelink from the RisingWave Cloud platform.

2. Write a resource definition to import the cluster. For example:

` + "```hcl" + `
  resource "risingwavecloud_privatelink" "test" {
    depends_on = [risingwavecloud_cluster.mycluster]

    cluster_id      = "cluster-id"
    connection_name = "test-connection"
    target          = "test-target"
  }
  ` + "```" + `

  ~> **Note:** When destroying all resources, make sure the Terraform be aware of the dependency between the cluster and the 
  private link resource. If the cluster is deleted before the private link resource, the deletion of the private link resource 
  will fail. You can either use the ` + "`" + `depends_on` + "`" + ` argument or use the output of the cluster to create the 
  private link resource.

3. Run the import command:

` + "```shell" + `
terraform import risingwavecloud_privatelink.test <privatelink_id>
` + "```" + `
`

var clusterResourceGroupMarkdownDescription = `
An additional (non-default) resource group in a RisingWave cluster. A resource group is a set of
compute nodes of its own, used to isolate streaming workloads from each other.

This resource only manages the compute capacity. Assigning a database to a resource group is done in
SQL when the database is created, and the databases themselves are not managed by this provider.

!> **Not every cluster can host a resource group.** The cluster must have a separate compute
component, which means the ` + "`" + `Invited` + "`" + ` or ` + "`" + `BYOC` + "`" + ` tier: a ` + "`" + `Standard` + "`" + ` tier cluster runs
standalone and the platform rejects resource groups on it. Note that ` + "`" + `Standard` + "`" + ` is the default
tier of ` + "`" + `risingwavecloud_cluster` + "`" + ` for SaaS clusters. The cluster also has to run RisingWave
` + "`" + `v2.3.0` + "`" + ` or later. Neither can be checked while planning, since the cluster may not exist yet, so
a mismatch surfaces as an error during apply.

~> **Note:** The ` + "`" + `default` + "`" + ` resource group is managed by the ` + "`" + `risingwavecloud_cluster` + "`" + ` resource
and cannot be created or imported here. Use this resource only for additional named resource groups,
and the cluster's ` + "`" + `spec` + "`" + ` to rescale the default one. Names starting with ` + "`" + `backfill` + "`" + ` are
reserved by the platform for its serverless backfill extension and are rejected as well.

Creating, rescaling, and deleting a resource group triggers a cluster rescale, so these operations
are asynchronous and Terraform waits for the cluster to become healthy again. A cluster can only run
one rescale at a time: the provider serializes the resource group changes of a cluster, so applying
several of them takes as long as the sum of their rescales.

## Import a Resource Group

To import a resource group, follow the steps below:

1. Get the UUID of the corresponding cluster from the RisingWave Cloud platform.

2. Write a resource definition to import the resource group. For example:

` + "```hcl" + `
  resource "risingwavecloud_cluster_resource_group" "streaming" {
    cluster_id        = risingwavecloud_cluster.mycluster.id
    name              = "streaming-rg"
    component_type_id = "p-1c4g"
    replica           = 1
  }
  ` + "```" + `

  ~> **Note:** Reference the cluster instead of hardcoding its ID, as shown above. This is how
  Terraform learns that the resource group has to be created after the cluster and deleted before it.
  Deleting a resource group fails while databases still run in it: drop those databases first.

3. Run the import command:

` + "```shell" + `
terraform import risingwavecloud_cluster_resource_group.test <cluster_id>.<resource_group_name>
` + "```" + `
`

var clusterUserMarkdownDescription = `
A database user in a RisingWave cluster. The username and password of the dabase user are used to
connect to the RisingWave cluster.

~> **Note:** Username and password will be stored in the state file in plain text.
[Read more about sensitive data in state](https://www.terraform.io/docs/state/sensitive-data.html).

## Import a Cluster User

To import a cluster user, follow the steps below:

1. Get the UUID of the corrsponding cluster from the RisingWave Cloud platform.

2. Write a resource definition to import the cluster user. For example:

` + "```hcl" + `
  resource "risingwavecloud_cluster_user" "test" {
    depends_on = [risingwavecloud_cluster.mycluster]

    cluster_id = "cluster-id"
    username   = "test-user"
    password   = "test-password"
  } 
  ` + "```" + `

  ~> **Note:** The password is stored in the state for comparing changes. RisingWave Cloud platform
  does not store the password after the user is created. If you change the password outside of Terraform,
  the new password won't be reflected in the state file.

  ~> **Note:** When destroying all resources, make sure the Terraform be aware of the dependency between the cluster and the user.
  If the cluster is deleted before the user, the deletion of the user will fail. You can either use the ` + "`" + `depends_on` + "`" + `
  argument or use the output of the cluster to create the user.

3. Run the import command:

` + "```shell" + `
terraform import risingwavecloud_cluster_user.test <cluster_id>.<username>
` + "```" + `

  ~> **Note:** The password is set to NULL in the state file after the import. Terraform will show a password change
  when you run ` + "`" + `terraform plan` + "`" + `. 
`
