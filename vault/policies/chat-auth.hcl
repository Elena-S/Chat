path "auth/approle/role/chat/role-id" {
   capabilities = [ "read" ]
}

path "auth/approle/role/chat/secret-id" {
   capabilities = [ "update" ]
}