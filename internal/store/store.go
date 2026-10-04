// Package store mendefinisikan repository data dan implementasi in-memory.
// Implementasi PostgreSQL dapat ditambahkan dengan mengimplementasikan interface Store.
package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"peringatan-dini/internal/domain"
)

// ErrNotFound dikembalikan bila data tidak ditemukan.
var ErrNotFound = errors.New("data tidak ditemukan")

// Store adalah abstraksi penyimpanan data aplikasi.
type Store interface {
	ListRegions(ctx context.Context) ([]domain.Region, error)
	GetRegion(ctx context.Context, id string) (domain.Region, error)
	SaveRegion(ctx context.Context, r domain.Region) error

	SaveRainfall(ctx context.Context, r domain.RainfallData) error
	GetRainfall(ctx context.Context, regionID string) (domain.RainfallData, error)

	SaveAssessment(ctx context.Context, a domain.RiskAssessment) error
	GetAssessment(ctx context.Context, regionID string) (domain.RiskAssessment, error)

	ListAlerts(ctx context.Context, activeOnly bool) ([]domain.Alert, error)
	GetAlert(ctx context.Context, id string) (domain.Alert, error)
	SaveAlert(ctx context.Context, a domain.Alert) error
	ActiveAlertForRegion(ctx context.Context, regionID string) (domain.Alert, bool, error)

	ListReports(ctx context.Context) ([]domain.Report, error)
	SaveReport(ctx context.Context, r domain.Report) error
	GetReport(ctx context.Context, id string) (domain.Report, error)

	SaveSubscription(ctx context.Context, s domain.Subscription) error
	DeleteSubscription(ctx context.Context, id string) error
	ListSubscriptions(ctx context.Context, regionID string) ([]domain.Subscription, error)

	GetUserByUsername(ctx context.Context, username string) (domain.User, error)
	SaveUser(ctx context.Context, u domain.User) error

	AppendAudit(ctx context.Context, actor, action, detail string)
	ListAudit(ctx context.Context, limit int) ([]AuditEntry, error)
}

// AuditEntry adalah catatan audit.
type AuditEntry struct {
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Detail string    `json:"detail"`
}

// MemoryStore adalah implementasi in-memory (thread-safe) untuk belajar.
type MemoryStore struct {
	mu         sync.RWMutex
	regions    map[string]domain.Region
	rainfall   map[string]domain.RainfallData
	assessment map[string]domain.RiskAssessment
	alerts     map[string]domain.Alert
	reports    map[string]domain.Report
	subs       map[string]domain.Subscription
	users      map[string]domain.User // by username
	audit      []AuditEntry
}

// NewMemoryStore membuat store in-memory kosong.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		regions:    make(map[string]domain.Region),
		rainfall:   make(map[string]domain.RainfallData),
		assessment: make(map[string]domain.RiskAssessment),
		alerts:     make(map[string]domain.Alert),
		reports:    make(map[string]domain.Report),
		subs:       make(map[string]domain.Subscription),
		users:      make(map[string]domain.User),
	}
}

// --- Regions ---

// ListRegions mengembalikan semua wilayah.
func (m *MemoryStore) ListRegions(_ context.Context) ([]domain.Region, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.Region, 0, len(m.regions))
	for _, r := range m.regions {
		out = append(out, r)
	}
	return out, nil
}

// GetRegion mengambil wilayah berdasarkan ID.
func (m *MemoryStore) GetRegion(_ context.Context, id string) (domain.Region, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.regions[id]
	if !ok {
		return domain.Region{}, ErrNotFound
	}
	return r, nil
}

// SaveRegion menyimpan/menimpa wilayah (dipakai untuk seeding).
func (m *MemoryStore) SaveRegion(_ context.Context, r domain.Region) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.regions[r.ID] = r
	return nil
}

// --- Rainfall ---

// SaveRainfall menyimpan data curah hujan.
func (m *MemoryStore) SaveRainfall(_ context.Context, r domain.RainfallData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rainfall[r.RegionID] = r
	return nil
}

// GetRainfall mengambil data curah hujan terakhir.
func (m *MemoryStore) GetRainfall(_ context.Context, regionID string) (domain.RainfallData, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.rainfall[regionID]
	if !ok {
		return domain.RainfallData{}, ErrNotFound
	}
	return r, nil
}

