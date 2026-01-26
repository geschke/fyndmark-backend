package generator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/geschke/fyndmark/config"
	dbpkg "github.com/geschke/fyndmark/pkg/db"
)

// Generate is a small wrapper around GenerateWithContext.
func Generate(siteID string) error {
	return GenerateWithContext(context.Background(), siteID)
}

// GenerateWithContext reads approved comments from SQLite and writes them as markdown
// files into each Hugo page bundle under <bundle>/comments/*.md.
//
// Bundle mapping:
//   - comments.post_path like "/posts/foo/" maps to "<workDir>/content/posts/foo/"
//   - within that directory, files are written to "<bundle>/comments/YYYY-MM-DD-NNN.md"
func GenerateWithContext(ctx context.Context, siteID string) error {
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return fmt.Errorf("site_id is required (use --site-id)")
	}

	siteCfg, ok := config.Cfg.CommentSites[siteID]
	if !ok {
		return fmt.Errorf("unknown site_id %q (not found in comment_sites)", siteID)
	}

	// Resolve repo working directory (same logic as your git wrapper).
	workDir := strings.TrimSpace(siteCfg.Git.CloneDir)
	if workDir == "" {
		workDir = filepath.Join(".", "website", siteID)
	} else {
		workDir = filepath.Clean(workDir)
	}

	// Load timezone for markdown timestamps.
	loc, err := resolveLocation(strings.TrimSpace(siteCfg.Timezone))
	if err != nil {
		return fmt.Errorf("invalid timezone for comment_sites.%s.timezone: %w", siteID, err)
	}

	// Open DB via db package (applies pragmas).
	sqlitePath := strings.TrimSpace(config.Cfg.SQLite.Path)
	if sqlitePath == "" {
		return fmt.Errorf("sqlite.path must be set")
	}

	d, err := dbpkg.Open(sqlitePath)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()

	comments, err := d.ListApprovedComments(ctx, siteID)
	if err != nil {
		return err
	}

	// Group by post_path.
	byPostPath := map[string][]dbpkg.Comment{}
	for _, c := range comments {
		postPath := normalizePostPath(c.PostPath)
		if postPath == "" {
			return fmt.Errorf("invalid post_path in DB (empty after normalization)")
		}
		c.PostPath = postPath
		byPostPath[postPath] = append(byPostPath[postPath], c)
	}

	// Deterministic iteration over bundles.
	postPaths := make([]string, 0, len(byPostPath))
	for p := range byPostPath {
		postPaths = append(postPaths, p)
	}
	sort.Strings(postPaths)

	for _, postPath := range postPaths {
		cs := byPostPath[postPath]

		// Ensure deterministic within bundle.
		sort.SliceStable(cs, func(i, j int) bool {
			if cs[i].CreatedAt != cs[j].CreatedAt {
				return cs[i].CreatedAt < cs[j].CreatedAt
			}
			return cs[i].ID < cs[j].ID
		})

		bundleDir := filepath.Join(workDir, "content", filepath.FromSlash(postPath))
		if !dirExists(bundleDir) {
			// Non-strict mode: skip comments for missing bundles.
			fmt.Printf("WARN: bundle directory not found for post_path %q → %q (skipping)\n", postPath, bundleDir)
			continue
		}

		commentsDir := filepath.Join(bundleDir, "comments")

		// Rebuild mode: remove and recreate comments directory to match DB exactly.
		if err := os.RemoveAll(commentsDir); err != nil {
			return fmt.Errorf("remove comments dir %q: %w", commentsDir, err)
		}
		if err := os.MkdirAll(commentsDir, 0o755); err != nil {
			return fmt.Errorf("create comments dir %q: %w", commentsDir, err)
		}

		// Counter per local day (in configured timezone).
		dayCounters := map[string]int{}

		for _, c := range cs {
			tLocal := time.Unix(c.CreatedAt, 0).In(loc)
			dayKey := tLocal.Format("2006-01-02")

			dayCounters[dayKey]++
			if dayCounters[dayKey] > 999 {
				return fmt.Errorf("more than 999 comments on %s for post_path %q", dayKey, postPath)
			}

			filename := fmt.Sprintf("%s-%03d.md", dayKey, dayCounters[dayKey])
			outPath := filepath.Join(commentsDir, filename)

			replyTo := ""
			if c.ParentID.Valid {
				replyTo = strings.TrimSpace(c.ParentID.String)
			}

			md := renderCommentMarkdown(
				c.ID,
				tLocal,
				c.Author,
				replyTo,
				"approved",
				c.Body,
			)

			if err := os.WriteFile(outPath, []byte(md), 0o644); err != nil {
				return fmt.Errorf("write comment file %q: %w", outPath, err)
			}
		}
	}

	return nil
}

func resolveLocation(tz string) (*time.Location, error) {
	if tz == "" {
		return time.UTC, nil
	}
	if strings.EqualFold(tz, "utc") {
		return time.UTC, nil
	}
	return time.LoadLocation(tz)
}

// normalizePostPath converts DB post_path like "/posts/foo/" to "posts/foo".
func normalizePostPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.Trim(p, "/")
	return p
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	return st.IsDir()
}

// renderCommentMarkdown matches your established front matter structure.
// Note: author_url is currently empty as your DB doesn't contain a URL field.
func renderCommentMarkdown(commentID string, date time.Time, authorName, replyTo, status, body string) string {
	authorName = strings.TrimSpace(authorName)
	replyTo = strings.TrimSpace(replyTo)
	status = strings.TrimSpace(status)

	// Normalize newlines and ensure trailing newline.
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.TrimRight(body, "\n") + "\n"

	return fmt.Sprintf(`---
comment_id: %q
date: %s
author_name: %q
author_url: ""
status: %q
reply_to: %q
---

%s`, commentID, date.Format(time.RFC3339), authorName, status, replyTo, body)
}
