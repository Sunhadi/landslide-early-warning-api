package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"peringatan-dini/internal/config"
	"peringatan-dini/internal/domain"
	"peringatan-dini/internal/engine"
	"peringatan-dini/internal/notifier"
	"peringatan-dini/internal/store"
)

// --- Test doubles ---

// captureNotifier mencatat pesan yang dikirim.
type captureNotifier struct {
	channel string
	sent    []notifier.Message
	fail    bool
}

func (c *captureNotifier) Send(_ context.Context, msg notifier.Message) error {
	if c.fail {
		return errors.New("gagal kirim")
	}
	c.sent = append(c.sent, msg)
	return nil
}

func (c *captureNotifier) Channel() string { return c.channel }

// fakeFetcher mengembalikan data hujan tetap atau error.
type fakeFetcher struct {
	data    domain.RainfallData
	err     error
	calls   int
	lastAdm string
}

func (f *fakeFetcher) FetchRainfall(_ context.Context, adm4, regionID string) (domain.RainfallData, error) {
	f.calls++
	f.lastAdm = adm4
	if f.err != nil {
		return domain.RainfallData{}, f.err
	}
	d := f.data
	d.RegionID = regionID
	return d, nil
}

// --- Helpers ---

func newTestService(t *testing.T) (*Service, *store.MemoryStore, *captureNotifier, *captureNotifier) {
	t.Helper()
	st := store.NewMemoryStore()
	cfg := config.Config{
		Rain24Waspada: 50, Rain24Siaga: 100, Rain24Awas: 150,
		Rain72Waspada: 80, Rain72Siaga: 150, Rain72Awas: 200,
	}
	wa := &captureNotifier{channel: "whatsapp"}
	sms := &captureNotifier{channel: "sms"}
	svc := New(st, engine.New(cfg), map[string]notifier.Notifier{"whatsapp": wa, "sms": sms}, nil)
	return svc, st, wa, sms
}

func seedRegion(t *testing.T, st *store.MemoryStore, id string, suscept, slope float64) {
	t.Helper()
	err := st.SaveRegion(context.Background(), domain.Region{
		ID: id, Adm4: "33.00.00.0000", Name: "Wilayah " + id,
		Susceptibility: suscept, SlopeDeg: slope,
	})
	if err != nil {
		t.Fatalf("seed region: %v", err)
	}
}

// --- Tests ---

func TestEvaluateRegionUsesRainfall(t *testing.T) {
	svc, st, _, _ := newTestService(t)
	seedRegion(t, st, "r1", 88, 32)
	_ = st.SaveRainfall(context.Background(), domain.RainfallData{
		RegionID: "r1", Rain24hMM: 160, Rain72hMM: 210, Forecast24h: 50, Freshness: "fresh",
	})

	a, err := svc.EvaluateRegion(context.Background(), "r1")
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if a.Level != domain.LevelAwas {
		t.Fatalf("mau AWAS, dapat %s", a.LevelLabel)
	}
	if a.RegionName != "Wilayah r1" {
		t.Errorf("nama wilayah tidak terisi: %q", a.RegionName)
	}
	if a.Freshness != "fresh" {
		t.Errorf("freshness = %q", a.Freshness)
	}
	// tersimpan
	stored, err := st.GetAssessment(context.Background(), "r1")
	if err != nil || stored.Level != domain.LevelAwas {
		t.Fatalf("assessment tidak tersimpan: %v %v", stored, err)
	}
}

func TestEvaluateRegionNoRainfall(t *testing.T) {
	svc, st, _, _ := newTestService(t)
	seedRegion(t, st, "r1", 20, 5)

	a, err := svc.EvaluateRegion(context.Background(), "r1")
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if a.Freshness != "no-data" {
		t.Errorf("freshness = %q, mau no-data", a.Freshness)
	}
	if a.Level != domain.LevelNormal {
		t.Errorf("mau NORMAL, dapat %s", a.LevelLabel)
	}
}

