variable "user_password" {
  type      = string
  sensitive = true
}

resource "risingwavecloud_cluster_user" "test" {
  # Reference the cluster instead of hardcoding its ID, so that Terraform creates the
  # cluster first and deletes the user before the cluster.
  cluster_id = risingwavecloud_cluster.mycluster.id
  username   = "test-user"

  # A write-only argument: Terraform passes the value to the provider but stores it in
  # neither the plan nor the state. Requires Terraform 1.11 or later. Use `password`
  # instead on older versions, at the cost of keeping the secret in the state file.
  password_wo = var.user_password

  # Terraform cannot detect a change in a value it does not store, so increment this
  # whenever the password changes. Changing `password_wo` on its own does nothing.
  password_wo_version = 1

  # Role attributes. The API can only change a password afterwards, so these are fixed
  # once the user exists: changing one is reported as an error rather than planned as a
  # replacement, because recreating a user drops everything granted to it.
  super_user  = false
  create_db   = true
  create_user = true # "Allow creating roles" in the RisingWave Cloud portal
}
