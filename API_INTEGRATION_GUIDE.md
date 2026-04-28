# WalletX - Master API Documentation & Integration Guide

> **IMPORTANT NOTICE FOR FLUTTER FE AI AGENT**
> This document is designed specifically for the Frontend AI Agent to integrate with the WalletX Backend. Please read the Network & HTTPS Setup and Authentication Architecture carefully before implementing any API calls.

## 1. Network & HTTPS Setup (CRITICAL FOR FE)

- **Strict HTTPS Requirement:** The backend runs strictly on HTTPS using `mkcert` for a local Certificate Authority (CA).
- **Network Resolution:** You **CANNOT** use `localhost` (or `10.0.2.2` on Android emulators) if testing on real devices or emulators that need to resolve the backend's local IP on the network. You must use the host machine's local LAN IP (e.g., `https://192.168.X.X:8080`).
- **SSL Certificate Handling:** Because the backend uses a local CA via `mkcert`, the Flutter app will throw a `HandshakeException` (CERTIFICATE_VERIFY_FAILED). You **MUST** bypass or handle the local SSL certificate validation during local development. 
  *Example bypass using `dio` in Flutter:*
  ```dart
  dio.httpClientAdapter = IOHttpClientAdapter(
    createHttpClient: () {
      final client = HttpClient();
      client.badCertificateCallback = (X509Certificate cert, String host, int port) => true;
      return client;
    },
  );
  ```

## 2. Authentication Architecture (Single JWT System)

- **Single Token System:** This API relies on a **SINGLE** token system. There are no refresh tokens implemented for now.
- **Token Delivery:** After calling the `/auth/google` endpoint successfully, the token is provided in the response payload inside `data.token` (this is the internal JWT).
- **Token Storage & Usage:** You must instruct the FE to store this token securely (e.g., using `flutter_secure_storage`). The token must be attached to the headers of all protected endpoints:
  `Authorization: Bearer <token>`
- **Token Expiration & 401s:** If any protected endpoint returns a `401 Unauthorized` status code, the FE should assume the token is expired or invalid. To handle this, the FE automatically needs to clear the stored token and prompt the user to re-authenticate via Google.

---

## 3. Complete Endpoints Specification

### 3.1. System & Authentication

#### Health Check (Ping)
- **Endpoint:** `GET /api/v1/ping`
- **Description:** Verify if the backend server is running and accessible.
- **Auth Required:** No
- **Headers:** None
- **Request Body:** None
- **Success Response** (`200 OK`):
  ```json
  {
    "message": "WalletX API is running!"
  }
  ```

#### Google Authentication (Mobile App)
- **Endpoint:** `POST /api/v1/auth/google`
- **Description:** Authenticate a user using a Google `id_token` obtained from the Google Sign-In SDK on the mobile app.
- **Auth Required:** No
- **Headers:** `Content-Type: application/json`
- **Request Body Payload:**
  ```json
  {
    "id_token": "string" // The JWT string received from Google Sign-In
  }
  ```
- **Success Response** (`200 OK` for Login, or `201 Created` for New Registration):
  ```json
  {
    "status": "success",
    "message": "Login berhasil", // OR "Registrasi berhasil"
    "data": {
      "user": {
        "id": "uuid string",
        "google_id": "string",
        "email": "string",
        "name": "string",
        "picture": "string",
        "created_at": "timestamp",
        "updated_at": "timestamp"
      },
      "token": "string" // The internal JWT to be used for protected requests
    }
  }
  ```
- **Expected Error Responses:**
  - `400 Bad Request`: `{"error": "Data tidak valid", "detail": "..."}`
  - `500 Internal Server Error`: `{"error": "Gagal memproses autentikasi", "detail": "..."}`


### 3.2. Transactions Management

