-- ==============================================================================
-- coredraw - one-time bootstrap (NOT a goose migration)
-- ==============================================================================
--
-- Creates the coredraw schema, the runtime role, and its grants. Run ONCE per
-- environment as neondb_owner over the DIRECT (non-pooler) endpoint, before
-- the first migration (goose -dir internal/db/migrations postgres "$MIGRATION_URL" up):
--
--   psql "$MIGRATION_URL" -f internal/db/bootstrap.sql
--
-- Roles are cluster-level and must exist before migrations run as the owner,
-- so they live here rather than in the goose chain (same split as the
-- gateway's and corefinance's bootstrap.sql).
-- ==============================================================================

CREATE SCHEMA IF NOT EXISTS coredraw AUTHORIZATION neondb_owner;

-- Runtime role: DML-only on its own schema; never the owner.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'coredraw_role') THEN
    CREATE ROLE coredraw_role LOGIN PASSWORD 'REPLACE_ME';
  END IF;
END
$$;

GRANT USAGE ON SCHEMA coredraw TO coredraw_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA coredraw TO coredraw_role;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA coredraw TO coredraw_role;

ALTER DEFAULT PRIVILEGES FOR ROLE neondb_owner IN SCHEMA coredraw
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO coredraw_role;
ALTER DEFAULT PRIVILEGES FOR ROLE neondb_owner IN SCHEMA coredraw
  GRANT USAGE, SELECT ON SEQUENCES TO coredraw_role;

-- Pooler-safe schema scoping: the pooler strips per-connection options.
ALTER ROLE coredraw_role SET search_path = coredraw;
