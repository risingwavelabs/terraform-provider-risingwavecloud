resource "risingwavecloud_cluster" "mycluster" {
  region  = "us-east-1"
  name    = "mycluster"
  version = "v1.8.0"
  spec = {
    compute = {
      default_node_group = {
        cpu     = "2"
        memory  = "8 GB"
        replica = 1
      }
    }
    compactor = {
      default_node_group = {
        cpu     = "1"
        memory  = "4 GB"
        replica = 1
      }
    }
    frontend = {
      default_node_group = {
        cpu     = "1"
        memory  = "4 GB"
        replica = 1
      }
    }
    meta = {
      default_node_group = {
        cpu     = "1"
        memory  = "4 GB"
        replica = 1
      }
      etcd_meta_store = {
        default_node_group = {
          cpu     = "1"
          memory  = "4 GB"
          replica = 1
        }
      }
    }
  }
}
