-- name: CreateTransaction :one
insert into public.transactions (
    user_id, category_id, amount, merchant, note, transaction_date,
    message_id, is_recurring
)
values (
    sqlc.arg(user_id), sqlc.narg(category_id), sqlc.arg(amount),
    sqlc.arg(merchant), coalesce(sqlc.narg(note)::text, ''),
    sqlc.arg(transaction_date), sqlc.narg(message_id), sqlc.arg(is_recurring)
)
returning id, user_id, category_id, amount, merchant, note,
    transaction_date, message_id, is_recurring, created_at, updated_at,
    deleted_at;

-- name: GetTransactionByID :one
select id, user_id, category_id, amount, merchant, note,
    transaction_date, message_id, is_recurring, created_at, updated_at,
    deleted_at
from public.transactions
where id = sqlc.arg(id)
  and deleted_at is null
limit 1;

-- name: GetTransactionByMessageID :one
select id, user_id, category_id, amount, merchant, note,
    transaction_date, message_id, is_recurring, created_at, updated_at,
    deleted_at
from public.transactions
where message_id = sqlc.arg(message_id)
  and deleted_at is null
limit 1;

-- name: UpdateTransaction :one
update public.transactions
set category_id = sqlc.narg(category_id),
    amount = sqlc.arg(amount),
    merchant = sqlc.arg(merchant),
    note = sqlc.arg(note),
    transaction_date = sqlc.arg(transaction_date),
    message_id = sqlc.narg(message_id),
    is_recurring = sqlc.arg(is_recurring)
where id = sqlc.arg(id)
  and deleted_at is null
returning id, user_id, category_id, amount, merchant, note,
    transaction_date, message_id, is_recurring, created_at, updated_at,
    deleted_at;

-- name: SoftDeleteTransaction :execrows
update public.transactions
set deleted_at = now()
where id = sqlc.arg(id)
  and deleted_at is null;

-- name: ListTransactions :many
select id, user_id, category_id, amount, merchant, note,
    transaction_date, message_id, is_recurring, created_at, updated_at,
    deleted_at
from public.transactions
where user_id = sqlc.arg(user_id)
  and (nullif(sqlc.arg(date_from)::text, '') is null
       or transaction_date::date >= nullif(sqlc.arg(date_from)::text, '')::date)
  and (nullif(sqlc.arg(date_to)::text, '') is null
       or transaction_date::date <= nullif(sqlc.arg(date_to)::text, '')::date)
  and (nullif(sqlc.arg(amount_min)::text, '') is null
       or amount >= nullif(sqlc.arg(amount_min)::text, '')::numeric)
  and (nullif(sqlc.arg(amount_max)::text, '') is null
       or amount <= nullif(sqlc.arg(amount_max)::text, '')::numeric)
  and (nullif(sqlc.arg(last_updated)::text, '') is null
       or updated_at > nullif(sqlc.arg(last_updated)::text, '')::timestamptz)
  and (not sqlc.arg(exclude_deleted)::boolean or deleted_at is null)
order by
    case when sqlc.arg(sort_field)::text = 'transaction_date'
              and sqlc.arg(sort_direction)::text = 'ASC'
         then transaction_date end asc,
    case when sqlc.arg(sort_field)::text = 'transaction_date'
              and sqlc.arg(sort_direction)::text <> 'ASC'
         then transaction_date end desc,
    case when sqlc.arg(sort_field)::text = 'amount'
              and sqlc.arg(sort_direction)::text = 'ASC'
         then amount end asc,
    case when sqlc.arg(sort_field)::text = 'amount'
              and sqlc.arg(sort_direction)::text <> 'ASC'
         then amount end desc,
    case when sqlc.arg(sort_field)::text = 'merchant'
              and sqlc.arg(sort_direction)::text = 'ASC'
         then merchant end asc,
    case when sqlc.arg(sort_field)::text = 'merchant'
              and sqlc.arg(sort_direction)::text <> 'ASC'
         then merchant end desc,
    id desc
limit sqlc.arg(limit_count)
offset sqlc.arg(offset_count);

-- name: CountTransactions :one
select count(*)::bigint
from public.transactions
where user_id = sqlc.arg(user_id)
  and (nullif(sqlc.arg(date_from)::text, '') is null
       or transaction_date::date >= nullif(sqlc.arg(date_from)::text, '')::date)
  and (nullif(sqlc.arg(date_to)::text, '') is null
       or transaction_date::date <= nullif(sqlc.arg(date_to)::text, '')::date)
  and (nullif(sqlc.arg(amount_min)::text, '') is null
       or amount >= nullif(sqlc.arg(amount_min)::text, '')::numeric)
  and (nullif(sqlc.arg(amount_max)::text, '') is null
       or amount <= nullif(sqlc.arg(amount_max)::text, '')::numeric)
  and (nullif(sqlc.arg(last_updated)::text, '') is null
       or updated_at > nullif(sqlc.arg(last_updated)::text, '')::timestamptz)
  and (not sqlc.arg(exclude_deleted)::boolean or deleted_at is null);

-- name: SearchTransactions :many
select id, user_id, category_id, amount, merchant, note,
    transaction_date, message_id, is_recurring, created_at, updated_at,
    deleted_at
from public.transactions
where user_id = sqlc.arg(user_id)
  and merchant ilike '%' || sqlc.arg(query) || '%'
  and deleted_at is null
order by transaction_date desc
limit sqlc.arg(limit_count)
offset sqlc.arg(offset_count);

-- name: CountSearchTransactions :one
select count(*)::bigint
from public.transactions
where user_id = sqlc.arg(user_id)
  and merchant ilike '%' || sqlc.arg(query) || '%'
  and deleted_at is null;

-- name: GetExpensesByCategory :many
select c.name as category_name, sum(t.amount)::numeric as total_amount
from public.transactions t
left join public.categories c on c.id = t.category_id
where t.user_id = sqlc.arg(user_id)
  and t.transaction_date >= sqlc.arg(start_date)
  and t.transaction_date < sqlc.arg(end_date)
  and t.deleted_at is null
group by c.name
order by total_amount desc;

-- name: GetTransactionsForExport :many
select
    t.id, t.user_id, t.category_id, t.amount, t.merchant, t.note,
    t.transaction_date, t.message_id, t.is_recurring, t.created_at,
    t.updated_at, t.deleted_at,
    c.name as category_name, c.icon as category_icon
from public.transactions t
left join public.categories c on c.id = t.category_id
where t.user_id = sqlc.arg(user_id)
  and t.transaction_date >= sqlc.arg(start_date)
  and t.transaction_date < sqlc.arg(end_date)
  and t.deleted_at is null
order by t.transaction_date asc;

-- name: GetTotalSpentByCategory :one
select coalesce(sum(amount), 0)::numeric as total_spent
from public.transactions
where user_id = sqlc.arg(user_id)
  and category_id = sqlc.arg(category_id)
  and transaction_date >= sqlc.arg(start_date)
  and transaction_date < sqlc.arg(end_date)
  and deleted_at is null;
