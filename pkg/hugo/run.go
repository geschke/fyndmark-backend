package hugo

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/hugocli"
)

type HugoRunner struct {
	SiteID string
}

func (r *HugoRunner) Run(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("hugo runner is nil")
	}
	return RunWithContext(ctx, r.SiteID)
}

func RunWithContext(ctx context.Context, siteId string) error {
	siteId = strings.TrimSpace(siteId)
	if siteId == "" {
		return fmt.Errorf("site_id is required (use --site-id)")
	}

	siteCfg, ok := config.Cfg.CommentSites[siteId]
	if !ok {
		return fmt.Errorf("unknown site_id %q (not found in comment_sites)", siteId)
	}

	// Determine the website repo workdir (same default logic as git checkout).
	workDir := strings.TrimSpace(siteCfg.Git.CloneDir)
	if workDir == "" {
		workDir = filepath.Join(".", "website", siteId)
	} else {
		workDir = filepath.Clean(workDir)
	}

	fmt.Printf("Running Hugo in: %s\n", workDir)

	// Prototype defaults: just run "hugo" with no args.
	// (Later we can add optional config-driven args if needed.)
	return hugocli.Run(ctx, hugocli.RunOptions{
		WorkingDir: workDir,
		HugoBin:    "hugo",
		Args:       nil,
		Timeout:    5 * time.Minute,
	})
}
