-- Skema awal PostgreSQL untuk API Peringatan Dini Longsor (Jawa Tengah).
-- Versi ini TANPA PostGIS (memakai kolom lat/lon biasa) agar bisa jalan
-- di PostgreSQL standar. Untuk upgrade ke PostGIS, lihat catatan di bawah.

-- Catatan: bila PostGIS tersedia, tambahkan:
--   CREATE EXTENSION IF NOT EXISTS postgis;
-- lalu ganti kolom lat/lon dengan GEOMETRY(Point, 4326).

-- Wilayah administratif
CREATE TABLE IF NOT EXISTS regions (
    id              TEXT PRIMARY KEY,
    adm4            TEXT UNIQUE NOT NULL,
    name            TEXT NOT NULL,
    kabupaten       TEXT NOT NULL,
    kecamatan       TEXT NOT NULL,
    lat             DOUBLE PRECISION NOT NULL DEFAULT 0,
    lon             DOUBLE PRECISION NOT NULL DEFAULT 0,
    slope_deg       DOUBLE PRECISION NOT NULL DEFAULT 0,
    susceptibility  DOUBLE PRECISION NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Data curah hujan (time-series)
CREATE TABLE IF NOT EXISTS rainfall (
    id              BIGSERIAL PRIMARY KEY,
    region_id       TEXT NOT NULL REFERENCES regions(id) ON DELETE CASCADE,
    source          TEXT NOT NULL,
    observed_at     TIMESTAMPTZ NOT NULL,
    rain_24h_mm     DOUBLE PRECISION NOT NULL DEFAULT 0,
    rain_72h_mm     DOUBLE PRECISION NOT NULL DEFAULT 0,
    forecast_24h_mm DOUBLE PRECISION NOT NULL DEFAULT 0,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    freshness       TEXT NOT NULL DEFAULT 'fresh'
);
CREATE INDEX IF NOT EXISTS idx_rainfall_region_time ON rainfall(region_id, observed_at DESC);

-- Hasil penilaian risiko
CREATE TABLE IF NOT EXISTS risk_assessments (
    id              BIGSERIAL PRIMARY KEY,
    region_id       TEXT NOT NULL REFERENCES regions(id) ON DELETE CASCADE,
    score           DOUBLE PRECISION NOT NULL,
    level           INT NOT NULL,
    level_label     TEXT NOT NULL,
    factors         JSONB NOT NULL DEFAULT '{}',
    triggers        JSONB NOT NULL DEFAULT '[]',
    freshness       TEXT NOT NULL DEFAULT 'fresh',
    evaluated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_assessment_region_time ON risk_assessments(region_id, evaluated_at DESC);

-- Peringatan
CREATE TABLE IF NOT EXISTS alerts (
    id              UUID PRIMARY KEY,
    region_id       TEXT NOT NULL REFERENCES regions(id) ON DELETE CASCADE,
    level           INT NOT NULL,
    level_label     TEXT NOT NULL,
    title           TEXT NOT NULL,
    description     TEXT NOT NULL,
    issued_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,
    source          TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active',
    channels        JSONB NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS idx_alerts_region_status ON alerts(region_id, status);

-- Laporan warga (nomor HP terenkripsi - UU PDP)
CREATE TABLE IF NOT EXISTS reports (
    id              UUID PRIMARY KEY,
    lat             DOUBLE PRECISION NOT NULL DEFAULT 0,
    lon             DOUBLE PRECISION NOT NULL DEFAULT 0,
    region_id       TEXT REFERENCES regions(id) ON DELETE SET NULL,
    description     TEXT NOT NULL,
    severity_guess  TEXT,
    status          TEXT NOT NULL DEFAULT 'pending',
    reporter_name   TEXT,
    reporter_phone_enc TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Langganan peringatan (nomor HP terenkripsi - UU PDP, consent wajib)
CREATE TABLE IF NOT EXISTS subscriptions (
    id              UUID PRIMARY KEY,
    region_id       TEXT NOT NULL REFERENCES regions(id) ON DELETE CASCADE,
    phone_enc       TEXT NOT NULL,
    phone_masked    TEXT NOT NULL,
    channel         TEXT NOT NULL CHECK (channel IN ('whatsapp','sms')),
    consent         BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Pengguna
CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY,
    username        TEXT UNIQUE NOT NULL,
    password_hash   TEXT NOT NULL,
    role            TEXT NOT NULL CHECK (role IN ('public','reporter','officer','admin','device')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Audit log
CREATE TABLE IF NOT EXISTS audit_log (
    id              BIGSERIAL PRIMARY KEY,
    at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor           TEXT NOT NULL,
    action          TEXT NOT NULL,
    detail          TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_audit_at ON audit_log(at DESC);
