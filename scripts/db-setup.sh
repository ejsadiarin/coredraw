#!/usr/bin/env bash
# One-time database setup for coredraw (per environment):
#   1. internal/db/bootstrap.sql: coredraw schema, coredraw_role, grants
#   2. a generated password for coredraw_role, written into .env's DATABASE_URL
#   3. goose migrations
#   4. a check that coredraw_role lands in the coredraw schema
#
# Safe to re-run. The password is only set while DATABASE_URL still holds the
# REPLACE_ME placeholder; pass --rotate to generate a new one anyway (restart
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

echo "==> bootstrap"
psql "$MURL" -X -q -v ON_ERROR_STOP=1 -f internal/db/bootstrap.sql

if $rotate || [[ "$DB" == *"coredraw_role:REPLACE_ME@"* ]]; then
  echo "==> setting coredraw_role password"
  PW=$(openssl rand -hex 24)
  psql "$MURL" -X -q -v ON_ERROR_STOP=1 -c "ALTER ROLE coredraw_role PASSWORD '$PW';"
  sed -i -E "s#^(DATABASE_URL=postgresql://coredraw_role:)[^@]*@#\1${PW}@#" .env
  DB=$(grep -m1 '^DATABASE_URL=' .env | cut -d= -f2-)
fi

echo "==> migrations"
goose -dir internal/db/migrations postgres "$MURL" up

echo "==> check"
psql "$DB" -X -A -t -v ON_ERROR_STOP=1 -c "show search_path;" -c "select count(*) || ' boards' from boards;"
echo "coredraw database ready."
