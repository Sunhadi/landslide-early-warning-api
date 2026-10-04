# Rencana API Peringatan Dini Bencana Longsor

> Dokumen perencanaan (blueprint) — versi 0.2
> Status: **Draft untuk ditinjau**
> Tujuan: Menyediakan API lengkap untuk memantau, menganalisis, dan menyebarkan peringatan dini bencana tanah longsor.
>
> **Keputusan yang sudah ditetapkan:**
> - Framework: **Go (Golang)**
> - Wilayah percontohan: **Jawa Tengah**
> - Sumber data hujan: **BMKG**
> - Kanal distribusi: **WhatsApp & SMS** (saja)
> - Metode risiko: **Rule-based**
> - Hosting: **free tier** untuk belajar (lihat §3.4)
> - Kepatuhan: **UU PDP No. 27/2022** + standar keamanan (lihat §8.1 & §9.1)

---

## 1. Ringkasan Eksekutif

API Peringatan Dini Longsor adalah layanan backend yang mengumpulkan data dari berbagai sumber
(curah hujan, sensor tanah, topografi, riwayat kejadian, laporan masyarakat), menghitung tingkat
risiko longsor, lalu menerbitkan peringatan dini ke kanal distribusi (aplikasi mobile, SMS/WhatsApp
gateway, dashboard BPBD, sirene IoT).

### Manfaat
- Deteksi dini area rawan longsor berbasis data real-time + prediksi.
- Menyediakan satu sumber kebenaran (single source of truth) status bahaya per wilayah.
- Distribusi peringatan multi-kanal yang cepat dan terverifikasi.
- Mendukung analisis pasca-kejadian dan perencanaan mitigasi.

### Sasaran pengguna
| Pengguna | Kebutuhan utama |
|---|---|
| BPBD / BNPB | Monitoring wilayah, verifikasi laporan, terbitkan peringatan |
| BMKG | Integrasi data curah hujan & prakiraan |
| Pemerintah daerah | Status bahaya per kecamatan/desa |
| Masyarakat umum | Peringatan & panduan evakuasi |
| Peneliti / akademisi | Akses data historis & model risiko |
| Integrator pihak ketiga | Webhook / API publik |

---

## 2. Tujuan & Ruang Lingkup

### 2.1 Tujuan
1. Mengumpulkan dan menormalisasi data multi-sumber secara berkala.
2. Menghitung **Indeks Risiko Longsor** (skor 0–100) dan **Level Peringatan** per grid/wilayah.
3. Menyediakan API baca (read) untuk status, prakiraan, dan histori.
4. Menerbitkan peringatan otomatis saat ambang batas terlampaui.
5. Menyediakan API tulis (write) untuk sensor, laporan warga, dan verifikasi petugas.

### 2.2 Ruang lingkup awal (MVP)
- Fokus wilayah awal: **Jawa Tengah** (35 kabupaten/kota, dapat diperluas).
- Data: curah hujan dari **BMKG**, topografi statis, batas wilayah, riwayat kejadian.
- Level peringatan 4 tingkat.
- Kanal distribusi: **WhatsApp & SMS**.
- Metode risiko: **rule-based** (tanpa ML di fase ini).

### 2.3 Di luar ruang lingkup (fase lanjutan)
- Model machine learning prediktif tingkat lanjut.
- Integrasi sirene IoT penuh.
- Aplikasi mobile native (cukup sediakan API).

---

## 3. Arsitektur Sistem

```
                    ┌──────────────────────────────────────────────┐
                    │                 DATA SOURCES                 │
                    │  BMKG API · Sensor IoT · Citra Satelit ·     │
                    │  Laporan Warga · Data Historis BNPB/InaRISK  │
                    └───────────────────────┬──────────────────────┘
                                            │ (ingestion)
                                            ▼
                    ┌──────────────────────────────────────────────┐
                    │              INGESTION LAYER                 │
                    │  Scheduler · Message Queue · Validator       │
                    └───────────────────────┬──────────────────────┘
                                            ▼
                    ┌──────────────────────────────────────────────┐
                    │              PROCESSING ENGINE               │
                    │  Normalisasi · Agregasi · Risk Scoring       │
                    │  Rule Engine · Model Prediksi                │
                    └───────────────────────┬──────────────────────┘
                                            ▼
                    ┌───────────────┬───────────────┬──────────────┐
                    │  PostgreSQL   │   TimescaleDB │    Redis     │
                    │  + PostGIS    │  (time-series)│   (cache)    │
                    └───────┬───────┴───────┬───────┴──────┬───────┘
                            └───────────────┼──────────────┘
                                            ▼
                    ┌──────────────────────────────────────────────┐
                    │                 API GATEWAY                  │
                    │   Auth · Rate Limit · Versioning · Logging   │
                    └───────────────────────┬──────────────────────┘
                                            ▼
                    ┌──────────────────────────────────────────────┐
                    │           ALERT & DISTRIBUTION               │
                    │  Webhook · Push · SMS/WA · Email · Sirene    │
                    └──────────────────────────────────────────────┘
```

