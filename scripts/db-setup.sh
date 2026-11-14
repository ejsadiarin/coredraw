#!/usr/bin/env bash
# One-time database setup for coredraw (per environment):
#   1. coredraw_role with a generated password, written into .env's DATABASE_URL
#   2. internal/db/bootstrap.sql: coredraw schema, grants, search_path
#   3. goose migrations
#   4. a check that coredraw_role lands in the coredraw schema
#
# Safe to re-run. The password is only set when the role doesn't exist yet or
# DATABASE_URL still holds the REPLACE_ME placeholder; pass --rotate to generate a new one anyway (restart
# coredraw-api afterwards so it picks up the new .env).
#
# Needs psql, openssl and the goose CLI
# (go install github.com/pressly/goose/v3/cmd/goose@latest).
set -euo pipefail
cd "$(dirname "$0")/.."

rotate=false
[[ "${1:-}" == "--rotate" ]] && rotate=true

[[ -f .env ]] || { echo "no .env; copy .env.example first" >&2; exit 1; }
MURL=$(grep -m1 '^MIGRATION_URL=' .env | cut -d= -f2-)
DB=$(grep -m1 '^DATABASE_URL=' .env | cut -d= -f2-)
[[ -n "$MURL" && -n "$DB" ]] || { echo "MIGRATION_URL and DATABASE_URL must be set in .env" >&2; exit 1; }
[[ "$DB" == *"://coredraw_role:"* ]] || { echo "DATABASE_URL must connect as coredraw_role" >&2; exit 1; }

role_exists=$(psql "$MURL" -X -A -t -v ON_ERROR_STOP=1 -c "select count(*) from pg_roles where rolname = 'coredraw_role';")

# Neon rejects weak passwords (including bootstrap.sql's REPLACE_ME) when a
# role is created, so create coredraw_role here with a generated password
# before bootstrap.sql runs; its CREATE ROLE is then skipped.
if $rotate || [[ "$role_exists" == "0" || "$DB" == *"coredraw_role:REPLACE_ME@"* ]]; then
  echo "==> setting coredraw_role password"
  PW=$(openssl rand -base64 36 | tr '+/' '-_' | tr -d '=\n')
  psql "$MURL" -X -q -v ON_ERROR_STOP=1 -v pw="$PW" <<'SQL'
SELECT format('CREATE ROLE coredraw_role LOGIN PASSWORD %L', :'pw')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'coredraw_role') \gexec
ALTER ROLE coredraw_role PASSWORD :'pw';
SQL
  sed -i -E "s#^(DATABASE_URL=postgresql://coredraw_role:)[^@]*@#\1${PW}@#" .env
  DB=$(grep -m1 '^DATABASE_URL=' .env | cut -d= -f2-)
fi

echo "==> bootstrap"
psql "$MURL" -X -q -v ON_ERROR_STOP=1 -f internal/db/bootstrap.sql

echo "==> migrations"
goose -dir internal/db/migrations postgres "$MURL" up

echo "==> check"
psql "$DB" -X -A -t -v ON_ERROR_STOP=1 -c "show search_path;" -c "select count(*) || ' boards' from boards;"
echo "coredraw database ready."
