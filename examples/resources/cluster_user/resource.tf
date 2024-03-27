resource "risingwavecloud_cluster_user" "test" {
  cluster_id = "cluster-id"
  username   = "test-user"
  password   = "test-password"
}
