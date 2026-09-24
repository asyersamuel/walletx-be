-- ==========================================
-- FILE: 20260914000000_setup_db.sql
-- TUJUAN: Inisialisasi Ekstensi, Tabel, dan Fungsi RAG WalletX
-- ==========================================

-- 1. AKTIFKAN EKSTENSI DULU
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA extensions;

-- 2. HAPUS OBJEK LAMA (Reset)
DROP FUNCTION IF EXISTS match_walletx_transactions(extensions.vector, float, int, uuid);
DROP TABLE IF EXISTS public.transactions CASCADE;
DROP TABLE IF EXISTS public.categories CASCADE;
DROP TABLE IF EXISTS public.users CASCADE;

-- 3. BUAT TABEL USERS
CREATE TABLE public.users (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    google_id VARCHAR NOT NULL UNIQUE,
    email VARCHAR NOT NULL UNIQUE,
    name VARCHAR NOT NULL,
    picture VARCHAR,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT timezone('utc'::text, now()) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT timezone('utc'::text, now()) NOT NULL,
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- 4. BUAT TABEL CATEGORIES
CREATE TABLE public.categories (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    name VARCHAR NOT NULL,
    type TEXT NOT NULL DEFAULT 'expense',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT timezone('utc'::text, now()) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT timezone('utc'::text, now()) NOT NULL,
    CONSTRAINT categories_type_check CHECK (type IN ('expense', 'income')),
    CONSTRAINT categories_user_name_type_key UNIQUE (user_id, name, type)
);

CREATE INDEX IF NOT EXISTS categories_user_id_type_idx ON public.categories (user_id, type);

-- Trigger untuk Updated_at
CREATE OR REPLACE FUNCTION public.set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = timezone('utc'::text, now());
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS categories_set_updated_at ON public.categories;
CREATE TRIGGER categories_set_updated_at
BEFORE UPDATE ON public.categories
FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();

-- 5. BUAT TABEL TRANSACTIONS (Memakai extensions.vector)
CREATE TABLE public.transactions (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    category_id UUID REFERENCES public.categories(id) ON DELETE SET NULL,
    merchant_name VARCHAR NOT NULL,
    amount NUMERIC NOT NULL,
    transaction_date TIMESTAMP WITH TIME ZONE NOT NULL,
    receipt_image_url VARCHAR,
    notes TEXT,
    embedding extensions.vector(768),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT timezone('utc'::text, now()) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT timezone('utc'::text, now()) NOT NULL
);

-- 6. BUAT FUNGSI PENCARIAN VEKTOR (RAG)
CREATE OR REPLACE FUNCTION match_walletx_transactions (
    query_embedding extensions.vector(768),
    match_threshold float,
    match_count int,
    p_user_id uuid
)
RETURNS TABLE (
    id uuid,
    merchant_name varchar,
    category_name varchar,
    amount numeric,
    transaction_date timestamp with time zone,
    notes text,
    similarity float
)
LANGUAGE sql STABLE
AS $$
    SELECT
        t.id,
        t.merchant_name,
        c.name AS category_name,
        t.amount,
        t.transaction_date,
        t.notes,
        1 - (t.embedding <=> query_embedding) AS similarity
    FROM public.transactions t
    LEFT JOIN public.categories c ON t.category_id = c.id
    WHERE t.user_id = p_user_id
      AND 1 - (t.embedding <=> query_embedding) > match_threshold
    ORDER BY t.embedding <=> query_embedding
    LIMIT match_count;
$$;