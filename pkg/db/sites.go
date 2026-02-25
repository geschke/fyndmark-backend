package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Site struct {
	ID        int64  `json:"ID"`
	SiteKey   string `json:"SiteKey"`
	Title     string `json:"Title"`
	Status    string `json:"Status"`
	CreatedAt int64  `json:"CreatedAt"`
	UpdatedAt int64  `json:"UpdatedAt"`
}

const (
	SiteStatusActive   = "active"
	SiteStatusDisabled = "disabled"
)

type cfgSiteInfo struct {
	seen  bool
	title string
}

// SyncSites reconciles config site keys with DB rows.
// Rules:
// - missing config key disables DB rows only when current status is "active"
// - existing config key enables DB rows only when current status is "disabled"
// - other statuses remain untouched
// - new config keys are inserted as active
func (d *DB) SyncSites(ctx context.Context, configuredSiteKeys map[string]string) error {
	if d == nil || d.SQL == nil {
		return fmt.Errorf("db not initialized")
	}

	cfgByKey := make(map[string]cfgSiteInfo, len(configuredSiteKeys))
	for rawKey, rawTitle := range configuredSiteKeys {
		siteKey := strings.TrimSpace(rawKey)
		if siteKey == "" {
			return fmt.Errorf("site key is required")
		}
		if _, exists := cfgByKey[siteKey]; exists {
			return fmt.Errorf("duplicate site key %q", siteKey)
		}
		cfgByKey[siteKey] = cfgSiteInfo{
			seen:  false,
			title: strings.TrimSpace(rawTitle),
		}
	}

	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sites sync tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Compare + Collect
	rows, err := tx.QueryContext(ctx, `
SELECT site_key, status
  FROM sites
 ORDER BY site_key ASC;
`)
	if err != nil {
		return fmt.Errorf("query sites for sync: %w", err)
	}

	toEnable := make([]string, 0)
	toDisable := make([]string, 0)

	for rows.Next() {
		var siteKey string
		var status string
		if err := rows.Scan(&siteKey, &status); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan site for sync: %w", err)
		}

		// site_key found in config?
		if info, inConfig := cfgByKey[siteKey]; inConfig {
			info.seen = true
			cfgByKey[siteKey] = info
			if status == SiteStatusDisabled {
				toEnable = append(toEnable, siteKey)
			}
		} else {
			// site_key not found: set to disabled if status is active in database
			if status == SiteStatusActive {
				toDisable = append(toDisable, siteKey)
			}
		}

	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate sites for sync: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close sites scan: %w", err)
	}

	toInsert := make([]string, 0)
	for siteKey, info := range cfgByKey {
		if !info.seen {
			toInsert = append(toInsert, siteKey)
		}
	}

	// Apply
	now := time.Now().Unix()

	enableStmt, err := tx.PrepareContext(ctx, `
UPDATE sites
   SET status = ?, title = ?, updated_at = ?
 WHERE site_key = ?
   AND status = ?;
`)
	if err != nil {
		return fmt.Errorf("prepare enable site: %w", err)
	}
	defer func() { _ = enableStmt.Close() }()

	disableStmt, err := tx.PrepareContext(ctx, `
UPDATE sites
   SET status = ?, updated_at = ?
 WHERE site_key = ?
   AND status = ?;
`)
	if err != nil {
		return fmt.Errorf("prepare disable site: %w", err)
	}
	defer func() { _ = disableStmt.Close() }()

	insertStmt, err := tx.PrepareContext(ctx, `
INSERT INTO sites (site_key, title, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?);
`)
	if err != nil {
		return fmt.Errorf("prepare insert site: %w", err)
	}
	defer func() { _ = insertStmt.Close() }()

	for _, siteKey := range toEnable {
		if _, err := enableStmt.ExecContext(
			ctx,
			SiteStatusActive,
			cfgByKey[siteKey].title,
			now,
			siteKey,
			SiteStatusDisabled,
		); err != nil {
			return fmt.Errorf("enable site %q: %w", siteKey, err)
		}
	}
	for _, siteKey := range toDisable {
		if _, err := disableStmt.ExecContext(ctx, SiteStatusDisabled, now, siteKey, SiteStatusActive); err != nil {
			return fmt.Errorf("disable site %q: %w", siteKey, err)
		}
	}
	for _, siteKey := range toInsert {
		if _, err := insertStmt.ExecContext(ctx, siteKey, cfgByKey[siteKey].title, SiteStatusActive, now, now); err != nil {
			return fmt.Errorf("insert site %q: %w", siteKey, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sites sync tx: %w", err)
	}
	committed = true
	return nil
}

// GetSiteIDByKey returns data for the requested input.
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

// GetSiteByID returns data for the requested input.
func (d *DB) GetSiteByID(ctx context.Context, siteID int64) (Site, bool, error) {
	if d == nil || d.SQL == nil {
		return Site{}, false, fmt.Errorf("db not initialized")
	}
	if siteID <= 0 {
		return Site{}, false, fmt.Errorf("site id must be > 0")
	}

	var s Site
	err := d.SQL.QueryRowContext(ctx, `
SELECT id, site_key, title, status, created_at, updated_at
  FROM sites
 WHERE id = ?
 LIMIT 1;
`, siteID).Scan(&s.ID, &s.SiteKey, &s.Title, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return Site{}, false, nil
	}
	if err != nil {
		return Site{}, false, fmt.Errorf("get site by id: %w", err)
	}
	return s, true, nil
}

// SiteExists performs its package-specific operation.
func (d *DB) SiteExists(ctx context.Context, siteKey string) (bool, error) {
	_, found, err := d.GetSiteIDByKey(ctx, siteKey)
	return found, err
}

// SiteExistsByID performs its package-specific operation.
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

// ListSites returns a list for the requested filter.
func (d *DB) ListSites(ctx context.Context) ([]Site, error) {
	if d == nil || d.SQL == nil {
		return nil, fmt.Errorf("db not initialized")
	}

	rows, err := d.SQL.QueryContext(ctx, `
SELECT id, site_key, title, status, created_at, updated_at
  FROM sites
 ORDER BY id ASC;
`)
	if err != nil {
		return nil, fmt.Errorf("list sites: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Site, 0)
	for rows.Next() {
		var s Site
		if err := rows.Scan(
			&s.ID,
			&s.SiteKey,
			&s.Title,
			&s.Status,
			&s.CreatedAt,
			&s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan site: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sites: %w", err)
	}

	return out, nil
}

// ListSitesByUserID returns a list for the requested filter.
func (d *DB) ListSitesByUserID(ctx context.Context, userID int64) ([]Site, error) {
	if d == nil || d.SQL == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	if userID <= 0 {
		return nil, fmt.Errorf("userID must be > 0")
	}

	rows, err := d.SQL.QueryContext(ctx, `
SELECT s.id, s.site_key, s.title, s.status, s.created_at, s.updated_at
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
		if err := rows.Scan(&s.ID, &s.SiteKey, &s.Title, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan site by user: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sites by user: %w", err)
	}
	return out, nil
}

// ListAllowedSiteIDsByUserID returns a list for the requested filter.
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

// UserHasSiteAccess performs its package-specific operation.
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

// AssignUserSite performs its package-specific operation.
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

// RemoveUserSite performs its package-specific operation.
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
