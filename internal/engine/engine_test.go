package engine

import (
	"testing"

	"peringatan-dini/internal/config"
	"peringatan-dini/internal/domain"
)

func testConfig() config.Config {
	return config.Config{
		Rain24Waspada: 50, Rain24Siaga: 100, Rain24Awas: 150,
		Rain72Waspada: 80, Rain72Siaga: 150, Rain72Awas: 200,
	}
}

func TestEvaluateNormal(t *testing.T) {
	e := New(testConfig())
	a := e.Evaluate(Input{Rain24hMM: 2, Rain72hMM: 5, Forecast24hMM: 2, Susceptibility: 20, SlopeDeg: 5, Freshness: "fresh"})
	if a.Level != domain.LevelNormal {
		t.Fatalf("mau NORMAL, dapat %s", a.LevelLabel)
	}
}

func TestEvaluateAwasJump(t *testing.T) {
	e := New(testConfig())
	// Hujan ekstrem + wilayah sangat rawan -> paksa AWAS.
	a := e.Evaluate(Input{Rain24hMM: 160, Rain72hMM: 210, Forecast24hMM: 50, Susceptibility: 88, SlopeDeg: 30, Freshness: "fresh"})
	if a.Level != domain.LevelAwas {
		t.Fatalf("mau AWAS, dapat %s (score %.1f)", a.LevelLabel, a.Score)
	}
	if len(a.Triggers) == 0 {
		t.Fatal("harus ada trigger")
	}
}

func TestEvaluateThresholdEscalation(t *testing.T) {
	e := New(testConfig())
	// Hujan siaga pada wilayah cukup rawan harus naik ke SIAGA.
	a := e.Evaluate(Input{Rain24hMM: 110, Rain72hMM: 120, Forecast24hMM: 30, Susceptibility: 70, SlopeDeg: 25, Freshness: "fresh"})
	if a.Level < domain.LevelSiaga {
		t.Fatalf("mau minimal SIAGA, dapat %s (score %.1f)", a.LevelLabel, a.Score)
	}
}

func TestScoreRange(t *testing.T) {
	e := New(testConfig())
	a := e.Evaluate(Input{Rain24hMM: 1000, Rain72hMM: 1000, Forecast24hMM: 1000, Susceptibility: 100, SlopeDeg: 90, Freshness: "fresh"})
	if a.Score < 0 || a.Score > 100 {
		t.Fatalf("skor di luar rentang 0..100: %.1f", a.Score)
	}
}
