chown vault -R /vault/data/*
chmod 600 -R /vault/data/*
vault server -config=/vault/config/config.hcl