#### Get User Transactions
- **Endpoint:** `GET /api/v1/transactions`
- **Description:** Retrieve paginated financial transactions for the authenticated user.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Query Parameters:**
  - `limit` (int, Optional, default: 20)
  - `offset` (int, Optional, default: 0)
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "Transactions retrieved successfully",
    "data": [
      {
        "id": "uuid string",
        "user_id": "uuid string",
        "category_id": "uuid string",
        "amount": 150000.00,
        "merchant": "string",
        "note": "string",
        "transaction_date": "timestamp",
        "message_id": "string",
        "is_recurring": false,
        "created_at": "timestamp",
        "updated_at": "timestamp"
      }
    ],
    "timestamp": "ISO 8601 string"
  }
  ```

#### Create Transaction (Manual)
- **Endpoint:** `POST /api/v1/transactions`
- **Description:** Create a manual transaction from the mobile app.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body Payload:**
  ```json
  {
    "amount": 150000.00, // required, > 0
    "merchant": "Coffee Shop", // required
    "note": "Meeting with client", // optional
    "category_id": "uuid string", // optional, can be null
    "transaction_date": "2026-03-20T10:00:00Z" // required, ISO 8601
  }
  ```
- **Success Response** (`201 Created`): Returns the created transaction object.

#### Update Transaction
- **Endpoint:** `PUT /api/v1/transactions/:id`
- **Description:** Update an existing transaction.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body Payload:**
  ```json
  {
    "amount": 120000.00, // required, > 0
    "merchant": "Coffee Shop", // required
    "note": "Updated note", // optional
    "category_id": "uuid string", // optional, can be null
    "transaction_date": "2026-03-20T10:00:00Z" // required, ISO 8601
  }
  ```
- **Success Response** (`200 OK`): Returns the updated transaction object.

#### Delete Transaction
- **Endpoint:** `DELETE /api/v1/transactions/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`): 
  ```json
  {
    "status": "success",
    "message": "Transaction deleted successfully",
    "data": null,
    "timestamp": "ISO 8601 string"
  }
  ```

#### Search Transactions
- **Endpoint:** `GET /api/v1/transactions/search`
- **Description:** Search transactions by merchant or note content.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Query Parameters:**
  - `q` (string, Required): The search query.
  - `limit` (int, Optional, default: 20)
  - `offset` (int, Optional, default: 0)
- **Success Response** (`200 OK`): Returns an array of matched transaction objects.

#### Export Transactions (CSV)
- **Endpoint:** `GET /api/v1/transactions/export`
- **Description:** Downloads a CSV file containing the user's transactions for a specific month and year.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Query Parameters:**
  - `month` (int, Required)
  - `year` (int, Required)
- **Success Response** (`200 OK`): Response is streamed as `text/csv` attachment.


### 3.3. Categories Management

#### Get All Categories
- **Endpoint:** `GET /api/v1/categories`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "Categories retrieved successfully",
    "data": [
      {
        "id": "uuid string",
        "user_id": "uuid string",
        "name": "Food & Drink",
        "icon": "ic_food",
        "created_at": "timestamp",
        "updated_at": "timestamp"
      }
    ]
  }
  ```

#### Create Category
- **Endpoint:** `POST /api/v1/categories`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body Payload:**
  ```json
  {
    "name": "Food & Drink", // required
    "icon": "ic_food" // optional
  }
  ```
- **Success Response** (`201 Created`): Returns the created category object.

#### Get Category By ID
- **Endpoint:** `GET /api/v1/categories/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`): Returns the single category object.

#### Update Category
- **Endpoint:** `PUT /api/v1/categories/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body Payload:**
  ```json
  {
    "name": "Dining", // required
    "icon": "ic_dining" // optional
  }
  ```
- **Success Response** (`200 OK`): Returns the updated category object.

#### Delete Category
- **Endpoint:** `DELETE /api/v1/categories/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`): Deletes category and returns standard success message.


### 3.4. Budget Limits Management

#### Get All Budgets
- **Endpoint:** `GET /api/v1/budgets`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "Budget limits retrieved successfully",
    "data": [
      {
        "id": "uuid string",
        "user_id": "uuid string",
        "category_id": "uuid string",
        "limit_amount": 500000.00,
        "period": "monthly", // "weekly" or "monthly"
        "is_active": true,
        "created_at": "timestamp",
        "updated_at": "timestamp"
      }
    ]
  }
  ```

#### Get Budget Progress
- **Endpoint:** `GET /api/v1/budgets/progress`
- **Description:** Returns the progress (spent vs limit) for all active budgets based on a targeted date.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Query Parameters:**
  - `date` (string, Optional): Returns progress for the period containing this date (YYYY-MM-DD). Defaults to today.
- **Success Response** (`200 OK`): Returns array of progress objects per budget.

