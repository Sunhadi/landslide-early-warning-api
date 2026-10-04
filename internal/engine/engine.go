// Package engine berisi logika rule-based untuk menghitung risiko longsor.
// Tidak memakai machine learning. Fokus: curah hujan BMKG + kerawanan statis wilayah.
package engine

import (
	"math"
	"time"

	"peringatan-dini/internal/config"
	"peringatan-dini/internal/domain"
)

// Input adalah faktor masukan untuk perhitungan risiko.
type Input struct {
	Rain24hMM      float64
	Rain72hMM      float64
	Forecast24hMM  float64
	Susceptibility float64 // 0..100
	SlopeDeg       float64
	Freshness      string
}

// Engine menghitung skor & level risiko berdasarkan konfigurasi ambang.
type Engine struct {
	cfg config.Config
}

// New membuat Engine baru.
func New(cfg config.Config) *Engine {
	return &Engine{cfg: cfg}
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}

// Evaluate menghitung skor risiko (0..100), level, faktor, dan trigger.
//
// Bobot:
//
//	0.35 rain24 + 0.25 rain72 + 0.15 forecast24 + 0.15 susceptibility + 0.10 slope
func (e *Engine) Evaluate(in Input) domain.RiskAssessment {
	nRain24 := clamp(in.Rain24hMM/150.0, 0, 1)
	nRain72 := clamp(in.Rain72hMM/200.0, 0, 1)
	nFcst := clamp(in.Forecast24hMM/100.0, 0, 1)
	nSus := clamp(in.Susceptibility/100.0, 0, 1)
	nSlope := clamp(in.SlopeDeg/45.0, 0, 1)

	score := (0.35*nRain24 + 0.25*nRain72 + 0.15*nFcst + 0.15*nSus + 0.10*nSlope) * 100
	score = math.Round(score*10) / 10

	level := levelFromScore(score)
	triggers := []string{}

	// Rule eskalasi berdasarkan ambang hujan + kerawanan wilayah.
	if in.Rain24hMM >= e.cfg.Rain24Waspada {
		triggers = append(triggers, "hujan_24jam_melebihi_ambang_waspada")
	}
	if in.Rain72hMM >= e.cfg.Rain72Siaga {
		triggers = append(triggers, "hujan_72jam_melebihi_ambang_siaga")
	}
	// Rule paksa Awas.
	if (in.Rain24hMM >= e.cfg.Rain24Awas || in.Rain72hMM >= e.cfg.Rain72Awas) && in.Susceptibility >= 70 {
		level = domain.LevelAwas
		triggers = append(triggers, "paksa_awas_hujan_ekstrem_wilayah_ravan")
	} else {
		// Naik minimal ke level berbasis ambang hujan bila wilayah cukup rawan.
		threshold := levelFromThresholds(in, e.cfg)
		if in.Susceptibility >= 60 && threshold > level {
			level = threshold
		}
	}

	factors := map[string]float64{
		"rain_24h_mm":     in.Rain24hMM,
		"rain_72h_mm":     in.Rain72hMM,
		"forecast_24h_mm": in.Forecast24hMM,
		"susceptibility":  in.Susceptibility,
		"slope_deg":       in.SlopeDeg,
	}

	return domain.RiskAssessment{
		Score:       score,
		Level:       level,
		LevelLabel:  level.Label(),
		Factors:     factors,
		Freshness:   in.Freshness,
		Triggers:    triggers,
		EvaluatedAt: time.Now(),
	}
}

// levelFromScore memetakan skor ke level.
func levelFromScore(score float64) domain.RiskLevel {
	switch {
	case score >= 75:
		return domain.LevelAwas
	case score >= 50:
		return domain.LevelSiaga
	case score >= 25:
		return domain.LevelWaspada
	default:
		return domain.LevelNormal
	}
}

// levelFromThresholds menentukan level dari ambang curah hujan saja.
func levelFromThresholds(in Input, cfg config.Config) domain.RiskLevel {
	if in.Rain24hMM >= cfg.Rain24Awas || in.Rain72hMM >= cfg.Rain72Awas {
		return domain.LevelAwas
	}
	if in.Rain24hMM >= cfg.Rain24Siaga || in.Rain72hMM >= cfg.Rain72Siaga {
		return domain.LevelSiaga
	}
	if in.Rain24hMM >= cfg.Rain24Waspada || in.Rain72hMM >= cfg.Rain72Waspada {
		return domain.LevelWaspada
	}
	return domain.LevelNormal
}
