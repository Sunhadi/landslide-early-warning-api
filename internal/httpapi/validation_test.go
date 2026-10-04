package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidPhone(t *testing.T) {
	valid := []string{"+628123456789", "628123456789", "08123456789", "0812-3456-789", "+62 812 3456 789"}
	for _, p := range valid {
		if !validPhone(p) {
			t.Errorf("%q seharusnya valid", p)
		}
	}
	invalid := []string{"", "123", "abc", "+1234567890", "628123", "08a1234567"}
	for _, p := range invalid {
		if validPhone(p) {
			t.Errorf("%q seharusnya tidak valid", p)
		}
	}
}

func TestNormalizePhone(t *testing.T) {
	if got := normalizePhone("+62 812-3456-789"); got != "+628123456789" {
		t.Errorf("normalize = %q", got)
	}
	if got := normalizePhone("(0812) 3456 789"); got != "08123456789" {
		t.Errorf("normalize = %q", got)
	}
}

func TestValidLatLon(t *testing.T) {
	if !validLatLon(-7.5, 109.4) {
		t.Error("koordinat Jateng harus valid")
	}
	if validLatLon(95, 109) {
		t.Error("lat > 90 harus invalid")
	}
	if validLatLon(-7, 200) {
		t.Error("lon > 180 harus invalid")
	}
}

func TestValidText(t *testing.T) {
	if !validText("halo", 10) {
		t.Error("teks pendek harus valid")
	}
	if validText("", 10) {
		t.Error("teks kosong harus invalid")
	}
	if validText("   ", 10) {
		t.Error("teks spasi harus invalid")
	}
	if validText("12345678901", 10) {
		t.Error("melebihi batas harus invalid")
	}
}

func TestOneOf(t *testing.T) {
	if !oneOf("sms", "whatsapp", "sms") {
		t.Error("sms harus cocok")
	}
	if oneOf("email", "whatsapp", "sms") {
		t.Error("email tidak boleh cocok")
	}
}

// --- Integrasi hardening ---

func TestBodyLimitReturns413(t *testing.T) {
	h := newHarness(t)
	// body jauh melebihi batas default 1 MiB
	big := `{"description":"` + strings.Repeat("a", 2<<20) + `"}`
	rec := h.do(t, http.MethodPost, "/v1/reports", big, "")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, mau 413", rec.Code)
	}
}

func TestCORSHeadersOnOptions(t *testing.T) {
	h := newHarness(t)
	req := httptest.NewRequest(http.MethodOptions, "/v1/regions", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	h.srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, mau 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("header CORS tidak ada")
	}
}

func TestCORSOriginNotAllowed(t *testing.T) {
	// harness default CORS "*", buat server dengan origin spesifik
	h := newHarnessWithConfig(t, map[string]any{"cors": []string{"https://boleh.example.com"}})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://jahat.example.com")
	rec := httptest.NewRecorder()
	h.srv.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("origin tidak dikenal tidak boleh mendapat header CORS")
	}
}

func TestHSTSOnlyInProduction(t *testing.T) {
	prod := newHarnessWithConfig(t, map[string]any{"env": "production"})
	rec := prod.do(t, http.MethodGet, "/health", "", "")
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Error("HSTS harus ada di produksi")
	}

	dev := newHarness(t)
	recDev := dev.do(t, http.MethodGet, "/health", "", "")
	if recDev.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS tidak boleh ada di development")
	}
}

func TestPermissionsPolicyHeader(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/health", "", "")
	if rec.Header().Get("Permissions-Policy") == "" {
		t.Error("Permissions-Policy harus ada")
	}
}

