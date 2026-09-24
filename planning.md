# WalletX Auth and Categories Master Plan

**Status:** Execution plan

**Primary standard:** Test-Driven Development (TDD)

This document is the single implementation plan for the `auth` and `categories`
modules. No production behavior is considered complete until its repository,
service, and handler tests have been written and passed.

## 1. Architecture and Technology Stack

### Repository shape

WalletX is a Go monorepo with a layered internal architecture:

- `cmd/server` and `api` are application entry points.
- `internal/app` owns composition, dependency wiring, and route registration.
- `internal/modules/auth` and `internal/modules/categories` contain each
  module's models, DTOs, repository, service, handler, routes, and tests.
- `internal/platform/database` owns the PostgreSQL connection pool, SQL source,
  and generated `sqlc` package.
- `internal/middleware` owns JWT authentication, recovery, CORS, and request
  logging.
- `internal/shared` owns response formatting, common errors, request helpers,
  and pagination.

### Non-negotiable technology decisions

- Go is the implementation language.
- Gin is the HTTP framework and route layer.
- PostgreSQL runs locally through Docker/Supabase containers.
- `pgx/v5` is the PostgreSQL driver and connection/pool implementation.
- `sqlc` is the only database query code generator and the only persistence
  access pattern. There is no ORM and no hand-written data mapper that bypasses
  generated queries.
- SQL migrations in `supabase/migrations` and query files in
  `internal/platform/database/queries` are the database source of truth.
- Generated `sqlc` files are never edited manually; run `sqlc generate` after
  changing migrations or query files.
- `go-playground/validator` through Gin binding validates request shape. The
  service layer repeats business-critical validation so business rules do not
  depend solely on HTTP binding.
- `github.com/golang-jwt/jwt/v5` creates and validates internal JWTs.
- Google OAuth/ID-token verification is isolated behind an injectable provider.
- Logrus is accessed only through the repository's logger abstraction. Logger
  dependencies are injected into components that need them.

### Dependency boundaries

- Handlers depend on services, never on repositories or `sqlc` directly.
- Services depend on repository interfaces and external-provider interfaces,
  never on Gin or concrete database implementations.
- Repositories depend on generated `sqlc` queries and `pgx`.
- Tests replace every dependency at the boundary under test.
- `user_id` for protected operations comes from verified JWT context, never from
  a client-controlled request field.

## 2. Module Specifications

## 2.1 Auth Module

### Responsibility

Auth authenticates a user with Google, creates or updates the corresponding
WalletX user, issues an internal JWT, and supports logout/token revocation where
the existing authentication design requires it.

### Google authentication flow

1. The client submits a Google ID token to `POST /api/v1/auth/google`, or uses
   the development OAuth helper flow when `DEV_MODE=true`.
2. The handler validates the request envelope and passes the token/code to the
   service.
3. The service asks an injected Google verifier/provider to validate the token
   with Google. The raw Google token must never be logged or returned.
4. The service validates the verified identity data required by WalletX:
   Google subject ID, email, display name, and optional picture.
5. The repository finds the user by `google_id` and creates the user when absent,
   or updates permitted profile fields when the user already exists.
6. The service creates a signed internal JWT containing the user identity and
   expiration data. Google tokens are not used as WalletX API tokens.
7. The handler returns the internal JWT and safe user information.
8. Protected requests use the JWT middleware. Missing, invalid, expired, or
   revoked tokens return `401 Unauthorized`.
9. Logout revokes the current token/JTI when revocation is enabled and returns a
   successful response without exposing token details.

### Users persistence

The `users` table is the system of record for authenticated identities. It must
enforce:

- `id` as the UUID primary key.
- Unique `google_id`.
- Unique `email`.
- Required `name`.
- Optional `picture`.
- `created_at`, `updated_at`, and optional `deleted_at` timestamps.

Repository operations must cover lookup by Google ID, lookup by ID where
needed, insert/upsert of an authenticated user, profile updates where required,
and soft deletion only if the account-management contract calls for it. Unique
violations and missing rows must be converted to application errors at the
repository boundary.

