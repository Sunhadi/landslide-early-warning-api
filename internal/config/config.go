// Package config memuat konfigurasi aplikasi dari environment variable.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config menyimpan seluruh konfigurasi runtime.
type Config struct {
	Env           string
	Port          string
	JWTSecret     string
	JWTTTL        time.Duration
	JWTRefreshTTL time.Duration
	PIIKey        string // secret enkripsi nomor HP (UU PDP)
	BMKGBaseURL   string
	FetchEnabled  bool
	FetchCron     string // ekspresi cron untuk penarikan data BMKG
	AlertCron     string // ekspresi cron untuk evaluasi risiko

	// Ambang curah hujan (mm), rule-based
	Rain24Waspada float64
	Rain24Siaga   float64
	Rain24Awas    float64
	Rain72Waspada float64
	Rain72Siaga   float64
	Rain72Awas    float64

	// Konfigurasi notifikasi
	NotifierMode string // "log" | "live"
	WAEndpoint   string
	WAKey        string
	SMSProvider  string
	SMSKey       string
	SMSFrom      string

	// Admin default untuk seeding
	AdminUser string
	AdminPass string

	// Keamanan tambahan
	CORSOrigins      []string // daftar origin yang diizinkan; ["*"] = semua
	MaxBodyBytes     int64    // batas ukuran body request (byte)
	LoginMaxAttempts int      // jumlah percobaan login gagal sebelum lockout
	LoginLockout     time.Duration
	TrustProxy       bool // percaya header X-Forwarded-For (di belakang reverse proxy)

	// Penyimpanan
	DatabaseURL string // bila diisi, pakai PostgreSQL; kosong = in-memory
}

// IsProduction mengembalikan true bila berjalan di mode produksi.
func (c Config) IsProduction() bool {
	return c.Env == "production" || c.Env == "prod"
}

// Validate memastikan konfigurasi aman. Mengembalikan error agar aplikasi
// gagal cepat (fail-fast) bila secret default dipakai di produksi.
func (c Config) Validate() error {
	if c.IsProduction() {
		if isWeakSecret(c.JWTSecret) {
			return errors.New("JWT_SECRET wajib diisi dengan nilai acak yang kuat di produksi")
		}
		if isWeakSecret(c.PIIKey) {
			return errors.New("PII_ENCRYPTION_KEY wajib diisi dengan nilai acak yang kuat di produksi")
		}
		if isWeakSecret(c.AdminPass) {
			return errors.New("ADMIN_PASS wajib diganti dari default di produksi")
		}
	}
	return nil
}

var weakSecrets = map[string]bool{
	"":                                  true,
	"dev-secret-ganti-di-produksi":      true,
	"dev-pii-key-ganti-di-produksi":     true,
	"ganti-dengan-rahasia-panjang-acak": true,
	"ganti-dengan-kunci-enkripsi-acak":  true,
	"ganti-di-produksi":                 true,
	"secret":                            true,
	"changeme":                          true,
	"password":                          true,
}

// isWeakSecret menganggap secret lemah bila kosong, ada di daftar umum,
// atau terlalu pendek (< 16 karakter).
func isWeakSecret(s string) bool {
	if weakSecrets[strings.ToLower(strings.TrimSpace(s))] {
		return true
	}
	return len(s) < 16
}

// Load membaca konfigurasi dari environment (default aman untuk belajar).
func Load() Config {
	return Config{
		Env:           getenv("APP_ENV", "development"),
		Port:          getenv("PORT", "8080"),
		JWTSecret:     getenv("JWT_SECRET", "dev-secret-ganti-di-produksi"),
		JWTTTL:        time.Duration(getenvInt("JWT_TTL_HOURS", 12)) * time.Hour,
		JWTRefreshTTL: time.Duration(getenvInt("JWT_REFRESH_TTL_HOURS", 168)) * time.Hour,
		PIIKey:        getenv("PII_ENCRYPTION_KEY", "dev-pii-key-ganti-di-produksi"),
		BMKGBaseURL:   getenv("BMKG_BASE_URL", "https://api.bmkg.go.id/publik/prakiraan-cuaca"),
		FetchEnabled:  getenvBool("FETCH_ENABLED", true),
		FetchCron:     getenv("FETCH_CRON", "*/30 * * * *"),
		AlertCron:     getenv("ALERT_CRON", "*/10 * * * *"),

		Rain24Waspada: getenvFloat("RAIN24_WASPADA", 50),
		Rain24Siaga:   getenvFloat("RAIN24_SIAGA", 100),
		Rain24Awas:    getenvFloat("RAIN24_AWAS", 150),
		Rain72Waspada: getenvFloat("RAIN72_WASPADA", 80),
		Rain72Siaga:   getenvFloat("RAIN72_SIAGA", 150),
		Rain72Awas:    getenvFloat("RAIN72_AWAS", 200),

		NotifierMode: getenv("NOTIFIER_MODE", "log"),
		WAEndpoint:   getenv("WA_ENDPOINT", ""),
		WAKey:        getenv("WA_API_KEY", ""),
		SMSProvider:  getenv("SMS_PROVIDER", ""),
		SMSKey:       getenv("SMS_API_KEY", ""),
		SMSFrom:      getenv("SMS_FROM", "BMKG-JTG"),

		AdminUser: getenv("ADMIN_USER", "admin"),
		AdminPass: getenv("ADMIN_PASS", "admin123"),

		CORSOrigins:      splitCSV(getenv("CORS_ORIGINS", "*")),
		MaxBodyBytes:     int64(getenvInt("MAX_BODY_BYTES", 1<<20)), // 1 MiB
		LoginMaxAttempts: getenvInt("LOGIN_MAX_ATTEMPTS", 5),
		LoginLockout:     time.Duration(getenvInt("LOGIN_LOCKOUT_MINUTES", 15)) * time.Minute,
		TrustProxy:       getenvBool("TRUST_PROXY", false),
		DatabaseURL:      getenv("DATABASE_URL", ""),
	}
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return def
}

func getenvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
