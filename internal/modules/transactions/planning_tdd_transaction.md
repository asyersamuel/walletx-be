# Transactions Module - TDD Execution Plan

## 1. Repository Layer Tests (`repository_test.go`)

### Schema Mapping and Create Queries

- [ ] Verify `CreateTransaction` inserts the caller-provided `user_id` into `transactions.user_id`.
- [ ] Verify `CreateTransaction` inserts the caller-provided `category_id` into `transactions.category_id`.
- [ ] Verify `CreateTransaction` inserts `merchant_name`, `amount`, `transaction_date`, `receipt_image_url`, and `notes` into the correct columns.
- [ ] Verify the create SQL does not reference or attempt to insert a `type` column because transaction type is derived from the related category.
- [ ] Verify a successful insert returns the generated transaction `id` as a UUID.
- [ ] Verify a successful insert returns the persisted `user_id`, `category_id`, `merchant_name`, `amount`, `transaction_date`, `receipt_image_url`, `notes`, and nullable `embedding`.
- [ ] Verify a transaction created without `receipt_image_url` stores SQL `NULL`, not an empty placeholder value unless the application contract explicitly normalizes it.
- [ ] Verify a transaction created without `notes` stores SQL `NULL`, not an arbitrary default value.
- [ ] Verify the repository preserves the exact numeric amount without floating-point conversion or rounding.
- [ ] Verify the repository preserves the transaction timestamp, including timezone offset, when writing `transaction_date`.
- [ ] Verify the repository does not silently replace a caller-provided timestamp with the database server time.
- [ ] Verify repository query arguments are parameterized and merchant names, notes, URLs, and UUIDs cannot alter the SQL statement.
- [ ] Verify a database error during insert is returned to the service unchanged or wrapped with a cause that can still be classified.
- [ ] Verify a duplicate or invalid UUID error is returned as a repository error rather than being reported as a successful creation.

### Database Constraints

- [ ] Verify inserting `amount = 0` fails with the database positive-amount constraint.
- [ ] Verify inserting a negative amount fails with the database positive-amount constraint.
- [ ] Verify inserting `amount = NULL` fails with the `amount NOT NULL` constraint.
- [ ] Verify inserting `merchant_name = NULL` fails with the `merchant_name NOT NULL` constraint.
- [ ] Verify inserting `transaction_date = NULL` fails with the `transaction_date NOT NULL` constraint.
- [ ] Verify inserting `category_id = NULL` fails if the schema declares the category relation as required; otherwise verify the repository behavior matches the explicitly documented nullable-category contract.
- [ ] Verify inserting a non-existent `category_id` fails with the category foreign-key constraint.
- [ ] Verify inserting a non-existent `user_id` fails with the user foreign-key constraint.
- [ ] Verify a valid positive numeric amount with decimal precision supported by PostgreSQL `numeric` is persisted without loss.
- [ ] Verify an amount exceeding the configured numeric precision or scale returns a database error and does not create a partial transaction.
- [ ] Verify a merchant name exceeding the configured `varchar` length returns a database error.
- [ ] Verify `receipt_image_url = NULL` is accepted.
- [ ] Verify `notes = NULL` is accepted.
- [ ] Verify an embedding value of `NULL` is accepted at initial transaction creation when embeddings are generated asynchronously.
- [ ] Verify a valid pgvector embedding with the configured dimensionality is persisted successfully.
- [ ] Verify an embedding with the wrong dimensionality is rejected by PostgreSQL or the repository before persistence.
- [ ] Verify a malformed vector literal is rejected and does not modify the transaction.
- [ ] Verify category ownership is enforced by the repository query or database policy: a valid category belonging to another user cannot be associated with the caller’s transaction.
- [ ] Verify category ownership is not inferred from category name alone; the category UUID and executing `user_id` must both be checked.
- [ ] Verify a transaction insert cannot be created for `user_id = A` with `category_id` belonging to `user_id = B`.

### Category Relation and Ownership Queries

- [ ] Verify the category validation query checks both `category_id = requestedCategoryID` and `category.user_id = executingUserID`.
- [ ] Verify an existing category belonging to the same user passes the repository category check.
- [ ] Verify a non-existent category returns a classified not-found/category error.
- [ ] Verify a category belonging to another user returns the same protected result expected for cross-user access, such as not-found or forbidden, without exposing the category owner.
- [ ] Verify the category query returns the category name required for embedding text generation.
- [ ] Verify the repository does not read a category belonging to another user merely because its UUID is known.
- [ ] Verify category metadata, including income/expense classification, is read from `categories` and not from a `transactions.type` column.
- [ ] Verify the transaction row returned from a join includes the category name and category classification only when the category belongs to the same user.
- [ ] Verify a deleted category referenced by a transaction follows the configured foreign-key behavior and is surfaced consistently.

### Get-by-ID Queries

- [ ] Verify `GetTransactionByID(userID, transactionID)` returns a transaction when both UUIDs belong to the same user.
- [ ] Verify the get-by-ID SQL includes the caller’s `user_id` predicate, not just the transaction UUID predicate.
- [ ] Verify requesting an existing transaction belonging to another user returns `ErrNotFound` or the repository’s protected-access error.
- [ ] Verify the returned result for a cross-user transaction contains no merchant name, amount, notes, receipt URL, category information, or embedding data.
- [ ] Verify requesting a non-existent transaction returns `ErrNotFound`.
- [ ] Verify an invalid transaction UUID returns a classified input/repository error and does not execute an unsafe query.
- [ ] Verify an invalid user UUID returns a classified input/repository error.
- [ ] Verify a database connection failure is returned as an infrastructure error.
- [ ] Verify a row with a `NULL` receipt URL scans successfully.
- [ ] Verify a row with `NULL` notes scans successfully.
- [ ] Verify a row with `NULL` embedding scans successfully.
- [ ] Verify a populated pgvector column scans into the expected embedding representation without dimension or ordering changes.
- [ ] Verify a category join does not cause a valid transaction to disappear when the category relation is expected to be present.
- [ ] Verify the query does not expose a transaction if its category is no longer owned by the caller under the configured isolation policy.

