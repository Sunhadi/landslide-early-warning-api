package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"peringatan-dini/internal/config"
	"peringatan-dini/internal/domain"
	"peringatan-dini/internal/engine"
	"peringatan-dini/internal/notifier"
	"peringatan-dini/internal/security"
	"peringatan-dini/internal/service"
	"peringatan-dini/internal/store"
)

// harness membungkus server + store untuk pengujian.
type harness struct {
	srv   http.Handler
	store *store.MemoryStore
	token string
}

func newHarness(t *testing.T) *harness {
	return newHarnessWithConfig(t, nil)
}

// newHarnessWithConfig membuat harness; opts dapat menimpa Config:
// "env", "cors" ([]string), "jwtSecret", "maxBody", "loginMax", "loginLockout".
func newHarnessWithConfig(t *testing.T, opts map[string]any) *harness {
	t.Helper()
	cfg := config.Config{
		Env: "test", Port: "0",
		JWTSecret: "test-secret-yang-cukup-panjang", JWTTTL: time.Hour,
		JWTRefreshTTL: 24 * time.Hour,
		PIIKey:        "test-pii-key-yang-cukup-panjang",
		Rain24Waspada: 50, Rain24Siaga: 100, Rain24Awas: 150,
		Rain72Waspada: 80, Rain72Siaga: 150, Rain72Awas: 200,
		NotifierMode:     "log",
		CORSOrigins:      []string{"*"},
		MaxBodyBytes:     1 << 20,
		LoginMaxAttempts: 5,
		LoginLockout:     15 * time.Minute,
	}
	if opts != nil {
		if v, ok := opts["env"].(string); ok {
			cfg.Env = v
		}
		if v, ok := opts["cors"].([]string); ok {
			cfg.CORSOrigins = v
		}
		if v, ok := opts["jwtSecret"].(string); ok {
			cfg.JWTSecret = v
		}
		if v, ok := opts["maxBody"].(int64); ok {
			cfg.MaxBodyBytes = v
		}
		if v, ok := opts["loginMax"].(int); ok {
			cfg.LoginMaxAttempts = v
		}
		if v, ok := opts["loginLockout"].(time.Duration); ok {
			cfg.LoginLockout = v
		}
	}
	st := store.NewMemoryStore()
	_ = st.SaveRegion(context.Background(), domain.Region{
		ID: "r1", Adm4: "33.04.01.2001", Name: "Susukan", Kabupaten: "Banjarnegara",
		Kecamatan: "Susukan", Susceptibility: 88, SlopeDeg: 32,
	})

	// akun petugas
	hash, _ := security.HashPassword("petugas123")
	_ = st.SaveUser(context.Background(), domain.User{ID: "u1", Username: "petugas", PasswordHash: hash, Role: "officer"})

	cipher, _ := security.NewCipher(cfg.PIIKey)
	tokens := security.NewTokenServiceWithRefresh(cfg.JWTSecret, cfg.JWTTTL, cfg.JWTRefreshTTL)
	eng := engine.New(cfg)
	notif := notifier.NewFromConfig(cfg)
	svc := service.New(st, eng, notif, nil)
	server := NewServer(cfg, st, svc, tokens, cipher)

	tok, _ := tokens.Issue("u1", "officer")
	return &harness{srv: server.Router(), store: st, token: tok}
}

// do menjalankan request dan mengembalikan recorder.
func (h *harness) do(t *testing.T, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.srv.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode gagal: %v (body=%s)", err, rec.Body.String())
	}
	return v
}

// --- Tests ---

func TestHealth(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/health", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("security header tidak ada")
	}
}

