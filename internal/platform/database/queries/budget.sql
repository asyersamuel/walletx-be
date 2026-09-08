-- name: CreateCategoryLimit :one
insert into public.category_limits (
    user_id, category_id, period, limit_amount, is_active
)
values (
    sqlc.arg(user_id),
    sqlc.arg(category_id),
    coalesce(nullif(sqlc.arg(period)::text, ''), 'monthly'),
    sqlc.arg(limit_amount),
    sqlc.arg(is_active)
)
returning id, user_id, category_id, period, limit_amount, is_active,
    created_at, updated_at;

-- name: ListCategoryLimits :many
select id, user_id, category_id, period, limit_amount, is_active,
    created_at, updated_at
from public.category_limits
where user_id = sqlc.arg(user_id)
order by created_at desc;

-- name: ListActiveCategoryLimitsWithCategory :many
select
    cl.id, cl.user_id, cl.category_id, cl.period, cl.limit_amount,
    cl.is_active, cl.created_at, cl.updated_at,
    c.name as category_name, c.icon as category_icon
from public.category_limits cl
join public.categories c on c.id = cl.category_id
where cl.user_id = sqlc.arg(user_id)
  and cl.is_active = true
  and c.deleted_at is null
order by cl.created_at desc;

-- name: GetCategoryLimitByID :one
select id, user_id, category_id, period, limit_amount, is_active,
    created_at, updated_at
from public.category_limits
where id = sqlc.arg(id)
  and user_id = sqlc.arg(user_id)
limit 1;

-- name: GetCategoryLimitByCategory :one
select id, user_id, category_id, period, limit_amount, is_active,
    created_at, updated_at
from public.category_limits
where user_id = sqlc.arg(user_id)
  and category_id = sqlc.arg(category_id)
limit 1;

-- name: UpdateCategoryLimit :one
update public.category_limits
set period = sqlc.arg(period),
    limit_amount = sqlc.arg(limit_amount),
    is_active = sqlc.arg(is_active)
where id = sqlc.arg(id)
  and user_id = sqlc.arg(user_id)
returning id, user_id, category_id, period, limit_amount, is_active,
    created_at, updated_at;

-- name: DeleteCategoryLimit :execrows
delete from public.category_limits
where id = sqlc.arg(id)
  and user_id = sqlc.arg(user_id);

-- name: GetLimitReportByUserID :many
select
    c.name as category_name,
    c.icon as category_icon,
    cl.limit_amount,
    coalesce(sum(t.amount), 0)::numeric as total_spent
from public.category_limits cl
join public.categories c on c.id = cl.category_id
left join public.transactions t
    on t.category_id = cl.category_id
   and t.user_id = cl.user_id
   and t.deleted_at is null
   and date_trunc('month', t.transaction_date at time zone 'Asia/Jakarta') =
       date_trunc('month', now() at time zone 'Asia/Jakarta')
where cl.user_id = sqlc.arg(user_id)
  and cl.is_active = true
  and c.deleted_at is null
group by c.id, c.name, c.icon, cl.id, cl.limit_amount
order by c.name asc;

-- name: GetSummaryByUserAndStartDate :many
select category_id, sum(total_amount)::numeric as total_amount,
    sum(transaction_count)::bigint as transaction_count
from public.daily_expense_summary
where user_id = sqlc.arg(user_id)
  and day >= sqlc.arg(start_date)
group by category_id;

-- name: GetDailyTotalByUserAndRange :many
select day, sum(total_amount)::numeric as total_amount
from public.daily_expense_summary
where user_id = sqlc.arg(user_id)
  and day >= sqlc.arg(start_date)
  and day < sqlc.arg(end_date)
group by day
order by day asc;

-- name: GetSpendingByCategoryAndRange :many
select category_id, sum(total_amount)::numeric as total_amount,
    sum(transaction_count)::bigint as transaction_count
from public.daily_expense_summary
where user_id = sqlc.arg(user_id)
  and day >= sqlc.arg(start_date)
  and day <= sqlc.arg(end_date)
group by category_id;