### List and Pagination Queries

- [ ] Verify listing with `userID = A`, `limit = 20`, and `offset = 0` returns only user A’s transactions.
- [ ] Verify listing never returns a transaction belonging to user B, even when user B’s transaction matches all date and pagination filters.
- [ ] Verify the default limit is applied when no limit is supplied.
- [ ] Verify the default offset is zero when no offset is supplied.
- [ ] Verify `limit = 1` returns at most one transaction.
- [ ] Verify `offset = 0` returns the first page according to the repository’s deterministic ordering.
- [ ] Verify an offset beyond the number of matching rows returns an empty list and no error.
- [ ] Verify an empty result returns an empty JSON-compatible collection rather than a `NULL` collection.
- [ ] Verify the SQL applies the caller’s `user_id` predicate before pagination so another user’s rows cannot affect page boundaries.
- [ ] Verify pagination uses parameterized `LIMIT` and `OFFSET` values.
- [ ] Verify negative limits are rejected by the repository or service contract and never reach unsafe SQL.
- [ ] Verify negative offsets are rejected by the repository or service contract and never reach unsafe SQL.
- [ ] Verify an excessively large limit is rejected or capped according to the defined API contract.
- [ ] Verify the ordering is deterministic when multiple transactions have the same `transaction_date`, using a stable secondary key such as `id`.
- [ ] Verify the newest-first or oldest-first ordering exactly matches the documented repository contract.
- [ ] Verify pagination does not duplicate or skip rows across adjacent pages when no data changes occur.
- [ ] Verify a transaction with a `NULL` embedding remains returned by normal list queries.
- [ ] Verify list results include the category relation required by the response contract but never derive transaction type from a transaction column.
- [ ] Verify list queries return category data only for categories belonging to the same user.

### Date Range Filtering

- [ ] Verify `start_date` alone returns transactions on or after the supplied start boundary.
- [ ] Verify `end_date` alone returns transactions on or before the supplied end boundary.
- [ ] Verify both `start_date` and `end_date` return only transactions inside the requested range.
- [ ] Verify a transaction exactly at `start_date` is included when the contract defines the start boundary as inclusive.
- [ ] Verify a transaction exactly at `end_date` is included when the contract defines the end boundary as inclusive.
- [ ] Verify timestamps with different timezone offsets are compared by their absolute instant, not by their raw string representation.
- [ ] Verify a date-only filter is converted consistently to the documented timezone and boundary semantics.
- [ ] Verify `start_date` later than `end_date` is rejected as an invalid filter rather than returning misleading data.
- [ ] Verify malformed `start_date` returns a classified validation error.
- [ ] Verify malformed `end_date` returns a classified validation error.
- [ ] Verify an empty date parameter is treated according to the API contract and is not silently interpreted as an arbitrary date.
- [ ] Verify date filters are applied before `LIMIT` and `OFFSET`.
- [ ] Verify date filtering remains scoped to the caller’s `user_id`.
- [ ] Verify date filtering does not accidentally include transactions with `transaction_date = NULL`; the schema should prevent such rows.

### Update Queries

- [ ] Verify updating a transaction owned by the caller changes only the requested mutable fields.
- [ ] Verify update SQL includes both `transaction_id` and executing `user_id`.
- [ ] Verify updating a transaction owned by another user returns `ErrNotFound` or the protected-access error and changes zero rows.
- [ ] Verify updating a non-existent transaction returns `ErrNotFound`.
- [ ] Verify updating `merchant_name` persists the new non-null value.
- [ ] Verify updating `amount` to a positive numeric value succeeds.
- [ ] Verify updating `amount` to zero fails with the positive-amount constraint.
- [ ] Verify updating `amount` to a negative value fails with the positive-amount constraint.
- [ ] Verify updating `amount` to `NULL` fails with the `NOT NULL` constraint if partial updates permit that SQL path.
- [ ] Verify updating `transaction_date` persists the supplied timestamp and timezone.
- [ ] Verify updating `category_id` succeeds only when the new category exists and belongs to the same executing user.
- [ ] Verify changing `category_id` to another user’s category changes zero rows and does not leak category information.
- [ ] Verify updating `receipt_image_url` to a valid URL persists it.
- [ ] Verify clearing `receipt_image_url` persists SQL `NULL` when explicitly requested.
- [ ] Verify updating `notes` persists the supplied text.
- [ ] Verify clearing `notes` persists SQL `NULL` when explicitly requested.
- [ ] Verify update SQL does not permit changing `user_id`.
- [ ] Verify update SQL does not accept or persist a transaction `type`.
- [ ] Verify a database failure during update does not return the updated transaction.
- [ ] Verify an update that violates a foreign key or check constraint leaves the original row unchanged.

### Delete Queries

