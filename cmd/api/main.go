// Package main adalah entry point API Peringatan Dini Longsor (Jawa Tengah).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"peringatan-dini/internal/config"
	"peringatan-dini/internal/engine"
	"peringatan-dini/internal/httpapi"
	"peringatan-dini/internal/ingestion"
	"peringatan-dini/internal/notifier"
	"peringatan-dini/internal/scheduler"
	"peringatan-dini/internal/security"
	"peringatan-dini/internal/service"
	"peringatan-dini/internal/store"
)

func main() {
	// Muat .env bila ada (abaikan bila tidak).
	_ = godotenv.Load()
	cfg := config.Load()

	// Logging terstruktur (JSON) ke stdout.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Fail-fast: tolak konfigurasi tidak aman saat produksi.
	if err := cfg.Validate(); err != nil {
		slog.Error("konfigurasi tidak aman", "err", err)
		os.Exit(1)
	}
	if cfg.IsProduction() && len(cfg.CORSOrigins) == 1 && cfg.CORSOrigins[0] == "*" {
		slog.Warn("CORS_ORIGINS='*' di produksi dapat berisiko; batasi origin yang dikenal")
	}

	ctx := context.Background()

	// Pilih penyimpanan: PostgreSQL bila DATABASE_URL diisi, selain itu in-memory.
	var st store.Store
	if cfg.DatabaseURL != "" {
		pg, err := store.NewPostgres(ctx, cfg.DatabaseURL)
		if err != nil {
			slog.Error("gagal koneksi PostgreSQL", "err", err)
			os.Exit(1)
		}
		defer pg.Close()
		st = pg
		slog.Info("penyimpanan: PostgreSQL")
	} else {
		st = store.NewMemoryStore()
		slog.Info("penyimpanan: in-memory (data hilang saat restart)")
	}
	seedRegions(ctx, st)
	seedAdmin(ctx, cfg, st)

	cipher, err := security.NewCipher(cfg.PIIKey)
	if err != nil {
		slog.Error("gagal membuat cipher PII", "err", err)
		os.Exit(1)
	}
	tokens := security.NewTokenServiceWithRefresh(cfg.JWTSecret, cfg.JWTTTL, cfg.JWTRefreshTTL)
	eng := engine.New(cfg)
	notif := notifier.NewFromConfig(cfg)
	bmkg := ingestion.NewClient(cfg.BMKGBaseURL)
	svc := service.New(st, eng, notif, bmkg)

	srv := httpapi.NewServer(cfg, st, svc, tokens, cipher)

	// Scheduler job berkala.
	sch := scheduler.New(cfg, st, svc, bmkg)
	sch.Start()
	defer sch.Stop()

	// Jalankan evaluasi awal. Bila penarikan aktif, ambil data BMKG terbaru dulu.
	if cfg.FetchEnabled {
		if n := svc.SyncAll(ctx); n > 0 {
			slog.Info("sinkronisasi awal BMKG selesai", "wilayah", n)
		}
	}
	if _, err := svc.EvaluateAll(ctx); err != nil {
		slog.Warn("evaluasi awal gagal", "err", err)
	}

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown.
	go func() {
		slog.Info("server berjalan", "port", cfg.Port, "env", cfg.Env)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server gagal", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	slog.Info("mematikan server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}
