// Package service berisi orkestrasi bisnis: evaluasi risiko, penerbitan alert,
// dan distribusi notifikasi. Dipakai oleh HTTP handler maupun scheduler.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"peringatan-dini/internal/domain"
	"peringatan-dini/internal/engine"
	"peringatan-dini/internal/notifier"
	"peringatan-dini/internal/store"
)

// Service mengoordinasikan store, engine, dan notifier.
type Service struct {
	Store    store.Store
	Engine   *engine.Engine
	Notifier map[string]notifier.Notifier
	Fetcher  Fetcher
}

// Fetcher abstraksi penarik data curah hujan (mis. BMKG).
type Fetcher interface {
	FetchRainfall(ctx context.Context, adm4, regionID string) (domain.RainfallData, error)
}

// New membuat Service.
func New(st store.Store, eng *engine.Engine, n map[string]notifier.Notifier, f Fetcher) *Service {
	return &Service{Store: st, Engine: eng, Notifier: n, Fetcher: f}
}

// SyncRegion menarik data hujan terbaru untuk satu wilayah.
func (s *Service) SyncRegion(ctx context.Context, regionID string) (domain.RainfallData, error) {
	if s.Fetcher == nil {
		return domain.RainfallData{}, fmt.Errorf("fetcher tidak tersedia")
	}
	region, err := s.Store.GetRegion(ctx, regionID)
	if err != nil {
		return domain.RainfallData{}, err
	}
	rf, err := s.Fetcher.FetchRainfall(ctx, region.Adm4, region.ID)
	if err != nil {
		return domain.RainfallData{}, err
	}
	if err := s.Store.SaveRainfall(ctx, rf); err != nil {
		return domain.RainfallData{}, err
	}
	return rf, nil
}

// SyncAll menarik data hujan untuk semua wilayah. Mengembalikan jumlah sukses.
func (s *Service) SyncAll(ctx context.Context) int {
	regions, err := s.Store.ListRegions(ctx)
	if err != nil {
		return 0
	}
	ok := 0
	for _, r := range regions {
		if _, err := s.SyncRegion(ctx, r.ID); err != nil {
			slog.Warn("sync gagal", "region", r.Name, "err", err)
			continue
		}
		ok++
	}
	return ok
}

// EvaluateRegion menghitung risiko satu wilayah dan menyimpannya.
func (s *Service) EvaluateRegion(ctx context.Context, regionID string) (domain.RiskAssessment, error) {
	region, err := s.Store.GetRegion(ctx, regionID)
	if err != nil {
		return domain.RiskAssessment{}, err
	}

	in := engine.Input{
		Susceptibility: region.Susceptibility,
		SlopeDeg:       region.SlopeDeg,
		Freshness:      "no-data",
	}
	if rf, err := s.Store.GetRainfall(ctx, regionID); err == nil {
		in.Rain24hMM = rf.Rain24hMM
		in.Rain72hMM = rf.Rain72hMM
		in.Forecast24hMM = rf.Forecast24h
		in.Freshness = rf.Freshness
	}

	assess := s.Engine.Evaluate(in)
	assess.RegionID = region.ID
	assess.RegionName = region.Name

	if err := s.Store.SaveAssessment(ctx, assess); err != nil {
		return domain.RiskAssessment{}, err
	}
	return assess, nil
}