### Auth endpoints

| Method | Path | Authentication | Contract |
|---|---|---|---|
| `POST` | `/api/v1/auth/google` | None | Verify Google identity, persist user, return internal JWT and safe user data. |
| `GET` | `/api/v1/auth/google/test-login` | None, development only | Start or simulate the local OAuth flow only when `DEV_MODE=true`. |
| `GET` | `/api/v1/auth/google/callback` | None, development/provider callback | Complete the configured OAuth callback and issue the internal session result. |
| `POST` | `/api/v1/auth/logout` | JWT | Revoke the current token/JTI when configured; do not log the token. |

Auth error expectations:

- Invalid/missing request data or unverifiable Google identity: `400` or `401`
  according to the established API error contract.
- Missing, invalid, expired, or revoked internal JWT: `401`.
- Database/provider failure: generic `500` with internal details only in logs.
- Duplicate identity conflicts: mapped to the defined conflict response rather
  than leaking PostgreSQL errors.

JWT tests must verify signing algorithm, required user claims, JTI behavior,
expiration, invalid signatures, expired tokens, malformed claims, and blacklist
behavior. Tests must never assert or print a real secret or token in logs.

## 2.2 Categories Module

### Responsibility and data rules

Categories belong to one user and classify transactions. Every category has a
mandatory transaction `type`:

- `expense` for money leaving the account.
- `income` for money entering the account.

Database invariants:

- `type` is non-null and restricted to `expense` or `income` by a check
  constraint.
- `(user_id, name, type)` is unique.
- The previous `(user_id, name)` uniqueness rule must not remain, because the
  same name is valid once for each type.
- `user_id` references `users(id)` with the existing ownership/delete behavior.
- An index on `(user_id, type)` supports filtered listing.
- Existing category rows, if any, are backfilled to the agreed default
  (`expense`) before the column becomes mandatory.

Service rules:

- Trim category names and reject empty names.
- Enforce the maximum name length used by the API, currently 100 characters.
- Trim and lowercase `type` before persistence.
- Accept only `expense` and `income`.
- Do not perform a pre-check as a substitute for the database unique
  constraint; concurrent duplicate creates must still be handled safely.
- Preserve ownership on every read, update, and delete.
- Updating a category may change its name and type, but the target combination
  must remain unique for that user.

### HTTP endpoints

| Method | Path | Authentication | Success |
|---|---|---|---|
| `POST` | `/api/v1/categories` | JWT | `201 Created` with the created category. |
| `GET` | `/api/v1/categories` | JWT | `200 OK`; optional `?type=expense` or `?type=income`. |
| `PUT` | `/api/v1/categories/:id` | JWT | `200 OK` with the updated category. |
| `DELETE` | `/api/v1/categories/:id` | JWT | `204 No Content`. |

Request bodies contain only `name` and `type`; clients cannot submit `user_id`,
timestamps, or ownership fields. Unknown fields should be rejected where the
handler's JSON policy supports strict decoding.

Category response fields are `id`, `name`, `type`, `created_at`, and
`updated_at`. Expected error mapping:

- Invalid/missing name, invalid/missing type, invalid UUID, malformed JSON, or
  invalid `type` filter: `400`.
- Missing/invalid/expired/revoked JWT: `401`.
- Category absent or owned by another user: `404`.
- Duplicate `(user_id, name, type)`: `409`.
- Unexpected database failure: `500`.

## 3. TDD Execution Strategy

### Required testing stack

- `testing` for test execution and subtests.
- `github.com/stretchr/testify/assert` for non-fatal value and state checks.
- `github.com/stretchr/testify/require` for fatal setup, dependency, and
  precondition checks.
- `github.com/stretchr/testify/mock` for service and handler dependency mocks
  where a mock provides useful call/argument assertions.
- `net/http/httptest` for Gin handler and middleware HTTP tests.
- Local PostgreSQL in Docker/Supabase for repository integration tests.
- Real migrations and real generated `sqlc` queries in repository tests; do not
  replace the database with a fake in the repository test suite.

