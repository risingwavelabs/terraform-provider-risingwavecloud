resource "risingwavecloud_cluster_user" "test" {
  # Reference the cluster instead of hardcoding its ID, so that Terraform creates the
  # cluster first and deletes the user before the cluster.
  cluster_id = risingwavecloud_cluster.mycluster.id
  username   = "test-user"
  password   = "test-password"

  # Role attributes. The API can only change a password afterwards, so these are fixed
  # once the user exists: changing one is reported as an error rather than planned as a
  # replacement, because recreating a user drops everything granted to it.
  super_user  = false
  create_db   = true
  create_user = true # "Allow creating roles" in the RisingWave Cloud portal
}