### 3.1 Komponen
| Komponen | Tanggung jawab |
|---|---|
| Ingestion Service | Menarik data berkala (polling) & menerima push dari sensor |
| Processing Engine | Menghitung risiko, menjalankan rule & model |
| Alert Engine | Menerbitkan, mendeduplikasi, dan mengeskalasi peringatan |
| API Gateway | Entry point publik & internal, auth, rate limit |
| Notification Service | Kirim ke webhook, push, SMS/WA, email, sirene |
| Scheduler | Penjadwalan job (cron) untuk refresh data & scoring |

### 3.2 Teknologi yang diusulkan (versi produksi)
| Lapisan | Opsi |
|---|---|
| Bahasa/Framework | **Go (Golang)** — keputusan final; Chi/Fiber sebagai router |
| Database | PostgreSQL 15 + PostGIS |
| Time-series | TimescaleDB (extension PostgreSQL) |
| Cache/Queue | Redis + worker Go (asynq) / RabbitMQ |
| Object storage | S3-compatible (untuk citra & lampiran) |
| Deploy | Docker + Kubernetes / Docker Compose |
| Observability | Prometheus + Grafana, OpenTelemetry, Sentry |
| Docs | OpenAPI 3.1 (Swagger UI via swaggo) |

### 3.3 Stack GRATIS untuk belajar (rekomendasi)
Semua di bawah ini bisa dipakai tanpa biaya, cocok untuk belajar:

| Lapisan | Pilihan gratis | Catatan |
|---|---|---|
| Bahasa/Framework | **Go + Chi router** atau **Fiber** | Ringan, cepat, build jadi 1 binary |
| Database | **PostgreSQL + PostGIS** (lokal via Docker) | Alternatif online gratis: **Supabase**, **Neon** |
| Time-series | Tabel biasa dulu, upgrade ke **TimescaleDB** | Bisa di-skip saat belajar |
| Cache/Queue | **Redis** lokal, atau langsung `robfig/cron` | Cukup scheduler bawaan dulu |
| Object storage | **Cloudflare R2 / Supabase Storage** free tier | Untuk foto laporan warga |
| Deploy | **Render / Railway / Fly.io** free tier, atau lokal | Lihat tabel di §3.4 |
| Container | **Docker Desktop** / **Docker Compose** | Gratis untuk personal |
| Observability | **Log + Sentry free tier** | Cukup untuk belajar |
| Docs | **swaggo/swag** (generator Swagger dari Go) | Gratis |
| CI/CD | **GitHub Actions** free (2000 menit/bulan) | Opsional |

#### Library Go yang direkomendasikan
| Kebutuhan | Library |
|---|---|
| HTTP router | `github.com/go-chi/chi/v5` (standar) atau `github.com/gofiber/fiber/v2` |
| DB driver | `github.com/jackc/pgx/v5` + `github.com/sqlc-dev/sqlc` (generate query) |
| Migrasi | `github.com/golang-migrate/migrate` atau `github.com/pressly/goose` |
| Konfigurasi | `github.com/joho/godotenv` + env |
| Validasi | `github.com/go-playground/validator/v10` |
| Scheduler | `github.com/robfig/cron/v3` |
| HTTP client | `net/http` bawaan atau `github.com/go-resty/resty/v2` |
| JWT/Auth | `github.com/golang-jwt/jwt/v5` |
| Logging | `log/slog` (bawaan Go 1.21+) |
| UUID | `github.com/google/uuid` |
| Geo/GeoJSON | `github.com/paulmach/orb` |

#### Sumber data gratis
| Data | Sumber gratis | API Key? |
|---|---|---|
| Curah hujan & prakiraan | **BMKG** (data terbuka) | Tidak (publik) |
| Cadangan/uji coba | Open-Meteo | Tidak perlu |
| Peta rawan longsor | **InaRISK / BNPB** (unduhan) | Tidak |
| Peta dasar & elevasi | **OpenStreetMap**, SRTM/NASADEM | Tidak |
| Batas wilayah Jawa Tengah | **geoBoundaries / GADM / OSM** | Tidak |
| Notifikasi | **WhatsApp & SMS gateway** (lihat §3.5) | Lihat §3.5 |
| Email (opsional) | Gmail SMTP / Mailtrap free | Ya (gratis) |

> **Rekomendasi belajar:** mulai dari **Go (Chi) + PostgreSQL (Docker) + BMKG + WA/SMS**.
> Semua gratis (kecuali pengiriman WA/SMS nyata — lihat §3.5), tanpa cloud, dan sudah bisa
> mendemonstrasikan alur peringatan dini end-to-end.

