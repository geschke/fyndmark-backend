package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/gitcli"
)

func Checkout(siteID string) error {
	return CheckoutWithContext(context.Background(), siteID)
}

func CheckoutWithContext(ctx context.Context, siteID string) error {
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return fmt.Errorf("site_id is required (use --site-id)")
	}

	siteCfg, ok := config.Cfg.CommentSites[siteID]
	if !ok {
		return fmt.Errorf("unknown site_id %q (not found in comment_sites)", siteID)
	}

	gc := siteCfg.Git
	repoURL := strings.TrimSpace(gc.RepoURL)
	if repoURL == "" {
		return fmt.Errorf("comment_sites.%s.git.repo_url must be set", siteID)
	}

	// Determine target directory (temp if empty).
	targetDir := strings.TrimSpace(gc.CloneDir)

	if targetDir == "" {
		// Default working directory (relative to current working dir)
		targetDir = filepath.Join(".", "website", siteID)
	} else {
		targetDir = filepath.Clean(targetDir)
	}

	// Idempotent behavior: always start with a clean directory.
	_ = os.RemoveAll(targetDir)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create clone dir %q: %w", targetDir, err)
	}

	fmt.Printf("Cloning repo into: %s\n", targetDir)

	return gitcli.Clone(ctx, gitcli.CloneOptions{
		RepoURL:     repoURL,
		Branch:      strings.TrimSpace(gc.Branch),
		AccessToken: strings.TrimSpace(gc.AccessToken),
		TargetDir:   targetDir,
		Depth:       gc.Depth,
		Timeout:     2 * time.Minute,
	})
}
