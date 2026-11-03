# Design: Modular Monolith for parts-tracker-backend

Status: Draft
Date: 2026-11-03

## 1. Context

Today the backend is one Gin service (`src/`) plus a separate CLI (`bomConverter/`) that logs in over HTTP and POSTs one part per request. Both live in the same Go module (`partlist`).

Problems with the current shape:

| # | Problem | Where |
|---|---|---|
| 1 | `User` and `Part` handlers share one `handler` package and both reach into `database.DB` | `src/handler/` |
| 2 | Package-level singletons (`database.DB`, `utilis.AppConfig`) block module isolation and testing | `src/db`, `src/utilis` |
| 3 | `Part` is defined twice (GORM model in `handler`, JSON-only in `bomConverter/lib/part`) | both |
| 4 | Part-type vocabulary (`GetPartType`) lives in the converter, but `/part?type=` filters on it | `bomConverter/lib/part` |
| 5 | Importer calls the API over HTTP with login + token, one request per part | `bomConverter/lib/request` |
| 6 | `jwtSecret` is read at package init, before `config.Init()` runs, so the secret is likely empty (**security bug**) | `src/app/auth/jwt/jwt.go` |
| 7 | `LIKE '%x%'` on five columns plus a separate `COUNT`: sequential scan on every search | `src/handler/part.go` |
| 8 | No migrations / `AutoMigrate`; two `Config` structs; Dockerfile does not build `bomConverter` | misc |

## 2. Goals and non-goals

Goals
- Split into modules with explicit boundaries and one-way dependencies.
- Importing a BOM becomes an asynchronous job that notifies the user on completion.
- Faster, cached part search.
- Keep existing API paths so the Angular UI and Bruno collection keep working.

Non-goals
- Splitting into separate deployables now (the design must allow it later).
- Redis, Elasticsearch, or any dedicated search engine.
- Roles / permissions (see Open questions).

## 3. Target architecture

Three business modules plus shared infrastructure.

```
                 ┌────────────────────────── cmd/server (composition root) ───────────────────────────┐
                 │                                                                                      │
   HTTP ───────▶ │  identity            parts                       bomconverter                        │
                 │  /register           /part*                      /bom/import, /bom/jobs/:id          │
                 │  /login              search + cache              worker (NATS consumer)              │
                 │  JWT middleware                ▲                          │                           │
                 │                                └──── parts.Service ───────┘                          │
                 │                                                                                      │
                 │  platform: config · db · eventbus (NATS) · http middleware · sse hub                 │
                 └──────────────────────────────────────────────────────────────────────────────────────┘
```

### 3.1 Dependency rules

```
bomconverter ──▶ parts
identity        (independent)
all modules  ──▶ platform
```

1. A module exposes only what is in its root package (`Service` interface, DTOs, `Register(...)`). Everything else is under `internal/` of that module.
2. Modules never import another module's internals, and never touch another module's tables.
3. `parts` does not import `identity`. Auth middleware is attached at route registration in `cmd/server`.
4. `platform` imports no module.
5. Only `cmd/server` knows every module. Enforce rules 1-4 in CI (`go-arch-lint` or a `depguard` config).

### 3.2 Directory layout

```
cmd/
  server/main.go                  # builds platform, constructs modules, mounts routes
internal/
  platform/
    config/                       # one Config, loaded once, passed down
    db/                           # *gorm.DB constructor, migration runner
    eventbus/                     # EventBus interface + NATS JetStream impl + in-memory impl (tests)
    httpx/                        # cors, error envelope, pagination
    sse/                          # per-user SSE hub
  identity/
    service.go                    # Service interface: Register, Login, Verify
    middleware.go                 # Gin middleware built from Service.Verify
    internal/{repo,jwt,handler}/
  parts/
    service.go                    # Service interface (see 4.2)
    designator.go                 # ClassifyDesignator (moved from bomConverter)
    internal/{repo,handler,cache}/
  bomconverter/
    service.go                    # EnqueueImport, GetJob
    internal/{parser,descclient,worker,repo,handler}/
migrations/                       # SQL files, one numbered pair per change
```

The existing `bomConverter` CLI can stay as a thin `cmd/bomcli` that calls the same `bomconverter` code with an in-memory bus, so local use is not lost.

## 4. Modules

### 4.1 identity

Owns: `users` table, `/register`, `/login`, JWT issue/verify.

```go
type Service interface {
    Register(ctx context.Context, in RegisterInput) error
    Login(ctx context.Context, username string, password string) (string, error) // returns JWT
    Verify(ctx context.Context, token string) (Claims, error)
}
```

- `JWTSecret` is passed to the constructor from `platform/config` (fixes problem 6). Fail startup if it is empty.
- Add a unique index on `users.username`; `Register` returns a typed `ErrUsernameTaken`.
- Split into `user` + `auth` modules only when there is a second reason (roles, profile, refresh tokens, OAuth). The boundary then is `auth → user`, with password hashing staying in `user`.