func TestEvaluateRegionNotFound(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	if _, err := svc.EvaluateRegion(context.Background(), "tidak-ada"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("mau ErrNotFound, dapat %v", err)
	}
}

func TestEvaluateAll(t *testing.T) {
	svc, st, _, _ := newTestService(t)
	seedRegion(t, st, "r1", 80, 30)
	seedRegion(t, st, "r2", 20, 5)
	out, err := svc.EvaluateAll(context.Background())
	if err != nil {
		t.Fatalf("evaluate all: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("hasil = %d, mau 2", len(out))
	}
}

func TestProcessAlertsCreatesAndDispatches(t *testing.T) {
	svc, st, wa, sms := newTestService(t)
	seedRegion(t, st, "r1", 90, 32)
	// berlangganan dua kanal
	_ = st.SaveSubscription(context.Background(), domain.Subscription{ID: "s1", RegionID: "r1", Channel: "whatsapp", PhoneMask: "+6281111****111"})
	_ = st.SaveSubscription(context.Background(), domain.Subscription{ID: "s2", RegionID: "r1", Channel: "sms", PhoneMask: "+6282222****222"})

	_ = st.SaveRainfall(context.Background(), domain.RainfallData{RegionID: "r1", Rain24hMM: 160, Rain72hMM: 210, Freshness: "fresh"})
	if _, err := svc.EvaluateRegion(context.Background(), "r1"); err != nil {
		t.Fatalf("eval: %v", err)
	}

	actions, err := svc.ProcessAlerts(context.Background())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("actions = %v, mau 1", actions)
	}
	if len(wa.sent) != 1 || len(sms.sent) != 1 {
		t.Fatalf("notifikasi wa=%d sms=%d, mau masing-masing 1", len(wa.sent), len(sms.sent))
	}
	if wa.sent[0].To != "+6281111****111" {
		t.Errorf("penerima WA salah: %q", wa.sent[0].To)
	}

	// memanggil lagi tidak boleh menerbitkan duplikat
	actions2, _ := svc.ProcessAlerts(context.Background())
	if len(actions2) != 0 {
		t.Fatalf("tidak boleh ada aksi duplikat, dapat %v", actions2)
	}
}

func TestProcessAlertsExpiresWhenCalm(t *testing.T) {
	svc, st, _, _ := newTestService(t)
	seedRegion(t, st, "r1", 20, 5)
	// sudah ada alert aktif, tapi kondisi sekarang normal
	_ = st.SaveAlert(context.Background(), domain.Alert{ID: "a1", RegionID: "r1", Level: domain.LevelSiaga, Status: "active"})
	if _, err := svc.EvaluateRegion(context.Background(), "r1"); err != nil {
		t.Fatalf("eval: %v", err)
	}

	actions, err := svc.ProcessAlerts(context.Background())
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("harus ada 1 aksi (akhiri), dapat %v", actions)
	}
	a, _ := st.GetAlert(context.Background(), "a1")
	if a.Status != "expired" {
		t.Errorf("status alert = %q, mau expired", a.Status)
	}
}

func TestProcessAlertsEscalatesLevel(t *testing.T) {
	svc, st, _, _ := newTestService(t)
	seedRegion(t, st, "r1", 90, 32)
	_ = st.SaveAlert(context.Background(), domain.Alert{ID: "a1", RegionID: "r1", Level: domain.LevelSiaga, Status: "active"})
	_ = st.SaveRainfall(context.Background(), domain.RainfallData{RegionID: "r1", Rain24hMM: 160, Rain72hMM: 210, Freshness: "fresh"})
	if _, err := svc.EvaluateRegion(context.Background(), "r1"); err != nil {
		t.Fatalf("eval: %v", err)
	}

	actions, _ := svc.ProcessAlerts(context.Background())
	if len(actions) != 1 {
		t.Fatalf("harus menerbitkan alert baru (eskalasi), dapat %v", actions)
	}
}