Use table-driven tests for validation and error variants. Keep tests isolated:
each integration test owns its fixtures and cleans up or runs inside a rollback
transaction. Tests must not depend on execution order or on shared user IDs.

### The mandatory Red-Green-Refactor cycle

For every behavior, follow this exact sequence:

1. **Red:** Write one focused test describing one observable behavior. Run the
   smallest relevant test command and confirm it fails for the expected reason.
2. **Green:** Implement the smallest production change that makes the test pass.
   Do not implement untested speculative behavior.
3. **Refactor:** Improve naming, duplication, boundaries, or error mapping while
   keeping the test suite green. Rerun the focused test after the refactor.
4. Run the complete module tests before starting the next behavior.

A test is not considered useful merely because it executes. It must assert the
returned result, error classification, HTTP status/body, database state, or
dependency interaction that represents the contract.

### Phase 1: Repository layer, integration-first

Repository tests run against the local PostgreSQL container using the real
migrations, `pgx`, and generated `sqlc` package. Start the database before the
test run, apply/reset migrations deterministically, and use a dedicated test
database or isolated schema. Never use production credentials or a committed
connection string.

#### Auth repository test sequence

1. Create a user with a valid Google ID and email; assert all persisted fields.
2. Find the user by `google_id`; assert the same identity is returned.
3. Upsert an existing Google identity; assert no duplicate user is created and
   permitted profile fields are updated according to the contract.
4. Attempt duplicate `google_id`; assert the repository returns the application
   conflict error and does not leak raw SQL details.
5. Attempt duplicate email with a different Google ID; assert the same conflict
   behavior.
6. Query a missing identity; assert the application `not found` error.
7. Test user update/soft-delete operations that are part of the implemented
   Auth contract, including affected-row handling.
8. If token revocation persistence is implemented in this module, test blacklist
   insert, active blacklist lookup, expired/missing JTI behavior, and database
   failure mapping.

#### Categories repository test sequence

1. Insert a category with `expense`; assert `type` and timestamps are returned.
2. Insert a category with `income` for the same user and same name; assert it is
   allowed.
3. Insert the same name and same type for the same user; assert PostgreSQL's
   unique constraint is reached and mapped to `ErrConflict`/the project error.
4. Insert the same name and type for a different user; assert it is allowed.
5. Insert an invalid type directly through the repository; assert the database
   check constraint failure is mapped to invalid input.
6. List without a type filter; assert both types are returned only for the
   requested user.
7. List with `expense`; assert income rows are excluded.
8. List with `income`; assert expense rows are excluded.
9. Update name and type atomically; assert the returned row and persisted row
   contain both changes.
10. Update into an existing `(user_id, name, type)` combination; assert
    conflict mapping.
11. Update or delete another user's category using the first user's ID; assert
    `not found` and no cross-user mutation.
12. Update/delete a missing category; assert `not found`.
13. Verify delete behavior remains compatible with transaction foreign keys,
    including `ON DELETE SET NULL` if transaction fixtures are available.

### Phase 2: Service layer, isolated unit tests

Service tests use mocked repositories and mocked external providers. They do not
open PostgreSQL, create Gin contexts, or call Google. The goal is business logic
and orchestration, not SQL behavior.

#### Auth service test sequence

1. Valid Google identity creates a user and returns a signed internal JWT.
2. Existing Google identity reuses/updates the user without creating a second
   record.
3. Missing required Google identity fields are rejected before persistence.
4. Google verification failure is returned without a repository call.
5. Repository failure is returned with the correct application classification.
6. JWT contains the expected user identity, JTI policy, and expiration.
7. JWT signing failure is returned and no successful auth response is produced.
8. Logout delegates revocation with the current token/JTI and propagates
   failures correctly.
9. Sensitive token and Google identity values are not passed to logger calls.

#### Categories service test sequence

