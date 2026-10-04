package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"peringatan-dini/internal/config"
	"peringatan-dini/internal/security"
	"peringatan-dini/internal/service"
	"peringatan-dini/internal/store"
)

// Server membungkus dependensi HTTP.
type Server struct {
	Cfg        config.Config
	Store      store.Store
	Service    *service.Service
	Tokens     *security.TokenService
	Cipher     *security.Cipher
	loginGuard *loginGuard
}

// NewServer membuat Server baru.
func NewServer(cfg config.Config, st store.Store, svc *service.Service, tokens *security.TokenService, cipher *security.Cipher) *Server {
	return &Server{
		Cfg:        cfg,
		Store:      st,
		Service:    svc,
		Tokens:     tokens,
		Cipher:     cipher,
		loginGuard: newLoginGuard(cfg.LoginMaxAttempts, cfg.LoginLockout),
	}
}

// Router membangun router dengan seluruh route & middleware.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Middleware global
	r.Use(recoverPanic)
	r.Use(requestID)
	r.Use(securityHeaders(s.Cfg.IsProduction()))
	r.Use(cors(s.Cfg.CORSOrigins))
	r.Use(maxBody(s.Cfg.MaxBodyBytes))
	r.Use(accessLog(s.Cfg.TrustProxy))

	pubLimiter := newRateLimiterBurst(120, 240)
	pubLimiter.trustProxy = s.Cfg.TrustProxy
	r.Use(pubLimiter.middleware)

	// Health (publik, tanpa auth)
	r.Get("/health", s.handleHealth)
	r.Get("/", s.handleRoot)

	// Dokumentasi API
	r.Get("/openapi.yaml", s.handleOpenAPISpec)
	r.Get("/docs", s.handleSwaggerUI)

	r.Route("/v1", func(r chi.Router) {
		// --- Publik ---
		r.Get("/regions", s.handleListRegions)
		r.Get("/regions/{id}", s.handleGetRegion)
		r.Get("/regions/{id}/risk", s.handleGetRisk)
		r.Get("/regions/{id}/forecast", s.handleGetForecast)
		r.Get("/alerts", s.handleListAlerts)
		r.Get("/alerts/{id}", s.handleGetAlert)
		r.Get("/rainfall", s.handleListRainfall)
		r.Post("/reports", s.handleCreateReport)
		r.Post("/subscriptions", s.handleCreateSubscription)

		// --- Auth (login dibatasi rate-limit lebih ketat + lockout) ---
		loginLimiter := newRateLimiterBurst(20, 40)
		loginLimiter.trustProxy = s.Cfg.TrustProxy
		r.With(loginLimiter.middleware).Post("/auth/login", s.handleLogin)
		r.Post("/auth/refresh", s.handleRefresh)
		r.With(s.withAuth(true)).Post("/auth/logout", s.handleLogout)
		r.With(s.withAuth(true)).Get("/auth/me", s.handleMe)

		// --- Butuh login (petugas/admin) ---
		r.Group(func(r chi.Router) {
			r.Use(s.withAuth(true))

			r.Post("/alerts", s.withRole(s.handleCreateAlert, "officer", "admin"))
			r.Patch("/alerts/{id}", s.withRole(s.handleUpdateAlert, "officer", "admin"))

			r.Get("/reports", s.handleListReports)
			r.Patch("/reports/{id}/verify", s.withRole(s.handleVerifyReport, "officer", "admin"))

			r.Get("/subscriptions", s.handleListSubscriptions)
			r.Delete("/subscriptions/{id}", s.handleDeleteSubscription)

			r.Post("/risk/evaluate", s.withRole(s.handleEvaluate, "officer", "admin"))
			r.Post("/risk/sync", s.withRole(s.handleSync, "officer", "admin"))
			r.Get("/audit", s.withRole(s.handleAudit, "admin"))
		})
	})

	return r
}

// withAuth membungkus middleware auth agar bisa dipakai sebagai method.
func (s *Server) withAuth(required bool) func(http.Handler) http.Handler {
	return auth(s.Tokens, required)
}

// withRole membungkus requireRole untuk sebuah handler.
func (s *Server) withRole(next http.HandlerFunc, roles ...string) http.HandlerFunc {
	return requireRole(roles...)(next).ServeHTTP
}