#### Contoh langkah cepat (lokal)
```bash
# 1. Siapkan database via Docker
docker run --name pg-longsor -e POSTGRES_PASSWORD=rahasia \
  -p 5432:5432 -d postgis/postgis:15-3.4

# 2. Buat project Go
mkdir api-peringatan-dini-longsor && cd api-peringatan-dini-longsor
go mod init github.com/USERNAME/api-peringatan-dini-longsor
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5
go get github.com/robfig/cron/v3

# 3. Uji data hujan BMKG (data terbuka, tanpa key)
#    https://data.bmkg.go.id/ (lihat dokumentasi endpoint per wilayah)

# 4. Jalankan API
go run ./cmd/api        # buka http://localhost:8080/health
```

### 3.4 SLA & Hosting GRATIS untuk belajar
Jawaban singkat: **ada**, tetapi semua free tier punya batasan (bukan SLA produksi).

| Platform | Free tier | Kelebihan | Keterbatasan |
|---|---|---|---|
| **Fly.io** | Beberapa VM kecil gratis | Bisa jalan terus, Docker | Kuota terbatas, butuh kartu |
| **Render** | Web service gratis | Mudah, auto-deploy dari GitHub | **Sleep** setelah idle ~15 menit |
| **Railway** | Kredit awal bulanan | Deploy cepat | Kredit habis → berhenti |
| **Koyeb** | 1 service gratis | Docker native | Resource kecil |
| **Deta Space** | Gratis (beta) | Simpel | Perkembangan tak pasti |
| **Oracle Cloud Always Free** | VM ARM 4 core/24GB | **Paling murah hati, tidak tidur** | Setup agak rumit |
| **Google Cloud Run** | Kuota gratis bulanan | Scale-to-zero | Cold start |
| **Lokal / PC sendiri** | Gratis | Kontrol penuh | Tidak online publik |

**Rekomendasi:**
- **Belajar offline**: jalankan lokal (Docker Compose) — paling bebas.
- **Butuh publik & tidak tidur**: **Oracle Cloud Always Free** atau **Fly.io**.
- **Paling gampang dari GitHub**: **Render** (siap-siap efek "sleep").
- Database gratis: **Neon** atau **Supabase** (jangan simpan DB di platform web yang sleep).

**Target SLA realistis (belajar):** "best effort", target internal uptime 95%,
harian backup ke file, bukan 99.9%. Saat naik produksi, baru pindah ke berbayar.

### 3.5 Kanal WhatsApp & SMS (realitas gratis/hemat)

> **Penting:** WhatsApp & SMS **tidak benar-benar gratis** untuk dikirim massal ke warga
> (ada biaya per pesan). Untuk belajar, gunakan mode simulasi/uji:

| Kanal | Cara belajar hemat | Biaya |
|---|---|---|
| **WhatsApp** | **WhatsApp Cloud API** (Meta) — sandbox & pesan template uji | Ada kuota percobaan gratis, lalu berbayar per percakapan |
| **WhatsApp (uji)** | **Simulasi via log/console**, atau bot WA unofficial (berisiko diblokir) | Gratis untuk demo lokal |
| **SMS (uji)** | **Twilio trial**, **Vonage trial**, provider lokal trial | Kredit gratis terbatas |
| **SMS (Indonesia)** | Provider lokal (mis. Zenziva, RajaSMS, dsb.) | Berbayar per SMS |
| **Mode dev** | Tulis adaptor `Notifier` yang print ke log / simpan ke DB | **Gratis penuh** |

**Desain yang disarankan (penting untuk belajar):**
- Buat **interface** `Notifier` dengan metode `Send(ctx, message)`.
- Sediakan 3 implementasi: `LogNotifier` (dev/gratis), `WhatsAppNotifier`, `SMSNotifier`.
- Ganti implementasi lewat konfigurasi `.env` — tanpa ubah logika inti.

```go
type Notifier interface {
    Send(ctx context.Context, to string, msg Message) error
}
// implementasi: LogNotifier, WhatsAppNotifier, SMSNotifier
```

> Prinsip ini (dependency inversion) membuatmu bisa **belajar gratis sepenuhnya**
> sambil tetap siap plug-in provider berbayar saat produksi.

---

## 4. Model Domain (Entitas Utama)

| Entitas | Deskripsi |
|---|---|
| `Region` | Wilayah administratif (provinsi/kabupaten/kecamatan/desa) dengan geometri |
| `Grid` | Sel grid analisis risiko (mis. 1 km²) |
| `Sensor` | Perangkat pemantau (rain gauge, tiltmeter, piezometer, extensometer) |
| `SensorReading` | Data time-series dari sensor |
| `RainfallData` | Data curah hujan (observasi & prakiraan) |
| `LandslideSusceptibility` | Peta kerawanan statis per area (slope, litologi, tata guna lahan) |
| `RiskAssessment` | Hasil perhitungan risiko per grid/region per waktu |
| `Alert` | Peringatan yang diterbitkan (level, area, periode, status) |
| `Report` | Laporan warga/kejadian (geotag, foto, deskripsi, status verifikasi) |
| `AlertSubscription` | Langganan peringatan per pengguna/perangkat |
| `NotificationLog` | Riwayat pengiriman peringatan per kanal |
| `User` | Pengguna sistem (admin, petugas BPBD, publik) |
| `AuditLog` | Jejak aktivitas penting untuk audit |

