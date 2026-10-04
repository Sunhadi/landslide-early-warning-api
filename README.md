# API Peringatan Dini Bencana Longsor — Jawa Tengah

Backend Go untuk memantau, menganalisis, dan menyebarkan **peringatan dini tanah longsor**
di provinsi **Jawa Tengah**. Data hujan bersumber dari **BMKG**, peringatan didistribusikan
melalui **WhatsApp & SMS**, dan perhitungan risiko memakai metode **rule-based**.

> Proyek ini dirancang agar bisa **dijalankan gratis tanpa Docker** untuk belajar:
> storage in-memory + data contoh Jawa Tengah + notifikasi mode log.

---

## Fitur

- **Data hujan real-time dari BMKG** (`api.bmkg.go.id`) untuk 10 wilayah contoh di Jateng.
- **Rule-based risk engine** — skor 0–100 & level NORMAL/WASPADA/SIAGA/AWAS.
- **Peringatan otomatis** saat ambang hujan + kerawanan wilayah terlampaui.
- **Distribusi WhatsApp & SMS** dengan pola adaptor (mode `log` untuk belajar).
- **Laporan warga** + verifikasi petugas.
- **Langganan peringatan** per wilayah & kanal.
- **Autentikasi JWT + RBAC** (admin, officer, public).
- **Kepatuhan UU PDP**: consent wajib, nomor HP dienkripsi (AES-GCM), masking di log.
- **Audit log**, rate limiting, security headers, graceful shutdown.
- **Scheduler** (cron) untuk tarik data & evaluasi berkala.

---

## Persyaratan

- **Go 1.22+** (diuji pada Go 1.24).
- Tidak wajib Docker. PostgreSQL opsional untuk pengembangan lanjutan.

---

## Menjalankan

```bash
# 1. (opsional) salin konfigurasi
copy .env.example .env

# 2. jalankan
go run ./cmd/api
```

Server berjalan di `http://localhost:8080`. Cek:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/v1/regions
```

### Build binary

```bash
go build -o bin/api.exe ./cmd/api
```

---

## Akun default

| Username | Password    | Role    |
|----------|-------------|---------|
| admin    | admin123    | admin   |
| petugas  | petugas123  | officer |

> Ganti lewat environment `ADMIN_USER` / `ADMIN_PASS` di produksi.

---

## Alur demo cepat

```bash
BASE=http://localhost:8080/v1

# 1. Login petugas
TOKEN=$(curl -s -X POST $BASE/auth/login -H "Content-Type: application/json" \
  -d '{"username":"petugas","password":"petugas123"}' | jq -r .token)

# 2. Tarik data hujan BMKG + evaluasi + proses alert
curl -s -X POST "$BASE/risk/sync" -H "Authorization: Bearer $TOKEN" | jq .

# 3. Lihat risiko satu wilayah
curl -s "$BASE/regions/jateng-banjarnegara-susukan/risk" | jq .

# 4. Berlangganan peringatan (mode log)
curl -s -X POST $BASE/subscriptions -H "Content-Type: application/json" \
  -d '{"region_id":"jateng-banjarnegara-susukan","phone":"+628123456789","channel":"whatsapp","consent":true}' | jq .

