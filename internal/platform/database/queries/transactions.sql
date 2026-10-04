-- name: CreateTransaction :one
-- The guarded_category CTE enforces relational integrity inside the very same
-- statement as the INSERT: the category must exist AND belong to the executing
-- user. If the guard returns zero rows, the INSERT ... SELECT inserts nothing
-- and this :one query returns pgx.ErrNoRows, which the repository maps to
-- apperrors.ErrNotFound without revealing whether the category is missing or
-- owned by another user. This removes the check-then-insert race between
-- category validation and transaction insertion.
-- The embedding column is intentionally omitted: it stays NULL at creation
-- time and is filled asynchronously by the background embedding job.
with guarded_category as (
    select id
    from public.categories
    where id = sqlc.arg(category_id)
      and user_id = sqlc.arg(user_id)
    for share
)
insert into public.transactions (
    user_id, category_id, merchant_name, amount,
    transaction_date, receipt_image_url, notes
)
select
    sqlc.arg(user_id),
    sqlc.arg(category_id),
    sqlc.arg(merchant_name),
    sqlc.arg(amount),
    sqlc.arg(transaction_date),
    sqlc.narg(receipt_image_url),
    sqlc.narg(notes)
from guarded_category
returning id, user_id, category_id, merchant_name, amount,
    transaction_date, receipt_image_url, notes,
    created_at, updated_at;

-- name: GetTransactionByID :one
-- Ownership isolation is enforced in the WHERE clause: the user_id predicate
-- is combined with the transaction id, so requesting a transaction that exists
-- but belongs to another user returns zero rows (pgx.ErrNoRows -> ErrNotFound).
-- This never leaks whether the id exists. The embedding column is intentionally
-- omitted; it is loaded by a later embedding iteration.
select id, user_id, category_id, merchant_name, amount, transaction_date,
    receipt_image_url, notes, created_at, updated_at
from public.transactions
where id = sqlc.arg(id)
  and user_id = sqlc.arg(user_id)
limit 1;

-- name: ListTransactions :many
-- Scope strictly to the caller. The user_id predicate lives in WHERE, so it is
-- applied BEFORE LIMIT/OFFSET: another user's rows can never shift page
-- boundaries or leak into results. Optional inclusive date bounds are compared
-- against transaction_date by absolute instant (timestamptz), so rows exactly
-- on start_date or end_date are included. A NULL bound means unbounded.
-- Ordering is deterministic and stable: transaction_date desc, then id desc as
-- a tie-breaker, so pagination never duplicates or skips rows across pages even
-- when multiple transactions share the same transaction_date.
select id, user_id, category_id, merchant_name, amount, transaction_date,
    receipt_image_url, notes, created_at, updated_at
from public.transactions
where user_id = sqlc.arg(user_id)
  and (
      sqlc.narg(start_date)::timestamptz is null
      or transaction_date >= sqlc.narg(start_date)::timestamptz
  )
  and (
      sqlc.narg(end_date)::timestamptz is null
      or transaction_date <= sqlc.narg(end_date)::timestamptz
  )
order by transaction_date desc, id desc
limit sqlc.arg(row_limit)
offset sqlc.arg(row_offset);

-- name: UpdateTransaction :one
-- Partial, ownership-safe update. Each mutable column is written only when its
-- matching set_<field> flag is true; otherwise the existing value is kept:
--     col = CASE WHEN $set_flag THEN $value ELSE col END
-- So callers send only the fields they want to change (a "full update" simply
-- sets every flag). The flags are derived in the repository from which domain
-- fields are non-nil, so this is a true partial update.
-- Why CASE + flags instead of coalesce(narg, col): amount is numeric, and sqlc
-- maps a *nullable* numeric parameter to pgtype.Numeric, which pgx cannot
-- encode from a bare float64 (its encoder requires the *big.Int field). Using a
-- NON-nullable float64 value guarded by a boolean flag keeps amount a plain
-- float64 (identical to CreateTransaction) and avoids pgtype.Numeric entirely.
-- Security / isolation:
--   * WHERE id = $id AND user_id = $user -> a transaction owned by another user
--     (or a missing one) matches zero rows => ErrNoRows => ErrNotFound.
--   * The category guard runs only when set_category_id is true: then the new
--     category must exist AND belong to the same user (correlated EXISTS on
--     transactions.user_id); otherwise zero rows are updated. Both failure
--     modes collapse to ErrNotFound, so a caller can neither attach another
--     user's category nor learn whether it exists.
--   * user_id is never in the SET list, so ownership can never be reassigned.
--   * The CHECK (amount > 0) constraint still applies to any supplied amount.
-- updated_at is always refreshed; embedding is left untouched (later iteration).
update public.transactions
set merchant_name     = case when sqlc.arg(set_merchant_name)::boolean then sqlc.arg(merchant_name) else merchant_name end,
    amount            = case when sqlc.arg(set_amount)::boolean then sqlc.arg(amount) else amount end,
    transaction_date  = case when sqlc.arg(set_transaction_date)::boolean then sqlc.arg(transaction_date) else transaction_date end,
    category_id       = case when sqlc.arg(set_category_id)::boolean then sqlc.arg(category_id) else category_id end,
    receipt_image_url = case when sqlc.arg(set_receipt_image_url)::boolean then sqlc.narg(receipt_image_url) else receipt_image_url end,
    notes             = case when sqlc.arg(set_notes)::boolean then sqlc.narg(notes) else notes end,
    updated_at        = now()
