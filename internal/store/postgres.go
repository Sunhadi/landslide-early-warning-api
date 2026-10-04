package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"peringatan-dini/internal/domain"
)

// PostgresStore adalah implementasi Store berbasis PostgreSQL + PostGIS.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// Pastikan PostgresStore memenuhi interface Store saat kompilasi.
var _ Store = (*PostgresStore)(nil)

// NewPostgres membuka koneksi pool ke PostgreSQL.
func NewPostgres(ctx context.Context, dsn string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat pool postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("gagal terhubung ke postgres: %w", err)
	}
	return &PostgresStore{pool: pool}, nil
}

// Close menutup pool koneksi.
func (p *PostgresStore) Close() { p.pool.Close() }

// --- Regions ---

func (p *PostgresStore) ListRegions(ctx context.Context) ([]domain.Region, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, adm4, name, kabupaten, kecamatan,
		       lat, lon, slope_deg, susceptibility, updated_at
		FROM regions ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.Region{}
	for rows.Next() {
		r, err := scanRegion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *PostgresStore) GetRegion(ctx context.Context, id string) (domain.Region, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, adm4, name, kabupaten, kecamatan,
		       lat, lon, slope_deg, susceptibility, updated_at
		FROM regions WHERE id = $1`, id)
	r, err := scanRegion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Region{}, ErrNotFound
	}
	return r, err
}

// SaveRegion menyimpan/menimpa wilayah (upsert).
func (p *PostgresStore) SaveRegion(ctx context.Context, r domain.Region) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO regions (id, adm4, name, kabupaten, kecamatan, lat, lon, slope_deg, susceptibility, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		ON CONFLICT (id) DO UPDATE SET
			adm4 = EXCLUDED.adm4, name = EXCLUDED.name, kabupaten = EXCLUDED.kabupaten,
			kecamatan = EXCLUDED.kecamatan, lat = EXCLUDED.lat, lon = EXCLUDED.lon,
			slope_deg = EXCLUDED.slope_deg,
			susceptibility = EXCLUDED.susceptibility, updated_at = now()`,
		r.ID, r.Adm4, r.Name, r.Kabupaten, r.Kecamatan, r.Lat, r.Lon, r.SlopeDeg, r.Susceptibility)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRegion(s rowScanner) (domain.Region, error) {
	var r domain.Region
	err := s.Scan(&r.ID, &r.Adm4, &r.Name, &r.Kabupaten, &r.Kecamatan,
		&r.Lat, &r.Lon, &r.SlopeDeg, &r.Susceptibility, &r.UpdatedAt)
	return r, err
}

// --- Rainfall ---

func (p *PostgresStore) SaveRainfall(ctx context.Context, r domain.RainfallData) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO rainfall (region_id, source, observed_at, rain_24h_mm, rain_72h_mm, forecast_24h_mm, fetched_at, freshness)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		r.RegionID, r.Source, r.ObservedAt, r.Rain24hMM, r.Rain72hMM, r.Forecast24h, r.FetchedAt, r.Freshness)
	return err
}

func (p *PostgresStore) GetRainfall(ctx context.Context, regionID string) (domain.RainfallData, error) {
	var r domain.RainfallData
	err := p.pool.QueryRow(ctx, `
		SELECT region_id, source, observed_at, rain_24h_mm, rain_72h_mm, forecast_24h_mm, fetched_at, freshness
		FROM rainfall WHERE region_id = $1 ORDER BY observed_at DESC LIMIT 1`, regionID).
		Scan(&r.RegionID, &r.Source, &r.ObservedAt, &r.Rain24hMM, &r.Rain72hMM, &r.Forecast24h, &r.FetchedAt, &r.Freshness)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RainfallData{}, ErrNotFound
	}
	return r, err
}

// --- Assessments ---

func (p *PostgresStore) SaveAssessment(ctx context.Context, a domain.RiskAssessment) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO risk_assessments (region_id, score, level, level_label, factors, triggers, freshness, evaluated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		a.RegionID, a.Score, int(a.Level), a.LevelLabel, a.Factors, a.Triggers, a.Freshness, a.EvaluatedAt)
	return err
}

