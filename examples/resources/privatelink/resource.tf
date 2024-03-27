resource "risingwavecloud_privatelink" "test" {
  cluster_id      = "cluster-id"
  connection_name = "test-connection"
  target          = "test-target"
}
