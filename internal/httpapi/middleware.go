// Package httpapi berisi router, middleware, dan handler HTTP.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"peringatan-dini/internal/security"
)

type ctxKey string

const (
	ctxUserID ctxKey = "user_id"
	ctxRole   ctxKey = "role"
)

// writeJSON menulis respons JSON.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// writeError menulis respons error dengan format standar.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": msg,
		},
	})
}

// --- Middleware ---

// requestID memberi tiap request sebuah ID unik sederhana (berbasis waktu).
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", time.Now().UTC().Format("20060102150405.000000000"))
		next.ServeHTTP(w, r)
	})
}

// recoverPanic menangkap panic agar server tidak mati.
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic", "err", rec, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// maxBody membatasi ukuran body request untuk mencegah penyalahgunaan memori.
func maxBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limit > 0 && r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// securityHeaders menambahkan header keamanan dasar.
// hsts diaktifkan saat produksi (di belakang TLS).
func securityHeaders(hsts bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Content-Security-Policy", "default-src 'self'")
			h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// cors menangani Cross-Origin Resource Sharing.
// origins berisi "*" untuk semua, atau daftar origin spesifik.
func cors(origins []string) func(http.Handler) http.Handler {
	allowAll := len(origins) == 0
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowAll || allowed[origin]) {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// accessLog mencatat setiap request.
func accessLog(trustProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(sw, r)
			slog.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"durasi_ms", time.Since(start).Milliseconds(),
				"ip", clientIP(r, trustProxy),
			)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// --- Rate limiter (token bucket per IP) ---

type rateLimiter struct {
	mu         sync.Mutex
	visitors   map[string]*bucket
	rate       float64 // token per menit
	burst      float64 // kapasitas maksimum
	trustProxy bool
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

func newRateLimiter(perMinute int) *rateLimiter {
	return newRateLimiterBurst(perMinute, perMinute)
}

func newRateLimiterBurst(perMinute, burst int) *rateLimiter {
	if burst < perMinute {
		burst = perMinute
	}
	rl := &rateLimiter{
		visitors: make(map[string]*bucket),
		rate:     float64(perMinute),
		burst:    float64(burst),
	}
	go rl.cleanup()
	return rl
}

func (rl *rateLimiter) cleanup() {
	for range time.Tick(5 * time.Minute) {
		rl.mu.Lock()
		for ip, b := range rl.visitors {
			if time.Since(b.lastSeen) > 10*time.Minute {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.visitors[ip]
	if !ok {
		rl.visitors[ip] = &bucket{tokens: rl.burst - 1, lastSeen: now}
		return true
	}
	// isi ulang token sesuai waktu berlalu
	elapsed := now.Sub(b.lastSeen).Minutes()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.lastSeen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// middleware membatasi request per IP.
func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r, rl.trustProxy)) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Terlalu banyak permintaan, coba lagi nanti")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loginGuard membatasi percobaan login gagal per IP + username (anti brute-force).
type loginGuard struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
	max      int
	lockout  time.Duration
}

type loginAttempt struct {
	count int
	until time.Time
}

func newLoginGuard(max int, lockout time.Duration) *loginGuard {
	if max <= 0 {
		max = 5
	}
	if lockout <= 0 {
		lockout = 15 * time.Minute
	}
	g := &loginGuard{attempts: make(map[string]*loginAttempt), max: max, lockout: lockout}
	go g.cleanup()
	return g
}

func (g *loginGuard) key(ip, username string) string {
	return ip + "|" + strings.ToLower(strings.TrimSpace(username))
}

func (g *loginGuard) cleanup() {
	for range time.Tick(5 * time.Minute) {
		g.mu.Lock()
		for k, a := range g.attempts {
			if time.Now().After(a.until) && time.Since(a.until) > 10*time.Minute {
				delete(g.attempts, k)
			}
		}
		g.mu.Unlock()
	}
}

// locked mengembalikan true bila masih terkunci.
func (g *loginGuard) locked(ip, username string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	a, ok := g.attempts[g.key(ip, username)]
	if !ok {
		return false
	}
	return time.Now().Before(a.until)
}

// fail mencatat kegagalan; mengunci bila melewati batas.
func (g *loginGuard) fail(ip, username string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	k := g.key(ip, username)
	a, ok := g.attempts[k]
	if !ok {
		a = &loginAttempt{}
		g.attempts[k] = a
	}
	a.count++
	if a.count >= g.max {
		a.until = time.Now().Add(g.lockout)
		a.count = 0
	}
}

// success menghapus catatan kegagalan setelah login berhasil.
func (g *loginGuard) success(ip, username string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.attempts, g.key(ip, username))
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// ambil hop pertama
			if i := strings.IndexByte(xff, ','); i >= 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// auth memverifikasi JWT dan menaruh identitas di context.
func auth(tokens *security.TokenService, required bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" || !strings.HasPrefix(header, "Bearer ") {
				if required {
					writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ditemukan")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			token := strings.TrimPrefix(header, "Bearer ")
			claims, err := tokens.Verify(token)
			if err != nil {
				if required {
					writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak valid")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), ctxUserID, claims.Subject)
			ctx = context.WithValue(ctx, ctxRole, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// requireRole membatasi akses berdasarkan peran.
func requireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, _ := r.Context().Value(ctxRole).(string)
			if !allowed[role] {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak berwenang mengakses sumber daya ini")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// actorFromContext mengambil identitas pemanggil untuk audit log.
func actorFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxUserID).(string); ok && v != "" {
		return v
	}
	return "anonymous"
}
