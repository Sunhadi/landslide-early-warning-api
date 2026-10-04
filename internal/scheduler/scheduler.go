// Package scheduler menjalankan job berkala: penarikan data BMKG & evaluasi risiko.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"peringatan-dini/internal/config"
	"peringatan-dini/internal/ingestion"
	"peringatan-dini/internal/service"
	"peringatan-dini/internal/store"
)

// Scheduler membungkus cron dan dependensinya.
type Scheduler struct {
	cfg  config.Config
	st   store.Store
	svc  *service.Service
	bmkg *ingestion.Client
	cron *cron.Cron
}

// New membuat Scheduler.
func New(cfg config.Config, st store.Store, svc *service.Service, bmkg *ingestion.Client) *Scheduler {
	return &Scheduler{
		cfg:  cfg,
		st:   st,
		svc:  svc,
		bmkg: bmkg,
		cron: cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger))),
	}
}

// Start menjadwalkan job dan memulai cron.
func (s *Scheduler) Start() {
	if !s.cfg.FetchEnabled {
		slog.Info("scheduler nonaktif (FETCH_ENABLED=false)")
		return
	}

	_, err := s.cron.AddFunc(s.cfg.FetchCron, func() { s.runFetch() })
	if err != nil {
		slog.Error("cron fetch tidak valid", "err", err)
	}
	_, err = s.cron.AddFunc(s.cfg.AlertCron, func() { s.runEvaluate() })
	if err != nil {
		slog.Error("cron alert tidak valid", "err", err)
	}

	s.cron.Start()
	slog.Info("scheduler dimulai", "fetch", s.cfg.FetchCron, "alert", s.cfg.AlertCron)
}

// Stop menghentikan cron.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

func (s *Scheduler) runFetch() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	synced := s.svc.SyncAll(ctx)
	slog.Info("fetch BMKG selesai", "wilayah_berhasil", synced)
	s.runEvaluate()
}

func (s *Scheduler) runEvaluate() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	if _, err := s.svc.EvaluateAll(ctx); err != nil {
		slog.Warn("evaluasi gagal", "err", err)
		return
	}
	actions, err := s.svc.ProcessAlerts(ctx)
	if err != nil {
		slog.Warn("proses alert gagal", "err", err)
		return
	}
	if len(actions) > 0 {
		slog.Info("alert actions", "actions", actions)
	}
}