func TestListRegions(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/v1/regions", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := decode[struct {
		Data []domain.Region `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}](t, rec)
	if body.Meta.Total != 1 || len(body.Data) != 1 {
		t.Fatalf("total = %d", body.Meta.Total)
	}
}

func TestGetRegionNotFound(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/v1/regions/tidak-ada", "", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404", rec.Code)
	}
}

func TestGetRiskAutoEvaluate(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/v1/regions/r1/risk", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := decode[map[string]any](t, rec)
	if body["level_label"] != "NORMAL" {
		t.Errorf("level = %v", body["level_label"])
	}
}

func TestLoginSuccessAndFailure(t *testing.T) {
	h := newHarness(t)

	ok := h.do(t, http.MethodPost, "/v1/auth/login", `{"username":"petugas","password":"petugas123"}`, "")
	if ok.Code != http.StatusOK {
		t.Fatalf("login sukses status = %d", ok.Code)
	}
	res := decode[map[string]any](t, ok)
	if res["token"] == "" || res["role"] != "officer" {
		t.Fatalf("respons login salah: %v", res)
	}

	bad := h.do(t, http.MethodPost, "/v1/auth/login", `{"username":"petugas","password":"salah"}`, "")
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("login gagal status = %d, mau 401", bad.Code)
	}
}

func TestProtectedEndpointRequiresAuth(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/v1/audit", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", rec.Code)
	}
}

func TestRoleEnforcement(t *testing.T) {
	h := newHarness(t)
	// token officer mencoba akses endpoint admin
	rec := h.do(t, http.MethodGet, "/v1/audit", "", h.token)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, mau 403", rec.Code)
	}
}

func TestCreateAlertValidation(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/alerts", `{"region_id":"r1","level":9}`, h.token)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, mau 422", rec.Code)
	}
}

func TestCreateAlertManual(t *testing.T) {
	h := newHarness(t)
	body := `{"region_id":"r1","level":4,"title":"Awas","description":"evakuasi","valid_hours":6}`
	rec := h.do(t, http.MethodPost, "/v1/alerts", body, h.token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	a := decode[domain.Alert](t, rec)
	if a.LevelLabel != "AWAS" || a.Source != "manual" || a.ID == "" {
		t.Fatalf("alert salah: %+v", a)
	}

	// tampil di daftar alert aktif
	list := h.do(t, http.MethodGet, "/v1/alerts?status=active", "", "")
	res := decode[struct {
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}](t, list)
	if res.Meta.Total != 1 {
		t.Fatalf("alert aktif = %d, mau 1", res.Meta.Total)
	}
}

func TestSubscriptionRequiresConsent(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/subscriptions",
		`{"region_id":"r1","phone":"+628123456789","channel":"whatsapp","consent":false}`, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, mau 422", rec.Code)
	}
	body := decode[map[string]any](t, rec)
	errObj := body["error"].(map[string]any)
	if errObj["code"] != "CONSENT_REQUIRED" {
		t.Errorf("kode = %v", errObj["code"])
	}
}

func TestSubscriptionMasksAndEncryptsPhone(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/subscriptions",
		`{"region_id":"r1","phone":"+628123456789","channel":"sms","consent":true}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	sub := decode[domain.Subscription](t, rec)
	if sub.PhoneMask != "+6281234****789" {
		t.Errorf("mask = %q", sub.PhoneMask)
	}
	if sub.ID == "" {
		t.Error("ID harus terisi")
	}
	// nomor asli tidak boleh ada di body respons
	if bytes.Contains(rec.Body.Bytes(), []byte("+628123456789")) {
		t.Error("nomor HP asli tidak boleh bocor di respons")
	}
	// tersimpan terenkripsi (bukan plaintext)
	stored, _ := h.store.ListSubscriptions(context.Background(), "r1")
	if len(stored) != 1 || stored[0].Phone == "+628123456789" {
		t.Error("nomor HP harus tersimpan terenkripsi")
	}
}

func TestCreateReportAndVerify(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/reports",
		`{"lat":-7.5,"lon":109.4,"description":"retakan tanah","reporter_phone":"+628999888777"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	rep := decode[domain.Report](t, rec)
	if rep.ID == "" {
		t.Fatal("ID laporan harus terisi")
	}
	if rep.Status != "pending" {
		t.Fatalf("status = %q, mau pending", rep.Status)
	}

	ver := h.do(t, http.MethodPatch, "/v1/reports/"+rep.ID+"/verify", `{"status":"verified"}`, h.token)
	if ver.Code != http.StatusOK {
		t.Fatalf("verifikasi status = %d, body=%s", ver.Code, ver.Body.String())
	}
	got := decode[domain.Report](t, ver)
	if got.Status != "verified" {
		t.Fatalf("status = %q", got.Status)
	}
	if got.ReporterPhone != "" {
		t.Error("nomor HP tidak boleh dikembalikan ke klien")
	}
}

func TestReportValidation(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/reports", `{"lat":-7.5,"lon":109.4}`, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, mau 422", rec.Code)
	}
}

func TestRiskEvaluateEndpoint(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/risk/evaluate", "", h.token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	res := decode[map[string]any](t, rec)
	if res["evaluated"].(float64) != 1 {
		t.Errorf("evaluated = %v", res["evaluated"])
	}
}

func TestNotFoundRoute(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/v1/tidak-ada", "", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404", rec.Code)
	}
}

func TestInvalidJSONBody(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/alerts", `{bukan json`, h.token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400", rec.Code)
	}
}