### 4.2 parts

Owns: `parts` table, `/part*` routes, the part-type vocabulary, search + cache.

```go
type Service interface {
    Get(ctx context.Context, id string) (Part, error)
    Search(ctx context.Context, q SearchQuery) (SearchResult, error) // cached
    Create(ctx context.Context, p NewPart) (Part, error)
    Update(ctx context.Context, id string, p PartPatch) (Part, error)
    Delete(ctx context.Context, id string) error
    BulkCreate(ctx context.Context, ps []NewPart) (int, error)       // one transaction
    ClassifyDesignator(designator string) PartType
}
```

Routes (unchanged paths, JWT required):

| Method | Path | Maps to |
|---|---|---|
| GET | `/api/part` | `Search` |
| GET | `/api/part/:id` | `Get` |
| POST | `/api/part` | `Create` |
| PUT | `/api/part/:id` | `Update` |
| DELETE | `/api/part/:id` | `Delete` |

Fixes carried over: return after empty-id checks, bind errors return 400 (not 500), messages no longer say "capacitor".

#### Search

- Add `pg_trgm` and GIN trigram indexes on `partnumber`, `pcbref`, `value`, `description`, `type` (migration `0003`; `0001` is the baseline, `0002` the users unique index). This fixes the cold-query cost, which a cache cannot.
- List and count are computed together and returned as one `SearchResult` (`{list, count}`), as today.

#### Cache (in-process, no Redis)

| Aspect | Decision |
|---|---|
| Store | Bounded LRU with TTL (`hashicorp/golang-lru/v2/expirable` or `otter`), max entries configurable |
| Key | `search:v{ver}:{sha256(normalized params)}`; params trimmed, lowercased, sorted |
| Value | the whole `SearchResult` |
| TTL | 60 s (safety net; bounds staleness across instances) |
| Invalidation | `atomic.Uint64` version bumped on every write; `BulkCreate` bumps once per call. No key scanning; stale entries age out |
| Stampede | `singleflight.Group` on the miss path |
| Not cached | `Get(id)` (primary-key lookup is already cheap) |
| Interface | `Cache[K, V]` defined in `parts`, so a Redis impl can replace it later |

Multi-instance: each instance has its own cache and counter, so a write on instance A does not invalidate B. Acceptable while running one instance (TTL bounds it). If we scale out, publish `parts.changed` on the event bus and have every instance bump its counter. Still no Redis.

### 4.3 bomconverter

Owns: `bom_import_jobs` table, uploaded-file storage, XLSM parsing, description enrichment, import worker.

```go
type Service interface {
    EnqueueImport(ctx context.Context, owner string, file io.Reader, name string) (JobID, error)
    GetJob(ctx context.Context, owner string, id JobID) (Job, error)
}
```

Routes (JWT required):

| Method | Path | Behaviour |
|---|---|---|
| POST | `/api/bom/import` | multipart `.xlsm`; stores file, inserts job (`pending`), publishes `bom.import.requested`; returns `202 {job_id}` |
| GET | `/api/bom/jobs/:id` | job status; only the owner may read it |

Flow:

```
client ─POST /bom/import─▶ bomconverter ─save file─▶ storage
                                  │ insert job(pending)
                                  └─publish bom.import.requested─▶ [JetStream]
                                                                       │
worker ◀──────────────────────────────────────────────────────────────┘
  1. UPDATE job SET status='processing' WHERE id=? AND status='pending'   (claim; 0 rows ⇒ already handled, ack and stop)
  2. parse XLSM, classify designators via parts.ClassifyDesignator
  3. enrich descriptions via DESCRIPTION_URL (best-effort, see 8)
  4. parts.BulkCreate(...) then UPDATE job status='done'
  5. publish bom.import.completed (or .failed)
                                                                       │
sse hub ◀── consumer of completed/failed ───────────────────────────────┘ ─▶ push to owner's open SSE streams
```

- The importer calls `parts.Service` in-process. No login, no token, no per-part HTTP calls.
- Delivery is at-least-once, so the job claim in step 1 and a single DB transaction for step 4 keep reprocessing from duplicating rows.
- The job table, not NATS, is the source of truth for status.

## 5. Events (NATS JetStream)

Stream `BOM`, subjects `bom.import.*`, durable pull consumer `bom-worker`, file storage, retention by limits (7 days), `MaxDeliver` 5 with backoff, dead-letter via advisory to `bom.import.dead`.

| Subject | Producer | Consumer | Payload |
|---|---|---|---|
| `bom.import.requested` | bomconverter (HTTP side) | bomconverter worker | `{job_id, owner, file_key, original_name}` |
| `bom.import.completed` | worker | sse hub | `{job_id, owner, inserted, skipped, finished_at}` |
| `bom.import.failed` | worker | sse hub | `{job_id, owner, reason, finished_at}` |
| `parts.changed` (later) | parts | parts (other instances) | `{version}` |

