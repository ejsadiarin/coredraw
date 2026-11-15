# coredraw

Whiteboards for the Core dashboard, served as a downstream service behind
[coregateway](https://github.com/ejsadiarin/coregateway/blob/rewrite-v2/README.md).
The dashboard's **Draw** section (`web/` in coregateway) is its UI. Each
board belongs to one user and is saved whole, as a snapshot.

## Quick start

**The easy way: run it with everything else.** From a coregateway checkout,
its `local-up` target builds and starts coredraw, the gateway, the other services
and the dashboard, with their own Postgres. See coregateway's
[Quick start](https://github.com/ejsadiarin/coregateway/blob/rewrite-v2/README.md#quick-start-the-whole-stack-locally).

**Run this repo's code against that stack.** With coregateway's `local-up` target
running, a native coredraw can use the same database and gateway. Put
this in `.env`:

```sh
DATABASE_URL=postgresql://coredraw_role:cdpw@localhost:25432/coredb?sslmode=disable
MIGRATION_URL=postgresql://neondb_owner:owner@localhost:25432/coredb?sslmode=disable&options=-c%20search_path%3Dcoredraw
ENV=development
PORT=16971
JWT_ISSUER=https://gateway.internal
JWT_AUDIENCE=coredraw
JWT_JWKS_URL=http://localhost:28080/.well-known/jwks.json
```

```sh
make run
```

The stack's gateway keeps sending dashboard traffic to its own coredraw
container, so use this to run code against real tables, not to replace the
container.

## Production prerequisites

- **Database:** the `coredraw` schema and `coredraw_role` in Neon's `coredb`.
  With `MIGRATION_URL` (direct host, `neondb_owner`) and `DATABASE_URL`
  (pooled host, `coredraw_role`, any password) in `.env`, run:

  ```sh
  make db-setup
  ```

  It creates the role with a generated password and writes it into `.env`,
  runs `internal/db/bootstrap.sql` and the migrations, and checks that the
  role lands in its schema. It is safe to run again. It needs `psql`,
  `openssl` and [goose](https://github.com/pressly/goose)
  (`go install github.com/pressly/goose/v3/cmd/goose@latest`).
- **The gateway:** running and reachable at `JWT_JWKS_URL`. coredraw refuses
  to start without its signing keys.
- **`.env`:** copy `.env.example`. `JWT_AUDIENCE` is `coredraw` alone, never
  the gateway's comma-separated list.
- **One image:** `Dockerfile.coredraw-api`. Run `make migrate-up` before
  deploying a new image; nothing migrates at start.

No other accounts are needed. For the whole stack's database setup, see
coregateway's [Setting up Neon](https://github.com/ejsadiarin/coregateway/blob/rewrite-v2/README.md#setting-up-neon).

## API

Every `/api/coredraw` route needs a gateway-minted internal token (audience
`coredraw`), and only sees the caller's own boards.

| Method | Path | What it does |
|---|---|---|
| GET | `/health` | Liveness, no token |
| GET | `/api/coredraw/boards` | List your boards |
| POST | `/api/coredraw/boards` | Create a board |
| GET | `/api/coredraw/boards/{id}` | Read a board |
| PUT | `/api/coredraw/boards/{id}` | Save a board's contents |
| PATCH | `/api/coredraw/boards/{id}` | Rename a board |
| DELETE | `/api/coredraw/boards/{id}` | Delete a board |

Example requests: `coredraw-api.http`.

## Development

Go 1.26+, with [sqlc](https://sqlc.dev) and goose. Docker is needed for the
integration tests.

| Command | What it does |
|---|---|
| `make run` | Run the API |
| `make build` | Build `bin/coredraw-api` |
| `make test` | Unit and Docker-backed integration tests |
| `make test-unit` | Unit tests only, no Docker |
| `make db-setup` | One-time database setup (above) |
| `make db-rotate-password` | New `coredraw_role` password into `.env`; restart afterwards |
| `make migrate-up` / `migrate-down` / `migrate-status` | Migrations against `MIGRATION_URL` |
| `make migrate-create` | New migration |
| `make sqlc` | Regenerate after editing SQL in `internal/db/queries/` |
| `make docker-build` | Build the image |