func TestLoginLockoutAfterMaxAttempts(t *testing.T) {
	h := newHarness(t)
	body := `{"username":"petugas","password":"salah"}`

	// 5 percobaan gagal (max default)
	for i := 0; i < 5; i++ {
		rec := h.do(t, http.MethodPost, "/v1/auth/login", body, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("percobaan %d status = %d", i+1, rec.Code)
		}
	}
	// percobaan ke-6 harus terkunci (429)
	locked := h.do(t, http.MethodPost, "/v1/auth/login", body, "")
	if locked.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, mau 429 terkunci", locked.Code)
	}
	if locked.Header().Get("Retry-After") == "" {
		t.Error("header Retry-After harus ada saat terkunci")
	}
	// bahkan dengan password benar tetap terkunci
	correct := h.do(t, http.MethodPost, "/v1/auth/login", `{"username":"petugas","password":"petugas123"}`, "")
	if correct.Code != http.StatusTooManyRequests {
		t.Fatalf("saat terkunci status = %d, mau 429", correct.Code)
	}
}

func TestSubscriptionInvalidPhoneRejected(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/subscriptions",
		`{"region_id":"r1","phone":"123","channel":"sms","consent":true}`, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, mau 422", rec.Code)
	}
}

func TestReportInvalidCoordinatesRejected(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/reports",
		`{"lat":200,"lon":999,"description":"tes"}`, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, mau 422", rec.Code)
	}
}

func TestReportInvalidPhoneRejected(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/reports",
		`{"lat":-7.5,"lon":109.4,"description":"tes","reporter_phone":"abc"}`, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, mau 422", rec.Code)
	}
}

func TestAlertTitleRequired(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/alerts",
		`{"region_id":"r1","level":3,"description":"x"}`, h.token)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, mau 422", rec.Code)
	}
}

func TestAlertInvalidChannelRejected(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/alerts",
		`{"region_id":"r1","level":3,"title":"x","description":"y","channels":["telegram"]}`, h.token)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, mau 422", rec.Code)
	}
}

// --- Refresh token & logout ---

func TestLoginReturnsRefreshToken(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/auth/login", `{"username":"petugas","password":"petugas123"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	res := decode[map[string]any](t, rec)
	if res["refresh_token"] == "" || res["refresh_token"] == nil {
		t.Fatal("respons login harus menyertakan refresh_token")
	}
	if res["expires_in"] == nil {
		t.Fatal("respons login harus menyertakan expires_in")
	}
}

func TestRefreshEndpointRotates(t *testing.T) {
	h := newHarness(t)
	login := h.do(t, http.MethodPost, "/v1/auth/login", `{"username":"petugas","password":"petugas123"}`, "")
	lr := decode[map[string]any](t, login)
	refresh := lr["refresh_token"].(string)

	rec := h.do(t, http.MethodPost, "/v1/auth/refresh", `{"refresh_token":"`+refresh+`"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body=%s", rec.Code, rec.Body.String())
	}
	rr := decode[map[string]any](t, rec)
	if rr["token"] == nil || rr["refresh_token"] == nil {
		t.Fatal("rotasi harus mengembalikan access + refresh baru")
	}

	// refresh lama tidak boleh dipakai lagi
	reuse := h.do(t, http.MethodPost, "/v1/auth/refresh", `{"refresh_token":"`+refresh+`"}`, "")
	if reuse.Code != http.StatusUnauthorized {
		t.Fatalf("pakai ulang refresh lama status = %d, mau 401", reuse.Code)
	}
}

func TestRefreshInvalidToken(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/auth/refresh", `{"refresh_token":"token-palsu"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", rec.Code)
	}
}

func TestLogoutRevokesToken(t *testing.T) {
	h := newHarness(t)
	// token valid bisa akses /me
	if rec := h.do(t, http.MethodGet, "/v1/auth/me", "", h.token); rec.Code != http.StatusOK {
		t.Fatalf("sebelum logout status = %d", rec.Code)
	}
	// logout
	rec := h.do(t, http.MethodPost, "/v1/auth/logout", "", h.token)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout status = %d", rec.Code)
	}
	// token yang sudah dicabut harus 401
	after := h.do(t, http.MethodGet, "/v1/auth/me", "", h.token)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("setelah logout status = %d, mau 401", after.Code)
	}
}

func TestLogoutRequiresAuth(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/v1/auth/logout", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", rec.Code)
	}
}
