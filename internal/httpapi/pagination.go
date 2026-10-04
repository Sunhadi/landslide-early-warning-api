package httpapi

import (
	"net/http"
	"sort"
	"strconv"

	"peringatan-dini/internal/domain"
)

const (
	defaultPerPage = 20
	maxPerPage     = 100
)

// pageParams menyimpan parameter pagination yang sudah divalidasi.
type pageParams struct {
	Page    int
	PerPage int
}

// parsePage membaca ?page=&per_page= dengan nilai aman.
func parsePage(r *http.Request) pageParams {
	page := 1
	perPage := defaultPerPage
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perPage = n
		}
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	return pageParams{Page: page, PerPage: perPage}
}

// paginate mengembalikan potongan data untuk halaman & total keseluruhan.
func paginate[T any](items []T, p pageParams) ([]T, int) {
	total := len(items)
	start := (p.Page - 1) * p.PerPage
	if start >= total {
		return []T{}, total
	}
	end := start + p.PerPage
	if end > total {
		end = total
	}
	return items[start:end], total
}

// writeList menulis respons daftar terpaginasi dengan metadata.
func writeList[T any](w http.ResponseWriter, items []T, p pageParams) {
	slice, total := paginate(items, p)
	writeJSON(w, http.StatusOK, map[string]any{
		"data": slice,
		"meta": map[string]any{
			"page":     p.Page,
			"per_page": p.PerPage,
			"total":    total,
			"pages":    pages(total, p.PerPage),
		},
	})
}

// pages menghitung jumlah halaman.
func pages(total, perPage int) int {
	if perPage <= 0 || total == 0 {
		return 0
	}
	return (total + perPage - 1) / perPage
}

// sortAlertsNewestFirst mengurutkan alert dari yang terbaru.
func sortAlertsNewestFirst(items []domain.Alert) {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].IssuedAt.After(items[j].IssuedAt)
	})
}

// sortReportsNewestFirst mengurutkan laporan dari yang terbaru.
func sortReportsNewestFirst(items []domain.Report) {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
}
