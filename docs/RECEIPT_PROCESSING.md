# Receipt processing

For receipt upload, AI extraction, and transaction CRUD through Bruno, see
[Transaction flow testing](TRANSACTION_TESTING.md). For isolated adapter diagnosis,
see [Storage testing](STORAGE_TESTING.md).

`POST /api/v1/transactions/receipt` accepts an authenticated multipart upload in
`receipt` (JPEG, PNG, GIF, or WebP; at most 10 MiB).

The service saves the image, commits a pending transaction with NULL extraction
fields, and then attempts extraction. Extraction and category resolution use a
bounded context. Completion updates the original row atomically; it starts the
existing embedding job only after that update succeeds.

## Receipt storage configuration

| Variable | Default | Meaning |
|---|---|---|
| `RECEIPT_STORAGE_PROVIDER` | `local` | Select `local` or `supabase`. Unsupported values fail startup. |
| `RECEIPT_STORAGE_DIR` | `.data/receipts` | Persistent, private directory containing receipts; used only by `local`. |
| `SUPABASE_URL` | Empty | Project origin, required for `supabase`. HTTPS is required except for loopback development. |
| `SUPABASE_SERVICE_ROLE_KEY` | Empty | Server-side service-role key, required for `supabase`. Never log or expose it to clients. |
| `RECEIPT_STORAGE_TIMEOUT` | `30s` | Positive Go duration bounding Supabase HTTP requests, including deletion. Invalid values fail startup when `supabase` is selected. |
| `RECEIPT_AI_TIMEOUT` | `20s` | Positive Go duration for extraction/category resolution. Invalid values fail startup. |

The local adapter writes private files with generated names, synchronizes their
contents, and publishes them with an atomic rename. It returns references such
as `receipt://<owner UUID>/<object UUID>.png`. These references survive adapter
and application restarts as long as the directory is retained. They are object
references, not public HTTP URLs. `storage.Local.ReadReceipt` retrieves bytes
using the owner ID and reference; it rejects cross-owner access and traversal.
Client filenames never determine paths. Filesystem operations stay inside an
`os.Root`. Receipt bytes are excluded from Git under `.data/`.

This adapter supports local development or a server with a persistent volume.
An ephemeral/serverless filesystem is not durable storage; select `supabase`
for such deployments. The `ReceiptStorage` port keeps that change out of the
receipt service.

For Supabase, set `RECEIPT_STORAGE_PROVIDER=supabase`, `SUPABASE_URL`, and
`SUPABASE_SERVICE_ROLE_KEY`. The existing public `receipts` bucket must exist.
The factory validates required settings, creates one reusable HTTP client with
the configured timeout, and initializes the adapter without a network request.
Missing credentials, malformed adapter settings, and invalid timeouts fail
startup; there is no automatic fallback to local storage. `configs.Load` loads
values; `ReceiptStorageConfig.Validate` is called during dependency construction.

Supabase returns a bucket-relative path such as
`<owner UUID>/<object UUID>_<Unix milliseconds>.png`, without a hostname or bucket
prefix. This path is persisted and returned in the existing `receipt_image_url`
field. The response examples below show local references; Supabase responses use
the relative path instead. Public URL construction belongs to a separate
presentation/resolution concern. Client filenames never determine object paths.

The Supabase adapter validates owner paths before authenticated batch deletion,
disables redirects, and returns safe errors to the service logging boundary.
Uploads are not automatically retried because a transport failure may occur
after an object has been accepted. Reconciliation remains necessary for uncertain
outcomes. Switching providers does not migrate local files or existing
`receipt://` references; preserve them and migrate separately when needed.

## Responses

Completed extraction returns **201 Created** using the standard success envelope:

```json
{
  "status": "success",
  "message": "Transaction recorded.",
  "data": {
    "status": "success",
    "transaction_id": "<UUID>",
    "receipt_image_url": "receipt://<owner>/<object>.png",
    "transaction": { "id": "<same UUID>", "amount": 123.45 }
  }
}
```

Extraction failure, timeout, invalid AI output, or unavailable category resolution
returns **202 Accepted** after durable acceptance:

