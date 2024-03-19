---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "risingwavecloud_cluster_user Resource - terraform-provider-risingwavecloud"
subcategory: ""
description: |-
  A RisingWave Cluster
---

# risingwavecloud_cluster_user (Resource)

A RisingWave Cluster



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