package domain

import "testing"

func TestLevelLabel(t *testing.T) {
	cases := []struct {
		level RiskLevel
		want  string
	}{
		{LevelNormal, "NORMAL"},
		{LevelWaspada, "WASPADA"},
		{LevelSiaga, "SIAGA"},
		{LevelAwas, "AWAS"},
		{RiskLevel(99), "NORMAL"}, // fallback
	}
	for _, c := range cases {
		if got := c.level.Label(); got != c.want {
			t.Errorf("Label(%d) = %q, mau %q", c.level, got, c.want)
		}
	}
}

func TestLevelRecommendation(t *testing.T) {
	for _, l := range []RiskLevel{LevelNormal, LevelWaspada, LevelSiaga, LevelAwas} {
		if l.Recommendation() == "" {
			t.Errorf("rekomendasi level %d tidak boleh kosong", l)
		}
	}
	// Awas harus menyebut evakuasi
	if r := LevelAwas.Recommendation(); !contains(r, "Evakuasi") {
		t.Errorf("rekomendasi AWAS = %q", r)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
