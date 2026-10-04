package scheduler

import (
	"context"
	"testing"
	"time"

	"peringatan-dini/internal/config"
	"peringatan-dini/internal/domain"
	"peringatan-dini/internal/engine"
	"peringatan-dini/internal/notifier"
	"peringatan-dini/internal/service"
	"peringatan-dini/internal/store"
)

func TestStartDisabledWhenFetchOff(t *testing.T) {
	cfg := config.Config{FetchEnabled: false}
	st := store.NewMemoryStore()
	svc := service.New(st, engine.New(cfg), map[string]notifier.Notifier{}, nil)
	s := New(cfg, st, svc, nil)
	s.Start() // harus langsung kembali tanpa error
	s.Stop()
}

func TestStartEnabledSchedulesJobs(t *testing.T) {
	cfg := config.Config{
		FetchEnabled: true,
		FetchCron:    "0 0 * * *",
		AlertCron:    "0 0 * * *",
	}
	st := store.NewMemoryStore()
	_ = st.SaveRegion(context.Background(), domain.Region{ID: "r1", Adm4: "x", Susceptibility: 20})
	svc := service.New(st, engine.New(cfg), map[string]notifier.Notifier{}, nil)
	s := New(cfg, st, svc, nil)
	s.Start()
	time.Sleep(20 * time.Millisecond)
	s.Stop()
}