1. Create trims a valid name and normalizes `EXPENSE` to `expense`.
2. Create accepts `income` and forwards normalized values to the repository.
3. Create rejects blank names, names over the limit, and invalid types without
   calling the repository.
4. Create propagates repository conflict as a conflict.
5. List with no filter passes `nil`/no filter and returns all owned categories.
6. List with each valid filter passes the normalized type.
7. List rejects an invalid filter before the repository call.
8. Update validates and normalizes both fields before calling the repository.
9. Update propagates duplicate-target conflicts and not-found errors.
10. Delete passes both authenticated user ID and category ID, preserving
    ownership, and propagates repository errors.
11. Service tests confirm no repository call occurs after invalid input.

### Phase 3: Handler layer, HTTP tests

Handler tests construct a Gin router with mocked services and use
`httptest.NewRequest`/`httptest.NewRecorder`. They verify the public HTTP
contract, not repository details. Authentication middleware is either tested
separately with its own unit tests or replaced with a controlled test middleware
that injects a known user ID.

#### Auth handler and middleware test sequence

1. Google login accepts a valid request and returns the service result with the
   expected success status and safe response shape.
2. Malformed JSON, missing token, and unknown fields return `400` where the
   contract requires it.
3. Google verification/service failure maps to the correct public status without
   exposing internal error text.
4. Logout without a bearer token returns `401` and does not call the service.
5. Logout with a valid authenticated context returns the expected success.
6. JWT middleware rejects missing, malformed, expired, invalid-signature, and
   revoked tokens.
7. JWT middleware injects the verified user ID and claims for downstream routes.
8. Development-only OAuth helper routes are unavailable when `DEV_MODE=false`.

#### Categories handler test sequence

1. `POST` with valid `name` and `type` returns `201` and forwards the JWT user
   ID, never a request user ID.
2. `POST` rejects missing/invalid type, blank/overlong name, malformed JSON,
   and forbidden unknown fields with `400`.
3. `POST` maps service conflict to `409` and database failure to `500`.
4. `GET` without a filter returns all categories for the authenticated user.
5. `GET?type=expense` and `GET?type=income` forward the appropriate filter.
6. `GET?type=invalid` returns `400` without a service call.
7. `PUT` validates UUID, body, ownership context, and maps success to `200`.
8. `PUT` maps not found to `404` and duplicate target to `409`.
9. `DELETE` returns `204` and passes authenticated user ID plus route UUID.
10. `DELETE` maps missing or foreign categories to `404`.
11. All protected category endpoints reject missing/invalid JWT with `401`.
12. Responses never expose internal database errors, credentials, tokens, or
    unnecessary personal data.

### Cross-layer completion gates

Before moving from one layer to the next:

- The current layer's focused tests are green.
- Error types are stable enough for the next layer to map them.
- Interfaces expose only the behavior needed by the caller.
- SQL changes have been regenerated through `sqlc`.
- No test relies on an implementation detail that the next layer should own.

## 4. Ordered Execution Roadmap

The following order is strict. Do not begin a later item while the required
earlier tests are red or missing.

### Priority 0: Test and database foundation

1. Confirm Docker/Supabase PostgreSQL starts locally and the test connection is
   isolated from development data.
2. Confirm the migration contains `users`, `categories`, the category type check,
   `(user_id, name, type)` uniqueness, and the required indexes/foreign keys.
3. Establish test helpers for database setup, cleanup, deterministic fixtures,
   UUIDs, and a no-op injected logger.
4. Establish mock/provider test helpers without introducing production code just
   for test convenience.
5. Define shared application error categories: invalid input, not found,
   conflict, unauthorized, and internal failure.

### Priority 1: Auth repository

1. Write the first failing integration test: create and retrieve a user by
   `google_id`.
2. Implement the minimum SQL query and repository method; run `sqlc generate`.
3. Add Red-Green-Refactor tests for existing-user upsert/update behavior.
4. Add duplicate Google ID and duplicate email constraint tests.
5. Add missing-user and database-error mapping tests.
6. Complete repository cleanup and run the full Auth repository integration
   suite against the Docker PostgreSQL instance.

