package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"peringatan-dini/internal/domain"
)

func TestPaginate(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6, 7}

	slice, total := paginate(items, pageParams{Page: 1, PerPage: 3})
	if total != 7 || len(slice) != 3 || slice[0] != 1 {
		t.Fatalf("halaman 1 salah: %v total=%d", slice, total)
	}
	slice, _ = paginate(items, pageParams{Page: 3, PerPage: 3})
	if len(slice) != 1 || slice[0] != 7 {
		t.Fatalf("halaman 3 salah: %v", slice)
	}
	slice, _ = paginate(items, pageParams{Page: 4, PerPage: 3})
	if len(slice) != 0 {
		t.Fatalf("halaman di luar rentang harus kosong: %v", slice)
	}
}

func TestPages(t *testing.T) {
	cases := []struct{ total, perPage, want int }{
		{0, 20, 0},
		{1, 20, 1},
		{20, 20, 1},
		{21, 20, 2},
		{100, 10, 10},
	}
	for _, c := range cases {
		if got := pages(c.total, c.perPage); got != c.want {
			t.Errorf("pages(%d,%d) = %d, mau %d", c.total, c.perPage, got, c.want)
		}
	}
}

func TestParsePageDefaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/alerts", nil)
	p := parsePage(req)
	if p.Page != 1 || p.PerPage != defaultPerPage {
		t.Fatalf("default salah: %+v", p)
	}
}

func TestParsePageCapsPerPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/alerts?per_page=9999&page=2", nil)
	p := parsePage(req)
	if p.PerPage != maxPerPage {
		t.Errorf("per_page harus dibatasi %d, dapat %d", maxPerPage, p.PerPage)
	}
	if p.Page != 2 {
		t.Errorf("page = %d", p.Page)
	}
}

func TestParsePageIgnoresInvalid(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/alerts?page=-5&per_page=abc", nil)
	p := parsePage(req)
	if p.Page != 1 || p.PerPage != defaultPerPage {
		t.Fatalf("nilai tidak valid harus default: %+v", p)
	}
}

func TestListRegionsPagination(t *testing.T) {
	h := newHarnessWithConfig(t, nil)
	// tambah wilayah agar ada > 1
	for i := 0; i < 5; i++ {
		_ = h.store.SaveRegion(context.Background(), domain.Region{ID: "r" + string(rune('a'+i)), Name: "W"})
	}
	rec := h.do(t, http.MethodGet, "/v1/regions?page=1&per_page=2", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := decode[struct {
		Data []domain.Region `json:"data"`
		Meta struct {
			Page    int `json:"page"`
			PerPage int `json:"per_page"`
			Total   int `json:"total"`
			Pages   int `json:"pages"`
		} `json:"meta"`
	}](t, rec)
	if body.Meta.Total != 6 || body.Meta.Pages != 3 {
		t.Fatalf("meta salah: %+v", body.Meta)
	}
	if len(body.Data) != 2 {
		t.Fatalf("data halaman = %d, mau 2", len(body.Data))
	}

	// halaman 2
	rec2 := h.do(t, http.MethodGet, "/v1/regions?page=2&per_page=2", "", "")
	body2 := decode[struct {
		Data []domain.Region `json:"data"`
	}](t, rec2)
	if len(body2.Data) != 2 {
		t.Fatalf("halaman 2 data = %d", len(body2.Data))
	}
}

func TestAlertsPaginationAndSort(t *testing.T) {
	h := newHarness(t)
	// buat 3 alert dengan waktu berbeda
	for i, lvl := range []int{2, 3, 4} {
		body := `{"region_id":"r1","level":` + itoa(lvl) + `,"title":"t","description":"d","valid_hours":1}`
		rec := h.do(t, http.MethodPost, "/v1/alerts", body, h.token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("alert %d status = %d", i, rec.Code)
		}
	}
	rec := h.do(t, http.MethodGet, "/v1/alerts?per_page=2", "", "")
	res := decode[struct {
		Data []domain.Alert `json:"data"`
		Meta struct {
			Total int `json:"total"`
			Pages int `json:"pages"`
		} `json:"meta"`
	}](t, rec)
	if res.Meta.Total != 3 || res.Meta.Pages != 2 || len(res.Data) != 2 {
		t.Fatalf("pagination alert salah: %+v", res.Meta)
	}
}

func itoa(n int) string {
	return string(rune('0' + n))
}
