resource "risingwavecloud_database" "test" {
  cluster_id     = "cluster-id"
  name           = "test_db"
  resource_group = "default"
}
