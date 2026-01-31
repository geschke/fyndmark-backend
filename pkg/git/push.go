package git

import (
	"context"
	"fmt"
	"strings"
	"time"

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

	workDir, _ := ResolveWorkdir(siteID)

	if err := gitcli.Push(ctx, workDir, 2*time.Minute); err != nil {
		return err
	}

	fmt.Println("Push completed.")
	return nil
}