func (p *PostgresStore) GetAssessment(ctx context.Context, regionID string) (domain.RiskAssessment, error) {
	var a domain.RiskAssessment
	var level int
	err := p.pool.QueryRow(ctx, `
		SELECT region_id, score, level, level_label, factors, triggers, freshness, evaluated_at
		FROM risk_assessments WHERE region_id = $1 ORDER BY evaluated_at DESC LIMIT 1`, regionID).
		Scan(&a.RegionID, &a.Score, &level, &a.LevelLabel, &a.Factors, &a.Triggers, &a.Freshness, &a.EvaluatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RiskAssessment{}, ErrNotFound
	}
	if err != nil {
		return domain.RiskAssessment{}, err
	}
	a.Level = domain.RiskLevel(level)

	// lengkapi nama wilayah
	if reg, err := p.GetRegion(ctx, regionID); err == nil {
		a.RegionName = reg.Name
	}
	return a, nil
}

// --- Alerts ---

func (p *PostgresStore) ListAlerts(ctx context.Context, activeOnly bool) ([]domain.Alert, error) {
	q := `SELECT id, region_id, level, level_label, title, description, issued_at, expires_at, source, status, channels
	      FROM alerts`
	args := []any{}
	if activeOnly {
		q += ` WHERE status = 'active'`
	}
	q += ` ORDER BY issued_at DESC`
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.Alert{}
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (p *PostgresStore) GetAlert(ctx context.Context, id string) (domain.Alert, error) {
	row := p.pool.QueryRow(ctx, `SELECT id, region_id, level, level_label, title, description, issued_at, expires_at, source, status, channels FROM alerts WHERE id = $1`, id)
	a, err := scanAlert(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Alert{}, ErrNotFound
	}
	return a, err
}

func (p *PostgresStore) SaveAlert(ctx context.Context, a domain.Alert) error {
	if a.ID == "" {
		a.ID = newUUID()
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO alerts (id, region_id, level, level_label, title, description, issued_at, expires_at, source, status, channels)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET level = EXCLUDED.level, level_label = EXCLUDED.level_label,
			title = EXCLUDED.title, description = EXCLUDED.description, expires_at = EXCLUDED.expires_at,
			status = EXCLUDED.status, channels = EXCLUDED.channels`,
		a.ID, a.RegionID, int(a.Level), a.LevelLabel, a.Title, a.Description, a.IssuedAt, a.ExpiresAt, a.Source, a.Status, a.Channels)
	return err
}

func (p *PostgresStore) ActiveAlertForRegion(ctx context.Context, regionID string) (domain.Alert, bool, error) {
	row := p.pool.QueryRow(ctx, `SELECT id, region_id, level, level_label, title, description, issued_at, expires_at, source, status, channels FROM alerts WHERE region_id = $1 AND status = 'active' ORDER BY issued_at DESC LIMIT 1`, regionID)
	a, err := scanAlert(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Alert{}, false, nil
	}
	if err != nil {
		return domain.Alert{}, false, err
	}
	return a, true, nil
}

func scanAlert(s rowScanner) (domain.Alert, error) {
	var a domain.Alert
	var level int
	err := s.Scan(&a.ID, &a.RegionID, &level, &a.LevelLabel, &a.Title, &a.Description,
		&a.IssuedAt, &a.ExpiresAt, &a.Source, &a.Status, &a.Channels)
	a.Level = domain.RiskLevel(level)
	return a, err
}

// --- Reports ---

func (p *PostgresStore) ListReports(ctx context.Context) ([]domain.Report, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, lat, lon, COALESCE(region_id, ''),
		       description, COALESCE(severity_guess, ''), status, COALESCE(reporter_name, ''),
		       COALESCE(reporter_phone_enc, ''), created_at
		FROM reports ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Report{}
	for rows.Next() {
		var r domain.Report
		if err := rows.Scan(&r.ID, &r.Lat, &r.Lon, &r.RegionID, &r.Description, &r.SeverityGuess,
			&r.Status, &r.ReporterName, &r.ReporterPhone, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *PostgresStore) SaveReport(ctx context.Context, r domain.Report) error {
	if r.ID == "" {
		r.ID = newUUID()
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO reports (id, lat, lon, region_id, description, severity_guess, status, reporter_name, reporter_phone_enc, created_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''), $7, NULLIF($8, ''), NULLIF($9, ''), $10)
		ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status, description = EXCLUDED.description,
			region_id = EXCLUDED.region_id`,
		r.ID, r.Lat, r.Lon, r.RegionID, r.Description, r.SeverityGuess, r.Status, r.ReporterName, r.ReporterPhone, r.CreatedAt)
	return err
}

func (p *PostgresStore) GetReport(ctx context.Context, id string) (domain.Report, error) {
	var r domain.Report
	err := p.pool.QueryRow(ctx, `
		SELECT id, lat, lon, COALESCE(region_id, ''),
		       description, COALESCE(severity_guess, ''), status, COALESCE(reporter_name, ''),
		       COALESCE(reporter_phone_enc, ''), created_at
		FROM reports WHERE id = $1`, id).
		Scan(&r.ID, &r.Lat, &r.Lon, &r.RegionID, &r.Description, &r.SeverityGuess,
			&r.Status, &r.ReporterName, &r.ReporterPhone, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Report{}, ErrNotFound
	}
	return r, err
}

// --- Subscriptions ---

func (p *PostgresStore) SaveSubscription(ctx context.Context, s domain.Subscription) error {
	if s.ID == "" {
		s.ID = newUUID()
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO subscriptions (id, region_id, phone_enc, phone_masked, channel, consent, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		s.ID, s.RegionID, s.Phone, s.PhoneMask, s.Channel, s.Consent, s.CreatedAt)
	return err
}

func (p *PostgresStore) DeleteSubscription(ctx context.Context, id string) error {
	tag, err := p.pool.Exec(ctx, `DELETE FROM subscriptions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *PostgresStore) ListSubscriptions(ctx context.Context, regionID string) ([]domain.Subscription, error) {
	q := `SELECT id, region_id, phone_enc, phone_masked, channel, consent, created_at FROM subscriptions`
	args := []any{}
	if regionID != "" {
		q += ` WHERE region_id = $1`
		args = append(args, regionID)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Subscription{}
	for rows.Next() {
		var s domain.Subscription
		if err := rows.Scan(&s.ID, &s.RegionID, &s.Phone, &s.PhoneMask, &s.Channel, &s.Consent, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// --- Users ---

func (p *PostgresStore) GetUserByUsername(ctx context.Context, username string) (domain.User, error) {
	var u domain.User
	err := p.pool.QueryRow(ctx, `SELECT id, username, password_hash, role FROM users WHERE lower(username) = lower($1)`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ErrNotFound
	}
	return u, err
}

func (p *PostgresStore) SaveUser(ctx context.Context, u domain.User) error {
	if u.ID == "" {
		u.ID = newUUID()
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO users (id, username, password_hash, role) VALUES ($1, $2, $3, $4)
		ON CONFLICT (username) DO UPDATE SET password_hash = EXCLUDED.password_hash, role = EXCLUDED.role`,
		u.ID, u.Username, u.PasswordHash, u.Role)
	return err
}

// --- Audit ---

func (p *PostgresStore) AppendAudit(ctx context.Context, actor, action, detail string) {
	_, _ = p.pool.Exec(ctx, `INSERT INTO audit_log (actor, action, detail) VALUES ($1, $2, $3)`, actor, action, detail)
}

func (p *PostgresStore) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := p.pool.Query(ctx, `SELECT at, actor, action, detail FROM audit_log ORDER BY at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.At, &e.Actor, &e.Action, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// newUUID membuat UUID string.
func newUUID() string {
	return uuid.NewString()
}
