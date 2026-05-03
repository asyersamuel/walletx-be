# WalletX Backend - LLM Context Document

## 1. System Overview & Tech Stack
- **System Overview**: WalletX Backend is a personal finance application equipped with IMAP automation for extracting transactions from bank emails, and tools for tracking expenses, managing budgets, and setting up recurring transactions.
- **Tech Stack**:
  - **Language**: Go (Golang)
  - **Framework**: Gin Web Framework
  - **ORM**: GORM
  - **Database**: PostgreSQL (Supabase/Postgres via GORM)
  - **Cache**: Redis (Upstash)
  - **AI Parser**: Gemini API (via `generative-ai-go`)
  - **Cron Jobs**: Vercel Serverless Cron Jobs

## 2. Architecture & Design Patterns
- **Architecture Pattern**: Clean Architecture / Layered Architecture.
- **Strict Rules**:
  - **Dependency Injection**: Centralized in `internal/bootstrap`, wiring up repositories, services, and handlers before starting the server.
  - **DTO Usage**: The Domain layer communicates using Data Transfer Objects (DTOs) strictly. Returning unstructured maps (`map[string]interface{}`) is strictly prohibited to maintain type safety.
  - **Context Propagation**: The `context.Context` (`ctx`) must be passed down from the Gin handlers all the way to the GORM queries. This is vital for timeout management, tracing, and structured logging.
  - **Centralized Error Handling**: Use domain Sentinel Errors (e.g., from `internal/domain/errors.go`) and map them to standard HTTP status codes within the handlers.

## 3. Directory Structure
```
walletx-be/
├── cmd/
│   └── server/          # Main application entry point
├── configs/             # Configuration loading (e.g., environment variables)
├── internal/
│   ├── bootstrap/       # Dependency injection and wiring
│   ├── domain/          # Domain entities, DTOs, interfaces, and sentinel errors
│   ├── infrastructure/  # External services implementations (Cache, Logger, AI Parser)
│   └── ports/           # Interface definitions for external services
└── pkg/
    ├── cache/           # Redis initialization and setup
    ├── database/        # PostgreSQL connection setup via GORM
    ├── handlers/        # HTTP controllers (Gin) mapping requests to services
    ├── middleware/      # Gin middlewares (Auth, Cron Auth, CORS)
    ├── models/          # GORM ORM models representing database tables
    ├── repository/      # Data access layer implementations
    ├── router/          # Route registrations and API endpoints grouping
    ├── services/        # Core business logic implementations
    └── utils/           # Helper functions
```

## 4. Domain Models & Database Schema
Entities found in `pkg/models/`:

1. **User (`user.go`)**
   - **Fields**: `ID` (UUID), `GoogleID`, `Email`, `Name`, `Picture`, timestamps.
   - **Relations**: 1 User has many `Transactions` (Cascade delete).
2. **Transaction (`transaction.go`)**
   - **Fields**: `ID`, `UserID`, `CategoryID` (nullable), `Amount` (numeric 15,2), `Merchant`, `Note`, `TransactionDate`, `MessageID` (nullable, unique for idempotency), `IsRecurring`.
   - **Relations**: Belongs to `User` and `Category`.
3. **Category (`category.go`)**
   - **Fields**: `ID`, `UserID`, `Name`, `Icon`.
4. **CategoryLimit / Budget (`category_limit.go`)**
   - **Fields**: `ID`, `UserID`, `CategoryID`, `LimitAmount` (numeric 15,2), `IsActive`.
   - **Relations**: Belongs to `Category`. Uses hard deletes (no `gorm.DeletedAt`).
5. **RecurringConfig (`recurring_config.go`)**
   - **Fields**: `ID`, `UserID`, `CategoryID`, `Amount`, `Frequency` ("weekly", "monthly", "yearly"), `StartDate`, `NextDueDate`.
   - **Relations**: Belongs to `Category`. Uses hard deletes.

## 5. Core Workflows (Crucial Logic)
- **IMAP Parser Flow**:
  - Connects to IMAP (e.g., Gmail) and fetches `UNSEEN` emails.
  - For each email, `TransactionProcessor` matches the user by `senderEmail`.
  - **Idempotency**: Checks if `message_id` already exists in DB to prevent duplicates.
  - Parses email body via `EmailParser` (fallback to Gemini API for JSON structured output with timeout).
  - Determines Category (fallback to default "Lainnya").
  - Saves the `Transaction` and invalidates the user's dashboard cache.
  - Marks email as `SEEN` on the IMAP server.
