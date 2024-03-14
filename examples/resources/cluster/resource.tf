resource "risingwavecloud_cluster" "example" {
  name    = "dev"
  version = "v1.3.0"
  region  = "us-east-2"
  resourcev1 = {
    compute = {
      cpu     = "1"
      memory  = "4 GB"
      replica = 1
    }
    compactor = {
      cpu     = "1"
      memory  = "4 GB"
      replica = 3
    },
    frontend = {
      cpu     = "1"
      memory  = "4 GB"
      replica = 1
    },
    meta = {
      cpu     = "1"
      memory  = "4 GB"
      replica = 1
    },
    etcd = {
      cpu     = "1"
      memory  = "4 GB"
      replica = 3
    }
  }
}