### 4.1 Relasi Inti (ringkas)
```
Region 1─* Grid
Grid   1─* RiskAssessment
Region 1─* Alert
Alert  1─* NotificationLog
Sensor 1─* SensorReading
Region 1─* Report
Region 1─* RainfallData
User   1─* AlertSubscription
Grid   1─1 LandslideSusceptibility  (berbasis geometri/region)
```

---

## 5. Sumber Data & Integrasi

> **Sumber data utama yang dipilih: BMKG** (provinsi Jawa Tengah). Open-Meteo hanya cadangan/uji.

| Sumber | Jenis | Metode | Frekuensi | Catatan |
|---|---|---|---|---|
| **BMKG (utama)** | Curah hujan & prakiraan | REST API data terbuka (data.bmkg.go.id) | 10–60 menit | Ikuti ToS & atribusi |
| Open-Meteo (cadangan) | Curah hujan & prakiraan | REST API publik | 15–60 menit | **Tanpa API key**, pengganti saat BMKG gagal |
| Sensor IoT (opsional) | Tanah, hujan, pergerakan | MQTT / HTTPS push | 1–15 menit | Bisa disimulasikan saat belajar |
| Citra satelit (DEM, SAR) | Topografi, deformasi | Batch / unduhan berkala | Bulanan | **SRTM/NASADEM gratis** |
| InaRISK / BNPB | Peta rawan bencana | Unduhan dataset | Tahunan | Baseline kerawanan Jawa Tengah |
| Data historis longsor | Kejadian masa lalu | Unduhan / input manual | Ad hoc | Untuk kalibrasi ambang rule |
| Laporan warga (opsional) | Crowdsourced | API / mobile app | Real-time | Perlu verifikasi |
| Batas wilayah | Administratif | Unduhan | Jarang | **geoBoundaries/GADM/OSM gratis** |

### 5.1 Integrasi BMKG (Jawa Tengah)
- Sumber: portal data terbuka BMKG (`data.bmkg.go.id`), mencakup prakiraan cuaca per
  wilayah/kecamatan dan data curah hujan.
- Format umum: XML/JSON. Endpoint prakiraan cuaca per wilayah tersedia dalam JSON.
- Strategi penarikan:
  1. Petakan kode wilayah BMKG → wilayah internal (`Region`) saat *seeding*.
  2. Polling berkala (mis. tiap 30 menit) lewat scheduler Go.
  3. Simpan `RainfallData` + tandai `source=bmkg` dan `fetched_at`.
  4. Validasi & normalisasi satuan (mm, jam lokal Asia/Jakarta, WIB).
- **Fallback**: jika BMKG gagal/format berubah → pakai **Open-Meteo** dan tandai
  `data_freshness = degraded`.

### 5.2 Strategi fallback
- Bila sumber utama gagal → gunakan cache terakhir + tandai `data_freshness`.
- Bila sensor mati > ambang → tandai sensor `offline` dan turunkan bobot kepercayaan.
- Jika BMKG & cadangan gagal → peringatan tetap jalan dari data terakhir + flag jelas di respons API.

---

## 6. Logika Perhitungan Risiko (Rule-Based)

> **Metode:** rule-based — kombinasi **ambang curah hujan BMKG** + **kerawanan statis wilayah**
> (lereng & peta rawan longsor InaRISK). Tanpa machine learning di fase ini.
> Fokus awal: **Jawa Tengah**.

### 6.1 Faktor rule-based
| Faktor | Bobot awal | Sumber | Sifat |
|---|---|---|---|
| Curah hujan kumulatif 24 jam | 0.35 | **BMKG** | dinamis |
| Curah hujan kumulatif 72 jam | 0.25 | **BMKG** | dinamis |
| Curah hujan prakiraan 24 jam ke depan | 0.15 | **BMKG** | dinamis |
| Kerawanan dasar (InaRISK, litologi, tata guna lahan) | 0.15 | InaRISK / peta | statis |
| Kemiringan lereng (slope) | 0.10 | DEM (SRTM/NASADEM) | statis |

> Bobot bersifat konfigurasi (`config`) dan wajib dikalibrasi dengan data lokal Jawa Tengah.

### 6.2 Aturan penentuan level (rule utama)
```
# Normalisasi tiap faktor ke 0..1
n_rain24  = min(rain24 / 150.0, 1)
n_rain72  = min(rain72 / 200.0, 1)
n_forecast= min(forecast24 / 100.0, 1)
n_suscept = susceptibility_score / 100.0     # 0..1 dari InaRISK
n_slope   = min(slope_deg / 45.0, 1)

RiskScore = (0.35*n_rain24 + 0.25*n_rain72 + 0.15*n_forecast
             + 0.15*n_suscept + 0.10*n_slope) * 100

Level:
  0–24   → Level 1 (NORMAL)
  25–49  → Level 2 (WASPADA)
  50–74  → Level 3 (SIAGA)
  75–100 → Level 4 (AWAS)
```