Messages carry a file key, never file bytes. `EventBus` in `platform/eventbus` is a small interface (`Publish`, `Subscribe`), so modules and tests do not depend on NATS.

## 6. Notification (SSE)

- `GET /api/notifications/stream` returns `text/event-stream`.
- The hub maps `username → set of connections`. On `bom.import.completed|failed` it writes an event to that user's connections.
- Browser `EventSource` cannot send an `Authorization` header. Use either `@microsoft/fetch-event-source` in the Angular app (header works) or a short-lived, single-use stream ticket from an authenticated `POST /api/notifications/ticket`. Recommended: fetch-based SSE.
- Missed events are recovered with `GET /api/bom/jobs/:id` after reconnect. The UI shows job status from that endpoint on load.
- The hub lives in `platform/sse`; it can become its own `notification` module if it grows (multiple channels, persistence, web push).

## 7. Data ownership and migrations

| Table | Owner | Notes |
|---|---|---|
| `users` | identity | unique index on `username` |
| `parts` | parts | trigram GIN indexes; consider a unique key for import dedupe (Open questions) |
| `bom_import_jobs` | bomconverter | `id uuid pk, owner text, file_key text, status text, total_rows int, inserted_rows int, error text, created_at, updated_at, finished_at` |

- Use versioned SQL migrations in `migrations/` (e.g. `golang-migrate`), run at startup behind a flag. No reliance on `AutoMigrate` in production.
- No foreign keys across modules. `bom_import_jobs.owner` stores the username from the JWT.

## 8. Cross-cutting

- **Config:** one `Config` struct loaded in `cmd/server`, passed into constructors. No package-level state. Validate required values at startup (`JWT_SECRET`, `DB_*`, `NATS_URL`, `DESCRIPTION_URL`).
- **Errors:** typed errors per module (`ErrNotFound`, `ErrUsernameTaken`, `ErrInvalidInput`) mapped to HTTP status in the handler layer. Never swallow errors.
- **Description service (`DESCRIPTION_URL`):** call with a timeout and retry; on failure, import with empty descriptions and record a warning on the job rather than failing it (to confirm, see Open questions).
- **Secrets:** move `JWT_SECRET` and DB password out of `docker-compose.yml` into `.env`.
- **Docker:** Dockerfile builds `./cmd/server`; compose adds a `nats` service (JetStream enabled) and a volume for uploaded files.

## 9. Testing

- Unit tests per module against its `Service` interface; use the in-memory `EventBus` and a fake repo.
- Repo tests against a real Postgres (testcontainers) for search, trigram queries, and `BulkCreate` transactionality.
- Cache tests: key normalization, version invalidation, single Postgres hit under concurrent identical searches.
- Worker tests: duplicate delivery of `bom.import.requested` does not insert twice.
- Architecture test in CI to enforce the dependency rules in 3.1.

## 10. Migration plan

Each step is shippable on its own and leaves the API working.

| Step | Work | Result |
|---|---|---|
| 1 | Create `internal/platform` (config, db, httpx) and `internal/identity`; inject the JWT secret | Fixes the empty-secret bug; auth isolated |
| 2 | Create `internal/parts`: move model/handlers, add `Service`, `ClassifyDesignator`, `BulkCreate`, `pg_trgm` indexes | Parts isolated; searches fast on cold cache |
| 3 | Add the in-process cache with versioning + singleflight | Repeat searches served from memory |
| 4 | Move `bomConverter` into `internal/bomconverter`, calling `parts.Service` in-process (still synchronous / CLI) | One Part model; no HTTP self-calls |
| 5 | Add NATS JetStream, job table, `POST /bom/import`, worker | Async imports |
| 6 | Add SSE hub + `/notifications/stream`; Angular consumes it | Users notified on completion |
| 7 | CI architecture check, Dockerfile + compose updates, remove old `src/handler`, `src/utilis` | Old structure gone |

## 11. Risks

- NATS adds an operational dependency; mitigated by the `EventBus` interface and an in-memory implementation for local runs and tests.
- In-process cache is per instance; staleness is bounded by the 60 s TTL until the `parts.changed` event is added.
- At-least-once delivery requires the claim + transaction discipline in 4.3; a bug there duplicates parts.
- SSE needs the auth approach in section 6 on the Angular side.

## 12. Open questions

1. Should `bomconverter` stay a CLI only, or also expose `POST /api/bom/import`? (This doc assumes both.)
2. Duplicate policy on import: insert always, skip, or upsert on (`pcb_ref`, `part_number`, `project_name`)? Today there is no unique constraint.
3. Uploaded file storage: local volume or S3?
4. "Push notification": is in-app SSE enough, or is web push / FCM needed when the tab is closed?
5. If the description service is down, should the import fail or proceed with empty descriptions?
6. Are roles needed soon? If yes, split `identity` into `user` + `auth` now.