#### Create Budget Limit
- **Endpoint:** `POST /api/v1/budgets`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body Payload:**
  ```json
  {
    "category_id": "uuid string", // required
    "limit_amount": 500000.00, // required, > 0
    "period": "monthly" // required ("weekly" or "monthly")
  }
  ```
- **Success Response** (`201 Created`): Returns the created budget limit object.

#### Get Budget By ID
- **Endpoint:** `GET /api/v1/budgets/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`): Returns the single budget limit object.

#### Update Budget Limit
- **Endpoint:** `PUT /api/v1/budgets/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body Payload:**
  ```json
  {
    "limit_amount": 600000.00, // required, > 0
    "period": "monthly", // required ("weekly" or "monthly")
    "is_active": true // optional
  }
  ```
- **Success Response** (`200 OK`): Returns the updated budget limit object.

#### Delete Budget
- **Endpoint:** `DELETE /api/v1/budgets/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`): Deletes budget limit and returns standard success message.


### 3.5. Recurring Transactions Config

#### Get All Recurring Configs
- **Endpoint:** `GET /api/v1/recurrings`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "Recurring configs retrieved successfully",
    "data": [
      {
        "id": "uuid string",
        "user_id": "uuid string",
        "category_id": "uuid string",
        "amount": 100000.00,
        "frequency": "monthly", // "weekly", "monthly", "yearly"
        "start_date": "timestamp (ISO 8601)",
        "next_due_date": "timestamp (ISO 8601)",
        "created_at": "timestamp",
        "updated_at": "timestamp"
      }
    ]
  }
  ```

#### Create Recurring Config
- **Endpoint:** `POST /api/v1/recurrings`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body Payload:**
  ```json
  {
    "category_id": "uuid string", // required
    "amount": 100000.00, // required, > 0
    "frequency": "monthly", // required ("weekly", "monthly", "yearly")
    "start_date": "2026-03-25T00:00:00Z" // required
  }
  ```
- **Success Response** (`201 Created`): Returns the created recurring config object.

#### Get Recurring By ID
- **Endpoint:** `GET /api/v1/recurrings/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`): Returns the single recurring config object.

#### Update Recurring Config
- **Endpoint:** `PUT /api/v1/recurrings/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body Payload:**
  ```json
  {
    "amount": 150000.00, // required, > 0
    "frequency": "monthly", // required
    "start_date": "2026-04-25T00:00:00Z" // optional
  }
  ```
- **Success Response** (`200 OK`): Returns the updated recurring config object.

#### Delete Recurring Config
- **Endpoint:** `DELETE /api/v1/recurrings/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`): Deletes recurring config and returns standard success message.


### 3.6. Dashboard & Aggregation

#### Get Budget Summary (Remaining Limits)
- **Endpoint:** `GET /api/v1/dashboard/budget-summary`
- **Description:** Returns a merged view of each active budget limit and the actual spent amount.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "Budget summary retrieved successfully",
    "data": [
      {
        "category_id": "uuid string",
        "limit_amount": 500000.00,
        "spent_amount": 250000.00,
        "remaining_budget": 250000.00,
        "period": "monthly"
      }
    ],
    "timestamp": "ISO 8601 string"
  }
  ```

#### Get Daily Calendar Totals
- **Endpoint:** `GET /api/v1/dashboard/calendar`
- **Description:** Returns aggregated spending grouped by day for a specific month and year.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Query Parameters:**
  - `month` (int, required): 1-12
  - `year` (int, required): e.g., 2026
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "Daily calendar retrieved successfully",
    "data": [
      {
        "day": "2026-03-01T00:00:00Z",
        "total_amount": 45000.00
      },
      {
        "day": "2026-03-02T00:00:00Z",
        "total_amount": 120000.00
      }
    ]
  }
  ```


### 3.7. Reports

#### Expenses By Category
- **Endpoint:** `GET /api/v1/reports/expenses-by-category`
- **Description:** Returns spending breakdowns grouped by category, suitable for pie charts or analytics.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Query Parameters:**
  - `month` (int, required)
  - `year` (int, required)
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "Reports retrieved",
    "data": [
      {
        "category_id": "uuid string",
        "category_name": "Food & Drink",
        "total_amount": 800000.00
      }
    ],
    "timestamp": "ISO 8601 string"
  }
  ```

---
*End of Document*