### 6.3 Ambang hujan BMKG (contoh, dapat dikonfigurasi per wilayah Jateng)
| Level | Curah hujan kumulatif 24 jam | Curah hujan kumulatif 72 jam |
|---|---|---|
| Waspada | ≥ 50 mm | ≥ 80 mm |
| Siaga | ≥ 100 mm | ≥ 150 mm |
| Awas | ≥ 150 mm | ≥ 200 mm |

### 6.4 Aturan eskalasi & de-eskalasi (rule-based murni)
1. **Naik level otomatis** jika salah satu:
   - `rain24 ≥ ambang level` **dan** `susceptibility ≥ 0.6` (wilayah memang rawan), atau
   - hujan intensitas tinggi berturut-turut dalam 1 jam (mis. `rain1h ≥ 30 mm`).
2. **Kenaikan langsung ke Awas** jika `rain24 ≥ 150 mm` **atau** `rain72 ≥ 200 mm`
   pada wilayah `susceptibility ≥ 0.7`.
3. **Turun level** hanya setelah **periode tenang** 12–24 jam tanpa hujan ekstrem → cegah *flapping*.
4. Semua keputusan rule dicatat: `RiskAssessment` (trigger + nilai faktor) dan `AuditLog`.
5. Ambang disimpan sebagai konfigurasi agar bisa diubah tanpa deploy ulang.

### 6.5 Contoh pseudocode engine (Go)
```go
func Evaluate(in Input, cfg Config) Result {
    nRain24 := clamp(in.Rain24/cfg.MaxRain24, 0, 1)
    nRain72 := clamp(in.Rain72/cfg.MaxRain72, 0, 1)
    nFcst   := clamp(in.Forecast24/cfg.MaxForecast, 0, 1)
    nSus    := in.Susceptibility / 100.0
    nSlope  := clamp(in.SlopeDeg/45.0, 0, 1)

    score := (0.35*nRain24 + 0.25*nRain72 + 0.15*nFcst +
              0.15*nSus + 0.10*nSlope) * 100

    level := levelFromScore(score)           // 1..4
    if in.Rain24 >= 150 || in.Rain72 >= 200 {
        if in.Susceptibility >= 70 { level = 4 } // paksa Awas
    }
    return Result{Score: score, Level: level, Factors: in}
}
```

---

## 7. Desain API

Base URL: `https://api.example.go.id/v1`
Format: JSON, UTF-8. Waktu: ISO 8601 (`2026-10-03T14:00:00+07:00`).
Autentikasi: Bearer Token (OAuth2 / API Key), kecuali endpoint publik tertentu.

### 7.1 Ringkasan Endpoint

| Method | Path | Deskripsi | Auth |
|---|---|---|---|
| GET | `/health` | Health check | Tidak |
| GET | `/regions` | Daftar wilayah | Ya |
| GET | `/regions/{id}` | Detail wilayah | Ya |
| GET | `/regions/{id}/risk` | Risiko terkini wilayah | Tidak* |
| GET | `/regions/{id}/risk/history` | Histori risiko | Ya |
| GET | `/regions/{id}/forecast` | Prakiraan risiko | Ya |
| GET | `/grids/{id}/risk` | Risiko per grid | Ya |
| GET | `/alerts` | Daftar peringatan aktif | Tidak |
| GET | `/alerts/{id}` | Detail peringatan | Tidak |
| POST | `/alerts` | Terbitkan peringatan (manual) | Ya (petugas) |
| PATCH | `/alerts/{id}` | Ubah/akhiri peringatan | Ya (petugas) |
| GET | `/sensors` | Daftar sensor | Ya |
| GET | `/sensors/{id}/readings` | Data sensor time-series | Ya |
| POST | `/sensors/{id}/readings` | Kirim data sensor | Perangkat |
| GET | `/rainfall` | Data curah hujan | Ya |
| GET | `/reports` | Daftar laporan warga | Ya |
| POST | `/reports` | Kirim laporan warga | Opsional |
| PATCH | `/reports/{id}/verify` | Verifikasi laporan | Ya (petugas) |
| POST | `/subscriptions` | Langganan peringatan | Ya |
| DELETE | `/subscriptions/{id}` | Berhenti berlangganan | Ya |
| GET | `/stats` | Statistik agregat | Ya |

\* endpoint publik dibatasi rate limit & hanya level ringkas.

---

### 7.2 Contoh Detail Endpoint

#### GET `/regions/{id}/risk`
Respons:
```json
{
  "region_id": "kec-pangalengan",
  "name": "Pangalengan",
  "level": 3,
  "level_label": "SIAGA",
  "risk_score": 67.4,
  "factors": {
    "rainfall_24h_mm": 112.0,
    "rainfall_72h_mm": 178.5,
    "soil_moisture_pct": 82.1,
    "slope_deg": 28.4
  },
  "valid_until": "2026-10-03T20:00:00+07:00",
  "updated_at": "2026-10-03T14:00:00+07:00",
  "data_freshness": "fresh",
  "recommendation": "Tingkatkan pemantauan lereng; siapkan jalur evakuasi."
}
```

