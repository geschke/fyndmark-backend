package git

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/geschke/fyndmark/config"
)

func ResolveWorkdir(siteID string) (string, error) {
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return "", fmt.Errorf("site_id is required (use --site-id)")
	}

	siteCfg, ok := config.Cfg.CommentSites[siteID]
	if !ok {
		return "", fmt.Errorf("unknown site_id %q (not found in comment_sites)", siteID)
	}

	gc := siteCfg.Git
	targetDir := strings.TrimSpace(gc.CloneDir)
	if targetDir == "" {
		targetDir = filepath.Join(".", "website", siteID)
	} else {
		targetDir = filepath.Clean(targetDir)
	}
	return targetDir, nil
}