where public.transactions.id = sqlc.arg(id)
  and public.transactions.user_id = sqlc.arg(user_id)
  and (
      sqlc.arg(set_category_id)::boolean = false
      or exists (
          select 1
          from public.categories c
          where c.id = sqlc.arg(category_id)
            and c.user_id = public.transactions.user_id
      )
  )
returning id, user_id, category_id, merchant_name, amount,
    transaction_date, receipt_image_url, notes,
    created_at, updated_at;

-- name: DeleteTransaction :execrows
-- Ownership isolation: both id AND user_id are required in the WHERE clause.
-- Deleting a transaction owned by another user (or a missing one) affects zero
-- rows, which the repository maps to ErrNotFound. The related category is never
-- touched. Returns the number of rows affected.
delete from public.transactions
where id = sqlc.arg(id)
  and user_id = sqlc.arg(user_id);

-- name: SaveEmbedding :execrows
-- Writes the pre-computed embedding vector for a single transaction that the
-- caller owns. Both id AND user_id are required so a background worker cannot
-- accidentally overwrite another user's embedding even with a valid transaction
-- UUID. Zero rows returned means the transaction is missing or owned by a
-- different user; the repository maps that to ErrNotFound.
--
-- Encoding contract: the Go caller converts []float32 to the pgvector literal
-- text format "[f0,f1,...,f767]" and passes it here as a text parameter. The
-- double cast ::text::extensions.vector lets pgx simple-protocol pass a raw
-- string and lets the database parser perform the type conversion, with no
-- pgvector-go dependency required.
update public.transactions
set embedding   = sqlc.arg(embedding)::text::extensions.vector,
    updated_at  = now()
where public.transactions.id      = sqlc.arg(id)
  and public.transactions.user_id = sqlc.arg(user_id)
  and public.transactions.updated_at = sqlc.arg(updated_at);

-- name: SearchSimilarTransactions :many
-- Retrieves the most similar transactions for a given query embedding, strictly
-- scoped to a single user. The vector distance uses cosine distance (<=>), so
-- similarity = 1 - distance. Only rows whose cosine similarity exceeds
-- match_threshold are returned, ordered best-match first.
--
-- Security: user_id is the first predicate so the planner can use the
-- (user_id, transaction_date) index even before the vector index, guaranteeing
-- no cross-user row is ever scored or returned.
--
-- Optional inclusive date bounds narrow the result set BEFORE ranking so that
-- "what did I spend this month" questions only consider the relevant window.
--
-- The embedding column is NOT returned to the caller; the query result carries
-- business fields only. embedding IS NULL rows are excluded because they have
-- no distance to compute.
--
-- Encoding contract: same text-casting trick as SaveEmbedding.
select
    t.id,
    t.user_id,
    t.category_id,
    t.merchant_name,
    t.amount,
    t.transaction_date,
    t.notes,
    c.name                                               as category_name,
    c.type                                               as category_type,
    (1.0 - (t.embedding <=> sqlc.arg(query_embedding)::text::extensions.vector))::float8 as similarity
from public.transactions t
left join public.categories c
    on c.id = t.category_id
   and c.user_id = t.user_id
where t.user_id   = sqlc.arg(user_id)
  and t.embedding is not null
  and (1.0 - (t.embedding <=> sqlc.arg(query_embedding)::text::extensions.vector))::float8
          >= sqlc.arg(match_threshold)::float8
  and (
      sqlc.narg(start_date)::timestamptz is null
      or t.transaction_date >= sqlc.narg(start_date)::timestamptz
  )
  and (
      sqlc.narg(end_date)::timestamptz is null
      or t.transaction_date <= sqlc.narg(end_date)::timestamptz
  )
order by t.embedding <=> sqlc.arg(query_embedding)::text::extensions.vector
limit sqlc.arg(match_count)::int;
