-- name: CreateCategory :one
insert into public.categories (user_id, name, type)
values (sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(type))
returning id, user_id, name, type, created_at, updated_at;

-- name: ListCategoriesByUser :many
select id, user_id, name, type, created_at, updated_at
from public.categories
where user_id = sqlc.arg(user_id)
  and (
      sqlc.narg(category_type)::text is null
      or type = sqlc.narg(category_type)::text
  )
order by type asc, name asc, id asc;

-- name: UpdateCategory :one
update public.categories
set name = sqlc.arg(name),
    type = sqlc.arg(type),
    updated_at = now()
where id = sqlc.arg(id)
  and user_id = sqlc.arg(user_id)
returning id, user_id, name, type, created_at, updated_at;

-- name: DeleteCategory :execrows
delete from public.categories
where id = sqlc.arg(id)
  and user_id = sqlc.arg(user_id);