- [ ] Verify deleting a transaction owned by the caller removes exactly one row.
- [ ] Verify delete SQL includes both `transaction_id` and executing `user_id`.
- [ ] Verify deleting another user’s transaction affects zero rows and returns the protected not-found/forbidden result.
- [ ] Verify deleting a non-existent transaction returns `ErrNotFound`.
- [ ] Verify deleting the same transaction twice returns not-found on the second call.
- [ ] Verify deletion does not delete or modify the related category.
- [ ] Verify deletion does not delete unrelated transactions belonging to the same user.
- [ ] Verify deletion does not delete transactions belonging to other users with the same merchant, amount, or date.
- [ ] Verify a database failure during deletion is returned and the transaction remains present.
- [ ] Verify any embedding cleanup behavior matches the schema design and does not attempt to delete a separate vector record that does not exist.

### Embedding Persistence and pgvector

- [ ] Verify `SaveEmbedding(transactionID, userID, embedding)` updates only the transaction identified by both IDs.
- [ ] Verify saving an embedding for a transaction owned by another user affects zero rows and returns a protected error.
- [ ] Verify saving an embedding for a non-existent transaction returns `ErrNotFound`.
- [ ] Verify saving a valid embedding stores the exact vector values in the `embedding` column.
- [ ] Verify vector component ordering is preserved.
- [ ] Verify the configured embedding dimension is validated.
- [ ] Verify an empty vector is rejected.
- [ ] Verify a vector containing `NaN`, positive infinity, or negative infinity is rejected before database persistence.
- [ ] Verify a vector containing an invalid numeric literal returns an error.
- [ ] Verify a pgvector write failure is returned without changing other transaction fields.
- [ ] Verify saving an embedding does not modify `merchant_name`, `amount`, `transaction_date`, `category_id`, `receipt_image_url`, or `notes`.
- [ ] Verify an embedding can be saved after initial transaction creation when the initial `embedding` is `NULL`.
- [ ] Verify a repeated embedding save replaces the previous vector only when replacement is explicitly supported.
- [ ] Verify concurrent embedding saves do not corrupt the vector or unrelated columns.
- [ ] Verify a transaction deleted before its background embedding save causes the save to return not-found without recreating the transaction.
- [ ] Verify pgvector similarity queries used by financial-history search filter by `user_id` before returning nearest neighbors.
- [ ] Verify vector search cannot return another user’s transaction even when its vector is more similar.
- [ ] Verify vector search excludes deleted transactions.
- [ ] Verify vector search returns category information through the user-scoped category relation.
- [ ] Verify vector search handles transactions with `NULL` embeddings without causing the entire query to fail.
- [ ] Verify vector search limit and offset are parameterized and bounded.
- [ ] Verify similarity ordering is deterministic for equal distances.
- [ ] Verify no transaction embedding is generated or persisted as a side effect of a failed database insert.

### Transactional and Concurrency Behavior

- [ ] Verify category validation and transaction insertion cannot be separated in a way that allows a category ownership race to bypass isolation.
- [ ] Verify a failed transaction insert does not leave an orphaned embedding row or background persistence record.
- [ ] Verify two concurrent creates for the same user produce two distinct transaction UUIDs.
- [ ] Verify two concurrent creates cannot cause one transaction’s embedding to be saved onto another transaction.
- [ ] Verify a concurrent delete and embedding save never results in an embedding attached to a different transaction.
- [ ] Verify repository context cancellation stops the query and returns the context error.
- [ ] Verify a repository timeout does not return partial rows as a successful result.
- [ ] Verify rows and database resources are closed on scan errors and query failures.

## 2. Service Layer Tests (`service_test.go`)

### Create Transaction Business Rules

- [ ] Verify creating a transaction with a valid authenticated `userID`, valid owned `categoryID`, non-empty merchant name, positive amount, and valid transaction date calls `mockRepo.CreateTransaction` exactly once with the expected values.
- [ ] Verify a successful create returns the transaction returned by `mockRepo` and does not alter its generated UUID.
- [ ] Verify creating a transaction with `amount = 50.00` succeeds.
- [ ] Verify creating a transaction with `amount = 0` returns a validation error containing exactly `"Amount must be > 0"` and does not call `mockRepo.CreateTransaction`.
- [ ] Verify creating a transaction with a negative amount returns a validation error containing exactly `"Amount must be > 0"` and does not call `mockRepo.CreateTransaction`.
- [ ] Verify creating a transaction with a missing amount is rejected before repository execution.
- [ ] Verify creating a transaction with a non-finite amount is rejected before repository execution.
- [ ] Verify creating a transaction with an empty merchant name returns a validation error and does not call `mockRepo.CreateTransaction`.
- [ ] Verify creating a transaction with whitespace-only merchant name returns a validation error and does not call `mockRepo.CreateTransaction`.
- [ ] Verify the service trims merchant-name whitespace only if that is the defined contract and passes the normalized value to `mockRepo`.
- [ ] Verify creating a transaction with a missing `transaction_date` returns a validation error and does not call `mockRepo.CreateTransaction`.
- [ ] Verify creating a transaction with an invalid timestamp returns a validation error and does not call `mockRepo.CreateTransaction`.
- [ ] Verify creating a transaction with an invalid `userID` is rejected before repository execution.
- [ ] Verify creating a transaction with an invalid `categoryID` is rejected before repository execution.
- [ ] Verify a category ID that does not exist returns a category-not-found error and causes zero transaction inserts.
- [ ] Verify a category ID that belongs to another user returns a protected category error and causes zero transaction inserts.
- [ ] Verify the service never trusts a client-provided transaction type.
- [ ] Verify income/expense classification is obtained from the category relation and is not inferred from amount sign.
- [ ] Verify a positive amount remains positive for both expense and income categories; income/expense behavior is determined by category metadata.
- [ ] Verify optional `receipt_image_url = nil` is accepted.
- [ ] Verify optional `notes = nil` is accepted.
- [ ] Verify notes containing Unicode, line breaks, punctuation, and apostrophes are passed unchanged or according to the normalization contract.
- [ ] Verify a merchant name containing SQL metacharacters is passed as data and does not bypass validation or repository isolation.
- [ ] Verify a receipt URL containing query parameters is passed unchanged or normalized according to the URL contract.
- [ ] Verify an oversized merchant name is rejected with a service validation error before the repository call when the service owns length validation.
- [ ] Verify an oversized notes payload is rejected before the repository call when a service limit exists.
- [ ] Verify a repository category-validation error is mapped to the correct service error without exposing another user’s category.
- [ ] Verify a repository insert error is returned and no success response is produced.
- [ ] Verify a repository insert failure does not trigger `mockEmbedding`.
- [ ] Verify the service does not call `mockLLM` during a direct manual CRUD create.

