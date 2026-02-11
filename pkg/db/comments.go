package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Comment struct {
	ID         string         `json:"ID"`
	SiteID     int64          `json:"SiteID"`
	EntryID    sql.NullString `json:"EntryID"`
	PostPath   string         `json:"PostPath"`
	ParentID   sql.NullString `json:"ParentID"`
	Status     string         `json:"Status"`
	Author     string         `json:"Author"`
	Email      string         `json:"Email"`
	AuthorUrl  sql.NullString `json:"AuthorUrl"`
	Body       string         `json:"Body"`
	CreatedAt  int64          `json:"CreatedAt"`
	ApprovedAt int64          `json:"ApprovedAt"`
	RejectedAt int64          `json:"RejectedAt"`
}

type CommentListFilter struct {
	SiteID int64
	// AllowedSiteIDs must contain all sites the current user may access.
	AllowedSiteIDs []int64
	// pending|approved|rejected|all
	Status string
	Limit  int
	Offset int
}

func (c Comment) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID         string `json:"ID"`
		SiteID     int64  `json:"SiteID"`
		EntryID    string `json:"EntryID"`
		PostPath   string `json:"PostPath"`
		ParentID   string `json:"ParentID"`
		Status     string `json:"Status"`
		Author     string `json:"Author"`
		Email      string `json:"Email"`
		AuthorUrl  string `json:"AuthorUrl"`
		Body       string `json:"Body"`
		CreatedAt  int64  `json:"CreatedAt"`
		ApprovedAt int64  `json:"ApprovedAt"`
		RejectedAt int64  `json:"RejectedAt"`
	}{
		ID:         c.ID,
		SiteID:     c.SiteID,
		EntryID:    nullStringToString(c.EntryID),
		PostPath:   c.PostPath,
		ParentID:   nullStringToString(c.ParentID),
		Status:     c.Status,
		Author:     c.Author,
		Email:      c.Email,
		AuthorUrl:  nullStringToString(c.AuthorUrl),
		Body:       c.Body,
		CreatedAt:  c.CreatedAt,
		ApprovedAt: c.ApprovedAt,
		RejectedAt: c.RejectedAt,
	})
}

