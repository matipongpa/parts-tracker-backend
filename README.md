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
assistant    ──▶ partstools ──▶ parts
mcpserver    ──▶ partstools
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
  mcp/                MCP server (stdio) exposing the read-only parts tools
internal/
  platform/           config, db, migrate, httpx (CORS)
  identity/           users, register/login, JWT issue/verify, auth middleware
  parts/              parts CRUD, search, cache, designator classification, bulk create
  bomconverter/       XLSM/XLSX BOM preview, column mapping and import
  partstools/         read-only parts queries as AI tools (search_parts, get_part_usage)
  assistant/          POST /api/ask: a model (Claude or OpenAI-compatible) calls partstools, answer + parts returned
  mcpserver/          the same tools served over MCP
migrations/           versioned SQL (embedded into the binary)
docs/design/          design doc
```

## API

All paths are under `/api`. Everything except `/ping`, `/register`, `/login` and `/auth/*` needs `Authorization: Bearer <access token>`.

| Method | Path | Description |
|---|---|---|
| GET | `/ping` | health check |
| POST | `/register` | `{invitecode, username, password, firstname, surname}`; 403 if the invite code is wrong or registration is closed, 409 if the username is taken |
| POST | `/login` | `{username, password}` → `{token}` (short-lived access token) and sets the httpOnly `refresh_token` cookie |
| POST | `/auth/refresh` | no body; reads the `refresh_token` cookie → `{token}`. 401 (and the cookie is cleared) if it is missing, expired or invalid |
| POST | `/auth/logout` | clears the `refresh_token` cookie, 204 |
| GET | `/part` | search; returns `{list, count}` |
| GET | `/part/:id` | one part |
| POST | `/part` | create; 409 if the part already exists |
| PUT | `/part/:id` | partial update (only fields present in the body); 409 if it would duplicate another part |
| DELETE | `/part/:id` | delete |
| POST | `/bom/preview` | read a BOM workbook and propose a column mapping (see below) |
| POST | `/bom/import` | import a BOM workbook with a confirmed mapping (see below) |
| POST | `/ask` | `{question}` → `{id, question, answer, parts, parts_truncated, tools_used, created_at}`; the answer is saved to the caller's history (`id` is empty if saving failed); only mounted when `ANTHROPIC_API_KEY` is set (see AI assistant) |
| GET | `/ask/history` | the caller's saved questions, newest first: `{list: [{id, question, created_at}]}` (works even when the assistant is disabled) |
| GET | `/ask/history/:id` | one saved conversation: `{id, question, answer, parts, parts_truncated, tools_used, created_at}`; 404 if it is not yours |
| DELETE | `/ask/history/:id` | delete one saved question, 204; 404 if it is not yours |

### Sessions

`/login` returns a 15-minute access token in the body, which the client keeps in memory only, and sets a refresh token as an `HttpOnly` cookie scoped to `/api/auth`, so page JavaScript can never read it. When the page reloads or the access token expires, the client calls `POST /auth/refresh` (with `credentials: 'include'`) to get a new one. A refresh token and an access token are not interchangeable: each is rejected where the other is expected.

| Env | Default | |
|---|---|---|
| `TOKEN_TTL` | `15m` | access token lifetime |
| `REFRESH_TOKEN_TTL` | `168h` | cookie lifetime; a login lasts this long at most, then the password is needed again |
| `COOKIE_SECURE` | `true` | set `false` only for plain-HTTP local dev (Safari will not store a Secure cookie on http) |
| `COOKIE_SAMESITE` | `lax` | `none` if the UI and API are on different sites (requires `COOKIE_SECURE=true`) |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:4200` | comma-separated UI origins; exact, no `*` (credentials are allowed) |

Known limit: refresh tokens are stateless JWTs, so `/auth/logout` removes the cookie but cannot revoke a token that was copied elsewhere; it expires on its own. A server-side token table with rotation would close that.

### Search

Query parameters: `pcb_ref`, `part_number`, `value`, `description`, `type`, `page` (zero-based), `page_size` (default 10, max 100). Filters are case-insensitive substring matches, combined with AND. Results are ordered by `id`.

### Part fields

`id`, `part_number`, `pcb_ref`, `description`, `footprint`, `fit`, `manufacturer`, `type`, `project_name`, `value`. On create, everything except `description` and `footprint` is required.

### Status codes

400 invalid input · 401 missing/invalid token · 404 not found (including a non-numeric id) · 409 duplicate · 413 upload too large · 422 unreadable BOM workbook · 500 internal (details are logged, never returned).

## BOM import

Two steps, so a file whose columns moved or were renamed is never stored with the wrong meaning:

1. `POST /api/bom/preview` reads the workbook and proposes how its columns map to part fields. Nothing is stored.
2. `POST /api/bom/import` stores the rows using the mapping the user confirmed.

Both are `multipart/form-data` with the workbook in the field `file` (`.xlsm` or `.xlsx`, at most 10 MB, JWT required).

**Preview** accepts optional `sheet`, `header_row` (zero-based) and `mapping` (JSON, to dry-run the user's edits). It returns the top of the sheet as a grid, the proposed `mapping`, where it came from (`mapping_source`: `saved`, `detected` or `given`), a per-field `confidence`, the required fields still `missing`, the `resolved` project name and PCB ref, and a `summary` (`rows`, `valid`, `skipped`; duplicates are only known at import).

**Import** accepts `sheet`, `mapping` and `save_mapping=true`:

```json
{ "total": 120, "inserted": 100, "skipped": 5, "duplicates": 15, "mapping_saved": true }
```

A mapping says, for each field, where its value comes from:

```json
{ "header_row": 3,
  "fields": {
    "designator":   { "kind": "column", "column": 0 },
    "part_number":  { "kind": "column", "column": 5 },
    "project_name": { "kind": "cell", "row": 0, "column": 1 },
    "pcb_ref":      { "kind": "text", "text": "PCB-1" } } }
```

`kind` is `column` (every row), `cell` (one fixed cell) or `text` (a constant). Rows and columns are zero-based. Fields: `designator` (must be a column; it decides the part type), `part_number`, `value`, `manufacturer`, `fit`, `project_name`, `pcb_ref` (all required) and `description`, `footprint` (optional). A mapping missing a required field is a **422** whose body lists them in `missing`.

- **Detection needs no AI.** Headers are normalised (case and punctuation ignored) and matched against a synonym list (`Manufacturer part no.`, `MPN` and `Part No` all mean part number). The header row is the first row naming at least two part fields. A designator column can also be found by its content (`C12`, `R3,R4`). Project name and PCB ref are found as columns, or as the cell next to a label such as `Project Name :` above the table. When no sheet is requested, the first sheet that looks like a BOM is used.
- **Saved mappings are global.** A confirmed mapping is stored in `bom_mappings` under a fingerprint of the header row (column positions and normalised names). The next file with the same header row is pre-filled with it, for every user; a changed header gives a new fingerprint and is detected afresh. The latest confirmation wins. A project name or PCB ref the user *typed* is not saved, since it belongs to that one file.
- **Plain upload still works.** `POST /api/bom/import` with only `file` uses the saved mapping, else the detected one, and fails with 422 if a required field cannot be found. The CLI does the same.
- Unknown designators are **skipped**, as are rows missing a required field. The part type comes from the designator prefix (`C12` is a capacitor, `R3,R4` a resistor).
- **Duplicates are not stored.** A part is identified by (`pcb_ref`, `part_number`, `project_name`). The same part in a different project is allowed; the same part twice in one project is not, whether it repeats in the file or already exists in the database.
- All valid rows are inserted in one transaction, synchronously. Workbooks are opened with unzip size limits.

CLI equivalent (needs the same environment as the server; refuses a file whose required columns it cannot find, so map those by hand in the web page):

```bash
go run ./cmd/bomcli path/to/bom.xlsm [more files...]
```

## AI assistant and MCP

Free-text questions such as "which projects use part GRM188?" are answered in two steps. The data is never searched or collected by the model: it calls **read-only tools** (`internal/partstools`) that run exact queries through `parts.Service`, then summarizes what they returned.

| Tool | Purpose |
|---|---|
| `search_parts` | the normal search (substring filters, paged, max 50 per page) |
| `get_part_usage` | one part number across all projects, grouped by project, with the boards (`pcb_ref`) and variants in each |

There is no tool that writes. Part descriptions come from uploaded spreadsheets and are treated as untrusted text.

The tools are exposed two ways from one definition:

- **`POST /api/ask`** (Bearer token). The server runs the model's tool loop (at most 6 tool rounds, 90 s). The response carries the model's `answer` and the `parts` the tools returned. The rows come from the tool results, not from the model's text, so the list cannot be altered by the model. At most 200 parts are returned (`parts_truncated`). It is only mounted when an API key is configured (below). There is no per-user rate limit yet.
- **`cmd/mcp`**, an MCP server on stdio for clients such as Claude Desktop, using the client's own model. It reads the database directly with the credentials in its environment, so run it only on a trusted machine:

```bash
go build -o parts-mcp ./cmd/mcp
```

```json
{ "mcpServers": { "parts-tracker": {
    "command": "/path/to/parts-mcp",
    "env": { "DB_URL": "postgres://…", "JWT_SECRET": "unused-but-required" } } } }
```

Quantities and reference designators are not stored (one row per part per board per project), so neither the answer nor the tools can say how many units a board uses.

### Ask history

Every answered question is saved per user in `ask_history` (migration `0005`): the question as sent (trimmed), the answer exactly as the model gave it, and a **snapshot** of the parts it was based on, so opening an old entry shows what the user saw then, even if parts have since changed or been deleted. Nothing is sent to the model when an entry is read. The newest 100 entries per user are kept; saving the 101st deletes the oldest. Failed or refused questions are not saved. If the save itself fails, the answer is still returned (with an empty `id`) and the error is logged.

Entries are keyed by the username from the token and every query filters on it, so another user's id answers 404. There is no foreign key to `users`, because modules do not touch each other's tables. Set `TEST_DB_URL` to run the Postgres-backed tests (`../postgres-image/scripts/dbctl.sh create partlist_test`, then `url partlist_test`).

### Choosing the model provider

`AI_PROVIDER` selects the adapter. Both implement the same small interface, so the tools, loop and MCP server are identical:

```bash
# Claude (default)
ANTHROPIC_API_KEY=...            # AI_MODEL optional, defaults to claude-opus-5-5

# Any OpenAI-compatible chat API (OpenAI, Ollama, OpenRouter, gateways such as tokenme)
AI_PROVIDER=openai-compatible
AI_API_KEY=...
AI_BASE_URL=https://api.tokenme.limited/api/v1   # the root; /chat/completions is appended
AI_MODEL=deepseek-ai/deepseek-v4-flash-0731
```

The model must support **tool calling**; without it the assistant cannot read any data. `AI_BASE_URL` must be `https` (plain `http` only for localhost). Keep keys in `.env` or the environment, never in the repository. A key that is missing leaves `/api/ask` unmounted; a key with missing or invalid companion settings fails at startup.

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
| `REGISTER_INVITE_CODE` | no | | shared code that `POST /api/register` requires; unset closes registration (403) |
| `RUN_MIGRATIONS` | no | `false` | apply pending migrations at startup |
| `PARTS_CACHE_MAX_ENTRIES` | no | `1000` | search cache size; `0` or less disables it |
| `PARTS_CACHE_TTL` | no | `60s` | search cache entry lifetime |
| `AI_PROVIDER` | no | `anthropic` | `anthropic` or `openai-compatible` |
| `ANTHROPIC_API_KEY` | no | | key for the `anthropic` provider; empty leaves `POST /api/ask` unmounted |
| `AI_API_KEY` | no | | key for the `openai-compatible` provider |
| `AI_BASE_URL` | with `AI_API_KEY` | | API root of the openai-compatible server |
| `AI_MODEL` | openai-compatible only | `claude-opus-5-5` (anthropic) | model id sent to the provider |

`env.example` lists every variable. The database itself is the shared Postgres in `../postgres-image`.

## Development

```bash
go run ./cmd/server          # start the API
docker compose up -d --build # or run it in a container (needs APP_DB_URL in .env, see env.example)
go test ./...                # unit tests
```

Repository tests need a disposable Postgres and are skipped when `TEST_DB_URL` is not set:

```bash
docker run -d --rm --name pt-test-pg -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=t -p 55432:5432 postgres:16-alpine
TEST_DB_URL='postgres://postgres:pw@localhost:55432/t?sslmode=disable' go test ./internal/parts/...
```

## Deploying with deploy-manager

`docker-compose.prod.yml` runs the whole stack: Postgres 16 (named volume `pgdata`), the API, and the UI served by nginx from the sibling repo `parts-tracker-ui`. Only the UI port is published (`WEB_PORT`, default 8082). Its nginx proxies `/api` to the API, so the browser sees one origin: no CORS, and the httpOnly refresh cookie stays first-party. Every service has a healthcheck, so `compose up --wait` fails the deploy on a bad release.

In deploy-manager, add an app with:

| Field | Value |
|---|---|
| ID | `parts-tracker` |
| Repository | `https://github.com/matipongpa/parts-tracker-backend` |
| Compose file | `docker-compose.prod.yml` |
| Extra repo | folder `parts-tracker-ui`, `https://github.com/matipongpa/parts-tracker-ui` |
| App URL | `http://<PC address>:8082` |
| Env file | `.env`, with at least `JWT_SECRET`, `POSTGRES_PASSWORD` (`openssl rand -hex 24`), and `COOKIE_SECURE=false` while served over plain HTTP; add `REGISTER_INVITE_CODE` and the AI settings as needed |

Both repos must be pushed first: the manager deploys what is on the remote branch. `DB_URL`, `PORT` and `RUN_MIGRATIONS` are set by the compose file and override `.env`. Over HTTPS (Tailscale or a TLS proxy) drop `COOKIE_SECURE=false`.

## Status and roadmap

Done: platform and identity, parts module, search cache, BOM import with column mapping (preview, saved layouts) and duplicate skipping, part-usage lookup, AI assistant (`/api/ask`) and MCP server over read-only tools.

Planned (see the design doc):

- Asynchronous imports: NATS JetStream with a job table as the source of truth.
- Completion notifications to the user over SSE.
- CI architecture check, Dockerfile/compose cleanup (move secrets to `.env`).