### Category Relation Checks

- [ ] Verify the service requests category details using both `categoryID` and authenticated `userID`.
- [ ] Verify the service passes the category name returned by the repository into the created transaction’s domain representation when required.
- [ ] Verify the service uses the category’s persisted income/expense classification rather than accepting a client-supplied classification.
- [ ] Verify a category with the correct UUID but a mismatched owner is rejected even when the category name matches the requested name.
- [ ] Verify a category name that does not uniquely identify the user’s category cannot cause selection of another user’s category.
- [ ] Verify a missing category name returned by a malformed mock repository response is handled as an internal/data-integrity error.
- [ ] Verify a repository category lookup failure prevents `CreateTransaction` from being called.
- [ ] Verify changing a transaction’s category repeats the ownership check against the executing user.
- [ ] Verify updating a transaction to an unowned category causes zero update calls.
- [ ] Verify deleting a transaction does not require or modify category ownership data beyond the transaction ownership check.

### Asynchronous Embedding Trigger

- [ ] Verify every successful manual transaction creation triggers exactly one asynchronous embedding generation.
- [ ] Verify every successful AI-created transaction triggers exactly one asynchronous embedding generation.
- [ ] Verify a failed transaction validation triggers zero calls to `mockEmbedding`.
- [ ] Verify a category ownership failure triggers zero calls to `mockEmbedding`.
- [ ] Verify a repository insert failure triggers zero calls to `mockEmbedding`.
- [ ] Verify the embedding goroutine receives the newly created transaction ID, authenticated user ID, merchant name, amount, notes, and resolved category name.
- [ ] Verify the embedding input text contains `merchant_name`.
- [ ] Verify the embedding input text contains the exact transaction amount in the defined canonical format.
- [ ] Verify the embedding input text contains notes when notes are present.
- [ ] Verify the embedding input text omits, rather than invents, notes when notes are absent.
- [ ] Verify the embedding input text contains the resolved category name.
- [ ] Verify the embedding input does not contain an untrusted or client-supplied transaction type.
- [ ] Verify `mockEmbedding.Generate` is called exactly once per successful transaction.
- [ ] Verify the generated vector is passed exactly once to `mockRepo.SaveEmbedding`.
- [ ] Verify `mockRepo.SaveEmbedding` receives the created transaction ID and authenticated user ID.
- [ ] Verify embedding generation occurs asynchronously and the create service returns the transaction without waiting for the embedding provider’s network latency.
- [ ] Verify the test waits deterministically for the goroutine using a channel, `WaitGroup`, or equivalent synchronization rather than using a fixed sleep.
- [ ] Verify `mockEmbedding` errors do not change an already successful create result into a failure.
- [ ] Verify an embedding-generation error is logged or reported through the defined background error mechanism.
- [ ] Verify an embedding-save error does not cause a second embedding generation attempt unless retries are explicitly configured.
- [ ] Verify the goroutine does not invoke `SaveEmbedding` when embedding generation returns an empty or invalid vector.
- [ ] Verify the goroutine does not save an embedding under a different transaction ID.
- [ ] Verify the goroutine does not save an embedding under a different user ID.
- [ ] Verify the goroutine remains safe when the request context is canceled immediately after the HTTP response; it must use the configured background context or job context if required by the design.
- [ ] Verify a panic or unexpected failure inside the background embedding task does not crash the process.
- [ ] Verify concurrent successful creates each trigger one embedding call and each vector is associated with the correct transaction.
- [ ] Verify `go test -race` reports no data race in embedding-trigger tests or mock call tracking.
- [ ] Verify no embedding task is launched for retries that did not result in a new successful transaction.
- [ ] Verify embedding generation is not synchronously invoked more than once by both the service and a repository hook.

### Read, Filter, and Pagination Services

- [ ] Verify `GetTransaction` passes the authenticated user ID and transaction ID to `mockRepo.GetTransactionByID`.
- [ ] Verify `GetTransaction` returns a repository not-found result as a service not-found result without leaking another user’s data.
- [ ] Verify `ListTransactions` passes the authenticated user ID, limit, offset, start date, and end date to `mockRepo.ListTransactions`.
- [ ] Verify default limit and offset values are applied when omitted.
- [ ] Verify a negative limit returns a validation error and causes zero repository list calls.
- [ ] Verify a negative offset returns a validation error and causes zero repository list calls.
- [ ] Verify a zero limit is rejected or normalized according to the contract, with the expected behavior covered explicitly.
- [ ] Verify an excessively large limit is rejected or capped according to the contract.
- [ ] Verify `start_date > end_date` returns a validation error and causes zero repository list calls.
- [ ] Verify malformed start and end dates are rejected before repository execution.
- [ ] Verify inclusive date-boundary behavior is passed correctly to the repository.
- [ ] Verify an empty repository result is returned as an empty collection.
- [ ] Verify repository results are not replaced with data from another user.
- [ ] Verify a repository failure during list is returned as an internal service error.

