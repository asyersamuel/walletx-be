-- name: CreateUser :one
insert into public.users (
    google_id, email, name, picture
)
values (
    sqlc.arg(google_id), sqlc.arg(email), sqlc.arg(name), sqlc.narg(picture)
)
returning id, google_id, email, name, picture, telegram_chat_id,
    created_at, updated_at, deleted_at;

-- name: GetUserByID :one
select id, google_id, email, name, picture, telegram_chat_id,
    created_at, updated_at, deleted_at
from public.users
where id = sqlc.arg(id)
  and deleted_at is null
limit 1;

-- name: GetUserByEmail :one
select id, google_id, email, name, picture, telegram_chat_id,
    created_at, updated_at, deleted_at
from public.users
where email = sqlc.arg(email)
  and deleted_at is null
limit 1;

-- name: GetUserByGoogleID :one
select id, google_id, email, name, picture, telegram_chat_id,
    created_at, updated_at, deleted_at
from public.users
where google_id = sqlc.arg(google_id)
  and deleted_at is null
limit 1;

-- name: GetUserByTelegramChatID :one
select id, google_id, email, name, picture, telegram_chat_id,
    created_at, updated_at, deleted_at
from public.users
where telegram_chat_id = sqlc.arg(telegram_chat_id)
  and deleted_at is null
limit 1;

-- name: UpdateUser :one
update public.users
set google_id = sqlc.arg(google_id),
    email = sqlc.arg(email),
    name = sqlc.arg(name),
    picture = sqlc.narg(picture),
    telegram_chat_id = sqlc.narg(telegram_chat_id)
where id = sqlc.arg(id)
returning id, google_id, email, name, picture, telegram_chat_id,
    created_at, updated_at, deleted_at;
