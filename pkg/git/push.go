package git

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/geschke/fyndmark/pkg/gitcli"
)

// Push performs its package-specific operation.
func (r *GitRunner) Push(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("git runner is nil")
	}
	return PushWithContext(ctx, r.SiteID)
}

// PushWithContext performs its package-specific operation.
func PushWithContext(ctx context.Context, siteID string) error {
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return fmt.Errorf("site_id is required (use --site-id)")
	}

	workDir, _ := ResolveWorkdir(siteID)

	if err := gitcli.Push(ctx, workDir, 2*time.Minute); err != nil {
		return err
	}

	fmt.Println("Push completed.")
	return nil
}
