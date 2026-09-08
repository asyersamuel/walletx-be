-- Auth-only baseline schema.
--
-- The migration directory is the source of truth for the database schema.
-- Feature-specific tables can be added later in separate migrations.

create extension if not exists pgcrypto;

create table if not exists public.users (
    id uuid primary key default gen_random_uuid(),
    google_id text not null,
    email text not null,
    name text not null,
    picture text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    deleted_at timestamptz
);

create unique index if not exists users_google_id_key
    on public.users (google_id);
create unique index if not exists users_email_key
    on public.users (email);
create index if not exists users_deleted_at_idx
    on public.users (deleted_at);

create or replace function public.set_updated_at()
returns trigger
language plpgsql
as $$
begin
    new.updated_at = now();
    return new;
end;
$$;

drop trigger if exists users_set_updated_at on public.users;
create trigger users_set_updated_at
before update on public.users
for each row execute function public.set_updated_at();
