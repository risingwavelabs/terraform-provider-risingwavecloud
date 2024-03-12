resource "risingwavecloud_cluster" "example" {
  name    = "dev"
  version = "v1.3.0"
  region  = "us-east-2"
  resourcev1 = {
    compute = {
      type    = "p-1c2g"
      replica = 1
    }
    compactor = {
      type    = "p-2c4g"
      replica = 3
    },
    frontend = {
      type    = "p-1c1g"
      replica = 1
    },
    meta = {
      type    = "p-1c1g"
      replica = 1
    },
    etcd = {
      type    = "p-1c1g"
      replica = 3
    }
  }
}
