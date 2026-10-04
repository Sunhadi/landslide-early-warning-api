package main

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"peringatan-dini/internal/config"
	"peringatan-dini/internal/domain"
	"peringatan-dini/internal/security"
	"peringatan-dini/internal/store"
)

// seedRegions mengisi wilayah contoh di Jawa Tengah.
// Kode adm4 valid & koordinat diambil dari API BMKG.
// susceptibility = kerawanan dasar (0..100), slope = kemiringan lereng (derajat).
func seedRegions(ctx context.Context, st store.Store) {
	list := []domain.Region{
		{ID: "jateng-banjarnegara-susukan", Adm4: "33.04.01.2001", Name: "Susukan, Banjarnegara", Kabupaten: "Banjarnegara", Kecamatan: "Susukan", Lat: -7.5010, Lon: 109.4423, SlopeDeg: 32, Susceptibility: 88},
		{ID: "jateng-pekalongan-kandangserang", Adm4: "33.26.01.2001", Name: "Kandangserang, Pekalongan", Kabupaten: "Pekalongan", Kecamatan: "Kandangserang", Lat: -7.2298, Lon: 109.5409, SlopeDeg: 30, Susceptibility: 82},
		{ID: "jateng-semarang-getasan", Adm4: "33.22.01.2001", Name: "Getasan, Semarang", Kabupaten: "Semarang", Kecamatan: "Getasan", Lat: -7.4145, Lon: 110.4504, SlopeDeg: 28, Susceptibility: 76},
		{ID: "jateng-karanganyar-jatipuro", Adm4: "33.13.01.2001", Name: "Jatipuro, Karanganyar", Kabupaten: "Karanganyar", Kecamatan: "Jatipuro", Lat: -7.7679, Lon: 111.0380, SlopeDeg: 27, Susceptibility: 74},
		{ID: "jateng-boyolali-selo", Adm4: "33.09.01.2001", Name: "Selo, Boyolali", Kabupaten: "Boyolali", Kecamatan: "Selo", Lat: -7.5191, Lon: 110.4172, SlopeDeg: 26, Susceptibility: 70},
		{ID: "jateng-magelang-salaman", Adm4: "33.08.01.2001", Name: "Salaman, Magelang", Kabupaten: "Magelang", Kecamatan: "Salaman", Lat: -7.6361, Lon: 110.1489, SlopeDeg: 22, Susceptibility: 64},
		{ID: "jateng-purworejo-grabag", Adm4: "33.06.01.2001", Name: "Grabag, Purworejo", Kabupaten: "Purworejo", Kecamatan: "Grabag", Lat: -7.8347, Lon: 109.9113, SlopeDeg: 20, Susceptibility: 60},
		{ID: "jateng-batang-wonotunggal", Adm4: "33.25.01.2001", Name: "Wonotunggal, Batang", Kabupaten: "Batang", Kecamatan: "Wonotunggal", Lat: -7.0955, Lon: 109.7615, SlopeDeg: 18, Susceptibility: 55},
		{ID: "jateng-grobogan-kedungjati", Adm4: "33.15.01.2001", Name: "Kedungjati, Grobogan", Kabupaten: "Grobogan", Kecamatan: "Kedungjati", Lat: -7.2113, Lon: 110.6435, SlopeDeg: 12, Susceptibility: 35},
		{ID: "jateng-semarangtengah", Adm4: "33.74.01.1001", Name: "Semarang Tengah, Kota Semarang", Kabupaten: "Kota Semarang", Kecamatan: "Semarang Tengah", Lat: -6.9837, Lon: 110.4187, SlopeDeg: 5, Susceptibility: 20},
	}

	for _, r := range list {
		if err := st.SaveRegion(ctx, r); err != nil {
			slog.Error("gagal seed wilayah", "region", r.ID, "err", err)
		}
	}
	slog.Info("seed wilayah selesai", "jumlah", len(list))
}

// seedAdmin membuat akun admin & petugas default.
func seedAdmin(ctx context.Context, cfg config.Config, st store.Store) {
	mk := func(username, password, role string) {
		hash, err := security.HashPassword(password)
		if err != nil {
			slog.Error("gagal hash password", "user", username, "err", err)
			return
		}
		_ = st.SaveUser(ctx, domain.User{
			ID:           uuid.NewString(),
			Username:     username,
			PasswordHash: hash,
			Role:         role,
		})
	}
	mk(cfg.AdminUser, cfg.AdminPass, "admin")
	mk("petugas", "petugas123", "officer")
	slog.Info("seed akun selesai", "admin", cfg.AdminUser, "petugas", "petugas")
}