- **Cache Segregation**:
  - Handled by `DashboardCacheManager` (e.g., `redisDashboardCacheManager`).
  - Redis cache keys use a pattern like `cache:dashboard:*:{userID}*`.
  - Invalidation uses Redis `SCAN` with a cursor to incrementally find and `DEL` keys matching the pattern, avoiding hardcoded loops or blocking `KEYS` commands that degrade performance.
- **Cron Jobs**:
  - Driven by Vercel serverless cron triggering endpoints like `GET /api/v1/cron/imap` and `GET /api/v1/cron/recurring`.
  - Protected by `CronAuthMiddleware` which validates a Bearer token against `CRON_SECRET`.

## 6. API Routing Summary
Base Path: `/api/v1`

| Method | Path | Handler | Access |
|--------|------|---------|--------|
| POST | `/auth/google` | `AuthHandler.HandleGoogleAuth` | Public |
| GET | `/auth/google/test-login` | `AuthHandler.GoogleLoginTest` | Public (Dev Mode) |
| GET | `/auth/google/callback` | `AuthHandler.GoogleCallbackTest`| Public (Dev Mode) |
| POST | `/auth/logout` | `AuthHandler.Logout` | Protected |
| GET | `/transactions` | `TransactionHandler.GetUserTransactions`| Protected |
| POST | `/transactions` | `TransactionHandler.CreateTransaction` | Protected |
| PUT | `/transactions/:id`| `TransactionHandler.UpdateTransaction` | Protected |
| DELETE | `/transactions/:id`| `TransactionHandler.DeleteTransaction` | Protected |
| GET | `/categories` | `CategoryHandler.ListCategories` | Protected |
| POST | `/categories` | `CategoryHandler.CreateCategory` | Protected |
| GET | `/categories/:id` | `CategoryHandler.GetCategoryByID` | Protected |
| PUT | `/categories/:id` | `CategoryHandler.UpdateCategory` | Protected |
| DELETE | `/categories/:id` | `CategoryHandler.DeleteCategory` | Protected |
| GET | `/budgets` | `BudgetHandler.ListBudgets` | Protected |
| GET | `/budgets/progress`| `BudgetHandler.GetBudgetProgress` | Protected |
| POST | `/budgets` | `BudgetHandler.CreateBudget` | Protected |
| GET | `/budgets/:id` | `BudgetHandler.GetBudgetByID` | Protected |
| PUT | `/budgets/:id` | `BudgetHandler.UpdateBudget` | Protected |
| DELETE | `/budgets/:id` | `BudgetHandler.DeleteBudget` | Protected |
| GET | `/recurrings` | `RecurringHandler.ListRecurrings` | Protected |
| POST | `/recurrings` | `RecurringHandler.CreateRecurring` | Protected |
| GET | `/recurrings/:id`| `RecurringHandler.GetRecurringByID` | Protected |
| PUT | `/recurrings/:id`| `RecurringHandler.UpdateRecurring` | Protected |
| DELETE | `/recurrings/:id`| `RecurringHandler.DeleteRecurring` | Protected |
| GET | `/reports/expenses`| `TransactionHandler.GetReports` | Protected |
| GET | `/reports/budget-summary`| `DashboardHandler.GetBudgetSummary` | Protected |
| GET | `/reports/daily-calendar`| `DashboardHandler.GetDailyCalendar` | Protected |
| GET | `/cron/keep-alive`| `CronHandler.KeepAlive` | Protected (Cron Secret) |
| GET | `/cron/imap` | `CronHandler.IMAPSync` | Protected (Cron Secret) |
| GET | `/cron/recurring` | `CronHandler.RecurringSync` | Protected (Cron Secret) |

## 7. Developer Guidelines: "How to Add a New Feature"
Follow these steps to add a new entity (e.g., "Goal"):

1. **Define the Domain Model**: Create `pkg/models/goal.go` mapping to the DB schema with GORM tags.
2. **Define DTOs**: Create request/response structs in `internal/domain/dto/goal.go` to be used for layer communication.
3. **Define Interfaces**: Define `GoalRepository` and `GoalService` interfaces in the appropriate directory.
4. **Implement Repository**: Write `goal_repository.go` inside `pkg/repository/` injecting `*gorm.DB` and making sure to pass `ctx context.Context` into all queries.
5. **Implement Service**: Write `goal_service.go` inside `pkg/services/` containing the core business logic, returning standard DTOs or Sentinel Errors.
6. **Create Handler**: Write `goal_handler.go` inside `pkg/handlers/` to parse Gin requests, map HTTP Status codes from domain errors, and return JSON responses.
7. **Routing & Bootstrap Wiring**: 
   - Register the endpoints in `pkg/router/route_registrars.go`.
   - Update `router.go` to include the new routes.
   - Inject the new repository, service, and handler dependencies, wiring them up cleanly in `internal/bootstrap/`.
