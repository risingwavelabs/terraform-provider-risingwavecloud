resource "risingwavecloud_database" "test" {
  # Reference the cluster instead of hardcoding its ID, so that Terraform creates the
  # cluster first and deletes the database before the cluster.
  cluster_id = risingwavecloud_cluster.mycluster.id
  name       = "test_db"

  # Reference the resource group instead of hardcoding its name, so that Terraform creates
  # the resource group first and deletes the database before the resource group. Omit this
  # argument to run the database's streaming jobs in the "default" resource group.
  resource_group = risingwavecloud_resource_group.streaming.name
}
