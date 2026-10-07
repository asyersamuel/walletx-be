-- 1. Tambahkan kolom status. 
-- Kita beri default sementara agar data lama tidak error, lalu langsung kita buang default-nya.
ALTER TABLE transactions ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'success';
ALTER TABLE transactions ALTER COLUMN status DROP DEFAULT;

-- 2. Longgarkan kolom yang belum diketahui saat struk baru di-upload (menjadi Nullable)
ALTER TABLE transactions ALTER COLUMN amount DROP NOT NULL;
ALTER TABLE transactions ALTER COLUMN merchant_name DROP NOT NULL;
ALTER TABLE transactions ALTER COLUMN category_id DROP NOT NULL;
ALTER TABLE transactions ALTER COLUMN transaction_date DROP NOT NULL;

-- 3. Tambahkan Composite Index untuk mempercepat query ke frontend (Riwayat Transaksi)
CREATE INDEX idx_transactions_user_status ON transactions(user_id, status);

-- 4. Tambahkan Partial Index khusus untuk Cron Job agar cepat menemukan yang pending
CREATE INDEX idx_transactions_pending ON transactions(status) WHERE status = 'pending';