### Priority 2: Auth service

1. Define the injectable Google verifier and JWT signer/generator boundaries.
2. Write the failing test for valid Google identity to persisted user to JWT.
3. Implement the smallest orchestration path and make it green.
4. Add tests for existing users, invalid Google claims, provider failures, and
   repository failures.
5. Add focused JWT claim, expiration, signature, and JTI/revocation tests.
6. Refactor service error handling and sensitive-data logging only while tests
   remain green.

### Priority 3: Auth handler, middleware, and routes

1. Write the first `httptest` for successful Google login.
2. Implement request binding and response mapping.
3. Add malformed-input, provider-error, and generic-error HTTP tests.
4. Write JWT middleware tests for missing, invalid, expired, and revoked tokens.
5. Add logout tests and wire the route through the authenticated group.
6. Add development-mode OAuth helper route tests and enforce the environment
   gate.
7. Finish Auth module wiring and run all Auth tests end to end.

### Priority 4: Categories schema and repository

1. Add or verify the migration/backfill for mandatory `type`, the allowed-value
   check, removal of old uniqueness, and `(user_id, name, type)` uniqueness.
2. Write the first failing integration test: duplicate same-type category is
   rejected while same-name different-type category is accepted.
3. Update SQL query files and run `sqlc generate`; never edit generated files.
4. Add create, list, filtered-list, update, delete, ownership, not-found, and
   database constraint mapping tests in the sequence defined above.
5. Verify transaction category foreign-key behavior after category deletion.
6. Run the complete Categories repository suite against a clean migrated
   PostgreSQL database.

### Priority 5: Categories service

1. Write the failing normalization/validation test for name and type.
2. Implement create validation and repository delegation.
3. Add tests for valid filters, invalid filters, conflicts, not-found results,
   update normalization, and ownership-preserving delete delegation.
4. Refactor shared validation only after the service tests are green.

### Priority 6: Categories handler and routes

1. Write the first successful `POST /api/v1/categories` HTTP test with a mocked
   service and authenticated user context.
2. Implement request/response mapping and `201` behavior.
3. Add invalid payload, strict-field, conflict, and internal-error tests.
4. Add list tests for no filter, both valid filters, and invalid filter.
5. Add update and delete tests for success, invalid UUID, not-found, conflict,
   and `204` behavior.
6. Register categories only under the JWT-protected API group and test that the
   user ID always comes from middleware context.

### Priority 7: Integrated verification and hardening

1. Run `sqlc generate` from the final migration/query state.
2. Run `gofmt` on changed Go files.
3. Run all unit and integration tests with the Docker database available.
4. Run `go test ./...`.
5. Run `go vet ./...`.
6. Run `go build ./...`.
7. Exercise the documented API flows manually or with an HTTP collection:
   Google login, logout, category create, duplicate conflict, same-name
   different-type create, filtered list, update type, ownership denial, and
   delete.
8. Review logs for required structured context and confirm no token, password,
   Google ID token, authorization header, database URL, or unnecessary PII is
   emitted.

## 5. Definition of Done

- Auth repository integration tests pass against real local PostgreSQL.
- Auth service tests pass with mocked repository and Google provider.
- Auth handler and JWT middleware HTTP tests pass with `httptest`.
- Categories repository integration tests prove the exact unique and check
  constraints, including duplicate-category violations.
- Categories service tests prove normalization, validation, ownership arguments,
  and error propagation using mocks.
- Categories handler tests prove every endpoint's status, response, validation,
  authentication, and error mapping.
- All SQL access goes through `sqlc` generated code using `pgx/v5`; no ORM is
  introduced.
- Category `type` is mandatory and limited to `income` or `expense`.
- Uniqueness is exactly `(user_id, name, type)`.
- Protected operations cannot use a client-supplied `user_id`.
- `go test ./...`, `go vet ./...`, and `go build ./...` pass.
- The implementation was produced through observable Red-Green-Refactor steps,
  with no untested production behavior left in Auth or Categories.
