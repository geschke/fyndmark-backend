package git

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/gitcli"
)

func Commit(siteID string, message string) error {
	return CommitWithContext(context.Background(), siteID, message)
}

func CommitWithContext(ctx context.Context, siteID string, message string) error {
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return fmt.Errorf("site_id is required (use --site-id)")
	}

	siteCfg, ok := config.Cfg.CommentSites[siteID]
	if !ok {
		return fmt.Errorf("unknown site_id %q (not found in comment_sites)", siteID)
	}

	workDir := strings.TrimSpace(siteCfg.Git.CloneDir)
	if workDir == "" {
		workDir = filepath.Join(".", "website", siteID)
	} else {
		workDir = filepath.Clean(workDir)
	}

	// If nothing changed, do nothing.
	status, err := gitcli.StatusPorcelain(ctx, workDir, 30*time.Second)
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) == "" {
		fmt.Println("Nothing to commit.")
		return nil
	}

	// Stage everything (including new files) and commit.
	if err := gitcli.AddAll(ctx, workDir, 30*time.Second); err != nil {
		return err
	}

	if strings.TrimSpace(message) == "" {
		message = "Update generated content"
	}

	if err := gitcli.Commit(ctx, workDir, message, 30*time.Second); err != nil {
		return err
	}

	fmt.Println("Commit created.")
	return nil
}