#### GET `/alerts?status=active&level=3,4`
Respons:
```json
{
  "data": [
    {
      "id": "alert_01HXYZ...",
      "region_id": "kec-pangalengan",
      "level": 3,
      "level_label": "SIAGA",
      "title": "Peringatan Siaga Longsor",
      "description": "Curah hujan tinggi 3 hari terakhir. Berpotensi longsor di lereng utara.",
      "area_geojson": { "type": "Polygon", "coordinates": [[[...]]] },
      "issued_at": "2026-10-03T14:05:00+07:00",
      "expires_at": "2026-10-04T14:05:00+07:00",
      "source": "automatic",
      "status": "active"
    }
  ],
  "meta": { "page": 1, "per_page": 20, "total": 1 }
}
```

#### POST `/reports`
Request:
```json
{
  "lat": -7.1234,
  "lon": 107.5678,
  "description": "Ada retakan tanah selebar 5 cm di halaman rumah",
  "severity_guess": "medium",
  "media_urls": ["https://storage.../foto1.jpg"],
  "reporter": { "name": "Budi", "phone": "+62..." }
}
```

#### POST `/alerts`
Request (petugas):
```json
{
  "region_id": "kec-pangalengan",
  "level": 4,
  "title": "Peringatan Awas Longsor",
  "description": "Evakuasi area lereng segera.",
  "valid_until": "2026-10-04T06:00:00+07:00",
  "channels": ["push", "sms", "webhook"]
}
```

---

### 7.3 Konvensi Umum

**Pagination**
```
GET /alerts?page=1&per_page=20
```

**Filtering & sorting**
```
GET /alerts?region_id=...&level=3,4&status=active&sort=-issued_at
```

**Error format**
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Field 'level' harus antara 1-4",
    "details": [{ "field": "level", "issue": "out_of_range" }]
  }
}
```

**Kode status HTTP**
| Kode | Arti |
|---|---|
| 200 | Sukses |
| 201 | Dibuat |
| 400 | Permintaan tidak valid |
| 401 | Tidak terautentikasi |
| 403 | Tidak berwenang |
| 404 | Tidak ditemukan |
| 409 | Konflik (mis. duplikat) |
| 422 | Validasi gagal |
| 429 | Rate limit |
| 500 | Kesalahan server |

---

### 7.4 Webhook (distribusi)

Sistem akan mengirim event ke subscriber:
```json
{
  "event": "alert.issued",
  "delivered_at": "2026-10-03T14:05:02+07:00",
  "signature": "sha256=...",
  "data": { "id": "alert_01HXYZ...", "level": 3, "region_id": "kec-pangalengan" }
}
```
- Event: `alert.issued`, `alert.updated`, `alert.expired`, `sensor.offline`, `report.created`.
- Verifikasi via HMAC-SHA256 header `X-Signature`.

---

## 8. Autentikasi & Otorisasi

- **Publik**: akses terbatas (status ringkas, daftar alert), rate limit ketat.
- **API Key**: untuk integrator pihak ketiga.
- **JWT**: untuk pengguna & dashboard (role-based).
- **Perangkat IoT**: token perangkat khusus (mTLS di produksi).

### Peran (roles)
| Role | Hak akses |
|---|---|
| `public` | Baca alert & risiko ringkas |
| `reporter` | Kirim laporan |
| `officer` | Verifikasi, terbitkan alert |
| `admin` | Kelola semua + konfigurasi |
| `device` | Kirim reading sensor |

### 8.1 Kepatuhan UU PDP No. 27/2022
Karena sistem menyimpan data pribadi (nomor HP/WhatsApp pelapor & pelanggan peringatan), wajib patuh UU PDP:

| Prinsip UU PDP | Penerapan di sistem |
|---|---|
| Dasar pemrosesan sah | Persetujuan (consent) eksplisit saat berlangganan / kirim laporan |
| Tujuan terbatas | Nomor HP hanya untuk kirim peringatan; tidak dipakai keperluan lain |
| Minimalisasi data | Simpan hanya yang perlu; pelapor boleh anonim |
| Transparansi | Kebijakan privasi + info penggunaan data |
| Hak subjek data | Endpoint untuk lihat/ubah/hapus data & berhenti langganan |
| Keamanan | Enkripsi at-rest & in-transit, akses berbasis peran, audit log |
| Retensi terbatas | Hapus/anonimkan data setelah tidak diperlukan (mis. 6–12 bulan) |
| Penanganan insiden | Prosedur notifikasi kebocoran ≤ 3×24 jam |
| Transfer lintas negara | Utamakan server di Indonesia / provider patuh PDP |

Penerapan teknis:
- Kolom `phone` disimpan **terenkripsi** (AES-GCM) + opsi hash untuk pencarian.
- Log **tidak menulis** nomor HP penuh (masking: `+62812****789`).
- `AuditLog` mencatat siapa mengakses data pribadi kapan.
- Endpoint `DELETE /me/subscriptions/{id}` & `DELETE /me/data` untuk hak subjek data.

---

## 9. Non-Fungsional

| Aspek | Target (produksi) | Catatan belajar (free tier) |
|---|---|---|
| Ketersediaan | ≥ 99.5% | best effort (~95%, bisa sleep) |
| Latensi API baca | p95 < 300 ms | p95 < 1 s |
| Throughput | 500 req/s | puluhan req/s |
| Kesegaran data | Sensor < 5 menit, hujan < 60 menit | hujan < 60 menit |
| Retensi data sensor | 2 tahun (raw), agregat permanen | secukupnya |
| Keamanan | TLS 1.3, enkripsi at-rest, audit log | TLS, hash password, mask PII |
| Backup | Harian, retensi 30 hari, uji restore bulanan | dump harian ke file |
| Rate limit | Publik 60/min, API key 600/min | publik 60/min |

### 9.1 Standar keamanan yang diterapkan
- **Transport**: HTTPS/TLS saja (HSTS), tolak HTTP.
- **Autentikasi**: JWT (HS256/RS256), expiry pendek + refresh; API key di-hash.
- **Otorisasi**: role-based access control (RBAC) di setiap endpoint.
- **Input**: validasi ketat + prepared statement (cegah SQL injection).
- **Rahasia**: semua kredensial via environment/secret manager, **bukan** di repo.
- **PII**: enkripsi nomor HP, masking di log (selaras UU PDP §8.1).
- **Rate limiting** & proteksi brute-force.
- **Audit logging** untuk aksi sensitif.
- **Headers keamanan**: `X-Content-Type-Options`, `X-Frame-Options`, CSP dasar.
- **Dependency**: `govulncheck` + update rutin.
- **Backup terenkripsi** + uji restore.

---

## 10. Alur Kerja Utama

### 10.1 Peringatan Otomatis
```
Scheduler (tiap 10 menit)
  → Tarik data hujan & sensor
  → Normalisasi & update time-series
  → Hitung RiskScore per grid
  → Agregasi ke level region
  → Rule Engine: apakah ambang terlampaui?
      → Jika ya & belum ada alert aktif → buat Alert
      → Jika level berubah → update + eskalasi
      → Jika aman & ada alert → akhiri setelah periode tenang
  → Notification Service kirim ke semua channel
  → Catat NotificationLog
