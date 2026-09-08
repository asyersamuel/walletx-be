-- WalletX baseline schema.
--
-- This migration intentionally recreates the public application schema. It is
-- suitable for a new local database and for the explicitly approved cleanup
-- of the current Supabase project. Do not run it against a database whose data
-- must be retained without taking a backup first.

create extension if not exists pgcrypto;

drop view if exists public.daily_expense_summary cascade;
drop table if exists public.transactions cascade;
drop table if exists public.recurring_configs cascade;
drop table if exists public.category_limits cascade;
drop table if exists public.categories cascade;
drop table if exists public.users cascade;

create table public.users (
    id uuid primary key default gen_random_uuid(),
    google_id text not null,
    email text not null,
    name text not null,
    picture text,
    telegram_chat_id text unique,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    deleted_at timestamptz
);

create unique index users_google_id_key on public.users (google_id);
create unique index users_email_key on public.users (email);
create index users_deleted_at_idx on public.users (deleted_at);

create table public.categories (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references public.users(id) on delete cascade,
    name text not null,
    icon text not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    deleted_at timestamptz
);

create unique index categories_user_name_key
    on public.categories (user_id, lower(name))
    where deleted_at is null;
create index categories_user_id_idx on public.categories (user_id);
create index categories_deleted_at_idx on public.categories (deleted_at);

create table public.category_limits (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references public.users(id) on delete cascade,
    category_id uuid not null references public.categories(id) on delete cascade,
    period text not null default 'monthly'
        check (period in ('weekly', 'monthly')),
    limit_amount numeric(15, 2) not null check (limit_amount > 0),
    is_active boolean not null default true,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint category_limits_user_category_key unique (user_id, category_id)
);

create index category_limits_user_id_idx on public.category_limits (user_id);
create index category_limits_category_id_idx on public.category_limits (category_id);

create table public.recurring_configs (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references public.users(id) on delete cascade,
    category_id uuid references public.categories(id) on delete set null,
    amount numeric(15, 2) not null check (amount > 0),
    frequency text not null
        check (frequency in ('weekly', 'monthly', 'yearly')),
    start_date timestamptz not null,
    next_due_date timestamptz not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index recurring_configs_user_id_idx on public.recurring_configs (user_id);
create index recurring_configs_due_idx on public.recurring_configs (next_due_date);

create table public.transactions (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references public.users(id) on delete cascade,
    category_id uuid references public.categories(id) on delete set null,
    amount numeric(15, 2) not null check (amount > 0),
    merchant text not null,
    note text not null default '',
    transaction_date timestamptz not null,
    message_id text,
    is_recurring boolean not null default false,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    deleted_at timestamptz
);

create unique index transactions_message_id_key
    on public.transactions (message_id)
    where message_id is not null;
create index transactions_user_date_idx
    on public.transactions (user_id, transaction_date desc);
create index transactions_user_category_date_idx
    on public.transactions (user_id, category_id, transaction_date desc);
create index transactions_deleted_at_idx on public.transactions (deleted_at);

create or replace function public.set_updated_at()
returns trigger
language plpgsql
as $$
begin
    new.updated_at = now();
    return new;
end;
$$;

create trigger users_set_updated_at
before update on public.users
for each row execute function public.set_updated_at();

create trigger categories_set_updated_at
before update on public.categories
for each row execute function public.set_updated_at();

create trigger category_limits_set_updated_at
before update on public.category_limits
for each row execute function public.set_updated_at();

create trigger recurring_configs_set_updated_at
before update on public.recurring_configs
for each row execute function public.set_updated_at();

create trigger transactions_set_updated_at
before update on public.transactions
for each row execute function public.set_updated_at();

create or replace view public.daily_expense_summary as
select
    t.user_id,
    (t.transaction_date at time zone 'Asia/Jakarta')::date as day,
    t.category_id,
    sum(t.amount) as total_amount,
    count(*)::bigint as transaction_count
from public.transactions t
where t.deleted_at is null
group by
    t.user_id,
    (t.transaction_date at time zone 'Asia/Jakarta')::date,
    t.category_id;
