storage "raft" {
  path = "/vault/data"
  node_id = "raft_node_0001"
  performance_multiplier = 1
}

listener "tcp" {
  address     = "0.0.0.0:8100"
  tls_disable = true
}

seal "transit" {
  address = "http://vault-autounseal:8200"
  disable_renewal = false
  key_name = "autounseal"
  mount_path = "transit/"
  tls_skip_verify = true
}

disable_mlock = true
ui = true
log_file = "/vault/logs"

api_addr = "http://127.0.0.1:8100"
cluster_addr = "https://127.0.0.1:8101"
