# WalletX Backend Documentation

Welcome to the **WalletX Backend** repository documentation. This document serves as a comprehensive guide for developers (and AI assistants like Gemini) to understand the architecture, tech stack, directory structure, and core features of the WalletX backend.

---

## 🛠️ Tech Stack & Environment

- **Language:** Golang (Go 1.x)
- **Framework:** [Gin](https://github.com/gin-gonic/gin) (High-performance HTTP web framework)
- **ORM:** [GORM](https://gorm.io/)
- **Database:** PostgreSQL (Hosted on Supabase)
- **Caching & State:** Redis (Hosted on Upstash)
- **Deployment:** Vercel (Serverless Go Environment)
- **Authentication:** Google OAuth 2.0 & JWT (JSON Web Tokens)
- **Integrations:**
  - Standard IMAP protocol (for parsing bank notification emails)
  - Telegram Bot API (for account linking and webhook-based notifications)

---

## 📁 Project Architecture & Structure

This project strictly adheres to **Clean Architecture** principles, enforcing separation of concerns across different layers (Handlers -> Services -> Repositories).

```text
walletx-be/
├── api/                  # Vercel serverless entrypoint
│   └── index.go          # Binds Gin router to standard net/http for Vercel
├── cmd/
│   └── server/           # Local development entrypoint
│       └── main.go       # `go run cmd/server/main.go`
├── configs/              # Configuration & Environment loading
│   └── config.go         # Strongly typed config structs (DB, Redis, JWT, IMAP, Telegram, OAuth)
├── pkg/                  # Application core
│   ├── cache/            # Redis client initialization
│   ├── database/         # PostgreSQL connection and GORM auto-migrations
│   ├── handlers/         # HTTP Layer (Gin contexts, parsing requests, returning JSON)
│   ├── middleware/       # Custom Gin middlewares (Auth JWT, CronAuth)
│   ├── models/           # Domain Entities & Database schemas (GORM tags)
│   ├── repository/       # Data Access Layer (DB and Redis abstractions)
│   ├── router/           # Centralized routing & endpoint grouping
│   ├── services/         # Core Business Logic and external API interactions
│   └── utils/            # Shared utilities (Standardized JSON Responses, Email SMTP sender)
├── .env                  # Environment variables
├── go.mod                # Go module dependencies
└── vercel.json           # Vercel deployment and routing configuration
```

---

## 🚀 Deployment (Vercel Serverless)

The backend is deployed to Vercel as a serverless function. 
- **`vercel.json`**: Maps all `/api/(.*)` requests to `api/index.go`.
- **`api/index.go`**: Initializes all Repositories, Services, and Handlers. It mounts the `gin.Engine` onto the global `http.Handler`. Vercel automatically invokes this function for incoming requests without keeping a long-running server alive.
- **Cron Jobs**: Configured via `vercel.json` (e.g., `/api/v1/cron/imap`, `/api/v1/cron/recurring`) and secured using `CronAuthMiddleware`.

---

## 🧠 Core Features & Modules

### 1. Authentication (OAuth & JWT)
- Handled in `AuthService` and `AuthHandler`.
- Users log in via Google SSO.
- Issues a JWT upon successful login.
- Logout is handled by adding the JWT token to a **Redis Blacklist** until it expires.

### 2. Transaction Management & Automated IMAP Sync
- **CRUD Operations**: Standard tracking of income and expenses.
- **IMAP Worker**: A cron-triggered service (`IMAPSync`) connects to a specified Gmail account via IMAP. It reads bank notification emails, extracts the transaction amount and merchant using Regex/LLM logic (in `ParserService`), and automatically creates transactions for users.
- **Idempotency**: Prevents duplicate email parsing using `MessageID` (Unique Index in DB).

### 3. Categories & Budgeting
- Users can create custom categories.
- Users can set monthly `Budgets` (tied to a `CategoryID`).
- The system computes real-time budget utilization and warns the user if spending exceeds the limit.

### 4. Recurring Transactions
- Users can define scheduled transactions (e.g., `weekly`, `monthly`, `yearly`).
- A cron job (`RecurringSync`) triggers daily to evaluate and inject due transactions automatically.

### 5. Telegram Integration (Webhook)
- Handled in `TelegramService` and `TelegramHandler`.
- Provides an Account Linking feature using an email verification flow.
- A user sends `/start` to the bot -> Types their registered email -> Backend generates a UUID token -> Token is saved in Redis -> Sends an email via `pkg/utils/email.go`.
- User clicks the link -> Endpoint `GET /api/v1/telegram/verify` is triggered -> Backend updates the `TelegramChatID` in the `User` table.
- Since it runs on Vercel, it uses **Webhooks** rather than long-polling.

---

## 💾 Database Schema (Core Entities)

- **`User`**: Core user profile (UUID, GoogleID, Email, Name, Picture, TelegramChatID).
- **`Transaction`**: Records of spending (UUID, UserID, CategoryID, Amount, Merchant, Date, MessageID [for IMAP uniqueness]).
- **`Category`**: User-defined categories (UUID, Name, Icon, Color).
- **`Budget`**: Budget limits (UUID, UserID, CategoryID, LimitAmount, Month, Year).
- **`RecurringConfig`**: Scheduled transaction settings (UUID, UserID, CategoryID, Amount, Frequency, NextDueDate).

---

## 🌐 API Routes Overview

All routes are prefixed with `/api/v1/`.

| Group | Method | Endpoint | Description |
|---|---|---|---|
| **Public** | `GET` | `/ping` | Health check |
| **Auth** | `POST` | `/auth/google` | Google SSO Login |
| **Telegram** | `POST` | `/telegram/webhook` | Receives updates from Telegram Bot |
| **Telegram** | `GET` | `/telegram/verify` | Email verification token handler |
| **Protected** | `POST` | `/auth/logout` | Invalidates JWT (Redis Blacklist) |
| **Protected** | `CRUD` | `/transactions` | Full Transaction CRUD |
| **Protected** | `CRUD` | `/categories` | Full Category CRUD |
| **Protected** | `CRUD` | `/budgets` | Full Budget CRUD |
| **Protected** | `GET` | `/budgets/progress` | Budget vs Actual Spending |
| **Protected** | `CRUD` | `/recurrings` | Full RecurringConfig CRUD |
| **Protected** | `GET` | `/reports/expenses` | Grouped expenses by category |
| **Protected** | `GET` | `/reports/daily-calendar`| Daily spending aggregation |
| **Cron** | `GET` | `/cron/imap` | Triggers IMAP email parsing |
| **Cron** | `GET` | `/cron/recurring` | Triggers Recurring transaction injection |

*(Note: `Protected` routes require a valid Bearer JWT. `Cron` routes require a secret cron token header matching `CRON_SECRET`)*

---

## 🔑 Key Design Decisions

1. **Vercel Cold Starts**: The database connection (`gorm.Open`) is executed inside the `init()` function of `api/index.go` to leverage Vercel's execution context caching where possible.
2. **Standardized Responses**: All API endpoints use `pkg/utils/response.go` to return consistent JSON shapes (`{ status, message, data, meta, timestamp }`).
3. **Soft vs Hard Deletes**: Some entities like `Transaction` use `gorm.DeletedAt` for soft deletes, while `RecurringConfig` uses hard deletes.
4. **Pointer Types in GORM**: Pointers (e.g., `*uuid.UUID`, `*string`) are heavily used in GORM models to cleanly handle database `NULL` values.

---
*Generated for the WalletX Backend Team.*
