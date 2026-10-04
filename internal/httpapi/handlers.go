package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"peringatan-dini/internal/domain"
	"peringatan-dini/internal/security"
	"peringatan-dini/internal/store"
)

// handleHealth mengembalikan status kesehatan layanan.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
		"env":    s.Cfg.Env,
	})
}

// handleRoot memberi informasi singkat API.
func (s *Server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "API Peringatan Dini Longsor - Jawa Tengah",
		"version": "1.0.0",
		"docs":    "/v1/regions",
	})
}

// --- Regions ---

func (s *Server) handleListRegions(w http.ResponseWriter, r *http.Request) {
	regions, err := s.Store.ListRegions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengambil wilayah")
		return
	}
	writeList(w, regions, parsePage(r))
}

func (s *Server) handleGetRegion(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	region, err := s.Store.GetRegion(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Wilayah tidak ditemukan")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengambil wilayah")
		return
	}
	writeJSON(w, http.StatusOK, region)
}

// riskResponse adalah bentuk respons risiko wilayah.
type riskResponse struct {
	domain.RiskAssessment
	Recommendation string `json:"recommendation"`
	ValidUntil     string `json:"valid_until"`
}

func (s *Server) handleGetRisk(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	assess, err := s.Store.GetAssessment(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		// hitung langsung bila belum ada
		if a, e := s.Service.EvaluateRegion(r.Context(), id); e == nil {
			assess = a
		} else {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Wilayah tidak ditemukan")
			return
		}
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengambil risiko")
		return
	}
	writeJSON(w, http.StatusOK, riskResponse{
		RiskAssessment: assess,
		Recommendation: assess.Level.Recommendation(),
		ValidUntil:     time.Now().Add(6 * time.Hour).Format(time.RFC3339),
	})
}

// handleGetForecast mengembalikan prakiraan risiko sederhana (rule-based).
func (s *Server) handleGetForecast(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	region, err := s.Store.GetRegion(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Wilayah tidak ditemukan")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengambil wilayah")
		return
	}
	rf, _ := s.Store.GetRainfall(r.Context(), id)
	assess, _ := s.Store.GetAssessment(r.Context(), id)

	writeJSON(w, http.StatusOK, map[string]any{
		"region_id":       region.ID,
		"region_name":     region.Name,
		"current_level":   assess.LevelLabel,
		"forecast_24h_mm": rf.Forecast24h,
		"potential_level": assess.Level.Label(),
		"generated_at":    time.Now().Format(time.RFC3339),
		"note":            "Prakiraan berbasis ambang hujan BMKG & kerawanan wilayah (rule-based).",
	})
}

// --- Rainfall ---

func (s *Server) handleListRainfall(w http.ResponseWriter, r *http.Request) {
	regions, _ := s.Store.ListRegions(r.Context())
	out := make([]domain.RainfallData, 0, len(regions))
	for _, reg := range regions {
		if rf, err := s.Store.GetRainfall(r.Context(), reg.ID); err == nil {
			out = append(out, rf)
		}
	}
	writeList(w, out, parsePage(r))
}

// --- Alerts ---

// handleListAlerts mengembalikan daftar peringatan dengan filter & pagination.
func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("status") == "active"
	levelFilter := parseLevels(r.URL.Query().Get("level"))
	regionFilter := r.URL.Query().Get("region_id")

	alerts, err := s.Store.ListAlerts(r.Context(), activeOnly)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengambil peringatan")
		return
	}
	out := make([]domain.Alert, 0, len(alerts))
	for _, a := range alerts {
		if regionFilter != "" && a.RegionID != regionFilter {
			continue
		}
		if len(levelFilter) > 0 && !levelFilter[a.Level] {
			continue
		}
		out = append(out, a)
	}
	sortAlertsNewestFirst(out)
	writeList(w, out, parsePage(r))
}

func (s *Server) handleGetAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, err := s.Store.GetAlert(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Peringatan tidak ditemukan")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengambil peringatan")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

