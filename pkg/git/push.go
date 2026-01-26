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

func Push(siteID string) error {
	return PushWithContext(context.Background(), siteID)
}

func PushWithContext(ctx context.Context, siteID string) error {
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

	if err := gitcli.Push(ctx, workDir, 2*time.Minute); err != nil {
		return err
	}

	fmt.Println("Push completed.")
	return nil
}
