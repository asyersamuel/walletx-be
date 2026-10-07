-- 1. Mengizinkan publik untuk melihat gambar (wajib untuk getPublicUrl)
CREATE POLICY "Public Access for receipts" 
ON storage.objects FOR SELECT 
USING (bucket_id = 'receipts');

-- 2. Mengizinkan user terautentikasi untuk upload hanya ke foldernya sendiri
CREATE POLICY "Users can upload to their own folder" 
ON storage.objects FOR INSERT 
TO authenticated 
WITH CHECK (
  bucket_id = 'receipts' AND 
  (storage.foldername(name))[1] = auth.uid()::text
);

-- 3. Mengizinkan user untuk menghapus/update file miliknya sendiri
CREATE POLICY "Users can update and delete their own files" 
ON storage.objects FOR UPDATE 
TO authenticated 
USING (
  bucket_id = 'receipts' AND 
  (storage.foldername(name))[1] = auth.uid()::text
);
CREATE POLICY "Users can delete their own files" 
ON storage.objects FOR DELETE 
TO authenticated 
USING (
  bucket_id = 'receipts' AND 
  (storage.foldername(name))[1] = auth.uid()::text
);