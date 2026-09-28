#!/bin/sh
set -eu

# The application reads environment variables. Keep their values out of Compose
# metadata and command-line arguments, then exec to preserve signal delivery.
JWT_SECRET=$(cat /run/secrets/jwt_secret)
DB_KEYS_MASTER_KEY_B64=$(cat /run/secrets/master_key)
ADMIN_PASSWORD=$(cat /run/secrets/admin_password)
REDIS_PASSWORD=$(cat /run/secrets/redis_password)
db_password=$(cat /run/secrets/postgres_password)
# ota-init.py generates a URL-safe hex password, so no DSN interpolation ambiguity.
case "$db_password" in ''|*[!0-9a-f]*) echo 'Invalid generated database password' >&2; exit 1 ;; esac
DB_URL="postgres://ota:${db_password}@postgres:5432/ota?sslmode=disable"
export JWT_SECRET DB_KEYS_MASTER_KEY_B64 ADMIN_PASSWORD REDIS_PASSWORD DB_URL
unset db_password
exec "$@"
