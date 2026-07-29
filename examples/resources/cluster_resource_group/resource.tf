resource "risingwavecloud_cluster_resource_group" "streaming" {
  # Reference the cluster instead of hardcoding its ID, so that Terraform creates the
  # cluster first and deletes the resource group before the cluster.
  cluster_id        = risingwavecloud_cluster.mycluster.id
  name              = "streaming-rg"
  component_type_id = "p-1c4g"
  replica           = 1
}
