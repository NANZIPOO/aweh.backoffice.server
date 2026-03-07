# Linux Docker Deployment

## 1) Clone and prepare

```bash
git clone https://github.com/NANZIPOO/aweh.backoffice.server.git
cd aweh.backoffice.server
cp .env.example .env
```

Edit `.env` and set:
- `DB_HOST` = Firebird server IP/DNS
- `DB_PORT` = `3050`
- `DB_PATH` = absolute Linux path on Firebird server (example `/var/lib/firebird/data/dinem.fdb`)
- `DB_USER`, `DB_PASS`, `JWT_SECRET`

## 2) Run deployment app

```bash
chmod +x deploy/deploy.sh
./deploy/deploy.sh install
```

## 3) Operations

```bash
./deploy/deploy.sh status
./deploy/deploy.sh logs
./deploy/deploy.sh update
./deploy/deploy.sh restart
./deploy/deploy.sh down
```

# Aweh POS — Go Gateway Server

> **Stack:** Go 1.22 · `net/http` stdlib router · `sqlx` · `nakagami/firebirdsql` · `golang-jwt/jwt v5`
> **Database:** Firebird 3.0 (`dinem.fdb`) on `localhost:3050`
> **Listens on:** `:8081` (port 8080 is permanently occupied by Windows `svchost`)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Repository Structure](#2-repository-structure)
3. [Configuration](#3-configuration)
   - [FirebirdDSN](#firebirddsn)
   - [Database Migrations](#35-database-migrations-auto-setup)
4. [How to Build & Run](#4-how-to-build--run)
5. [Authentication & JWT](#5-authentication--jwt)
6. [Access Level System](#6-access-level-system)
7. [Database Connection Architecture](#7-database-connection-architecture)
8. [API Endpoint Reference](#8-api-endpoint-reference)
9. [Error Response Format](#9-error-response-format)
10. [Mandatory Write Sequence (DB Rule)](#10-mandatory-write-sequence-db-rule)
11. [Startup Sequence & Self-Healing](#11-startup-sequence--self-healing)
12. [Known Issues Log & Fixes Applied](#12-known-issues-log--fixes-applied)

---

## 1. Overview

The Go Gateway is a **multi-tenant HTTP API** that acts as the single backend for the Flutter POS frontend. It translates incoming JSON REST calls into direct SQL operations against the legacy Firebird 3 database (`dinem.fdb`), preserving all original business logic 1:1.

**Design rules:**
- No complex `INNER JOINs` — relationships are resolved in Go, not SQL.
- No ORM — direct `sqlx` struct scanning from Firebird tables.
- Tenant isolation at the connection level (one `*sqlx.DB` pool per tenant).
- All writes follow the mandatory DB sequence: `BEGIN TX → GEN_ID → business logic → INSERT/UPDATE → COMMIT`.

---

## 2. Repository Structure

```
gateway/
├── main.go                         # Entry point: wires all routes, starts server
├── go.mod / go.sum
├── pos-gateway.exe                 # Compiled binary (Windows)
└── internal/
    ├── config/
    │   └── config.go               # Env-var config + FirebirdDSN() builder
    ├── middleware/
    │   ├── auth.go                 # JWT middleware, Claims struct, GenerateToken
    │   └── context_keys.go         # Typed context key constants
    ├── models/
    │   ├── models.go               # Employee, SaleItem, Firebird generator names
    │   ├── bill.go                 # Bill, CreateBillRequest, CloseBillRequest, etc.
    │   ├── purchase_order.go       # PO, OrderLine, Supplier structs
    │   ├── api_purchase_order.go   # API request/response types for PO module
    │   ├── inventory.go            # InventoryItem, StockSheet, GRV, Wastage structs
    │   ├── company.go              # Company struct
    │   └── common.go               # APIError, shared helpers
    ├── repository/
    │   ├── base.go                 # TenantManager, PingTenant, BaseRepository
    │   ├── employee_repo.go        # EMPLOYEE table queries
    │   ├── bill_repo.go            # BILLS / SALEITEMS queries
    │   ├── purchase_order_repo.go  # EXPENSES / PITEMS / SUPPLIERS queries
    │   ├── inventory_repository.go # InventoryRepository interface
    │   ├── inventory_firebird.go   # Inventory CRUD + reports implementation
    │   ├── lookup_firebird.go      # Stock sheets, bins, categories lookups
    │   ├── stock_take_firebird.go  # Closing stock, period finalization
    │   ├── grv_firebird.go         # Goods Received Voucher
    │   ├── wastage_firebird.go     # Wastage recording & posting
    │   └── company_repo.go         # COMPANY table queries
    ├── handler/
        ├── inventory_handler.go    # InventoryHandler HTTP handlers
        ├── respond.go              # JSON()/Err() response helpers used by all handlers
        └── routes.go               # RegisterInventoryRoutes wiring
```

---

## 3. Configuration

All configuration is driven by **environment variables**. If a variable is not set, the safe local-dev default is used automatically.

| Env Var | Default | Description |
|---|---|---|
| `DB_HOST` | `localhost` | Firebird server hostname or IP |
| `DB_PORT` | `3050` | Firebird **3.0** instance port (**not** 3055 which is FB5) |
| `DB_PATH` | `c:/Users/herna/aweh.pos/dinem.fdb` | Absolute path to the `.fdb` file on the Firebird server |
| `DB_USER` | `SYSDBA` | Firebird superuser |
| `DB_PASS` | `profes` | SYSDBA password for this installation |
| `AUTH_PLUGIN` | `Srp256` | Authentication method: `Srp256` (modern) or `Legacy_Auth` (older servers) |
| `WIRE_CRYPT` | `true` | Wire encryption: `true` (encrypted connection) or `false` (unencrypted) |
| `JWT_SECRET` | `your-secret-key` | HS256 signing secret — **change in production** |
| `PORT` | `8081` | HTTP listen port for the gateway |
| `AUTO_MIGRATE` | `false` | Auto-apply migrations on startup: `true` or `false` |

### FirebirdDSN

The DSN is constructed dynamically from environment variables:

```
SYSDBA:profes@192.168.0.152:3050/var/lib/firebird/3.0/data/aweh_test/dinem.fdb?auth_plugin_name=Srp256&wire_crypt=true
```

**Authentication Options:**
- `auth_plugin_name=Srp256` — Modern SRP-based authentication (Firebird 3.0+ with SRP enabled)
- `auth_plugin_name=Legacy_Auth` — Legacy password hash authentication (older Firebird installations or custom configs)

**Wire Encryption:**
- `wire_crypt=true` — Encrypted connection (recommended for production)
- `wire_crypt=false` — Unencrypted connection (dev/testing only)

Set these via environment variables (`AUTH_PLUGIN`, `WIRE_CRYPT`) to match your Firebird server's configuration.

---

## 3.5 Database Migrations (Auto-Setup)

The gateway includes an **automatic schema migration system** that runs on startup. It is designed to be idempotent and side-effect-free for production environments.

### How It Works

1. **Startup Phase**: After the DB ping succeeds, the migrator runs (if `AUTO_MIGRATE=true`).
2. **Tracking Table**: A table called `RDB$MIGRATIONS` is created to track which migrations have been applied.
3. **File Discovery**: All `.sql` files in the `migrations/` folder are discovered and sorted by filename (e.g., `001_*.sql`, `002_*.sql`, etc.).
4. **Idempotent Execution**: For each file, the migrator checks if it's already in the tracking table. If yes, it skips it. If no, it executes all SQL statements and records the migration.
5. **Transaction Safety**: Each migration runs within a Firebird transaction. If any statement fails, the entire migration is rolled back and the server exits with an error.

### Configuration

Set `AUTO_MIGRATE` in your `.env` file:

```bash
# Enable automatic schema migrations on startup
AUTO_MIGRATE=true     # Run pending migrations on boot
AUTO_MIGRATE=false    # Skip migrations (manual application)
```

**Safe defaults:**
- **Development**: Set `AUTO_MIGRATE=true` to automatically seed new tables and schema changes.
- **Production**: Set `AUTO_MIGRATE=false` (default). Review and apply migrations manually via other tools.

### Adding New Migrations

To add a new migration:

1. Create a new `.sql` file in `gateway/migrations/` with a sequential numeric prefix (e.g., `010_add_my_column.sql`).
2. Write standard Firebird SQL (`CREATE TABLE`, `ALTER TABLE`, etc.).
3. On next deployment with `AUTO_MIGRATE=true`, the system will automatically detect and apply it.
4. The migration state is persisted in `RDB$MIGRATIONS`, so re-deploying the same image will skip already-applied migrations.

### Example Migration File

**`gateway/migrations/010_add_my_column.sql`:**
```sql
ALTER TABLE DMASTER ADD MY_NEW_COLUMN VARCHAR(100);
COMMIT;
```

On startup, if `AUTO_MIGRATE=true`:
```
2026/03/07 10:15:22 migrations: Starting automatic migration runner...
2026/03/07 10:15:22 migrations: RDB$MIGRATIONS tracking table created
2026/03/07 10:15:22 migrations: Found 10 migration file(s)
2026/03/07 10:15:22 migrations: 001_create_generators.sql already applied, skipping
...
2026/03/07 10:15:22 migrations: Applying 010_add_my_column.sql...
2026/03/07 10:15:22 migrations: ✓ 010_add_my_column.sql applied successfully
2026/03/07 10:15:22 migrations: All pending migrations applied successfully
```

---

## 4. How to Build & Run

### Build the binary

```powershell
cd gateway
go build -o pos-gateway.exe .
```

### Run

```powershell
.\pos-gateway.exe
```

On startup you will see:
```
2026/03/06 00:36:56 DB connection OK — SYSDBA:profes@localhost:3050/...?auth_plugin_name=Legacy_Auth&wire_crypt=false
2026/03/06 00:36:57 Aweh POS Gateway starting on :8081...
```

### Development (no binary, live reload via `go run`)

```powershell
cd gateway
go run main.go
```

### Verify the server is up

```powershell
# Company info (no auth required)
Invoke-RestMethod -Uri "http://localhost:8081/api/v1/company"
# Expected: { "company_name": "St. George's Cafe", "status": "ok" }

# Login to get a JWT
Invoke-RestMethod -Method POST -Uri "http://localhost:8081/login" `
  -ContentType "application/json" `
  -Body '{"tenant_id":"tenant_test_001","user_no":1}'
```

---

## 5. Authentication & JWT

All protected endpoints require a `Bearer` token in the `Authorization` header.

### Obtaining a Token

**`POST /login`** — No auth required.

Request:
```json
{
  "tenant_id": "tenant_test_001",
  "user_no": 1
}
```

Response:
```json
{
  "token": "<jwt>"
}
```

### JWT Claims

```go
type Claims struct {
    TenantID    string `json:"tenant_id"`
    UserNo      int16  `json:"user_no"`
    UserID      int64  `json:"user_id"`
    Username    string `json:"username"`
    AccessLevel int    `json:"access_level"`
    jwt.RegisteredClaims  // ExpiresAt: 24h from issue
}
```

- **Legacy endpoints** (bills, employee, PO) use `tenant_id` + `user_no`.
- **Inventory endpoints** use `access_level` for RBAC in addition to `tenant_id`.

### Two token generators

| Function | Use case |
|---|---|
| `GenerateToken(tenantID, userNo, secret)` | Legacy login — fills `tenant_id` + `user_no` only |
| `GenerateFullToken(tenantID, userNo, userID, username, accessLevel, secret)` | Full token — fills all fields including `access_level` |

---

## 6. Access Level System

Inventory endpoints enforce a numeric access level extracted from the JWT. The levels map to operation sensitivity:

| Level | Permitted operations |
|---|---|
| `≥ 1` | All `GET` (read-only) endpoints, lookups |
| `≥ 2` | Update closing stock, record wastage |
| `≥ 3` | Create/update inventory items, clone, assign barcode, create GRV |
| `≥ 5` | Delete items, finalize stock period, post pending wastage |

Enforcement is applied by the `RequireLevel(n)` middleware, chained after `AuthMiddleware`. Legacy endpoints (bills, PO) do not use level gating.

---

## 7. Database Connection Architecture

### TenantManager

The `TenantManager` (`internal/repository/base.go`) owns all Firebird connection pools.

```
TenantManager
  ├── configs  map[tenantID] → DSN string   (registered at startup)
  └── conns    map[tenantID] → *sqlx.DB      (opened lazily on first request)
```

- `RegisterTenantDB(tenantID, dsn)` — registers a DSN at startup; no connection is opened yet.
- `PingTenant(tenantID)` — opens the pool eagerly and pings; called at startup to **fail fast** if the DB is unreachable.
- `GetDB(ctx)` — extracts `tenant_id` from JWT context, opens the pool if not yet cached, returns the live `*sqlx.DB`.

Connection pool settings per tenant:
- `MaxOpenConns: 10`
- `MaxIdleConns: 5`

### Firebird generators (sequences)

All IDs come from Firebird generators, never from Go code or auto-increment:

| Constant | Generator name | Used for |
|---|---|---|
| `GenEmployee` | `employee_gen` | EMPLOYEE.USERNO |
| `GenBills` | `bills_gen` | BILLS.CHECKNO |
| `GenOrders` | `ORDERS_GEN` | EXPENSES.ORDERNO |
| `GenOrderItems` | `LINE_ORDERNO_GEN` | PITEMS.ITEMNO |
| `GenSuppliers` | `suppliers_gen` | SUPPLIERS.ITEMNO |

---

## 8. API Endpoint Reference

### Public

| Method | Path | Description |
|---|---|---|
| `POST` | `/login` | Obtain a JWT token |

### Employee

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/employee` | ✅ | Get current user's EMPLOYEE record |

### Bills

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/bills?date=YYYY-MM-DD` | ✅ | List all bills for a business day |
| `GET` | `/bills/{checkno}` | ✅ | Get single bill by CHECKNO |
| `POST` | `/bills` | ✅ | Open a new bill (GEN_ID assigned) |
| `PATCH` | `/bills/{checkno}/close` | ✅ | Settle a bill (money path) |
| `PATCH` | `/bills/{checkno}/cashup` | ✅ | Mark a closed bill as cashed-up |
| `PATCH` | `/bills/{checkno}/void` | ✅ | Void an open bill |

### Purchase Orders

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/purchase-orders?supplier_no={s}` | ✅ | List orders (optionally filter by supplier) |
| `GET` | `/api/v1/purchase-orders/{order_no}` | ✅ | Get full order with lines |
| `POST` | `/api/v1/purchase-orders` | ✅ | Create new order |
| `DELETE` | `/api/v1/purchase-orders/{order_no}` | ✅ | Delete a draft order |
| `GET` | `/api/v1/purchase-orders/{order_no}/lines` | ✅ | Get order lines |
| `POST` | `/api/v1/purchase-orders/{order_no}/lines` | ✅ | Add a line item |
| `PUT` | `/api/v1/purchase-orders/{order_no}/lines/{item_no}` | ✅ | Update a line item |
| `DELETE` | `/api/v1/purchase-orders/{order_no}/lines/{item_no}` | ✅ | Remove a line item |
| `GET` | `/api/v1/purchase-orders/{order_no}/totals` | ✅ | Get order totals |
| `POST` | `/api/v1/purchase-orders/{order_no}/capture-invoice` | ✅ | Capture supplier invoice |
| `POST` | `/api/v1/purchase-orders/{order_no}/post` | ✅ | Post invoice to ledger |
| `PUT` | `/api/v1/purchase-orders/{order_no}/update-costs` | ✅ | Update inventory costs |

### Suppliers

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/suppliers` | ✅ | List all suppliers |
| `GET` | `/api/v1/suppliers/{supplier_no}/items` | ✅ | Get supplier's linked items |

### Inventory

| Method | Path | Level | Description |
|---|---|---|---|
| `GET` | `/api/v1/inventory/lookups` | ≥ 1 | Stock sheets, bins, categories, cost categories, suppliers |
| `GET` | `/api/v1/inventory/items` | ≥ 1 | List inventory items |
| `GET` | `/api/v1/inventory/items/{id}` | ≥ 1 | Get single item |
| `POST` | `/api/v1/inventory/items` | ≥ 3 | Create item |
| `PUT` | `/api/v1/inventory/items/{id}` | ≥ 3 | Update item |
| `DELETE` | `/api/v1/inventory/items/{id}` | ≥ 5 | Delete item |
| `POST` | `/api/v1/inventory/items/{id}/clone` | ≥ 3 | Clone item |
| `POST` | `/api/v1/inventory/items/{id}/barcode` | ≥ 3 | Assign primary barcode |
| `POST` | `/api/v1/inventory/items/{id}/barcode/bulk` | ≥ 3 | Add linked barcode |
| `GET` | `/api/v1/inventory/stock-take` | ≥ 1 | Get stock take sheet |
| `PUT` | `/api/v1/inventory/stock-take/closing` | ≥ 2 | Update closing stock counts |
| `POST` | `/api/v1/inventory/stock-take/finalize` | ≥ 5 | Finalize stock period |
| `POST` | `/api/v1/inventory/grv` | ≥ 3 | Create Goods Received Voucher |
| `POST` | `/api/v1/inventory/wastage` | ≥ 2 | Record wastage |
| `POST` | `/api/v1/inventory/wastage/post-pending` | ≥ 5 | Post all pending wastage |
| `GET` | `/api/v1/inventory/reports/value` | ≥ 1 | Inventory value report |
| `GET` | `/api/v1/inventory/reports/variance` | ≥ 1 | Stock variance report |
| `GET` | `/api/v1/inventory/search?mpart_no={p}` | ✅ | Search item by manufacturer part no |

---

## 9. Error Response Format

Inventory endpoints return structured errors:

```json
{
  "error": "ERR_NOT_FOUND",
  "message": "item 99 not found"
}
```

Legacy endpoints (bills, PO) return plain-text `http.Error` responses with the appropriate HTTP status code.

### HTTP Status Codes in use

| Code | Meaning |
|---|---|
| `200 OK` | Successful GET / PATCH |
| `201 Created` | Successful POST (new resource) |
| `204 No Content` | Successful DELETE |
| `400 Bad Request` | Invalid input / missing required field |
| `401 Unauthorized` | Missing or invalid JWT |
| `403 Forbidden` | Access level too low |
| `404 Not Found` | Resource does not exist |
| `409 Conflict` | Business rule violation (already posted, cannot void, etc.) |
| `422 Unprocessable` | Semantic error (zero pack size, pay reference required) |
| `500 Internal Server Error` | Unexpected DB or server error |

---

## 10. Mandatory Write Sequence (DB Rule)

Every write operation **must** follow this sequence to preserve Firebird generator integrity:

```
1. BeginTxx()                          — start explicit transaction
2. SELECT GEN_ID(gen_name, 1)          — fetch next ID from Firebird generator
   FROM RDB$DATABASE
3. Apply business logic / struct mapping in Go
4. INSERT / UPDATE with the fetched ID
5. tx.Commit()                         — commit; tx.Rollback() on any error
```

Never use Go-generated UUIDs or auto-increment for primary keys. Firebird generators are the single source of truth for IDs.

---

## 11. Startup Sequence & Self-Healing

When `pos-gateway.exe` starts, it executes the following in order:

```
1. config.Load()              — read env vars / apply defaults
2. freePort("8080")           — kill any stale gateway process holding :8080
3. TenantManager.Register()   — register tenant DSN(s)
4. TenantManager.PingTenant() — open pool + ping DB → fatal if unreachable
5. Initialise repositories    — Employee, Bill, PO, Inventory, etc.
6. Register HTTP routes       — all handlers wired to http.DefaultServeMux
7. http.ListenAndServe(":8080")
```

### freePort behaviour

`freePort` runs before `ListenAndServe` every time. On Windows it:

1. Runs `netstat -ano` to find the PID listening on `:8080`.
2. Resolves the process name via PowerShell `Get-Process`.
3. **Only kills** `pos-gateway.exe` or `go.exe` (development mode). System processes (`svchost`, etc.) are **explicitly skipped** and logged.
4. Waits 400 ms for the OS to release the socket before binding.

This means you can always just run `.\pos-gateway.exe` to restart — no manual task-killing required.

---

## 12. Known Issues Log & Fixes Applied

| Date | Issue | Root Cause | Fix Applied |
|---|---|---|---|
| 2026-03-05 | `listen tcp :8080: bind: Only one usage of each socket address` | A stale `pos-gateway.exe` process was left running from a previous session | Added `freePort("8080")` at startup — auto-kills the old gateway process before binding |
| 2026-03-05 | `freePort` was killing `svchost.exe` (Windows system process) | Original implementation killed every PID on :8080 without checking the process name | Fixed: `freePort` now resolves process name via `Get-Process` and only kills `pos-gateway.exe` / `go.exe` |
| 2026-03-06 | `DB ping failed: connect: Error op_response:92` | Wrong default password (`masterkey`) and driver trying SRP wire auth against a `Legacy_UserManager` FB3 instance | 1. Password updated to `profes`. 2. DSN now appends `?auth_plugin_name=Legacy_Auth&wire_crypt=false` |
| 2026-03-06 | Silent startup failures — no indication if DB was reachable | Connection was lazy (opened on first request); server started with exit code 0 even when DB was down | Added `PingTenant()` call immediately after `RegisterTenantDB` — gateway now **fails fast** at boot with a clear error message |
| 2026-03-06 | `GET /api/v1/company` returned `tenant_id not found in context` | Endpoint is pre-auth (no JWT middleware), so `GetDB()` couldn't find tenant_id from context | Manually inject `tenant_id = "tenant_test_001"` into context at the top of the handler before calling the repo |
| 2026-03-06 | `GET /api/v1/company` returned `SQL error -104 Token unknown` | `LIMIT 1` is MySQL syntax; Firebird uses `ROWS 1` | Fixed `company_repo.go` query to use `ROWS 1` |
| 2026-03-06 | `GET /api/v1/company` returned `SQL error -204 Table unknown COMPANY` | Table is named `COMPANYINFO` in this database, not `COMPANY` | Updated `Company` model struct and `company_repo.go` query to map `COMPANYINFO` with correct column names (`COMPANYNO`, `PHONENO`, `ADDRESS`, `EMAILADDRESS`, etc.) |
| 2026-03-06 | Dead code in `handler/auth_handler.go` | `AuthHandler` struct and `Login` method were never registered or instantiated — login is handled inline in `main.go` | Deleted `auth_handler.go`; removed `RespondJSON`/`RespondError` aliases from `respond.go` that only existed for it |
| 2026-03-06 | Hardcoded `masterkey` in `/api/v1/company` dynamic DSN override | Copy-paste remnant from original scaffold | Fixed to use `cfg.DBUser`/`cfg.DBPass` with correct `Legacy_Auth` params |
| 2026-03-06 | Port hardcoded as `:8080` in log message but gateway actually ran on `:8081` | Config `Port` field was not plumbed into `ListenAndServe` or log | `ListenAndServe` and startup log now use `cfg.Port`; config default updated to `8081` |

---

*Last updated: 2026-03-06 — gateway verified live, returning real data from `COMPANYINFO` (St. George's Cafe)*
