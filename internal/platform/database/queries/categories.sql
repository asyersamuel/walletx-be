-- name: CreateCategory :one
insert into public.categories (user_id, name, icon)
values (sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(icon)::text)
returning id, user_id, name, icon, created_at, updated_at, deleted_at;

-- name: ListCategories :many
select id, user_id, name, icon, created_at, updated_at, deleted_at
from public.categories
where user_id = sqlc.arg(user_id)
  and deleted_at is null
order by created_at desc;

-- name: GetCategoryByID :one
select id, user_id, name, icon, created_at, updated_at, deleted_at
from public.categories
where id = sqlc.arg(id)
  and user_id = sqlc.arg(user_id)
  and deleted_at is null
limit 1;

-- name: GetCategoryByName :one
select id, user_id, name, icon, created_at, updated_at, deleted_at
from public.categories
where lower(name) = lower(sqlc.arg(name))
  and user_id = sqlc.arg(user_id)
  and deleted_at is null
limit 1;

-- name: UpdateCategory :one
update public.categories
set name = sqlc.arg(name), icon = sqlc.arg(icon)
where id = sqlc.arg(id)
  and user_id = sqlc.arg(user_id)
  and deleted_at is null
returning id, user_id, name, icon, created_at, updated_at, deleted_at;

-- name: SoftDeleteCategory :execrows
update public.categories
set deleted_at = now()
where id = sqlc.arg(id)
  and user_id = sqlc.arg(user_id)
  and deleted_at is null;