```json
{
  "status": "success",
  "message": "Receipt saved; processing pending.",
  "data": {
    "status": "pending",
    "transaction_id": "<UUID>",
    "receipt_image_url": "receipt://<owner>/<object>.png",
    "transaction": null
  }
}
```

The outer `status` describes successful acceptance; `data.status` describes the
transaction. Transaction GET/list and vector search include only successful rows.
Fetching a pending receipt by ID returns 404; it is excluded before list pagination.
Owner-scoped DELETE can still remove pending receipts and clean up their images. Parent request
cancellation cannot guarantee HTTP delivery, but a
previously committed pending row and image are retained.

The pending table rows are a durable backlog. This phase does not introduce an
extraction retry worker. GET/list does not expose pending rows or retry extraction.

## Failures and upload lifecycle

- A storage failure returns a generic error without attempting a DB insert or AI.
- Confirmed insertion rejection (`ErrInvalidInput` / `ErrNotFound`) triggers
  idempotent upload deletion using a detached, bounded cleanup context.
- Cancellation after upload but before any insertion also deletes the upload.
- Cleanup failure leaves the object for reconciliation and is logged without the
  object reference, client filename, receipt contents, or provider error text.
- Connection loss, cancellation during insertion, and invalid insert results can
  have an uncertain commit outcome. Their objects are retained rather than
  risking deletion of an image belonging to a committed row.
- Completion errors preserve existing rows/images and keep the normal error
  classification. The service never deletes transactions on extraction failure.
- Transaction deletion fetches the owned row, deletes it from the database,
  then attempts idempotent image cleanup with a detached 5-second timeout.
  A failed database deletion retains the image. A cleanup failure is logged with
  safe context and does not fail the already committed transaction deletion;
  orphan cleanup still needs reconciliation (there is no durable cleanup queue).

Retained/unreferenced objects must be reconciled against **all** transaction rows
by owner and `receipt_image_url`, including pending rows, before deletion. Do not
apply blind age-based deletion to pending images. The local adapter has no
automatic expiry; it deliberately retains uncertain outcomes until reconciliation.
Handled provider errors are logged once with safe classifications and are never
included in response DTOs.

## Extraction diagnostics

Receipt extraction logs preserve Gemini diagnostics through the AI service's
wrapped errors: `provider_operation`, `provider_model`, `provider_http_code`,
`provider_status`, and `provider_reason` when available. SDK failures also include
`provider_exact_message` (credential-like strings redacted; capped at 4096 characters),
`provider_error_reason`, and selected `provider_details` from Google ErrorInfo.
Normal messages such as "Consumer invalid" are preserved verbatim. The `error` field is a
safe operational summary, while the original SDK cause remains accessible through
`errors.Is`/`errors.As`. Raw response bodies/headers and arbitrary detail metadata
are excluded. Only ErrorInfo reason/domain/service are retained in detail fields.

Examples of `provider_reason`: `model_or_endpoint_not_found`,
`model_access_restricted`, `invalid_api_key`, `invalid_response_schema`,
`quota_or_rate_limit`, `timeout`, `extraction_json_syntax_error`,
`extraction_missing_fields`, and `extraction_output_token_limit`. A provider reason
identifies the observed failure class; it does not prove that another independent
configuration problem is absent. The HTTP response still uses durable acceptance
and returns 202 after extraction failure.

The pinned SDK uses `genai.NewPartFromBytes` for inline image bytes with the
validated MIME type. Extraction requests JSON with a response schema. The parser
also accepts one outer plain or JSON Markdown fence, while retaining strict field,
category, date, and trailing-data validation. The model comes from
`GEMINI_CHAT_MODEL` (falling back to `configs.DefaultChatModel`).

## Verification

`go test ./...` and `go vet ./...` cover storage, service lifecycle, DI, and HTTP.
Repository receipt-flow integration tests require `TEST_DATABASE_URL` pointing
at an isolated PostgreSQL database with all migrations and pgvector applied.
Existing DB suites truncate tables; use a disposable test database.
