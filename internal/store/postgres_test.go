package store

import (
	"context"
	"os"
	"testing"
	"time"

	"peringatan-dini/internal/domain"
)

// TestPostgresStore menjalankan uji integrasi terhadap PostgreSQL nyata.
// Di-skip kecuali TEST_DATABASE_URL diisi, mis:
//
//	$env:TEST_DATABASE_URL="postgres://longsor:rahasia@localhost:5432/longsor?sslmode=disable"
//	go test ./internal/store -run TestPostgresStore -v
//
// Skema harus sudah diterapkan (lihat migrations/0001_init.sql).
func TestPostgresStore(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL tidak diisi; lewati uji integrasi PostgreSQL")
	}

	ctx := context.Background()
	st, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("koneksi postgres: %v", err)
	}
	// Daftarkan Close lebih dulu agar berjalan TERAKHIR (t.Cleanup bersifat LIFO),
	// sehingga pembersihan data di bawah masih bisa memakai pool.
	t.Cleanup(st.Close)

	// Region
	reg := domain.Region{ID: "test-region-1", Adm4: "33.00.00.0001", Name: "Uji Wilayah",
		Kabupaten: "Uji", Kecamatan: "Uji", Lat: -7.5, Lon: 109.4, SlopeDeg: 30, Susceptibility: 80}
	if err := st.SaveRegion(ctx, reg); err != nil {
		t.Fatalf("save region: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.pool.Exec(context.Background(), "DELETE FROM regions WHERE id = $1", reg.ID)
		_, _ = st.pool.Exec(context.Background(), "DELETE FROM alerts WHERE region_id = $1", reg.ID)
	})

	got, err := st.GetRegion(ctx, reg.ID)
	if err != nil || got.Name != reg.Name {
		t.Fatalf("get region: %+v err=%v", got, err)
	}
	if _, err := st.GetRegion(ctx, "tidak-ada"); !isNotFound(err) {
		t.Fatalf("region tidak ada harus ErrNotFound, dapat %v", err)
	}

	list, err := st.ListRegions(ctx)
	if err != nil || len(list) == 0 {
		t.Fatalf("list regions: %v len=%d", err, len(list))
	}

	// Rainfall
	rf := domain.RainfallData{RegionID: reg.ID, Source: "bmkg", ObservedAt: time.Now(),
		Rain24hMM: 120, Rain72hMM: 180, Forecast24h: 40, FetchedAt: time.Now(), Freshness: "fresh"}
	if err := st.SaveRainfall(ctx, rf); err != nil {
		t.Fatalf("save rainfall: %v", err)
	}
	gotRF, err := st.GetRainfall(ctx, reg.ID)
	if err != nil || gotRF.Rain24hMM != 120 {
		t.Fatalf("get rainfall: %+v err=%v", gotRF, err)
	}

	// Assessment
	as := domain.RiskAssessment{RegionID: reg.ID, Score: 77.5, Level: domain.LevelAwas,
		LevelLabel: "AWAS", Factors: map[string]float64{"rain_24h_mm": 120},
		Triggers: []string{"uji"}, Freshness: "fresh", EvaluatedAt: time.Now()}
	if err := st.SaveAssessment(ctx, as); err != nil {
		t.Fatalf("save assessment: %v", err)
	}
	gotAs, err := st.GetAssessment(ctx, reg.ID)
	if err != nil || gotAs.Level != domain.LevelAwas || gotAs.RegionName != reg.Name {
		t.Fatalf("get assessment: %+v err=%v", gotAs, err)
	}

	// Alert
	al := domain.Alert{RegionID: reg.ID, Level: domain.LevelAwas, LevelLabel: "AWAS",
		Title: "Uji", Description: "Uji", IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
		Source: "manual", Status: "active", Channels: []string{"sms"}}
	if err := st.SaveAlert(ctx, al); err != nil {
		t.Fatalf("save alert: %v", err)
	}
	if a, ok, err := st.ActiveAlertForRegion(ctx, reg.ID); err != nil || !ok || a.Level != domain.LevelAwas {
		t.Fatalf("active alert: %+v ok=%v err=%v", a, ok, err)
	}
	alerts, _ := st.ListAlerts(ctx, true)
	if len(alerts) == 0 {
		t.Fatal("list active alerts kosong")
	}
	if err := st.SaveAlert(ctx, al); err == nil {
		// update status -> expired
	}

	// Report
	rep := domain.Report{Lat: -7.5, Lon: 109.4, Description: "uji", Status: "pending", CreatedAt: time.Now()}
	if err := st.SaveReport(ctx, rep); err != nil {
		t.Fatalf("save report: %v", err)
	}
	t.Cleanup(func() { _, _ = st.pool.Exec(context.Background(), "DELETE FROM reports WHERE description = 'uji'") })

	// Subscription
	sub := domain.Subscription{RegionID: reg.ID, Phone: "enc", PhoneMask: "+62****", Channel: "sms", Consent: true, CreatedAt: time.Now()}
	if err := st.SaveSubscription(ctx, sub); err != nil {
		t.Fatalf("save subscription: %v", err)
	}
	subs, _ := st.ListSubscriptions(ctx, reg.ID)
	if len(subs) == 0 {
		t.Fatal("list subscriptions kosong")
	}
	if err := st.DeleteSubscription(ctx, subs[0].ID); err != nil {
		t.Fatalf("delete subscription: %v", err)
	}
	if err := st.DeleteSubscription(ctx, subs[0].ID); !isNotFound(err) {
		t.Fatal("hapus dua kali harus ErrNotFound")
	}

	// User
	u := domain.User{Username: "pgtest_" + reg.ID, PasswordHash: "hash", Role: "officer"}
	if err := st.SaveUser(ctx, u); err != nil {
		t.Fatalf("save user: %v", err)
	}
	t.Cleanup(func() { _, _ = st.pool.Exec(context.Background(), "DELETE FROM users WHERE username = $1", u.Username) })
	if gotU, err := st.GetUserByUsername(ctx, u.Username); err != nil || gotU.Role != "officer" {
		t.Fatalf("get user: %+v err=%v", gotU, err)
	}

	// Audit
	st.AppendAudit(ctx, "tester", "pg.test", "ok")
	entries, err := st.ListAudit(ctx, 5)
	if err != nil || len(entries) == 0 {
		t.Fatalf("list audit: %v len=%d", err, len(entries))
	}
}

func isNotFound(err error) bool {
	return err != nil && err == ErrNotFound
}
