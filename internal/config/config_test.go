package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	// pastikan env bersih dari variabel yang diuji
	for _, k := range []string{"PORT", "APP_ENV", "RAIN24_WASPADA", "FETCH_ENABLED", "JWT_TTL_HOURS", "NOTIFIER_MODE"} {
		t.Setenv(k, "")
	}
	cfg := Load()
	if cfg.Env != "development" {
		t.Errorf("env = %q", cfg.Env)
	}
	if cfg.Port != "8080" {
		t.Errorf("port = %q", cfg.Port)
	}
	if cfg.Rain24Waspada != 50 {
		t.Errorf("rain24 waspada = %v", cfg.Rain24Waspada)
	}
	if !cfg.FetchEnabled {
		t.Error("fetch harus aktif secara default")
	}
	if cfg.JWTTTL != 12*time.Hour {
		t.Errorf("ttl = %v", cfg.JWTTTL)
	}
	if cfg.NotifierMode != "log" {
		t.Errorf("notifier mode = %q", cfg.NotifierMode)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("APP_ENV", "production")
	t.Setenv("RAIN24_AWAS", "170")
	t.Setenv("FETCH_ENABLED", "false")
	t.Setenv("JWT_TTL_HOURS", "1")
	t.Setenv("NOTIFIER_MODE", "live")

	cfg := Load()
	if cfg.Port != "9999" || cfg.Env != "production" {
		t.Errorf("port/env salah: %s/%s", cfg.Port, cfg.Env)
	}
	if cfg.Rain24Awas != 170 {
		t.Errorf("rain24 awas = %v", cfg.Rain24Awas)
	}
	if cfg.FetchEnabled {
		t.Error("fetch harus nonaktif")
	}
	if cfg.JWTTTL != time.Hour {
		t.Errorf("ttl = %v", cfg.JWTTTL)
	}
	if cfg.NotifierMode != "live" {
		t.Errorf("mode = %q", cfg.NotifierMode)
	}
}

func TestLoadIgnoresInvalidEnv(t *testing.T) {
	t.Setenv("RAIN24_SIAGA", "bukan-angka")
	t.Setenv("JWT_TTL_HOURS", "xx")
	cfg := Load()
	if cfg.Rain24Siaga != 100 {
		t.Errorf("nilai tidak valid harus pakai default, dapat %v", cfg.Rain24Siaga)
	}
	if cfg.JWTTTL != 12*time.Hour {
		t.Errorf("ttl default tidak dipakai: %v", cfg.JWTTTL)
	}
}

func TestValidateProductionRejectsWeakSecrets(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"jwt default", Config{Env: "production", JWTSecret: "dev-secret-ganti-di-produksi", PIIKey: "ini-kunci-panjang-sekali", AdminPass: "sangat-rahasia-123"}},
		{"pii default", Config{Env: "production", JWTSecret: "ini-rahasia-jwt-panjang", PIIKey: "dev-pii-key-ganti-di-produksi", AdminPass: "sangat-rahasia-123"}},
		{"admin default", Config{Env: "production", JWTSecret: "ini-rahasia-jwt-panjang", PIIKey: "ini-kunci-panjang-sekali", AdminPass: "admin123"}},
		{"jwt terlalu pendek", Config{Env: "production", JWTSecret: "pendek", PIIKey: "ini-kunci-panjang-sekali", AdminPass: "sangat-rahasia-123"}},
		{"jwt kosong", Config{Env: "production", JWTSecret: "", PIIKey: "ini-kunci-panjang-sekali", AdminPass: "sangat-rahasia-123"}},
	}
	for _, c := range cases {
		if err := c.cfg.Validate(); err == nil {
			t.Errorf("%s: seharusnya error", c.name)
		}
	}
}

func TestValidateProductionAcceptsStrongSecrets(t *testing.T) {
	cfg := Config{
		Env:       "production",
		JWTSecret: "rahasia-jwt-yang-panjang-dan-acak-12345",
		PIIKey:    "kunci-enkripsi-pii-panjang-dan-acak-67890",
		AdminPass: "password-admin-yang-kuat-abcdef",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("seharusnya valid, dapat error: %v", err)
	}
}

func TestValidateDevelopmentAllowsDefaults(t *testing.T) {
	cfg := Config{Env: "development", JWTSecret: "dev", PIIKey: "dev", AdminPass: "admin123"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("mode development tidak boleh menolak default: %v", err)
	}
}

func TestIsProduction(t *testing.T) {
	if !(Config{Env: "production"}).IsProduction() {
		t.Error("production harus true")
	}
	if !(Config{Env: "prod"}).IsProduction() {
		t.Error("prod harus true")
	}
	if (Config{Env: "development"}).IsProduction() {
		t.Error("development harus false")
	}
}
