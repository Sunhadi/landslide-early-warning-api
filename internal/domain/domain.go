// Package domain berisi tipe inti (entitas) aplikasi peringatan dini longsor.
// Tipe-tipe di sini tidak bergantung pada HTTP maupun database.
package domain

import "time"

// RiskLevel adalah level peringatan longsor.
type RiskLevel int

const (
	LevelNormal  RiskLevel = 1 // NORMAL
	LevelWaspada RiskLevel = 2 // WASPADA
	LevelSiaga   RiskLevel = 3 // SIAGA
	LevelAwas    RiskLevel = 4 // AWAS
)

// Label mengembalikan label teks level.
func (l RiskLevel) Label() string {
	switch l {
	case LevelWaspada:
		return "WASPADA"
	case LevelSiaga:
		return "SIAGA"
	case LevelAwas:
		return "AWAS"
	default:
		return "NORMAL"
	}
}

// Recommendation mengembalikan saran tindakan sesuai level.
func (l RiskLevel) Recommendation() string {
	switch l {
	case LevelWaspada:
		return "Pantau perkembangan cuaca dan kondisi lereng sekitar."
	case LevelSiaga:
		return "Tingkatkan pemantauan lereng; siapkan jalur evakuasi dan tas siaga."
	case LevelAwas:
		return "Evakuasi segera ke lokasi aman; ikuti arahan petugas BPBD."
	default:
		return "Tidak ada tindakan khusus. Tetap waspada saat hujan lebat."
	}
}

// Region adalah wilayah administratif (kabupaten/kota/kecamatan) di Jawa Tengah.
type Region struct {
	ID             string    `json:"id"`
	Adm4           string    `json:"adm4"` // kode wilayah BMKG level desa
	Name           string    `json:"name"`
	Kabupaten      string    `json:"kabupaten"`
	Kecamatan      string    `json:"kecamatan"`
	Lat            float64   `json:"lat"`
	Lon            float64   `json:"lon"`
	SlopeDeg       float64   `json:"slope_deg"`      // kemiringan lereng rata-rata
	Susceptibility float64   `json:"susceptibility"` // 0..100 kerawanan dasar (InaRISK)
	UpdatedAt      time.Time `json:"updated_at"`
}

// RainfallData adalah data curah hujan (observasi/prakiraan) dari BMKG.
type RainfallData struct {
	RegionID    string    `json:"region_id"`
	Source      string    `json:"source"` // "bmkg" | "open-meteo"
	ObservedAt  time.Time `json:"observed_at"`
	Rain24hMM   float64   `json:"rain_24h_mm"`
	Rain72hMM   float64   `json:"rain_72h_mm"`
	Forecast24h float64   `json:"forecast_24h_mm"`
	FetchedAt   time.Time `json:"fetched_at"`
	Freshness   string    `json:"freshness"` // "fresh" | "stale" | "degraded"
}

// RiskAssessment adalah hasil perhitungan risiko untuk sebuah wilayah.
type RiskAssessment struct {
	RegionID    string             `json:"region_id"`
	RegionName  string             `json:"region_name"`
	Score       float64            `json:"risk_score"`
	Level       RiskLevel          `json:"level"`
	LevelLabel  string             `json:"level_label"`
	Factors     map[string]float64 `json:"factors"`
	Freshness   string             `json:"data_freshness"`
	Triggers    []string           `json:"triggers,omitempty"`
	EvaluatedAt time.Time          `json:"evaluated_at"`
}

// Alert adalah peringatan yang diterbitkan.
type Alert struct {
	ID          string    `json:"id"`
	RegionID    string    `json:"region_id"`
	RegionName  string    `json:"region_name"`
	Level       RiskLevel `json:"level"`
	LevelLabel  string    `json:"level_label"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	IssuedAt    time.Time `json:"issued_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Source      string    `json:"source"` // "automatic" | "manual"
	Status      string    `json:"status"` // "active" | "expired" | "cancelled"
	Channels    []string  `json:"channels"`
}

// Report adalah laporan warga / kejadian.
type Report struct {
	ID            string    `json:"id"`
	Lat           float64   `json:"lat"`
	Lon           float64   `json:"lon"`
	RegionID      string    `json:"region_id,omitempty"`
	Description   string    `json:"description"`
	SeverityGuess string    `json:"severity_guess"`
	Status        string    `json:"status"` // "pending" | "verified" | "rejected"
	ReporterName  string    `json:"reporter_name,omitempty"`
	ReporterPhone string    `json:"reporter_phone,omitempty"` // disimpan terenkripsi di produksi
	CreatedAt     time.Time `json:"created_at"`
}

// Subscription adalah langganan peringatan per nomor WA/SMS.
type Subscription struct {
	ID        string    `json:"id"`
	RegionID  string    `json:"region_id"`
	Phone     string    `json:"-"`
	PhoneMask string    `json:"phone_masked"`
	Channel   string    `json:"channel"` // "whatsapp" | "sms"
	Consent   bool      `json:"consent"` // persetujuan UU PDP
	CreatedAt time.Time `json:"created_at"`
}

// User adalah pengguna sistem.
type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // public | reporter | officer | admin | device
}