### Update and Delete Services

- [ ] Verify updating an owned transaction with valid fields calls `mockRepo.UpdateTransaction` exactly once.
- [ ] Verify updating an unowned transaction returns a protected not-found/forbidden error and makes zero update calls.
- [ ] Verify updating a missing transaction returns not-found.
- [ ] Verify updating amount to zero returns `"Amount must be > 0"` and makes zero update calls.
- [ ] Verify updating amount to a negative value returns `"Amount must be > 0"` and makes zero update calls.
- [ ] Verify updating merchant name to empty or whitespace-only returns a validation error and makes zero update calls.
- [ ] Verify updating transaction date to an invalid value returns a validation error and makes zero update calls.
- [ ] Verify updating category ID invokes a user-scoped category ownership check before the update.
- [ ] Verify updating to another user’s category makes zero update calls.
- [ ] Verify a partial update changes only explicitly provided fields.
- [ ] Verify an explicit request to clear notes or receipt URL is distinguishable from an omitted field.
- [ ] Verify update requests cannot change `user_id`.
- [ ] Verify update requests cannot set a transaction `type`.
- [ ] Verify a successful delete of an owned transaction calls `mockRepo.DeleteTransaction` exactly once.
- [ ] Verify deleting an unowned transaction returns a protected not-found/forbidden error.
- [ ] Verify deleting a missing transaction returns not-found.
- [ ] Verify a repository delete failure is propagated and no success result is returned.
- [ ] Verify deleting a transaction does not trigger `mockEmbedding`.
- [ ] Verify updating a transaction does not trigger an embedding task unless the product contract explicitly requires re-embedding; cover the selected behavior explicitly.

### LLM Routing and Function Calling

- [ ] Verify the single chat service sends a normal finance statement such as `"Spent 50k on coffee at AWANN COSTA"` to `mockLLM` exactly once.
- [ ] Verify the LLM response for the spending statement selects the `record_transaction` function.
- [ ] Verify the `record_transaction` function arguments decode into `merchant_name`, `amount`, `transaction_date`, `category`, and `notes`.
- [ ] Verify the amount `"50k"` is normalized to the agreed numeric value before `CreateTransaction` is called.
- [ ] Verify the merchant name `"AWANN COSTA"` is preserved in the create request.
- [ ] Verify a function call with an explicit transaction date uses that date.
- [ ] Verify a function call without a transaction date applies the defined current-time behavior and tests the resulting timestamp source.
- [ ] Verify a function call with notes passes those notes into the CRUD create path.
- [ ] Verify a function call with a category name resolves that category for the authenticated user only.
- [ ] Verify a category name matching another user’s category cannot be used to create a transaction for the caller.
- [ ] Verify an unknown category name returns a controlled category error and does not call `mockRepo.CreateTransaction`.
- [ ] Verify malformed `record_transaction` arguments return a controlled validation error and do not call `mockRepo.CreateTransaction`.
- [ ] Verify missing `merchant_name` in the function arguments is rejected.
- [ ] Verify missing or non-positive `amount` in the function arguments returns `"Amount must be > 0"` where applicable.
- [ ] Verify malformed `transaction_date` in the function arguments is rejected.
- [ ] Verify unexpected fields in the function arguments are ignored or rejected according to the strict JSON contract.
- [ ] Verify the AI route reuses the same CRUD validation and repository path as manual creation.
- [ ] Verify AI-created transactions trigger exactly one `mockEmbedding` call after the CRUD insert succeeds.
- [ ] Verify a failed AI CRUD create does not trigger `mockEmbedding`.

### Receipt Image Processing

- [ ] Verify a receipt image upload is routed to the receipt extraction flow and not treated as a general conversation.
- [ ] Verify the LLM receives the image or image reference in the expected format.
- [ ] Verify a receipt containing merchant, amount, date, category, and notes produces a valid `record_transaction` call.
- [ ] Verify the extracted merchant name, amount, date, category, and notes are passed through the same service validation as manual input.
- [ ] Verify an unsupported image content type is rejected without LLM or repository execution.
- [ ] Verify an empty image upload is rejected without LLM or repository execution.
- [ ] Verify an oversized image upload is rejected before LLM execution.
- [ ] Verify a corrupt image causes a controlled extraction error and no transaction insert.
- [ ] Verify an image with unreadable amount data does not result in a guessed zero or negative amount.
- [ ] Verify an image with multiple receipts is rejected or handled according to the defined single-receipt contract.
- [ ] Verify an image extraction that returns an unknown category does not create a transaction.
- [ ] Verify a successful receipt-based creation triggers exactly one embedding generation.

### Financial History Query and RAG Routing

