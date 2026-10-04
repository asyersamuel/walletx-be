-- name: MatchTransactions :many
-- Explicit casts let sqlc type the table-valued RPC's projection.
SELECT id::uuid AS id, merchant_name::text AS merchant_name,
    COALESCE(category_name::text, '')::text AS category_name, amount::numeric AS amount,
    transaction_date::timestamptz AS transaction_date,
    COALESCE(notes::text, '')::text AS notes, (notes IS NULL)::boolean AS notes_is_null
FROM public.match_walletx_transactions(
    sqlc.arg(query_embedding)::text::extensions.vector,
    sqlc.arg(match_threshold)::float8,
    sqlc.arg(match_count)::integer,
    sqlc.arg(user_id)::uuid
);
