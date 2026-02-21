package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/geschke/fyndmark/config"
)

type Site struct {
	ID        int64  `json:"ID"`
	SiteKey   string `json:"SiteKey"`
	Name      string `json:"Name"`
	Status    string `json:"Status"`
	CreatedAt int64  `json:"CreatedAt"`
	UpdatedAt int64  `json:"UpdatedAt"`
}

const (
	SiteStatusActive   = "active"
	SiteStatusDisabled = "disabled"
)

func isValidSiteStatus(status string) bool {
	switch status {
	case SiteStatusActive, SiteStatusDisabled:
		return true
	default:
		return false
	}
}

func (d *DB) UpsertSite(ctx context.Context, site Site) error {
	if d == nil || d.SQL == nil {
		return fmt.Errorf("db not initialized")
	}

	siteKey := strings.TrimSpace(site.SiteKey)
	if siteKey == "" {
		return fmt.Errorf("site key is required")
	}
	name := strings.TrimSpace(site.Name)
	status := strings.TrimSpace(site.Status)
	if !isValidSiteStatus(status) {
		return fmt.Errorf("invalid site status %q", status)
	}
	now := time.Now().Unix()
	created := site.CreatedAt
	if created <= 0 {
		created = now
	}

	_, err := d.SQL.ExecContext(ctx, `
INSERT INTO sites (site_key, name, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(site_key) DO UPDATE SET
  name = excluded.name,
  status = excluded.status,
  updated_at = excluded.updated_at;
`, siteKey, name, status, created, now)
	if err != nil {
		return fmt.Errorf("upsert site: %w", err)
	}
	return nil
}

func (d *DB) SyncSitesFromConfig(ctx context.Context, cfg map[string]config.CommentsSiteConfig) error {
	if d == nil || d.SQL == nil {
		return fmt.Errorf("db not initialized")
	}

	configSiteKeys := make([]string, 0, len(cfg))
	for siteKey, siteCfg := range cfg {
		siteKey = strings.TrimSpace(siteKey)
		if siteKey == "" {
			return fmt.Errorf("site key is required")
		}
		configSiteKeys = append(configSiteKeys, siteKey)

		if err := d.UpsertSite(ctx, Site{
			SiteKey: siteKey,
			Name:    siteCfg.Title,
			Status:  SiteStatusActive,
		}); err != nil {
			return fmt.Errorf("upsert config site %q: %w", siteKey, err)
		}
	}

	if err := d.disableSitesNotInConfig(ctx, configSiteKeys); err != nil {
		return err
	}

	return nil
}

func (d *DB) disableSitesNotInConfig(ctx context.Context, activeSiteKeys []string) error {
	if d == nil || d.SQL == nil {
		return fmt.Errorf("db not initialized")
	}

	now := time.Now().Unix()

	// Empty config means all existing DB sites are no longer configured.
	if len(activeSiteKeys) == 0 {
		_, err := d.SQL.ExecContext(ctx, `
UPDATE sites
   SET status = ?, updated_at = ?
 WHERE status <> ?;
`, SiteStatusDisabled, now, SiteStatusDisabled)
		if err != nil {
			return fmt.Errorf("disable sites not in config: %w", err)
		}
		return nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(activeSiteKeys)), ",")
	query := fmt.Sprintf(`
UPDATE sites
   SET status = ?, updated_at = ?
 WHERE status <> ?
   AND site_key NOT IN (%s);
`, placeholders)

	args := make([]any, 0, 3+len(activeSiteKeys))
	args = append(args, SiteStatusDisabled, now, SiteStatusDisabled)
	for _, siteKey := range activeSiteKeys {
		args = append(args, siteKey)
	}

	if _, err := d.SQL.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("disable sites not in config: %w", err)
	}
	return nil
}

func (d *DB) GetSiteIDByKey(ctx context.Context, siteKey string) (int64, bool, error) {
	if d == nil || d.SQL == nil {
		return 0, false, fmt.Errorf("db not initialized")
	}
	siteKey = strings.TrimSpace(siteKey)
	if siteKey == "" {
		return 0, false, fmt.Errorf("site key is required")
	}

	var siteID int64
	err := d.SQL.QueryRowContext(ctx, `
SELECT id
  FROM sites
 WHERE site_key = ?
 LIMIT 1;
`, siteKey).Scan(&siteID)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("get site id by key: %w", err)
	}
	return siteID, true, nil
}

func (d *DB) GetSiteByID(ctx context.Context, siteID int64) (Site, bool, error) {
	if d == nil || d.SQL == nil {
		return Site{}, false, fmt.Errorf("db not initialized")
	}
	if siteID <= 0 {
		return Site{}, false, fmt.Errorf("site id must be > 0")
	}

	var s Site
	err := d.SQL.QueryRowContext(ctx, `
SELECT id, site_key, name, status, created_at, updated_at
  FROM sites
 WHERE id = ?
 LIMIT 1;
`, siteID).Scan(&s.ID, &s.SiteKey, &s.Name, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return Site{}, false, nil
	}
	if err != nil {
		return Site{}, false, fmt.Errorf("get site by id: %w", err)
	}
	return s, true, nil
}

