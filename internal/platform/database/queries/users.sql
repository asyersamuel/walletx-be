-- name: UpdateUserProfile :one
update public.users
set name = sqlc.arg(name),
    picture = sqlc.narg(picture)
where id = sqlc.arg(id)
  and deleted_at is null
returning id, google_id, email, name, picture,
    created_at, updated_at, deleted_at;

-- name: SoftDeleteUser :execrows
update public.users
set deleted_at = now()
where id = sqlc.arg(id)
  and deleted_at is null;
