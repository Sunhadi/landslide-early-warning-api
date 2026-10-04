// Package ingestion menarik data dari sumber eksternal (BMKG) dan menormalisasinya.
package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"peringatan-dini/internal/domain"
)

// Client adalah klien HTTP untuk API data terbuka BMKG.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient membuat klien BMKG.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// bmkgResponse memetakan struktur respons API prakiraan cuaca BMKG.
type bmkgResponse struct {
	Lokasi bmkgLokasi `json:"lokasi"`
	Data   []struct {
		Cuaca [][]bmkgEntry `json:"cuaca"`
	} `json:"data"`
}

type bmkgLokasi struct {
	Adm4      string  `json:"adm4"`
	Provinsi  string  `json:"provinsi"`
	Kotkab    string  `json:"kotkab"`
	Kecamatan string  `json:"kecamatan"`
	Desa      string  `json:"desa"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
}

type bmkgEntry struct {
	Datetime      string  `json:"datetime"` // RFC3339 UTC
	TP            float64 `json:"tp"`       // curah hujan (mm)
	LocalDatetime string  `json:"local_datetime"`
}

// FetchRainfall mengambil prakiraan cuaca BMKG untuk satu kode adm4 dan
// mengubahnya menjadi data curah hujan.
//
// Catatan (untuk belajar): endpoint prakiraan bersifat prospektif. Kita memakai
// jendela prakiraan ke depan sebagai pendekatan:
//   - Rain24hMM    = total tp 24 jam ke depan
//   - Rain72hMM    = total tp 72 jam ke depan
//   - Forecast24h  = total tp 24 jam ke depan (sama, disimpan eksplisit)
//
// Saat produksi, data observasi curah hujan aktual (AWS/ARG) dapat ditambahkan.
func (c *Client) FetchRainfall(ctx context.Context, adm4, regionID string) (domain.RainfallData, error) {
	url := fmt.Sprintf("%s?adm4=%s", c.baseURL, adm4)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return domain.RainfallData{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return domain.RainfallData{}, fmt.Errorf("gagal menghubungi BMKG: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return domain.RainfallData{}, fmt.Errorf("BMKG mengembalikan status %d", resp.StatusCode)
	}

	var parsed bmkgResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return domain.RainfallData{}, fmt.Errorf("gagal membaca respons BMKG: %w", err)
	}

	now := time.Now()
	limit24 := now.Add(24 * time.Hour)
	limit72 := now.Add(72 * time.Hour)

	var sum24, sum72 float64
	for _, block := range parsed.Data {
		for _, day := range block.Cuaca {
			for _, e := range day {
				t, err := time.Parse(time.RFC3339, e.Datetime)
				if err != nil {
					continue
				}
				if !t.After(now) || t.After(limit72) {
					continue
				}
				sum72 += e.TP
				if !t.After(limit24) {
					sum24 += e.TP
				}
			}
		}
	}

	freshness := "fresh"
	if len(parsed.Data) == 0 {
		freshness = "degraded"
	}

	return domain.RainfallData{
		RegionID:    regionID,
		Source:      "bmkg",
		ObservedAt:  now,
		Rain24hMM:   round1(sum24),
		Rain72hMM:   round1(sum72),
		Forecast24h: round1(sum24),
		FetchedAt:   now,
		Freshness:   freshness,
	}, nil
}

// FetchLocation mengambil metadata lokasi (koordinat & nama wilayah) dari BMKG.
// Berguna untuk seeding wilayah otomatis.
func (c *Client) FetchLocation(ctx context.Context, adm4 string) (bmkgLokasi, error) {
	url := fmt.Sprintf("%s?adm4=%s", c.baseURL, adm4)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return bmkgLokasi{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return bmkgLokasi{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return bmkgLokasi{}, fmt.Errorf("BMKG status %d", resp.StatusCode)
	}
	var parsed bmkgResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return bmkgLokasi{}, err
	}
	return parsed.Lokasi, nil
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