// EvaluateAll mengevaluasi seluruh wilayah.
func (s *Service) EvaluateAll(ctx context.Context) ([]domain.RiskAssessment, error) {
	regions, err := s.Store.ListRegions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.RiskAssessment, 0, len(regions))
	for _, r := range regions {
		a, err := s.EvaluateRegion(ctx, r.ID)
		if err != nil {
			slog.Warn("gagal evaluasi wilayah", "region", r.ID, "err", err)
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// ProcessAlerts memeriksa hasil evaluasi dan menerbitkan/mengakhiri alert
// sesuai aturan (rule-based). Mengembalikan daftar aksi yang dilakukan.
func (s *Service) ProcessAlerts(ctx context.Context) ([]string, error) {
	regions, err := s.Store.ListRegions(ctx)
	if err != nil {
		return nil, err
	}
	actions := []string{}

	for _, r := range regions {
		assess, err := s.Store.GetAssessment(ctx, r.ID)
		if err != nil {
			continue
		}
		active, hasActive, _ := s.Store.ActiveAlertForRegion(ctx, r.ID)

		if assess.Level >= domain.LevelSiaga {
			// Terbitkan atau perbarui alert bila belum sesuai.
			if !hasActive || active.Level != assess.Level {
				a := s.buildAlert(assess)
				if err := s.Store.SaveAlert(ctx, a); err != nil {
					slog.Warn("gagal simpan alert", "region", r.ID, "err", err)
					continue
				}
				actions = append(actions, fmt.Sprintf("terbitkan %s untuk %s", assess.LevelLabel, r.Name))
				s.dispatch(ctx, a)
			}
		} else if hasActive {
			// Level turun di bawah Siaga -> akhiri alert.
			active.Status = "expired"
			_ = s.Store.SaveAlert(ctx, active)
			actions = append(actions, fmt.Sprintf("akhiri alert untuk %s", r.Name))
		}
	}
	return actions, nil
}

// PublishManual menerbitkan alert manual oleh petugas.
func (s *Service) PublishManual(ctx context.Context, regionID string, level domain.RiskLevel, title, desc string, validFor time.Duration, channels []string) (domain.Alert, error) {
	region, err := s.Store.GetRegion(ctx, regionID)
	if err != nil {
		return domain.Alert{}, err
	}
	a := domain.Alert{
		ID:          uuid.NewString(),
		RegionID:    region.ID,
		RegionName:  region.Name,
		Level:       level,
		LevelLabel:  level.Label(),
		Title:       title,
		Description: desc,
		IssuedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(validFor),
		Source:      "manual",
		Status:      "active",
		Channels:    channels,
	}
	if err := s.Store.SaveAlert(ctx, a); err != nil {
		return domain.Alert{}, err
	}
	s.dispatch(ctx, a)
	return a, nil
}

func (s *Service) buildAlert(a domain.RiskAssessment) domain.Alert {
	region, _ := s.Store.GetRegion(context.Background(), a.RegionID)
	title := fmt.Sprintf("Peringatan %s Longsor - %s", a.LevelLabel, a.RegionName)
	desc := fmt.Sprintf(
		"Curah hujan %d jam: %.1f mm; %d jam: %.1f mm. Skor risiko %.1f. %s",
		24, a.Factors["rain_24h_mm"], 72, a.Factors["rain_72h_mm"], a.Score, a.Level.Recommendation(),
	)
	a2 := domain.Alert{
		ID:          uuid.NewString(),
		RegionID:    a.RegionID,
		RegionName:  a.RegionName,
		Level:       a.Level,
		LevelLabel:  a.LevelLabel,
		Title:       title,
		Description: desc,
		IssuedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(12 * time.Hour),
		Source:      "automatic",
		Status:      "active",
		Channels:    []string{"whatsapp", "sms"},
	}
	_ = region
	return a2
}

// dispatch mengirim notifikasi alert ke semua subscriber wilayah (WA & SMS).
func (s *Service) dispatch(ctx context.Context, a domain.Alert) {
	subs, err := s.Store.ListSubscriptions(ctx, a.RegionID)
	if err != nil {
		slog.Warn("gagal ambil subscriber", "err", err)
		return
	}
	body := fmt.Sprintf("[%s] %s\n%s", a.LevelLabel, a.Title, a.Description)
	for _, sub := range subs {
		n, ok := s.Notifier[sub.Channel]
		if !ok {
			continue
		}
		if err := n.Send(ctx, notifier.Message{To: sub.PhoneMask, Body: body}); err != nil {
			slog.Warn("gagal kirim notifikasi", "channel", sub.Channel, "err", err)
		}
	}
	s.Store.AppendAudit(ctx, "system", "alert.dispatch",
		fmt.Sprintf("alert %s level %s ke %d subscriber", a.RegionID, a.LevelLabel, len(subs)))
}
