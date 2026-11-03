# parts-tracker-backend

Backend for tracking electronic parts. Go (Gin + GORM + Postgres), built as a **modular monolith**: one deployable, split into modules with explicit boundaries so it can be separated later.

Sibling UI: `parts-tracker-ui` (Angular). Full design notes and the migration plan: [docs/design/modular-monolith.md](docs/design/modular-monolith.md).

## Architecture

```
                 ┌──────────────────── cmd/server (composition root) ────────────────────┐
   HTTP ───────▶ │  identity          parts                    bomconverter                │
                 │  /register         /part*                   /bom/import                 │
                 │  /login            search + cache                │                      │
                 │  JWT middleware          ▲                       │                      │
                 │                          └──── parts.Service ────┘                      │
                 │  platform: config · db · migrate · httpx                                │
                 └─────────────────────────────────────────────────────────────────────────┘
```

Dependency rules:

```
bomconverter ──▶ parts
identity        (independent)
all modules  ──▶ platform
```

1. A module exposes only its root package (`Service` interface, DTOs, `RegisterRoutes`). Everything else lives under that module's `internal/`.
2. Modules never import another module's internals or touch another module's tables.
3. `parts` does not import `identity`; auth middleware is attached to the route group in `cmd/server`.
4. `platform` imports no module. Only `cmd/server` knows every module.

### Layout

```
cmd/
  server/             HTTP server (builds platform, constructs modules, mounts routes)
  bomcli/             import BOM files straight into the database (no HTTP, no login)
internal/
  platform/           config, db, migrate, httpx (CORS)
  identity/           users, register/login, JWT issue/verify, auth middleware
  parts/              parts CRUD, search, cache, designator classification, bulk create
  bomconverter/       XLSM/XLSX BOM parsing and import
migrations/           versioned SQL (embedded into the binary)
docs/design/          design doc
```

## API

All paths are under `/api`. Everything except `/ping`, `/register` and `/login` needs `Authorization: Bearer <token>`.

| Method | Path | Description |
|---|---|---|
| GET | `/ping` | health check |
| POST | `/register` | `{username, password, firstname, surname}`; 409 if the username is taken |
| POST | `/login` | `{username, password}` → `{token}` |
| GET | `/part` | search; returns `{list, count}` |
| GET | `/part/:id` | one part |
| POST | `/part` | create; 409 if the part already exists |
| PUT | `/part/:id` | partial update (only fields present in the body); 409 if it would duplicate another part |
| DELETE | `/part/:id` | delete |
| POST | `/bom/import` | upload a BOM workbook (see below) |

### Search

Query parameters: `pcb_ref`, `part_number`, `value`, `description`, `type`, `page` (zero-based), `page_size` (default 10, max 100). Filters are case-insensitive substring matches, combined with AND. Results are ordered by `id`.

### Part fields

`id`, `part_number`, `pcb_ref`, `description`, `footprint`, `fit`, `manufacturer`, `type`, `project_name`, `value`. On create, everything except `description` and `footprint` is required.

### Status codes

400 invalid input · 401 missing/invalid token · 404 not found (including a non-numeric id) · 409 duplicate · 413 upload too large · 422 unreadable BOM workbook · 500 internal (details are logged, never returned).

## BOM import

`POST /api/bom/import`, `multipart/form-data`, file in the field `file`. `.xlsm` or `.xlsx`, at most 10 MB.

```json
{ "total": 120, "inserted": 100, "skipped": 5, "duplicates": 15 }
```

- The first sheet is read. `Project Name :` and `PCB Ref:` rows above the header row give the project name and PCB ref. The row starting with `Designator` is the header; every row after it is a part.
- The part type comes from the designator prefix (`C12` → Capacitor, `R3,R4` → Resistor, …). Unknown designators are **skipped**, as are rows missing a required field.
- **Duplicates are not stored.** A part is identified by (`pcb_ref`, `part_number`, `project_name`). The same part in a different project is allowed; the same part twice in one project is not, whether it repeats in the file or already exists in the database.
- All valid rows are inserted in one transaction.
- The request is synchronous: it returns when the import has finished.

CLI equivalent (needs the same environment as the server):

```bash
go run ./cmd/bomcli path/to/bom.xlsm [more files...]
```

## Search cache

`Search` is cached in process (no Redis): bounded LRU with TTL. The key is `search:v{version}:{sha256(normalized query)}`; every successful write bumps an atomic version so old entries become unreachable and age out. Concurrent identical misses share one database query (singleflight). With several instances, each has its own cache, so staleness is bounded by the TTL.

## Database

Migrations are versioned SQL files in `migrations/`, embedded in the binary and applied by [golang-migrate](https://github.com/golang-migrate/migrate) when `RUN_MIGRATIONS=true`. There is no `AutoMigrate`.

| Migration | Purpose |
|---|---|
| `0001_baseline` | creates `users` and `parts` if missing (no-op on existing databases) |
| `0002_users_unique_username` | unique index on `users.username` |
| `0003_parts_trgm_indexes` | `pg_trgm` extension and GIN trigram indexes |
| `0004_parts_unique_identity` | unique index on `parts (pcbref, partnumber, projectname)` |

Indexes:

| Index | Table | Columns | Type |
|---|---|---|---|
| `parts_pkey` | parts | `id` | primary key |
| `users_username_key` | users | `username` | unique |
| `parts_{partnumber,pcbref,value,description,type}_trgm_idx` | parts | one column each | GIN trigram |
| `parts_identity_key` | parts | `pcbref, partnumber, projectname` | unique |

The trigram indexes make `ILIKE '%term%'` use an index instead of scanning the table. They cost some write speed and disk space, and do not help search terms under 3 characters.

`0002` and `0004` fail if existing data already has duplicates. To find duplicate parts:

```sql
SELECT pcbref, partnumber, projectname, count(*) FROM parts GROUP BY 1,2,3 HAVING count(*) > 1;
```

## Configuration

Read from the environment (a `.env` file is loaded if present).

| Variable | Required | Default | Description |
|---|---|---|---|
| `DB_URL` | yes | | Postgres connection string |
| `JWT_SECRET` | yes | | JWT signing secret; startup fails if empty |
| `PORT` | no | `8080` | HTTP port |
| `BCRYPT_COST` | no | bcrypt default | password hashing cost |
| `RUN_MIGRATIONS` | no | `false` | apply pending migrations at startup |
| `PARTS_CACHE_MAX_ENTRIES` | no | `1000` | search cache size; `0` or less disables it |
| `PARTS_CACHE_TTL` | no | `60s` | search cache entry lifetime |

> `docker-compose.yml` and `env.example` still list the old `DB_HOST`/`DB_PORT`/`DB_USER`… variables; the app reads `DB_URL`. They need updating.

## Development

```bash
go run ./cmd/server          # start the API
go test ./...                # unit tests
```

Repository tests need a disposable Postgres and are skipped when `TEST_DB_URL` is not set:

```bash
docker run -d --rm --name pt-test-pg -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=t -p 55432:5432 postgres:16-alpine
TEST_DB_URL='postgres://postgres:pw@localhost:55432/t?sslmode=disable' go test ./internal/parts/...
```

## Status and roadmap

Done: platform and identity, parts module, search cache, synchronous BOM import with duplicate skipping.

Planned (see the design doc):

- Asynchronous imports: NATS JetStream with a job table as the source of truth.
- Completion notifications to the user over SSE.
- CI architecture check, Dockerfile/compose cleanup (move secrets to `.env`).
