chown vault -R /vault
(sleep 2 && vault operator unseal $VAULT_UNSEAL_KEY) &
vault server -config=/vault/config/config.hcl