// --- Assessment ---

// SaveAssessment menyimpan hasil penilaian risiko.
func (m *MemoryStore) SaveAssessment(_ context.Context, a domain.RiskAssessment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.assessment[a.RegionID] = a
	return nil
}

// GetAssessment mengambil hasil penilaian risiko terakhir.
func (m *MemoryStore) GetAssessment(_ context.Context, regionID string) (domain.RiskAssessment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.assessment[regionID]
	if !ok {
		return domain.RiskAssessment{}, ErrNotFound
	}
	return a, nil
}

// --- Alerts ---

// ListAlerts mengembalikan daftar peringatan; activeOnly membatasi status "active".
func (m *MemoryStore) ListAlerts(_ context.Context, activeOnly bool) ([]domain.Alert, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.Alert, 0, len(m.alerts))
	for _, a := range m.alerts {
		if activeOnly && a.Status != "active" {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// GetAlert mengambil peringatan berdasarkan ID.
func (m *MemoryStore) GetAlert(_ context.Context, id string) (domain.Alert, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.alerts[id]
	if !ok {
		return domain.Alert{}, ErrNotFound
	}
	return a, nil
}

// SaveAlert menyimpan peringatan.
func (m *MemoryStore) SaveAlert(_ context.Context, a domain.Alert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	m.alerts[a.ID] = a
	return nil
}

// ActiveAlertForRegion mencari peringatan aktif untuk wilayah tertentu.
func (m *MemoryStore) ActiveAlertForRegion(_ context.Context, regionID string) (domain.Alert, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, a := range m.alerts {
		if a.RegionID == regionID && a.Status == "active" {
			return a, true, nil
		}
	}
	return domain.Alert{}, false, nil
}

// --- Reports ---

// ListReports mengembalikan semua laporan.
func (m *MemoryStore) ListReports(_ context.Context) ([]domain.Report, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.Report, 0, len(m.reports))
	for _, r := range m.reports {
		out = append(out, r)
	}
	return out, nil
}

// SaveReport menyimpan laporan.
func (m *MemoryStore) SaveReport(_ context.Context, r domain.Report) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	m.reports[r.ID] = r
	return nil
}

// GetReport mengambil laporan berdasarkan ID.
func (m *MemoryStore) GetReport(_ context.Context, id string) (domain.Report, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.reports[id]
	if !ok {
		return domain.Report{}, ErrNotFound
	}
	return r, nil
}

// --- Subscriptions ---

// SaveSubscription menyimpan langganan.
func (m *MemoryStore) SaveSubscription(_ context.Context, s domain.Subscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	m.subs[s.ID] = s
	return nil
}

// DeleteSubscription menghapus langganan.
func (m *MemoryStore) DeleteSubscription(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.subs[id]; !ok {
		return ErrNotFound
	}
	delete(m.subs, id)
	return nil
}

// ListSubscriptions mengembalikan langganan (difilter regionID bila diisi).
func (m *MemoryStore) ListSubscriptions(_ context.Context, regionID string) ([]domain.Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.Subscription, 0, len(m.subs))
	for _, s := range m.subs {
		if regionID != "" && s.RegionID != regionID {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// --- Users ---

// GetUserByUsername mengambil user berdasarkan username.
func (m *MemoryStore) GetUserByUsername(_ context.Context, username string) (domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[strings.ToLower(username)]
	if !ok {
		return domain.User{}, ErrNotFound
	}
	return u, nil
}

// SaveUser menyimpan user.
func (m *MemoryStore) SaveUser(_ context.Context, u domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[strings.ToLower(u.Username)] = u
	return nil
}

// --- Audit ---

// AppendAudit menambahkan catatan audit.
func (m *MemoryStore) AppendAudit(_ context.Context, actor, action, detail string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audit = append(m.audit, AuditEntry{
		At:     time.Now(),
		Actor:  actor,
		Action: action,
		Detail: detail,
	})
}

// ListAudit mengembalikan catatan audit terbaru.
func (m *MemoryStore) ListAudit(_ context.Context, limit int) ([]AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > len(m.audit) {
		limit = len(m.audit)
	}
	out := make([]AuditEntry, 0, limit)
	// ambil dari yang terbaru
	for i := len(m.audit) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, m.audit[i])
	}
	return out, nil
}