# 5. Lihat peringatan aktif
curl -s "$BASE/alerts?status=active" | jq .
```

---

## Endpoint

| Method | Path | Auth | Deskripsi |
|--------|------|------|-----------|
| GET  | `/health` | — | Health check |
| GET  | `/v1/regions` | — | Daftar wilayah |
| GET  | `/v1/regions/{id}` | — | Detail wilayah |
| GET  | `/v1/regions/{id}/risk` | — | Risiko terkini |
| GET  | `/v1/regions/{id}/forecast` | — | Prakiraan risiko |
| GET  | `/v1/alerts` | — | Daftar peringatan |
| GET  | `/v1/alerts/{id}` | — | Detail peringatan |
| GET  | `/v1/rainfall` | — | Data curah hujan |
| POST | `/v1/reports` | — | Kirim laporan warga |
| POST | `/v1/subscriptions` | — | Berlangganan |
| POST | `/v1/auth/login` | — | Login (kembalikan access + refresh token) |
| POST | `/v1/auth/refresh` | — | Tukar refresh token (rotasi) |
| POST | `/v1/auth/logout` | JWT | Cabut token (blacklist) |
| GET  | `/v1/auth/me` | JWT | Info pengguna |
| POST | `/v1/alerts` | officer/admin | Terbitkan peringatan manual |
| PATCH| `/v1/alerts/{id}` | officer/admin | Ubah status peringatan |
| GET  | `/v1/reports` | JWT | Daftar laporan |
| PATCH| `/v1/reports/{id}/verify` | officer/admin | Verifikasi laporan |
| GET  | `/v1/subscriptions` | JWT | Daftar langganan |
| DELETE| `/v1/subscriptions/{id}` | JWT | Hapus langganan (hak UU PDP) |
| POST | `/v1/risk/sync` | officer/admin | Tarik data BMKG + evaluasi |
| POST | `/v1/risk/evaluate` | officer/admin | Evaluasi risiko saja |
| GET  | `/v1/audit` | admin | Audit log |

Contoh error standar:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "region_id wajib & level harus 1-4"
  }
}
```

### Pagination

Semua endpoint daftar mendukung `?page=` dan `?per_page=` (maks 100):

```
GET /v1/alerts?page=1&per_page=20
```

Respons menyertakan metadata:

```json
{
  "data": [ ... ],
  "meta": { "page": 1, "per_page": 20, "total": 42, "pages": 3 }
}
```

### Autentikasi & sesi

```json
// POST /v1/auth/login
{ "token": "<access>", "refresh_token": "<refresh>", "expires_in": 43200, "role": "officer" }

// POST /v1/auth/refresh  (rotasi: refresh lama hangus)
{ "refresh_token": "<refresh>" }
```

- **Access token** berlaku singkat (`JWT_TTL_HOURS`).
- **Refresh token** berlaku lama (`JWT_REFRESH_TTL_HOURS`), dirotasi setiap dipakai.
- `POST /v1/auth/logout` mencabut access token (blacklist) sekaligus refresh token terkait.


---

## Konfigurasi

Semua lewat environment (lihat `.env.example`). Yang penting:

| Variabel | Default | Keterangan |
|----------|---------|------------|
| `PORT` | `8080` | Port HTTP |
| `JWT_SECRET` | dev | **Ganti di produksi** |
| `PII_ENCRYPTION_KEY` | dev | Kunci enkripsi nomor HP |
| `FETCH_ENABLED` | `true` | Aktifkan tarik BMKG |
| `FETCH_CRON` | `*/30 * * * *` | Jadwal tarik data |
| `ALERT_CRON` | `*/10 * * * *` | Jadwal evaluasi |
| `NOTIFIER_MODE` | `log` | `log` (belajar) atau `live` |
| `WA_ENDPOINT`, `WA_API_KEY` | — | Gateway WhatsApp |
| `SMS_PROVIDER`, `SMS_API_KEY` | — | Gateway SMS |
| `DATABASE_URL` | (kosong) | Kosong = in-memory; isi = PostgreSQL |

### Menggunakan PostgreSQL

Secara default aplikasi memakai **in-memory** (data hilang saat restart). Untuk persisten,
siapkan PostgreSQL, terapkan skema, lalu set `DATABASE_URL`.

> Skema `migrations/0001_init.sql` **tidak memerlukan PostGIS** — memakai kolom `lat`/`lon`
> biasa agar jalan di PostgreSQL standar. Bila PostGIS tersedia, lihat catatan di dalam file
> migrasi untuk mengaktifkannya.

Contoh cepat (PostgreSQL lokal sudah terinstall):

```bash
# 1. Buat database
psql -U postgres -h localhost -c "CREATE DATABASE longsor;"

# 2. Terapkan skema
psql -U postgres -h localhost -d longsor -f migrations/0001_init.sql

# 3. Jalankan API mode PostgreSQL (Windows cmd)
set DATABASE_URL=postgres://postgres:postgres@localhost:5432/longsor?sslmode=disable
go run ./cmd/api
```

Uji integrasi PostgreSQL:

