---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "risingwavecloud_cluster_user Resource - terraform-provider-risingwavecloud"
subcategory: ""
description: |-
  A database user in a RisingWave cluster. The username and password of the dabase user are used to
  connect to the RisingWave cluster.
  ~> Note: Username and password will be stored in the state file in plain text.
  Read more about sensitive data in state https://www.terraform.io/docs/state/sensitive-data.html.
  Import a Cluster User
  To import a cluster user, follow the steps below:
  
  Get the UUID of the corrsponding cluster from the RisingWave Cloud platform.
  Write a resource definition to import the cluster user. For example:
  
    resource "risingwavecloud_cluster_user" "test" {
      depends_on = [risingwavecloud_cluster.mycluster]
  
      cluster_id = "cluster-id"
      username   = "test-user"
      password   = "test-password"
    } 
  
  ~> Note: The password is stored in the state for comparing changes. RisingWave Cloud platform
  does not store the password after the user is created. If you change the password outside of Terraform,
  the new password won't be reflected in the state file.
  ~> Note: When destroying all resources, make sure the Terraform be aware of the dependency between the cluster and the user.
  If the cluster is deleted before the user, the deletion of the user will fail. You can either use the depends_on
  argument or use the output of the cluster to create the user.
  Run the import command:
  
  terraform import risingwavecloud_cluster_user.test <cluster_id>.<username>
  
  ~> Note: The password is set to NULL in the state file after the import. Terraform will show a password change
  when you run terraform plan.
---

# risingwavecloud_cluster_user (Resource)

A database user in a RisingWave cluster. The username and password of the dabase user are used to
connect to the RisingWave cluster.

~> **Note:** Username and password will be stored in the state file in plain text.
[Read more about sensitive data in state](https://www.terraform.io/docs/state/sensitive-data.html).

## Import a Cluster User

To import a cluster user, follow the steps below:

1. Get the UUID of the corrsponding cluster from the RisingWave Cloud platform.

2. Write a resource definition to import the cluster user. For example:

```hcl
  resource "risingwavecloud_cluster_user" "test" {
    depends_on = [risingwavecloud_cluster.mycluster]

    cluster_id = "cluster-id"
    username   = "test-user"
    password   = "test-password"
  } 
  ```

  ~> **Note:** The password is stored in the state for comparing changes. RisingWave Cloud platform
  does not store the password after the user is created. If you change the password outside of Terraform,
  the new password won't be reflected in the state file.

  ~> **Note:** When destroying all resources, make sure the Terraform be aware of the dependency between the cluster and the user.
  If the cluster is deleted before the user, the deletion of the user will fail. You can either use the `depends_on`
  argument or use the output of the cluster to create the user.

3. Run the import command:

```shell
terraform import risingwavecloud_cluster_user.test <cluster_id>.<username>
```

  ~> **Note:** The password is set to NULL in the state file after the import. Terraform will show a password change
  when you run `terraform plan`.



<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `cluster_id` (String) The NsID (namespace id) of the cluster.
- `password` (String, Sensitive) The password for connecting to the cluster
- `username` (String) The username for connecting to the cluster. The username is unique within the cluster.

### Optional

- `create_db` (Boolean) The create db flag for the user
- `super_user` (Boolean) The super user flag for the user

### Read-Only

- `id` (String) The global identifier for the resource: [cluster ID].[username]
