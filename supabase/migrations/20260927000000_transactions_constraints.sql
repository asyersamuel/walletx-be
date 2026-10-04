-- ==========================================
-- FILE: 20260927000000_transactions_constraints.sql
-- TUJUAN: Menambahkan constraint/indeks/trigger untuk tabel transactions
--         tanpa mengubah migrasi setup yang sudah diterapkan (additive).
-- ==========================================

-- Business rule: nominal transaksi harus selalu positif.
-- Semantik income vs expense berasal dari baris categories yang direferensikan
-- (categories.type), bukan dari tanda amount.
ALTER TABLE public.transactions
    DROP CONSTRAINT IF EXISTS transactions_amount_positive_check;
ALTER TABLE public.transactions
    ADD CONSTRAINT transactions_amount_positive_check CHECK (amount > 0);

-- Mendukung filter rentang tanggal + pagination per user (GET /transactions).
CREATE INDEX IF NOT EXISTS transactions_user_id_transaction_date_idx
    ON public.transactions (user_id, transaction_date);

-- Menjaga updated_at tetap sinkron saat baris transaksi diperbarui.
DROP TRIGGER IF EXISTS transactions_set_updated_at ON public.transactions;
CREATE TRIGGER transactions_set_updated_at
BEFORE UPDATE ON public.transactions
FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();
