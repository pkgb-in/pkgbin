#!/bin/sh
set -eu

apk add --no-cache postgresql-client >/dev/null

for db in pkgbinnpm pkgbinruby pkgbinpython; do
  echo "Ensuring database ${db}"
  psql -h postgres -U pkgbin_user -d postgres -tc "SELECT 1 FROM pg_database WHERE datname='${db}'" | grep -q 1 || \
    psql -h postgres -U pkgbin_user -d postgres -c "CREATE DATABASE \"${db}\";"
  migrate -path /migrations -database "postgres://pkgbin_user:pkgbin_password@postgres:5432/${db}?sslmode=disable" up
done
