package ingestion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// buildBMKGResponse membuat respons BMKG tiruan dengan sejumlah entri curah hujan.
func buildBMKGResponse(adm4 string, entries []struct {
	offset time.Duration
	tp     float64
}) string {
	type cuacaEntry struct {
		Datetime string  `json:"datetime"`
		TP       float64 `json:"tp"`
	}
	var days [][]cuacaEntry
	for _, e := range entries {
		days = append(days, []cuacaEntry{
			{Datetime: time.Now().Add(e.offset).UTC().Format(time.RFC3339), TP: e.tp},
		})
	}
	payload := map[string]any{
		"lokasi": map[string]any{"adm4": adm4, "provinsi": "Jawa Tengah", "lat": -7.5, "lon": 109.4},
		"data":   []any{map[string]any{"cuaca": days}},
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func TestFetchRainfallSumsWindows(t *testing.T) {
	body := buildBMKGResponse("33.04.01.2001", []struct {
		offset time.Duration
		tp     float64
	}{
		{2 * time.Hour, 10},   // dalam 24 jam
		{20 * time.Hour, 5},   // dalam 24 jam
		{30 * time.Hour, 7},   // di luar 24, dalam 72
		{60 * time.Hour, 3},   // di luar 24, dalam 72
		{100 * time.Hour, 99}, // di luar 72, diabaikan
		{-1 * time.Hour, 100}, // masa lalu, diabaikan
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("adm4") != "33.04.01.2001" {
			t.Errorf("adm4 tidak diteruskan: %q", r.URL.Query().Get("adm4"))
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	rf, err := c.FetchRainfall(context.Background(), "33.04.01.2001", "reg-1")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if rf.Rain24hMM != 15 {
		t.Errorf("rain24 = %v, mau 15", rf.Rain24hMM)
	}
	if rf.Rain72hMM != 25 {
		t.Errorf("rain72 = %v, mau 25", rf.Rain72hMM)
	}
	if rf.Source != "bmkg" {
		t.Errorf("source = %q", rf.Source)
	}
	if rf.Freshness != "fresh" {
		t.Errorf("freshness = %q", rf.Freshness)
	}
}

func TestFetchRainfallHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if _, err := c.FetchRainfall(context.Background(), "x", "r"); err == nil {
		t.Fatal("status non-200 harus error")
	}
}

func TestFetchRainfallInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("bukan json"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if _, err := c.FetchRainfall(context.Background(), "x", "r"); err == nil {
		t.Fatal("JSON tidak valid harus error")
	}
}

func TestFetchLocation(t *testing.T) {
	body := buildBMKGResponse("33.04.01.2001", nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	loc, err := NewClient(srv.URL).FetchLocation(context.Background(), "33.04.01.2001")
	if err != nil {
		t.Fatalf("fetch location: %v", err)
	}
	if loc.Adm4 != "33.04.01.2001" || loc.Provinsi != "Jawa Tengah" {
		t.Fatalf("lokasi salah: %+v", loc)
	}
}
