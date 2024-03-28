package provider

var providerMarkdownDescription = `
The Terraform plugin for [RisingWave Cloud](https://cloud.risingwave.com/) allows you to manage your resources 
on the RisingWave Cloud platform with Terraform.

**This project is under heavy development. Please join our 
[Slack](https://join.slack.com/t/risingwave-community/shared_invite/zt-1jei7dk79-fguGadPI2KnhtWnnxBVGoA) to get the latest information.**

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
	api_key    = <API Key>
	api_secret = <API Secret>
}

# Create a RisingWave Cluster
resource "risingwavecloud_cluster" "mycluster" {
  name    = "mycluster"
  version = "v1.7.1"
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
      etcd_meta_store = {
        default_node_group = {
          cpu    = "0.5"
          memory = "2 GB"
        }
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


## Authentication
The API key and API secret are created at the [RisingWave Cloud portal](https://cloud.risingwave.com/).

Note that you can also use environment variables to set the API key and API secret:
` + "```hcl" + `
RWC_API_KEY=myapikeyvalue
RWC_API_SECRET=myapisecretvalue
` + "```" + `
This allows you to manage your credentials in a more secure way.


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
    cluster_id      = "cluster-id"
    connection_name = "test-connection"
    target          = "test-target"
  }
  ` + "```" + `

3. Run the import command:

` + "```shell" + `
terraform import risingwavecloud_privatelink.test <privatelink_id>
` + "```" + `
`

var clusterUserMarkdownDescription = `
A database user in a RisingWave cluster. The username and password of the dabase user are used to
connect to the RisingWave cluster.

## Import a Cluster User

To import a cluster user, follow the steps below:

1. Get the UUID of the corrsponding cluster from the RisingWave Cloud platform.

2. Write a resource definition to import the cluster user. For example:

` + "```hcl" + `
  resource "risingwavecloud_cluster_user" "test" {
    cluster_id = "cluster-id"
    username   = "test-user"
    password   = "test-password"
  } 
  ` + "```" + `

3. Run the import command:

` + "```shell" + `
terraform import risingwavecloud_cluster_user.test <cluster_id>.<username>
` + "```" + `
`