- [ ] Verify an analytical prompt such as `"What is my biggest expense this month?"` selects `query_financial_history`.
- [ ] Verify analytical prompts do not call `mockRepo.CreateTransaction`.
- [ ] Verify analytical prompts do not call `mockRepo.UpdateTransaction` or `mockRepo.DeleteTransaction`.
- [ ] Verify the query service creates an embedding for the user’s question exactly once when vector search requires query embeddings.
- [ ] Verify the query embedding is passed to the user-scoped vector search repository method.
- [ ] Verify vector search receives the authenticated user ID as a mandatory filter.
- [ ] Verify vector search receives the parsed date range for `"this month"`.
- [ ] Verify vector search results contain only the caller’s transactions.
- [ ] Verify the service computes or requests the biggest expense using category classification and amount, not by assuming all transactions are expenses.
- [ ] Verify income transactions are excluded from an expense-only question according to the category’s income/expense relation.
- [ ] Verify a question requesting income uses category classification rather than a transaction `type` field.
- [ ] Verify an empty vector-search result returns a useful no-data response and not an arbitrary transaction.
- [ ] Verify a vector-search repository error is returned as a controlled RAG failure.
- [ ] Verify an embedding-provider error for the question prevents vector search and returns a controlled error.
- [ ] Verify a malformed analytical LLM function call is rejected without vector search.
- [ ] Verify the RAG answer cannot include transactions belonging to another user even if the LLM prompt or retrieved context attempts to request them.
- [ ] Verify the service does not create a transaction when the user asks a historical or analytical question.
- [ ] Verify a prompt combining an analytical question with an unrelated mutation is routed according to the explicit function-call policy and does not execute both tools accidentally.

### Fallback and Circuit Breaker

- [ ] Verify a general non-finance question such as `"What is the weather today?"` produces a graceful fallback response.
- [ ] Verify the fallback path executes zero CRUD repository calls.
- [ ] Verify the fallback path executes zero vector-search calls.
- [ ] Verify the fallback path executes zero embedding-generation calls.
- [ ] Verify an unsupported intent does not produce a fabricated transaction.
- [ ] Verify an unsupported intent does not produce fabricated financial-history results.
- [ ] Verify an LLM timeout returns a controlled service error or fallback response according to the circuit-breaker contract.
- [ ] Verify an LLM provider 5xx error does not execute a partially parsed tool call.
- [ ] Verify an LLM response with no tool call and no usable text follows the fallback/error contract.
- [ ] Verify an LLM response containing multiple conflicting tool calls is rejected or resolved according to the explicit policy without duplicate transaction creation.
- [ ] Verify the circuit breaker opens after the configured number of consecutive LLM failures.
- [ ] Verify once open, the circuit breaker prevents calls to `mockLLM`.
- [ ] Verify once open, finance requests receive the configured degraded response without CRUD or RAG execution.
- [ ] Verify the circuit breaker resets only after the configured cooldown and successful probe behavior.
- [ ] Verify a failed circuit-breaker probe does not execute any transaction tool.
- [ ] Verify concurrent requests observe the same circuit-breaker state safely.
- [ ] Verify circuit-breaker state and mock call counters are race-free under `go test -race`.

## 3. Handler Layer Tests (`handler_test.go`)

### Authentication and Authorization Middleware

- [ ] Verify a request without an authentication token returns HTTP `401 Unauthorized`.
- [ ] Verify a request with an invalid or expired token returns HTTP `401 Unauthorized`.
- [ ] Verify a request with a valid token injects the authenticated user UUID into the request context.
- [ ] Verify a request attempting to provide a different `user_id` in the JSON body cannot override the authenticated user.
- [ ] Verify a request attempting to provide a different `user_id` in query parameters cannot override the authenticated user.
- [ ] Verify `GET /transactions/{id}` for another user’s transaction returns HTTP `403 Forbidden` or HTTP `404 Not Found`, according to the selected anti-enumeration policy.
- [ ] Verify `PATCH` or `PUT` for another user’s transaction returns HTTP `403 Forbidden` or HTTP `404 Not Found`.
- [ ] Verify `DELETE /transactions/{id}` for another user’s transaction returns HTTP `403 Forbidden` or HTTP `404 Not Found`.
- [ ] Verify cross-user responses do not include the other transaction’s merchant name, amount, category, notes, receipt URL, or embedding.
- [ ] Verify `GET /transactions` never returns another user’s transaction, even when query filters match it.
- [ ] Verify chat requests always use the authenticated user ID for both `record_transaction` and `query_financial_history`.
- [ ] Verify an authenticated user cannot use a category ID or category name to access another user’s category metadata.

### Create Transaction HTTP Contract