type alertRequest struct {
	RegionID    string   `json:"region_id"`
	Level       int      `json:"level"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	ValidHours  int      `json:"valid_hours"`
	Channels    []string `json:"channels"`
}

func (s *Server) handleCreateAlert(w http.ResponseWriter, r *http.Request) {
	var req alertRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.RegionID == "" || req.Level < 1 || req.Level > 4 {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "region_id wajib & level harus 1-4")
		return
	}
	if !validText(req.Title, 200) {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "title wajib diisi (maks 200 karakter)")
		return
	}
	if !validText(req.Description, 2000) {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "description wajib diisi (maks 2000 karakter)")
		return
	}
	for _, ch := range req.Channels {
		if !oneOf(ch, "whatsapp", "sms") {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "channel tidak dikenal: "+ch)
			return
		}
	}
	if req.ValidHours <= 0 {
		req.ValidHours = 12
	}
	if req.ValidHours > 168 {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "valid_hours maksimal 168 (7 hari)")
		return
	}
	if len(req.Channels) == 0 {
		req.Channels = []string{"whatsapp", "sms"}
	}
	actor := actorFromContext(r.Context())
	a, err := s.Service.PublishManual(r.Context(), req.RegionID, domain.RiskLevel(req.Level),
		req.Title, req.Description, time.Duration(req.ValidHours)*time.Hour, req.Channels)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Wilayah tidak ditemukan")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menerbitkan peringatan")
		return
	}
	s.Store.AppendAudit(r.Context(), actor, "alert.manual",
		"terbitkan alert "+a.LevelLabel+" untuk "+a.RegionName)
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handleUpdateAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, err := s.Store.GetAlert(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Peringatan tidak ditemukan")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengambil peringatan")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	switch req.Status {
	case "active", "expired", "cancelled":
		a.Status = req.Status
	default:
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "status harus active/expired/cancelled")
		return
	}
	if err := s.Store.SaveAlert(r.Context(), a); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menyimpan peringatan")
		return
	}
	s.Store.AppendAudit(r.Context(), actorFromContext(r.Context()), "alert.update",
		"ubah status alert "+a.ID+" menjadi "+a.Status)
	writeJSON(w, http.StatusOK, a)
}

// --- Reports ---

type reportRequest struct {
	Lat           float64 `json:"lat"`
	Lon           float64 `json:"lon"`
	Description   string  `json:"description"`
	SeverityGuess string  `json:"severity_guess"`
	ReporterName  string  `json:"reporter_name"`
	ReporterPhone string  `json:"reporter_phone"`
}

func (s *Server) handleCreateReport(w http.ResponseWriter, r *http.Request) {
	var req reportRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if !validText(req.Description, 2000) {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "description wajib diisi (maks 2000 karakter)")
		return
	}
	if !validLatLon(req.Lat, req.Lon) {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "lat/lon di luar rentang yang valid")
		return
	}
	if req.SeverityGuess != "" && !oneOf(req.SeverityGuess, "low", "medium", "high") {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "severity_guess harus low/medium/high")
		return
	}
	if len(req.ReporterName) > 120 {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "reporter_name terlalu panjang")
		return
	}
	// Enkripsi nomor telepon (UU PDP) sebelum disimpan.
	encPhone := ""
	if req.ReporterPhone != "" {
		if !validPhone(req.ReporterPhone) {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "format nomor telepon tidak valid")
			return
		}
		if enc, err := s.Cipher.Encrypt(normalizePhone(req.ReporterPhone)); err == nil {
			encPhone = enc
		}
	}
	rep := domain.Report{
		ID:            uuid.NewString(),
		Lat:           req.Lat,
		Lon:           req.Lon,
		Description:   req.Description,
		SeverityGuess: req.SeverityGuess,
		Status:        "pending",
		ReporterName:  req.ReporterName,
		ReporterPhone: encPhone,
		CreatedAt:     time.Now(),
	}
	if err := s.Store.SaveReport(r.Context(), rep); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menyimpan laporan")
		return
	}
	s.Store.AppendAudit(r.Context(), "public", "report.create", "laporan baru pada "+rep.ID)
	writeJSON(w, http.StatusCreated, rep)
}

func (s *Server) handleListReports(w http.ResponseWriter, r *http.Request) {
	reports, err := s.Store.ListReports(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengambil laporan")
		return
	}
	// jangan kirim nomor HP terenkripsi ke klien
	for i := range reports {
		reports[i].ReporterPhone = ""
	}
	sortReportsNewestFirst(reports)
	writeList(w, reports, parsePage(r))
}

func (s *Server) handleVerifyReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rep, err := s.Store.GetReport(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Laporan tidak ditemukan")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengambil laporan")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Status != "verified" && req.Status != "rejected" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "status harus verified/rejected")
		return
	}
	rep.Status = req.Status
	if err := s.Store.SaveReport(r.Context(), rep); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menyimpan laporan")
		return
	}
	s.Store.AppendAudit(r.Context(), actorFromContext(r.Context()), "report.verify",
		"verifikasi laporan "+rep.ID+" -> "+rep.Status)
	rep.ReporterPhone = ""
	writeJSON(w, http.StatusOK, rep)
}

// --- Subscriptions ---

type subscriptionRequest struct {
	RegionID string `json:"region_id"`
	Phone    string `json:"phone"`
	Channel  string `json:"channel"`
	Consent  bool   `json:"consent"`
}

func (s *Server) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	var req subscriptionRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Phone == "" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "phone wajib diisi")
		return
	}
	if !validPhone(req.Phone) {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "format nomor telepon tidak valid (contoh: +628123456789)")
		return
	}
	if !req.Consent {
		writeError(w, http.StatusUnprocessableEntity, "CONSENT_REQUIRED",
			"Persetujuan (consent) wajib sesuai UU PDP No. 27/2022")
		return
	}
	if req.Channel != "whatsapp" && req.Channel != "sms" {
		req.Channel = "whatsapp"
	}
	if _, err := s.Store.GetRegion(r.Context(), req.RegionID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Wilayah tidak ditemukan")
		return
	}
	phone := normalizePhone(req.Phone)
	enc, err := s.Cipher.Encrypt(phone)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memproses data")
		return
	}
	sub := domain.Subscription{
		ID:        uuid.NewString(),
		RegionID:  req.RegionID,
		Phone:     enc,
		PhoneMask: security.MaskPhone(phone),
		Channel:   req.Channel,
		Consent:   req.Consent,
		CreatedAt: time.Now(),
	}
	if err := s.Store.SaveSubscription(r.Context(), sub); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menyimpan langganan")
		return
	}
	s.Store.AppendAudit(r.Context(), "public", "subscription.create", "langganan "+sub.PhoneMask)
	writeJSON(w, http.StatusCreated, sub)
}

func (s *Server) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	subs, err := s.Store.ListSubscriptions(r.Context(), r.URL.Query().Get("region_id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengambil langganan")
		return
	}
	writeList(w, subs, parsePage(r))
}

func (s *Server) handleDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.Store.DeleteSubscription(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Langganan tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menghapus langganan")
		return
	}
	s.Store.AppendAudit(r.Context(), actorFromContext(r.Context()), "subscription.delete", "hapus langganan "+id)
	w.WriteHeader(http.StatusNoContent)
}

// --- Risk evaluation ---

// handleSync menarik data curah hujan terbaru dari BMKG, lalu mengevaluasi & memproses alert.
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	regionID := r.URL.Query().Get("region_id")
	actor := actorFromContext(r.Context())

	synced := 0
	if regionID != "" {
		if _, err := s.Service.SyncRegion(r.Context(), regionID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Wilayah tidak ditemukan")
				return
			}
			writeError(w, http.StatusBadGateway, "UPSTREAM_ERROR", "Gagal mengambil data BMKG")
			return
		}
		synced = 1
	} else {
		synced = s.Service.SyncAll(r.Context())
	}

	assessments, _ := s.Service.EvaluateAll(r.Context())
	actions, _ := s.Service.ProcessAlerts(r.Context())
	s.Store.AppendAudit(r.Context(), actor, "risk.sync",
		"sync BMKG "+strconv.Itoa(synced)+" wilayah")
	writeJSON(w, http.StatusOK, map[string]any{
		"source":    "bmkg",
		"synced":    synced,
		"evaluated": len(assessments),
		"actions":   actions,
		"data":      assessments,
	})
}

func (s *Server) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	assessments, err := s.Service.EvaluateAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengevaluasi risiko")
		return
	}
	actions, _ := s.Service.ProcessAlerts(r.Context())
	s.Store.AppendAudit(r.Context(), actorFromContext(r.Context()), "risk.evaluate",
		"evaluasi manual "+strconv.Itoa(len(assessments))+" wilayah")
	writeJSON(w, http.StatusOK, map[string]any{
		"evaluated": len(assessments),
		"actions":   actions,
		"data":      assessments,
	})
}

// --- Audit ---

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	p := parsePage(r)
	// ambil cukup banyak untuk menutupi halaman yang diminta
	need := p.Page * p.PerPage
	entries, err := s.Store.ListAudit(r.Context(), need)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengambil audit")
		return
	}
	writeList(w, entries, p)
}

// --- Auth ---

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeBody(w, r, &req) {
		return
	}
	ip := clientIP(r, s.Cfg.TrustProxy)

	// Anti brute-force: tolak lebih awal bila masih terkunci.
	if s.loginGuard.locked(ip, req.Username) {
		w.Header().Set("Retry-After", strconv.Itoa(int(s.Cfg.LoginLockout.Seconds())))
		writeError(w, http.StatusTooManyRequests, "ACCOUNT_LOCKED",
			"Terlalu banyak percobaan gagal. Coba lagi nanti.")
		return
	}

	user, err := s.Store.GetUserByUsername(r.Context(), req.Username)
	if err != nil || !security.CheckPassword(user.PasswordHash, req.Password) {
		s.loginGuard.fail(ip, req.Username)
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Username atau password salah")
		return
	}
	token, err := s.Tokens.Issue(user.ID, user.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal membuat token")
		return
	}
	refresh, err := s.Tokens.IssueRefresh(user.ID, user.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal membuat refresh token")
		return
	}
	s.loginGuard.success(ip, req.Username)
	s.Store.AppendAudit(r.Context(), user.Username, "auth.login", "login berhasil")
	writeJSON(w, http.StatusOK, map[string]any{
		"token":         token,
		"refresh_token": refresh,
		"token_type":    "Bearer",
		"expires_in":    int(s.Cfg.JWTTTL.Seconds()),
		"role":          user.Role,
	})
}

// handleRefresh menukar refresh token dengan access + refresh token baru (rotasi).
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "refresh_token wajib diisi")
		return
	}
	access, refresh, claims, err := s.Tokens.RotateRefresh(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Refresh token tidak valid atau sudah dipakai")
		return
	}
	s.Store.AppendAudit(r.Context(), claims.Subject, "auth.refresh", "token diperbarui")
	writeJSON(w, http.StatusOK, map[string]any{
		"token":         access,
		"refresh_token": refresh,
		"token_type":    "Bearer",
		"expires_in":    int(s.Cfg.JWTTTL.Seconds()),
		"role":          claims.Role,
	})
}

// handleLogout mencabut access token yang sedang dipakai.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	token := strings.TrimPrefix(header, "Bearer ")
	if err := s.Tokens.Revoke(token); err != nil {
		// token mungkin sudah tidak valid; tetap anggap berhasil logout
		slog.Warn("logout: gagal mencabut token", "err", err)
	}
	s.Store.AppendAudit(r.Context(), actorFromContext(r.Context()), "auth.logout", "logout")
	writeJSON(w, http.StatusOK, map[string]any{"status": "logged_out"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": r.Context().Value(ctxUserID),
		"role":    r.Context().Value(ctxRole),
	})
}

// parseLevels mengubah "3,4" menjadi set level.
func parseLevels(raw string) map[domain.RiskLevel]bool {
	out := map[domain.RiskLevel]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil && n >= 1 && n <= 4 {
			out[domain.RiskLevel(n)] = true
		}
	}
	return out
}
