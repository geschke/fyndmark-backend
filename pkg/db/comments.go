package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Comment struct {
	ID        string
	SiteID    string
	EntryID   sql.NullString
	PostPath  string
	ParentID  sql.NullString
	Status    string
	Author    string
	Email     string
	AuthorUrl sql.NullString
	Body      string
	CreatedAt int64
}

func (c Comment) AuthorURLString() string {
	if c.AuthorUrl.Valid {
		return c.AuthorUrl.String
	}
	return ""
}

func normalizeNullString(ns sql.NullString) sql.NullString {
	s := strings.TrimSpace(ns.String)
	if s == "" {
		return sql.NullString{String: "", Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

func (d *DB) InsertComment(ctx context.Context, c Comment) error {
	if d == nil || d.SQL == nil {
		return fmt.Errorf("db not initialized")
	}

	c.SiteID = strings.TrimSpace(c.SiteID)
	c.PostPath = strings.TrimSpace(c.PostPath)
	c.Author = strings.TrimSpace(c.Author)
	c.AuthorUrl = normalizeNullString(c.AuthorUrl)

	c.Body = strings.TrimSpace(c.Body)
	c.Status = strings.TrimSpace(c.Status)

	if c.CreatedAt == 0 {
		c.CreatedAt = time.Now().Unix()
	}

	_, err := d.SQL.ExecContext(ctx, `
INSERT INTO comments (
  id, site_id, entry_id, post_path, parent_id, status, author, email, author_url, body, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
`, c.ID, c.SiteID, c.EntryID, c.PostPath, c.ParentID, c.Status, c.Author, c.Email, c.AuthorUrl, c.Body, c.CreatedAt)

	if err != nil {
		return fmt.Errorf("insert comment: %w", err)
	}

	return nil
}

// ApproveComment sets a pending comment to approved (idempotent-ish).
// Returns true if a row was updated, false if nothing changed (not found or already decided).
func (d *DB) ApproveComment(ctx context.Context, siteID, commentID string) (bool, error) {
	if d == nil || d.SQL == nil {
		return false, fmt.Errorf("db not initialized")
	}

	now := time.Now().Unix()

	res, err := d.SQL.ExecContext(ctx, `
UPDATE comments
   SET status = 'approved',
       approved_at = ?,
       rejected_at = NULL
 WHERE site_id = ?
   AND id = ?
   AND status = 'pending';
`, now, siteID, commentID)
	if err != nil {
		return false, fmt.Errorf("approve comment: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("approve comment rows affected: %w", err)
	}
	return affected > 0, nil
}

// RejectComment sets a pending comment to rejected (idempotent-ish).
// Returns true if a row was updated, false if nothing changed (not found or already decided).
func (d *DB) RejectComment(ctx context.Context, siteID, commentID string) (bool, error) {
	if d == nil || d.SQL == nil {
		return false, fmt.Errorf("db not initialized")
	}

	now := time.Now().Unix()

	res, err := d.SQL.ExecContext(ctx, `
UPDATE comments
   SET status = 'rejected',
       rejected_at = ?,
       approved_at = NULL
 WHERE site_id = ?
   AND id = ?
   AND status = 'pending';
`, now, siteID, commentID)
	if err != nil {
		return false, fmt.Errorf("reject comment: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("reject comment rows affected: %w", err)
	}
	return affected > 0, nil
}

// ListApprovedComments returns all approved comments for a site, ordered deterministically.
// Ordering: post_path ASC, created_at ASC, id ASC.
func (d *DB) ListApprovedComments(ctx context.Context, siteID string) ([]Comment, error) {
	if d == nil || d.SQL == nil {
		return nil, fmt.Errorf("db not initialized")
	}

	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return nil, fmt.Errorf("siteID is required")
	}

	rows, err := d.SQL.QueryContext(ctx, `
SELECT id, site_id, entry_id, post_path, parent_id, status, author, email, author_url, body, created_at
  FROM comments
 WHERE site_id = ?
   AND status = 'approved'
 ORDER BY post_path ASC, created_at ASC, id ASC;
`, siteID)
	if err != nil {
		return nil, fmt.Errorf("list approved comments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(
			&c.ID,
			&c.SiteID,
			&c.EntryID,
			&c.PostPath,
			&c.ParentID,
			&c.Status,
			&c.Author,
			&c.Email,
			&c.AuthorUrl,
			&c.Body,
			&c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan approved comment: %w", err)
		}
		out = append(out, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate approved comments: %w", err)
	}

	return out, nil
}
