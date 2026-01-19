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
	Body      string
	CreatedAt int64
}

func (d *DB) InsertComment(ctx context.Context, c Comment) error {
	if d == nil || d.SQL == nil {
		return fmt.Errorf("db not initialized")
	}

	c.SiteID = strings.TrimSpace(c.SiteID)
	c.PostPath = strings.TrimSpace(c.PostPath)
	c.Author = strings.TrimSpace(c.Author)
	c.Body = strings.TrimSpace(c.Body)
	c.Status = strings.TrimSpace(c.Status)

	if c.CreatedAt == 0 {
		c.CreatedAt = time.Now().Unix()
	}

	_, err := d.SQL.ExecContext(ctx, `
INSERT INTO comments (
  id, site_id, entry_id, post_path, parent_id, status, author, body, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
`, c.ID, c.SiteID, c.EntryID, c.PostPath, c.ParentID, c.Status, c.Author, c.Body, c.CreatedAt)

	if err != nil {
		return fmt.Errorf("insert comment: %w", err)
	}

	return nil
}
