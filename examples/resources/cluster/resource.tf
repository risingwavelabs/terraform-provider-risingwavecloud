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
    }
  }
}

# The platform-managed extensions. An extension is enabled while its block is present.
#
# `spec.compactor.default_node_group.replica` is declared as usual. Serverless compaction holds
# the compactor at zero while it runs and the platform restores the count when the extension is
# removed, so the declared value is what the cluster goes back to rather than something to
# rewrite here.
resource "risingwavecloud_cluster" "with_extensions" {
  region = "us-east-1"
  name   = "my-cluster"
  tier   = "Invited"

  spec = {
    compute   = { default_node_group = { cpu = "1", memory = "4 GB", replica = 1 } }
    compactor = { default_node_group = { cpu = "1", memory = "4 GB", replica = 1 } }
    frontend  = { default_node_group = { cpu = "1", memory = "4 GB", replica = 1 } }
    meta      = { default_node_group = { cpu = "1", memory = "4 GB", replica = 1 } }
  }

  extensions = {
    serverless_compaction = {
      maximum_compaction_concurrency = 4
    }
    serverless_backfill = {
      component_type_id = "p-1c4g"
      replica           = 2
    }

    # The config is TOML, not JSON: the platform parses it with a TOML decoder.
    iceberg_compaction = {
      component_type_id = "p-1c4g"
      replica           = 1
      config            = <<-TOML
        max_task_parallelism = 4
      TOML
    }
  }
}