- [ ] Verify `POST /transactions` with a valid JSON payload returns HTTP `201 Created`.
- [ ] Verify a successful create response contains the generated transaction UUID.
- [ ] Verify a successful create response contains `user_id` equal to the authenticated user, regardless of any body `user_id`.
- [ ] Verify a successful create response contains `category_id`, `merchant_name`, `amount`, `transaction_date`, `receipt_image_url`, and `notes` according to the response contract.
- [ ] Verify the response does not expose an unsupported transaction `type` field sourced from the request.
- [ ] Verify a valid payload with `amount = 50.00` is accepted.
- [ ] Verify a payload with `amount = 0` returns HTTP `400 Bad Request` and an error message containing exactly `"Amount must be > 0"`.
- [ ] Verify a payload with a negative amount returns HTTP `400 Bad Request` and an error message containing exactly `"Amount must be > 0"`.
- [ ] Verify a payload missing `amount` returns HTTP `400 Bad Request` with a specific amount-required validation message.
- [ ] Verify a payload with a non-numeric amount returns HTTP `400 Bad Request`.
- [ ] Verify a payload with an amount encoded in an unsupported JSON type returns HTTP `400 Bad Request`.
- [ ] Verify a payload with an amount too large for the configured numeric representation returns HTTP `400 Bad Request`, not HTTP `500`.
- [ ] Verify a payload with an empty `merchant_name` returns HTTP `400 Bad Request`.
- [ ] Verify a payload with a whitespace-only `merchant_name` returns HTTP `400 Bad Request`.
- [ ] Verify a payload missing `merchant_name` returns HTTP `400 Bad Request`.
- [ ] Verify a payload with an invalid `transaction_date` returns HTTP `400 Bad Request`.
- [ ] Verify a payload missing `transaction_date` returns HTTP `400 Bad Request`.
- [ ] Verify a payload with a valid RFC3339 timestamp is decoded with the correct timezone instant.
- [ ] Verify a payload with an invalid `category_id` UUID returns HTTP `400 Bad Request`.
- [ ] Verify a payload with a non-existent category returns HTTP `404 Not Found` or the documented category validation status.
- [ ] Verify a payload using another user’s category returns HTTP `403 Forbidden` or `404 Not Found` and does not create a transaction.
- [ ] Verify a payload with `receipt_image_url = null` is accepted.
- [ ] Verify a payload with `notes = null` is accepted.
- [ ] Verify a payload with valid Unicode merchant and notes values is decoded without corruption.
- [ ] Verify a payload with unknown JSON fields is rejected with HTTP `400 Bad Request` when strict decoding is enabled.
- [ ] Verify a payload with duplicate JSON keys follows the defined decoder policy and cannot override authenticated ownership.
- [ ] Verify an empty request body returns HTTP `400 Bad Request`.
- [ ] Verify malformed JSON returns HTTP `400 Bad Request`.
- [ ] Verify a JSON array sent where an object is expected returns HTTP `400 Bad Request`.
- [ ] Verify a request with an unsupported `Content-Type` returns HTTP `415 Unsupported Media Type` when content-type enforcement is enabled.
- [ ] Verify a repository/database failure during create returns HTTP `500 Internal Server Error`.
- [ ] Verify a successful HTTP `201` response is returned even if asynchronous embedding generation later fails, provided the transaction insert succeeded.
- [ ] Verify the create handler does not wait for embedding completion before writing the response.
- [ ] Verify the create handler does not expose embedding-provider credentials, raw provider errors, or stack traces.

### Get-by-ID HTTP Contract

- [ ] Verify `GET /transactions/{id}` for an owned transaction returns HTTP `200 OK`.
- [ ] Verify the response JSON decodes into the expected transaction DTO.
- [ ] Verify the response amount preserves numeric precision and does not become an inaccurate floating-point value.
- [ ] Verify the response transaction date preserves the expected timezone-aware representation.
- [ ] Verify nullable `receipt_image_url`, `notes`, and `embedding` fields are encoded according to the JSON contract.
- [ ] Verify an invalid transaction UUID returns HTTP `400 Bad Request`.
- [ ] Verify a syntactically valid but non-existent transaction UUID returns HTTP `404 Not Found`.
- [ ] Verify another user’s transaction returns HTTP `403 Forbidden` or `404 Not Found`, according to the selected policy.
- [ ] Verify a repository failure returns HTTP `500 Internal Server Error`.
- [ ] Verify a cross-user response body does not contain transaction details.
- [ ] Verify the handler does not accept a body `user_id` or query `user_id` as an authorization override.

### List, Pagination, and Date Filters HTTP Contract

- [ ] Verify `GET /transactions` for an authenticated user returns HTTP `200 OK`.
- [ ] Verify the response contains an empty JSON array, not `null`, when the user has no transactions.
- [ ] Verify `GET /transactions?limit=20&offset=0` passes `limit = 20` and `offset = 0` to the service.
- [ ] Verify omitted `limit` and `offset` use the documented defaults.
- [ ] Verify `limit=1` returns at most one transaction.
- [ ] Verify `offset=1` excludes the first row according to the deterministic ordering contract.
- [ ] Verify a negative `limit` returns HTTP `400 Bad Request`.
- [ ] Verify a negative `offset` returns HTTP `400 Bad Request`.
- [ ] Verify a non-numeric `limit` returns HTTP `400 Bad Request`.
- [ ] Verify a non-numeric `offset` returns HTTP `400 Bad Request`.
- [ ] Verify `limit=0` returns HTTP `400 Bad Request` or the documented normalization behavior.
- [ ] Verify a limit above the maximum permitted value returns HTTP `400 Bad Request` or is capped according to the documented contract.
- [ ] Verify `start_date` alone filters transactions on or after the supplied boundary.
- [ ] Verify `end_date` alone filters transactions on or before the supplied boundary.
- [ ] Verify both `start_date` and `end_date` are passed to the service correctly.
- [ ] Verify a transaction exactly on an inclusive start boundary is included.
- [ ] Verify a transaction exactly on an inclusive end boundary is included.
- [ ] Verify timezone offsets in query parameters are parsed correctly.
- [ ] Verify malformed `start_date` returns HTTP `400 Bad Request`.
- [ ] Verify malformed `end_date` returns HTTP `400 Bad Request`.
- [ ] Verify `start_date` later than `end_date` returns HTTP `400 Bad Request`.
- [ ] Verify empty date query values return HTTP `400 Bad Request` or the documented omitted-filter behavior.
- [ ] Verify query parameters cannot include another user’s `user_id` to broaden the result set.
- [ ] Verify list results contain only transactions for the authenticated user.
- [ ] Verify a service/database list failure returns HTTP `500 Internal Server Error`.
- [ ] Verify date filters are applied before pagination by asserting the returned page contains only rows inside the requested range.
- [ ] Verify the handler does not return a transaction `type` field unless it is explicitly derived from the user-owned category relation.

