resource "risingwavecloud_cluster_allowed_iam_roles" "example" {
  # Reference the cluster instead of hardcoding its ID, so that Terraform creates the
  # cluster first and removes the principals before deleting it.
  cluster_id = risingwavecloud_cluster.mycluster.id

  # This resource owns the whole list: an ARN added in the console is removed on the next
  # apply. That is the point of declaring an access control list in configuration.
  role_arns = [
    "arn:aws:iam::123456789012:role/data-platform",
    "arn:aws:iam::123456789012:role/etl",
  ]
}