```powershell
$env:TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/longsor?sslmode=disable"
go test ./internal/store -run TestPostgresStore -v
```

### Mengaktifkan pengiriman nyata

Set `NOTIFIER_MODE=live` dan isi endpoint gateway. Implementasi adaptor ada di
`internal/notifier/notifier.go` (`WhatsAppNotifier`, `SMSNotifier`).

---

## Struktur Proyek

```
cmd/api/              entry point + seeding
internal/
  config/             konfigurasi env
  domain/             entitas inti
  engine/             rule-based risk engine (+ tes)
  httpapi/            router, middleware, handler
  ingestion/          klien BMKG
  notifier/           adaptor WhatsApp/SMS
  security/           JWT (+refresh/blacklist), enkripsi PII, masking
  service/            orkestrasi bisnis
  scheduler/          cron job
  store/              repository (in-memory + PostgreSQL)
docs/
  openapi.yaml        spesifikasi OpenAPI 3.1
  docs.go             embed spec ke binary
```

---

## Menjalankan dengan Docker

Dua cara, keduanya sudah teruji.

### A. Container tunggal (in-memory)

```cmd
docker build -t api-longsor .
docker run -d --name api-longsor -p 8080:8080 ^
  -e JWT_SECRET=rahasia-jwt-panjang-minimal-16 ^
  -e PII_ENCRYPTION_KEY=kunci-pii-panjang-minimal-16 ^
  api-longsor
```

### B. Docker Compose (API + PostgreSQL, persisten)

```cmd
docker compose up -d --build
```

- API: http://localhost:8080 (docs: http://localhost:8080/docs)
- PostgreSQL container dipetakan ke host port **5433** (agar tidak bentrok dengan PostgreSQL lokal di 5432).
- Skema `migrations/0001_init.sql` otomatis diterapkan saat DB pertama kali dibuat.
- Data disimpan di volume `pgdata` → **tetap ada setelah restart**.

Hentikan:

```cmd
docker compose down          # hentikan (data tetap)
docker compose down -v       # hentikan + hapus data
```

> Catatan: saat pertama `up`, API mungkin gagal konek beberapa detik sampai PostgreSQL siap,
> lalu Docker me-restart otomatis hingga tersambung.

## Dokumentasi API

Dokumentasi interaktif (Swagger UI) tersedia langsung saat API berjalan:

| URL | Isi |
|-----|-----|
| `http://localhost:8080/docs` | Swagger UI |
| `http://localhost:8080/openapi.yaml` | Spesifikasi OpenAPI 3.1 (mentah) |
| `docs/openapi.yaml` | Sumber spec (di-embed ke binary) |

> Swagger UI memuat aset dari CDN `unpkg`. Untuk lingkungan offline, unduh
> `swagger-ui-dist` dan sajikan sendiri.

## Menjalankan tes

```bash
go test ./...                 # jalankan semua tes
go test ./... -cover          # dengan coverage
go test ./internal/engine -v  # satu paket, verbose
go test ./... -run TestEvaluate # filter nama tes
go vet ./...                  # analisis statis
```

### Cakupan tes (unit + integrasi)

| Paket | Fokus yang diuji | Coverage |
|-------|------------------|----------|
| `config` | default & override env, nilai tidak valid | 100% |
| `domain` | label & rekomendasi level | 100% |
| `engine` | skor, ambang, eskalasi paksa AWAS, rentang | 88.6% |
| `notifier` | LogNotifier, payload WA/SMS, error status, pemilihan mode | 86.2% |
| `security` | bcrypt, AES-GCM (round-trip, nonce, kunci salah), JWT (expired/tamper), masking | 84.6% |
| `ingestion` | penjumlahan jendela 24/72 jam, error HTTP/JSON, lokasi | 83.3% |
| `service` | evaluasi, terbit/akhiri alert, eskalasi, dispatch, sync | 82.3% |
| `store` | CRUD, filter, audit, akses konkuren, integrasi PostgreSQL nyata | 70.5% |
| `httpapi` | integrasi end-to-end: auth, RBAC, validasi, consent, PII, pagination, refresh/logout, OpenAPI | 60%+ |