### Update Transaction HTTP Contract

- [ ] Verify updating an owned transaction with valid JSON returns HTTP `200 OK` and the updated transaction, or HTTP `204 No Content` if that is the defined contract.
- [ ] Verify a valid partial update changes only fields included in the request.
- [ ] Verify a malformed JSON update body returns HTTP `400 Bad Request`.
- [ ] Verify an empty update object returns HTTP `400 Bad Request` when at least one mutable field is required.
- [ ] Verify `amount = 0` returns HTTP `400 Bad Request` with exactly `"Amount must be > 0"`.
- [ ] Verify a negative amount returns HTTP `400 Bad Request` with exactly `"Amount must be > 0"`.
- [ ] Verify an invalid merchant name returns HTTP `400 Bad Request`.
- [ ] Verify an invalid transaction date returns HTTP `400 Bad Request`.
- [ ] Verify an invalid category UUID returns HTTP `400 Bad Request`.
- [ ] Verify an unowned category returns HTTP `403 Forbidden` or `404 Not Found`.
- [ ] Verify updating another user’s transaction returns HTTP `403 Forbidden` or `404 Not Found`.
- [ ] Verify updating a non-existent transaction returns HTTP `404 Not Found`.
- [ ] Verify an update body containing `user_id` is rejected or the field is ignored without changing ownership.
- [ ] Verify an update body containing `type` is rejected or ignored and never changes category-derived classification.
- [ ] Verify explicitly setting `notes: null` clears notes according to the API contract.
- [ ] Verify explicitly setting `receipt_image_url: null` clears the receipt URL according to the API contract.
- [ ] Verify a database/service failure during update returns HTTP `500 Internal Server Error`.
- [ ] Verify an update response does not expose another user’s category or transaction information.

### Delete Transaction HTTP Contract

- [ ] Verify deleting an owned transaction returns HTTP `204 No Content` with an empty response body, or the explicitly documented success status.
- [ ] Verify deleting an owned transaction removes it from a subsequent `GET /transactions/{id}` response.
- [ ] Verify deleting a non-existent transaction returns HTTP `404 Not Found`.
- [ ] Verify deleting another user’s transaction returns HTTP `403 Forbidden` or `404 Not Found`.
- [ ] Verify an invalid transaction UUID returns HTTP `400 Bad Request`.
- [ ] Verify a repository/database deletion failure returns HTTP `500 Internal Server Error`.
- [ ] Verify a delete request cannot specify a different `user_id` to bypass authorization.
- [ ] Verify deleting a transaction does not delete the related category.
- [ ] Verify the delete handler does not return a success response when the repository reports zero affected rows.

### Chat and AI Tool HTTP Contract

- [ ] Verify an authenticated chat request containing `"Spent 50k on coffee at AWANN COSTA"` returns the documented successful AI response status and includes the created transaction or creation confirmation.
- [ ] Verify the chat response for a successful `record_transaction` call includes the created transaction UUID.
- [ ] Verify the chat response does not claim success when the CRUD create fails.
- [ ] Verify the `record_transaction` tool payload is decoded into `merchant_name`, `amount`, `transaction_date`, `category`, and `notes`.
- [ ] Verify malformed tool arguments return HTTP `400 Bad Request` or the documented controlled AI validation status.
- [ ] Verify a non-positive tool amount returns HTTP `400 Bad Request` with `"Amount must be > 0"` where the chat API exposes validation errors directly.
- [ ] Verify an unknown category from a tool call returns HTTP `404 Not Found` or the documented category error status.
- [ ] Verify another user’s category cannot be selected through a category name or category UUID in a tool call.
- [ ] Verify a receipt image upload reaches the receipt extraction path.
- [ ] Verify a valid receipt upload returns the created transaction confirmation.
- [ ] Verify an empty, corrupt, unsupported, or oversized receipt image returns HTTP `400 Bad Request` or `415 Unsupported Media Type` as appropriate.
- [ ] Verify an analytical prompt such as `"What is my biggest expense this month?"` invokes `query_financial_history` and returns HTTP `200 OK`.
- [ ] Verify an analytical prompt does not create, update, or delete a transaction.
- [ ] Verify the analytical response is based only on the authenticated user’s RAG context.
- [ ] Verify a query for biggest expense excludes income categories according to category relation metadata.
- [ ] Verify an analytical request with no matching transactions returns a controlled no-data response, not HTTP `500`.
- [ ] Verify a vector-search or question-embedding failure returns the documented AI dependency error without exposing internal details.
- [ ] Verify a general non-finance question returns a graceful fallback response with HTTP `200 OK` or the documented non-error status.
- [ ] Verify the fallback response performs zero CRUD operations.
- [ ] Verify the fallback response performs zero vector searches.
- [ ] Verify the fallback response performs zero embedding writes.
- [ ] Verify an LLM timeout returns the configured fallback or dependency error status.
- [ ] Verify an open circuit breaker prevents tool execution and returns the configured degraded response.
- [ ] Verify an LLM response containing conflicting or duplicate tool calls does not create duplicate transactions.
- [ ] Verify unauthenticated chat and receipt requests are blocked by middleware with HTTP `401 Unauthorized`.
- [ ] Verify chat input cannot include a user ID that overrides the authenticated context.
- [ ] Verify AI error responses do not expose raw prompts, database SQL, provider credentials, stack traces, or another user’s financial data.
