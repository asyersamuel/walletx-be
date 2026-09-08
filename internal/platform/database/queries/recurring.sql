-- name: CreateRecurringConfig :one
insert into public.recurring_configs (
    user_id, category_id, amount, frequency, start_date, next_due_date
)
values (
    sqlc.arg(user_id), sqlc.narg(category_id), sqlc.arg(amount),
    sqlc.arg(frequency), sqlc.arg(start_date), sqlc.arg(next_due_date)
)
returning id, user_id, category_id, amount, frequency, start_date,
    next_due_date, created_at, updated_at;

-- name: ListRecurringConfigs :many
select id, user_id, category_id, amount, frequency, start_date,
    next_due_date, created_at, updated_at
from public.recurring_configs
where user_id = sqlc.arg(user_id)
order by created_at desc;

-- name: GetRecurringConfigByID :one
select id, user_id, category_id, amount, frequency, start_date,
    next_due_date, created_at, updated_at
from public.recurring_configs
where id = sqlc.arg(id)
  and user_id = sqlc.arg(user_id)
limit 1;

-- name: UpdateRecurringConfig :one
update public.recurring_configs
set category_id = sqlc.narg(category_id),
    amount = sqlc.arg(amount),
    frequency = sqlc.arg(frequency),
    start_date = sqlc.arg(start_date),
    next_due_date = sqlc.arg(next_due_date)
where id = sqlc.arg(id)
  and user_id = sqlc.arg(user_id)
returning id, user_id, category_id, amount, frequency, start_date,
    next_due_date, created_at, updated_at;

-- name: DeleteRecurringConfig :execrows
delete from public.recurring_configs
where id = sqlc.arg(id)
  and user_id = sqlc.arg(user_id);

-- name: ListDueRecurringConfigs :many
select id, user_id, category_id, amount, frequency, start_date,
    next_due_date, created_at, updated_at
from public.recurring_configs
where next_due_date <= now()
order by next_due_date asc;
