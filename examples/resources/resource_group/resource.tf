resource "risingwavecloud_resource_group" "test" {
  cluster_id        = "cluster-id"
  name              = "streaming-rg"
  component_type_id = "p-1c4g"
  replica           = 1
}
