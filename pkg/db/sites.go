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
	ID          int64  `json:"id"`
	SiteKey     string `json:"site_key"`
	Name        string `json:"name"`
	DateCreated int64  `json:"date_created"`
	DateUpdated int64  `json:"date_updated"`
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
	now := time.Now().Unix()
	created := site.DateCreated
	if created <= 0 {
		created = now
	}

	_, err := d.SQL.ExecContext(ctx, `
INSERT INTO sites (site_key, name, date_created, date_updated)
VALUES (?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  name = excluded.name,
  date_updated = excluded.date_updated;
`, siteKey, name, created, now)
	if err != nil {
		return fmt.Errorf("upsert site: %w", err)
	}
	return nil
}

func (d *DB) SyncSitesFromConfig(ctx context.Context, cfg map[string]config.CommentsSiteConfig) error {
	if d == nil || d.SQL == nil {
		return fmt.Errorf("db not initialized")
	}
	for siteKey, siteCfg := range cfg {
		if err := d.UpsertSite(ctx, Site{
			SiteKey: siteKey,
			Name:    siteCfg.Title,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) SiteExists(ctx context.Context, siteKey string) (bool, error) {
	if d == nil || d.SQL == nil {
		return false, fmt.Errorf("db not initialized")
	}
	siteKey = strings.TrimSpace(siteKey)
	if siteKey == "" {
		return false, fmt.Errorf("siteID is required")
	}

	var one int
	err := d.SQL.QueryRowContext(ctx, `
SELECT 1
  FROM sites
 WHERE site_key = ?
 LIMIT 1;
`, siteKey).Scan(&one)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("site exists query: %w", err)
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
SELECT s.id, s.site_key, s.name, s.date_created, s.date_updated
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
		if err := rows.Scan(&s.ID, &s.SiteKey, &s.Name, &s.DateCreated, &s.DateUpdated); err != nil {
			return nil, fmt.Errorf("scan site by user: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sites by user: %w", err)
	}
	return out, nil
}

func (d *DB) ListAllowedSiteIDsByUserID(ctx context.Context, userID int64) ([]string, error) {
	if d == nil || d.SQL == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	if userID <= 0 {
		return nil, fmt.Errorf("userID must be > 0")
	}

	rows, err := d.SQL.QueryContext(ctx, `
SELECT s.site_key
  FROM user_sites us, sites s
 WHERE user_id = ? AND us.site_id=s.id
 ORDER BY s.site_key ASC;
`, userID)
	if err != nil {
		return nil, fmt.Errorf("list allowed site ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]string, 0)
	for rows.Next() {
		var siteKey string
		if err := rows.Scan(&siteKey); err != nil {
			return nil, fmt.Errorf("scan allowed site id: %w", err)
		}
		out = append(out, siteKey)
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
	//siteID = strings.TrimSpace(siteID)
	//if siteID == "" {
	//	return false, fmt.Errorf("siteID is required")
	//}

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

func (d *DB) AssignUserSite(ctx context.Context, userID int64, siteID string) error {
	if d == nil || d.SQL == nil {
		return fmt.Errorf("db not initialized")
	}
	if userID <= 0 {
		return fmt.Errorf("userID must be > 0")
	}
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return fmt.Errorf("siteID is required")
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

func (d *DB) RemoveUserSite(ctx context.Context, userID int64, siteID string) (bool, error) {
	if d == nil || d.SQL == nil {
		return false, fmt.Errorf("db not initialized")
	}
	if userID <= 0 {
		return false, fmt.Errorf("userID must be > 0")
	}
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return false, fmt.Errorf("siteID is required")
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
