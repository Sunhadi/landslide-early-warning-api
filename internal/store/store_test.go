package store

import (
	"context"
	"errors"
	"testing"

	"peringatan-dini/internal/domain"
)

func TestRegionSaveGetList(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()

	if _, err := st.GetRegion(ctx, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("mau ErrNotFound, dapat %v", err)
	}
	_ = st.SaveRegion(ctx, domain.Region{ID: "r1", Name: "A"})
	_ = st.SaveRegion(ctx, domain.Region{ID: "r2", Name: "B"})

	r, err := st.GetRegion(ctx, "r1")
	if err != nil || r.Name != "A" {
		t.Fatalf("get region: %v %v", r, err)
	}
	list, _ := st.ListRegions(ctx)
	if len(list) != 2 {
		t.Fatalf("jumlah wilayah = %d, mau 2", len(list))
	}
}

func TestRainfallSaveGet(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	if _, err := st.GetRainfall(ctx, "r1"); !errors.Is(err, ErrNotFound) {
		t.Fatal("harus ErrNotFound sebelum disimpan")
	}
	_ = st.SaveRainfall(ctx, domain.RainfallData{RegionID: "r1", Rain24hMM: 120})
	rf, err := st.GetRainfall(ctx, "r1")
	if err != nil || rf.Rain24hMM != 120 {
		t.Fatalf("rainfall: %v %v", rf, err)
	}
}

func TestActiveAlertForRegion(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()

	if _, ok, _ := st.ActiveAlertForRegion(ctx, "r1"); ok {
		t.Fatal("awalnya tidak boleh ada alert aktif")
	}
	_ = st.SaveAlert(ctx, domain.Alert{ID: "a1", RegionID: "r1", Status: "expired", Level: domain.LevelSiaga})
	if _, ok, _ := st.ActiveAlertForRegion(ctx, "r1"); ok {
		t.Fatal("alert expired tidak dihitung aktif")
	}
	_ = st.SaveAlert(ctx, domain.Alert{ID: "a2", RegionID: "r1", Status: "active", Level: domain.LevelAwas})
	a, ok, _ := st.ActiveAlertForRegion(ctx, "r1")
	if !ok || a.Level != domain.LevelAwas {
		t.Fatalf("harus menemukan alert aktif AWAS, dapat %+v ok=%v", a, ok)
	}
	_ = st.SaveAlert(ctx, domain.Alert{ID: "a3", RegionID: "r2", Status: "active"})
	list, _ := st.ListAlerts(ctx, true)
	if len(list) != 2 {
		t.Fatalf("alert aktif = %d, mau 2", len(list))
	}
}

func TestSaveAlertAssignsID(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	_ = st.SaveAlert(ctx, domain.Alert{RegionID: "r1"})
	list, _ := st.ListAlerts(ctx, false)
	if len(list) != 1 || list[0].ID == "" {
		t.Fatal("SaveAlert harus mengisi ID bila kosong")
	}
}

func TestSubscriptions(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	_ = st.SaveSubscription(ctx, domain.Subscription{ID: "s1", RegionID: "r1", Channel: "sms"})
	_ = st.SaveSubscription(ctx, domain.Subscription{ID: "s2", RegionID: "r2", Channel: "whatsapp"})

	all, _ := st.ListSubscriptions(ctx, "")
	if len(all) != 2 {
		t.Fatalf("semua langganan = %d, mau 2", len(all))
	}
	filtered, _ := st.ListSubscriptions(ctx, "r1")
	if len(filtered) != 1 || filtered[0].ID != "s1" {
		t.Fatalf("filter region gagal: %+v", filtered)
	}
	if err := st.DeleteSubscription(ctx, "s1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := st.DeleteSubscription(ctx, "s1"); !errors.Is(err, ErrNotFound) {
		t.Fatal("hapus dua kali harus ErrNotFound")
	}
}

func TestUsers(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	_ = st.SaveUser(ctx, domain.User{ID: "u1", Username: "Admin", Role: "admin"})

	// pencarian tidak case-sensitive
	u, err := st.GetUserByUsername(ctx, "admin")
	if err != nil || u.ID != "u1" {
		t.Fatalf("get user: %v %v", u, err)
	}
	if _, err := st.GetUserByUsername(ctx, "tidak-ada"); !errors.Is(err, ErrNotFound) {
		t.Fatal("user tidak ada harus ErrNotFound")
	}
}

func TestAudit(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	st.AppendAudit(ctx, "a", "login", "ok")
	st.AppendAudit(ctx, "b", "logout", "ok")
	st.AppendAudit(ctx, "c", "x", "ok")

	entries, _ := st.ListAudit(ctx, 2)
	if len(entries) != 2 {
		t.Fatalf("limit audit = %d, mau 2", len(entries))
	}
	// terbaru dulu
	if entries[0].Actor != "c" {
		t.Errorf("entri terbaru harus 'c', dapat %q", entries[0].Actor)
	}
}

func TestMemoryStoreConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	_ = st.SaveRegion(ctx, domain.Region{ID: "r1", Name: "A"})

	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_, _ = st.ListRegions(ctx)
			_ = st.SaveRainfall(ctx, domain.RainfallData{RegionID: "r1"})
			st.AppendAudit(ctx, "g", "a", "d")
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}