func (d *DB) SiteExists(ctx context.Context, siteKey string) (bool, error) {
	_, found, err := d.GetSiteIDByKey(ctx, siteKey)
	return found, err
}

func (d *DB) SiteExistsByID(ctx context.Context, siteID int64) (bool, error) {
	if d == nil || d.SQL == nil {
		return false, fmt.Errorf("db not initialized")
	}
	if siteID <= 0 {
		return false, fmt.Errorf("site id must be > 0")
	}
	var one int
	err := d.SQL.QueryRowContext(ctx, `
SELECT 1
  FROM sites
 WHERE id = ?
 LIMIT 1;
`, siteID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("site exists by id query: %w", err)
	}
	return true, nil
}

func (d *DB) ListSitesByUserID(ctx context.Context, userID int64) ([]Site, error) {
	if d == nil || d.SQL == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	if userID <= 0 {
		return nil, fmt.Errorf("userID must be > 0")
	}

	rows, err := d.SQL.QueryContext(ctx, `
SELECT s.id, s.site_key, s.name, s.status, s.created_at, s.updated_at
  FROM sites s
  JOIN user_sites us ON us.site_id = s.id
 WHERE us.user_id = ?
 ORDER BY s.id ASC;
`, userID)
	if err != nil {
		return nil, fmt.Errorf("list sites by user: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Site, 0)
	for rows.Next() {
		var s Site
		if err := rows.Scan(&s.ID, &s.SiteKey, &s.Name, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan site by user: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sites by user: %w", err)
	}
	return out, nil
}

func (d *DB) ListAllowedSiteIDsByUserID(ctx context.Context, userID int64) ([]int64, error) {
	if d == nil || d.SQL == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	if userID <= 0 {
		return nil, fmt.Errorf("userID must be > 0")
	}

	rows, err := d.SQL.QueryContext(ctx, `
SELECT site_id
  FROM user_sites
 WHERE user_id = ?
 ORDER BY site_id ASC;
`, userID)
	if err != nil {
		return nil, fmt.Errorf("list allowed site ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]int64, 0)
	for rows.Next() {
		var siteID int64
		if err := rows.Scan(&siteID); err != nil {
			return nil, fmt.Errorf("scan allowed site id: %w", err)
		}
		out = append(out, siteID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate allowed site ids: %w", err)
	}
	return out, nil
}

func (d *DB) UserHasSiteAccess(ctx context.Context, userID int64, siteID int64) (bool, error) {
	if d == nil || d.SQL == nil {
		return false, fmt.Errorf("db not initialized")
	}
	if userID <= 0 {
		return false, fmt.Errorf("userID must be > 0")
	}
	if siteID <= 0 {
		return false, fmt.Errorf("siteID must be > 0")
	}

	var one int
	err := d.SQL.QueryRowContext(ctx, `
SELECT 1
  FROM user_sites
 WHERE user_id = ?
   AND site_id = ?
 LIMIT 1;
`, userID, siteID).Scan(&one)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("user has site access query: %w", err)
	}
	return true, nil
}

func (d *DB) AssignUserSite(ctx context.Context, userID int64, siteID int64) error {
	if d == nil || d.SQL == nil {
		return fmt.Errorf("db not initialized")
	}
	if userID <= 0 {
		return fmt.Errorf("userID must be > 0")
	}
	if siteID <= 0 {
		return fmt.Errorf("siteID must be > 0")
	}

	_, err := d.SQL.ExecContext(ctx, `
INSERT INTO user_sites (user_id, site_id)
VALUES (?, ?)
ON CONFLICT(user_id, site_id) DO NOTHING;
`, userID, siteID)
	if err != nil {
		return fmt.Errorf("assign user site: %w", err)
	}
	return nil
}

func (d *DB) RemoveUserSite(ctx context.Context, userID int64, siteID int64) (bool, error) {
	if d == nil || d.SQL == nil {
		return false, fmt.Errorf("db not initialized")
	}
	if userID <= 0 {
		return false, fmt.Errorf("userID must be > 0")
	}
	if siteID <= 0 {
		return false, fmt.Errorf("siteID must be > 0")
	}

	res, err := d.SQL.ExecContext(ctx, `
DELETE FROM user_sites
 WHERE user_id = ?
   AND site_id = ?;
`, userID, siteID)
	if err != nil {
		return false, fmt.Errorf("remove user site: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("remove user site rows affected: %w", err)
	}
	return affected > 0, nil
}