func nullStringToString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
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
	if c.SiteID <= 0 {
		return fmt.Errorf("siteID must be > 0")
	}

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
func (d *DB) ApproveComment(ctx context.Context, siteID int64, commentID string) (bool, error) {
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
func (d *DB) RejectComment(ctx context.Context, siteID int64, commentID string) (bool, error) {
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
func (d *DB) ListApprovedComments(ctx context.Context, siteID int64) ([]Comment, error) {
	if d == nil || d.SQL == nil {
		return nil, fmt.Errorf("db not initialized")
	}

	if siteID <= 0 {
		return nil, fmt.Errorf("siteID must be > 0")
	}

	rows, err := d.SQL.QueryContext(ctx, `
SELECT id, site_id, entry_id, post_path, parent_id, status, author, email, author_url, body, created_at, COALESCE(approved_at, 0), COALESCE(rejected_at, 0)
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
			&c.ApprovedAt,
			&c.RejectedAt,
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

// ParentExists checks whether a parent comment exists for the given site and post path.
// If requireApproved is true, the parent must have status = 'approved'.
// Returns (true, nil) if a matching parent exists, (false, nil) if not found.
func (d *DB) ParentExists(ctx context.Context, siteID int64, parentID, postPath string, requireApproved bool) (bool, error) {
	if d == nil || d.SQL == nil {
		return false, fmt.Errorf("db not initialized")
	}

	parentID = strings.TrimSpace(parentID)
	postPath = strings.TrimSpace(postPath)

	if siteID <= 0 || parentID == "" || postPath == "" {
		return false, fmt.Errorf("siteID, parentID and postPath are required")
	}

	query := `
SELECT 1
  FROM comments
 WHERE site_id = ?
   AND id = ?
   AND post_path = ?
`
	if requireApproved {
		query += "   AND status = 'approved'\n"
	}
	query += " LIMIT 1;"

	var one int
	err := d.SQL.QueryRowContext(ctx, query, siteID, parentID, postPath).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("parent exists query: %w", err)
	}

	return true, nil
}

func normalizeCommentFilter(f CommentListFilter) (CommentListFilter, error) {
	f.Status = strings.ToLower(strings.TrimSpace(f.Status))
	allowed := make([]int64, 0, len(f.AllowedSiteIDs))
	for _, s := range f.AllowedSiteIDs {
		if s <= 0 {
			continue
		}
		allowed = append(allowed, s)
	}
	f.AllowedSiteIDs = allowed
	if len(f.AllowedSiteIDs) == 0 {
		return f, fmt.Errorf("allowedSiteIDs is required")
	}
	if f.Status == "" {
		f.Status = "pending"
	}
	switch f.Status {
	case "pending", "approved", "rejected", "all":
	default:
		return f, fmt.Errorf("invalid status %q", f.Status)
	}
	if f.Limit < 0 {
		return f, fmt.Errorf("limit must be >= 0")
	}
	if f.Offset < 0 {
		return f, fmt.Errorf("offset must be >= 0")
	}
	return f, nil
}

func (d *DB) CountComments(ctx context.Context, f CommentListFilter) (int64, error) {
	if d == nil || d.SQL == nil {
		return 0, fmt.Errorf("db not initialized")
	}

	f, err := normalizeCommentFilter(f)
	if err != nil {
		return 0, err
	}

	inPlaceholders := strings.Repeat("?,", len(f.AllowedSiteIDs))
	inPlaceholders = strings.TrimSuffix(inPlaceholders, ",")
	query := `
SELECT COUNT(1)
  FROM comments
 WHERE site_id IN (` + inPlaceholders + `)
`
	args := make([]any, 0, len(f.AllowedSiteIDs)+2)
	for _, siteID := range f.AllowedSiteIDs {
		args = append(args, siteID)
	}
	if f.SiteID > 0 {
		query += "   AND site_id = ?\n"
		args = append(args, f.SiteID)
	}
	if f.Status != "all" {
		query += "   AND status = ?\n"
		args = append(args, f.Status)
	}

	var count int64
	if err := d.SQL.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count comments: %w", err)
	}
	return count, nil
}

func (d *DB) ListComments(ctx context.Context, f CommentListFilter) ([]Comment, error) {
	if d == nil || d.SQL == nil {
		return nil, fmt.Errorf("db not initialized")
	}

	f, err := normalizeCommentFilter(f)
	if err != nil {
		return nil, err
	}

	inPlaceholders := strings.Repeat("?,", len(f.AllowedSiteIDs))
	inPlaceholders = strings.TrimSuffix(inPlaceholders, ",")
	query := `
SELECT id, site_id, entry_id, post_path, parent_id, status, author, email, author_url, body, created_at, COALESCE(approved_at, 0), COALESCE(rejected_at, 0)
  FROM comments
 WHERE site_id IN (` + inPlaceholders + `)
`
	args := make([]any, 0, len(f.AllowedSiteIDs)+4)
	for _, siteID := range f.AllowedSiteIDs {
		args = append(args, siteID)
	}
	if f.SiteID > 0 {
		query += "   AND site_id = ?\n"
		args = append(args, f.SiteID)
	}
	if f.Status != "all" {
		query += "   AND status = ?\n"
		args = append(args, f.Status)
	}
	query += " ORDER BY created_at DESC, id DESC\n"

	if f.Limit > 0 {
		query += " LIMIT ?\n"
		args = append(args, f.Limit)
		if f.Offset > 0 {
			query += " OFFSET ?\n"
			args = append(args, f.Offset)
		}
	} else if f.Offset > 0 {
		// SQLite needs LIMIT when OFFSET is used.
		query += " LIMIT -1 OFFSET ?\n"
		args = append(args, f.Offset)
	}

	rows, err := d.SQL.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
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
			&c.ApprovedAt,
			&c.RejectedAt,
		); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate comments: %w", err)
	}

	return out, nil
}
