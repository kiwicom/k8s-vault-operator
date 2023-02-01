path "secret" {
  capabilities = ["list"]
}

path "secret/data/*" {
  capabilities = ["create", "update", "delete", "list", "read"]
}