```

### 10.2 Laporan Warga
```
Warga POST /reports
  → Validasi & simpan (status: pending)
  → Notifikasi petugas terdekat
  → Petugas PATCH /reports/{id}/verify
  → Jika valid & signifikan → bisa memicu/eskalasi alert
```

---

## 11. Struktur Proyek (Go)

```
api-peringatan-dini-longsor/
├── cmd/
│   └── api/
│       └── main.go            # entry point
├── internal/
│   ├── config/                # konfigurasi & env
│   ├── httpapi/               # router, handler, middleware
│   │   ├── router.go
│   │   ├── middleware.go
│   │   └── handlers_*.go
│   ├── domain/                # model & tipe inti
│   ├── store/                 # repository (interface + implementasi)
│   │   ├── memory.go          # implementasi in-memory (belajar)
│   │   └── postgres.go        # implementasi PostgreSQL
│   ├── engine/                # rule-based risk engine
│   ├── ingestion/             # penarik data BMKG
│   ├── notifier/              # Log, WhatsApp, SMS
│   ├── security/              # JWT, enkripsi PII, masking
│   └── scheduler/             # cron job
├── migrations/                # skema SQL
├── docs/
│   ├── RENCANA_API_PERINGATAN_DINI_LONGSOR.md
│   └── openapi.yaml
├── docker-compose.yml
├── Dockerfile
├── Makefile
├── .env.example
├── go.mod
└── README.md
```

> **Catatan penting:** implementasi awal memakai **storage in-memory** + **data contoh Jawa Tengah**,
> sehingga bisa langsung dijalankan **tanpa Docker/PostgreSQL**. PostgreSQL disiapkan sebagai
> pengembangan lanjutan (interface `Store` sudah disiapkan untuk itu).

---

## 12. Roadmap Pengembangan

| Fase | Fokus | Estimasi |
|---|---|---|
| Fase 0 | Riset data, definisi wilayah & model risiko | 1–2 minggu |
| Fase 1 (MVP) | Region, rainfall, risk scoring, alerts read | 3–4 minggu |
| Fase 2 | Sensor ingestion, reports, verifikasi | 3–4 minggu |
| Fase 3 | Distribusi (push/SMS/WA), subscriptions, webhook | 2–3 minggu |
| Fase 4 | Dashboard, statistik, hardening & audit | 3–4 minggu |
| Fase 5 | Model ML prediktif, integrasi sirene IoT | Berkelanjutan |

---

## 13. Risiko & Mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Kualitas data sensor rendah | Salah prediksi | Kalibrasi, validasi silang, tandai kepercayaan |
| Keterlambatan data BMKG | Peringatan telat | Cache, sumber cadangan, radar |
| False alarm tinggi | Kepercayaan turun | Kalibrasi ambang, periode tenang, verifikasi |
| Beban puncak saat bencana | API down | Autoscale, cache, degradasi anggun |
| Ketergantungan pihak ketiga | Single point of failure | Fallback multi-sumber |
| Privasi pelapor | Risiko hukum | Anonimisasi, persetujuan, enkripsi |
| Regulasi & ToS data | Akses dicabut | Patuhi lisensi & atribusi |

---

## 14. Rencana Belajar Gratis (Langkah demi Langkah)

Rencana praktis memakai stack Go, dari nol sampai jalan.

### 14.1 Prasyarat (semua gratis)
- **Go 1.22+** dan VS Code + ekstensi Go
- Docker Desktop (opsional, untuk PostgreSQL) — atau pakai in-memory dulu
- Nomor WhatsApp/SMS uji (opsional; mode dev pakai log)
- Git + GitHub (opsional)

### 14.2 Tahapan belajar
| Step | Target | Yang dipelajari |
|---|---|---|
| 1 | `GET /health` & `GET /regions` | Routing Go (Chi), handler |
| 2 | Simpan wilayah Jawa Tengah (in-memory) | Struktur data, repository |
| 3 | Tarik curah hujan dari BMKG | HTTP client, scheduler (cron) |
| 4 | Hitung `RiskScore` rule-based | Logika bisnis, ambang |
| 5 | Endpoint `/regions/{id}/risk` | Agregasi, JSON |
| 6 | Alert otomatis + simpan | Rule engine sederhana |
| 7 | Notifikasi via adaptor WA/SMS (dev: log) | Interface, dependency inversion |
| 8 | Migrasi ke PostgreSQL | SQL, migration |
| 9 | Deploy gratis (Fly.io/Oracle) | Deployment |
| 10 | Dokumentasi + tes | Swagger, `go test` |

### 14.3 Endpoint gratis yang dipakai
```
BMKG (data terbuka, tanpa key):
  https://data.bmkg.go.id/  → prakiraan cuaca per wilayah (JSON)

