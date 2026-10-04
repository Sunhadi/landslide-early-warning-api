# Kebijakan Keamanan

## Melaporkan Kerentanan

Jika Anda menemukan kerentanan keamanan pada proyek ini, mohon **jangan** membuka
issue publik. Laporkan secara privat melalui email ke pemelihara repositori, dengan
menyertakan:

- Deskripsi kerentanan dan dampaknya
- Langkah reproduksi
- Versi/commit yang terdampak
- Saran perbaikan (bila ada)

## Praktik Keamanan Proyek

- **Rahasia** tidak disimpan di repositori. Semua kredensial melalui environment
  variable (lihat `.env.example`).
- Di mode produksi (`APP_ENV=production`), aplikasi **menolak jalan** bila secret
  default/lemah masih dipakai (fail-fast).
- Data pribadi (nomor telepon) dienkripsi (AES-GCM) dan disamarkan di log,
  sesuai UU PDP No. 27/2022.
- Autentikasi memakai JWT + refresh token dengan mekanisme pencabutan (blacklist).
- Dependency dipindai dengan `govulncheck` di CI.

## Sebelum Deploy

1. Set `APP_ENV=production`.
2. Isi `JWT_SECRET`, `PII_ENCRYPTION_KEY`, `ADMIN_PASS` dengan nilai acak kuat (≥ 16 karakter).
3. Batasi `CORS_ORIGINS`.
4. Jalankan di belakang reverse proxy TLS; set `TRUST_PROXY=true`.
5. Ganti password PostgreSQL default dengan yang kuat.