func TestPublishManual(t *testing.T) {
	svc, st, _, _ := newTestService(t)
	seedRegion(t, st, "r1", 50, 20)

	a, err := svc.PublishManual(context.Background(), "r1", domain.LevelAwas, "Judul", "Deskripsi", time.Hour, []string{"sms"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if a.Source != "manual" || a.Status != "active" {
		t.Errorf("alert salah: source=%q status=%q", a.Source, a.Status)
	}
	if a.ID == "" {
		t.Error("ID harus terisi")
	}
	if !a.ExpiresAt.After(a.IssuedAt) {
		t.Error("expires_at harus setelah issued_at")
	}
}

func TestPublishManualRegionNotFound(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	if _, err := svc.PublishManual(context.Background(), "x", domain.LevelSiaga, "", "", time.Hour, nil); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("mau ErrNotFound, dapat %v", err)
	}
}

func TestSyncRegion(t *testing.T) {
	svc, st, _, _ := newTestService(t)
	seedRegion(t, st, "r1", 80, 30)
	f := &fakeFetcher{data: domain.RainfallData{Rain24hMM: 120, Rain72hMM: 130, Freshness: "fresh"}}
	svc.Fetcher = f

	rf, err := svc.SyncRegion(context.Background(), "r1")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if f.calls != 1 || f.lastAdm != "33.00.00.0000" {
		t.Errorf("fetcher dipanggil %d kali dengan adm4 %q", f.calls, f.lastAdm)
	}
	if rf.Rain24hMM != 120 {
		t.Errorf("rain24 = %v", rf.Rain24hMM)
	}
	stored, _ := st.GetRainfall(context.Background(), "r1")
	if stored.Rain24hMM != 120 {
		t.Error("rainfall harus tersimpan")
	}
}

func TestSyncAllCountsSuccess(t *testing.T) {
	svc, st, _, _ := newTestService(t)
	seedRegion(t, st, "r1", 80, 30)
	seedRegion(t, st, "r2", 60, 20)
	svc.Fetcher = &fakeFetcher{data: domain.RainfallData{Freshness: "fresh"}}

	if n := svc.SyncAll(context.Background()); n != 2 {
		t.Fatalf("sync sukses = %d, mau 2", n)
	}
}

func TestSyncRegionNoFetcher(t *testing.T) {
	svc, st, _, _ := newTestService(t)
	seedRegion(t, st, "r1", 80, 30)
	if _, err := svc.SyncRegion(context.Background(), "r1"); err == nil {
		t.Fatal("tanpa fetcher harus error")
	}
}

func TestSyncRegionFetcherError(t *testing.T) {
	svc, st, _, _ := newTestService(t)
	seedRegion(t, st, "r1", 80, 30)
	svc.Fetcher = &fakeFetcher{err: errors.New("bmkg down")}
	if _, err := svc.SyncRegion(context.Background(), "r1"); err == nil {
		t.Fatal("error fetcher harus diteruskan")
	}
}

func TestDispatchIgnoresUnknownChannel(t *testing.T) {
	svc, st, wa, _ := newTestService(t)
	seedRegion(t, st, "r1", 90, 32)
	// langganan dengan kanal yang tidak punya notifier
	_ = st.SaveSubscription(context.Background(), domain.Subscription{ID: "s1", RegionID: "r1", Channel: "telegram"})
	_ = st.SaveSubscription(context.Background(), domain.Subscription{ID: "s2", RegionID: "r1", Channel: "whatsapp", PhoneMask: "+62****"})
	_ = st.SaveRainfall(context.Background(), domain.RainfallData{RegionID: "r1", Rain24hMM: 160, Rain72hMM: 210})

	_, _ = svc.EvaluateRegion(context.Background(), "r1")
	_, _ = svc.ProcessAlerts(context.Background())

	if len(wa.sent) != 1 {
		t.Fatalf("WA terkirim = %d, mau 1", len(wa.sent))
	}
}