Cadangan (Open-Meteo, tanpa key):
  https://api.open-meteo.com/v1/forecast
    ?latitude=-7.12&longitude=110.42
    &hourly=precipitation&forecast_days=3&timezone=Asia%2FJakarta
```

### 14.4 Batasan versi gratis (yang perlu disadari)
- Free tier cloud bisa *sleep* saat idle → API lambat saat bangun.
- WhatsApp/SMS **tidak gratis** untuk pengiriman nyata → pakai mode log saat belajar (§3.5).
- Penyimpanan & bandwidth terbatas.
- Tanpa SLA / dukungan.

> Ini semua wajar untuk belajar. Saat naik ke produksi, tinggal ganti komponen bertahap.

---

## 15. Pertanyaan Terbuka (sudah sebagian dijawab)

1. ~~Framework & bahasa final?~~ → **Go** ✅
2. ~~Wilayah percontohan?~~ → **Jawa Tengah** ✅
3. ~~Sumber data?~~ → **BMKG** ✅
4. ~~Kanal notifikasi?~~ → **WhatsApp & SMS** ✅ (mode dev = log)
5. ~~Model risiko?~~ → **Rule-based** ✅
6. ~~SLA & hosting?~~ → **free tier untuk belajar** ✅ (lihat §3.4)
7. ~~Kepatuhan?~~ → **UU PDP + standar keamanan** ✅ (lihat §8.1 & §9.1)

**Masih terbuka:**
- Vendor WhatsApp/SMS final saat naik produksi?
- Cakupan wilayah: seluruh 35 kabupaten/kota Jateng, atau subset prioritas?
- Titik/koordinat wilayah BMKG mana yang jadi sumber data hujan utama?

---

## 16. Lampiran

### 16.1 Glosarium
- **DEM**: Digital Elevation Model — model elevasi digital.
- **SAR**: Synthetic Aperture Radar — radar untuk deteksi deformasi tanah.
- **Susceptibility**: tingkat kerawanan dasar suatu area.
- **Flapping**: kondisi level peringatan naik-turun cepat tak stabil.
- **Freshness**: indikator kesegaran data.

### 16.2 Referensi
- BMKG — https://data.bmkg.go.id (data cuaca terbuka)
- BNPB / InaRISK (peta rawan bencana)
- **Open-Meteo** — https://open-meteo.com (cadangan, tanpa API key)
- **OpenStreetMap / SRTM / geoBoundaries** (peta & batas wilayah gratis)
- **WhatsApp Cloud API** & provider SMS lokal (kanal distribusi)
- UU No. 27 Tahun 2022 tentang Pelindungan Data Pribadi (UU PDP)
- OpenAPI Specification 3.1
- Praktik terbaik desain REST API

---

> **Catatan:** Dokumen ini adalah kerangka awal. Silakan tandai bagian yang ingin diperdalam
> (misalnya model risiko, skema database detail, atau kontrak API final) agar bisa dilanjutkan.