Selain unit/integrasi `go test`, sudah diverifikasi secara manual:
- **PostgreSQL lokal** (port 5432) — persistensi setelah restart.
- **Docker**: build image (27.5 MB), container tunggal, dan `docker compose` (API + PostgreSQL) dengan persistensi.
| `scheduler` | start/stop job cron | 37.5% |

> Total **68 tes**. Tes HTTP memakai `httptest` tanpa server eksternal; tes BMKG memakai
> server tiruan (`httptest`), sehingga `go test ./...` tidak memerlukan internet.

### Prinsip yang diverifikasi tes

- **UU PDP**: consent wajib, nomor HP terenkripsi, tidak bocor di respons, masking benar.
- **Keamanan**: JWT kedaluwarsa/dirusak ditolak, RBAC menolak role salah, header keamanan ada.
- **Rule-based**: hujan ekstrem + wilayah rawan → AWAS; kondisi tenang → alert diakhiri.
- **Anti-duplikat**: alert tidak diterbitkan berulang untuk kondisi yang sama.


---

## Keamanan & Hardening

| Kontrol | Penerapan |
|---------|-----------|
| **Fail-fast secret** | `APP_ENV=production` menolak JWT_SECRET/PII_KEY/ADMIN_PASS default atau < 16 karakter |
| **Autentikasi** | JWT HS256, expiry, verifikasi signature |
| **Otorisasi** | RBAC per endpoint (`admin`/`officer`/`public`) |
| **Password** | bcrypt |
| **Anti brute-force** | Lockout per IP+username setelah `LOGIN_MAX_ATTEMPTS` gagal |
| **Rate limiting** | Token bucket per IP (publik 120/menit, login 20/menit) |
| **Batas body** | `MAX_BODY_BYTES` (default 1 MiB) → HTTP 413 |
| **CORS** | `CORS_ORIGINS` (daftar origin atau `*`) |
| **HSTS** | Aktif otomatis saat produksi |
| **Security headers** | nosniff, X-Frame-Options, CSP, Referrer-Policy, Permissions-Policy |
| **PII (UU PDP)** | Consent wajib, nomor HP AES-GCM, masking di log/respons |
| **Validasi input** | Telepon Indonesia, rentang lat/lon, panjang teks, whitelist channel/status |
| **Audit log** | Aksi sensitif (login, alert, verifikasi, langganan) |
| **Refresh token** | Rotasi refresh token, bisa dicabut (revocation) |
| **Token blacklist** | Logout mencabut access + refresh token |
| **Pagination** | `?page=&per_page=` dengan batas maks 100 per halaman |
| **Panic recovery** | Server tidak mati saat panic |
| **Dependency scanning** | `govulncheck` di CI |
| **Proxy-aware IP** | `TRUST_PROXY` (aktifkan hanya di belakang reverse proxy) |

### Sebelum deploy ke produksi (checklist)

1. Set `APP_ENV=production`.
2. Isi `JWT_SECRET`, `PII_ENCRYPTION_KEY`, `ADMIN_PASS` dengan nilai acak kuat
   (≥ 16 karakter, unik). Aplikasi **menolak jalan** kalau masih default.
3. Batasi `CORS_ORIGINS` ke domain dashboard yang dikenal.
4. Jalankan di belakang reverse proxy TLS (Nginx/Caddy) → set `TRUST_PROXY=true`.
5. Ganti storage in-memory ke PostgreSQL (set `DATABASE_URL` + terapkan migrasi).
6. Jalankan `govulncheck ./...` di CI (sudah ada di workflow).

### Menjalankan pemeriksaan keamanan lokal

```bash
go vet ./...
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

---

## Catatan penting

- **Mode belajar**: notifikasi WhatsApp/SMS hanya dicatat ke log (gratis). Pengiriman
  nyata memerlukan gateway berbayar.
- **Data BMKG** bersifat prakiraan; untuk produksi tambahkan data observasi (AWS/ARG).
- **In-memory store** hilang saat restart. Untuk produksi, implementasikan interface
  `store.Store` dengan PostgreSQL/PostGIS (lihat `migrations/`).
- Lihat dokumen perencanaan lengkap di `RENCANA_API_PERINGATAN_DINI_LONGSOR.md`.
